package fish.nexa.ui.bugs

import android.net.Uri
import android.os.Build
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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import coil3.compose.AsyncImage
import fish.nexa.BuildConfig
import fish.nexa.Nexa
import fish.nexa.net.ApiError
import fish.nexa.ui.I18n
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.ToggleLine
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.layers.Layer
import fish.nexa.ui.layers.Layers
import fish.nexa.ui.layers.Sheet
import fish.nexa.ui.nav.Nav
import fish.nexa.ui.nav.Route
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.put

/** Where the dialog was opened from, which the report can say (lib/bugs.ts BugFrom). */
private val FROM = setOf("dm", "group", "friends", "settings", "nexa")

/** What the server holds a report to (lib/bugs.ts BUG_LIMITS). */
private object Limits {
    const val SUBJECT_MIN = 3
    const val SUBJECT_MAX = 120
    const val DESCRIPTION_MIN = 10
    const val DESCRIPTION_MAX = 4000
    const val FILES = 5
    const val IMAGE_BYTES = 10L * 1024 * 1024
    const val VIDEO_BYTES = 50L * 1024 * 1024
    const val TOTAL_BYTES = 100L * 1024 * 1024
    val IMAGES = setOf("image/png", "image/jpeg", "image/gif", "image/webp")
    val VIDEOS = setOf("video/mp4", "video/quicktime", "video/webm")
}

/** What was written before the dialog closed without sending, kept while the app runs. */
private val drafts = mutableMapOf("bug" to ("" to ""), "suggestion" to ("" to ""))

/** One file on its way into a report: shown from this device while it goes up. */
private class Pending(val uri: Uri, val name: String, val type: String, val size: Long) {
    val video get() = type.startsWith("video/")
    var progress by mutableStateOf(0f)
    var state by mutableStateOf("uploading")
    var id: String? = null
    var error by mutableStateOf<String?>(null)
    var job: Job? = null
}

/** The words for a failed upload or send that the server named by code. */
private fun problemText(e: Exception, fallback: String): String = when ((e as? ApiError)?.code) {
    "image_too_large" -> t("bugs.form.imageTooLarge")
    "video_too_large", "too_large" -> t("bugs.form.videoTooLarge")
    "unsupported_type" -> t("bugs.form.fileType")
    "bad_dimensions" -> t("bugs.form.imageTooBig")
    "files_too_large" -> t("bugs.form.filesTooLarge")
    "attachment_gone" -> t("bugs.form.fileGone")
    "rate_limited" -> t("bugs.form.rateLimited")
    else -> (e as? ApiError)?.message?.takeIf { it.isNotBlank() } ?: fallback
}

/**
 * The facts a report carries unless the reporter turns them off: which app
 * and version, the system, the screen and window, the language and look.
 * Never anything from a conversation, and never a key.
 */
private fun deviceContext(from: String): Map<String, String> {
    val res = Nexa.app.resources
    val m = res.displayMetrics
    val cfg = res.configuration
    val wDp = Math.round(m.widthPixels / m.density)
    val hDp = Math.round(m.heightPixels / m.density)
    val theme = (Nexa.settings.doc.value["theme"] as? JsonPrimitive)?.contentOrNull ?: ""
    val out = linkedMapOf(
        "app" to "android",
        "app_version" to BuildConfig.VERSION_NAME,
        "os" to "Android ${Build.VERSION.RELEASE}",
        "device" to if (cfg.smallestScreenWidthDp < 600) "phone" else "tablet",
        "screen" to "$wDp × $hDp, ${Math.round(m.density * 100) / 100.0}x",
        "window" to "${cfg.screenWidthDp} × ${cfg.screenHeightDp}",
        "language" to I18n.tag,
        "theme" to if (theme.startsWith("custom-")) "custom" else theme,
        "opened_from" to (if (from in FROM) from else ""),
    )
    return out.filterValues { it.isNotEmpty() }
}

/** A sent report's device details, in reading order and put into words (BugFacts). */
fun factRows(c: JsonObject): List<Pair<String, String>> = contextRows(c.mapNotNull { (k, v) -> (v as? JsonPrimitive)?.contentOrNull?.let { k to it } }.toMap())

