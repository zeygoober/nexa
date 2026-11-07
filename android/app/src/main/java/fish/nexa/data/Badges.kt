package fish.nexa.data

import fish.nexa.Nexa
import fish.nexa.ui.t
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.put

/**
 * Profile badges (web lib/badges.ts, internal/store/badges.go). The server
 * works out which an account holds and puts them in its order; this names
 * and pictures them, staff rank and plan included, and keeps this account's
 * own list for the profile designer.
 */
object Badges {
    const val CONTRIBUTOR = "contributor"
    const val EVENT_PREFIX = "event:"

    /** One badge: [level] is Bug Hunter's, Ideas' or Gift Giver's tier, or the years on Nexa; [name], [icon] and [color] are an event badge's. */
    data class Badge(
        val id: String,
        val level: Int = 0,
        val name: String? = null,
        val icon: String? = null,
        val color: Int? = null,
        val hidden: Boolean = false,
    ) {
        val isEvent get() = id.startsWith(EVENT_PREFIX)
    }

    /** The icons the Badges tab offers for an event badge, as the web's EVENT_ICONS. */
    val EVENT_ICONS = listOf(
        "award", "trophy", "medal", "crown", "star", "sparkles", "heart", "flame",
        "zap", "rocket", "ghost", "skull", "candy", "snowflake", "tree-pine", "gift",
        "party-popper", "cake", "music", "gamepad-2", "palette", "camera", "paintbrush", "pen-tool",
        "flag", "shield", "gem", "sun", "moon", "cloud", "leaf", "flower-2",
        "bug", "code", "terminal", "megaphone", "mic", "headphones", "book-open", "graduation-cap",
    )

    /** The colours the Badges tab offers first, as the web's EVENT_COLOURS. */
    val EVENT_COLOURS = listOf(0xF0C04A, 0xFF7A45, 0xFF5C93, 0xB48CFF, 0x6AA2FF, 0x4CC9B0, 0x5FD17A, 0xB9C3CF)

    fun parse(el: JsonElement?): List<Badge>? = (el as? JsonArray)?.mapNotNull { e ->
        val o = e as? JsonObject ?: return@mapNotNull null
        fun s(k: String) = (o[k] as? JsonPrimitive)?.contentOrNull
        fun i(k: String) = (o[k] as? JsonPrimitive)?.intOrNull
        Badge(
            id = s("id") ?: return@mapNotNull null,
            level = i("level") ?: 0,
            name = s("name"),
            icon = s("icon"),
            color = i("color"),
            hidden = (o["hidden"] as? JsonPrimitive)?.booleanOrNull == true,
        )
    }

    /**
     * How a badge looks (web badgeLook): its picture's name under badges/ (written by
     * tools/badge_art.py, copied from the web and carried over the air), or for an
     * event badge staff made, its Lucide [icon] on a disc in [color]; its name and what it means.
     */
    data class Look(val art: String?, val label: String, val tip: String, val icon: String? = null, val color: Int? = null) {
        /** What pressing it says, as the web's tooltip. */
        val said get() = if (tip.isEmpty()) label else "$label: $tip"
    }

    private val GIFT_TONES = listOf("bronze", "silver", "gold")

    /** The look of a badge; null for ids this app does not know, and for staff or a plan the account no longer has. */
    fun look(b: Badge, userId: String, flags: Long, plan: String): Look? = when (b.id) {
        "staff" -> StaffRanks.of(userId, flags)?.let { Look("staff-${it.key}", it.label, it.tip) }
        "plan" -> when (plan) {
            "pro" -> Look("plan-pro", t("support.plan.pro"), t("support.badge.proTip"))
            "supporter" -> Look("plan-supporter", t("support.plan.supporter"), t("support.badge.supporterTip"))
            else -> null
        }
        "og" -> Look("og", t("badges.og"), t("badges.ogTip"))
        "years" -> b.level.coerceAtLeast(1).let { n -> Look("years", t("badges.years", "count" to n), t("badges.yearsTip", "count" to n)) }
        "bug_hunter" -> b.level.coerceIn(1, 5).let { n -> Look("bug-$n", t("badges.bug.l$n"), t("badges.bugTip.l$n")) }
        "ideas" -> b.level.coerceIn(1, 5).let { n -> Look("ideas-$n", t("badges.ideas.l$n"), t("badges.ideasTip.l$n")) }
        "app_developer" -> Look("app-developer", t("badges.appDeveloper"), t("badges.appDeveloperTip"))
        "gift_giver" -> b.level.coerceIn(1, 3).let { n -> Look("gift-$n", t("badges.giftGiver"), t("badges.giftGiverTip.${GIFT_TONES[n - 1]}")) }
        CONTRIBUTOR -> Look("contributor", t("badges.contributor"), t("badges.contributorTip"))
        // a badge staff made says only its own name: what it is for is theirs to put in it
        else -> if (b.isEvent && b.name != null) Look(null, b.name, "", b.icon ?: "award", b.color) else null
    }

