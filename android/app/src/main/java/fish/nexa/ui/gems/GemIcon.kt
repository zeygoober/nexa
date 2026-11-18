package fish.nexa.ui.gems

import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import fish.nexa.data.Ota
import java.text.NumberFormat
import java.util.Locale

/** The Gem, drawn as the badge art is (tools/badge_art.py), wherever an amount of Gems is shown (web GemIcon). */
@Composable
fun GemIcon(size: Dp = 16.dp, modifier: Modifier = Modifier) {
    coil3.compose.AsyncImage(Ota.uri("badges/gem.svg"), null, Modifier.size(size).then(modifier))
}

/** A whole number as the app's language writes it. */
fun gemNumber(n: Long, digits: Int = 0): String = NumberFormat.getNumberInstance(fish.nexa.ui.I18n.locale).apply { maximumFractionDigits = digits }.format(n)
fun gemNumber(n: Double, digits: Int): String = NumberFormat.getNumberInstance(fish.nexa.ui.I18n.locale).apply { maximumFractionDigits = digits }.format(n)

/** A count short enough for a tile's corner: 1.2K (web compact). */
fun compactNumber(n: Long): String =
    if (android.os.Build.VERSION.SDK_INT >= 30) {
        android.icu.number.NumberFormatter.withLocale(fish.nexa.ui.I18n.locale)
            .notation(android.icu.number.Notation.compactShort())
            .precision(android.icu.number.Precision.maxFraction(1))
            .format(n).toString()
    } else if (n < 10_000) gemNumber(n) else gemNumber(n / 1000.0, 1) + "K"
