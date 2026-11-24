package fish.nexa.ui.gems

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.keyframes
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.key
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.ProgressBarRangeInfo
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.progressBarRangeInfo
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import fish.nexa.Nexa
import fish.nexa.data.Quests
import fish.nexa.data.ServerClock
import fish.nexa.data.StaffCan
import fish.nexa.data.StaffRanks
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.SelectField
import fish.nexa.ui.components.Spinner
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.settings.PageTitle
import fish.nexa.ui.settings.settingAnchor
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.look.Colour
import fish.nexa.ui.theme.ring
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/*
 * Settings, Quests (web settings/QuestsPage.tsx and QuestsPage.module.css):
 * the day's three quests in one list closed by their bonus, then the Quest
 * Tower as a ladder seen through a window, the next step ghosted above and the
 * steps cleared below. The server counts and pays; the page only shows it.
 * The owner also gets Debug Quests, which works their own quests by hand.
 */

/** How long a finished tower step shows full before the ladder moves (web CLIMB_HOLD_MS). */
private const val CLIMB_HOLD_MS = 900L

/** The width of the Trial's step column, which its rail runs down the middle of (web --step-col). */
private val STEP_COL = 64.dp
private val POP_EASE = CubicBezierEasing(0.3f, 1.6f, 0.5f, 1f)

/** "5 h 12 min" until the daily quests change. */
private fun untilReset(at: Long, now: Long): String {
    val mins = maxOf(1L, (at - now + 59_999) / 60_000)
    val h = mins / 60
    val m = mins % 60
    return if (h > 0) t("quests.time.hm", "h" to gemNumber(h), "m" to gemNumber(m)) else t("quests.time.m", "m" to gemNumber(m))
}

/** `color-mix(in oklch, a, b p)`: p of the way from a to b, the hue the short way round (a grey takes the other's hue). */
private fun mixOklch(a: Color, b: Color, p: Double): Color {
    fun ok(x: Color) = Colour.rgbToOklch(Colour.Rgb(x.red * 255.0, x.green * 255.0, x.blue * 255.0))
    val x = ok(a)
    val y = ok(b)
    val hx = if (x.c < 1e-4) y.h else x.h
    val hy = if (y.c < 1e-4) hx else y.h
    var d = hy - hx
    if (d > 180) d -= 360 else if (d < -180) d += 360
    val hex = Colour.oklchToHex(Colour.Oklch(x.l + (y.l - x.l) * p, x.c + (y.c - x.c) * p, ((hx + d * p) % 360 + 360) % 360))
    return Color(android.graphics.Color.parseColor(hex))
}

/**
 * Finished is a bright, clear green made from the theme's own positive colour (which themes keep dark enough for
 * white text), with ink dark enough to read on it: web `--quest-done` and `--quest-done-ink`.
 */
private data class DoneColours(val fill: Color, val ink: Color)

@Composable
private fun doneColours(): DoneColours {
    val positive = Nx.colors.positive
    return remember(positive) { DoneColours(mixOklch(positive, Color(0xFF2EE87A), 0.75), mixOklch(positive, Color.Black, 0.72)) }
}

