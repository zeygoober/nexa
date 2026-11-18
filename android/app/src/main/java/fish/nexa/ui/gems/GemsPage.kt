package fish.nexa.ui.gems

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.Easing
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.GenericShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.Gems
import fish.nexa.media.GemSounds
import fish.nexa.media.rememberGemSounds
import fish.nexa.data.Trades
import fish.nexa.ui.Time
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Spinner
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.decorClock
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.profile.BadgePill
import fish.nexa.ui.settings.PageTitle
import fish.nexa.ui.settings.SectionHeading
import fish.nexa.ui.settings.settingAnchor
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import kotlinx.coroutines.launch
import kotlin.math.PI
import kotlin.math.sin

/*
 * Settings, Gems (web settings/GemsPage.tsx and GemsPage.module.css): the
 * balance, the capsule as the page's one big thing, then the collection with
 * fusing and the team as quiet tiles. The server draws every capsule and rolls
 * every fuse before anything moves; what moves only shows what came out.
 */

private const val TEAM_SIZE = 5

/** A capsule's life on the page, as the web's: drawing, the reveal's rocks, the halves parting, what came out. */
private enum class Phase { IDLE, OPENING, REVEAL, POP, GOT }

/** A rarity's colour, never shown alone: always beside its word. */
@Composable
internal fun rarityColor(rarity: String?): Color {
    val c = Nx.colors
    return when (rarity) {
        "common" -> c.textMuted
        "rare" -> c.link
        "epic" -> c.textBrand
        "huge" -> c.warning
        else -> c.link
    }
}

@Composable
internal fun RarityTag(rarity: String, size: Float = 14f) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Box(Modifier.size(9.dp).background(rarityColor(rarity), CircleShape))
        Txt(t("gems.rarity.$rarity"), style = Nx.type.body.copy(fontSize = Nx.type.sp(size)), color = Nx.colors.text, maxLines = 1)
    }
}

/** A point on a keyframed track: at [at] (0 to 1) the value is [v]; the way there eases in and out, as CSS's ease-in-out. */
private class Key(val at: Float, val v: Float)
private val EASE_IN_OUT = CubicBezierEasing(0.42f, 0f, 0.58f, 1f)
private val EASE_IN = CubicBezierEasing(0.42f, 0f, 1f, 1f)
private fun track(keys: List<Key>, p: Float, ease: Easing = EASE_IN_OUT): Float {
    if (p <= keys.first().at) return keys.first().v
    for (i in 1 until keys.size) {
        val a = keys[i - 1]
        val b = keys[i]
        if (p <= b.at) {
            val f = if (b.at == a.at) 1f else ease.transform((p - a.at) / (b.at - a.at))
            return a.v + (b.v - a.v) * f
        }
    }
    return keys.last().v
}

// the reveal's three rocks, each wider than the last; a Huge holds its breath for a fourth (web @keyframes build, buildLong)
private val BUILD_ROT = listOf(Key(0f, 0f), Key(.08f, -6f), Key(.16f, 6f), Key(.24f, 0f), Key(.36f, 0f), Key(.44f, -10f), Key(.52f, 10f), Key(.60f, 0f), Key(.70f, 0f), Key(.78f, -15f), Key(.86f, 15f), Key(1f, 0f))
private val BUILD_SCALE = listOf(Key(0f, 1f), Key(.70f, 1f), Key(.78f, 1.04f), Key(.86f, 1.06f), Key(1f, 1.08f))
private val LONG_ROT = listOf(Key(0f, 0f), Key(.05f, -6f), Key(.10f, 6f), Key(.15f, 0f), Key(.23f, 0f), Key(.28f, -10f), Key(.33f, 10f), Key(.38f, 0f), Key(.45f, 0f), Key(.50f, -15f), Key(.55f, 15f), Key(.60f, 0f), Key(.74f, 0f), Key(.80f, -20f), Key(.86f, 20f), Key(1f, 0f))
private val LONG_SCALE = listOf(Key(0f, 1f), Key(.45f, 1f), Key(.50f, 1.04f), Key(.55f, 1.06f), Key(.74f, 1.06f), Key(.80f, 1.1f), Key(.86f, 1.12f), Key(1f, 1.14f))
// the tell: on the last rock the light takes the colour of what is inside
private val FLARE_ALPHA = listOf(Key(0f, 0f), Key(.62f, 0f), Key(1f, 1f))
private val FLARE_SCALE = listOf(Key(0f, .8f), Key(.62f, .8f), Key(1f, 1.12f))
private val LONG_FLARE_ALPHA = listOf(Key(0f, 0f), Key(.45f, 0f), Key(.60f, .6f), Key(.74f, .6f), Key(1f, 1f))
private val LONG_FLARE_SCALE = listOf(Key(0f, .8f), Key(.45f, .8f), Key(.60f, .95f), Key(.74f, .95f), Key(1f, 1.2f))

