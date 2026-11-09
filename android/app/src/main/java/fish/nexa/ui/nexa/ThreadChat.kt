package fish.nexa.ui.nexa

import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.ui.layout.layout
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.currentStateAsState
import coil3.compose.AsyncImage
import fish.nexa.Nexa
import fish.nexa.data.BugMessage
import fish.nexa.data.CaseMessage
import fish.nexa.data.NexaFile
import fish.nexa.data.NexaInbox
import fish.nexa.data.NexaPerson
import fish.nexa.data.PrivateFiles
import fish.nexa.media.Fit
import fish.nexa.net.ApiError
import fish.nexa.ui.Icon
import fish.nexa.ui.Time
import fish.nexa.ui.chat.DateDivider
import fish.nexa.ui.chat.JumpButton
import fish.nexa.ui.chat.StartAvatar
import fish.nexa.ui.chat.StartText
import fish.nexa.ui.chat.StartTitle
import fish.nexa.ui.chat.messageKeyboard
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.Spinner
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.components.sidewaysScroll
import fish.nexa.ui.layers.MenuItem
import fish.nexa.ui.layers.openMenu
import fish.nexa.ui.markdown.MarkdownView
import fish.nexa.ui.markdown.RenderCtx
import fish.nexa.ui.media.LightboxItem
import fish.nexa.ui.media.MosaicGrid
import fish.nexa.ui.media.VideoPlayer
import fish.nexa.ui.media.VideoSource
import fish.nexa.ui.media.openLightbox
import fish.nexa.ui.profile.NexaAvatar
import fish.nexa.ui.profile.OfficialBadge
import fish.nexa.ui.shell.copyText
import fish.nexa.ui.shell.openProfile
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.boxShadow
import fish.nexa.ui.theme.brandGradient
import fish.nexa.ui.theme.mixSrgb
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.shadowLow
import fish.nexa.ui.theme.shadowMedium
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.launch

// ---------------------------------------------------------------------------
// A case's thread or a bug report's, as a conversation (ThreadChat.tsx): the
// notice or the report first, then everyone's messages with their names,
// avatars and times, grouped as a conversation groups them, the days apart,
// and a line for everything that happened to it. The box at the foot is the
// conversation's own, with files, emoji and formatting; once staff close the
// thread, a lock bar takes its place. Members and staff see the same thing,
// each from their side.
// ---------------------------------------------------------------------------

/** The most a line can hold; the server refuses anything longer. */
const val MAX_THREAD_MESSAGE = 2000
/** How many files one line carries; the server holds a line to the same. */
const val MAX_THREAD_FILES = 4
/** Lines from the same writer this close together share one header, as in any conversation. */
private const val GROUP_GAP = 7 * 60_000L
/** What a thread takes: pictures and short recordings. */
private val FILE_TYPES = setOf("image/png", "image/jpeg", "image/gif", "image/webp", "video/mp4", "video/webm", "video/quicktime")
/** The size limits the server keeps to, so a file too big is turned away before it goes up. */
private const val IMAGE_MAX = 10L shl 20
private const val VIDEO_MAX = 50L shl 20
/** How sharp the copy a message shows is fetched (twice its size on screen); a GIF comes whole, or it would be its first frame. */
private const val SHOWN_SIZE = 1024

/** One line of a thread, whichever kind of thread it is in. */
data class ThreadLine(
    val id: String,
    val authorId: String,
    val kind: String,
    val event: String,
    val body: String,
    val createdAt: Long,
    val files: List<NexaFile> = emptyList(),
)

fun CaseMessage.line() = ThreadLine(id, authorId, kind, event, body, createdAt, files)
fun BugMessage.line() = ThreadLine(id, authorId, kind, event, body, createdAt, files)

/** The first message of the conversation: the notice Nexa sent ("nexa"), or the report its sender wrote (their id). */
class ThreadOpener(val from: String, val at: Long, val body: @Composable () -> Unit)

/** Someone a thread names: from what the thread's answers carried, this account, or the people the app knows. */
fun threadPerson(id: String): NexaPerson? = NexaInbox.people.value[id]
    ?: Nexa.users.me.value?.takeIf { it.id == id }?.let { NexaPerson(it.id, it.username, it.displayName, it.avatarId) }
    ?: Nexa.users.get(id)?.let { NexaPerson(it.id, it.username, it.displayName, it.avatarId, it.flags) }

/** threadPerson, drawn again when the people the app knows change. */
@Composable
fun rememberPerson(id: String): NexaPerson? {
    NexaInbox.people.collectAsState().value
    Nexa.users.me.collectAsState().value
    return threadPerson(id)
}