/** What a quest asks, in words. */
fun questText(q: Quests.Quest): String = t("quests.kind.${q.kind}", "count" to q.target)

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun QuestsPage() {
    val c = Nx.colors
    val board by Quests.board.collectAsState()
    val error by Quests.error.collectAsState()
    val visible by Nexa.ui.visible.collectAsState()
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf<String?>(null) }
    var now by remember { mutableLongStateOf(ServerClock.now()) }
    // quests already finished when the page opened draw still; one that finishes while it is open pops its reward once
    val doneAtOpen = remember { mutableSetOf<String>() }
    val seeded = remember { booleanArrayOf(false) }
    board?.let { b ->
        if (!seeded[0]) {
            for (q in b.daily + listOfNotNull(b.bonus, b.trial)) if (q.done) doneAtOpen += q.id
            seeded[0] = true
        }
    }
    fun fresh(q: Quests.Quest) = q.done && q.id !in doneAtOpen

    LaunchedEffect(Unit) { Quests.load() }
    // the countdown, only while it can be seen; and a new board once the day turns
    LaunchedEffect(visible) {
        while (visible) {
            now = ServerClock.now()
            delay(30_000)
        }
    }
    val resetsAt = board?.resetsAt ?: 0L
    LaunchedEffect(now >= resetsAt && resetsAt > 0) { if (resetsAt in 1..now) Quests.load() }

    fun act(key: String, run: suspend () -> Quests.Board, then: () -> Unit = {}) {
        busy = key
        scope.launch {
            try {
                Quests.set(run())
                then()
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(describeError(e, t("quests.actionFailed")), "error")
            } finally {
                busy = null
            }
        }
    }

    val owner = StaffRanks.can(StaffCan.OWNER_TOOLS)
    var debugging by remember { mutableStateOf(false) }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Box(Modifier.weight(1f)) { PageTitle(t("quests.title")) }
        if (owner) {
            NxButton(
                t("quests.debug.toggle"), { debugging = !debugging },
                kind = if (debugging) ButtonKind.PRIMARY else ButtonKind.SECONDARY, size = ButtonSize.SM, icon = "Wrench",
                modifier = Modifier.padding(bottom = 22.dp).semantics { selected = debugging },
            )
        }
    }
    val debug = owner && debugging
    val b = board
    if (error != null && b == null) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Note(t("quests.failed"))
            NxButton(t("chat.messageList.tryAgain"), { scope.launch { Quests.load() } }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
        }
        return
    }
    if (b == null) {
        Box(Modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) { Spinner() }
        return
    }
    Note(t("quests.hint"))
    if (debug) Note(t("quests.debug.hint"), Modifier.padding(top = 6.dp), small = true)

    // the day's quests
    FlowRow(
        Modifier.fillMaxWidth().settingAnchor("questsDaily").padding(top = 28.dp, bottom = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
        itemVerticalAlignment = Alignment.Bottom,
    ) {
        Heading(t("quests.daily.title"), Modifier.weight(1f))
        Txt(
            t("quests.resetsIn", "time" to untilReset(b.resetsAt, now)),
            style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontFeatureSettings = "tnum"),
            color = c.textMuted, maxLines = 1,
            modifier = Modifier.padding(bottom = 2.dp),
        )
    }
    val doneCount = b.daily.count { it.done }
    Column(Modifier.fillMaxWidth().background(c.bgSurface, Radii.Md).ring(Radii.Md, c.borderSubtle)) {
        b.daily.forEachIndexed { i, q ->
            if (i > 0) Rule()
            Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Txt(t("quests.level.${q.level}"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontWeight = FontWeight(600)), color = c.textMuted, maxLines = 1, modifier = Modifier.weight(1f))
                    if (!q.done && b.rerollLeft) SwapButton(q, busy) { act(q.id, { Quests.reroll(q.id) }) }
                    Reward(q, fresh(q))
                }
                QuestWords(q, Modifier.fillMaxWidth())
                Progress(q)
                if (debug && !q.done) DebugStrip(q, busy, ::act)
            }
        }
        b.bonus?.let { bonus ->
            Rule()
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Row(
                    Modifier.semantics(mergeDescendants = true) { contentDescription = t("quests.progress", "progress" to gemNumber(doneCount.toLong()), "target" to gemNumber(b.daily.size.toLong())) },
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    val done = doneColours()
                    for (q in b.daily) Box(Modifier.size(10.dp).then(if (q.done) Modifier.background(done.fill, CircleShape) else Modifier.border(2.dp, c.textMuted, CircleShape)))
                }
                Txt(
                    t("quests.bonus", "count" to bonus.reward),
                    style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), lineHeight = Nx.type.sp(20.25f), fontWeight = FontWeight(500)),
                    color = if (bonus.done) c.textMuted else c.text,
                    modifier = Modifier.weight(1f),
                )
                Reward(bonus, fresh(bonus))
            }
        }
    }
    if (b.rerollLeft && b.daily.any { !it.done }) Note(t("quests.swapHint"), Modifier.padding(top = 10.dp))
    if (debug) {
        FlowRow(Modifier.fillMaxWidth().padding(top = 10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            NxButton(t("quests.debug.newDay"), { act("debug:day", { Quests.debug("new_day") }) }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = busy == "debug:day", enabled = busy == null)
            NxButton(t("quests.debug.resetUses"), { act("debug:uses", { Quests.debug("uses") }) }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = busy == "debug:uses", enabled = busy == null)
        }
    }

    // the Quest Tower
    val trial = b.trial ?: return
    Heading(t("quests.trial.title"), Modifier.settingAnchor("questsTrial").padding(top = 28.dp))
    Note(t("quests.trial.hint"), Modifier.padding(top = 4.dp, bottom = 14.dp))
    // the climb (web QuestsPage's): a step finished while the page is open fills and pops where it stands, then the
    // ladder moves down a rung, the finished step sinking toward the cleared rail as the next drops in from above.
    // Fixed when the step changes, so the finished step never shows its old state; a swap or a jump is instant.
    val last = remember { arrayOfNulls<Quests.Quest>(1) }
    val winHeight = remember { intArrayOf(0) }
    val reduced = Nx.motion.reduced
    val climbFrom = remember(trial.id) {
        last[0]?.takeIf { it.id != trial.id && trial.level == it.level + 1 && !reduced }?.let { it.copy(doneAt = maxOf(it.doneAt, 1L)) }
    }
    last[0] = trial
    val heldHeight = remember(trial.id) { winHeight[0] }
    var climbing by remember(trial.id) { mutableStateOf(climbFrom != null) }
    var sliding by remember(trial.id) { mutableStateOf(false) }
    val slide = remember(trial.id) { Animatable(0f) }
    LaunchedEffect(trial.id) {
        if (climbFrom == null) return@LaunchedEffect
        delay(CLIMB_HOLD_MS)
        sliding = true
        slide.animateTo(1f, tween(420, easing = CubicBezierEasing(0.5f, 0f, 0.2f, 1f)))
        climbing = false
    }
    val from = climbFrom?.takeIf { climbing }
    // the rungs name the step in the window until the ladder moves
    val at = if (from != null && !sliding) from else trial
    val cleared = if (from != null && !sliding) maxOf(0L, from.level - 1) else b.trialCleared
    var boxHeight by remember { mutableIntStateOf(0) }
    Column(Modifier.fillMaxWidth()) {
        Rung(t("quests.trial.step", "step" to gemNumber(at.level + 1)), next = true)
        Box(
            Modifier.fillMaxWidth()
                .then(if (from != null) Modifier.heightIn(min = with(LocalDensity.current) { heldHeight.toDp() }) else Modifier)
                .onSizeChanged { boxHeight = it.height; if (from == null) winHeight[0] = it.height }
                .background(c.bgSurface, Radii.Md)
                .ring(Radii.Md, c.textBrand.copy(alpha = 0.4f), 2.dp)
                .ring(Radii.Md, c.borderSubtle)
                .clip(Radii.Md),
        ) {
            // one call site, keyed by step, so the finished step keeps its bar and pill as they fill and pop
            for (q in listOfNotNull(from, trial.takeIf { from == null || sliding })) key(q.id) {
                val sinking = q === from
                TowerStep(
                    q, fresh(q),
                    Modifier.graphicsLayer {
                        translationY = when {
                            from == null -> 0f
                            sinking -> boxHeight * slide.value
                            else -> boxHeight * (slide.value - 1f)
                        }
                    }
                        // the finished step fills the held window, its divider running the whole way down
                        .then(if (sinking) Modifier.matchParentSize() else Modifier)
                        .then(if (sinking && sliding) Modifier.clearAndSetSemantics {} else Modifier),
                ) {
                    if (b.skipLeft && !q.done) {
                        NxButton(
                            t("quests.trial.skip"), { act("trial", { Quests.skipTrial() }) },
                            kind = ButtonKind.SECONDARY, size = ButtonSize.SM,
                            loading = busy == "trial", enabled = busy == null || busy == "trial",
                        )
                    }
                    if (debug && !q.done) DebugStrip(q, busy, ::act)
                }
            }
        }
        Rung(if (cleared > 0) t("quests.trial.cleared", "count" to cleared) else t("quests.trial.first"), next = false)
    }
    if (b.skipLeft && !at.done) Note(t("quests.trial.skipHint"), Modifier.padding(top = 10.dp))
    if (debug) {
        var step by remember { mutableStateOf("") }
        val goTo = step.toLongOrNull() ?: 0L
        fun go() {
            if (goTo >= 1) act("debug:step", { Quests.debug("step", step = goTo) }) { step = "" }
        }
        Row(Modifier.padding(top = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            NxTextField(
                step, { v -> step = v.filter(Char::isDigit).take(6) },
                modifier = Modifier.width(96.dp).semantics { contentDescription = t("quests.debug.step") },
                placeholder = gemNumber(trial.level), keyboardType = KeyboardType.Number, imeAction = ImeAction.Go, onIme = ::go,
            )
            NxButton(t("quests.debug.goToStep"), ::go, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = busy == "debug:step", enabled = busy == null && goTo >= 1)
        }
    }
}

/** One step of the Quest Tower in its window: the step's number in the rail's column, then the quest. */
@Composable
private fun TowerStep(q: Quests.Quest, fresh: Boolean, modifier: Modifier, controls: @Composable ColumnScope.() -> Unit) {
    val c = Nx.colors
    Row(modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
        Box(
            Modifier.width(STEP_COL).fillMaxHeight()
                .drawBehind { drawLine(c.borderSubtle, Offset(size.width - density / 2, 0f), Offset(size.width - density / 2, size.height), density) }
                .semantics { contentDescription = t("quests.trial.step", "step" to gemNumber(q.level)) },
            contentAlignment = Alignment.Center,
        ) {
            Txt(
                gemNumber(q.level),
                style = Nx.type.title.copy(fontSize = Nx.type.sp(if (q.level >= 1000) 22f else 30f), lineHeight = Nx.type.sp(32f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum"),
                color = c.textBrand, maxLines = 1, align = TextAlign.Center,
            )
        }
        Column(Modifier.weight(1f).padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                QuestWords(q, Modifier.weight(1f))
                Reward(q, fresh)
            }
            Progress(q)
            controls()
        }
    }
}

/** The owner's tools under one open quest: add to it, finish it, or make it another quest (web `DebugStrip`). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun DebugStrip(q: Quests.Quest, busy: String?, act: (String, suspend () -> Quests.Board, () -> Unit) -> Unit) {
    val c = Nx.colors
    var changing by remember(q.id, q.kind, q.target) { mutableStateOf(false) }
    var kind by remember(q.id, q.kind) { mutableStateOf(q.kind) }
    var target by remember(q.id, q.target) { mutableStateOf(q.target.toString()) }
    fun key(k: String) = "debug:${q.id}:$k"
    val off = busy != null
    val dashed = Modifier.fillMaxWidth().padding(top = 4.dp).drawBehind {
        drawLine(c.borderSubtle, Offset(0f, 0f), Offset(size.width, 0f), density, pathEffect = PathEffect.dashPathEffect(floatArrayOf(3 * density, 3 * density)))
    }.padding(top = 10.dp)
    if (changing) {
        val n = maxOf(1L, target.toLongOrNull() ?: 0L)
        Column(dashed, verticalArrangement = Arrangement.spacedBy(8.dp)) {
            SelectField(null, Quests.KINDS.map { it to t("quests.kind.$it", "count" to n) }, kind, Modifier.fillMaxWidth()) { kind = it }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                NxTextField(
                    target, { v -> target = v.filter(Char::isDigit).take(9) },
                    modifier = Modifier.width(96.dp).semantics { contentDescription = t("quests.debug.target") },
                    keyboardType = KeyboardType.Number, imeAction = ImeAction.Done,
                )
                Box(Modifier.weight(1f))
                NxButton(t("quests.debug.cancel"), { changing = false }, kind = ButtonKind.GHOST, size = ButtonSize.SM)
                NxButton(
                    t("quests.debug.save"), { act(key("set"), { Quests.debug("set", questId = q.id, kind = kind, target = n) }) { changing = false } },
                    size = ButtonSize.SM, loading = busy == key("set"), enabled = !off,
                )
            }
        }
        return
    }
    FlowRow(dashed, horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp), itemVerticalAlignment = Alignment.CenterVertically) {
        Icon("Wrench", c.textMuted, size = 14.dp, modifier = Modifier.padding(end = 2.dp))
        for (a in listOf(1L, 10L)) {
            NxButton(
                "+" + gemNumber(a), { act(key("$a"), { Quests.debug("progress", questId = q.id, amount = a) }) {} },
                kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = busy == key("$a"), enabled = !off,
                modifier = Modifier.widthIn(min = 44.dp).semantics { contentDescription = t("quests.debug.add", "n" to gemNumber(a)) },
            )
        }
        NxButton(
            t("quests.debug.finish"), { act(key("finish"), { Quests.debug("progress", questId = q.id, amount = maxOf(1L, q.target - q.progress)) }) {} },
            kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = busy == key("finish"), enabled = !off,
        )
        NxButton(t("quests.debug.change"), { kind = q.kind; target = q.target.toString(); changing = true }, kind = ButtonKind.GHOST, size = ButtonSize.SM, enabled = !off)
    }
}

@Composable
private fun Note(text: String, modifier: Modifier = Modifier, small: Boolean = false) {
    Txt(text, style = Nx.type.body.copy(fontSize = Nx.type.sp(if (small) 13f else 14f), lineHeight = Nx.type.sp(if (small) 18.85f else 20.3f)), color = Nx.colors.textMuted, modifier = modifier)
}

/** `.sectionTitle`: 18 sp in the display face. */
@Composable
private fun Heading(text: String, modifier: Modifier = Modifier) {
    Txt(
        text,
        style = Nx.type.title.copy(fontSize = Nx.type.sp(18f), lineHeight = Nx.type.sp(23.4f), fontWeight = FontWeight(700), letterSpacing = Nx.type.body.letterSpacing),
        color = Nx.colors.textStrong,
        modifier = modifier.semantics { heading() },
    )
}

@Composable
private fun Rule() = Box(Modifier.fillMaxWidth().height(1.dp).background(Nx.colors.borderSubtle))

@Composable
private fun QuestWords(q: Quests.Quest, modifier: Modifier) {
    Txt(
        questText(q),
        style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), lineHeight = Nx.type.sp(20.25f), fontWeight = FontWeight(600)),
        color = if (q.done) Nx.colors.textMuted else Nx.colors.textStrong,
        modifier = modifier,
    )
}

