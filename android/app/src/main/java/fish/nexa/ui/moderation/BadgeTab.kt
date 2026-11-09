package fish.nexa.ui.moderation

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.TextUnitType
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Badges
import fish.nexa.data.ModEvent
import fish.nexa.data.ModUser
import fish.nexa.data.Moderation
import fish.nexa.media.Media
import fish.nexa.ui.Icon
import fish.nexa.ui.Lucide
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.TickBox
import fish.nexa.ui.components.ToggleRow
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.profile.BadgePill
import fish.nexa.ui.server.ServerApi
import fish.nexa.ui.server.openImageCrop
import fish.nexa.ui.settings.keptAboveSaveBar
import fish.nexa.ui.shell.confirm
import fish.nexa.ui.support.Segmented
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.shadowMedium
import fish.nexa.ui.theme.sheen
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private const val MAX_NAME = 32
private const val MAX_DESCRIPTION = 120
private const val HOLDERS_PAGE = 50
/** The most icons a search shows at once, so a short query does not draw a thousand (web BadgeIconSearch). */
private const val ICONS_SHOWN = 120

private fun colourHex(c: Int) = "#%06x".format(c and 0xFFFFFF)
private fun opaque(rgb: Int) = Color(0xFF000000 or (rgb.toLong() and 0xFFFFFF))
private fun lengthOf(s: String) = s.trim().let { it.codePointCount(0, it.length) }

/** What the labels above a group of controls look like (`.label`). */
@Composable
private fun FieldLabel(text: String) =
    Txt(text, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontWeight = FontWeight(650)), color = Nx.colors.text)

/** `.listTitle`: the small capitals over a part of the page. */
@Composable
private fun PartTitle(text: String, modifier: Modifier = Modifier) = Txt(
    text.uppercase(),
    style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(700), letterSpacing = TextUnit(0.04f, TextUnitType.Em)),
    color = Nx.colors.textMuted,
    modifier = modifier.padding(start = 2.dp).semantics { heading() },
)

/** Every Lucide icon whose name holds each word typed, best first; with nothing typed, the short list (and what is chosen). */
private fun findIcons(query: String, current: String): List<String> {
    val words = query.trim().lowercase().split(Regex("[\\s-]+")).filter { it.isNotEmpty() }
    if (words.isEmpty()) return (if (current in Badges.EVENT_ICONS) emptyList() else listOf(current)) + Badges.EVENT_ICONS
    val q = words.joinToString("-")
    fun rank(n: String) = if (n == q) 0 else if (n.startsWith(q)) 1 else 2
    return Lucide.names().filter { n -> words.all { it in n } }
        .sortedWith(compareBy({ rank(it) }, { it.length }, { it }))
        .take(ICONS_SHOWN)
}

private fun earnedLabel(id: String) = when (id) {
    "og" -> t("badges.og")
    "years" -> t("badges.staff.yearsName")
    "bug_hunter" -> t("badges.staff.bugName")
    "ideas" -> t("badges.staff.ideasName")
    "app_developer" -> t("badges.appDeveloper")
    "gift_giver" -> t("badges.giftGiver")
    "friendly" -> t("badges.family.friendly")
    "host" -> t("badges.family.host")
    "booster" -> t("badges.booster")
    "regular" -> t("badges.family.regular")
    "open_mic" -> t("badges.family.openMic")
    else -> id
}

private fun levelLabel(badge: String, level: Int) =
    if (badge == "years") t("badges.years", "count" to level) else t("badges.holders.level", "level" to level)

private fun levelOptions(badge: String, top: Int) = (1..top).map { it.toString() to levelLabel(badge, it) }

