package fish.nexa.ui.moderation

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import fish.nexa.data.Moderation
import fish.nexa.data.NexaInbox
import fish.nexa.data.Trades
import fish.nexa.ui.Icon
import fish.nexa.ui.Time
import fish.nexa.ui.chat.TradeSide
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.shadowLow
import fish.nexa.ui.theme.sheen
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private const val TRADE_PAGE = 30

/** The status filters, in the web's order (`FILTERS` in TradesTab.tsx). */
private val TRADE_FILTERS = listOf("" to "moderation.trades.all", "accepted" to "moderation.trades.completed", "pending" to "moderation.trades.open", "declined" to "moderation.trades.declined", "cancelled" to "moderation.trades.cancelled")

/**
 * The owner's Trade Log (web `TradesTab.tsx`): every trade since trading began, newest first, each
 * drawn as the two people see it in their messages, one side each.
 */
@Composable
fun TradeLogTab() {
    val scope = rememberCoroutineScope()
    var status by remember { mutableStateOf("") }
    var q by remember { mutableStateOf("") }
    val trades = remember { mutableStateListOf<Trades.Trade>() }
    var loaded by remember { mutableStateOf(false) }
    var more by remember { mutableStateOf(false) }
    var loading by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    // only the newest request may fill the list
    var request by remember { mutableStateOf(0) }
    fun load(before: String? = null) {
        val id = ++request
        loading = true
        failed = false
        scope.launch {
            try {
                val (r, m) = Moderation.tradeLog(status, q, before, TRADE_PAGE)
                if (id != request) return@launch
                if (before == null) trades.clear()
                trades.addAll(r.filter { x -> trades.none { it.id == x.id } })
                more = m
                loaded = true
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                if (id == request) failed = true
            } finally {
                if (id == request) loading = false
            }
        }
    }
    LaunchedEffect(status, q) {
        request++
        more = false
        // a name is looked for once typing stops
        if (q.isNotEmpty()) delay(250)
        loaded = false
        trades.clear()
        load()
    }
    val c = Nx.colors
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PanelPad, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item("tools") {
            Column(Modifier.padding(bottom = 8.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Txt(t("moderation.trades.hint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), lineHeight = Nx.type.sp(19.6f)), color = c.textMuted)
                SearchField(q, { q = it }, t("moderation.trades.search"), Modifier.fillMaxWidth())
                ChipRow(t("moderation.trades.filter")) { for ((id, key) in TRADE_FILTERS) ModChip(t(key), status == id, { status = id }) }
            }
        }
        when {
            failed && trades.isEmpty() -> item("failed") { LoadFailed({ load() }, loading) }
            !loaded -> item("loading") { LoadingRow() }
            trades.isEmpty() -> item("empty") { EmptyState("ArrowLeftRight", if (q.isNotEmpty() || status.isNotEmpty()) t("moderation.noMatches") else t("moderation.trades.none")) }
            else -> {
                items(trades, key = { it.id }) { tr -> TradeLogCard(tr) }
                if (more) item("more") {
                    Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                        NxButton(t("moderation.trades.loadMore"), { load(trades.lastOrNull()?.id) }, kind = ButtonKind.GHOST, size = ButtonSize.TOUCH, loading = loading)
                    }
                }
            }
        }
    }
}

/** `.trade`: who traded with whom, how it ended and when, then what each side gave or offered. */
@Composable
private fun TradeLogCard(tr: Trades.Trade) {
    val c = Nx.colors
    val shape = Radii.Lg
    val people by NexaInbox.people.collectAsState()
    fun named(id: String) = people[id]?.let { it.displayName.ifEmpty { it.username } }?.ifEmpty { null } ?: t("moderation.trades.gone")
    // only a completed trade moved anything; the rest were offers
    fun side(id: String) = t(if (tr.status == "accepted") "moderation.trades.gave" else "moderation.trades.offered", "name" to named(id))
    val label = TRADE_FILTERS.firstOrNull { it.first == tr.status }?.second?.let { t(it) } ?: tr.status
    val icon = when (tr.status) { "accepted" -> "Check"; "pending" -> "Clock"; "declined" -> "X"; else -> "Ban" }
    val iconTint = when (tr.status) { "accepted" -> c.statusOnline; "pending" -> c.warning; else -> c.textMuted }
    val whenMs = tr.resolvedAt ?: tr.createdAt
    Column(
        Modifier.fillMaxWidth().shadowLow(shape, c).ring(shape, c.panelEdge).background(cardFill(c), shape).background(sheen(c), shape).topEdge(shape, c.edgeLight).padding(start = 14.dp, end = 14.dp, top = 12.dp, bottom = 14.dp),
    ) {
        // the people take the whole first line on a phone; how it ended sits under them
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            TradePerson(tr.fromId, named(tr.fromId), people[tr.fromId] != null, Modifier.weight(1f, fill = false))
            Box(Modifier.semantics { contentDescription = t("moderation.trades.with") }) { Icon("ArrowLeftRight", c.textMuted, size = 16.dp) }
            TradePerson(tr.toId, named(tr.toId), people[tr.toId] != null, Modifier.weight(1f, fill = false))
        }
        Row(Modifier.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
                Icon(icon, iconTint, size = 15.dp)
                Txt(label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), fontWeight = FontWeight(650)), color = if (tr.status == "accepted") c.text else c.textMuted, maxLines = 1)
            }
            if (whenMs > 0) Txt(Time.timestamp(whenMs), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontFeatureSettings = "tnum"), color = c.textMuted, maxLines = 1)
        }
        Box(Modifier.padding(vertical = 12.dp).fillMaxWidth().height(1.dp).background(c.borderSubtle))
        // side by side when there is room for both, stacked on a phone (`.sides`)
        BoxWithConstraints(Modifier.fillMaxWidth()) {
            if (maxWidth >= 520.dp) Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                Box(Modifier.weight(1f)) { TradeSide(side(tr.fromId), tr.fromOffer) }
                Box(Modifier.weight(1f)) { TradeSide(side(tr.toId), tr.toOffer) }
            } else Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                TradeSide(side(tr.fromId), tr.fromOffer)
                TradeSide(side(tr.toId), tr.toOffer)
            }
        }
    }
}

/** `.person`: an avatar and a name that open that person's page in the panel. */
@Composable
private fun TradePerson(id: String, name: String, known: Boolean, modifier: Modifier = Modifier) {
    val c = Nx.colors
    val p = NexaInbox.people.collectAsState().value[id]
    Row(
        modifier.clip(Radii.Pill).then(if (known) Modifier.pressable(remember { MutableInteractionSource() }) { goPanel("users/$id") } else Modifier).padding(start = 2.dp, end = 8.dp, top = 2.dp, bottom = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Avatar(id, name, p?.avatarId, 28.dp)
        fish.nexa.ui.chat.PersonName(id.takeIf { known }, name, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(650)), color = if (known) c.textStrong else c.textMuted, maxLines = 1)
    }
}