/** The capsule art is cut along its lip, which it tilts by 16 degrees (web .half clip-path). */
private val TOP_HALF = GenericShape { s, _ ->
    moveTo(0f, 0f); lineTo(s.width, 0f); lineTo(s.width, s.height * .345f); lineTo(0f, s.height * .631f); close()
}
private val BOTTOM_HALF = GenericShape { s, _ ->
    moveTo(0f, s.height * .631f); lineTo(s.width, s.height * .345f); lineTo(s.width, s.height); lineTo(0f, s.height); close()
}
private val SPRING_OUT = CubicBezierEasing(0.34f, 1.56f, 0.64f, 1f)

/** A pool of light under the stage, a radial fade from [color] (web .stage, .flare). */
private fun Modifier.pool(color: Color, inner: Float, mid: Float, at: Float = 0.35f) = drawBehind {
    val r = minOf(size.width, size.height) / 2
    scale(size.width / (2 * r), size.height / (2 * r), Offset(size.width / 2, size.height / 2)) {
        drawCircle(
            Brush.radialGradient(
                0f to color.copy(alpha = color.alpha * inner),
                at to color.copy(alpha = color.alpha * inner),
                0.7f to color.copy(alpha = color.alpha * mid),
                1f to Color.Transparent,
                center = Offset(size.width / 2, size.height / 2),
                radius = r,
            ),
            radius = r,
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun GemsPage() {
    val c = Nx.colors
    val view by Gems.view.collectAsState()
    val error by Gems.error.collectAsState()
    val me by Nexa.users.me.collectAsState()
    val scope = rememberCoroutineScope()
    var phase by remember { mutableStateOf(Phase.IDLE) }
    var got by remember { mutableStateOf<Gems.Collectible?>(null) }
    var confirming by remember { mutableStateOf(false) }
    val reduced = Nx.motion.reduced
    LaunchedEffect(Unit) { Gems.load() }
    rememberGemSounds()
    // the capsule's sounds follow its phases: a rattle while drawing, the build through the reveal, the split once it opens
    val was = remember { arrayOf(Phase.IDLE) }
    LaunchedEffect(phase, got) {
        val before = was[0]
        was[0] = phase
        val g = got
        when {
            phase == Phase.OPENING -> {
                var k = 0
                while (true) {
                    GemSounds.play("gem-rattle", 0.9f, 0.95f + Math.random().toFloat() * 0.1f, ((k++ * 37) % 7) / 10f - 0.3f)
                    kotlinx.coroutines.delay(350)
                }
            }
            g == null -> {}
            phase == Phase.REVEAL -> {
                val s = GemSounds.play(if (g.rarity == "huge") "gem-build-huge" else "gem-build")
                try { kotlinx.coroutines.awaitCancellation() } finally { GemSounds.stop(s) }
            }
            // pop after the build, or got straight from a skip or under Reduced motion
            (phase == Phase.POP && before == Phase.REVEAL) || (phase == Phase.GOT && (before == Phase.REVEAL || before == Phase.OPENING)) ->
                GemSounds.play("gem-open-${g.rarity}")
        }
    }

    PageTitle(t("gems.title"))
    val v = view
    if (error != null && v == null) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Note(t("gems.failed"))
            NxButton(t("chat.messageList.tryAgain"), { scope.launch { Gems.load() } }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
        }
        return
    }
    if (v == null || me == null) {
        Box(Modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) { Spinner() }
        return
    }
    val w = v.wallet
    val held = w.state == "check" || w.state == "report"
    val short = maxOf(0L, v.capsulePrice - w.gems)
    val empty = v.collectibles.isEmpty()

    fun open() {
        phase = Phase.OPENING
        got = null
        scope.launch {
            try {
                val res = Gems.openCapsule()
                Gems.setView(res.view)
                got = res.got
                // the draw is already made; what follows only shows it, and a press skips it
                phase = if (reduced) Phase.GOT else Phase.REVEAL
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                phase = Phase.IDLE
                Nexa.ui.toast(describeError(e, t("gems.capsule.failed")), "error")
            }
        }
    }
    fun confirm() {
        confirming = true
        scope.launch {
            try {
                Gems.setView(Gems.confirm())
                Nexa.users.me.value?.let { Nexa.users.setMe(it.copy(gemState = null)) }
                Nexa.ui.toast(t("gems.hold.confirmed"), "success")
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast(describeError(e, t("gems.hold.confirmFailed")), "error")
            } finally {
                confirming = false
            }
        }
    }

    // the wallet
    FlowRow(
        Modifier.fillMaxWidth().padding(bottom = 8.dp).semantics(mergeDescendants = true) {},
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
        itemVerticalAlignment = Alignment.CenterVertically,
    ) {
        GemIcon(44.dp)
        Row(Modifier.weight(1f), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            Txt(
                gemNumber(w.gems),
                style = Nx.type.title.copy(fontSize = Nx.type.sp(34f), lineHeight = Nx.type.sp(36f), fontWeight = FontWeight(800), fontFeatureSettings = "tnum"),
                color = c.textStrong, maxLines = 1,
            )
            Txt(t("gems.unit"), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f)), color = c.textMuted, maxLines = 1, modifier = Modifier.padding(bottom = 3.dp))
        }
        Txt(
            t("gems.multiplier", "value" to gemNumber(v.multiplier / 100.0, 2)),
            style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), fontWeight = FontWeight(600), fontFeatureSettings = "tnum"),
            color = c.textStrong, maxLines = 1,
            modifier = Modifier.background(c.bgSurface, RoundedCornerShape(999.dp)).ring(RoundedCornerShape(999.dp), c.borderSubtle).padding(horizontal = 10.dp, vertical = 3.dp),
        )
    }
    Note(t("gems.hint"))
    if (w.state.isNotEmpty()) {
        FlowRow(
            Modifier.fillMaxWidth().padding(top = 16.dp).background(c.bgRaised, Radii.Md).ring(Radii.Md, c.borderSubtle).padding(horizontal = 14.dp, vertical = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            itemVerticalAlignment = Alignment.CenterVertically,
        ) {
            val words = when (w.state) {
                "check" -> t("gems.hold.check", "count" to w.held)
                "report" -> t("gems.hold.report", "count" to w.held)
                else -> t("gems.hold.off", "date" to Time.shortDate(w.offUntil))
            }
            Txt(words, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(19.6f)), color = c.textWarning, modifier = Modifier.weight(1f).widthIn(min = 180.dp))
            if (w.state == "check") NxButton(t("gems.hold.confirm"), { confirm() }, size = ButtonSize.SM, loading = confirming)
        }
    }

    // the capsule
    SectionHeading(t("gems.capsule.title"), Modifier.settingAnchor("gemCapsule").padding(top = 4.dp))
    val shownRarity = if ((phase == Phase.GOT || phase == Phase.POP) && got != null) got?.rarity else null
    val poolColor by animateColorAsState(if (shownRarity != null) rarityColor(shownRarity) else c.link, tween(360), label = "pool")
    BoxWithConstraints(Modifier.fillMaxWidth().background(c.bgSurface, Radii.Lg).ring(Radii.Lg, c.borderSubtle).padding(horizontal = 24.dp, vertical = 20.dp)) {
        val side = maxWidth >= 520.dp
        val draw: @Composable () -> Unit = {
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(14.dp), modifier = if (side) Modifier.width(240.dp) else Modifier.fillMaxWidth()) {
                CapsuleStage(phase, got, poolColor, onPhase = { phase = it })
                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (empty) Note(t("gems.capsule.empty"))
                    else {
                        val busy = phase == Phase.OPENING || phase == Phase.REVEAL || phase == Phase.POP
                        NxButton(
                            if (phase == Phase.GOT) t("gems.capsule.again") else t("gems.capsule.open"),
                            { open() },
                            loading = busy,
                            enabled = short == 0L && !held,
                            modifier = Modifier.semantics { contentDescription = "${if (phase == Phase.GOT) t("gems.capsule.again") else t("gems.capsule.open")}, ${t("gems.count", "count" to v.capsulePrice)}" },
                            trailing = {
                                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                                    Box(Modifier.width(1.dp).height(18.dp).background(c.onBrand.copy(alpha = 0.35f)))
                                    Box(Modifier.width(4.dp))
                                    GemIcon(18.dp)
                                    Txt(gemNumber(v.capsulePrice), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(600), fontFeatureSettings = "tnum"), color = c.onBrand, maxLines = 1)
                                }
                            },
                        )
                        if (short > 0 && !held) Txt(t("gems.capsule.need", "count" to short), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
                    }
                }
            }
        }
        val odds: @Composable (Modifier) -> Unit = { m ->
            Column(m, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                val shown = Gems.RARITIES.filter { (v.odds[it] ?: 0) > 0 }
                shown.forEachIndexed { i, r ->
                    Column {
                        Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                            Box(Modifier.weight(1f)) { RarityTag(r) }
                            Txt("${gemNumber((v.odds[r] ?: 0) / 100.0, 2)}%", style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(700), fontFeatureSettings = "tnum"), color = c.textStrong, maxLines = 1)
                        }
                        if (i < shown.lastIndex) Box(Modifier.fillMaxWidth().height(1.dp).background(c.borderSubtle))
                    }
                }
                if (empty) Note(t("gems.capsule.noOdds"))
            }
        }
        if (side) Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(40.dp)) {
            draw()
            odds(Modifier.weight(1f))
        } else Column(verticalArrangement = Arrangement.spacedBy(20.dp)) {
            draw()
            odds(Modifier.fillMaxWidth())
        }
    }

    // the collection
    SectionHeading(t("gems.collection.title"), Modifier.settingAnchor("gemCollection").padding(top = 4.dp))
    if (v.collection.isEmpty()) Note(t("gems.collection.empty"))
    else {
        BoxWithConstraints(Modifier.fillMaxWidth()) {
            // as the web's repeat(auto-fill, minmax(72px, 1fr)) with 10 between
            val cols = maxOf(1, ((maxWidth + 10.dp) / 82.dp).toInt())
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                for (row in v.collection.chunked(cols)) {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        for (l in row) Box(Modifier.weight(1f)) { CollectionItem(l, v) }
                        repeat(cols - row.size) { Box(Modifier.weight(1f)) }
                    }
                }
            }
        }
        Note(t("gems.collection.chance", "percent" to Math.round(100.0 / v.fuseCost).toInt(), "count" to v.fuseCost), Modifier.padding(top = 10.dp))
    }

    // the team
    SectionHeading(t("gems.team.title"), Modifier.settingAnchor("gemTeam").padding(top = 4.dp))
    FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        for (i in 0 until TEAM_SIZE) {
            val m = v.team.getOrNull(i)
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                if (m != null) {
                    Tile(Modifier.size(64.dp)) { BadgePill(m.badge, "0", 0L, "", size = 40.dp) }
                    Txt(t("gems.team.power", "value" to gemNumber(m.power.toLong())), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(if (m.power == 0) 500 else 700), fontFeatureSettings = "tnum"), color = if (m.power == 0) c.textMuted else c.textStrong, maxLines = 1)
                } else {
                    Box(Modifier.size(64.dp).ring(Radii.Md, c.border).graphicsLayer { alpha = 0.6f }.semantics { contentDescription = t("gems.team.emptySlot") })
                    Txt(" ", style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12f)))
                }
            }
        }
    }
    Note(t("gems.team.hint"), Modifier.padding(top = 10.dp))
}

