package fish.nexa.ui.gems

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.asComposeRenderEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Gems
import fish.nexa.data.Trades
import fish.nexa.media.GemSounds
import fish.nexa.media.rememberGemSounds
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.layers.openSheet
import fish.nexa.ui.profile.BadgePill
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import kotlinx.coroutines.launch

/** The roll, as on the web (FuseMachine.tsx rollPlan, angleAt): pulled back, let go, slowing on a cubic over 3 or 4 laps, then a still beat. */
private const val WIND_MS = 450
private const val HOLD_MS = 450L
private const val WIND_DEG = 12f
/** How long a teeter takes to move from where it hung to where it rests. */
private const val SETTLE_MS = 800f
/** The least distance between where the marker rests and a line, so its disc never sits on one. */
private const val CLEAR = 7f
/** The arc's round caps reach this far past its ends (half the 12 wide stroke on the ring of 84). */
private val CAP_DEG = Math.toDegrees(6.0 / 84.0).toFloat()
private val SPRING_OUT = CubicBezierEasing(0.34f, 1.56f, 0.64f, 1f)

private enum class FusePhase { IDLE, WAITING, ROLLING, WON, LOST }

/**
 * A roll worked out before it starts, the same as the web's RollPlan: angles in
 * degrees clockwise from the top counting laps. The spin slows to [stop]; a
 * teeter hangs there [hang] ms leaning, then moves to [end] across [line].
 */
private class RollPlan(val stop: Float, val end: Float, val spin: Float, val hang: Float, val line: Float?) {
    val total: Float get() = WIND_MS + spin + if (hang > 0f) hang + SETTLE_MS else 0f
}

private fun between(a: Float, b: Float) = a + Math.random().toFloat() * (b - a)

/** Plans where a roll goes for an answer the server already gave: clear of both lines, close to one, or a teeter over one. */
private fun rollPlan(chance: Float, won: Boolean): RollPlan {
    val laps = (if (Math.random() < 0.5) 3 else 4) * 360f
    val spin = between(4400f, 5600f)
    fun plain(rest: Float, line: Float?) = RollPlan(rest, rest, spin, 0f, line)
    if (chance >= 1f) return plain(laps + between(0f, 360f), null)
    val arc = chance * 360f
    // the side's two lines, in order round the dial: a win lies between the top and the far edge, a loss after it
    val (from, to) = if (won) 0f to arc else arc to 360f
    val kind = Math.random()
    if (kind < 0.35) return plain(laps + between(from + 2 * CLEAR, to - 2 * CLEAR), null)
    // close to one line or the other: `after` when the right side lies past it, clockwise
    val after = Math.random() < 0.5
    val line = laps + if (after) from else to
    val side = if (after) 1f else -1f
    val rest = line + side * between(CLEAR, CLEAR + 6f)
    if (kind < 0.75) return plain(rest, line)
    return RollPlan(line - side * between(6f, 10f), rest, spin, between(700f, 1300f), line)
}

private fun easeInOut(x: Float) = if (x < 0.5f) 2 * x * x else 1 - (-2 * x + 2) * (-2 * x + 2) / 2

/** The marker's angle [ms] into a roll (FuseMachine.tsx angleAt). */
private fun angleAt(ms: Float, p: RollPlan): Float {
    if (ms < WIND_MS) return -WIND_DEG * easeInOut(ms / WIND_MS)
    var t = ms - WIND_MS
    if (t < p.spin) { val v = 1 - t / p.spin; return -WIND_DEG + (p.stop + WIND_DEG) * (1 - v * v * v) }
    t -= p.spin
    if (p.hang <= 0f) return p.end
    val move = p.end - p.stop
    // it leans toward where it will go and back while it hangs
    if (t < p.hang) return p.stop + move * 0.18f * kotlin.math.sin(Math.PI.toFloat() * t / p.hang)
    t -= p.hang
    if (t < SETTLE_MS) return p.stop + move * easeInOut(t / SETTLE_MS)
    return p.end
}

