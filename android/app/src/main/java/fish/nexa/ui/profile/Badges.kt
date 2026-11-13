package fish.nexa.ui.profile

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import androidx.compose.ui.window.PopupProperties
import fish.nexa.Nexa
import fish.nexa.data.Badges
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.shadowInPopup

/**
 * One badge (components/badges/Badges.tsx BadgePill): its picture (badges/,
 * written by tools/badge_art.py and read through Ota), or for an event badge
 * staff made, its icon in the colour or gradient they chose, or its uploaded
 * picture, with nothing behind it. Pressing
 * one opens the card with its name and meaning, as the web's tooltip does; [still] draws it
 * with nothing to press, and [named] puts the name beside it, for lists (the
 * designer, the staff panel).
 */
@Composable
fun BadgePill(badge: Badges.Badge, userId: String, flags: Long, plan: String, still: Boolean = false, named: Boolean = false, size: Dp = 20.dp) {
    val look = Badges.look(badge, userId, flags, plan) ?: return
    val c = Nx.colors
    var open by remember { mutableStateOf(false) }
    val press = if (still) Modifier else Modifier
        .pressable(remember { MutableInteractionSource() }) { open = true }
        .semantics { contentDescription = badge.exist?.let { "${look.said} (${t("badges.exist", "count" to it)})" } ?: look.said }
    val mark = @Composable {
        Box {
            BadgeMark(look, size, press)
            if (open) BadgeCardPopup(look, badge.exist) { open = false }
        }
    }
    if (!named) return mark()
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        mark()
        Txt(look.label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1)
    }
}

/** The relative luminance of the top and bottom of every badge (tools/badge_art.py), so one reads like another on any card. */
private const val BADGE_TOP = 0.27f
private const val BADGE_BOTTOM = 0.2f

/** [color] moved to [target] luminance, towards black or white, so its hue stays (web badgeTone). */
private fun badgeTone(color: Color, target: Float): Color {
    val end = if (color.luminance() > target) 0f else 1f
    fun at(t: Float) = Color(color.red + (end - color.red) * t, color.green + (end - color.green) * t, color.blue + (end - color.blue) * t)
    var low = 0f
    var high = 1f
    repeat(20) {
        val t = (low + high) / 2
        if ((at(t).luminance() > target) == (end == 0f)) low = t else high = t
    }
    return at(high)
}

/** A badge at any size. */
@Composable
internal fun BadgeMark(look: Badges.Look, size: Dp, modifier: Modifier = Modifier) {
    val c = Nx.colors
    if (look.art != null) {
        coil3.compose.AsyncImage(fish.nexa.data.Ota.uri("badges/${look.art}.svg"), null, Modifier.size(size).then(modifier))
    } else if (look.image != null) {
        coil3.compose.AsyncImage(fish.nexa.media.Media.absolute(look.image), null, Modifier.size(size).then(modifier), contentScale = ContentScale.Fit)
    } else {
        // drawn as the badge art is: a short gradient at its luminance, with a tint inside closed shapes
        val first = look.color?.let { opaque(it) }
        val tone = first?.let { badgeTone(it, BADGE_TOP) } ?: c.brand
        val second = first?.let { badgeTone(look.color2?.let { two -> opaque(two) } ?: it, BADGE_BOTTOM) }
        val icon = look.icon ?: "award"
        val paint = if (second == null) Modifier else Modifier
            .graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen }
            .drawWithContent {
                drawContent()
                drawRect(Brush.verticalGradient(listOf(tone, second)), blendMode = BlendMode.SrcIn)
            }
        Icon(if (fish.nexa.ui.Lucide.exists(icon)) icon else "award", tone, Modifier.then(modifier).then(paint), size = size, stroke = 2.3f, bodied = true)
    }
}

/** Room around the card inside its window, so its shadow is not cut off. */
private val CardRoom = 12.dp

/** Above the badge with its arrow on the badge's middle, or below it when there is no room, kept on screen. */
private class BadgeCardPlace(private val room: Int, private val clear: Int, private val placed: (arrowX: Int, below: Boolean) -> Unit) : PopupPositionProvider {
    override fun calculatePosition(anchorBounds: IntRect, windowSize: IntSize, layoutDirection: LayoutDirection, popupContentSize: IntSize): IntOffset {
        val edge = clear - room
        val x = (anchorBounds.center.x - popupContentSize.width / 2).coerceIn(edge, maxOf(edge, windowSize.width - popupContentSize.width - edge))
        val above = anchorBounds.top + room - clear - popupContentSize.height
        val below = above < edge
        placed(anchorBounds.center.x - x, below)
        return IntOffset(x, if (below) anchorBounds.bottom - room + clear else above)
    }
}