    /** A badge's name in a list (the designer, the staff panel), staff and plan included. */
    fun name(b: Badge, userId: String, flags: Long, plan: String): String =
        if (b.id == "staff" && StaffRanks.of(userId, flags) == null) t("badges.staff")
        else look(b, userId, flags, plan)?.label ?: b.id

    /** What a profile shows before its list arrives: the staff rank and the plan. */
    fun fallback(userId: String, flags: Long, plan: String): List<Badge> = buildList {
        if (StaffRanks.of(userId, flags) != null) add(Badge("staff"))
        if (plan == "supporter" || plan == "pro") add(Badge("plan"))
    }

    // ---- this account's own ----

    private val _mine = MutableStateFlow<List<Badge>?>(null)
    /** Every badge this account holds, in its order, the hidden ones marked; null until loaded. */
    val mine: StateFlow<List<Badge>?> = _mine
    val mineFailed = MutableStateFlow(false)

    suspend fun loadMine() {
        mineFailed.value = false
        try {
            _mine.value = parse((Nexa.api.get("/users/@me/badges") as JsonObject)["badges"]) ?: emptyList()
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            mineFailed.value = true
        }
    }

    /** Saves an order and hidden set, showing it at once and going back if the server refuses (the error is rethrown). */
    suspend fun save(list: List<Badge>) {
        val before = _mine.value
        _mine.value = list
        try {
            val body = buildJsonObject {
                put("order", JsonArray(list.map { JsonPrimitive(it.id) }))
                put("hidden", JsonArray(list.filter { it.hidden }.map { JsonPrimitive(it.id) }))
            }
            _mine.value = parse((Nexa.api.request("PUT", "/users/@me/badges", body) as JsonObject)["badges"]) ?: list
        } catch (e: Exception) {
            _mine.value = before
            throw e
        }
    }

    fun reset() {
        _mine.value = null
        mineFailed.value = false
    }

    // ---- staff ----

    /** An event badge staff made, with how many hold it. */
    data class Def(val id: String, val name: String, val icon: String, val color: Int, val holders: Int)

    fun parseDef(o: JsonObject): Def? {
        fun s(k: String) = (o[k] as? JsonPrimitive)?.contentOrNull
        return Def(s("id") ?: return null, s("name") ?: "", s("icon") ?: "award", (o["color"] as? JsonPrimitive)?.intOrNull ?: 0, (o["holders"] as? JsonPrimitive)?.intOrNull ?: 0)
    }

    /** The event badges, and how many hold Contributor. */
    suspend fun defs(): Pair<List<Def>, Int> {
        val res = Nexa.api.get("/moderation/badges") as JsonObject
        val list = (res["badges"] as? JsonArray)?.mapNotNull { (it as? JsonObject)?.let(::parseDef) } ?: emptyList()
        return list to ((res["contributors"] as? JsonPrimitive)?.intOrNull ?: 0)
    }

    private fun defBody(name: String, icon: String, color: Int) = buildJsonObject {
        put("name", name)
        put("icon", icon)
        put("color", color)
    }

    suspend fun createDef(name: String, icon: String, color: Int): Def? =
        parseDef(Nexa.api.request("POST", "/moderation/badges", defBody(name, icon, color)) as JsonObject)

    suspend fun editDef(id: String, name: String, icon: String, color: Int): Def? =
        parseDef(Nexa.api.request("PATCH", "/moderation/badges/$id", defBody(name, icon, color)) as JsonObject)

    suspend fun deleteDef(id: String) {
        Nexa.api.request("DELETE", "/moderation/badges/$id")
    }

    /** The badges staff gave someone: "contributor" and "event:<id>". */
    suspend fun granted(userId: String): List<String> =
        ((Nexa.api.get("/moderation/users/$userId/badges") as JsonObject)["granted"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull } ?: emptyList()

    suspend fun setGranted(userId: String, badges: List<String>) {
        Nexa.api.request("PUT", "/moderation/users/$userId/badges", buildJsonObject { put("badges", JsonArray(badges.map { JsonPrimitive(it) })) })
    }
}
