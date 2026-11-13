package fish.nexa.ui.profile

import android.graphics.Bitmap
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import coil3.request.ImageRequest
import coil3.request.SuccessResult
import coil3.request.allowHardware
import coil3.toBitmap
import fish.nexa.media.Media
import fish.nexa.ui.components.AvatarColor
import fish.nexa.ui.theme.NexaColors
import fish.nexa.ui.theme.adjustOklch
import fish.nexa.ui.theme.oklabLightness
import fish.nexa.ui.theme.oklchChroma
import fish.nexa.ui.theme.mixOklab
import fish.nexa.ui.theme.mixSrgb
import kotlin.math.pow

/** The colours a profile card is drawn in (Profile.module.css --profile-* and the card's own tokens). */
data class ProfileTokens(
    val accent: Color,
    val chip: Color,
    val chipHover: Color,
    val bubble: Color,
    val sheet: Color,
    /** Two theme colours: the card is a gradient from [top] to [bottom]. */
    val top: Color? = null,
    val bottom: Color? = null,
    /** The glow of a profile without colours of its own, from its banner or picture. */
    val tint: Color? = null,
    /** "light" when light text sits on dark colours, "dark" for the reverse. */
    val text: String? = null,
    /** What a two-colour profile's badges sit on: opaque, from the card's own colour (web --badge-pill). */
    val badgePill: Color? = null,
)

/** The pill on a card with light text: a shade under the card, never so deep it goes black, and lifted on a near-black card (web --badge-pill). */
private fun badgePillOn(top: Color): Color = adjustOklch(top, maxOf(oklabLightness(top) * 0.74f, 0.34f) - oklabLightness(top), -oklchChroma(top) * 0.15f)

val LocalProfile = staticCompositionLocalOf { ProfileTokens(Color.Unspecified, Color.Unspecified, Color.Unspecified, Color.Unspecified, Color.Unspecified) }

/** lib/avatarColor.ts profileThemeOf / profileTheme, and the card's tokens for each look. */
object ProfileTheme {
    data class Theme(val top: Color, val bottom: Color, val text: String)

    private fun hex(n: Long) = Color(0xFF000000L or (n and 0xFFFFFF))

    fun luminance(c: Color): Float {
        fun lin(v: Float) = if (v <= 0.03928f) v / 12.92f else ((v + 0.055f) / 1.055f).toDouble().pow(2.4).toFloat()
        return 0.2126f * lin(c.red) + 0.7152f * lin(c.green) + 0.0722f * lin(c.blue)
    }

    fun of(primary: Long?, secondary: Long?): Theme? {
        if (primary == null || secondary == null) return null
        val top = hex(primary)
        val bottom = hex(secondary)
        val light = (luminance(top) + luminance(bottom)) / 2 > 0.45f
        return Theme(top, bottom, if (light) "dark" else "light")
    }

    /** The banner without a picture: both colours at 135°, the first alone, or the person's avatar colour. */
    fun bannerFill(primary: Long?, secondary: Long?, fallbackId: String?, brand: Color): Brush = when {
        primary != null && secondary != null -> Brush.linearGradient(listOf(hex(primary), hex(secondary)))
        primary != null -> Brush.linearGradient(listOf(hex(primary), hex(primary)))
        fallbackId != null -> AvatarColor.of(fallbackId).let { Brush.linearGradient(listOf(it, it)) }
        else -> Brush.linearGradient(listOf(brand, brand))
    }

    fun tokens(c: NexaColors, theme: Theme?, tint: Color?): ProfileTokens {
        if (theme == null) return ProfileTokens(
            accent = c.brand,
            chip = c.bgActive,
            chipHover = mixSrgb(c.bgActive, c.text, 0.14f),
            bubble = c.bgSurface.copy(alpha = 0.86f),
            sheet = mixOklab(c.bgBase, c.bgFloating, 0.55f),
            tint = tint,
        )
        return if (theme.text == "light") ProfileTokens(
            accent = adjustOklch(theme.top, 0.2f, 0.08f),
            chip = Color.Black.copy(alpha = 0.28f),
            chipHover = Color.Black.copy(alpha = 0.4f),
            bubble = Color(14, 10, 24).copy(alpha = 0.74f),
            sheet = Color.Black.copy(alpha = 0.3f),
            top = theme.top, bottom = theme.bottom, text = "light",
            badgePill = badgePillOn(theme.top),
        ) else ProfileTokens(
            accent = adjustOklch(theme.top, -0.32f, 0.1f),
            chip = Color.White.copy(alpha = 0.55f),
            chipHover = Color.White.copy(alpha = 0.78f),
            bubble = Color.White.copy(alpha = 0.88f),
            sheet = Color.White.copy(alpha = 0.42f),
            top = theme.top, bottom = theme.bottom, text = "dark",
            badgePill = mixOklab(theme.top, Color.White, 0.4f),
        )
    }

