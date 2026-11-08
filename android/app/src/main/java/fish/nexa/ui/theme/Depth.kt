package fish.nexa.ui.theme

import androidx.compose.foundation.background
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.draw.dropShadow
import androidx.compose.ui.draw.innerShadow
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.ClipOp
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.addOutline
import androidx.compose.ui.graphics.drawscope.clipPath
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.shadow.Shadow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.DpOffset
import androidx.compose.ui.unit.dp

/**
 * Depth made from light, as the end of tokens.css describes it: light comes
 * from above, so a raised thing catches it along its top edge and throws a
 * shadow tinted with the theme's own darkness, never a neutral grey. These are
 * the CSS box-shadows one for one (Compose's dropShadow/innerShadow take the
 * same blur, spread and offset).
 */
private fun Modifier.shadowLayer(shape: Shape, y: Float, blur: Float, spread: Float, color: Color): Modifier =
    dropShadow(shape, Shadow(radius = blur.dp, color = color, spread = spread.dp, offset = DpOffset(0.dp, y.dp)))

/** One CSS `box-shadow: 0 y blur spread color` layer, for the few shadows that are not tokens. */
fun Modifier.boxShadow(shape: Shape, y: Float, blur: Float, color: Color, spread: Float = 0f): Modifier = shadowLayer(shape, y, blur, spread, color)

/** `inset 0 1px 0 color`: the lit top edge. */
fun Modifier.topEdge(shape: Shape, color: Color): Modifier = innerShadow(shape, Shadow(radius = 0.dp, color = color, offset = DpOffset(0.dp, 1.dp)))

/**
 * `0 0 0 1px color`: a hairline ring outside the shape. Like a CSS box-shadow
 * it never draws inside the shape, so it can sit anywhere in a modifier chain
 * without painting over the background it surrounds.
 */
fun Modifier.ring(shape: Shape, color: Color, width: Dp = 1.dp): Modifier = drawWithCache {
    val w = width.toPx()
    val inner = Path().apply { addOutline(shape.createOutline(size, layoutDirection, this@drawWithCache)) }
    val outer = Path().apply {
        addOutline(shape.createOutline(Size(size.width + 2 * w, size.height + 2 * w), layoutDirection, this@drawWithCache))
        translate(Offset(-w, -w))
    }
    onDrawBehind { clipPath(inner, ClipOp.Difference) { drawPath(outer, color) } }
}

fun Modifier.shadowLow(shape: Shape, c: NexaColors) = shadowLayer(shape, 1f, 2f, 0f, c.shadeAt(0.55f))

fun Modifier.shadowMedium(shape: Shape, c: NexaColors) =
    shadowLayer(shape, 1f, 2f, 0f, c.shadeAt(0.45f)).shadowLayer(shape, 6f, 16f, -4f, c.shadeAt(0.55f))

fun Modifier.shadowHigh(shape: Shape, c: NexaColors) =
    shadowLayer(shape, 2f, 6f, 0f, c.shadeAt(0.40f)).shadowLayer(shape, 18f, 48f, -10f, c.shadeAt(0.80f))

/** A Popup's card: small enough to end inside 12 dp, since a Popup's own window cuts off whatever goes past it. */
fun Modifier.shadowInPopup(shape: Shape, c: NexaColors) =
    shadowLayer(shape, 2f, 6f, 0f, c.shadeAt(0.40f)).shadowLayer(shape, 3f, 9f, -1f, c.shadeAt(0.50f))

fun Modifier.shadowPanel(shape: Shape, c: NexaColors) =
    shadowLayer(shape, 1f, 3f, 0f, c.shadeAt(0.50f)).shadowLayer(shape, 10f, 30f, -12f, c.shadeAt(0.60f))

/** `--lift`: a raised thing, its lit top edge and the shadow under it. Background goes after this. */
fun Modifier.lift(shape: Shape, c: NexaColors) = shadowMedium(shape, c)
fun Modifier.liftLow(shape: Shape, c: NexaColors) = shadowLow(shape, c)

/** `--glow-brand`: the accent as light round a pressed button or a chosen tile. */
fun Modifier.glowBrand(shape: Shape, c: NexaColors) =
    ring(shape, c.brand.copy(alpha = 0.55f)).shadowLayer(shape, 6f, 22f, -6f, c.brand.copy(alpha = 0.75f))

/** `--ring-glow`: a soft halo for a field in use. */
fun Modifier.ringGlow(shape: Shape, c: NexaColors) = ring(shape, c.focusRing.copy(alpha = 0.28f), 3.dp)

/** `--grad-brand`: a fill with the light on it, a touch brighter at the top. */
fun brandGradient(c: NexaColors, brand: Color = c.brand): Brush {
    val top = mixOklab(brand, Color.White, 0.16f)
    val bottom = mixOklab(brand, Color.Black, 0.08f)
    return Brush.verticalGradient(0f to top, 0.55f to brand, 1f to bottom)
}

fun dangerGradient(c: NexaColors): Brush = Brush.verticalGradient(0f to c.gradDangerTop, 0.6f to c.danger, 1f to c.danger)
fun positiveGradient(c: NexaColors): Brush = Brush.verticalGradient(0f to c.gradPositiveTop, 0.6f to c.positive, 1f to c.positive)

/** `--sheen`: a faint brightness at the top of a raised panel. */
fun sheen(c: NexaColors): Brush = Brush.verticalGradient(0f to c.sheenTop, 0.6f to Color.Transparent, 1f to Color.Transparent)

/**
 * A floating panel on the canvas (the sidebar, the composer, menus, sheets):
 * its fill, a sheen, a lit top edge, a hairline edge where the canvas is as
 * dark as the panel, and the panel shadow.
 */
fun Modifier.panel(shape: Shape, c: NexaColors, fill: Color = c.bgRaised, high: Boolean = false): Modifier =
    (if (high) shadowHigh(shape, c) else shadowPanel(shape, c))
        .ring(shape, c.panelEdge)
        .background(fill, shape)
        .background(sheen(c), shape)
        .topEdge(shape, c.edgeLight)

/** A well sunk into its panel (text fields, the recovery key): the shadow falls inside, along the top. */
fun Modifier.sunken(shape: Shape, c: NexaColors, fill: Color = c.bgInput): Modifier =
    background(fill, shape).innerShadow(shape, Shadow(radius = 2.dp, color = c.shadeAt(0.40f), offset = DpOffset(0.dp, 1.dp)))
