package fish.nexa.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.ui.AbsoluteAlignment
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.absoluteOffset
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.scale
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import fish.nexa.data.SecretBadges
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Badges
import fish.nexa.data.Ota
import fish.nexa.data.StaffCan
import fish.nexa.data.StaffRanks
import fish.nexa.media.Media
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.pressable
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.components.Txt
import fish.nexa.ui.profile.BadgeMark
import fish.nexa.ui.profile.BadgePill
import fish.nexa.ui.server.NoSwipe
import fish.nexa.ui.support.Segmented
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import kotlin.math.floor
import kotlin.math.roundToInt
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull

/** One place in the index: the badge when it is held, otherwise only what draws its shape. */
private class IndexEntry(val art: String?, val icon: String?, val image: String?, val badge: Badges.Badge?, val category: String, val qty: Int = 0)

/** The parts of the index, always in this order (web BADGE_SECTIONS). */
private val SECTIONS = listOf("obtainable", "exclusive", "special")

private class BadgeIndex(val collected: Int, val total: Int, val entries: List<IndexEntry>)

private fun parseIndex(el: kotlinx.serialization.json.JsonElement): BadgeIndex {
    val o = el as JsonObject
    fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.contentOrNull
    return BadgeIndex(
        (o["collected"] as? JsonPrimitive)?.intOrNull ?: 0,
        (o["total"] as? JsonPrimitive)?.intOrNull ?: 0,
        (o["entries"] as? JsonArray).orEmpty().mapNotNull { e ->
            val x = e as? JsonObject ?: return@mapNotNull null
            IndexEntry(x.s("art"), x.s("icon"), x.s("image"), x["badge"]?.let { Badges.parse(JsonArray(listOf(it)))?.firstOrNull() }, x.s("category") ?: "obtainable", (x["qty"] as? JsonPrimitive)?.intOrNull ?: 0)
        },
    )
}

/** The narrowest a tile gets and the gap between them (BadgeIndexPage.module.css). */
private const val TILE = 64
private const val GAP = 10

/**
 * Settings, Badge Index (web BadgeIndexPage.tsx): every badge there is, each
 * level its own, the ones this account lacks blacked out with nothing that
 * says what they are or how to get them. Staff ranks are not in it.
 */