/** What the sheet's body and footer share. */
private class FuseControl(val line: Trades.Line, val name: String) {
    var phase by mutableStateOf(FusePhase.IDLE)
    var copies by mutableIntStateOf(1)
    var plan by mutableStateOf<RollPlan?>(null)
    /** The roll's answer, kept for the sweep's end or a press that skips to it. */
    var pending: Pair<Boolean, Gems.View>? = null
    val busy get() = phase == FusePhase.WAITING || phase == FusePhase.ROLLING
    val m = Gems.materialOf(line.key)

    fun held(): Int = Gems.view.value?.held(line.key) ?: 0

    fun settle(won: Boolean, v: Gems.View) {
        phase = if (won) FusePhase.WON else FusePhase.LOST
        GemSounds.play(if (won) "gem-win" else "gem-lose")
        Gems.setView(v)
    }

    fun fuse(cost: Int, reduced: Boolean) {
        val n = copies
        phase = FusePhase.WAITING
        Nexa.scope.launch {
            try {
                val res = Gems.fuse(line.key, n)
                if (reduced) {
                    settle(res.upgraded, res.view)
                    return@launch
                }
                pending = res.upgraded to res.view
                plan = rollPlan(n / cost.toFloat(), res.upgraded)
                phase = FusePhase.ROLLING
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                phase = FusePhase.IDLE
                Nexa.ui.toast(describeError(e, t("gems.fuseFailed")), "error")
            }
        }
    }

    fun finish() {
        val p = pending ?: return
        pending = null
        settle(p.first, p.second)
    }

    fun again(cost: Int) {
        copies = maxOf(1, minOf(cost, held()))
        phase = FusePhase.IDLE
    }
}

/**
 * The fuse machine (web gems/FuseMachine.tsx): put 1 to fuse_cost copies in,
 * each an equal share of the chance at the next material. The server rolls
 * first; the dial then sweeps the marker round to where that roll lands, so
 * what it shows is what happened.
 */
fun openFuseMachine(line: Trades.Line, name: String) {
    val ctl = FuseControl(line, name)
    val cost = Gems.view.value?.fuseCost ?: 5
    ctl.copies = maxOf(1, minOf(cost, ctl.held()))
    openSheet(
        key = "fuse",
        title = t("gems.fusing.title", "name" to name),
        canClose = { !ctl.busy },
        footer = { close -> FuseFooter(ctl, close) },
    ) { FuseBody(ctl) }
}

