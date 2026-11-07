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
 * staff made, its icon in white on a disc in the colour they chose. Pressing
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
            val tone = look.color?.let { Color(0xFF000000 or (it.toLong() and 0xFFFFFF)) } ?: c.brand
            Box(
                Modifier
                    .size(size)
                    .clip(CircleShape)
                    .background(tone)
                    .background(Brush.verticalGradient(0f to Color.White.copy(alpha = 0.32f), 0.55f to Color.Transparent))
                    .ring(CircleShape, Color.Black.copy(alpha = 0.22f))
                    .then(press),
                contentAlignment = Alignment.Center,
            ) {
                val icon = look.icon ?: "award"
                Icon(if (fish.nexa.ui.Lucide.exists(icon)) icon else "award", Color.White, size = size * 0.62f, stroke = 2.25f)
            }
        }
    }
    if (!named) return mark()
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        mark()
        Txt(look.label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1)
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