@Composable
fun BadgeIndexPage() {
    val c = Nx.colors
    val me by Nexa.users.me.collectAsState()
    var index by remember { mutableStateOf<BadgeIndex?>(null) }
    var failed by remember { mutableStateOf(false) }
    var attempt by remember { mutableIntStateOf(0) }
    var arranging by remember { mutableStateOf(false) }
    var describing by remember { mutableStateOf(false) }
    LaunchedEffect(attempt) {
        failed = false
        try {
            index = parseIndex(Nexa.api.get("/users/@me/badges/index"))
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            failed = true
        }
    }
    PageTitle(t("badges.index.title"))
    val i = index
    val who = me
    if (failed) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Txt(t("badges.index.failed"), style = Nx.type.body, color = c.textMuted)
            NxButton(t("chat.messageList.tryAgain"), { attempt += 1 }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
        }
        return
    }
    if (i == null || who == null) return
    if (arranging) {
        Arrange(who.id, who.flags) { saved ->
            arranging = false
            if (saved) attempt += 1
        }
        return
    }
    if (describing) {
        Describe(who.id, who.flags) {
            describing = false
            attempt += 1
        }
        return
    }
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.weight(1f), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
            Txt(i.collected.toString(), style = Nx.type.title.copy(fontSize = Nx.type.sp(28f), lineHeight = Nx.type.sp(28f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum"), color = c.textStrong)
            Txt(t("badges.index.of", "total" to i.total) + " " + t("badges.index.collected"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
        }
        if (StaffRanks.can(StaffCan.OWNER_TOOLS)) Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            NxButton(t("badges.index.describe"), { describing = true }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
            NxButton(t("badges.index.arrange"), { arranging = true }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
        }
    }
    Spacer(Modifier.height(10.dp))
    Box(Modifier.fillMaxWidth().height(6.dp).background(c.bgActive, RoundedCornerShape(999.dp))) {
        val part = if (i.total > 0) (i.collected.toFloat() / i.total).coerceIn(0f, 1f) else 0f
        if (part > 0f) Box(Modifier.fillMaxWidth(part).height(6.dp).background(c.textBrand, RoundedCornerShape(999.dp)))
    }
    Spacer(Modifier.height(32.dp))
    var first = true
    for (section in SECTIONS) {
        val entries = i.entries.filter { it.category == section }
        // Special starts out empty and still says it is there
        if (entries.isEmpty() && section != "special") continue
        if (!first) Spacer(Modifier.height(36.dp))
        first = false
        Txt(t("badges.index.$section"), style = Nx.type.heading.copy(fontSize = Nx.type.sp(18f), lineHeight = Nx.type.sp(24f)), color = c.textStrong, modifier = Modifier.semantics { heading() })
        Spacer(Modifier.height(12.dp))
        if (entries.isEmpty()) {
            Txt(t("badges.index.specialEmpty"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
            continue
        }
        BoxWithConstraints(Modifier.fillMaxWidth()) {
            val columns = (((maxWidth.value + GAP) / (TILE + GAP)).toInt()).coerceAtLeast(1)
            Column(verticalArrangement = Arrangement.spacedBy(GAP.dp)) {
                for (row in entries.chunked(columns)) Row(horizontalArrangement = Arrangement.spacedBy(GAP.dp)) {
                    for (e in row) Box(Modifier.weight(1f)) { Tile(e, who.id, who.flags, who.plan) { attempt += 1 } }
                    repeat(columns - row.size) { Spacer(Modifier.weight(1f)) }
                }
            }
        }
    }
}

@Composable
private fun Tile(e: IndexEntry, userId: String, flags: Long, plan: String, onFound: () -> Unit) {
    val c = Nx.colors
    val b = e.badge
    val scope = rememberCoroutineScope()
    // the same tile whether collected or not: only the mark says which
    val tile = Modifier.fillMaxWidth().aspectRatio(1f).padding(1.dp).background(c.bgSurface, Radii.Md).ring(Radii.Md, c.borderSubtle)
    if (b != null) {
        Box(tile, contentAlignment = Alignment.Center) {
            BadgePill(b, userId, flags, plan, size = 40.dp)
            // how many copies, when trades gave more than one
            if (e.qty > 1) Txt(
                "×${e.qty}",
                modifier = Modifier.align(Alignment.BottomEnd).padding(4.dp).background(c.bgBase, RoundedCornerShape(999.dp)).ring(RoundedCornerShape(999.dp), c.border).padding(horizontal = 5.dp)
                    .semantics { contentDescription = t("badges.index.copies", "count" to e.qty) },
                style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(11f), lineHeight = Nx.type.sp(16f), fontWeight = FontWeight(700), fontFeatureSettings = "tnum"),
                color = c.textStrong,
                maxLines = 1,
            )
        }
        return
    }
    val locked = t("badges.index.locked")
    // a padlock held still long enough is Curious; nothing shows while it is held, and a scroll lets go
    val hold = if (e.art != "secret") Modifier else Modifier.pointerInput(Unit) {
        awaitEachGesture {
            awaitFirstDown(requireUnconsumed = false)
            val ended = withTimeoutOrNull(SecretBadges.CURIOUS_HOLD_MS) { waitForUpOrCancellation(); true }
            if (ended == null) scope.launch { if (SecretBadges.report("curious")) onFound() }
        }
    }
    Box(tile.then(hold).semantics { contentDescription = locked }, contentAlignment = Alignment.Center) {
        when {
            e.art != null -> coil3.compose.AsyncImage(Ota.uri("badges/${e.art}-locked.svg"), null, Modifier.size(40.dp))
            e.image != null -> coil3.compose.AsyncImage(Media.absolute(e.image), null, Modifier.size(40.dp), colorFilter = ColorFilter.tint(Color.Black))
            // a badge staff made, blacked out as the art's -locked files are: solid inside, in the same heavier line
            else -> Icon(e.icon?.takeIf { fish.nexa.ui.Lucide.exists(it) } ?: "award", Color.Black, size = 40.dp, stroke = 2.8f, bodied = true, bodyAlpha = 1f)
        }
    }
}

/** One place while arranging: every badge is shown as held, so the owner can tell its levels apart. */
private class ArrangeItem(val key: String, val badge: Badges.Badge, val category: String)

/** Which plan's badge a plan entry is drawn as, since every plan shows here at once. */
private fun lookOf(item: ArrangeItem, userId: String, flags: Long) =
    Badges.look(item.badge, userId, flags, if (item.key.startsWith("plan-")) item.key.removePrefix("plan-") else "")

/** The owner arranging the Badge Index for everyone, in place of the summary (web BadgeArrange.tsx). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun Arrange(userId: String, flags: Long, done: (saved: Boolean) -> Unit) {
    val c = Nx.colors
    val scope = rememberCoroutineScope()
    val haptics = LocalHapticFeedback.current
    val rtl = LocalLayoutDirection.current == LayoutDirection.Rtl
    var items by remember { mutableStateOf<List<ArrangeItem>?>(null) }
    var aToZ by remember { mutableStateOf(emptyList<String>()) }
    var changed by remember { mutableStateOf(false) }
    var toAlphabetical by remember { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var chosen by remember { mutableStateOf<String?>(null) }
    var heldKey by remember { mutableStateOf<String?>(null) }
    var heldAt by remember { mutableStateOf(Offset.Zero) }
    LaunchedEffect(Unit) {
        try {
            val o = Nexa.api.get("/moderation/badges/index-order") as JsonObject
            items = parseArrangement(o)
            aToZ = (o["a_to_z"] as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.contentOrNull }
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            Nexa.ui.toast(t("badges.index.failed"), "error")
            done(false)
        }
    }
    DisposableEffect(Unit) { onDispose { NoSwipe.active = false } }
    val all = items ?: return
    val parts = SECTIONS.filter { p -> all.any { it.category == p } }
    val part = chosen?.takeIf { it in parts } ?: parts.firstOrNull()
    val list = all.filter { it.category == part }
    Txt(t("badges.index.arrangeTitle"), style = Nx.type.title.copy(fontSize = Nx.type.sp(16f), fontWeight = FontWeight(700)), color = c.textStrong)
    Spacer(Modifier.height(6.dp))
    Txt(t("badges.index.arrangeHint"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
    Spacer(Modifier.height(14.dp))
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        NxButton(t("badges.staff.save"), {
            saving = true
            val order = if (toAlphabetical) emptyList() else items.orEmpty().map { it.key }
            scope.launch {
                try {
                    Nexa.api.request("PUT", "/moderation/badges/index-order", buildJsonObject { put("order", JsonArray(order.map { JsonPrimitive(it) })) })
                    Nexa.ui.toast(t("badges.index.arranged"), "success")
                    done(true)
                } catch (e: Exception) {
                    if (e is kotlinx.coroutines.CancellationException) throw e
                    Nexa.ui.toast(t("badges.index.arrangeFailed"), "error")
                    saving = false
                }
            }
        }, size = ButtonSize.SM, enabled = changed, loading = saving)
        NxButton(t("moderation.cancel"), { done(false) }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, enabled = !saving)
        NxButton(t("badges.index.useAlphabetical"), {
            val byKey = items.orEmpty().associateBy { it.key }
            items = aToZ.mapNotNull { byKey[it] }
            changed = true
            toAlphabetical = true
        }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, enabled = !saving && !toAlphabetical)
    }
    Spacer(Modifier.height(20.dp))
    if (parts.size > 1) {
        Segmented(parts.map { it to t("badges.index.$it") }, part.orEmpty(), { if (heldKey == null) chosen = it }, t("badges.index.arrangeTitle"))
        Spacer(Modifier.height(14.dp))
    }
    BoxWithConstraints(Modifier.fillMaxWidth()) {
        val columns = (((maxWidth.value + GAP) / (TILE + GAP)).toInt()).coerceAtLeast(1)
        val density = LocalDensity.current
        val gap = with(density) { GAP.dp.toPx() }
        val width = with(density) { maxWidth.toPx() }
        val tile = (width - gap * (columns - 1)) / columns
        val step = tile + gap
        val gesture = Modifier.pointerInput(columns, step, part) {
            fun place(p: Offset): Int {
                val column = floor((p.x + gap / 2) / step).toInt().coerceIn(0, columns - 1)
                val row = floor((p.y + gap / 2) / step).toInt().coerceAtLeast(0)
                return row * columns + if (rtl) columns - 1 - column else column
            }
            var grab = Offset.Zero
            fun end() {
                heldKey = null
                NoSwipe.active = false
            }
            detectDragGesturesAfterLongPress(
                onDragStart = { p ->
                    val now = items.orEmpty().filter { it.category == part }
                    if (now.isEmpty() || saving) return@detectDragGesturesAfterLongPress
                    val i = place(p).coerceAtMost(now.size - 1)
                    val column = if (rtl) columns - 1 - i % columns else i % columns
                    grab = p - Offset(column * step, (i / columns) * step)
                    heldAt = p - grab
                    heldKey = now[i].key
                    NoSwipe.active = true
                    haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                },
                onDrag = { change, _ ->
                    val key = heldKey ?: return@detectDragGesturesAfterLongPress
                    change.consume()
                    heldAt = change.position - grab
                    // a place in the grid is a place in this part; the badge goes where the one there stands in the whole list
                    val now = items.orEmpty()
                    val shown = now.filter { it.category == part }
                    val there = shown.getOrNull(place(change.position).coerceAtMost(shown.size - 1))?.key
                    val from = now.indexOfFirst { it.key == key }
                    val to = now.indexOfFirst { it.key == there }
                    if (from >= 0 && to >= 0 && from != to) {
                        items = now.toMutableList().apply { add(to, removeAt(from)) }
                        changed = true
                        toAlphabetical = false
                    }
                },
                onDragEnd = { end() },
                onDragCancel = { end() },
            )
        }
        Column(gesture, verticalArrangement = Arrangement.spacedBy(GAP.dp)) {
            for (row in list.chunked(columns)) Row(horizontalArrangement = Arrangement.spacedBy(GAP.dp)) {
                for (item in row) Box(Modifier.weight(1f)) {
                    val left = item.key == heldKey
                    val look = lookOf(item, userId, flags)
                    val label = look?.label.orEmpty()
                    Box(
                        Modifier.fillMaxWidth().aspectRatio(1f).padding(1.dp).background(if (left) c.bgActive else c.bgSurface, Radii.Md).ring(Radii.Md, c.borderSubtle).semantics { contentDescription = label },
                        contentAlignment = Alignment.Center,
                    ) { if (look != null && !left) BadgeMark(look, 40.dp) }
                }
                repeat(columns - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
        val inHand = heldKey?.let { key -> list.firstOrNull { it.key == key } }
        val look = inHand?.let { lookOf(it, userId, flags) }
        if (look != null) {
            val side = with(density) { tile.toDp() }
            Box(
                Modifier.align(AbsoluteAlignment.TopLeft).absoluteOffset { IntOffset(heldAt.x.roundToInt(), heldAt.y.roundToInt()) }.size(side)
                    .scale(if (Nx.motion.reduced) 1f else 1.08f).shadow(8.dp, Radii.Md).background(c.bgSurface, Radii.Md).border(2.dp, c.textBrand, Radii.Md),
                contentAlignment = Alignment.Center,
            ) { BadgeMark(look, 40.dp) }
        }
    }
}

/** The longest tooltip (store.MaxBadgeTip). */
private const val MAX_TIP = 120

private fun tipLength(s: String) = s.trim().let { it.codePointCount(0, it.length) }

/** Whether the owner's own words stand in for a built-in badge's tooltip. */
private fun ArrangeItem.changed() = !badge.isEvent && !badge.description.isNullOrEmpty()

private fun parseArrangement(o: JsonObject) = (o["entries"] as? JsonArray).orEmpty().mapNotNull { e ->
    val x = e as? JsonObject ?: return@mapNotNull null
    val key = (x["key"] as? JsonPrimitive)?.contentOrNull ?: return@mapNotNull null
    val badge = x["badge"]?.let { Badges.parse(JsonArray(listOf(it)))?.firstOrNull() } ?: return@mapNotNull null
    ArrangeItem(key, badge, (x["category"] as? JsonPrimitive)?.contentOrNull ?: "obtainable")
}

/**
 * The owner writing what a badge's card says, for everyone and in every
 * language, in place of the summary (web BadgeDescriptions.tsx).
 */
@Composable
private fun Describe(userId: String, flags: Long, done: () -> Unit) {
    val c = Nx.colors
    var items by remember { mutableStateOf<List<ArrangeItem>?>(null) }
    var chosen by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(Unit) {
        try {
            items = parseArrangement(Nexa.api.get("/moderation/badges/index-order") as JsonObject)
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            Nexa.ui.toast(t("badges.index.failed"), "error")
            done()
        }
    }
    val all = items ?: return
    val parts = SECTIONS.filter { p -> all.any { it.category == p } }
    val part = chosen?.takeIf { it in parts } ?: parts.firstOrNull()
    Txt(t("badges.index.describeTitle"), style = Nx.type.title.copy(fontSize = Nx.type.sp(16f), fontWeight = FontWeight(700)), color = c.textStrong)
    Spacer(Modifier.height(6.dp))
    Txt(t("badges.index.describeHint"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
    Spacer(Modifier.height(14.dp))
    NxButton(t("badges.index.describeDone"), done, size = ButtonSize.SM)
    Spacer(Modifier.height(20.dp))
    if (parts.size > 1) {
        Segmented(parts.map { it to t("badges.index.$it") }, part.orEmpty(), { chosen = it }, t("badges.index.describeTitle"))
        Spacer(Modifier.height(14.dp))
    }
    BoxWithConstraints(Modifier.fillMaxWidth()) {
        val columns = (((maxWidth.value + GAP) / (TILE + GAP)).toInt()).coerceAtLeast(1)
        Column(verticalArrangement = Arrangement.spacedBy(GAP.dp)) {
            for (row in all.filter { it.category == part }.chunked(columns)) Row(horizontalArrangement = Arrangement.spacedBy(GAP.dp)) {
                for (item in row) Box(Modifier.weight(1f)) {
                    val look = lookOf(item, userId, flags)
                    val label = look?.label.orEmpty() + if (item.changed()) ", " + t("badges.index.describeChanged") else ""
                    Box(
                        Modifier.fillMaxWidth().aspectRatio(1f).padding(1.dp).clip(Radii.Md).background(c.bgSurface).ring(Radii.Md, c.borderSubtle)
                            .pressable(remember { MutableInteractionSource() }, role = Role.Button) {
                                openDescribeSheet(item, userId, flags) { words ->
                                    items = items?.map { if (it.key == item.key) ArrangeItem(it.key, it.badge.copy(description = words.ifEmpty { null }), it.category) else it }
                                }
                            }
                            .semantics { contentDescription = label },
                        contentAlignment = Alignment.Center,
                    ) {
                        if (look != null) BadgeMark(look, 40.dp)
                        if (item.changed()) Box(Modifier.align(Alignment.TopEnd).padding(6.dp).size(8.dp).background(c.textBrand, RoundedCornerShape(999.dp)))
                    }
                }
                repeat(columns - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}

/** The editor for one badge's words: the card as it will read, the text, and Reset, Cancel and Save. */
private fun openDescribeSheet(item: ArrangeItem, userId: String, flags: Long, saved: (String) -> Unit) {
    val event = item.badge.isEvent
    var text by mutableStateOf(if (event || item.changed()) item.badge.description.orEmpty() else "")
    var saving by mutableStateOf(false)
    openSheet("badge-describe", title = t("badges.index.describeEdit"), canClose = { !saving }, footer = { close ->
        val scope = rememberCoroutineScope()
        val long = tipLength(text) > MAX_TIP
        FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (!event) NxButton(t("badges.index.describeReset"), { text = "" }, kind = ButtonKind.SECONDARY, enabled = !saving && text.isNotEmpty())
            NxButton(t("moderation.cancel"), { close() }, kind = ButtonKind.SECONDARY, enabled = !saving)
            NxButton(t("chat.editMessage.save"), {
                val words = text.trim()
                saving = true
                scope.launch {
                    try {
                        Badges.setTip(item.key, words)
                        saved(words)
                        Nexa.ui.toast(t("badges.index.describeSaved"), "success")
                        saving = false
                        close()
                    } catch (e: Exception) {
                        if (e is kotlinx.coroutines.CancellationException) throw e
                        Nexa.ui.toast(t("badges.index.describeFailed"), "error")
                        saving = false
                    }
                }
            }, loading = saving, enabled = !long)
        }
    }) {
        val c = Nx.colors
        val builtIn = if (event) null else lookOf(ArrangeItem(item.key, item.badge.copy(description = null), item.category), userId, flags)
        val words = text.trim()
        val preview = lookOf(ArrangeItem(item.key, item.badge.copy(description = words.ifEmpty { if (event) "" else null }), item.category), userId, flags)
        val length = tipLength(text)
        Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
            if (preview != null) Row(
                Modifier.fillMaxWidth().background(c.bgInput, Radii.Md).ring(Radii.Md, c.borderSubtle).padding(14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(14.dp),
            ) {
                BadgeMark(preview, 40.dp)
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Txt(preview.label, style = Nx.type.body.copy(fontSize = Nx.type.sp(16f), fontWeight = FontWeight(700)), color = c.textStrong)
                    if (preview.tip.isNotEmpty()) Txt(preview.tip, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)), color = c.text)
                }
            }
            NxTextField(
                text, { text = it.take(MAX_TIP + 8) },
                label = t("badges.index.describeField"),
                placeholder = builtIn?.tip?.ifEmpty { null } ?: t("badges.staff.descriptionPlaceholder"),
                hint = "$length/$MAX_TIP",
                error = if (length > MAX_TIP) t("badges.index.describeLong") else null,
                singleLine = false, minLines = 3, capitalize = true,
            )
            if (builtIn != null) Txt(t("badges.index.describeEmptyHint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
        }
    }
}
