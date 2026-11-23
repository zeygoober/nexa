package fish.nexa.data

import fish.nexa.Nexa
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.longOrNull

/**
 * Quests (web api/quests.ts, stores/quests.ts; internal/api/quests.go): three
 * daily quests and their bonus, and the Infinite Trial, paid in Gems by the
 * server the moment one is finished. The app only shows the board.
 */
object Quests {
    /** One quest. [track] is daily, bonus or trial; [level] 1 to 3 for a daily, the step for the Trial. */
    data class Quest(
        val id: String,
        val track: String,
        val level: Long,
        val kind: String,
        val target: Long,
        val progress: Long,
        val reward: Long,
        val doneAt: Long,
        val paid: Long,
    ) {
        val done get() = doneAt > 0
    }

    data class Board(
        val resetsAt: Long,
        val daily: List<Quest>,
        val bonus: Quest?,
        val trial: Quest?,
        val trialCleared: Long,
        val rerollLeft: Boolean,
        val skipLeft: Boolean,
    )

    private val _board = MutableStateFlow<Board?>(null)
    /** The board, loaded when the Quests page opens; quests_update keeps it current. */
    val board: StateFlow<Board?> = _board
    private val _error = MutableStateFlow<Throwable?>(null)
    val error: StateFlow<Throwable?> = _error

    private fun JsonObject.str(k: String) = (this[k] as? JsonPrimitive)?.contentOrNull
    private fun JsonObject.long(k: String) = (this[k] as? JsonPrimitive)?.longOrNull ?: 0L
    private fun JsonObject.bool(k: String) = (this[k] as? JsonPrimitive)?.booleanOrNull ?: false

    fun parseQuest(el: JsonElement?): Quest? {
        val o = el as? JsonObject ?: return null
        return Quest(
            id = o.str("id") ?: return null,
            track = o.str("track") ?: "",
            level = o.long("level"),
            kind = o.str("kind") ?: return null,
            target = o.long("target"),
            progress = o.long("progress"),
            reward = o.long("reward"),
            doneAt = o.long("done_at"),
            paid = o.long("paid"),
        )
    }

    fun parse(el: JsonElement?): Board? {
        val o = el as? JsonObject ?: return null
        return Board(
            resetsAt = o.long("resets_at"),
            daily = (o["daily"] as? JsonArray)?.mapNotNull(::parseQuest).orEmpty(),
            bonus = parseQuest(o["bonus"]),
            trial = parseQuest(o["trial"]),
            trialCleared = o.long("trial_cleared"),
            rerollLeft = o.bool("reroll_left"),
            skipLeft = o.bool("skip_left"),
        )
    }

    /** The quests a quests_update says were just finished. */
    fun completed(el: JsonElement?): List<Quest> = ((el as? JsonObject)?.get("completed") as? JsonArray)?.mapNotNull(::parseQuest).orEmpty()

    private fun need(el: JsonElement?): Board = parse(el) ?: throw IllegalStateException("quests unreadable")

    private var loading = false
    suspend fun load() {
        if (loading) return
        loading = true
        _error.value = null
        try {
            _board.value = need(Nexa.api.get("/quests"))
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            _error.value = e
        } finally {
            loading = false
        }
    }

    fun set(b: Board) { _board.value = b }
    fun reset() {
        _board.value = null
        _error.value = null
    }

    /** Swaps an open daily quest for another of its level, once a day. */
    suspend fun reroll(id: String): Board = need(Nexa.api.request("POST", "/quests/$id/reroll"))

    /** Swaps the Trial's quest for another at the same step, once a day. */
    suspend fun skipTrial(): Board = need(Nexa.api.request("POST", "/quests/trial/skip"))
}
