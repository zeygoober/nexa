package fish.nexa.data

import fish.nexa.Nexa
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put

/**
 * Gems (web api/gems.ts, stores/gems.ts; internal/api/gems.go): earned by
 * chatting, spent on capsules of collectibles, fused and traded. The server
 * draws every capsule and rolls every fuse; the app only shows what came out.
 */
object Gems {
    /** Rarities from the commonest, as capsules list them (store.GemRarities). */
    val RARITIES = listOf("common", "rare", "epic", "huge")
    /** The trade line that carries Gems (store.GemsKey): its qty is how many. */
    const val KEY = "gems"
    /** Materials a collectible fuses through, bronze to prism. */
    const val MATERIALS = 5

    /** [state] "" earns as usual; "check" holds new Gems until a captcha; "report" waits on staff; "off" until [offUntil]. */
    data class Wallet(val gems: Long, val held: Long, val earned: Long, val state: String, val offUntil: Long)
    /** A team badge and what it adds to earning, in percent. */
    data class Member(val badge: Badges.Badge, val power: Int)
    data class Collectible(val id: String, val name: String, val rarity: String, val badge: Badges.Badge)
    data class View(
        val wallet: Wallet,
        val team: List<Member>,
        /** Earning in percent: 100 is one Gem a message. */
        val multiplier: Int,
        /** What the plan alone multiplies earning by, in percent (100 without one). */
        val planBoost: Int,
        val capsulePrice: Long,
        val fuseCost: Int,
        /** The chance of each rarity in hundredths of a percent, as the server draws it. */
        val odds: Map<String, Int>,
        val collectibles: List<Collectible>,
        /** The collectibles this account holds, with copies. */
        val collection: List<Trades.Line>,
    ) {
        fun held(key: String) = collection.firstOrNull { it.key == key }?.qty ?: 0
        fun nameOf(key: String) = collectibles.firstOrNull { key.startsWith("${Badges.EVENT_PREFIX}${it.id}-") }?.name ?: ""
    }

    /** A collectible's key says its material last: `event:<id>-<m>`. */
    fun materialOf(key: String): Int = key.substringAfterLast('-').toIntOrNull() ?: 1

    private val _view = MutableStateFlow<View?>(null)
    /** The Gems page's view, loaded when it opens; the gateway's gems_update keeps the wallet current. */
    val view: StateFlow<View?> = _view
    private val _error = MutableStateFlow<Throwable?>(null)
    val error: StateFlow<Throwable?> = _error

    private fun JsonObject.str(k: String) = (this[k] as? JsonPrimitive)?.contentOrNull
    private fun JsonObject.long(k: String) = (this[k] as? JsonPrimitive)?.longOrNull ?: 0L
    private fun JsonObject.int(k: String) = (this[k] as? JsonPrimitive)?.intOrNull ?: 0
    private fun badge(el: JsonElement?) = el?.let { Badges.parse(JsonArray(listOf(it)))?.firstOrNull() }

    fun parseWallet(el: JsonElement?): Wallet? {
        val o = el as? JsonObject ?: return null
        return Wallet(o.long("gems"), o.long("held"), o.long("earned"), o.str("state") ?: "", o.long("off_until"))
    }

    private fun parseCollectible(el: JsonElement): Collectible? {
        val o = el as? JsonObject ?: return null
        return Collectible(o.str("id") ?: return null, o.str("name") ?: "", o.str("rarity") ?: "common", badge(o["badge"]) ?: return null)
    }

    fun parse(el: JsonElement?): View? {
        val o = el as? JsonObject ?: return null
        return View(
            wallet = parseWallet(o["wallet"]) ?: return null,
            team = (o["team"] as? JsonArray)?.mapNotNull { e ->
                val m = e as? JsonObject ?: return@mapNotNull null
                Member(badge(m["badge"]) ?: return@mapNotNull null, m.int("power"))
            }.orEmpty(),
            multiplier = o.int("multiplier").takeIf { it > 0 } ?: 100,
            planBoost = o.int("plan_boost").takeIf { it > 0 } ?: 100,
            capsulePrice = o.long("capsule_price"),
            fuseCost = o.int("fuse_cost").takeIf { it > 0 } ?: 5,
            odds = (o["odds"] as? JsonObject)?.mapNotNull { (k, v) -> (v as? JsonPrimitive)?.intOrNull?.let { k to it } }?.toMap().orEmpty(),
            collectibles = (o["collectibles"] as? JsonArray)?.mapNotNull(::parseCollectible).orEmpty(),
            collection = Trades.lines(o["collection"]),
        )
    }

    private fun need(el: JsonElement?): View = parse(el) ?: throw IllegalStateException("gems unreadable")

    private var loading = false
    suspend fun load() {
        if (loading) return
        loading = true
        _error.value = null
        try {
            _view.value = need(Nexa.api.get("/gems"))
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            _error.value = e
        } finally {
            loading = false
        }
    }

    fun setView(v: View) { _view.value = v }
    fun setWallet(w: Wallet) = _view.update { it?.copy(wallet = w) }
    fun reset() {
        _view.value = null
        _error.value = null
    }

    /** What a capsule gave, and the view after it. */
    data class Opened(val got: Collectible, val key: String, val view: View)
    /** Spends a capsule's price on one collectible, drawn at the odds shown. */
    suspend fun openCapsule(): Opened {
        val o = Nexa.api.request("POST", "/gems/capsule") as JsonObject
        val got = o["got"]?.let(::parseCollectible) ?: throw IllegalStateException("capsule unreadable")
        return Opened(got, o.str("key") ?: "", need(o["gems"]))
    }

    /** How a fuse went: [upgraded] says whether it worked; the copies are spent either way. */
    data class Fused(val key: String, val upgraded: Boolean, val view: View)
    /** Puts 1 to fuse_cost copies of [key] in for one of the next material; each copy is an equal share of the chance. */
    suspend fun fuse(key: String, copies: Int): Fused {
        val o = Nexa.api.request("POST", "/gems/fuse", buildJsonObject { put("key", key); put("copies", copies) }) as JsonObject
        return Fused(o.str("key") ?: key, (o["upgraded"] as? JsonPrimitive)?.contentOrNull == "true", need(o["gems"]))
    }

    /** Answers a farming check; the captcha is asked for by the client's solver. */
    suspend fun confirm(): View = need(Nexa.api.request("POST", "/gems/confirm"))

    /** Everything kept about this account's Gems, for the data export. */
    suspend fun record(): JsonElement? = Nexa.api.get("/gems/record")
}
