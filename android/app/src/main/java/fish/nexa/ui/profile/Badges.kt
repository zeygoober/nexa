package fish.nexa.ui.profile

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Matrix
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.foundation.shape.GenericShape
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Badges
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.ring

/**
 * One badge (components/badges/Badges.tsx BadgePill): its picture (badges/,
 * written by tools/badge_art.py and read through Ota), or for an event badge
 * staff made, its icon (white or dark) or uploaded picture on its shape in the
 * colour, or gradient, they chose. Pressing
 * one says its name and meaning, as the web's tooltip does; [still] draws it
 * with nothing to press, and [named] puts the name beside it, for lists (the
 * designer, the staff panel).
 */
@Composable
fun BadgePill(badge: Badges.Badge, userId: String, flags: Long, plan: String, still: Boolean = false, named: Boolean = false, size: Dp = 20.dp) {
    val look = Badges.look(badge, userId, flags, plan) ?: return
    val c = Nx.colors
    val press = if (still) Modifier else Modifier
        .pressable(remember { MutableInteractionSource() }) { Nexa.ui.toast(look.said) }
        .semantics { contentDescription = look.said }
    val mark = @Composable {
        if (look.art != null) {
            coil3.compose.AsyncImage(
                fish.nexa.data.Ota.uri("badges/${look.art}.svg"), null,
                Modifier.size(size).clip(RoundedCornerShape(size * 0.3f)).then(press),
            )
        } else {
            val tone = look.color?.let(::opaque) ?: c.brand
            val shape = badgeShape(look.shape)
            val fill = look.color2?.let { Brush.verticalGradient(listOf(tone, opaque(it))) } ?: SolidColor(tone)
            Box(
                Modifier
                    .size(size)
                    .clip(shape)
                    .background(fill)
                    .then(
                        if (look.image != null) Modifier
                        else Modifier.background(Brush.verticalGradient(0f to Color.White.copy(alpha = 0.32f), 0.55f to Color.Transparent)),
                    )
                    .ring(shape, Color.Black.copy(alpha = 0.22f))
                    .then(press),
                contentAlignment = Alignment.Center,
            ) {
                if (look.image != null) {
                    coil3.compose.AsyncImage(
                        fish.nexa.media.Media.absolute(look.image), null,
                        Modifier.matchParentSize(), contentScale = ContentScale.Crop,
                    )
                } else {
                    val icon = look.icon ?: "award"
                    Icon(if (fish.nexa.ui.Lucide.exists(icon)) icon else "award", if (look.dark) DARK_GLYPH else Color.White, size = size * 0.62f, stroke = 2.25f)
                }
            }
        }
    }
    if (!named) return mark()
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        mark()
        Txt(look.label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1)
    }
}

/** The dark glyph, for light colours a white icon would vanish on (web DARK_GLYPH). */
private val DARK_GLYPH = Color(0xFF1B1724)

private fun opaque(rgb: Int) = Color(0xFF000000 or (rgb.toLong() and 0xFFFFFF))

/** The outlines in a 24 by 24 box, the same points as the web's SHAPE_PATHS. */
private val SHAPE_PATHS = mapOf(
    "shield" to "M12 0.6 L22.2 4.2 Q22.6 16.6 12 23.4 Q1.4 16.6 1.8 4.2 Z",
    "hexagon" to "M12 0.4 L22.3 6.2 L22.3 17.8 L12 23.6 L1.7 17.8 L1.7 6.2 Z",
)

private val shapes = HashMap<String, Shape>()

/** An event badge's outline: a disc, a rounded square (30% corners) or one of [SHAPE_PATHS] scaled to the size. */
fun badgeShape(name: String): Shape = shapes.getOrPut(name) {
    val d = SHAPE_PATHS[name]
    when {
        name == "rounded" -> RoundedCornerShape(27)
        d == null -> CircleShape
        else -> {
            val path = PathParser().parsePathString(d).toPath()
            GenericShape { sz, _ ->
                addPath(path)
                transform(Matrix().apply { scale(sz.width / 24f, sz.height / 24f) })
            }
        }
    }
}

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
    val shape = RoundedCornerShape(8.dp)
    FlowRow(
        modifier
            .background(c.textStrong.copy(alpha = 0.07f), shape)
            .ring(shape, c.textStrong.copy(alpha = 0.10f))
            .padding(horizontal = 5.dp, vertical = 3.dp),
        horizontalArrangement = Arrangement.spacedBy(3.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp),
        itemVerticalAlignment = Alignment.CenterVertically,
    ) {
        for (b in list) BadgePill(b, userId, flags, plan)
    }
}
