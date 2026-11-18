package fish.nexa.ui.moderation

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import fish.nexa.Nexa
import fish.nexa.data.ModGems
import fish.nexa.data.ModUser
import fish.nexa.data.Moderation
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.Txt
import fish.nexa.ui.describeError
import fish.nexa.ui.gems.GemIcon
import fish.nexa.ui.gems.gemNumber
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.support.Segmented
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import kotlinx.coroutines.launch

/** The most given or taken at once (store.MaxTradeGems). */
private const val MAX_GEMS = 1_000_000L
private const val REASON_MAX = 300

/**
 * The web's GemsModal: the owner's Give or Take Gems, with what the person can
 * spend, the change typed and what they will have after it, in the same place.
 * Held Gems are left to the check, and taking stops at what they can spend.
 */
fun openGemsSheet(u: ModUser) {
    var wallet by mutableStateOf<ModGems?>(null)
    var error by mutableStateOf<String?>(null)
    var mode by mutableStateOf("give")
    var amount by mutableStateOf("")
    var reason by mutableStateOf("")
    var busy by mutableStateOf(false)
    fun n(): Long = amount.trim().toLongOrNull() ?: 0L
    fun tooMany() = wallet != null && mode == "take" && n() > wallet!!.gems
    fun ok() = wallet != null && n() in 1..MAX_GEMS && !tooMany() && reason.length <= REASON_MAX
    val title = t("moderation.gems.title", "name" to u.displayName)
    openSheet("owner-gems", title = title, heading = { fish.nexa.ui.chat.PersonHeading(fish.nexa.ui.chat.PersonWords(t("moderation.gems.title", "name" to fish.nexa.ui.chat.PERSON_SLOT), listOf(u.id to u.displayName))) }, footer = { close ->
        val scope = rememberCoroutineScope()
        fun save() {
            if (!ok() || busy) return
            busy = true
            scope.launch {
                try {
                    val w = Moderation.changeGems(u.id, if (mode == "give") n() else -n(), reason.trim())
                    Nexa.ui.toast(t("moderation.gems.done", "name" to u.displayName, "gems" to t("gems.count", "count" to w.gems)), "success")
                    close()
                } catch (e: Exception) {
                    if (e is kotlinx.coroutines.CancellationException) throw e
                    Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
                } finally {
                    busy = false
                }
            }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
            NxButton(t("chat.editMessage.cancel"), { close() }, kind = ButtonKind.SECONDARY)
            NxButton(
                t(if (mode == "give") "moderation.gems.giveButton" else "moderation.gems.takeButton"), { save() },
                kind = if (mode == "give") ButtonKind.POSITIVE else ButtonKind.DANGER, loading = busy, enabled = ok(),
            )
        }
    }) {
        val c = Nx.colors
        LaunchedEffect(Unit) {
            try {
                wallet = Moderation.gems(u.id)
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                error = describeError(e, t("moderation.loadFailed"))
            }
        }
        error?.let { e -> DangerBox { Txt(e, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text) } }
        val w = wallet
        if (w == null && error == null) LoadingRow()
        if (w != null) {
            val num = n()
            val outOfRange = amount.isNotBlank() && num !in 1..MAX_GEMS
            val after = if (ok()) w.gems + (if (mode == "give") num else -num) else null
            Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                // the one figure that matters, and what it becomes, in the same place
                Column(
                    Modifier.fillMaxWidth().background(c.bgInput, RoundedCornerShape(14.dp)).padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    val figure = Nx.type.title.copy(fontSize = Nx.type.sp(28f), lineHeight = Nx.type.sp(31f), fontWeight = FontWeight(690), letterSpacing = 0.sp)
                    Row(Modifier.semantics { liveRegion = LiveRegionMode.Polite }, verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        GemIcon(26.dp)
                        Txt(gemNumber(w.gems), style = figure, color = if (after != null) c.textMuted else c.textStrong, maxLines = 1)
                        if (after != null) {
                            Icon("ArrowRight", c.textMuted, size = 18.dp, modifier = Modifier.graphicsLayer { if (fish.nexa.ui.I18n.rtl) scaleX = -1f })
                            Txt(gemNumber(after), style = figure, color = if (mode == "give") c.statusOnline else c.textDanger, maxLines = 1)
                        }
                    }
                    Txt(t(if (after != null) "moderation.gems.after" else "moderation.gems.now"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
                }
                if (w.held > 0 || w.state == "check" || w.state == "report") NoteBox(t("moderation.gems.held", "count" to w.held))
                Segmented(
                    listOf("give" to t("moderation.gems.give"), "take" to t("moderation.gems.take")),
                    mode, { mode = it }, t("moderation.gems.menu"), fill = false,
                )
                NxTextField(
                    amount, { v -> amount = v.filter { it.isDigit() }.take(7) },
                    label = t("moderation.gems.amount"),
                    error = when {
                        tooMany() -> t("moderation.gems.tooMany", "count" to w.gems)
                        outOfRange -> t("moderation.gems.range")
                        else -> null
                    },
                    keyboardType = KeyboardType.Number,
                    imeAction = ImeAction.Next,
                )
                NxTextField(
                    reason, { v -> reason = v.take(REASON_MAX) },
                    label = t("moderation.gems.reason"),
                    hint = t("moderation.gems.reasonHint"),
                    singleLine = false, minLines = 2, capitalize = true, autoCorrect = true,
                    imeAction = ImeAction.Default,
                )
            }
        }
    }
}
