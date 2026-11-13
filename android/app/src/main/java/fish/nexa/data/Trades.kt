package fish.nexa.data

import fish.nexa.Nexa
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put

/**
 * Badge trades (web stores/trades.ts, api/trades.ts; internal/store/badge_trades.go).
 * A trade is a window two people share: each puts badges of their own on their
 * side, and when both press Ready on what is there, it happens. Every change
 * goes to both as a trade_update. A Ready names the version of the offers it was
 * given for, so it never agrees to offers that changed under it.
 */
object Trades {
    /** Some copies of one badge, named by its Badge Index key (bug-3, og, event:12); [badge] is how to draw it. */
    data class Line(val key: String, val qty: Int, val badge: Badges.Badge?)

    data class Trade(
        val id: String,
        /** The person who opened the trade, and the one it was opened with. */
        val fromId: String,
        val toId: String,
        val fromOffer: List<Line>,
        val toOffer: List<Line>,
        val fromReady: Boolean,
        val toReady: Boolean,
        val version: Int,
        /** pending, accepted, declined or cancelled. */
        val status: String,
        val channelId: String,
        val messageId: String,
    ) {
        val pending get() = status == "pending"
        fun mineIsFrom(me: String?) = fromId == me
        fun offerOf(me: String?) = if (mineIsFrom(me)) fromOffer else toOffer
        fun offerOfOther(me: String?) = if (mineIsFrom(me)) toOffer else fromOffer
        fun readyOf(me: String?) = if (mineIsFrom(me)) fromReady else toReady
        fun readyOfOther(me: String?) = if (mineIsFrom(me)) toReady else fromReady
        fun otherId(me: String?) = if (mineIsFrom(me)) toId else fromId
    }

    private val _byId = MutableStateFlow<Map<String, Trade>>(emptyMap())
    /** Trades as the server last said they stand: the window and the cards read them, the gateway keeps them current. */
    val byId: StateFlow<Map<String, Trade>> = _byId

    fun set(t: Trade) = _byId.update { it + (t.id to t) }
    fun reset() { _byId.value = emptyMap() }

    private fun lines(el: JsonElement?): List<Line> = (el as? JsonArray)?.mapNotNull { e ->
        val o = e as? JsonObject ?: return@mapNotNull null
        val key = (o["key"] as? JsonPrimitive)?.contentOrNull ?: return@mapNotNull null
        Line(key, (o["qty"] as? JsonPrimitive)?.intOrNull ?: 0, o["badge"]?.let { Badges.parse(JsonArray(listOf(it)))?.firstOrNull() })
    }.orEmpty()

    fun parse(el: JsonElement?): Trade? {
        val o = el as? JsonObject ?: return null
        fun s(k: String) = (o[k] as? JsonPrimitive)?.contentOrNull
        return Trade(
            id = s("id") ?: return null,
            fromId = s("from_id") ?: return null,
            toId = s("to_id") ?: return null,
            fromOffer = lines(o["from_offer"]),
            toOffer = lines(o["to_offer"]),
            fromReady = (o["from_ready"] as? JsonPrimitive)?.booleanOrNull == true,
            toReady = (o["to_ready"] as? JsonPrimitive)?.booleanOrNull == true,
            version = (o["version"] as? JsonPrimitive)?.intOrNull ?: 0,
            status = s("status") ?: "pending",
            channelId = s("channel_id") ?: "",
            messageId = s("message_id") ?: "",
        )
    }

    private fun need(el: JsonElement): Trade = parse(el) ?: throw IllegalStateException("trade unreadable")

    /** What this account can put in a trade, with how many copies of each. */
    suspend fun mine(): List<Line> = lines(Nexa.api.get("/trades/holdings"))

    /** Starts a trade with someone, or gives the one already open between the two. */
    suspend fun open(to: String): Trade = need(Nexa.api.request("POST", "/trades", buildJsonObject { put("to", to) })).also(::set)

    suspend fun get(id: String): Trade = need(Nexa.api.get("/trades/$id")).also(::set)

    /** Replaces this account's side. Both Ready marks go. */
    suspend fun offer(id: String, lines: Map<String, Int>): Trade {
        val body = buildJsonObject {
            put("lines", buildJsonArray {
                for ((k, n) in lines) if (n > 0) add(buildJsonObject { put("key", k); put("qty", n) })
            })
        }
        return need(Nexa.api.request("PUT", "/trades/$id/offer", body)).also(::set)
    }

    /** Marks this account ready, or not, for the offers at [version]; when both are, it happens. */
    suspend fun ready(id: String, ready: Boolean, version: Int): Trade =
        need(Nexa.api.request("PUT", "/trades/$id/ready", buildJsonObject { put("ready", ready); put("version", version) })).also(::set)

    /** Ends it before it happened: declined by the one it was opened with, cancelled by the one who opened it. */
    suspend fun end(id: String): Trade = need(Nexa.api.request("POST", "/trades/$id/end")).also(::set)

    /** A message's own copy of its trade, for a card drawn before any update came. */
    fun of(m: fish.nexa.net.Message): Trade? = byId.value[parse(m.trade)?.id ?: return null] ?: parse(m.trade)
}