/** A report's context in reading order, put into words (lib/bugs.ts contextRows). */
private fun contextRows(c: Map<String, String>): List<Pair<String, String>> {
    val order = listOf("app", "app_version", "build", "server", "browser", "os", "device", "screen", "window", "language", "theme", "custom_css", "plugins", "opened_from")
    return order.mapNotNull { key ->
        val v = c[key]?.takeIf { it.isNotEmpty() } ?: return@mapNotNull null
        val label = when (key) {
            "app" -> t("bugs.context.app")
            "app_version" -> t("bugs.context.appVersion")
            "build" -> t("bugs.context.build")
            "server" -> t("bugs.context.server")
            "browser" -> t("bugs.context.browser")
            "os" -> t("bugs.context.system")
            "device" -> t("bugs.context.device")
            "screen" -> t("bugs.context.screen")
            "window" -> t("bugs.context.window")
            "language" -> t("bugs.context.language")
            "theme" -> t("bugs.context.theme")
            "custom_css" -> t("bugs.context.customCss")
            "plugins" -> t("bugs.context.plugins")
            else -> t("bugs.context.openedFrom")
        }
        val value = when (key) {
            "app" -> when (v) {
                "desktop" -> t("bugs.context.appDesktop")
                "web" -> t("bugs.context.appWeb")
                "android" -> t("bugs.context.appAndroid")
                else -> v
            }
            "device" -> when (v) {
                "phone" -> t("bugs.context.phone")
                "tablet" -> t("bugs.context.tablet")
                "computer" -> t("bugs.context.computer")
                else -> v
            }
            "custom_css" -> when (v) {
                "on" -> t("bugs.context.on")
                "off" -> t("bugs.context.off")
                else -> v
            }
            "theme" -> if (v == "custom") t("bugs.context.customTheme") else v
            "opened_from" -> if (v in FROM) t("bugs.from.$v") else v
            else -> v
        }
        label to value
    }
}

/**
 * Report a Bug (ReportBugModal.tsx): what went wrong in the reporter's own
 * words, the screenshots or recordings that show it, and unless they turn it
 * off, the facts about their app and phone, which they can read before
 * sending. Files go up as they are added, so sending is quick. It says
 * plainly that reports are not end-to-end encrypted, and once sent, where the
 * answer will come.
 */