/** What a quest pays, beside the Gem; a check on the positive colour once paid. A quest finished while the page is open pops once. */
@Composable
private fun Reward(q: Quests.Quest, fresh: Boolean) {
    val c = Nx.colors
    val reduced = Nx.motion.reduced
    val pop = remember(fresh) { Animatable(if (fresh && !reduced) 0.6f else 1f) }
    LaunchedEffect(fresh) {
        if (fresh && !reduced) pop.animateTo(1f, keyframes { durationMillis = 420; 1.15f at 252 using POP_EASE })
    }
    val shape = RoundedCornerShape(999.dp)
    val doneC = doneColours()
    Row(
        Modifier
            .graphicsLayer { scaleX = pop.value; scaleY = pop.value }
            .then(if (q.done) Modifier.background(doneC.fill, shape).padding(start = 6.dp, end = 8.dp, top = 3.dp, bottom = 3.dp) else Modifier)
            .semantics(mergeDescendants = true) { contentDescription = (if (q.done) t("quests.done") + " " else "") + t("gems.count", "count" to q.reward) },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp),
    ) {
        if (q.done) Icon("Check", doneC.ink, size = 14.dp, stroke = 3f) else GemIcon(16.dp)
        Txt(
            gemNumber(q.reward),
            style = Nx.type.title.copy(fontSize = Nx.type.sp(15f), lineHeight = Nx.type.sp(15f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum", letterSpacing = Nx.type.body.letterSpacing),
            color = if (q.done) doneC.ink else c.textStrong, maxLines = 1,
        )
    }
}

@Composable
private fun RowScope.SwapButton(q: Quests.Quest, busy: String?, onClick: () -> Unit) {
    val c = Nx.colors
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier.size(28.dp).clip(Radii.Sm)
            .background(if (pressed) c.bgActive else Color.Transparent)
            .pressable(interaction, enabled = busy == null) { onClick() }
            .semantics { contentDescription = "${t("quests.swap")}: ${questText(q)}" }
            .graphicsLayer { alpha = if (busy != null && busy != q.id) 0.5f else 1f },
        contentAlignment = Alignment.Center,
    ) {
        if (busy == q.id) Spinner(size = 14.dp) else Icon("RefreshCw", if (pressed) c.textStrong else c.icon, size = 16.dp)
    }
}

