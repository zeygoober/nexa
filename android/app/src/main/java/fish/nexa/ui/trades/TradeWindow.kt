package fish.nexa.ui.trades

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Trades
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.Spinner
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.profile.BadgePill
import fish.nexa.ui.shell.Names
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** How long a change waits for the next one before it is sent (TradeWindow.tsx SEND_AFTER_MS). */
private const val SEND_AFTER_MS = 250L
/** How many slots the other person's side always shows, empty or not. */
private const val MIN_SLOTS = 6

/** What the window and its footer share: this account's side as it is being picked, and what is on its way. */
private class TradeControl(val userId: String?, val tradeId: String?) {
    var id by mutableStateOf(tradeId)
    var mine by mutableStateOf<List<Trades.Line>?>(null)
    var failed by mutableStateOf(false)
    val sel = mutableStateMapOf<String, Int>()
    var q by mutableStateOf("")
    var busy by mutableStateOf(false)
    var dirty = false
    var job: Job? = null
    var endedByMe = false

    private val me get() = Nexa.users.meId

    private fun adopt(t: Trades.Trade) {
        sel.clear()
        t.offerOf(me).forEach { sel[it.key] = it.qty }
    }

    fun load() {
        failed = false
        Nexa.scope.launch {
            try {
                val t = if (tradeId != null) Trades.get(tradeId) else Trades.open(userId ?: "")
                id = t.id
                adopt(t)
                mine = Trades.mine()
            } catch (e: Exception) {
                failed = true
                Nexa.ui.toast(describeError(e, t("trades.couldNotOpen")), "error")
            }
        }
    }

    /** Sends a change that is waiting; the trade as it stands after. A change on its way is taken over, not repeated. */
    suspend fun flush(): Trades.Trade? {
        job?.cancel()
        return send()
    }