fun openBugReport(from: String, kind: String = "bug") {
    lateinit var layer: Layer
    layer = Layer("report-bug", canClose = { true }) { close -> ReportBugSheet(layer, from, kind, close) }
    Layers.open(layer)
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ReportBugSheet(layer: Layer, from: String, kind: String, close: () -> Unit) {
    val c = Nx.colors
    val idea = kind == "suggestion"
    // the words that differ between the two: a key under bugs.form or suggestions.form
    fun w(key: String, vararg values: Pair<String, Any>) = t("${if (idea) "suggestions" else "bugs"}.form.$key", *values)
    val draft = drafts.getValue(kind)
    val scope = androidx.compose.runtime.rememberCoroutineScope()
    var subject by remember { mutableStateOf(draft.first) }
    var description by remember { mutableStateOf(draft.second) }
    val files = remember { mutableStateListOf<Pending>() }
    var fileErrors by remember { mutableStateOf(listOf<String>()) }
    var withContext by remember { mutableStateOf(!idea) }
    var showFacts by remember { mutableStateOf(false) }
    var tried by remember { mutableStateOf(false) }
    var sending by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var sent by remember { mutableStateOf<Pair<String, Long>?>(null) }
    val context = remember { deviceContext(from) }
    val facts = remember(context) { contextRows(context) }

    // what is written stays for the next time, until it is sent
    if (sent == null) drafts[kind] = subject to description
    // files still going up stop when the dialog does
    androidx.compose.runtime.DisposableEffect(Unit) { onDispose { files.forEach { if (it.state == "uploading") it.job?.cancel() } } }

    fun upload(p: Pending) {
        p.state = "uploading"
        p.progress = 0f
        p.error = null
        p.job = scope.launch {
            try {
                val name = java.net.URLEncoder.encode(p.name.take(120), "UTF-8")
                val body = fish.nexa.net.UriBody(p.uri, p.type, p.size) { f -> p.progress = f }
                val req = Nexa.api.requestBuilder("/bugs/attachments?name=$name").post(body).header("Content-Type", p.type).build()
                val res = withContext(kotlinx.coroutines.Dispatchers.IO) { Nexa.api.execute(Nexa.api.http, req, true) } as? JsonObject
                p.id = (res?.get("id") as? JsonPrimitive)?.contentOrNull
                p.progress = 1f
                p.state = if (p.id != null) "done" else "failed"
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                p.state = "failed"
                p.error = problemText(e, t("bugs.form.uploadFailed"))
            }
        }
    }

    fun add(uris: List<Uri>) {
        val resolver = Nexa.app.contentResolver
        val problems = ArrayList<String>()
        var count = files.size
        var bytes = files.sumOf { it.size }
        for (uri in uris) {
            val type = (resolver.getType(uri) ?: "").lowercase()
            var name = ""
            var size = 0L
            runCatching {
                resolver.query(uri, arrayOf(android.provider.OpenableColumns.DISPLAY_NAME, android.provider.OpenableColumns.SIZE), null, null, null)?.use { cur ->
                    if (cur.moveToFirst()) {
                        name = cur.getString(0) ?: ""
                        size = cur.getLong(1)
                    }
                }
            }
            if (name.isEmpty()) name = t("bugs.file.screenshot") + "." + type.substringAfter('/').replace("jpeg", "jpg")
            val video = type in Limits.VIDEOS
            val problem = when {
                !video && type !in Limits.IMAGES -> t("bugs.form.fileType")
                count >= Limits.FILES -> t("bugs.form.tooManyFiles", "count" to Limits.FILES)
                !video && size > Limits.IMAGE_BYTES -> t("bugs.form.imageTooLarge")
                video && size > Limits.VIDEO_BYTES -> t("bugs.form.videoTooLarge")
                bytes + size > Limits.TOTAL_BYTES -> t("bugs.form.filesTooLarge")
                else -> null
            }
            if (problem != null) {
                problems += "$name: $problem"
                continue
            }
            count += 1
            bytes += size
            val p = Pending(uri, name, type, size)
            files += p
            upload(p)
        }
        fileErrors = problems
    }

    val pick = rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(Limits.FILES)) { uris -> if (uris.isNotEmpty()) add(uris) }
    fun openPicker() = pick.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageAndVideo))

    val subjectLen = subject.trim().codePointCount(0, subject.trim().length)
    val descriptionLen = description.trim().codePointCount(0, description.trim().length)
    val subjectProblem = when {
        subjectLen < Limits.SUBJECT_MIN -> t("bugs.form.subjectNeeded")
        subjectLen > Limits.SUBJECT_MAX -> t("bugs.form.subjectLong")
        else -> null
    }
    val descriptionProblem = when {
        descriptionLen < Limits.DESCRIPTION_MIN -> w("descriptionNeeded")
        descriptionLen > Limits.DESCRIPTION_MAX -> t("bugs.form.descriptionLong")
        else -> null
    }

    fun send() {
        tried = true
        error = null
        if (subjectProblem != null || descriptionProblem != null || sending) return
        if (files.any { it.state == "uploading" }) {
            error = t("bugs.form.stillUploading")
            return
        }
        if (files.any { it.state == "failed" }) {
            error = t("bugs.form.removeFailed")
            return
        }
        sending = true
        scope.launch {
            try {
                val body = buildJsonObject {
                    put("subject", subject.trim())
                    put("description", description.trim())
                    if (idea) put("kind", "suggestion")
                    if (withContext) put("context", buildJsonObject { for ((k, v) in context) put(k, v) })
                    put("attachments", JsonArray(files.map { f -> buildJsonObject { put("id", f.id); put("name", f.name) } }))
                }
                val res = Nexa.api.request("POST", "/bugs", body) as? JsonObject
                val report = res?.get("report") as? JsonObject
                val id = (report?.get("id") as? JsonPrimitive)?.contentOrNull ?: ""
                val number = (report?.get("number") as? JsonPrimitive)?.contentOrNull?.toLongOrNull() ?: 0
                drafts[kind] = "" to ""
                sent = id to number
            } catch (e: Exception) {
                error = problemText(e, w("sendFailed"))
            } finally {
                sending = false
            }
        }
    }

    val done = sent
    if (done != null) {
        Sheet(layer, title = w("title"), footer = {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
                NxButton(t("bugs.form.done"), close, kind = ButtonKind.SECONDARY)
                NxButton(t("bugs.form.openThread"), {
                    close()
                    Nav.go(Route.NexaConvo)
                })
            }
        }) {
            Column(Modifier.padding(top = 4.dp, bottom = 8.dp).semantics { liveRegion = LiveRegionMode.Polite }, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Box(Modifier.size(44.dp).background(c.positive.copy(alpha = 0.14f), CircleShape), contentAlignment = Alignment.Center) { Icon("CircleCheck", c.positive, size = 24.dp) }
                Txt(w("sentTitle", "number" to t("bugs.number", "number" to done.second)), style = Nx.type.heading.copy(fontSize = Nx.type.sp(18f)), color = c.textStrong, modifier = Modifier.padding(top = 4.dp))
                Txt(w("sentText"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14.5f), lineHeight = Nx.type.sp(21.75f)), color = c.textSubtle)
            }
        }
        return
    }

    Sheet(layer, title = w("title"), footer = {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
            NxButton(t("bugs.form.cancel"), close, kind = ButtonKind.SECONDARY)
            NxButton(w("send"), { send() }, loading = sending)
        }
    }) {
        Column(verticalArrangement = Arrangement.spacedBy(18.dp)) {
            Txt(w("lead"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14.5f), lineHeight = Nx.type.sp(21.75f)), color = c.textSubtle, modifier = Modifier.padding(top = 0.dp))
            Field(t("bugs.form.subject"), count = if (subjectLen > Limits.SUBJECT_MAX - 20) Limits.SUBJECT_MAX - subjectLen else null) {
                NxTextField(
                    subject, { subject = it.take(Limits.SUBJECT_MAX + 20) },
                    placeholder = w("subjectPlaceholder"),
                    error = if (tried) subjectProblem else null,
                    capitalize = true, autoCorrect = true,
                )
            }
            Field(w("description"), count = if (descriptionLen > Limits.DESCRIPTION_MAX - 300) Limits.DESCRIPTION_MAX - descriptionLen else null) {
                NxTextField(
                    description, { description = it },
                    placeholder = w("descriptionPlaceholder"),
                    error = if (tried) descriptionProblem else null,
                    hint = if (tried && descriptionProblem != null) null else w("descriptionHint"),
                    singleLine = false, minLines = 5, capitalize = true, autoCorrect = true,
                    imeAction = androidx.compose.ui.text.input.ImeAction.Default,
                )
            }
            Field(w("files")) {
                if (files.isEmpty()) {
                    // one wide target before there are any files
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .dashed(c.borderStrong)
                            .clip(Radii.Sm)
                            .pressable(remember { MutableInteractionSource() }) { openPicker() }
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(14.dp),
                    ) {
                        Box(Modifier.size(40.dp).background(c.brand.copy(alpha = 0.14f), CircleShape), contentAlignment = Alignment.Center) { Icon("ImagePlus", c.textBrand, size = 20.dp) }
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Txt(w("addFiles"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14.5f), fontWeight = FontWeight(600)), color = c.textStrong)
                            Txt(t("bugs.form.filesLimits"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.2f)), color = c.textMuted)
                        }
                    }
                } else {
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        for (f in files) FileTile(f, retry = { upload(f) }) {
                            if (f.state == "uploading") f.job?.cancel()
                            files.remove(f)
                            fileErrors = emptyList()
                        }
                        if (files.size < Limits.FILES) Column(
                            Modifier
                                .size(92.dp)
                                .dashed(c.borderStrong)
                                .clip(Radii.Sm)
                                .pressable(remember { MutableInteractionSource() }) { openPicker() }
                                .semantics { contentDescription = t("bugs.form.addMore") },
                            verticalArrangement = Arrangement.spacedBy(4.dp, Alignment.CenterVertically),
                            horizontalAlignment = Alignment.CenterHorizontally,
                        ) {
                            Icon("Plus", c.textMuted, size = 20.dp)
                            Txt(t("bugs.form.add"), style = Nx.type.hint.copy(fontWeight = FontWeight(600)), color = c.textMuted)
                        }
                    }
                }
                val failedLines = files.filter { it.state == "failed" }.map { "${it.name}: ${it.error ?: t("bugs.form.uploadFailed")}" }
                for (line in fileErrors + failedLines) ErrorLine(line)
            }
            if (!idea) Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                ToggleLine(t("bugs.form.includeDetails"), withContext, { withContext = it }, description = t("bugs.form.includeDetailsHint"))
                if (withContext) {
                    Row(
                        Modifier.clip(Radii.Xs).pressable(remember { MutableInteractionSource() }) { showFacts = !showFacts }.padding(vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp),
                    ) {
                        Txt(if (showFacts) t("bugs.form.hideDetails") else t("bugs.form.showDetails"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontWeight = FontWeight(600)), color = c.link)
                        Icon(if (showFacts) "ChevronUp" else "ChevronDown", c.link, size = 15.dp)
                    }
                    if (showFacts) Column(
                        Modifier
                            .fillMaxWidth()
                            .background(c.bgBase, Radii.Sm)
                            .border(1.dp, c.borderSubtle, Radii.Sm)
                            .padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalArrangement = Arrangement.spacedBy(4.dp),
                    ) {
                        for ((label, value) in facts) Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                            Txt(label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, modifier = Modifier.width(110.dp))
                            Txt(value, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.text, modifier = Modifier.weight(1f))
                        }
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(Modifier.padding(top = 1.dp)) { Icon("ShieldAlert", c.textMuted, size = 16.dp) }
                Txt(w("notEncrypted"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.85f)), color = c.textMuted)
            }
            error?.let {
                Txt(
                    it, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(20.3f)), color = c.textDanger,
                    modifier = Modifier.fillMaxWidth().background(c.danger.copy(alpha = 0.12f), Radii.Sm).padding(horizontal = 12.dp, vertical = 10.dp).semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        }
    }
}