/** What a system line says happened (systemLineText); a staff member reopening says who, a member writing does not. */
fun systemLineText(line: ThreadLine, action: String?, staff: Boolean, whoName: String? = null): String {
    val who = whoName ?: personName(threadPerson(line.authorId))
    val what = if (action == "ban") t("nexa.thread.theBan") else t("nexa.thread.theMute")
    return when (line.event) {
        "status" -> t("bugs.thread.statusChanged", "who" to who, "status" to bugStatusLabel(line.body))
        "lifted" -> if (line.body.isNotEmpty()) t("nexa.thread.liftedWithReason", "who" to who, "what" to what, "reason" to line.body) else t("nexa.thread.lifted", "who" to who, "what" to what)
        "closed" -> t("nexa.thread.closed", "who" to who)
        "reopened" -> if (staff && NexaInbox.people.value[line.authorId] != null) t("nexa.thread.reopened", "who" to who) else t("nexa.thread.reopenedByReply")
        "expired" -> t("nexa.thread.expired", "what" to what)
        "replaced" -> t("nexa.thread.replaced", "number" to line.body)
        else -> line.body
    }
}

private fun systemIcon(event: String) = when (event) {
    "lifted" -> "LockOpen"
    "closed" -> "CircleCheck"
    "reopened" -> "RotateCcw"
    "expired" -> "Clock"
    "replaced" -> "ShieldAlert"
    "status" -> "CircleDot"
    else -> "History"
}

@Composable
fun ThreadChat(
    lines: List<ThreadLine>,
    /** Whose side this is seen from: staff see each other's names, members see the team as Nexa. */
    staff: Boolean,
    /** Where the thread's files are fetched from: "cases" or "bugs". */
    fileKind: String,
    placeholder: String,
    modifier: Modifier = Modifier,
    /** The case's action, which some of its system lines name; a bug report has none. */
    action: String? = null,
    opener: ThreadOpener? = null,
    intro: (@Composable () -> Unit)? = null,
    /** Sends a line and its files; without it the lock bar stands where the box would be. */
    onSend: (suspend (String, List<NexaFile>) -> Unit)? = null,
    /** Where a file for the next line goes up. */
    uploadPath: String? = null,
    /** In place of the box once nobody can write here: why, and maybe a way to change it. */
    locked: (@Composable RowScope.() -> Unit)? = null,
    /** The quiet line over the box, where a conversation says who is typing. */
    note: (@Composable RowScope.() -> Unit)? = null,
    /** Called with the newest line when it is on screen in front of the person, so it can be marked read. */
    onSeen: ((String?) -> Unit)? = null,
) {
    val state = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val life by lifecycle.currentStateAsState()
    val lastId = lines.lastOrNull()?.id
    val atBottom by remember { derivedStateOf { state.firstVisibleItemIndex == 0 && state.firstVisibleItemScrollOffset < 120 } }
    val far by remember { derivedStateOf { state.firstVisibleItemIndex > 6 } }
    // seen means seen: at the bottom, with the app in front of the person
    LaunchedEffect(lastId, atBottom, life) {
        if (atBottom && life.isAtLeast(Lifecycle.State.RESUMED)) onSeen?.invoke(lastId)
    }
    // follow new lines while the reader is at the bottom; leave them alone when they scrolled up.
    // Where they were is read here, before the new line is laid out: the list keeps its place by
    // key, which leaves the new line out of sight and the reader no longer at the bottom.
    val follow = atBottom
    LaunchedEffect(lastId) { if (follow) state.scrollToItem(0) }

    // newest at the bottom, where the eye starts, as in any conversation
    val rows = remember(lines, opener?.at) {
        val out = ArrayList<Any>()
        lines.forEachIndexed { i, l ->
            val prev = lines.getOrNull(i - 1)
            val before = prev?.createdAt ?: opener?.at
            val newDay = before == null || !Time.sameDay(before, l.createdAt)
            if (newDay) out += ThreadDay(l.createdAt)
            val grouped = prev != null && !newDay && l.kind != "system" && prev.kind == l.kind && prev.authorId == l.authorId && l.createdAt - prev.createdAt < GROUP_GAP
            out += ThreadRowItem(l, grouped)
        }
        out.reversed()
    }

    Column(modifier.fillMaxSize().imePadding()) {
        Box(Modifier.weight(1f).fillMaxWidth()) {
            LazyColumn(
                Modifier.fillMaxSize().semantics { contentDescription = t("modals.quickSwitcher.messages") },
                state = state,
                reverseLayout = true,
                contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 8.dp),
            ) {
                for (r in rows) when (r) {
                    is ThreadDay -> item("day${r.at}") { DateDivider(r.at) }
                    is ThreadRowItem -> item(r.line.id) {
                        if (r.line.kind == "system") SystemLine(r.line, action, staff) else LineRow(r.line, r.grouped, staff, fileKind)
                    }
                }
                if (opener != null) item("opener") { OpenerRow(opener) }
                if (intro != null) item("intro") { intro() }
            }
            if (far) JumpButton { scope.launch { state.animateScrollToItem(0) } }
        }
        if (note != null && onSend != null) Row(
            Modifier.fillMaxWidth().heightIn(min = 24.dp).padding(start = 16.dp, end = 16.dp, bottom = 2.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            content = note,
        )
        when {
            onSend != null -> ThreadComposer(placeholder, uploadPath) { body, files ->
                onSend(body, files)
                // what you just sent is what you want to see, wherever you had scrolled to
                state.scrollToItem(0)
            }
            locked != null -> LockBar(locked)
        }
    }
}