@Composable
private fun Note(text: String, modifier: Modifier = Modifier) {
    Txt(text, style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(20.3f)), color = Nx.colors.textMuted, modifier = modifier)
}

/** `.tile`: a quiet square a badge sits in. */
@Composable
private fun Tile(modifier: Modifier, content: @Composable androidx.compose.foundation.layout.BoxScope.() -> Unit) {
    val c = Nx.colors
    Box(modifier.background(c.bgSurface, Radii.Md).ring(Radii.Md, c.borderSubtle), contentAlignment = Alignment.Center) { content() }
}

@Composable
private fun CollectionItem(l: Trades.Line, v: Gems.View) {
    val c = Nx.colors
    val m = Gems.materialOf(l.key)
    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Tile(Modifier.fillMaxWidth().aspectRatio(1f)) {
            if (l.badge != null) BadgePill(l.badge, "0", 0L, "", size = 48.dp)
            if (l.qty > 1) Txt(
                "×${l.qty}",
                modifier = Modifier.align(Alignment.BottomEnd).padding(4.dp).background(c.bgBase, RoundedCornerShape(999.dp)).ring(RoundedCornerShape(999.dp), c.border).padding(horizontal = 5.dp, vertical = 1.dp)
                    .semantics { contentDescription = t("badges.index.copies", "count" to l.qty) },
                style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(11f), lineHeight = Nx.type.sp(16f), fontWeight = FontWeight(700), fontFeatureSettings = "tnum"),
                color = c.textStrong, maxLines = 1,
            )
        }
        if (m >= Gems.MATERIALS) {
            Box(Modifier.height(32.dp), contentAlignment = Alignment.Center) {
                Txt(t("gems.material.5"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(600)), color = c.textMuted, maxLines = 1)
            }
        } else {
            NxButton(
                t("gems.fuse"),
                { openFuseMachine(l, v.nameOf(l.key)) },
                kind = ButtonKind.SECONDARY,
                size = ButtonSize.SM,
                modifier = Modifier.semantics { contentDescription = t("gems.fusing.open", "material" to t("gems.material.${m + 1}")) },
            )
        }
    }
}

