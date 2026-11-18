package fish.nexa.media

import android.media.AudioAttributes
import android.media.SoundPool
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import fish.nexa.Nexa
import fish.nexa.data.Ota
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * The Gems sounds (made by tools/gem_sounds.py, the web's lib/gemSounds.ts):
 * the fuse dial clicks once a peg as the marker passes it, knocks as it crosses
 * the line it ends beside and answers with a win or a loss; a capsule builds while
 * it rocks and opens with chimes by rarity. Off with the synced `gemSounds`,
 * at the sounds volume.
 *
 * A SoundPool, since a click has to start on the frame the marker passes its
 * peg; it holds the sounds decoded only while a Gems screen is open
 * ([rememberGemSounds]), which lets them go when the last one closes.
 */
object GemSounds {
    private val NAMES = listOf(
        "gem-tick", "gem-edge", "gem-win", "gem-lose", "gem-rattle", "gem-build", "gem-build-huge",
        "gem-open-common", "gem-open-rare", "gem-open-epic", "gem-open-huge",
    )
    /** Degrees between the dial's pegs (lib/gemSounds.ts PEG). */
    const val PEG = 15f
    private const val FILE_GAIN = 0.7f

    private var pool: SoundPool? = null
    private var users = 0
    private val ids = HashMap<String, Int>()
    private val loaded = ConcurrentHashMap.newKeySet<Int>()

    fun on(): Boolean = runCatching { Nexa.settings.gemSounds }.getOrDefault(true)

    @Synchronized internal fun acquire() {
        users++
        if (pool != null) return
        val p = SoundPool.Builder()
            .setMaxStreams(16)
            .setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_ASSISTANCE_SONIFICATION)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                    .build(),
            )
            .build()
        p.setOnLoadCompleteListener { _, id, status -> if (status == 0) loaded += id }
        pool = p
        for (n in NAMES) runCatching { ids[n] = Ota.load(p, "sounds/$n.m4a") }
    }

    @Synchronized internal fun release() {
        if (--users > 0) return
        users = 0
        pool?.release()
        pool = null
        ids.clear()
        loaded.clear()
    }

    /**
     * Plays [name] unless the sounds are off ([always] for Settings' preview):
     * [gain] on top of the volume, [rate] 0.5 to 2, [pan] -1 left to 1 right.
     * Returns its stream, or 0 when nothing played (not loaded yet, or off).
     */
    @Synchronized fun play(name: String, gain: Float = 1f, rate: Float = 1f, pan: Float = 0f, always: Boolean = false): Int {
        if (!always && !on()) return 0
        val p = pool ?: return 0
        val id = ids[name]?.takeIf { it in loaded } ?: return 0
        val v = (FILE_GAIN * Sounds.volume() / 100f * gain).coerceIn(0f, 1f)
        val l = v * minOf(1f, 1f - pan)
        val r = v * minOf(1f, 1f + pan)
        return p.play(id, l, r, 1, 0, rate.coerceIn(0.5f, 2f))
    }

    /** What Settings, Sounds plays to hear them, on or off: a dial slowing to a stop, then a win. */
    fun preview() {
        Nexa.scope.launch {
            repeat(9) { i ->
                play("gem-tick", 0.75f - i * 0.04f, 0.96f + Math.random().toFloat() * 0.08f, -0.15f, always = true)
                delay(60L + i * i * 6L)
            }
            delay(350)
            play("gem-win", always = true)
        }
    }

    @Synchronized fun stop(stream: Int) {
        if (stream != 0) pool?.stop(stream)
    }

    /**
     * The roll's clicks, fed the marker's angle each frame: one when it passes a
     * peg, louder the faster it goes and lower while it winds back, and a knock
     * each time it crosses [line], the line it ends beside, either way, once
     * wound up and on the last lap (null when it is not close).
     */
    class Roll(private val line: Float?) {
        private var last = 0
        private var side = 0f
        private val streams = ArrayList<Int>()

        fun at(angle: Float, degPerSec: Float, winding: Boolean) {
            val peg = kotlin.math.floor(angle / PEG).toInt()
            if (peg != last) {
                last = peg
                val g = 0.35f + 0.4f * minOf(1f, kotlin.math.abs(degPerSec) / 600f)
                val wobble = 0.96f + Math.random().toFloat() * 0.08f
                keep(play("gem-tick", if (winding) g * 0.6f else g, (if (winding) 0.81f else 1f) * wobble, -0.15f))
            }
            if (line == null) return
            val now = kotlin.math.sign(angle - line)
            if (winding || side == 0f) { if (now != 0f) side = now; return }
            if (now != 0f && now != side) {
                if (angle > line - 180f) keep(play("gem-edge", 0.8f, 1f, 0.1f))
                side = now
            }
        }

        private fun keep(s: Int) {
            if (s == 0) return
            streams += s
            if (streams.size > 24) streams.removeAt(0)
        }

        /** A skip: what is still sounding stops. */
        fun stop() {
            streams.forEach { stop(it) }
            streams.clear()
        }
    }
}

/** Keeps the Gems sounds loaded while the calling screen is shown. */
@Composable
fun rememberGemSounds() {
    DisposableEffect(Unit) {
        GemSounds.acquire()
        onDispose { GemSounds.release() }
    }
}
