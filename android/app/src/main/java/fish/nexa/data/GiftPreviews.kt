package fish.nexa.data

import androidx.compose.runtime.mutableStateMapOf
import fish.nexa.Nexa
import fish.nexa.net.ApiError
import fish.nexa.net.GiftLinkView
import fish.nexa.net.wireJson
import kotlinx.coroutines.launch
import java.net.URLEncoder

/** Gift links this device has asked about, for the cards under messages (web lib/giftPreviews.ts). */
object GiftPreviews {
    sealed interface Entry {
        data object Loading : Entry
        data class Ready(val gift: GiftLinkView, val at: Long) : Entry
        data class Invalid(val at: Long) : Entry
        data class Failed(val at: Long) : Entry
    }

    /** Someone else may claim it: a card drawn again after this long asks again. */
    private const val FRESH_MS = 60_000L
    private const val RETRY_MS = 30_000L

    val byCode = mutableStateMapOf<String, Entry>()
    private val inflight = HashSet<String>()
    private var accountGeneration = 0

    fun clear() {
        accountGeneration++
        byCode.clear()
        synchronized(inflight) { inflight.clear() }
    }

    private fun path(code: String) = "/gifts/" + URLEncoder.encode(code, "UTF-8")

    fun load(code: String, force: Boolean = false) {
        val now = System.currentTimeMillis()
        val had = byCode[code]
        val age = when (had) {
            is Entry.Ready -> now - had.at < FRESH_MS
            is Entry.Invalid -> now - had.at < FRESH_MS
            is Entry.Failed -> now - had.at < RETRY_MS
            else -> false
        }
        if (!force && age) return
        if (!synchronized(inflight) { inflight.add(code) }) return
        val generation = accountGeneration
        // a card already drawn keeps what it shows while it is asked again
        if (had !is Entry.Ready) byCode[code] = Entry.Loading
        Nexa.scope.launch {
            val next = try {
                Entry.Ready(wireJson.decodeFromJsonElement(GiftLinkView.serializer(), Nexa.api.request("GET", path(code))), System.currentTimeMillis())
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                if (e is ApiError && e.status == 404) Entry.Invalid(System.currentTimeMillis()) else Entry.Failed(System.currentTimeMillis())
            }
            if (generation == accountGeneration) {
                byCode[code] = next
                synchronized(inflight) { inflight.remove(code) }
            }
        }
    }

    /** Claims a gift from its card. Throws what the server said. */
    suspend fun claim(code: String) {
        val gift = wireJson.decodeFromJsonElement(GiftLinkView.serializer(), Nexa.api.request("POST", path(code) + "/claim"))
        byCode[code] = Entry.Ready(gift, System.currentTimeMillis())
        Plans.load()
        if (gift.plan == "short_name") Nexa.users.me.value?.let { Nexa.users.setMe(it.copy(shortNameCredits = it.shortNameCredits + 1)) }
    }
}
