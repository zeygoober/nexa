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
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Quests
import fish.nexa.data.ServerClock
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Spinner
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.settings.PageTitle
import fish.nexa.ui.settings.settingAnchor
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/*
 * Settings, Quests (web settings/QuestsPage.tsx and QuestsPage.module.css):
 * the day's three quests in one list closed by their bonus, then the Infinite
 * Trial as a ladder seen through a window, the next step ghosted above and the
 * steps cleared below. The server counts and pays; the page only shows it.
 */

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

    fun act(key: String, run: suspend () -> Quests.Board) {
        busy = key
        scope.launch {
            try {
                Quests.set(run())
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(describeError(e, t("quests.actionFailed")), "error")
            } finally {
                busy = null
            }
        }
    }

    PageTitle(t("quests.title"))
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
                    if (!q.done && b.rerollLeft) SwapButton(q, busy) { act(q.id) { Quests.reroll(q.id) } }
                    Reward(q, fresh(q))
                }
                QuestWords(q, Modifier.fillMaxWidth())
                Progress(q)
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
                    for (q in b.daily) Box(Modifier.size(10.dp).then(if (q.done) Modifier.background(c.positive, CircleShape) else Modifier.border(2.dp, c.textMuted, CircleShape)))
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

    // the Infinite Trial
    val trial = b.trial ?: return
    Heading(t("quests.trial.title"), Modifier.settingAnchor("questsTrial").padding(top = 28.dp))
    Note(t("quests.trial.hint"), Modifier.padding(top = 4.dp, bottom = 14.dp))
    Column(Modifier.fillMaxWidth()) {
        Rung(t("quests.trial.step", "step" to gemNumber(trial.level + 1)), next = true)
        Row(
            Modifier.fillMaxWidth().height(IntrinsicSize.Min)
                .background(c.bgSurface, Radii.Md)
                .ring(Radii.Md, c.textBrand.copy(alpha = 0.4f), 2.dp)
                .ring(Radii.Md, c.borderSubtle),
        ) {
            Box(
                Modifier.width(STEP_COL).fillMaxHeight()
                    .drawBehind { drawLine(c.borderSubtle, Offset(size.width - density / 2, 0f), Offset(size.width - density / 2, size.height), density) }
                    .semantics { contentDescription = t("quests.trial.step", "step" to gemNumber(trial.level)) },
                contentAlignment = Alignment.Center,
            ) {
                Txt(
                    gemNumber(trial.level),
                    style = Nx.type.title.copy(fontSize = Nx.type.sp(if (trial.level >= 1000) 22f else 30f), lineHeight = Nx.type.sp(32f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum"),
                    color = c.textBrand, maxLines = 1, align = TextAlign.Center,
                )
            }
            Column(Modifier.weight(1f).padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    QuestWords(trial, Modifier.weight(1f))
                    Reward(trial, fresh(trial))
                }
                Progress(trial)
                if (b.skipLeft && !trial.done) {
                    NxButton(
                        t("quests.trial.skip"), { act("trial") { Quests.skipTrial() } },
                        kind = ButtonKind.SECONDARY, size = ButtonSize.SM,
                        loading = busy == "trial", enabled = busy == null || busy == "trial",
                    )
                }
            }
        }
        Rung(if (b.trialCleared > 0) t("quests.trial.cleared", "count" to b.trialCleared) else t("quests.trial.first"), next = false)
    }
    if (b.skipLeft && !trial.done) Note(t("quests.trial.skipHint"), Modifier.padding(top = 10.dp))
}

@Composable
private fun Note(text: String, modifier: Modifier = Modifier) {
    Txt(text, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(20.3f)), color = Nx.colors.textMuted, modifier = modifier)
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
    Row(
        Modifier
            .graphicsLayer { scaleX = pop.value; scaleY = pop.value }
            .then(if (q.done) Modifier.background(c.positive, shape).padding(start = 6.dp, end = 8.dp, top = 3.dp, bottom = 3.dp) else Modifier)
            .semantics(mergeDescendants = true) { contentDescription = (if (q.done) t("quests.done") + " " else "") + t("gems.count", "count" to q.reward) },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp),
    ) {
        if (q.done) Icon("Check", c.onPositive, size = 14.dp, stroke = 3f) else GemIcon(16.dp)
        Txt(
            gemNumber(q.reward),
            style = Nx.type.title.copy(fontSize = Nx.type.sp(15f), lineHeight = Nx.type.sp(15f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum", letterSpacing = Nx.type.body.letterSpacing),
            color = if (q.done) c.onPositive else c.textStrong, maxLines = 1,
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
            Box(Modifier.fillMaxWidth(width).fillMaxHeight().clip(CircleShape).background(if (q.done) c.positive else c.link))
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
