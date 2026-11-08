package fish.nexa.ui

import android.content.Context
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.Image
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import java.util.concurrent.ConcurrentHashMap

/**
 * Lucide, the icon set the web client draws with (lucide-react), from
 * assets/lucide.json (android/tools/lucide_assets.py): every icon by name, at
 * 24 x 24 with a round 2 px stroke in the colour of the text around it.
 * Names are the web's component names ("Mic", "AlertTriangle") or file names
 * ("triangle-alert"), so a channel glyph or a tag badge chosen by name on the
 * web draws the same here.
 */
object Lucide {
    private lateinit var icons: JsonObject
    private lateinit var aliases: JsonObject
    private val cache = ConcurrentHashMap<String, ImageVector>()

    fun init(context: Context) {
        val root = Json.parseToJsonElement(context.assets.open("lucide.json").use { it.readBytes().decodeToString() }) as JsonObject
        icons = root["icons"] as JsonObject
        aliases = root["aliases"] as JsonObject
    }

    /** The file name for a component name or a file name; null if there is no such icon. */
    fun resolve(name: String): String? {
        if (icons.containsKey(name)) return name
        (aliases[name] as? JsonPrimitive)?.contentOrNull?.let { return it }
        val kebab = name.replace(Regex("([a-z0-9])([A-Z])"), "$1-$2").replace(Regex("([A-Za-z])([0-9])"), "$1-$2").lowercase()
        return kebab.takeIf { icons.containsKey(it) }
    }

    fun exists(name: String) = resolve(name) != null

    /** Every icon's file name, for a search over them all. */
    fun names(): Set<String> = icons.keys

    /** [filled] paints the shapes in too, as lucide-react's fill="currentColor" does (a solid play mark, a starred star). */
    fun vector(name: String, stroke: Float = 2f, filled: Boolean = false): ImageVector? {
        val file = resolve(name) ?: return null
        val key = "$file@$stroke${if (filled) "#f" else ""}"
        cache[key]?.let { return it }
        val paths = icons[file] as? JsonArray ?: return null
        val b = ImageVector.Builder(name = file, defaultWidth = 24.dp, defaultHeight = 24.dp, viewportWidth = 24f, viewportHeight = 24f)
        for (p in paths) {
            val d = (p as? JsonPrimitive)?.contentOrNull ?: continue
            b.addPath(
                pathData = PathParser().parsePathString(d).toNodes(),
                fill = if (filled) SolidColor(Color.Black) else null,
                stroke = SolidColor(Color.Black),
                strokeLineWidth = stroke,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
        }
        return b.build().also { cache[key] = it }
    }
}

/** A Lucide icon, tinted. `label` makes it speak to TalkBack; without one it is decoration. */
@Composable
fun Icon(name: String, tint: Color, modifier: Modifier = Modifier, size: Dp = 20.dp, stroke: Float = 2f, label: String? = null, filled: Boolean = false) {
    val v = remember(name, stroke, filled) { Lucide.vector(name, stroke, filled) } ?: return
    val painter = rememberVectorPainter(v)
    Image(
        painter = painter,
        contentDescription = null,
        colorFilter = ColorFilter.tint(tint),
        modifier = modifier.size(size).let { m -> if (label != null) m.semantics { contentDescription = label } else m },
    )
}