private data class ThreadDay(val at: Long)
private data class ThreadRowItem(val line: ThreadLine, val grouped: Boolean)

/** What stands where the box was, once nobody can write here: a lock, why, and maybe a way to open it again. */
@Composable
fun RowScope.LockNote(text: String, action: (@Composable () -> Unit)? = null) {
    val c = Nx.colors
    Icon("Lock", c.textMuted, size = 18.dp)
    Txt(text, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), lineHeight = Nx.type.sp(20.25f)), color = c.textMuted, modifier = Modifier.weight(1f))
    action?.invoke()
}

@Composable
private fun LockBar(content: @Composable RowScope.() -> Unit) {
    val c = Nx.colors
    val shape = Radii.Md
    Box(Modifier.fillMaxWidth().navigationBarsPadding().padding(start = 8.dp, end = 8.dp, bottom = 8.dp)) {
        Row(
            Modifier
                .fillMaxWidth()
                .shadowMedium(shape, c)
                .ring(shape, c.borderSubtle)
                .background(c.bgComposer, shape)
                .topEdge(shape, c.edgeLight)
                .heightIn(min = 48.dp)
                .padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            content = content,
        )
    }
}

/**
 * The start of a case's or a report's conversation, as any conversation
 * starts: who it is with, big, what it is about, and who can read it.
 */
@Composable
fun ThreadIntro(avatar: @Composable () -> Unit, title: String, sub: String?, text: String) {
    val c = Nx.colors
    Column(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 20.dp, bottom = 8.dp)) {
        StartAvatar(avatar)
        StartTitle(title)
        if (!sub.isNullOrEmpty()) Txt(sub, style = Nx.type.body.copy(fontSize = Nx.type.sp(18f), fontWeight = FontWeight(500)), color = c.textSubtle, modifier = Modifier.padding(top = 6.dp), maxLines = 1)
        Spacer(Modifier.height(10.dp))
        StartText(text)
    }
}

/** A header's time: a tap shows the exact second, as the conversation's own does. */
@Composable
private fun LineTime(at: Long) {
    val c = Nx.colors
    Txt(Time.timestamp(at), style = Nx.type.timestamp.copy(fontWeight = FontWeight(500)), color = c.textMuted, maxLines = 1)
}

/** The avatar's place in a row: 40 dp at the start, as a conversation's rows have it. */
@Composable
private fun RowAvatar(onClick: (() -> Unit)?, content: @Composable () -> Unit) {
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier
            .padding(start = 12.dp, top = 2.dp)
            .graphicsLayer { val s = if (pressed) 0.95f else 1f; scaleX = s; scaleY = s }
            .then(if (onClick != null) Modifier.pressable(interaction, onClick = onClick) else Modifier),
    ) { content() }
}