@Composable
private fun Progress(q: Quests.Quest) {
    val c = Nx.colors
    val shown = if (q.done) q.target else minOf(q.progress, q.target)
    val frac = if (q.target > 0) shown.toFloat() / q.target else 0f
    val width by animateFloatAsState(frac, if (Nx.motion.reduced) tween(0) else tween(500, easing = CubicBezierEasing(0.2f, 0.8f, 0.2f, 1f)), label = "quest")
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Box(
            Modifier.weight(1f).height(6.dp).clip(CircleShape).background(c.bgActive)
                .semantics {
                    contentDescription = questText(q)
                    progressBarRangeInfo = ProgressBarRangeInfo(shown.toFloat(), 0f..maxOf(1f, q.target.toFloat()))
                },
        ) {
            Box(Modifier.fillMaxWidth(width).fillMaxHeight().clip(CircleShape).background(if (q.done) doneColours().fill else c.link))
        }
        Txt(
            if (q.done) t("quests.done") else t("quests.progress", "progress" to gemNumber(shown), "target" to gemNumber(q.target)),
            style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontFeatureSettings = "tnum"),
            color = c.textMuted, maxLines = 1, align = TextAlign.End,
            modifier = Modifier.widthIn(min = 64.dp),
        )
    }
}

/** A step of the ladder outside the window: the rail through the step column, dashed toward what is to come. */
@Composable
private fun Rung(text: String, next: Boolean) {
    val c = Nx.colors
    Row(
        Modifier.fillMaxWidth().heightIn(min = 34.dp).height(IntrinsicSize.Min).graphicsLayer { alpha = if (next) 0.7f else 1f },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier.width(STEP_COL).fillMaxHeight().drawBehind {
                val x = size.width / 2
                drawLine(
                    c.textBrand, Offset(x, 0f), Offset(x, size.height), 2 * density,
                    pathEffect = if (next) PathEffect.dashPathEffect(floatArrayOf(4 * density, 4 * density)) else null,
                )
            },
        )
        Txt(text, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontFeatureSettings = "tnum"), color = c.textMuted, modifier = Modifier.padding(vertical = 8.dp))
    }
}