@Composable
private fun FuseFooter(ctl: FuseControl, close: () -> Unit) {
    val view by Gems.view.collectAsState()
    val cost = view?.fuseCost ?: 5
    val reduced = Nx.motion.reduced
    val held = view?.held(ctl.line.key) ?: 0
    Row(Modifier.fillMaxWidth().padding(top = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
        if (ctl.phase == FusePhase.WON || ctl.phase == FusePhase.LOST) {
            NxButton(t("gems.fusing.done"), close, kind = ButtonKind.SECONDARY)
            if (held > 0) NxButton(t("gems.fusing.again"), { ctl.again(cost) })
        } else {
            NxButton(t("chat.editMessage.cancel"), close, kind = ButtonKind.SECONDARY, enabled = !ctl.busy)
            NxButton(
                t("gems.fusing.go", "percent" to Math.round(ctl.copies * 100.0 / cost).toInt()),
                { ctl.fuse(cost, reduced) },
                loading = ctl.busy,
                enabled = held >= 1,
            )
        }
    }
}

@Composable
private fun FuseBody(ctl: FuseControl) {
    val c = Nx.colors
    val view by Gems.view.collectAsState()
    val cost = view?.fuseCost ?: 5
    val held = view?.held(ctl.line.key) ?: 0
    val phase = ctl.phase
    val m = ctl.m
    val chance = ctl.copies / cost.toFloat()
    val pct = Math.round(chance * 100)
    val done = phase == FusePhase.WON || phase == FusePhase.LOST
    val badge = ctl.line.badge?.copy(level = m)
    val nextBadge = ctl.line.badge?.copy(level = minOf(Gems.MATERIALS, m + 1))
    fun material(n: Int) = t("gems.material.$n")

    val arcColor by animateColorAsState(
        when (phase) { FusePhase.WON -> c.positive; FusePhase.LOST -> c.borderStrong; else -> c.link }, tween(360), label = "arc",
    )
    val glow by animateColorAsState(if (phase == FusePhase.LOST) c.textMuted.copy(alpha = 0.10f) else (if (phase == FusePhase.WON) c.positive else c.link).copy(alpha = 0.18f), tween(360), label = "glow")
    val sweep by androidx.compose.animation.core.animateFloatAsState(chance, tween(220), label = "chance")
    val dot = when (phase) { FusePhase.WON -> c.positive; FusePhase.LOST -> c.textDanger; else -> c.textStrong }

    rememberGemSounds()
    // the marker's turn: from the top round to where the roll landed, sampled from angleAt as on the web
    var turn by remember { mutableFloatStateOf(0f) }
    LaunchedEffect(phase) {
        val plan = ctl.plan
        when {
            phase == FusePhase.ROLLING && plan != null -> {
                // a click a peg, and a knock each time it crosses the line it ends beside
                val roll = GemSounds.Roll(plan.line)
                val clock = Animatable(0f)
                try {
                    turn = 0f
                    clock.animateTo(plan.total, tween(plan.total.toInt(), easing = LinearEasing)) {
                        val a = angleAt(value, plan)
                        turn = a
                        roll.at(a, (angleAt(value + 1f, plan) - a) * 1000f, winding = value < WIND_MS)
                    }
                    kotlinx.coroutines.delay(HOLD_MS)
                    ctl.finish()
                } finally {
                    roll.stop()
                }
            }
            phase == FusePhase.WON || phase == FusePhase.LOST -> turn = plan?.end ?: 0f
            phase == FusePhase.IDLE -> turn = 0f
        }
    }
    // the new material is there from its first frame and settles into place
    val arrive = remember { Animatable(1f) }
    val reduced = Nx.motion.reduced
    LaunchedEffect(phase) {
        if (phase == FusePhase.WON && !reduced) { arrive.snapTo(0f); arrive.animateTo(1f, tween(420, easing = SPRING_OUT)) } else arrive.snapTo(1f)
    }
    val lost = remember { Animatable(0f) }
    LaunchedEffect(phase) { lost.animateTo(if (phase == FusePhase.LOST) 1f else 0f, tween(360)) }

    Column(Modifier.fillMaxWidth().padding(top = 4.dp, bottom = 2.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Box(
            Modifier.size(196.dp)
                .then(if (phase == FusePhase.ROLLING) Modifier.pressable(remember { MutableInteractionSource() }) { ctl.finish() }.semantics { contentDescription = t("gems.skip") } else Modifier),
            contentAlignment = Alignment.Center,
        ) {
            Canvas(Modifier.fillMaxSize()) {
                val k = size.minDimension / 200f
                val r = 84f * k
                val mid = Offset(size.width / 2, size.height / 2)
                drawCircle(Brush.radialGradient(0f to glow, 0.92f to Color.Transparent, center = mid, radius = size.minDimension / 2), radius = size.minDimension / 2)
                val stroke = 12f * k
                drawCircle(c.bgActive, r, mid, style = Stroke(stroke))
                val tl = Offset(mid.x - r, mid.y - r)
                val box = androidx.compose.ui.geometry.Size(2 * r, 2 * r)
                // drawn inside its round caps, so the arc seen is exactly the chance
                if (sweep >= 1f) drawCircle(arcColor, r, mid, style = Stroke(stroke))
                else if (sweep > 0f) drawArc(arcColor, -90f + CAP_DEG, maxOf(0.01f, 360f * sweep - 2 * CAP_DEG), false, tl, box, style = Stroke(stroke, cap = StrokeCap.Round))
                rotate(turn % 360f, mid) {
                    val at = Offset(mid.x, mid.y - r)
                    drawCircle(c.bgFloating, 10f * k, at)
                    drawCircle(dot, 6f * k, at)
                }
            }
            val shown = if (phase == FusePhase.WON) nextBadge else badge
            Box(
                Modifier.graphicsLayer {
                    val a = arrive.value
                    val l = lost.value
                    alpha = (0.6f + 0.4f * a.coerceIn(0f, 1f)) * (1f - 0.55f * l)
                    val s = (0.7f + 0.3f * a) * (1f - 0.1f * l)
                    scaleX = s; scaleY = s
                    if (l > 0f && android.os.Build.VERSION.SDK_INT >= 31) {
                        val cm = android.graphics.ColorMatrix().apply { setSaturation(1f - l) }
                        renderEffect = android.graphics.RenderEffect.createColorFilterEffect(android.graphics.ColorMatrixColorFilter(cm)).asComposeRenderEffect()
                    }
                },
            ) { if (shown != null) BadgePill(shown, "0", 0L, "", size = 76.dp) }
        }

        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(2.dp)) {
            val big = Nx.type.title.copy(fontSize = Nx.type.sp(30f), lineHeight = Nx.type.sp(34f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum")
            if (done) {
                Txt(
                    if (phase == FusePhase.WON) material(m + 1) else "$pct%",
                    style = if (phase == FusePhase.LOST) big.copy(textDecoration = TextDecoration.LineThrough) else big,
                    color = if (phase == FusePhase.LOST) c.textMuted else c.textStrong, maxLines = 1,
                )
            } else {
                Txt("$pct%", style = big, color = c.textStrong, maxLines = 1)
                Txt("${material(m)} → ${material(minOf(Gems.MATERIALS, m + 1))}", style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, maxLines = 1)
            }
        }

        if (!done) {
            Row(Modifier.selectableGroup().semantics { contentDescription = t("gems.fusing.copies") }, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                for (n in 1..cost) {
                    val on = n <= ctl.copies
                    val enabled = n <= held && !ctl.busy
                    Box(
                        Modifier.size(44.dp)
                            .graphicsLayer { alpha = if (!enabled && !on) 0.4f else 1f }
                            .background(if (on) c.bgSurface else Color.Transparent, Radii.Sm)
                            .ring(Radii.Sm, if (on) arcColor else c.border, if (on) 2.dp else 1.5.dp)
                            .pressable(remember { MutableInteractionSource() }, enabled = enabled) { ctl.copies = n }
                            .semantics {
                                role = Role.RadioButton
                                selected = n == ctl.copies
                                contentDescription = t("gems.fusing.copyCount", "count" to n, "percent" to Math.round(n * 100.0 / cost).toInt())
                            },
                        contentAlignment = Alignment.Center,
                    ) { if (on && badge != null) BadgePill(badge, "0", 0L, "", still = true, size = 26.dp) }
                }
            }
        }

        val words = when {
            phase == FusePhase.WON -> t("gems.fusing.won", "material" to material(m + 1), "name" to ctl.name)
            phase == FusePhase.LOST -> t("gems.fusing.lost", "count" to ctl.copies)
            ctl.copies >= cost -> t("gems.fusing.sure", "count" to cost)
            else -> t("gems.fusing.warn")
        }
        val tone = when {
            phase == FusePhase.LOST -> c.textMuted
            !done && ctl.copies < cost -> c.textWarning
            else -> c.text
        }
        Txt(words, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(20f), textAlign = TextAlign.Center), color = tone, modifier = Modifier.widthIn(max = 320.dp))
        if (!done) Txt(t("gems.fusing.held", "count" to held), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18f)), color = c.textMuted, modifier = Modifier.padding(top = 0.dp))
    }
}