/** A field's label (and its count near the limit) over what it holds. */
@Composable
private fun Field(label: String, count: Int? = null, content: @Composable () -> Unit) {
    val c = Nx.colors
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Bottom) {
            Txt(label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textStrong, modifier = Modifier.weight(1f))
            if (count != null) Txt(count.toString(), style = Nx.type.hint.copy(fontFeatureSettings = "tnum"), color = if (count < 0) c.textDanger else c.textMuted)
        }
        content()
    }
}

@Composable
private fun ErrorLine(text: String) = Txt(text, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.2f)), color = Nx.colors.textDanger)

/** A 1.5 dp dashed edge in the given colour, at the small radius. */
private fun Modifier.dashed(colour: Color): Modifier = drawBehind {
    val r = 10.dp.toPx()
    drawRoundRect(colour, cornerRadius = CornerRadius(r), style = Stroke(1.5.dp.toPx(), pathEffect = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 3.dp.toPx()))))
}

/** A file as it will arrive: its picture, how far it is up, a way to take it out or try again. */
@Composable
private fun FileTile(f: Pending, retry: () -> Unit, remove: () -> Unit) {
    val c = Nx.colors
    val shape = Radii.Sm
    Box(
        Modifier
            .size(92.dp)
            .clip(shape)
            .background(c.bgBase)
            .border(1.dp, if (f.state == "failed") c.danger else c.borderSubtle, shape),
    ) {
        AsyncImage(
            f.uri, null, contentScale = ContentScale.Crop,
            modifier = Modifier.fillMaxSize().alpha(if (f.state == "uploading") 0.55f else 1f),
        )
        if (f.video) Row(
            Modifier.align(Alignment.BottomStart).padding(6.dp).height(20.dp).background(Color.Black.copy(alpha = 0.62f), RoundedCornerShape(10.dp)).padding(horizontal = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(3.dp),
        ) {
            Icon("Play", Color.White, size = 10.dp)
            Txt(t("bugs.file.video"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(11.5f), fontWeight = FontWeight(600)), color = Color.White)
        }
        if (f.state == "uploading") Box(Modifier.align(Alignment.BottomStart).fillMaxWidth().height(4.dp).background(Color.Black.copy(alpha = 0.35f))) {
            Box(Modifier.fillMaxWidth(f.progress).height(4.dp).background(c.brand))
        }
        if (f.state == "failed") Box(
            Modifier
                .fillMaxSize()
                .background(c.danger.copy(alpha = 0.3f))
                .pressable(remember { MutableInteractionSource() }, onClick = retry)
                .semantics { contentDescription = t("bugs.form.retryFile", "name" to f.name) },
            contentAlignment = Alignment.Center,
        ) { Icon("AlertCircle", Color.White, size = 22.dp) }
        Box(
            Modifier
                .align(Alignment.TopEnd)
                .size(40.dp)
                .pressable(remember { MutableInteractionSource() }, onClick = remove)
                .semantics { contentDescription = t("bugs.form.removeFile", "name" to f.name) },
            contentAlignment = Alignment.TopEnd,
        ) {
            Box(Modifier.padding(4.dp).size(26.dp).background(Color.Black.copy(alpha = 0.62f), CircleShape), contentAlignment = Alignment.Center) { Icon("X", Color.White, size = 14.dp) }
        }
    }
}