/**
 * The stage: the capsule on a pool of the Gem's blue. While the server draws it
 * rocks patiently; once the draw is in, three rocks build (four for a Huge) as
 * the light flares to what came out, the halves part and it rises between them.
 */
@Composable
private fun CapsuleStage(phase: Phase, got: Gems.Collectible?, poolColor: Color, onPhase: (Phase) -> Unit) {
    val c = Nx.colors
    val huge = got?.rarity == "huge"
    val reveal = remember { Animatable(0f) }
    val split = remember { Animatable(0f) }
    val flareOut = remember { Animatable(0f) }
    val rise = remember { Animatable(1f) }
    val reduced = Nx.motion.reduced
    LaunchedEffect(phase) {
        when (phase) {
            Phase.REVEAL -> {
                reveal.snapTo(0f)
                rise.snapTo(0f)
                reveal.animateTo(1f, tween(if (huge) 2300 else 1500, easing = LinearEasing))
                onPhase(Phase.POP)
            }
            Phase.POP -> {
                split.snapTo(0f); flareOut.snapTo(0f); rise.snapTo(0f)
                launch { flareOut.animateTo(1f, tween(420, easing = FastOutSlowInEasing)) }
                launch { rise.animateTo(1f, tween(300, easing = SPRING_OUT)) }
                split.animateTo(1f, tween(320, easing = FastOutSlowInEasing))
                onPhase(Phase.GOT)
            }
            Phase.GOT -> {
                // a press that skipped the reveal still lets what came out rise, from partly there
                if (rise.value < 1f && !reduced) rise.animateTo(1f, tween(300, easing = SPRING_OUT)) else rise.snapTo(1f)
            }
            else -> {
                reveal.snapTo(0f); split.snapTo(0f); flareOut.snapTo(0f); rise.snapTo(1f)
            }
        }
    }
    // while the server draws: a small, patient rock, which says something is happening
    val wait by decorClock(700, informs = true)
    val p = reveal.value
    val rot = when (phase) {
        Phase.OPENING -> -4f * sin(2 * PI.toFloat() * wait)
        Phase.REVEAL -> track(if (huge) LONG_ROT else BUILD_ROT, p)
        else -> 0f
    }
    val scale = when (phase) {
        Phase.REVEAL -> track(if (huge) LONG_SCALE else BUILD_SCALE, p)
        Phase.POP -> 1.08f
        else -> 1f
    }
    val flareColor = rarityColor(got?.rarity)
    val flareAlpha = when (phase) {
        Phase.REVEAL -> track(if (huge) LONG_FLARE_ALPHA else FLARE_ALPHA, p, EASE_IN)
        Phase.POP -> 1f - flareOut.value
        else -> 0f
    }
    val flareScale = when (phase) {
        Phase.REVEAL -> track(if (huge) LONG_FLARE_SCALE else FLARE_SCALE, p, EASE_IN)
        Phase.POP -> 1.12f + 0.28f * flareOut.value
        else -> 1f
    }

    Box(
        Modifier.size(180.dp, 168.dp).pool(poolColor, 0.30f, 0.08f, at = 0f),
        contentAlignment = Alignment.Center,
    ) {
        Box(Modifier.fillMaxSize().graphicsLayer { alpha = flareAlpha; scaleX = flareScale; scaleY = flareScale }.pool(flareColor, 0.85f, 0.30f))
        if (phase != Phase.GOT) {
            val s = split.value
            Box(
                Modifier.size(120.dp)
                    .graphicsLayer { transformOrigin = TransformOrigin(0.5f, 0.88f); rotationZ = rot; scaleX = scale; scaleY = scale }
                    .then(if (phase == Phase.REVEAL) Modifier.pressable(remember { MutableInteractionSource() }) { onPhase(Phase.GOT) }.semantics { contentDescription = t("gems.skip") } else Modifier),
            ) {
                val art = fish.nexa.data.Ota.uri("badges/capsule.svg")
                coil3.compose.AsyncImage(art, null, Modifier.fillMaxSize().graphicsLayer {
                    shape = TOP_HALF; clip = true
                    translationX = -12.dp.toPx() * s; translationY = -36.dp.toPx() * s; rotationZ = -26f * s; alpha = 1f - s
                })
                coil3.compose.AsyncImage(art, null, Modifier.fillMaxSize().graphicsLayer {
                    shape = BOTTOM_HALF; clip = true
                    translationX = 8.dp.toPx() * s; translationY = 26.dp.toPx() * s; rotationZ = 12f * s; alpha = 1f - s
                })
            }
        }
        val g = got
        if ((phase == Phase.POP || phase == Phase.GOT) && g != null) {
            val r = rise.value
            Column(
                Modifier.widthIn(max = 180.dp).graphicsLayer {
                    // starts already partly there, so no frame shows neither the capsule nor what came out
                    alpha = 0.55f + 0.45f * r.coerceIn(0f, 1f)
                    translationY = 10.dp.toPx() * (1f - r)
                    val k = 0.86f + 0.14f * r
                    scaleX = k; scaleY = k
                },
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                BadgePill(g.badge, "0", 0L, "", size = 88.dp)
                Txt(g.name, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(700)), color = c.textStrong, maxLines = 1)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    RarityTag(g.rarity, 13f)
                    Txt(t("gems.material.1"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, maxLines = 1)
                }
            }
        }
    }
}