/**
 * The web's BadgesTab: the card that makes and changes a badge (its shape,
 * colours, icon or picture, and what it says), then every badge there is,
 * each opening who holds it. Deleting one takes it from everyone.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun BadgesTab() {
    val c = Nx.colors
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val listState = rememberLazyListState()
    val list = remember { mutableStateListOf<Badges.Def>() }
    var contributors by remember { mutableIntStateOf(0) }
    var loaded by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<Badges.Def?>(null) }
    var name by remember { mutableStateOf("") }
    var description by remember { mutableStateOf("") }
    var icon by remember { mutableStateOf(Badges.EVENT_ICONS[0]) }
    var colour by remember { mutableIntStateOf(Badges.EVENT_COLOURS[0]) }
    var colour2 by remember { mutableStateOf<Int?>(null) }
    var glyph by remember { mutableStateOf("light") }
    var shape by remember { mutableStateOf("circle") }
    var mode by remember { mutableStateOf("icon") }
    var imageId by remember { mutableStateOf("0") }
    var query by remember { mutableStateOf("") }
    var uploading by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }

    fun load() {
        failed = false
        scope.launch {
            try {
                val (defs, n) = Badges.defs()
                list.clear()
                list.addAll(defs)
                contributors = n
                loaded = true
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                failed = true
            }
        }
    }
    LaunchedEffect(Unit) { load() }
    // another member of staff gave, made or deleted one
    LaunchedEffect(Unit) {
        Moderation.events.collect { e -> if (e is ModEvent.Log && e.entry.action.startsWith("badge_")) load() }
    }
    fun startEdit(d: Badges.Def?) {
        editing = d
        name = d?.name ?: ""
        description = d?.description ?: ""
        icon = d?.icon ?: Badges.EVENT_ICONS[0]
        colour = d?.color ?: Badges.EVENT_COLOURS[0]
        colour2 = d?.color2?.takeIf { it >= 0 }
        glyph = if (d?.dark == true) "dark" else "light"
        shape = d?.shape ?: "circle"
        imageId = d?.imageId?.takeIf { it.isNotEmpty() } ?: "0"
        mode = if (imageId != "0") "picture" else "icon"
        query = ""
    }
    val nameLength = lengthOf(name)
    val descLength = lengthOf(description)
    val ready = nameLength in 1..MAX_NAME && descLength <= MAX_DESCRIPTION && (mode == "icon" || imageId != "0") && !uploading
    fun current(id: String, holders: Int) = Badges.Def(
        id, name.trim(), icon, colour, holders, description.trim(), colour2 ?: -1, glyph == "dark", shape, if (mode == "picture") imageId else "0",
    )
    fun submit() {
        busy = true
        scope.launch {
            try {
                val was = editing
                if (was != null) {
                    Badges.editDef(current(was.id, was.holders))?.let { d -> list.replaceAll { if (it.id == d.id) d.copy(holders = it.holders) else it } }
                    Nexa.ui.toast(t("badges.staff.saved"), "success")
                } else {
                    Badges.createDef(current("", 0))?.let { list.add(0, it) }
                    Nexa.ui.toast(t("badges.staff.created"), "success")
                }
                startEdit(null)
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
            } finally {
                busy = false
            }
        }
    }
    fun remove(d: Badges.Def) = confirm(t("badges.staff.deleteTitle"), t("badges.staff.deleteBody", "name" to d.name), t("badges.staff.delete")) {
        Badges.deleteDef(d.id)
        list.removeAll { it.id == d.id }
        if (editing?.id == d.id) startEdit(null)
        Nexa.ui.toast(t("badges.staff.deleted"), "success")
    }
    val pickPicture = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            val file = runCatching { ServerApi.read(context, uri) }.getOrNull() ?: return@launch
            openImageCrop(file, 1f, false, t("badges.staff.cropTitle"), 256, serverCrop = true) { r ->
                uploading = true
                scope.launch {
                    try {
                        val area = r.crop?.let { "?x=${it.x}&y=${it.y}&w=${it.w}&h=${it.h}" } ?: ""
                        Badges.uploadImage(r.bytes, r.type, area)?.let { imageId = it }
                    } catch (e: Exception) {
                        if (e is kotlinx.coroutines.CancellationException) throw e
                        Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
                    } finally {
                        uploading = false
                    }
                }
            }
        }
    }

    fun holdersButtonOpen(badge: Badges.Badge, label: String, top: Int) = openHoldersSheet(badge, label, top) { load() }

    LazyColumn(Modifier.fillMaxSize().imePadding(), state = listState, contentPadding = PanelPad, verticalArrangement = Arrangement.spacedBy(22.dp)) {
        item("form") {
            val card = Radii.Lg
            Column(Modifier.fillMaxWidth().shadowMedium(card, c).ring(card, c.panelEdge).background(cardFill(c), card).background(sheen(c), card).topEdge(card, c.edgeLight).padding(18.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 4.dp)) {
                    Icon("Award", c.textStrong, size = 18.dp)
                    Txt(if (editing != null) t("badges.staff.edit") else t("badges.staff.new"), style = Nx.type.name.copy(fontSize = Nx.type.sp(17f), fontWeight = FontWeight(680)), color = c.textStrong, modifier = Modifier.semantics { heading() })
                }
                Txt(t("badges.staff.hint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), lineHeight = Nx.type.sp(19.6f)), color = c.textMuted, modifier = Modifier.padding(bottom = 14.dp))
                Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    val preview = current("0", 0).let { d ->
                        d.copy(name = d.name.ifEmpty { t("badges.staff.namePlaceholder") }).badge
                    }
                    Stage(preview, colour2 ?: colour, description.trim())
                    NxTextField(name, { name = it.take(MAX_NAME + 8) }, label = t("badges.staff.name"), placeholder = t("badges.staff.namePlaceholder"), hint = if (nameLength > MAX_NAME - 8) "$nameLength/$MAX_NAME" else null, capitalize = true)
                    NxTextField(description, { description = it.take(MAX_DESCRIPTION + 8) }, label = t("badges.staff.description"), placeholder = t("badges.staff.descriptionPlaceholder"), hint = "$descLength/$MAX_DESCRIPTION", capitalize = true)
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        FieldLabel(if (colour2 == null) t("badges.staff.colour") else t("badges.staff.firstColour"))
                        Swatches(colour) { colour = it }
                    }
                    ToggleRow(
                        colour2 != null,
                        { on -> colour2 = if (on) Badges.EVENT_COLOURS.first { it != colour } else null },
                        t("badges.staff.gradient"), t("badges.staff.gradientHint"), rule = false,
                    )
                    colour2?.let { second ->
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            FieldLabel(t("badges.staff.secondColour"))
                            Swatches(second) { colour2 = it }
                        }
                    }
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        FieldLabel(t("badges.staff.look"))
                        Segmented(listOf("icon" to t("badges.staff.lookIcon"), "picture" to t("badges.staff.lookPicture")), mode, { mode = it }, t("badges.staff.look"))
                    }
                    if (mode == "icon") {
                        // the search sits at the bottom of the form, under where the keyboard comes up, and its results under it
                        var searching by remember { mutableStateOf(false) }
                        val keyboard = WindowInsets.isImeVisible
                        Column(
                            Modifier.onFocusChanged { searching = it.hasFocus }.keptAboveSaveBar(query to keyboard, searching && keyboard),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            FieldLabel(t("badges.staff.icon"))
                            SearchField(query, { query = it }, t("badges.staff.iconSearch"))
                            IconGrid(remember(query, icon) { findIcons(query, icon) }, icon) { icon = it }
                        }
                    } else {
                        PictureField(imageId, uploading) { pickPicture.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) }
                    }
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        if (editing != null) NxButton(t("chat.editMessage.cancel"), { startEdit(null) }, kind = ButtonKind.GHOST, size = ButtonSize.TOUCH)
                        Spacer(Modifier.weight(1f))
                        NxButton(if (editing != null) t("badges.staff.save") else t("badges.staff.create"), ::submit, size = ButtonSize.TOUCH, icon = if (editing != null) null else "Plus", enabled = ready, loading = busy)
                    }
                }
            }
        }
        when {
            failed && !loaded -> item("failed") { LoadFailed({ load() }) }
            !loaded -> item("loading") { LoadingRow() }
            else -> {
                item("made") {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        PartTitle(t("badges.staff.madeByStaff"))
                        for (d in list) {
                            val badge = d.badge
                            BadgeRow(
                                pill = { BadgePill(badge, "0", 0L, "", still = true, named = true) },
                                note = d.description.ifEmpty { null },
                                editing = editing?.id == d.id,
                            ) {
                                HoldersButton(d.holders) { holdersButtonOpen(badge, d.name, 1) }
                                ModIconButton("PenLine", t("badges.staff.editNamed", "name" to d.name), {
                                    startEdit(d)
                                    // the card it opens in is at the top of the page
                                    scope.launch { listState.scrollToItem(0) }
                                })
                                ModIconButton("Trash2", t("badges.staff.deleteNamed", "name" to d.name), { remove(d) })
                            }
                        }
                        if (list.isEmpty()) Txt(t("badges.staff.none"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted, modifier = Modifier.padding(14.dp))
                    }
                }
                item("hand") {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        PartTitle(t("badges.staff.givenByHand"))
                        val badge = Badges.Badge(Badges.CONTRIBUTOR)
                        BadgeRow(pill = { BadgePill(badge, "0", 0L, "", still = true, named = true) }, note = t("badges.staff.contributorNote")) {
                            HoldersButton(contributors) { holdersButtonOpen(badge, t("badges.contributor"), 1) }
                        }
                    }
                }
                item("earned") {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp), modifier = Modifier.padding(bottom = 2.dp)) {
                            PartTitle(t("badges.staff.earned"))
                            Txt(t("badges.staff.earnedHint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, modifier = Modifier.padding(start = 2.dp))
                        }
                        for ((id, top) in Badges.OVERRIDABLE) {
                            val badge = Badges.Badge(id, level = 1)
                            BadgeRow(pill = { BadgePill(badge, "0", 0L, "", still = true, named = true) }) {
                                HoldersButton(null) { holdersButtonOpen(badge, earnedLabel(id), top) }
                            }
                        }
                        for (id in listOf("staff", "plan")) {
                            val badge = Badges.Badge(id)
                            val label = if (id == "staff") t("badges.staff.ranks") else t("badges.staff.plans")
                            BadgeRow(
                                pill = {
                                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                        // a sample of what they look like: the picture of the top rank and plan
                                        BadgePill(badge, "0", 1L, "pro", still = true)
                                        Txt(label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textStrong, maxLines = 1)
                                    }
                                },
                                note = t("badges.staff.followsAccount"),
                            ) {
                                HoldersButton(null) { holdersButtonOpen(badge, label, 0) }
                            }
                        }
                    }
                }
            }
        }
    }
}

/** The badge being made, large on a glow of its own colour, with what pressing it will say. */
@Composable
private fun Stage(preview: Badges.Badge, glow: Int, description: String) {
    val c = Nx.colors
    val shape = Radii.Md
    val tint = opaque(glow).copy(alpha = 0.22f)
    Row(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(c.bgInput)
            .drawBehind {
                drawRect(Brush.radialGradient(listOf(tint, Color.Transparent), center = Offset(48.dp.toPx(), size.height / 2), radius = 120.dp.toPx()))
            }
            .ring(shape, c.borderSubtle)
            .padding(16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        BadgePill(preview, "0", 0L, "", still = true, size = 64.dp)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Txt(preview.name ?: "", style = Nx.type.body.copy(fontSize = Nx.type.sp(16f), fontWeight = FontWeight(700)), color = c.textStrong, maxLines = 2)
            if (description.isNotEmpty()) {
                Txt(description, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.2f)), color = c.text, maxLines = 3)
            } else {
                Txt(t("badges.staff.descriptionPlaceholder"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.2f), fontStyle = FontStyle.Italic), color = c.textMuted, maxLines = 3)
            }
        }
        Box(Modifier.align(Alignment.Top)) { BadgePill(preview, "0", 0L, "", still = true) }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun Swatches(value: Int, onPick: (Int) -> Unit) {
    val c = Nx.colors
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp), itemVerticalAlignment = Alignment.CenterVertically) {
        for (sw in Badges.EVENT_COLOURS) {
            val on = value == sw
            val col = opaque(sw)
            Box(
                Modifier
                    .size(28.dp)
                    .then(if (on) Modifier.ring(CircleShape, col).padding(3.dp) else Modifier)
                    .clip(CircleShape)
                    .background(col)
                    .pressable(remember { MutableInteractionSource() }, role = Role.RadioButton) { onPick(sw) }
                    .semantics { contentDescription = colourHex(sw); selected = on },
            )
        }
        // a colour of its own chosen: the button wears it, ringed as a picked swatch is
        val own = value !in Badges.EVENT_COLOURS
        val ownColour = opaque(value)
        Box(
            Modifier
                .size(28.dp)
                .then(if (own) Modifier.ring(CircleShape, ownColour).padding(3.dp).clip(CircleShape).background(ownColour) else Modifier.clip(CircleShape).ring(CircleShape, c.textMuted.copy(alpha = 0.45f)))
                .pressable(remember { MutableInteractionSource() }, role = Role.Button) {
                    fish.nexa.ui.settings.openColourPicker(t("badges.staff.customColour"), colourHex(value), alpha = false) { v -> v.removePrefix("#").take(6).toIntOrNull(16)?.let(onPick) }
                }
                .semantics { contentDescription = t("badges.staff.customColour") },
            contentAlignment = Alignment.Center,
        ) { Icon("Pipette", if (own) Color.White else c.textMuted, size = if (own) 12.dp else 15.dp) }
    }
}

