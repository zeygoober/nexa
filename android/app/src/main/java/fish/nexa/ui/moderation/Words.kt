package fish.nexa.ui.moderation

import fish.nexa.data.ModLogEntry
import fish.nexa.ui.Time
import fish.nexa.ui.nexa.fieldLabel
import fish.nexa.ui.nexa.formatSpan
import fish.nexa.ui.nexa.packFieldLabel
import fish.nexa.ui.t
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

// ---------------------------------------------------------------------------
// How moderation is put into words for the staff (lib/moderation.ts), so the
// panel describes an action the same way the web's does.
// ---------------------------------------------------------------------------

/** What staff did, as the panel lists it: "Muted", "Removed profile picture". */
private const val OWNER_SERVER = "owner_server_"
private val OWNER_SERVER_ACTIONS = listOf("begin", "view", "refused", "server_edit", "server_icon_clear", "server_banner_clear", "channel_create", "channel_edit", "channel_move", "channel_delete", "overwrite_set", "overwrite_remove", "overwrite_sync", "role_create", "role_edit", "role_delete", "member_role_add", "member_role_remove", "member_remove", "member_ban", "member_unban", "invite_revoke", "invites_revoke_all", "vanity_clear", "webhook_delete", "app_remove", "automod_create", "automod_edit", "automod_delete", "transfer").map { OWNER_SERVER + it }

private fun ownerServerLabel(action: String): String {
    val what = action.removePrefix(OWNER_SERVER)
    return if (what == "begin" || what == "view" || what == "refused") t("moderation.log.ownerServer.$what") else t("moderation.manage.a.$what")
}

/** What an owner's change to a server was, in a few words: "Edit Channel (general)". */
fun managedWhat(d: JsonObject): String {
    val what = (d["what"] as? JsonPrimitive)?.contentOrNull ?: return ""
    if (what.isEmpty()) return ""
    val shown = t("moderation.manage.a.$what")
    val target = (d["target"] as? JsonPrimitive)?.contentOrNull
    return if (target.isNullOrEmpty()) shown else "$shown ($target)"
}

fun actionLabel(action: String): String = if (action.startsWith(OWNER_SERVER)) ownerServerLabel(action) else when (action) {
    "server_manage" -> t("moderation.action.serverManage")
    "avatar_remove" -> t("moderation.action.avatarRemove")
    "field_reset" -> t("moderation.action.fieldReset")
    "server_reset" -> t("moderation.action.serverReset")
    "expression_takedown" -> t("moderation.action.expressionTakedown")
    "expression_relist" -> t("moderation.action.expressionRelist")
    "discovery_restore" -> t("moderation.action.discoveryRestore")
    "mute" -> t("moderation.action.mute")
    "ban" -> t("moderation.action.ban")
    "delete" -> t("moderation.action.delete")
    "unmute" -> t("moderation.action.unmute")
    "unban" -> t("moderation.action.unban")
    "announcement" -> t("moderation.action.announcement")
    "announcement_delete" -> t("moderation.action.announcementDelete")
    "report_review" -> t("reports.review")
    "message_remove" -> t("reports.messageRemoved")
    "rank_change" -> t("staff.log.rankChange")
    "badge_change" -> t("badges.log.change")
    "badge_create" -> t("badges.log.create")
    "badge_edit" -> t("badges.log.edit")
    "badge_delete" -> t("badges.log.delete")
    "account_view" -> t("moderation.log.accountView")
    "owner_join" -> t("moderation.log.ownerJoin")
    "server_preview" -> t("moderation.log.serverPreview")
    "gift_link_create" -> t("moderation.log.giftCreate")
    "gift_link_revoke" -> t("moderation.log.giftRevoke")
    "game_list" -> t("moderation.games.logList")
    "game_reject" -> t("moderation.games.logReject")
    "game_forget" -> t("moderation.games.logForget")
    "admin_grant" -> t("moderation.action.adminGrant")
    "admin_revoke" -> t("moderation.action.adminRevoke")
    "account_disable" -> t("moderation.action.accountDisable")
    "account_enable" -> t("moderation.action.accountEnable")
    "account_delete_profile" -> t("moderation.action.accountDeleteProfile")
    else -> action
}

/** How serious an action is, which decides its colour everywhere (the log's own list is longer than a case's). */
fun logSeverity(action: String): String = if (action == "server_manage" || action.startsWith(OWNER_SERVER)) "warn" else when (action) {
    "ban", "delete", "account_disable", "account_delete_profile" -> "danger"
    "mute", "field_reset", "avatar_remove", "server_reset", "expression_takedown", "admin_revoke" -> "warn"
    else -> "info"
}