    /** The card's own colours over a two-colour profile, so its words read on it. */
    fun colors(c: NexaColors, theme: Theme?): NexaColors {
        if (theme == null) return c
        return if (theme.text == "light") c.copy(
            text = Color(0xFFF5F5F7), textStrong = Color.White, textSubtle = Color.White.copy(alpha = 0.86f), textMuted = Color.White.copy(alpha = 0.72f),
            bgSurface = Color.Black.copy(alpha = 0.3f), bgRaised = Color.Black.copy(alpha = 0.34f), bgInput = Color.Black.copy(alpha = 0.26f),
            bgHover = Color.White.copy(alpha = 0.1f), bgActive = Color.White.copy(alpha = 0.18f),
            border = Color.White.copy(alpha = 0.24f), borderSubtle = Color.White.copy(alpha = 0.14f), textBrand = Color.White, link = Color(0xFFD5E6FF),
        ) else c.copy(
            text = Color(0xFF1D1E22), textStrong = Color.Black, textSubtle = Color.Black.copy(alpha = 0.74f), textMuted = Color.Black.copy(alpha = 0.62f),
            bgSurface = Color.White.copy(alpha = 0.5f), bgRaised = Color.White.copy(alpha = 0.46f), bgInput = Color.White.copy(alpha = 0.52f),
            bgHover = Color.Black.copy(alpha = 0.06f), bgActive = Color.Black.copy(alpha = 0.12f),
            border = Color.Black.copy(alpha = 0.18f), borderSubtle = Color.Black.copy(alpha = 0.12f), textBrand = Color(0xFF1D1E22), link = Color(0xFF0B4EA2),
        )
    }

    private val known = HashMap<String, Color?>()

    /** lib/dominantColor.ts: the colour most of a picture is, its vivid parts counted more. */
    fun dominant(bmp: Bitmap): Color? {
        val small = Bitmap.createScaledBitmap(bmp, 24, 24, true)
        data class Bucket(var weight: Float = 0f, var r: Float = 0f, var g: Float = 0f, var b: Float = 0f)
        val buckets = HashMap<Int, Bucket>()
        for (y in 0 until 24) for (x in 0 until 24) {
            val p = small.getPixel(x, y)
            if ((p ushr 24) < 128) continue
            val r = (p shr 16) and 255
            val g = (p shr 8) and 255
            val b = p and 255
            val chroma = (maxOf(r, g, b) - minOf(r, g, b)) / 255f
            val weight = 0.15f + chroma
            val key = ((r shr 5) shl 6) or ((g shr 5) shl 3) or (b shr 5)
            val cur = buckets.getOrPut(key) { Bucket() }
            cur.weight += weight
            cur.r += r * weight
            cur.g += g * weight
            cur.b += b * weight
        }
        if (small !== bmp) small.recycle()
        val best = buckets.values.maxByOrNull { it.weight } ?: return null
        return Color(Math.round(best.r / best.weight), Math.round(best.g / best.weight), Math.round(best.b / best.weight))
    }

    /** useDominantColor: the picture's colour, once it has loaded. */
    @Composable
    fun rememberDominant(url: String?): Color? {
        val context = LocalContext.current
        var color by remember(url) { mutableStateOf(url?.let { synchronized(known) { known[it] } }) }
        LaunchedEffect(url) {
            if (url == null || synchronized(known) { known.containsKey(url) }) return@LaunchedEffect
            val result = Media.loader.execute(ImageRequest.Builder(context).data(url).size(64).allowHardware(false).build())
            val found = (result as? SuccessResult)?.image?.toBitmap()?.let { dominant(it) }
            synchronized(known) { known[url] = found }
            color = found
        }
        return color
    }
}