@Composable
private fun OpenerRow(opener: ThreadOpener) {
    val c = Nx.colors
    val nexa = opener.from == "nexa"
    val person = if (nexa) null else rememberPerson(opener.from)
    val name = if (nexa) "Nexa" else personName(person)
    val nexaId = NexaInbox.standing.collectAsState().value?.accountId.orEmpty()
    val open = when {
        nexa -> nexaId.takeIf { it.isNotEmpty() }?.let { id -> { openProfile(id) } }
        person != null && !person.deleted -> { { openProfile(opener.from) } }
        else -> null
    }
    Box(Modifier.fillMaxWidth().padding(top = 17.dp)) {
        RowAvatar(open) { if (nexa) NexaAvatar(40.dp) else Avatar(opener.from, name, person?.avatarId, 40.dp) }
        Column(Modifier.fillMaxWidth().padding(start = 64.dp, end = 16.dp, top = 2.dp, bottom = 2.dp)) {
            Row(Modifier.heightIn(min = 22.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                fish.nexa.ui.chat.PersonName(opener.from.takeIf { !nexa && person?.deleted != true }, name, style = Nx.type.body.copy(fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f, fill = false).then(if (open != null) Modifier.pressable(remember { MutableInteractionSource() }, onClick = open) else Modifier))
                if (nexa) OfficialBadge(compact = true)
                LineTime(opener.at)
            }
            Column(Modifier.padding(top = 4.dp).widthIn(max = 640.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { opener.body() }
        }
    }
}

/** A line: the team's as from Nexa ("sent by" whoever wrote it) to the member, as themselves to each other; the member's as from them. */
@Composable
private fun LineRow(m: ThreadLine, grouped: Boolean, staffView: Boolean, fileKind: String) {
    val c = Nx.colors
    val person = rememberPerson(m.authorId)
    val staffLine = m.kind == "staff"
    val asNexa = staffLine && !staffView
    val name = if (asNexa) "Nexa" else personName(person)
    val nexaId = NexaInbox.standing.collectAsState().value?.accountId.orEmpty()
    val openAuthor = if (person != null && !person.deleted) ({ openProfile(m.authorId) }) else null
    val openShown = if (asNexa) nexaId.takeIf { it.isNotEmpty() }?.let { id -> { openProfile(id) } } else openAuthor
    val label = if (grouped) t("chat.messageItem.whoWhen", "name" to name, "time" to Time.timestamp(m.createdAt)) else null
    Box(
        Modifier
            .fillMaxWidth()
            .padding(top = if (grouped) 0.dp else 17.dp)
            .then(if (label != null) Modifier.semantics { contentDescription = label } else Modifier)
            // a line's words can be copied from its menu, as a message's can
            .then(if (m.body.isNotEmpty()) Modifier.pressable(remember { MutableInteractionSource() }, onLongClick = {
                openMenu(listOf(MenuItem.Action(t("chat.messageMenu.copyText")) { copyText(m.body) }), haptic = true)
            }) {} else Modifier),
    ) {
        if (!grouped) RowAvatar(openShown) {
            if (asNexa) NexaAvatar(40.dp) else Avatar(m.authorId, name, person?.avatarId, 40.dp)
        }
        Column(Modifier.fillMaxWidth().padding(start = 64.dp, end = 16.dp, top = 2.dp, bottom = 2.dp).heightIn(min = 22.dp)) {
            if (!grouped) Row(Modifier.heightIn(min = 22.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                fish.nexa.ui.chat.PersonName(m.authorId.takeIf { !asNexa && person?.deleted != true }, name, style = Nx.type.body.copy(fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f, fill = false).then(if (openShown != null) Modifier.pressable(remember { MutableInteractionSource() }, onClick = openShown) else Modifier))
                if (asNexa) {
                    OfficialBadge(compact = true)
                    Txt(
                        t("nexa.thread.sentBy") + " " + personName(person),
                        style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(500)), color = c.textMuted, maxLines = 1,
                        modifier = Modifier.weight(1f, fill = false).then(if (openAuthor != null) Modifier.pressable(remember { MutableInteractionSource() }, onClick = openAuthor) else Modifier),
                    )
                } else if (staffLine) StaffTag()
                LineTime(m.createdAt)
            }
            if (m.body.isNotEmpty()) MarkdownView(m.body, RenderCtx(surface = c.bgCanvas))
            if (m.files.isNotEmpty()) ThreadFiles(m.files, fileKind, name)
        }
    }
}

/** "as Nexa": a staff line, as the staff see it. */
@Composable
private fun StaffTag() {
    val c = Nx.colors
    Box(Modifier.background(c.brand.copy(alpha = 0.16f), RoundedCornerShape(4.dp)).padding(horizontal = 6.dp, vertical = 1.dp)) {
        Txt(t("nexa.thread.asNexa"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(11f), fontWeight = FontWeight(600)), color = c.textBrand, maxLines = 1)
    }
}

/** A line about the thread itself (a lift, a close, a status), drawn as a conversation draws its own (SystemMessage). */
@Composable
private fun SystemLine(line: ThreadLine, action: String?, staff: Boolean) {
    val c = Nx.colors
    val tint = when {
        line.event == "lifted" || line.event == "closed" || (line.event == "status" && line.body == "resolved") -> c.positive
        line.event == "replaced" || (line.event == "status" && line.body == "in_progress") -> c.warning
        line.event == "status" && line.body == "open" -> c.textBrand
        else -> c.textMuted
    }
    val person = rememberPerson(line.authorId)
    Box(Modifier.fillMaxWidth().padding(top = 17.dp).heightIn(min = 26.dp).padding(start = 64.dp, end = 16.dp, top = 4.dp, bottom = 4.dp)) {
        Icon(systemIcon(line.event), tint, size = 18.dp, modifier = Modifier.padding(top = 2.dp).layout { measurable, constraints ->
            val p = measurable.measure(constraints)
            layout(0, p.height) { p.placeRelative((-40 * density).toInt(), 0) }
        })
        fish.nexa.ui.chat.PersonLine(Nx.type.body.copy(fontSize = Nx.type.sp(15.5f), lineHeight = Nx.type.sp(22f)), c.textMuted) { names ->
            buildAnnotatedString {
                systemLineText(line, action, staff, fish.nexa.ui.chat.PERSON_SLOT).split(fish.nexa.ui.chat.PERSON_SLOT).forEachIndexed { i, part ->
                    if (i > 0) names.append(this, line.authorId.takeIf { person?.deleted != true }, personName(person))
                    append(part)
                }
                withStyle(SpanStyle(fontSize = Nx.type.sp(12f), color = c.textMuted)) { append("  " + Time.timestamp(line.createdAt)) }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// files (ThreadFiles.tsx)

private fun fileName(f: NexaFile) = f.name.ifEmpty { if (f.mime.startsWith("video/")) t("bugs.file.recording") else t("bugs.file.screenshot") }

/**
 * The pictures and recordings sent in a case thread or a bug report, drawn as
 * a conversation draws its attachments: one picture at its own shape, two or
 * more sharing one grid, a recording in the same player, and the viewer
 * stepping through a line's pictures. They are fetched with this session's
 * token (PrivateFiles), since nothing public serves them.
 */
@Composable
fun ThreadFiles(files: List<NexaFile>, kind: String, from: String?) {
    val scope = rememberCoroutineScope()
    val pictures = remember(files) { files.filter { !it.mime.startsWith("video/") } }
    val tiles = if (pictures.size > 1) Fit.mosaic(pictures.size) else null
    // the viewer shows what it is given, so the full-size pictures are fetched first
    fun open(f: NexaFile) {
        scope.launch {
            try {
                val full = pictures.map { PrivateFiles.file(kind, it.id) }
                openLightbox(pictures.mapIndexed { i, p -> LightboxItem(fileName(p), p.mime, src = Uri.fromFile(full[i]).toString(), from = from) }, pictures.indexOf(f).coerceAtLeast(0))
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(t("bugs.card.fileFailed"), "error")
            }
        }
    }
    Column(Modifier.padding(top = 4.dp), verticalArrangement = Arrangement.spacedBy(6.dp), horizontalAlignment = Alignment.Start) {
        if (tiles != null) MosaicGrid(tiles) { for (p in pictures) ThreadPicture(p, kind, tile = true) { open(p) } }
        for (f in files) {
            if (f.mime.startsWith("video/")) ThreadRecording(f, kind)
            else if (tiles == null) ThreadPicture(f, kind, tile = false) { open(f) }
        }
    }
}

@Composable
private fun ThreadPicture(f: NexaFile, kind: String, tile: Boolean, onOpen: () -> Unit) {
    val c = Nx.colors
    var file by remember(f.id) { mutableStateOf<java.io.File?>(null) }
    var failed by remember(f.id) { mutableStateOf(false) }
    var tries by remember(f.id) { mutableStateOf(0) }
    LaunchedEffect(f.id, tries) {
        failed = false
        file = runCatching { PrivateFiles.file(kind, f.id, if (f.mime == "image/gif") null else SHOWN_SIZE) }.onFailure { failed = true }.getOrNull()
    }
    val fit = Fit.media(f.width?.toDouble(), f.height?.toDouble())
    val name = fileName(f)
    Box(
        (if (tile) Modifier.fillMaxSize() else Modifier.width(fit.width.dp).aspectRatio(fit.width.toFloat() / fit.height).ring(Radii.Md, c.borderSubtle).clip(Radii.Md))
            .background(c.bgInput)
            .pressable(remember { MutableInteractionSource() }) { if (failed) tries++ else if (file != null) onOpen() }
            .semantics { contentDescription = if (failed) t("bugs.card.retryPicture", "name" to name) else t("bugs.card.openPicture", "name" to name) },
        contentAlignment = Alignment.Center,
    ) {
        val f2 = file
        when {
            f2 != null -> AsyncImage(f2, name, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
            failed -> Icon("ImageOff", c.textMuted, size = 22.dp)
            else -> Spinner(size = 20.dp)
        }
    }
}

/** A recording: a frame the player's shape that loads it when pressed, then the conversation's own player. */
@Composable
private fun ThreadRecording(f: NexaFile, kind: String) {
    val c = Nx.colors
    var wanted by remember(f.id) { mutableStateOf(false) }
    var failed by remember(f.id) { mutableStateOf(false) }
    var file by remember(f.id) { mutableStateOf<java.io.File?>(null) }
    var tries by remember(f.id) { mutableStateOf(0) }
    LaunchedEffect(f.id, wanted, tries) {
        if (!wanted || file != null) return@LaunchedEffect
        failed = false
        file = runCatching { PrivateFiles.file(kind, f.id) }.onFailure { failed = true }.getOrNull()
    }
    val name = fileName(f)
    val loaded = file
    if (loaded != null) {
        VideoPlayer(VideoSource.Url(Uri.fromFile(loaded).toString()), name, f.width?.toDouble(), f.height?.toDouble())
        return
    }
    val box = Fit.video((f.width ?: 640).toDouble(), (f.height ?: 360).toDouble())
    Box(
        Modifier.padding(top = 4.dp).width(box.width.dp).aspectRatio(box.width.toFloat() / box.height).clip(Radii.Sm).background(Color.Black),
        contentAlignment = Alignment.Center,
    ) {
        val pressed = remember { MutableInteractionSource() }
        Row(
            Modifier
                .clip(Radii.Sm)
                .background(Color.Black.copy(alpha = 0.55f))
                .pressable(pressed, enabled = !wanted || failed) { if (failed) tries++ else wanted = true }
                .padding(horizontal = 14.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            when {
                failed -> Txt(t("bugs.card.videoFailed"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = Color.White)
                wanted -> {
                    Spinner(Color.White, 20.dp)
                    Txt(t("chat.attachments.loadingNamed", "name" to name, "size" to Time.bytes(f.size)), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = Color.White, maxLines = 1)
                }
                else -> {
                    Icon("Play", Color.White, size = 20.dp)
                    Txt(t("chat.attachments.loadVideo", "size" to Time.bytes(f.size)), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = Color.White, maxLines = 1)
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// the box

/** A file picked for the next line, on its way up or there. */
private class Staged(val uri: Uri, val name: String, val type: String, val size: Long) {
    var progress by mutableStateOf(0f)
    var done by mutableStateOf<NexaFile?>(null)
    var failed by mutableStateOf<String?>(null)
    var job: kotlinx.coroutines.Job? = null
}

/**
 * The conversation's own message box: the plus for files, the words, and
 * Send, with formatting and emoji behind the arrow as on a phone's chat box.
 * What is picked shows in the tray over the box while it goes up.
 */
@Composable
private fun ThreadComposer(placeholder: String, uploadPath: String?, onSend: suspend (String, List<NexaFile>) -> Unit) {
    val c = Nx.colors
    val scope = rememberCoroutineScope()
    var value by remember { mutableStateOf(TextFieldValue("")) }
    var busy by remember { mutableStateOf(false) }
    val staged = remember { mutableStateListOf<Staged>() }
    val text = value.text
    val length = text.codePointCount(0, text.length)
    val over = length > MAX_THREAD_MESSAGE
    val uploading = staged.any { it.done == null && it.failed == null }
    val ready = staged.mapNotNull { it.done }
    val canSend = (text.isNotBlank() || ready.isNotEmpty()) && !over && !busy && !uploading
    // what is still on its way up when the thread closes stops
    DisposableEffect(Unit) { onDispose { staged.forEach { it.job?.cancel() } } }

    fun add(uris: List<Uri>) {
        val path = uploadPath ?: return
        val resolver = Nexa.app.contentResolver
        var room = MAX_THREAD_FILES - staged.size
        if (room <= 0) {
            Nexa.ui.toast(t("nexa.thread.tooManyFiles", "count" to MAX_THREAD_FILES), "error")
            return
        }
        var wrongType = false
        var tooBig = false
        for (uri in uris) {
            val type = (resolver.getType(uri) ?: "").lowercase()
            if (type !in FILE_TYPES) {
                wrongType = true
                continue
            }
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
            if (size > (if (type.startsWith("video/")) VIDEO_MAX else IMAGE_MAX)) {
                tooBig = true
                continue
            }
            if (room <= 0) {
                Nexa.ui.toast(t("nexa.thread.tooManyFiles", "count" to MAX_THREAD_FILES), "error")
                break
            }
            room--
            val s = Staged(uri, name.ifEmpty { t("bugs.file.screenshot") }, type, size)
            staged += s
            s.job = scope.launch {
                try {
                    val q = java.net.URLEncoder.encode(s.name.take(120), "UTF-8")
                    val body = fish.nexa.net.UriBody(uri, type, s.size) { f -> s.progress = f }
                    val req = Nexa.api.requestBuilder("$path?name=$q").post(body).header("Content-Type", type).build()
                    val res = Nexa.api.execute(Nexa.api.http, req, true)
                    s.done = fish.nexa.net.wireJson.decodeFromJsonElement(NexaFile.serializer(), res)
                    s.progress = 1f
                } catch (e: Exception) {
                    if (e is kotlinx.coroutines.CancellationException) throw e
                    s.failed = (e as? ApiError)?.message?.takeIf { it.isNotBlank() } ?: t("nexa.thread.uploadFailed")
                }
            }
        }
        if (wrongType) Nexa.ui.toast(t("nexa.thread.fileType"), "error")
        if (tooBig) Nexa.ui.toast(t("nexa.chat.fileTooBig"), "error")
    }
    val pick = androidx.activity.compose.rememberLauncherForActivityResult(androidx.activity.result.contract.ActivityResultContracts.PickMultipleVisualMedia(MAX_THREAD_FILES)) { uris -> if (uris.isNotEmpty()) add(uris) }

    fun send(words: String = value.text) {
        val body = words.trim()
        if ((body.isEmpty() && ready.isEmpty()) || body.codePointCount(0, body.length) > MAX_THREAD_MESSAGE || busy || uploading) return
        busy = true
        val files = ready
        scope.launch {
            try {
                onSend(body, files)
                value = TextFieldValue("")
                staged.clear()
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast((e as? ApiError)?.message?.takeIf { it.isNotBlank() } ?: t("nexa.thread.couldNotSend"), "error")
            } finally {
                busy = false
            }
        }
    }

    fun insert(what: String) {
        val sel = value.selection
        val next = value.text.substring(0, sel.min) + what + value.text.substring(sel.max)
        value = TextFieldValue(next, TextRange(sel.min + what.length))
    }

    fun tools() {
        openMenu(
            listOf(
                MenuItem.Action(t("composer.more.formatting")) {
                    fish.nexa.ui.format.openFormattingEditor(
                        value.text, placeholder,
                        onInsert = { md -> value = TextFieldValue(md, TextRange(md.length)) },
                        onSend = { md ->
                            if (md.trim().codePointCount(0, md.trim().length) > MAX_THREAD_MESSAGE) false
                            else {
                                send(md)
                                true
                            }
                        },
                        max = MAX_THREAD_MESSAGE,
                    )
                },
                MenuItem.Action(t("composer.more.emoji")) {
                    fish.nexa.ui.pickers.Pickers.open(fish.nexa.ui.pickers.Pickers.Opener(
                        listOf(fish.nexa.ui.pickers.Pickers.Tab.EMOJI), null, thisServerOnly = true,
                        onEmoji = { p -> insert(p.text + " ") },
                    ))
                },
            ),
            title = t("composer.more.label"),
            haptic = false,
        )
    }

    val shape = Radii.Md
    Column(Modifier.fillMaxWidth().navigationBarsPadding().padding(start = 8.dp, end = 8.dp, bottom = 8.dp)) {
        if (staged.isNotEmpty()) Tray(staged, RoundedCornerShape(topStart = 14.dp, topEnd = 14.dp)) { s ->
            s.job?.cancel()
            staged.remove(s)
        }
        val boxShape = if (staged.isNotEmpty()) RoundedCornerShape(bottomStart = 14.dp, bottomEnd = 14.dp) else shape
        val interaction = remember { MutableInteractionSource() }
        val focused by interaction.collectIsFocusedAsState()
        val edge = if (focused) mixSrgb(c.borderSubtle, c.focusRing, 0.45f) else c.borderSubtle
        val maxH = (LocalConfiguration.current.screenHeightDp * 0.4f).dp
        Row(
            Modifier
                .fillMaxWidth()
                .shadowMedium(boxShape, c)
                .then(if (focused) Modifier.ring(boxShape, c.focusRing.copy(alpha = 0.10f), 4.dp) else Modifier)
                .ring(boxShape, edge)
                .background(c.bgComposer, boxShape)
                .topEdge(boxShape, c.edgeLight)
                .then(if (staged.isNotEmpty()) Modifier.topEdge(boxShape, c.borderSubtle) else Modifier)
                .heightIn(min = 48.dp),
            verticalAlignment = Alignment.Bottom,
        ) {
            if (uploadPath != null) Box(
                Modifier
                    .size(48.dp)
                    .alpha(if (busy || staged.size >= MAX_THREAD_FILES) 0.5f else 1f)
                    .pressable(remember { MutableInteractionSource() }, enabled = !busy && staged.size < MAX_THREAD_FILES) {
                        pick.launch(androidx.activity.result.PickVisualMediaRequest(androidx.activity.result.contract.ActivityResultContracts.PickVisualMedia.ImageAndVideo))
                    }
                    .semantics { contentDescription = t("composer.composer.uploadAFile") },
                contentAlignment = Alignment.Center,
            ) { Icon("CirclePlus", c.icon, size = 24.dp) } else Spacer(Modifier.width(16.dp))
            Box(Modifier.weight(1f).padding(top = 13.dp, bottom = 13.dp, end = 8.dp)) {
                BasicTextField(
                    value,
                    { value = it },
                    modifier = Modifier.fillMaxWidth().heightIn(max = maxH).semantics { contentDescription = placeholder },
                    textStyle = Nx.type.body.copy(color = c.text),
                    cursorBrush = SolidColor(c.text),
                    interactionSource = interaction,
                    keyboardOptions = messageKeyboard(),
                    decorationBox = { inner ->
                        Box {
                            if (value.text.isEmpty()) Txt(placeholder, style = Nx.type.body, color = c.textPlaceholder, maxLines = 1)
                            inner()
                        }
                    },
                )
            }
            Row(Modifier.height(48.dp).padding(end = 2.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                BoxButton("ChevronUp", t("composer.more.label"), onClick = ::tools)
                SendButton(canSend) { send() }
            }
        }
        if (length > MAX_THREAD_MESSAGE - 200) Txt(
            (MAX_THREAD_MESSAGE - length).toString(),
            style = Nx.type.hint.copy(fontWeight = if (over) FontWeight(600) else FontWeight(400)),
            color = if (over) c.textDanger else c.textMuted,
            align = androidx.compose.ui.text.style.TextAlign.End,
            modifier = Modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, top = 4.dp),
        )
    }
}

@Composable
private fun BoxButton(icon: String, label: String, onClick: () -> Unit) {
    val c = Nx.colors
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier
            .size(44.dp)
            .graphicsLayer { val s = if (pressed) 0.92f else 1f; scaleX = s; scaleY = s }
            .clip(Radii.Sm)
            .pressable(interaction, onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
    ) { Icon(icon, c.icon, size = 24.dp) }
}

@Composable
private fun SendButton(enabled: Boolean, onClick: () -> Unit) {
    val c = Nx.colors
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier
            .size(44.dp)
            .graphicsLayer { val s = if (pressed && enabled) 0.92f else 1f; scaleX = s; scaleY = s }
            .then(
                if (enabled) Modifier
                    .boxShadow(CircleShape, 4f, 12f, c.brand.copy(alpha = 0.8f), spread = -4f)
                    .background(brandGradient(c), CircleShape)
                    .topEdge(CircleShape, Color.White.copy(alpha = 0.22f))
                else Modifier,
            )
            .pressable(interaction, enabled = enabled, onClick = onClick)
            .semantics { contentDescription = t("composer.composer.sendMessage") },
        contentAlignment = Alignment.Center,
    ) { Icon("SendHorizontal", if (enabled) c.onBrand else c.textMuted, size = 22.dp) }
}

/** The files waiting to go: a count and total, then a card for each, with its progress and a way to take it out. */
@Composable
private fun Tray(files: List<Staged>, shape: androidx.compose.ui.graphics.Shape, onRemove: (Staged) -> Unit) {
    val c = Nx.colors
    Column(Modifier.fillMaxWidth().ring(shape, c.borderSubtle).background(c.bgComposer, shape).topEdge(shape, c.edgeLight)) {
        Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 10.dp), horizontalArrangement = Arrangement.SpaceBetween) {
            Txt(t("composer.composer.trayCount", "count" to files.size, "max" to MAX_THREAD_FILES), style = Nx.type.hint.copy(fontWeight = FontWeight(600)), color = c.textMuted)
            Txt(Time.bytes(files.sumOf { it.size }), style = Nx.type.hint.copy(fontWeight = FontWeight(500), fontFeatureSettings = "tnum"), color = c.textMuted)
        }
        Row(Modifier.fillMaxWidth().sidewaysScroll().padding(16.dp, 12.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            for (f in files) TrayCard(f) { onRemove(f) }
        }
    }
}

@Composable
private fun TrayCard(f: Staged, onRemove: () -> Unit) {
    val c = Nx.colors
    val shape = Radii.Sm
    val failed = f.failed
    Box(Modifier.width(150.dp)) {
        Column(
            Modifier
                .fillMaxWidth()
                .shadowLow(shape, c)
                .ring(shape, if (failed != null) c.danger else c.borderSubtle)
                .background(c.bgSurface, shape)
                .clip(shape)
                .padding(8.dp),
        ) {
            Box(Modifier.fillMaxWidth().height(100.dp).clip(Radii.Sm).background(c.bgInput), contentAlignment = Alignment.Center) {
                if (f.type.startsWith("image/")) AsyncImage(f.uri, f.name, Modifier.fillMaxSize().alpha(if (failed != null) 0.45f else 1f), contentScale = ContentScale.Fit)
                else Icon("Film", c.textMuted, size = 26.dp)
            }
            Txt(f.name, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text, maxLines = 1, modifier = Modifier.padding(top = 6.dp))
            Txt(
                when {
                    failed != null -> failed
                    f.done != null -> Time.bytes(f.size)
                    else -> t("nexa.chat.uploading", "percent" to Math.round(f.progress * 100))
                },
                style = Nx.type.hint, color = if (failed != null) c.textDanger else c.textMuted, maxLines = 2,
            )
            if (f.done == null && failed == null) Box(Modifier.padding(top = 6.dp).fillMaxWidth().height(3.dp).background(c.bgActive, RoundedCornerShape(2.dp))) {
                Box(Modifier.fillMaxWidth(f.progress.coerceIn(0.02f, 1f)).height(3.dp).background(c.brand, RoundedCornerShape(2.dp)))
            }
        }
        Box(
            Modifier
                .align(Alignment.TopEnd)
                .offset(x = 8.dp, y = (-8).dp)
                .shadowMedium(Radii.Sm, c)
                .background(c.bgFloating, Radii.Sm),
        ) {
            val interaction = remember { MutableInteractionSource() }
            val pressed by interaction.collectIsPressedAsState()
            Box(
                Modifier.size(40.dp).pressable(interaction, onClick = onRemove).semantics { contentDescription = t("nexa.thread.removeFile", "name" to f.name) },
                contentAlignment = Alignment.Center,
            ) { Icon("Trash2", if (pressed) c.danger else c.icon, size = 16.dp) }
        }
    }
}

// ---------------------------------------------------------------------------
// a report as the first message of its conversation (ThreadParts.tsx)

/**
 * A bug report as the first message of its conversation, from whoever sent
 * it: its subject, what they wrote, their screenshots and recording as a
 * message carries them, and the facts about their device folded away.
 */
@Composable
fun ReportOpener(subject: String, description: String, attachments: List<NexaFile>, context: kotlinx.serialization.json.JsonObject, from: String?, facts: Boolean = true) {
    val c = Nx.colors
    Txt(subject, style = Nx.type.body.copy(fontSize = Nx.type.sp(16f), fontWeight = FontWeight(650), lineHeight = Nx.type.sp(22f)), color = c.textStrong, maxLines = 3)
    MarkdownView(description, RenderCtx(surface = c.bgCanvas))
    if (attachments.isNotEmpty()) ThreadFiles(attachments, "bugs", from)
    if (facts) BugFacts(context)
}

/** A coloured mark for the panel's lists: something new to this member of staff. */
@Composable
fun UnreadDot(modifier: Modifier = Modifier) {
    val c = Nx.colors
    Box(
        modifier
            .size(8.dp)
            .ring(CircleShape, c.textStrong.copy(alpha = 0.12f), 3.dp)
            .background(c.textStrong, CircleShape)
            .semantics { contentDescription = t("sidebar.serverSidebar.unread") },
    )
}
