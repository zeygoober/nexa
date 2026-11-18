package fish.nexa.ui.moderation

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import fish.nexa.Nexa
import fish.nexa.data.ModCollectables
import fish.nexa.data.ModUser
import fish.nexa.data.Moderation
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.Txt
import fish.nexa.ui.describeError
import fish.nexa.ui.gems.rarityColor
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.profile.BadgePill
import fish.nexa.ui.support.Segmented
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.ring
import kotlinx.coroutines.launch

/** The most given or taken at once (store.MaxOwnerCopies). */
private const val MAX_COPIES = 100
private const val REASON_MAX = 300

private fun keyOf(id: String, material: Int) = "event:$id-$material"

/**
 * The web's CollectablesModal: the owner's Give or Take Collectables. Pick a
 * collectable, then a material with the copies held of each in sight, then how
 * many; laid out as the Gems sheet is, so the two owner tools work alike.
 */
fun openCollectablesSheet(u: ModUser) {
    var data by mutableStateOf<ModCollectables?>(null)
    var error by mutableStateOf<String?>(null)
    var pick by mutableStateOf<String?>(null)
    var material by mutableIntStateOf(1)
    var mode by mutableStateOf("give")
    var amount by mutableStateOf("")
    var reason by mutableStateOf("")
    var busy by mutableStateOf(false)
    fun n(): Int = amount.trim().toIntOrNull() ?: 0
    fun have(): Int = pick?.let { data?.held?.get(keyOf(it, material)) } ?: 0
    fun tooMany() = mode == "take" && n() > have()
    fun ok() = data?.sets?.any { it.id == pick } == true && n() in 1..MAX_COPIES && !tooMany() && reason.length <= REASON_MAX
    val title = t("moderation.collectables.title", "name" to u.displayName)
    openSheet("owner-collectables", title = title, heading = { fish.nexa.ui.chat.PersonHeading(fish.nexa.ui.chat.PersonWords(t("moderation.collectables.title", "name" to fish.nexa.ui.chat.PERSON_SLOT), listOf(u.id to u.displayName))) }, footer = { close ->
        val scope = rememberCoroutineScope()
        fun save() {
            val set = data?.sets?.firstOrNull { it.id == pick } ?: return
            if (!ok() || busy) return
            busy = true
            val m = material
            scope.launch {
                try {
                    val copies = Moderation.changeCollectable(u.id, keyOf(set.id, m), if (mode == "give") n() else -n(), reason.trim())
                    Nexa.ui.toast(
                        t("moderation.collectables.done", "name" to u.displayName, "copies" to t("badges.index.copies", "count" to copies), "badge" to set.name, "material" to t("gems.material.$m")),
                        "success",
                    )
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
                t(if (mode == "give") "moderation.collectables.giveButton" else "moderation.collectables.takeButton"), { save() },
                kind = if (mode == "give") ButtonKind.POSITIVE else ButtonKind.DANGER, loading = busy, enabled = ok(),
            )
        }
    }) {
        val c = Nx.colors
        LaunchedEffect(Unit) {
            try {
                val d = Moderation.collectables(u.id)
                data = d
                pick = d.sets.firstOrNull()?.id
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                error = describeError(e, t("moderation.loadFailed"))
            }
        }
        error?.let { e -> DangerBox { Txt(e, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text) } }
        val d = data
        if (d == null && error == null) LoadingRow()
        if (d != null && d.sets.isEmpty()) NoteBox(t("moderation.collectables.none"))
        val set = d?.sets?.firstOrNull { it.id == pick }
        if (d != null && set != null) {
            val num = n()
            val have = have()
            val outOfRange = amount.isNotBlank() && num !in 1..MAX_COPIES
            val after = if (ok()) have + (if (mode == "give") num else -num) else null
            val round = RoundedCornerShape(14.dp)
            Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                // every collectable, the chosen one ringed; scrolls inside itself past about four rows
                Column(
                    Modifier.fillMaxWidth().heightIn(max = 196.dp).background(c.bgInput, round)
                        .verticalScroll(rememberScrollState()).padding(4.dp).selectableGroup(),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    d.sets.forEach { s ->
                        val on = s.id == pick
                        val count = (1..5).sumOf { d.held[keyOf(s.id, it)] ?: 0 }
                        val inner = RoundedCornerShape(10.dp)
                        Row(
                            Modifier.fillMaxWidth().heightIn(min = 44.dp)
                                .then(if (on) Modifier.background(c.bgRaised, inner).border(2.dp, c.brand, inner) else Modifier)
                                .selectable(on, role = Role.RadioButton) { pick = s.id }
                                .padding(horizontal = 10.dp, vertical = 6.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(10.dp),
                        ) {
                            s.materials.firstOrNull()?.let { BadgePill(it, "0", 0L, "", still = true, size = 28.dp) }
                            Txt(s.name, style = Nx.type.body.copy(fontSize = Nx.type.sp(14.5f), fontWeight = FontWeight(600)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f))
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                Box(Modifier.size(8.dp).background(rarityColor(s.rarity), CircleShape))
                                Txt(t("gems.rarity.${s.rarity}"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, maxLines = 1)
                            }
                            Txt(
                                if (count > 0) t("badges.index.copies", "count" to count) else "",
                                style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontFeatureSettings = "tnum", textAlign = TextAlign.End),
                                color = c.textMuted, maxLines = 1, modifier = Modifier.widthIn(min = 52.dp),
                            )
                        }
                    }
                }
                // the five materials, each with the copies held
                Row(Modifier.fillMaxWidth().selectableGroup(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    set.materials.forEachIndexed { i, b ->
                        val k = i + 1
                        val q = d.held[keyOf(set.id, k)] ?: 0
                        val on = material == k
                        val name = t("gems.material.$k")
                        Column(
                            Modifier.weight(1f).selectable(on, role = Role.RadioButton) { material = k }
                                .semantics(mergeDescendants = true) { contentDescription = if (q > 0) "$name, ${t("badges.index.copies", "count" to q)}" else name },
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(6.dp),
                        ) {
                            Box(
                                Modifier.fillMaxWidth().widthIn(max = 68.dp).aspectRatio(1f).background(c.bgSurface, round)
                                    .border(if (on) 2.dp else 1.dp, if (on) c.brand else c.borderSubtle, round),
                                contentAlignment = Alignment.Center,
                            ) {
                                BadgePill(b, "0", 0L, "", still = true, size = 36.dp)
                                if (q > 0) Txt(
                                    "×$q",
                                    modifier = Modifier.align(Alignment.BottomEnd).padding(3.dp).background(c.bgBase, RoundedCornerShape(999.dp)).ring(RoundedCornerShape(999.dp), c.border).padding(horizontal = 5.dp, vertical = 1.dp),
                                    style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(11f), lineHeight = Nx.type.sp(16f), fontWeight = FontWeight(700), fontFeatureSettings = "tnum"),
                                    color = c.textStrong, maxLines = 1,
                                )
                            }
                            Txt(
                                name,
                                style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12f), fontWeight = if (on) FontWeight(600) else FontWeight.Normal),
                                color = if (on) c.textStrong else c.textMuted, maxLines = 1,
                            )
                        }
                    }
                }
                // the one figure that matters, and what it becomes, in the same place
                Column(
                    Modifier.fillMaxWidth().background(c.bgInput, round).padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    val figure = Nx.type.title.copy(fontSize = Nx.type.sp(28f), lineHeight = Nx.type.sp(31f), fontWeight = FontWeight(690), letterSpacing = 0.sp, fontFeatureSettings = "tnum")
                    Row(Modifier.semantics { liveRegion = LiveRegionMode.Polite }, verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        set.materials.getOrNull(material - 1)?.let { BadgePill(it, "0", 0L, "", still = true, size = 26.dp) }
                        Txt(have.toString(), style = figure, color = if (after != null) c.textMuted else c.textStrong, maxLines = 1)
                        if (after != null) {
                            Icon("ArrowRight", c.textMuted, size = 18.dp, modifier = Modifier.graphicsLayer { if (fish.nexa.ui.I18n.rtl) scaleX = -1f })
                            Txt(after.toString(), style = figure, color = if (mode == "give") c.statusOnline else c.textDanger, maxLines = 1)
                        }
                    }
                    Txt(t(if (after != null) "moderation.collectables.after" else "moderation.collectables.now"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
                }
                Segmented(
                    listOf("give" to t("moderation.gems.give"), "take" to t("moderation.gems.take")),
                    mode, { mode = it }, t("moderation.collectables.menu"), fill = false,
                )
                NxTextField(
                    amount, { v -> amount = v.filter { it.isDigit() }.take(3) },
                    label = t("moderation.gems.amount"),
                    error = when {
                        tooMany() -> t("moderation.collectables.tooMany", "count" to have)
                        outOfRange -> t("moderation.collectables.range")
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