@Composable
private fun IconGrid(names: List<String>, chosen: String, onPick: (String) -> Unit) {
    val c = Nx.colors
    val box = Radii.Md
    if (names.isEmpty()) {
        Box(Modifier.fillMaxWidth().background(c.bgInput, box).padding(horizontal = 8.dp, vertical = 18.dp), contentAlignment = Alignment.Center) {
            Txt(t("badges.staff.iconsNone"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
        }
        return
    }
    LazyVerticalGrid(
        GridCells.Adaptive(40.dp),
        Modifier.fillMaxWidth().heightIn(max = 168.dp).background(c.bgInput, box),
        contentPadding = PaddingValues(4.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        items(names, key = { it }) { n ->
            val on = chosen == n
            val cell = RoundedCornerShape(8.dp)
            Box(
                Modifier
                    .height(38.dp)
                    .clip(cell)
                    .then(if (on) Modifier.background(c.brand.copy(alpha = 0.16f)).ring(cell, c.brand.copy(alpha = 0.4f)) else Modifier)
                    .pressable(remember { MutableInteractionSource() }, role = Role.RadioButton) { onPick(n) }
                    .semantics { contentDescription = n; selected = on },
                contentAlignment = Alignment.Center,
            ) { Icon(n, if (on) c.textBrand else c.textMuted, size = 18.dp) }
        }
    }
}

@Composable
private fun PictureField(imageId: String, uploading: Boolean, onPick: () -> Unit) {
    val c = Nx.colors
    val thumb = RoundedCornerShape(12.dp)
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
        if (imageId != "0") {
            coil3.compose.AsyncImage(Media.absolute("/media/badge/$imageId"), null, Modifier.size(56.dp).clip(thumb), contentScale = ContentScale.Crop)
        } else {
            val dash = c.textMuted.copy(alpha = 0.55f)
            Box(
                Modifier.size(56.dp).clip(thumb).background(c.bgInput).drawBehind {
                    val gap = 4.dp.toPx()
                    drawRoundRect(dash, cornerRadius = CornerRadius(12.dp.toPx()), style = Stroke(1.dp.toPx(), pathEffect = PathEffect.dashPathEffect(floatArrayOf(gap, gap))))
                },
                contentAlignment = Alignment.Center,
            ) { Icon("ImagePlus", c.textMuted, size = 20.dp) }
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp), horizontalAlignment = Alignment.Start) {
            NxButton(if (imageId != "0") t("badges.staff.replacePicture") else t("badges.staff.uploadPicture"), onPick, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = uploading)
            Txt(t("badges.staff.pictureHint"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted)
        }
    }
}

@Composable
private fun HoldersButton(count: Int?, onClick: () -> Unit) = NxButton(
    if (count == null) t("badges.staff.holdersButton") else t("badges.staff.holders", "count" to count),
    onClick, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, icon = "Users",
)

/** `.row`: a badge, what it is, and what can be done with it, the tools going under the badge when they do not fit beside it. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun BadgeRow(pill: @Composable () -> Unit, note: String? = null, editing: Boolean = false, tools: @Composable () -> Unit) {
    val c = Nx.colors
    val shape = Radii.Md
    Column(
        Modifier.fillMaxWidth().heightIn(min = 52.dp).ring(shape, if (editing) c.focusRing else c.panelEdge, if (editing) 2.dp else 1.dp).background(cardFill(c), shape).background(sheen(c), shape).topEdge(shape, c.edgeLight).padding(start = 14.dp, end = 10.dp, top = 8.dp, bottom = 8.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        FlowRow(
            Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalArrangement = Arrangement.spacedBy(6.dp),
            itemVerticalAlignment = Alignment.CenterVertically,
        ) {
            Box(Modifier.heightIn(min = 32.dp), contentAlignment = Alignment.CenterStart) { pill() }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(2.dp)) { tools() }
        }
        if (note != null) Txt(note, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
    }
}

/** A person's row in the list: how they came to hold it. */
@Composable
private fun SourceChip(text: String, source: String) {
    val c = Nx.colors
    val pill = RoundedCornerShape(999.dp)
    val (fill, tone) = when (source) {
        "given" -> c.brand.copy(alpha = 0.14f) to c.textBrand
        "taken" -> c.danger.copy(alpha = 0.14f) to c.textDanger
        else -> c.bgSurface to c.textMuted
    }
    Txt(
        text,
        style = Nx.type.hint.copy(fontSize = Nx.type.sp(11f), fontWeight = FontWeight(600)),
        color = tone,
        maxLines = 1,
        modifier = Modifier
            .background(fill, pill)
            .then(if (source == "given" || source == "taken") Modifier else Modifier.ring(pill, c.borderSubtle))
            .padding(horizontal = 7.dp, vertical = 1.dp),
    )
}

/**
 * The web's BadgeHoldersModal: who holds a badge, earned or given, and who
 * staff took an earned one from, with Give To at the top. Staff ranks and
 * plans ([top] 0) are only listed: they follow the account and change elsewhere.
 */
@OptIn(ExperimentalLayoutApi::class)
private fun openHoldersSheet(badge: Badges.Badge, label: String, top: Int, onChanged: () -> Unit) {
    var holders by mutableStateOf<List<Badges.Holder>?>(null)
    var taken by mutableStateOf<List<Badges.Holder>>(emptyList())
    var total by mutableIntStateOf(0)
    var failed by mutableStateOf(false)
    var more by mutableStateOf(false)
    var busy by mutableStateOf<String?>(null)
    var query by mutableStateOf("")
    var found by mutableStateOf<List<ModUser>?>(null)
    var level by mutableIntStateOf(1)
    suspend fun load() {
        failed = false
        try {
            val r = Badges.holders(badge.id, 0, HOLDERS_PAGE)
            holders = r.holders
            taken = r.taken
            total = r.total
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            failed = true
        }
    }
    suspend fun loadMore() {
        val have = holders ?: return
        more = true
        try {
            val r = Badges.holders(badge.id, have.size, HOLDERS_PAGE)
            holders = have + r.holders
            total = r.total
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            Nexa.ui.toast(describeError(e, t("moderation.loadFailed")), "error")
        } finally {
            more = false
        }
    }
    suspend fun act(userId: String, state: String, lv: Int = 1) {
        busy = userId + state
        try {
            Badges.setHolder(badge.id, userId, state, lv)
            if (state == "give") {
                query = ""
                found = null
            }
            load()
            onChanged()
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
        } finally {
            busy = null
        }
    }
    val tiered = top > 1
    openSheet("badge-holders", title = t("badges.holders.title", "name" to label)) {
        val c = Nx.colors
        val scope = rememberCoroutineScope()
        LaunchedEffect(Unit) { load() }
        // people to give it to, asked for as the name is typed
        LaunchedEffect(query) {
            val q = query.trim()
            if (q.isEmpty() || top == 0) {
                found = null
                return@LaunchedEffect
            }
            delay(200)
            found = try {
                Moderation.users(q, "", "", "", 6).first
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                emptyList()
            }
        }
        Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(18.dp)) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Box(Modifier.weight(1f)) { BadgePill(badge, "0", 1L, "pro", still = true, named = true) }
                if (holders != null) Txt(t("badges.staff.holders", "count" to total), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, maxLines = 1)
            }
            if (top == 0) Txt(t("badges.staff.followsAccount"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)

            if (top > 0) {
                val box = Radii.Md
                Column(Modifier.fillMaxWidth().background(c.bgInput, box).ring(box, c.borderSubtle).padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    PartTitle(t("badges.holders.giveTo"))
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                        SearchField(query, { query = it }, t("badges.holders.searchPlaceholder"), Modifier.weight(1f))
                        if (tiered) SortField(levelOptions(badge.id, top), level.toString(), Modifier.width(116.dp)) { level = it.toInt() }
                    }
                    found?.let { people ->
                        val held = (holders ?: emptyList()).map { it.userId }.toSet()
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            if (people.isEmpty()) Txt(t("badges.holders.noMatch"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, modifier = Modifier.padding(8.dp))
                            for (u in people) {
                                val name = u.displayName.ifEmpty { u.username }
                                val has = u.id in held
                                Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                    Avatar(u.id, name, u.avatarId, 24.dp)
                                    Box(Modifier.weight(1f)) {
                                        fish.nexa.ui.chat.PersonName(u.id, name, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textStrong, maxLines = 1)
                                    }
                                    NxButton(
                                        if (has && tiered) t("badges.holders.setLevel") else if (has) t("badges.holders.has") else t("badges.holders.give"),
                                        { scope.launch { act(u.id, "give", level) } },
                                        kind = if (has) ButtonKind.SECONDARY else ButtonKind.PRIMARY,
                                        size = ButtonSize.SM,
                                        loading = busy == u.id + "give",
                                    )
                                }
                            }
                        }
                    }
                }
            }

            val list = holders
            when {
                failed -> LoadFailed({ scope.launch { load() } })
                list == null -> LoadingRow()
                else -> {
                    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        PartTitle(t("badges.holders.heldBy"), Modifier.padding(bottom = 6.dp))
                        if (list.isEmpty()) Txt(t("badges.holders.nobody"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, modifier = Modifier.padding(8.dp))
                        for (h in list) HolderRow(h, false, badge.id, top, busy) { state -> scope.launch { act(h.userId, state) } }
                        if (list.size < total) NxButton(t("badges.holders.more"), { scope.launch { loadMore() } }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = more, modifier = Modifier.padding(top = 8.dp))
                    }
                    if (taken.isNotEmpty()) Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        PartTitle(t("badges.holders.takenTitle"), Modifier.padding(bottom = 4.dp))
                        Txt(t("badges.holders.takenHint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, modifier = Modifier.padding(bottom = 8.dp))
                        for (h in taken) HolderRow(h, true, badge.id, top, busy) { state -> scope.launch { act(h.userId, state) } }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun HolderRow(h: Badges.Holder, gone: Boolean, badge: String, top: Int, busy: String?, act: (String) -> Unit) {
    val c = Nx.colors
    val source = when {
        gone -> t("badges.holders.takenChip")
        h.source == "account" -> t("badges.holders.account")
        h.source == "given" -> if (h.overridden) t("badges.holders.givenOver") else t("badges.holders.given")
        else -> t("badges.holders.earned")
    }
    Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Avatar(h.userId, h.displayName, h.avatarId, 32.dp)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            fish.nexa.ui.chat.PersonName(h.userId, h.displayName, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textStrong, maxLines = 1)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                val small = Nx.type.hint.copy(fontSize = Nx.type.sp(12f))
                SourceChip(source, if (gone) "taken" else h.source)
                if (top > 1 && !gone) Txt(levelLabel(badge, h.level), style = small, color = c.textMuted, maxLines = 1)
                h.username?.let { Txt("@$it", style = small, color = c.textMuted, maxLines = 1) }
            }
        }
        if (top > 0) Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            if (gone) {
                NxButton(t("badges.holders.restore"), { act("auto") }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = busy == h.userId + "auto")
            } else {
                if (h.overridden) NxButton(t("badges.holders.automatic"), { act("auto") }, kind = ButtonKind.GHOST, size = ButtonSize.SM, loading = busy == h.userId + "auto")
                NxButton(t("badges.holders.take"), { act("take") }, kind = ButtonKind.DANGER, size = ButtonSize.SM, loading = busy == h.userId + "take")
            }
        }
    }
}

/**
 * The web's GrantBadgesModal: Contributor and every event badge, each ticked
 * when they hold it, then the earned badges, each following the account or
 * given or taken by hand until set back. Saving gives and takes the difference.
 */
fun openGiveBadgesSheet(u: ModUser) {
    var defs by mutableStateOf<List<Badges.Def>?>(null)
    var held by mutableStateOf<List<String>?>(null)
    var pick by mutableStateOf<Set<String>>(emptySet())
    var over by mutableStateOf<Map<String, Int>>(emptyMap())
    var overWas by mutableStateOf<Map<String, Int>>(emptyMap())
    var earned by mutableStateOf<Map<String, Int>>(emptyMap())
    var failed by mutableStateOf(false)
    var busy by mutableStateOf(false)
    suspend fun load() {
        failed = false
        try {
            val all = Badges.defs().first
            val mine = Badges.userBadges(u.id)
            defs = all
            held = mine.granted
            pick = mine.granted.toSet()
            over = mine.overrides
            overWas = mine.overrides
            earned = mine.earned
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            failed = true
        }
    }
    fun setState(k: String, state: String) {
        over = when (state) {
            "auto" -> over - k
            "take" -> over + (k to 0)
            else -> over + (k to ((over[k] ?: 0).takeIf { it != 0 } ?: maxOf(1, earned[k] ?: 0)))
        }
    }
    fun earnedText(k: String): String {
        val lv = earned[k] ?: 0
        return when {
            lv <= 0 -> t("badges.staff.earnedNot")
            (Badges.OVERRIDABLE[k] ?: 1) > 1 ->
                if (k == "years") t("badges.staff.earnedYears", "years" to t("badges.years", "count" to lv)) else t("badges.staff.earnedLevel", "level" to lv)
            else -> t("badges.staff.earnedYes")
        }
    }
    openSheet("give-badges", title = t("badges.staff.giveTitle", "name" to u.displayName), heading = { fish.nexa.ui.chat.PersonHeading(fish.nexa.ui.chat.PersonWords(t("badges.staff.giveTitle", "name" to fish.nexa.ui.chat.PERSON_SLOT), listOf(u.id to u.displayName))) }, footer = { close ->
        val scope = rememberCoroutineScope()
        val h = held
        val changed = h != null && (h.size != pick.size || h.any { it !in pick } || over != overWas)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
            NxButton(t("chat.editMessage.cancel"), { close() }, kind = ButtonKind.SECONDARY)
            NxButton(t("badges.staff.save"), {
                busy = true
                scope.launch {
                    try {
                        Badges.setGranted(u.id, pick.toList(), over)
                        Nexa.ui.toast(t("badges.staff.givenDone", "name" to u.displayName), "success")
                        close()
                    } catch (e: Exception) {
                        if (e is kotlinx.coroutines.CancellationException) throw e
                        Nexa.ui.toast(describeError(e, t("moderation.failed")), "error")
                    } finally {
                        busy = false
                    }
                }
            }, loading = busy, enabled = changed)
        }
    }) {
        val c = Nx.colors
        LaunchedEffect(Unit) { load() }
        val scope = rememberCoroutineScope()
        Txt(t("badges.staff.giveBody"), style = Nx.type.bodySmall, color = c.textMuted, modifier = Modifier.padding(bottom = 14.dp))
        val h = held
        when {
            failed -> LoadFailed({ scope.launch { load() } })
            h == null -> LoadingRow()
            else -> Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                val options = listOf(Badges.Badge(Badges.CONTRIBUTOR)) + (defs ?: emptyList()).map { it.badge }
                for (o in options) {
                    val on = o.id in pick
                    val shape = Radii.Md
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .clip(shape)
                            .background(if (on) c.brand.copy(alpha = 0.12f) else c.bgInput)
                            .ring(shape, if (on) c.brand else c.borderSubtle, if (on) 2.dp else 1.dp)
                            .pressable(remember { MutableInteractionSource() }, role = Role.Checkbox) { pick = if (on) pick - o.id else pick + o.id }
                            .padding(horizontal = 14.dp, vertical = 12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        TickBox(on)
                        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                BadgePill(o, "0", 0L, "", still = true, named = true)
                                if (o.id in h) Txt(t("badges.staff.held"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(11f), fontWeight = FontWeight(700)), color = c.textMuted)
                            }
                            if (o.id == Badges.CONTRIBUTOR) Txt(t("badges.contributorTip"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
                        }
                    }
                }
                if (defs?.isEmpty() == true) Txt(t("badges.staff.noneYet"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
                Column(verticalArrangement = Arrangement.spacedBy(4.dp), modifier = Modifier.padding(top = 14.dp, bottom = 4.dp)) {
                    PartTitle(t("badges.staff.earned"))
                    Txt(t("badges.staff.overrideHint"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, modifier = Modifier.padding(start = 2.dp))
                }
                for ((k, top) in Badges.OVERRIDABLE) {
                    val set = over[k]
                    val state = if (set == null) "auto" else if (set == 0) "take" else "give"
                    val shown = if (state == "give") set!! else maxOf(1, earned[k] ?: 1)
                    val shape = Radii.Md
                    val edge = when (state) {
                        "give" -> c.brand
                        "take" -> c.danger.copy(alpha = 0.7f)
                        else -> c.borderSubtle
                    }
                    Column(
                        Modifier.fillMaxWidth().background(c.bgInput, shape).ring(shape, edge, if (state == "auto") 1.dp else 2.dp).padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            BadgePill(Badges.Badge(k, level = shown), "0", 0L, "", still = true, named = true)
                            Txt(earnedText(k), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
                        }
                        Segmented(
                            listOf("auto", "give", "take").map { it to t("badges.staff.override.$it") },
                            state, { setState(k, it) }, t("badges.staff.overrideLabel"),
                        )
                        if (state == "give" && top > 1) {
                            SortField(levelOptions(k, top), shown.toString(), Modifier.fillMaxWidth()) { v -> over = over + (k to v.toInt()) }
                        }
                    }
                }
            }
        }
    }
}