fun logIcon(action: String): String = if (action == "server_manage" || action.startsWith(OWNER_SERVER)) "Settings2" else when (action) {
    "avatar_remove" -> "ImageOff"
    "field_reset" -> "Eraser"
    "server_reset" -> "ServerCog"
    "expression_takedown" -> "Sticker"
    "mute" -> "MessageSquareOff"
    "ban" -> "Ban"
    "delete", "account_delete_profile" -> "UserX"
    "unmute", "unban", "expression_relist", "discovery_restore" -> "LockOpen"
    "announcement", "announcement_delete" -> "Megaphone"
    "admin_grant", "rank_change" -> "ShieldPlus"
    "admin_revoke" -> "ShieldMinus"
    "badge_change", "badge_create", "badge_edit", "badge_delete" -> "Award"
    "game_list", "game_reject", "game_forget" -> "Gamepad2"
    "account_disable" -> "Lock"
    "account_view" -> "Fingerprint"
    "owner_join" -> "LogIn"
    "server_preview" -> "Eye"
    "gift_link_create", "gift_link_revoke" -> "Gift"
    else -> "KeyRound"
}

/** Log filters as the panel groups them. */
val LOG_GROUPS: Map<String, List<String>> = mapOf(
    "punishments" to listOf("message_remove", "avatar_remove", "field_reset", "server_reset", "expression_takedown", "mute", "ban", "delete", "account_delete_profile"),
    "lifts" to listOf("unmute", "unban", "expression_relist", "discovery_restore"),
    "announcements" to listOf("announcement", "announcement_delete"),
    "access" to listOf("rank_change", "admin_grant", "admin_revoke", "account_disable", "account_enable", "account_view", "owner_join", "server_preview", *OWNER_SERVER_ACTIONS.toTypedArray(), "gift_link_create", "gift_link_revoke"),
    "badges" to listOf("badge_change", "badge_create", "badge_edit", "badge_delete"),
    "games" to listOf("game_list", "game_reject", "game_forget"),
)

private fun JsonObject.str(k: String) = (this[k] as? JsonPrimitive)?.contentOrNull
private fun JsonObject.bool(k: String) = (this[k] as? JsonPrimitive)?.contentOrNull == "true"
private fun JsonObject.list(k: String) = (this[k] as? JsonArray) ?: JsonArray(emptyList())
private fun JsonObject.fields() = list("fields").mapNotNull { (it as? JsonPrimitive)?.contentOrNull }

/** What an expression_takedown took, in a line: "2 stickers from Cats", "Cats, deleted", "3 emoji in Night Owls". */
fun expressionSummary(d: JsonObject): String {
    if (d.bool("deleted")) return t("moderation.log.packDeleted", "name" to (d.str("pack_name") ?: ""))
    val parts = ArrayList<String>()
    val stickers = d.list("stickers").size
    val emojis = d.list("emojis").size
    if (stickers > 0) parts += t("moderation.log.stickersFrom", "count" to stickers, "name" to (d.str("pack_name") ?: ""))
    if (emojis > 0) parts += t("moderation.log.emojiIn", "count" to emojis, "name" to (d.str("server_name") ?: ""))
    val fields = d.fields().filter { it == "name" || it == "description" || it == "public" }
    if (fields.isNotEmpty()) parts += t("moderation.log.packFields", "fields" to fields.joinToString(", ") { packFieldLabel(it) }, "name" to (d.str("pack_name") ?: ""))
    return parts.joinToString("; ")
}