/** What pressing a badge opens, as the web's tooltip card: the badge large, its name, and what it means. */
@Composable
private fun BadgeCardPopup(look: Badges.Look, exist: Int?, onDismiss: () -> Unit) {
    val c = Nx.colors
    val density = LocalDensity.current
    var arrowX by remember { mutableStateOf(-1) }
    var below by remember { mutableStateOf(false) }
    val place = remember(density) {
        with(density) {
            BadgeCardPlace(CardRoom.roundToPx(), 8.dp.roundToPx()) { x, b ->
                arrowX = x
                below = b
            }
        }
    }
    val shape = RoundedCornerShape(14.dp)
    Popup(popupPositionProvider = place, onDismissRequest = onDismiss, properties = PopupProperties(focusable = true)) {
        Box(
            Modifier
                .padding(CardRoom)
                .width(232.dp)
                .shadowInPopup(shape, c)
                .drawBehind {
                    if (arrowX < 0) return@drawBehind
                    val tip = 5.dp.toPx()
                    val x = arrowX.toFloat() - CardRoom.toPx()
                    val edge = if (below) 0f else this.size.height
                    val out = if (below) -tip else tip
                    drawPath(Path().apply { moveTo(x - tip, edge); lineTo(x, edge + out); lineTo(x + tip, edge); close() }, c.tooltipBg)
                }
                .background(c.tooltipBg, shape)
                .ring(shape, c.tooltipText.copy(alpha = 0.10f))
                .padding(16.dp),
        ) {
            Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
                BadgeMark(look, 64.dp)
                Spacer(Modifier.height(10.dp))
                Txt(look.label, style = Nx.type.heading.copy(fontSize = Nx.type.sp(17f), fontWeight = FontWeight(800), lineHeight = Nx.type.sp(21f)), color = c.tooltipText, align = TextAlign.Center)
                if (look.tip.isNotEmpty()) {
                    Spacer(Modifier.height(4.dp))
                    Txt(look.tip, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), fontWeight = FontWeight(500), lineHeight = Nx.type.sp(18f)), color = c.tooltipText.copy(alpha = 0.72f), align = TextAlign.Center)
                }
                if (exist != null && exist > 0) {
                    Spacer(Modifier.height(8.dp))
                    Txt(t("badges.exist", "count" to exist), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12.5f), fontWeight = FontWeight(600), lineHeight = Nx.type.sp(16f), fontFeatureSettings = "tnum"), color = c.tooltipText.copy(alpha = 0.72f), align = TextAlign.Center)
                }
            }
        }
    }
}

private fun opaque(rgb: Int) = Color(0xFF000000 or (rgb.toLong() and 0xFFFFFF))

/**
 * A profile's badges in its holder's order, all in one pill that sits on the
 * username line beside the pronouns (web BadgeRow). Until the list arrives,
 * the staff rank and plan come from the account itself.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun BadgeRow(userId: String, flags: Long, plan: String, badges: List<Badges.Badge>?, modifier: Modifier = Modifier) {
    val list = (badges ?: Badges.fallback(userId, flags, plan)).filter { !it.hidden && Badges.look(it, userId, flags, plan) != null }
    if (list.isEmpty()) return
    val c = Nx.colors
    val shape = RoundedCornerShape(9.dp)
    val pill = LocalProfile.current.badgePill
    FlowRow(
        modifier
            .background(pill ?: c.textStrong.copy(alpha = 0.07f), shape)
            .ring(shape, if (pill != null) c.borderSubtle else c.textStrong.copy(alpha = 0.10f))
            .padding(horizontal = 6.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp),
        itemVerticalAlignment = Alignment.CenterVertically,
    ) {
        for (b in list.take(SHOWN_BADGES)) BadgePill(b, userId, flags, plan, size = 22.dp)
        val rest = list.size - SHOWN_BADGES
        if (rest > 0) Box(
            Modifier
                .height(22.dp)
                .widthIn(min = 22.dp)
                .clip(RoundedCornerShape(6.dp))
                .pressable(remember { MutableInteractionSource() }) { openAllBadges(userId, flags, plan, list) }
                .semantics { contentDescription = t("badges.showAll", "count" to list.size) }
                .padding(horizontal = 4.dp),
            contentAlignment = Alignment.Center,
        ) {
            Txt("+$rest", style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(700), fontFeatureSettings = "tnum"), color = c.textSubtle, maxLines = 1)
        }
    }
}

/** How many badges a profile shows before the rest go behind +x. */
private const val SHOWN_BADGES = 5

/** Every badge a profile has, each with its name and meaning: what +x opens (web AllBadges). */
private fun openAllBadges(userId: String, flags: Long, plan: String, list: List<Badges.Badge>) {
    openSheet("all-badges", title = t("badges.all")) {
        val c = Nx.colors
        Column(Modifier.fillMaxWidth().padding(bottom = 8.dp)) {
            for (b in list) {
                val look = Badges.look(b, userId, flags, plan) ?: continue
                Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    BadgeMark(look, 36.dp)
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Txt(look.label, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(650)), color = c.textStrong)
                        if (look.tip.isNotEmpty()) Txt(look.tip, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.2f)), color = c.textMuted)
                    }
                }
            }
        }
    }
}
