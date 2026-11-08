package fish.nexa.ui.moderation

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import fish.nexa.Nexa
import fish.nexa.data.Moderation
import fish.nexa.net.GiftLink
import fish.nexa.ui.Icon
import fish.nexa.ui.Time
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.settings.copyAndSay
import fish.nexa.ui.support.Segmented
import fish.nexa.ui.support.giftLinkUrl
import fish.nexa.ui.support.giftTitle
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.launch

/**
 * The web's GiftLinksTab: gift links the owner makes without paying. Pick a
 * plan and a length, and the link goes on the clipboard to be sent like an
 * invite. Each works once, for whoever claims it first.
 */
@Composable
fun GiftLinksTab() {
    val c = Nx.colors
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var links by remember { mutableStateOf<List<GiftLink>?>(null) }
    var failed by remember { mutableStateOf(false) }
    var plan by remember { mutableStateOf("supporter") }
    var period by remember { mutableStateOf("month") }
    var busy by remember { mutableStateOf(false) }

    fun load() {
        failed = false
        scope.launch {
            try {
                links = Moderation.giftLinks()
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                failed = true
            }
        }
    }
    LaunchedEffect(Unit) { load() }
    fun create() {
        if (busy) return
        busy = true
        scope.launch {
            try {
                val g = Moderation.createGiftLink(plan, period)
                links = listOf(g) + links.orEmpty()
                copyAndSay(context, giftLinkUrl(g.code), t("moderation.gifts.created"))
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
            } finally {
                busy = false
            }
        }
    }
    fun revoke(g: GiftLink) {
        fish.nexa.ui.shell.confirm(t("moderation.gifts.revokeTitle"), t("moderation.gifts.revokeBody"), t("moderation.gifts.revoke")) {
            try {
                Moderation.revokeGiftLink(g.code)
                links = links?.filter { it.code != g.code }
                Nexa.ui.toast(t("moderation.gifts.revoked"), "success")
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
            }
        }
    }

    LazyColumn(Modifier.fillMaxSize(), contentPadding = PanelPad, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        item("new") {
            SheetPanel(t("moderation.gifts.new"), t("moderation.gifts.hint")) {
                Column(Modifier.padding(start = 14.dp, end = 14.dp, bottom = 12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    androidx.compose.foundation.layout.FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        GiftChoice(t("moderation.gifts.plan"), listOf("supporter" to t("support.plan.supporter"), "pro" to t("support.plan.pro")), plan) { plan = it }
                        GiftChoice(t("moderation.gifts.length"), listOf("month" to t("moderation.gifts.month"), "year" to t("moderation.gifts.year")), period) { period = it }
                    }
                    NxButton(t("moderation.gifts.create"), { create() }, size = ButtonSize.TOUCH, icon = "Link2", enabled = !busy)
                }
            }
        }
        item("title") {
            Txt(t("moderation.gifts.list").uppercase(), Modifier.padding(start = 2.dp, top = 10.dp, bottom = 2.dp), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(700), letterSpacing = 0.5.sp), color = c.textMuted)
        }
        val list = links
        when {
            failed -> item("failed") { EmptyState("Gift", null, t("moderation.loadFailed")) { NxButton(t("moderation.retry"), { load() }, kind = ButtonKind.SECONDARY) } }
            list == null -> item("loading") { LoadingRow() }
            list.isEmpty() -> item("none") { EmptyState("Gift", null, t("moderation.gifts.none")) }
            else -> items(list, key = { it.code }) { g -> GiftRow(g, onCopy = { copyAndSay(context, giftLinkUrl(g.code), t("support.giftLink.copied")) }, onRevoke = { revoke(g) }) }
        }
    }
}

@Composable
private fun GiftChoice(label: String, options: List<Pair<String, String>>, picked: String, onPick: (String) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Txt(label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontWeight = FontWeight(600)), color = Nx.colors.text)
        Segmented(options, picked, onPick, label, fill = false)
    }
}

@Composable
private fun GiftRow(g: GiftLink, onCopy: () -> Unit, onRevoke: () -> Unit) {
    val c = Nx.colors
    Row(
        Modifier.fillMaxWidth().clip(Radii.Md).background(c.bgSurface, Radii.Md).topEdge(Radii.Md, c.edgeLight).padding(start = 12.dp, top = 10.dp, bottom = 10.dp, end = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Box(Modifier.size(32.dp).background(if (g.claimed) c.bgInput else c.brand.copy(alpha = 0.14f), CircleShape), contentAlignment = Alignment.Center) {
            Icon(if (g.claimed) "Check" else "Link2", if (g.claimed) c.textMuted else c.textBrand, size = 16.dp)
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
            Txt(giftTitle(g.plan, g.days), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(600)), color = c.textStrong, maxLines = 2)
            val state = if (g.claimed) t("support.giftLink.claimedBy", "name" to g.claimedName.ifEmpty { t("support.giftLink.someone") }) else t("support.giftLink.waiting")
            Txt("$state · ${Time.relative(g.createdAt)}", style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted, maxLines = 3)
        }
        if (!g.claimed) {
            NxButton(t("support.giftLink.copy"), onCopy, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH, icon = "Copy")
            Row(
                Modifier.height(40.dp).clip(RoundedCornerShape(8.dp)).pressable(remember { MutableInteractionSource() }, onClick = onRevoke).padding(horizontal = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(5.dp),
            ) {
                Icon("Trash2", c.textDanger, size = 15.dp)
                Txt(t("moderation.gifts.revoke"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textDanger, maxLines = 1)
            }
        }
    }
}