/** One sentence for a log entry's details, when it has any worth showing. */
fun logDetail(e: ModLogEntry): String {
    val d = e.details
    if (e.action.startsWith(OWNER_SERVER)) {
        val parts = listOf(
            d.str("server_name")?.takeIf { it.isNotEmpty() }?.let { "“$it”" } ?: "",
            if (e.action == OWNER_SERVER + "begin" || e.action == OWNER_SERVER + "refused") managedWhat(d) else d.str("target") ?: "",
            if (d.str("status") == "failed") t("moderation.log.ownerServer.failed") else "",
        )
        return parts.filter { it.isNotEmpty() }.joinToString(" · ")
    }
    return when (e.action) {
        "mute", "ban" -> when {
            d.bool("permanent") -> t("moderation.log.permanent")
            else -> d.str("duration_ms")?.toLongOrNull()?.let { t("moderation.log.forSpan", "span" to formatSpan(it)) } ?: ""
        }
        "field_reset" -> d.fields().joinToString(", ") { fieldLabel(it) }
        "server_reset" -> d.str("server_name")?.let { t("moderation.log.serverFields", "name" to it, "fields" to d.fields().joinToString(", ") { f -> fieldLabel(f) }) } ?: ""
        "avatar_remove" -> listOfNotNull(if (d.bool("avatar")) t("moderation.field.avatar") else null, if (d.bool("banner")) t("moderation.field.banner") else null).joinToString(", ")
        "expression_takedown" -> expressionSummary(d)
        "expression_relist" -> d.str("pack_name")?.let { "“$it”" } ?: ""
        "discovery_restore" -> d.str("server_name")?.let { "“$it”" } ?: ""
        "delete" -> (d["counts"] as? JsonObject)?.let { cnt ->
            t("moderation.log.deleteCounts", "messages" to (cnt.str("messages")?.toLongOrNull() ?: 0L), "servers" to (cnt.str("owned_servers")?.toLongOrNull() ?: 0L))
        } ?: ""
        "announcement", "announcement_delete" -> d.str("title")?.takeIf { it.isNotEmpty() }?.let { "“$it”" } ?: ""
        "account_delete_profile" -> t("moderation.log.messagesKept")
        "badge_change" -> {
            fun named(k: String) = d.list(k).mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.map { if (it == "contributor") t("badges.contributor") else it }.joinToString(", ")
            listOfNotNull(
                named("added").takeIf { it.isNotEmpty() }?.let { t("badges.log.gave", "names" to it) },
                named("removed").takeIf { it.isNotEmpty() }?.let { t("badges.log.took", "names" to it) },
            ).joinToString("; ")
        }
        "badge_create", "badge_delete" -> d.str("name")?.let { "“$it”" } ?: ""
        "owner_join", "server_preview" -> d.str("server_name")?.let { "“$it”" } ?: ""
        "gift_link_create" -> d.str("plan")?.let { fish.nexa.ui.support.planName(it) } ?: ""
        "game_list", "game_reject" -> d.str("process")?.let { "$it: “${d.str("name") ?: ""}”" } ?: ""
        "game_forget" -> d.str("process") ?: ""
        "badge_edit" -> {
            val from = d.str("from")
            val to = d.str("name")
            if (from != null && to != null && from != to) t("staff.log.fromTo", "from" to from, "to" to to) else to?.let { "“$it”" } ?: ""
        }
        "rank_change" -> {
            fun name(x: String?) = fish.nexa.data.StaffRank.entries.firstOrNull { it.key == x && it != fish.nexa.data.StaffRank.OWNER }?.label ?: t("staff.rankModal.none")
            t("staff.log.fromTo", "from" to name(d.str("from")), "to" to name(d.str("to")))
        }
        else -> ""
    }
}

private val CODE_BLOCK = Regex("```[^\\n]*\\n?([\\s\\S]*?)```")
private val LINE_MARKS = Regex("^[ \\t]*(?:#{1,6}|>+|-#|[-*+]|\\d+[.)])[ \\t]+", RegexOption.MULTILINE)
private val LINK = Regex("\\[([^\\]\\n]*)\\]\\([^)\\s]*\\)")
private val ANGLE_URL = Regex("<(https?:[^>\\s]+)>")
private val SPOILER = Regex("\\|\\|[\\s\\S]*?\\|\\|")
private val EMPHASIS = Regex("(\\*\\*\\*|\\*\\*|\\*|~~|`)(?=\\S)([\\s\\S]*?\\S)\\1")
private val UNDERSCORES = Regex("(^|[^\\p{L}\\p{N}_])(___|__|_)(?=\\S)([\\s\\S]*?\\S)\\2(?![\\p{L}\\p{N}_])")
private val SPACE = Regex("\\s+")

/**
 * A line of a thread as one line of plain words, for a list's preview: what
 * the formatting says rather than its marks, and what a line of only files
 * carries (previewText).
 */
fun previewText(body: String, files: Int = 0): String {
    val words = body
        .replace(CODE_BLOCK, " $1 ")
        .replace(LINE_MARKS, "")
        .replace(LINK, "$1")
        .replace(ANGLE_URL, "$1")
        .replace(SPOILER, "[" + t("moderation.case.spoiler") + "]")
        .replace(EMPHASIS, "$2")
        .replace(UNDERSCORES, "$1$3")
        .replace(SPACE, " ")
        .trim()
    if (words.isNotEmpty()) return words
    return if (files > 0) t("moderation.case.sentFiles", "count" to files) else ""
}

/** "Thursday, September 24, 2026 at 4:10 PM". */
fun fullDate(ms: Long) = Time.full(ms)