    private suspend fun send(): Trades.Trade? {
        val now = id ?: return null
        if (!dirty) return Trades.byId.value[now]
        val sent = sel.toMap()
        return try {
            Trades.offer(now, sent).also { if (sel.toMap() == sent) dirty = false }
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (e: Exception) {
            dirty = false
            Nexa.ui.toast(describeError(e, t("trades.couldNotUpdate")), "error")
            runCatching { Trades.get(now) }.getOrNull()?.also { adopt(it) }
        }
    }

    fun change(key: String, n: Int) {
        if (n <= 0) sel.remove(key) else sel[key] = n
        dirty = true
        job?.cancel()
        job = Nexa.scope.launch { delay(SEND_AFTER_MS); send() }
    }

    fun press(ready: Boolean) {
        val now = id ?: return
        if (busy) return
        busy = true
        Nexa.scope.launch {
            try {
                val waiting = dirty
                val trade = flush() ?: return@launch
                // a change just sent cleared the marks, so what is wanted then is Ready
                Trades.ready(now, if (waiting) true else !ready, trade.version)
            } catch (e: Exception) {
                Nexa.ui.toast(describeError(e, t("trades.couldNotUpdate")), "error")
                runCatching { Trades.get(now) }
            } finally {
                busy = false
            }
        }
    }

    fun cancel(close: () -> Unit) {
        val now = id
        val pending = now?.let { Trades.byId.value[it]?.pending } == true
        if (now == null || !pending) return close()
        endedByMe = true
        Nexa.scope.launch {
            try {
                Trades.end(now)
                close()
            } catch (e: Exception) {
                endedByMe = false
                Nexa.ui.toast(describeError(e, t("trades.couldNotUpdate")), "error")
            }
        }
    }
}

/**
 * TradeWindow.tsx: Trade Badges, a window two people share. The other person's
 * offer is the slots on top, yours the grid of badges you hold below it. Each
 * change goes to the server at once and to them live, and clears both Ready
 * marks; when both of you are ready, it happens. Opened with a person it starts
 * the trade (or finds the one already open), with a trade it joins it, which
 * works just as well when the other person comes by later.
 */
fun openTradeWindow(userId: String? = null, tradeId: String? = null) {
    val ctl = TradeControl(userId, tradeId)
    ctl.load()
    openSheet(
        key = "trade",
        title = t("trades.title"),
        canClose = { true },
        footer = { close -> TradeFooter(ctl, close) },
    ) { close -> TradeBody(ctl, close) }
}

@Composable
private fun TradeFooter(ctl: TradeControl, close: () -> Unit) {
    val trades by Trades.byId.collectAsState()
    val me = Nexa.users.meId
    val trade = ctl.id?.let { trades[it] }
    val ready = trade?.readyOf(me) == true
    Row(Modifier.fillMaxWidth().padding(top = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
        NxButton(t("chat.editMessage.cancel"), onClick = { ctl.cancel(close) }, kind = ButtonKind.DANGER)
        NxButton(
            if (ready) t("trades.notReady") else t("trades.ready"),
            onClick = { ctl.press(ready) },
            kind = if (ready) ButtonKind.SECONDARY else ButtonKind.POSITIVE,
            loading = ctl.busy,
            enabled = trade?.pending == true && ctl.mine != null,
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun TradeBody(ctl: TradeControl, close: () -> Unit) {
    val c = Nx.colors
    val trades by Trades.byId.collectAsState()
    val users by Nexa.users.byId.collectAsState()
    val self by Nexa.users.me.collectAsState()
    val me = self?.id
    val trade = ctl.id?.let { trades[it] }
    val otherId = trade?.otherId(me) ?: ctl.userId ?: ""
    val other = users[otherId]
    val otherName = Names.display(other)

    // what the server holds of mine is what shows, unless a change of mine is on its way
    LaunchedEffect(trade?.id, trade?.version) {
        if (!ctl.dirty && trade != null) {
            ctl.sel.clear()
            trade.offerOf(me).forEach { ctl.sel[it.key] = it.qty }
        }
    }
    // it ended: by the other person, or by the trade happening
    LaunchedEffect(trade?.status) {
        if (trade != null && !trade.pending && !ctl.endedByMe) {
            Nexa.ui.toast(if (trade.status == "accepted") t("trades.done") else t("trades.endedByThem", "name" to otherName), "info")
            close()
        }
    }

    val mine = ctl.mine
    if (ctl.failed) {
        Txt(t("trades.couldNotLoad"), color = c.textMuted, modifier = Modifier.padding(vertical = 16.dp))
        NxButton(t("chat.messageList.tryAgain"), onClick = { ctl.load() }, kind = ButtonKind.SECONDARY)
        return
    }
    if (trade == null || mine == null) {
        Box(Modifier.fillMaxWidth().height(160.dp), contentAlignment = Alignment.Center) { Spinner() }
        return
    }
    val myReady = trade.readyOf(me)
    val theirReady = trade.readyOfOther(me)
    val theirOffer = trade.offerOfOther(me)

    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        // the other person's offer, filling slots as they pick
        Column(
            Modifier.fillMaxWidth().ring(Radii.Md, if (theirReady) c.positive else Nx.colors.bgInput, 2.dp).background(c.bgInput, Radii.Md).padding(10.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Avatar(otherId, otherName, other?.avatarId, 28.dp)
                Txt(t("trades.theirOffer", "name" to otherName), style = Nx.type.heading.copy(fontSize = Nx.type.sp(14f)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f))
                if (theirReady) ReadyTag()
            }
            val slots = maxOf(MIN_SLOTS, (theirOffer.size + 2) / 3 * 3)
            for (row in 0 until slots / 3) {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (col in 0 until 3) {
                        val l = theirOffer.getOrNull(row * 3 + col)
                        Box(
                            Modifier.weight(1f).aspectRatio(1f).let { m ->
                                if (l == null) m.ring(Radii.Sm, c.border) else m.background(c.bgSurface, Radii.Sm).ring(Radii.Sm, c.borderSubtle)
                            },
                            contentAlignment = Alignment.Center,
                        ) {
                            if (l?.badge != null) BadgePill(l.badge, "0", 0L, "", size = 36.dp)
                            if (l != null && l.qty > 1) Chip("×${l.qty}", c.textStrong, Modifier.align(Alignment.BottomEnd).padding(3.dp))
                        }
                    }
                }
            }
            Txt(
                if (theirReady) t("trades.theirReady", "name" to otherName) else t("trades.theirNotReady", "name" to otherName),
                style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted,
            )
        }

        // my badges: tap adds one, double tap adds them all
        Column(
            Modifier.fillMaxWidth().ring(Radii.Md, if (myReady) c.positive else c.bgInput, 2.dp).background(c.bgInput, Radii.Md).padding(10.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Avatar(me ?: "", self?.displayName?.ifBlank { null } ?: self?.username ?: "", self?.avatarId, 28.dp)
                Txt(t("trades.yourOffer"), style = Nx.type.heading.copy(fontSize = Nx.type.sp(14f)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f))
                if (myReady) ReadyTag()
            }
            if (mine.isEmpty()) {
                Txt(t("trades.noneToGive"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted, modifier = Modifier.padding(4.dp))
            } else {
                val needle = ctl.q.trim().lowercase()
                val shown = mine.filter { l -> needle.isEmpty() || (l.badge?.let { fish.nexa.data.Badges.look(it, "0", 0L, "")?.label } ?: l.key).lowercase().contains(needle) }
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (l in shown) {
                        val n = ctl.sel[l.key] ?: 0
                        val name = l.badge?.let { fish.nexa.data.Badges.look(it, "0", 0L, "")?.label } ?: l.key
                        val say = "$name, ${t("trades.have", "count" to l.qty)}"
                        Box(Modifier.size(64.dp).padding(2.dp)) {
                            Box(
                                Modifier.fillMaxWidth().aspectRatio(1f)
                                    .background(if (n > 0) c.bgSelected else c.bgSurface, Radii.Sm)
                                    .ring(Radii.Sm, if (n > 0) c.brand else c.borderSubtle, if (n > 0) 2.dp else 1.dp)
                                    .semantics { contentDescription = say }
                                    .pointerInput(l.key, l.qty, n) {
                                        detectTapGestures(onDoubleTap = { ctl.change(l.key, l.qty) }, onTap = { ctl.change(l.key, minOf(l.qty, n + 1)) })
                                    },
                                contentAlignment = Alignment.Center,
                            ) {
                                if (l.badge != null) BadgePill(l.badge, "0", 0L, "", still = true, size = 36.dp)
                                if (n > 0) Chip("$n/${l.qty}", c.textBrand, Modifier.align(Alignment.BottomEnd).padding(3.dp))
                                else if (l.qty > 1) Chip("×${l.qty}", c.textStrong, Modifier.align(Alignment.BottomEnd).padding(3.dp))
                            }
                            if (n > 0) Box(
                                Modifier.align(Alignment.TopEnd).size(22.dp).background(c.bgBase, CircleShape).ring(CircleShape, c.borderStrong)
                                    .pressable(remember { MutableInteractionSource() }) { ctl.change(l.key, n - 1) }
                                    .semantics { contentDescription = t("trades.removeOne") },
                                contentAlignment = Alignment.Center,
                            ) { Icon("Minus", c.textSubtle, size = 12.dp) }
                        }
                    }
                }
                NxTextField(
                    value = ctl.q,
                    onValueChange = { ctl.q = it },
                    placeholder = t("trades.search"),
                    leading = { Icon("Search", c.textPlaceholder, size = 16.dp) },
                )
            }
        }
        Txt(t("trades.hint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
    }
}

/** A small count over a tile's corner, in a pill of its own. */
@Composable
private fun Chip(text: String, color: androidx.compose.ui.graphics.Color, modifier: Modifier) {
    val c = Nx.colors
    Txt(
        text,
        modifier = modifier.background(c.bgBase, RoundedCornerShape(999.dp)).ring(RoundedCornerShape(999.dp), c.border).padding(horizontal = 5.dp),
        style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(11f), lineHeight = Nx.type.sp(16f), fontWeight = FontWeight(700), fontFeatureSettings = "tnum"),
        color = color,
        maxLines = 1,
    )
}

/** The green mark of someone who pressed Ready. */
@Composable
internal fun ReadyTag() {
    val c = Nx.colors
    Row(
        Modifier.background(c.positive, RoundedCornerShape(999.dp)).padding(start = 7.dp, end = 9.dp, top = 1.dp, bottom = 1.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Icon("Check", c.onPositive, size = 14.dp)
        Txt(t("trades.ready"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(700)), color = c.onPositive, maxLines = 1)
    }
}
