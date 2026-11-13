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
    /** The Exclusive badges staff give by hand (store.ExclusiveHandBadges), in their order. */
    val EXCLUSIVE_HAND = listOf("legend", "mentor", "patron", "influencer", "artist")
    /** Every built-in badge staff give by hand (store.HandBadges). */
    val HAND_BADGES = listOf(CONTRIBUTOR) + EXCLUSIVE_HAND
    const val EVENT_PREFIX = "event:"

    /**
     * One badge: [level] is Bug Hunter's, Ideas' or Gift Giver's tier, or the years on Nexa.
     * The rest are an event badge's: its name, what it means (its tooltip), its Lucide [icon]
     * on a [shape] in [color] (to [color2], a gradient), drawn [dark] or white, or its uploaded [image].
     */
    data class Badge(
        val id: String,
        val level: Int = 0,
        val name: String? = null,
        val icon: String? = null,
        val color: Int? = null,
        val hidden: Boolean = false,
        val description: String? = null,
        val color2: Int? = null,
        val dark: Boolean = false,
        val shape: String = "circle",
        val image: String? = null,
        /** How many reports a Bug Hunter badge stands for. */
        val count: Int? = null,
        /** How many people hold this badge, at this level or above. */
        val exist: Int? = null,
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
            description = s("description"),
            color2 = i("color2"),
            dark = s("glyph") == "dark",
            shape = s("shape")?.takeIf { it in SHAPES } ?: "circle",
            image = s("image"),
            count = i("count"),
            exist = i("exist"),
        )
    }

    /** The outlines an event badge can take (web BADGE_SHAPES). */
    val SHAPES = listOf("circle", "rounded", "shield", "hexagon", "star", "diamond", "seal", "ribbon")

    /** The badges staff can give or take against what an account earned, with their top level (store.OverridableBadges). */
    val OVERRIDABLE = linkedMapOf("og" to 1, "years" to 50, "bug_hunter" to 5, "ideas" to 5, "app_developer" to 3, "gift_giver" to 3, "friendly" to 3, "host" to 3, "booster" to 1, "regular" to 4, "open_mic" to 3)

    /**
     * How a badge looks (web badgeLook): its picture's name under badges/ (written by
     * tools/badge_art.py, copied from the web and carried over the air), or for an
     * event badge staff made, its Lucide [icon] on its [shape] in [color] (to [color2]), or its
     * uploaded [image]; its name and what it means.
     */
    data class Look(
        val art: String?,
        val label: String,
        val tip: String,
        val icon: String? = null,
        val color: Int? = null,
        val color2: Int? = null,
        val dark: Boolean = false,
        val shape: String = "circle",
        val image: String? = null,
    ) {
        /** What pressing it says, as the web's tooltip. */
        val said get() = if (tip.isEmpty()) label else "$label: $tip"
    }

    private val GIFT_TONES = listOf("bronze", "silver", "gold")

    /** What each level needs (the server's tiers of the same names). */
    private val BOT_MAKER_TIERS = listOf(10, 100, 1000)
    private val FRIENDLY_TIERS = listOf(5, 25, 100)
    private val HOST_TIERS = listOf(10, 100, 1000)
    private val REGULAR_TIERS = listOf(7, 30, 100, 365)
    private val OPEN_MIC_TIERS = listOf(10, 100, 1000)

    /** "32nd" in English; other languages' sentences take the plain number. */
    private fun joinPlace(n: Int): String {
        if (fish.nexa.ui.I18n.tag != "en") return n.toString()
        val end = if (n % 100 in 11..13) "th" else when (n % 10) { 1 -> "st"; 2 -> "nd"; 3 -> "rd"; else -> "th" }
        return "$n$end"
    }

    /** The look of a badge; null for ids this app does not know, and for staff or a plan the account no longer has. */
    fun look(b: Badge, userId: String, flags: Long, plan: String): Look? {
        val look = builtInLook(b, userId, flags, plan)
        // the owner's own words for a built-in badge, in every language (store/badge_tips.go)
        return if (look != null && !b.description.isNullOrEmpty() && b.id != "staff" && !b.isEvent) look.copy(tip = b.description) else look
    }

    private fun builtInLook(b: Badge, userId: String, flags: Long, plan: String): Look? = when (b.id) {
        "staff" -> StaffRanks.of(userId, flags)?.let { Look("staff-${it.key}", it.label, it.tip) }
        "plan" -> when (plan) {
            "pro" -> Look("plan-pro", t("support.plan.pro"), t("support.badge.proTip"))
            "supporter" -> Look("plan-supporter", t("support.plan.supporter"), t("support.badge.supporterTip"))
            else -> null
        }
        "og" -> Look("og", t("badges.og"), if (b.count != null && b.count > 0) t("badges.ogTipPlace", "place" to joinPlace(b.count)) else t("badges.ogTip"))
        "years" -> b.level.coerceAtLeast(1).let { n -> Look("years-${n.coerceAtMost(10)}", t("badges.years", "count" to n), t("badges.yearsTip", "count" to n)) }
        "bug_hunter" -> b.level.coerceIn(1, 5).let { n -> Look("bug-$n", t("badges.bug.l$n"), if (b.count != null && b.count > 0) t("badges.bugTipCount", "count" to b.count) else t("badges.bugTip.l$n")) }
        "ideas" -> b.level.coerceIn(1, 5).let { n -> Look("ideas-$n", t("badges.ideas.l$n"), if (b.count != null && b.count > 0) t("badges.ideasTipCount", "count" to b.count) else t("badges.ideasTip.l$n")) }
        "app_developer" -> b.level.coerceIn(1, 3).let { n -> Look("bot-maker-$n", t("badges.botMaker.l$n"), if (b.count != null && b.count > 0) t("badges.appDeveloperTipCount", "count" to b.count) else t("badges.botMakerTip", "count" to BOT_MAKER_TIERS[n - 1])) }
        "gift_giver" -> b.level.coerceIn(1, 3).let { n -> Look("gift-$n", t("badges.giftGiver"), if (b.count != null && b.count > 0) t("badges.giftGiverTipCount", "count" to b.count) else t("badges.giftGiverTip.${GIFT_TONES[n - 1]}")) }
        // friends, hours and streaks stay private: these say the level reached, never the holder's number
        "friendly" -> b.level.coerceIn(1, 3).let { n -> Look("friendly-$n", t("badges.friendly.l$n"), t("badges.friendlyTip", "count" to FRIENDLY_TIERS[n - 1])) }
        "host" -> b.level.coerceIn(1, 3).let { n -> Look("host-$n", t("badges.host.l$n"), t("badges.hostTip", "count" to HOST_TIERS[n - 1])) }
        "booster" -> Look("booster", t("badges.booster"), t("badges.boosterTip"))
        "regular" -> b.level.coerceIn(1, 4).let { n -> Look("regular-$n", t("badges.regular.l$n"), t("badges.regularTip", "count" to REGULAR_TIERS[n - 1])) }
        "open_mic" -> b.level.coerceIn(1, 3).let { n -> Look("open-mic-$n", t("badges.openMic.l$n"), t("badges.openMicTip", "count" to OPEN_MIC_TIERS[n - 1])) }
        CONTRIBUTOR -> Look("contributor", t("badges.contributor"), t("badges.contributorTip"))
        "legend", "mentor", "patron", "influencer", "artist" -> Look(b.id, t("badges.hand.${b.id}"), t("badges.hand.${b.id}Tip"))
        // the Secret badges: their tooltips say nothing of how they are found
        "hopeful", "curious", "nosy", "lucky" -> Look(b.id, t("badges.secret.${b.id}"), t("badges.secret.${b.id}Tip"))
        // a badge staff made says what they wrote for it
        else -> if (b.isEvent && b.name != null) Look(null, b.name, b.description ?: "", b.icon ?: "award", b.color, b.color2, b.dark, b.shape, b.image) else null
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

    /** An event badge staff made, with how many hold it; [color2] -1 for one colour, [imageId] "0" for none. */
    data class Def(
        val id: String,
        val name: String,
        val icon: String,
        val color: Int,
        val holders: Int,
        val description: String = "",
        val color2: Int = -1,
        val dark: Boolean = false,
        val shape: String = "circle",
        val imageId: String = "0",
        /** Where the Badge Index puts it: "exclusive", "special" or "hidden". */
        val indexKind: String = "exclusive",
    ) {
        /** As a badge someone holds, for drawing it in the panel (web defBadge). */
        val badge get() = Badge(
            id = EVENT_PREFIX + id, name = name, icon = icon, color = color, description = description,
            color2 = color2.takeIf { it >= 0 }, dark = dark, shape = shape,
            image = imageId.takeIf { it != "0" && it.isNotEmpty() }?.let { "/media/badge/$it" },
        )
    }

    fun parseDef(o: JsonObject): Def? {
        fun s(k: String) = (o[k] as? JsonPrimitive)?.contentOrNull
        fun i(k: String) = (o[k] as? JsonPrimitive)?.intOrNull
        return Def(
            s("id") ?: return null, s("name") ?: "", s("icon") ?: "award", i("color") ?: 0, i("holders") ?: 0,
            description = s("description") ?: "", color2 = i("color2") ?: -1, dark = s("glyph") == "dark",
            shape = s("shape")?.takeIf { it in SHAPES } ?: "circle", imageId = s("image_id") ?: "0",
            indexKind = s("index_kind") ?: "exclusive",
        )
    }

    /** The event badges, and how many hold each badge given by hand. */
    suspend fun defs(): Pair<List<Def>, Map<String, Int>> {
        val res = Nexa.api.get("/moderation/badges") as JsonObject
        val list = (res["badges"] as? JsonArray)?.mapNotNull { (it as? JsonObject)?.let(::parseDef) } ?: emptyList()
        val hand = (res["hand"] as? JsonObject)?.mapNotNull { (k, v) -> (v as? JsonPrimitive)?.intOrNull?.let { k to it } }?.toMap()
            ?: mapOf(CONTRIBUTOR to ((res["contributors"] as? JsonPrimitive)?.intOrNull ?: 0))
        return list to hand
    }

    /** The owner's words for one badge by its index key ("" goes back to the built-in text). */
    suspend fun setTip(key: String, description: String) {
        Nexa.api.request("PUT", "/moderation/badges/descriptions", buildJsonObject { put("key", key); put("description", description) })
    }

    private fun defBody(d: Def) = buildJsonObject {
        put("name", d.name)
        put("description", d.description)
        put("icon", d.icon)
        put("color", d.color)
        put("color2", d.color2)
        put("glyph", if (d.dark) "dark" else "light")
        put("shape", d.shape)
        put("image_id", d.imageId)
        put("index_kind", d.indexKind)
    }

    suspend fun createDef(d: Def): Def? =
        parseDef(Nexa.api.request("POST", "/moderation/badges", defBody(d)) as JsonObject)

    suspend fun editDef(d: Def): Def? =
        parseDef(Nexa.api.request("PATCH", "/moderation/badges/${d.id}", defBody(d)) as JsonObject)

    suspend fun deleteDef(id: String) {
        Nexa.api.request("DELETE", "/moderation/badges/$id")
    }

    /** Uploads a picture for an event badge, cut square on the server at [crop] ("?x=..&y=..&w=..&h=.." or ""); answers its id. */
    suspend fun uploadImage(bytes: ByteArray, type: String, crop: String): String? =
        ((fish.nexa.ui.server.ServerApi.upload("POST", "/moderation/badges/image$crop", bytes, type) as? JsonObject)?.get("id") as? JsonPrimitive)?.contentOrNull

    /** Someone holding a badge: [source] "earned", "given" or "account"; [overridden] when staff set it by hand. */
    data class Holder(val userId: String, val level: Int, val source: String, val overridden: Boolean, val user: JsonObject?) {
        private fun u(k: String) = (user?.get(k) as? JsonPrimitive)?.contentOrNull
        val username get() = u("username")
        val displayName get() = u("display_name")?.takeIf { it.isNotBlank() } ?: u("username") ?: userId
        val avatarId get() = u("avatar_id")
    }

    data class Holders(val holders: List<Holder>, val taken: List<Holder>, val total: Int)

    private fun parseHolders(el: JsonElement?) = (el as? JsonArray)?.mapNotNull { e ->
        val o = e as? JsonObject ?: return@mapNotNull null
        Holder(
            (o["user_id"] as? JsonPrimitive)?.contentOrNull ?: return@mapNotNull null,
            (o["level"] as? JsonPrimitive)?.intOrNull ?: 0,
            (o["source"] as? JsonPrimitive)?.contentOrNull ?: "earned",
            (o["overridden"] as? JsonPrimitive)?.booleanOrNull == true,
            o["user"] as? JsonObject,
        )
    } ?: emptyList()

    /** Who holds a badge ("og", "contributor", "event:<id>", ...), and who staff took it from. */
    suspend fun holders(badge: String, offset: Int, limit: Int): Holders {
        val o = Nexa.api.get("/moderation/badges/${java.net.URLEncoder.encode(badge, "UTF-8")}/holders?offset=$offset&limit=$limit") as JsonObject
        return Holders(parseHolders(o["holders"]), parseHolders(o["taken"]), (o["total"] as? JsonPrimitive)?.intOrNull ?: 0)
    }

    /** Gives a badge (at a level, for tiered ones), takes it ("take"), or lets an earned one follow the account again ("auto"). */
    suspend fun setHolder(badge: String, userId: String, state: String, level: Int = 1) {
        Nexa.api.request("PUT", "/moderation/badges/${java.net.URLEncoder.encode(badge, "UTF-8")}/holders/$userId", buildJsonObject {
            put("state", state)
            put("level", level)
        })
    }

    /** Someone's badges as staff set them: given by hand, earned ones set by hand (0 taken, else the level given), and what they earned (0 none). */
    data class UserBadges(val granted: List<String>, val overrides: Map<String, Int>, val earned: Map<String, Int>)

    private fun intMap(el: JsonElement?) = (el as? JsonObject)?.mapNotNull { (k, v) -> (v as? JsonPrimitive)?.intOrNull?.let { k to it } }?.toMap() ?: emptyMap()

    suspend fun userBadges(userId: String): UserBadges {
        val o = Nexa.api.get("/moderation/users/$userId/badges") as JsonObject
        return UserBadges(
            (o["granted"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull } ?: emptyList(),
            intMap(o["overrides"]),
            intMap(o["earned"]),
        )
    }

    suspend fun setGranted(userId: String, badges: List<String>, overrides: Map<String, Int>? = null) {
        Nexa.api.request("PUT", "/moderation/users/$userId/badges", buildJsonObject {
            put("badges", JsonArray(badges.map { JsonPrimitive(it) }))
            if (overrides != null) put("overrides", JsonObject(overrides.mapValues { JsonPrimitive(it.value) }))
        })
    }
}
