package fish.nexa.data

import androidx.compose.runtime.mutableStateMapOf
import fish.nexa.Nexa
import fish.nexa.crypto.Bytes
import fish.nexa.net.ApiError
import fish.nexa.net.Channel
import fish.nexa.net.InvitePreview
import fish.nexa.net.PublicUser
import fish.nexa.net.ServerView
import fish.nexa.net.wireJson
import fish.nexa.ui.nav.Route
import fish.nexa.ui.profile.ProfileLinks
import fish.nexa.ui.t
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/**
 * lib/inviteLink.ts and lib/joinInvite.ts: invite links, which for a server
 * (and a group) may carry the current key in the part after `#` that never
 * reaches the server, and joining through one.
 */
object Invites {
    private val ageConfirmed = mutableSetOf<String>()
    private var accountGeneration = 0
    fun clear() { accountGeneration++; ageConfirmed.clear(); byCode.clear(); synchronized(inflight) { inflight.clear() } }
    private fun ageKey(code: String) = "${Nexa.users.meId}:$code"
    fun confirmAge(code: String) { ageConfirmed += ageKey(code) }
    private fun ageQuery(code: String) = if (ageKey(code) in ageConfirmed) "?nsfw=1" else ""
    data class Key(val epoch: Int, val root: ByteArray)

    private fun toUrl(b: String) = b.replace('+', '-').replace('/', '_').trimEnd('=')

    fun encodeKey(k: Key) = "k=${k.epoch}.${toUrl(Bytes.b64(k.root))}"

    fun parseFragment(hash: String): Key? {
        val m = Regex("(?:^|[#&])k=(\\d+)\\.([A-Za-z0-9_-]{40,50})(?:&|$)").find(hash) ?: return null
        return try {
            val s = m.groupValues[2].replace('-', '+').replace('_', '/')
            val root = Bytes.unb64(s + "=".repeat((4 - s.length % 4) % 4))
            if (root.size != 32) null else Key(m.groupValues[1].toInt(), root)
        } catch (_: Exception) {
            null
        }
    }

    fun url(code: String, key: Key?): String {
        val base = "${ProfileLinks.origin()}/invite/$code"
        return if (key != null) "$base#${encodeKey(key)}" else base
    }

    /** The current key of a conversation (a server or a group), for a link to carry. */
    fun keyOf(conv: String): Key? = Nexa.keys.ring.latest(conv)?.let { Key(it.first, it.second) }

    /** CreateServerModal's join field: a link or just the code, the key part kept. */
    fun parseCode(text: String): Pair<String, String>? {
        val trimmed = text.trim()
        val m = Regex("(?:invite/)?([A-Za-z0-9_-]{4,32})(?:[#?].*)?$").find(trimmed) ?: return null
        val code = m.groupValues[1]
        val hash = trimmed.substringAfter('#', "")
        return code to (if (hash.isNotEmpty()) "#$hash" else "")
    }

    private val TEMPLATE_CODE_RE = Regex("^[A-Za-z0-9_-]{2,32}$")

    sealed interface ParsedTemplate {
        data class Ok(val code: String) : ParsedTemplate
        data class Err(val reason: String) : ParsedTemplate
    }

    /** lib/discordTemplate.ts: parses a discord.new link, discord.com/template link, or bare code. */
    fun parseDiscordTemplate(input: String): ParsedTemplate {
        val s = input.trim()
        if (s.isEmpty()) return ParsedTemplate.Err("empty")
        if (!s.contains("://") && !s.contains('/') && !s.contains('.')) {
            return if (TEMPLATE_CODE_RE.matches(s)) ParsedTemplate.Ok(s) else ParsedTemplate.Err("invalid")
        }
        val withScheme = if (s.contains("://")) s else "https://$s"
        val uri = try { java.net.URI(withScheme) } catch (_: Exception) { return ParsedTemplate.Err("invalid") }
        var host = (uri.host ?: return ParsedTemplate.Err("invalid")).lowercase()
        for (prefix in listOf("www.", "ptb.", "canary.")) {
            if (host.startsWith(prefix)) host = host.removePrefix(prefix)
        }
        val parts = (uri.rawPath ?: "").split('/').filter { it.isNotEmpty() }
        if (host == "discord.gg") return ParsedTemplate.Err("invite")
        if (host == "discord.new") {
            val code = parts.firstOrNull()
            return if (parts.size == 1 && code != null && TEMPLATE_CODE_RE.matches(code)) ParsedTemplate.Ok(code) else ParsedTemplate.Err("invalid")
        }
        if (host == "discord.com" || host == "discordapp.com") {
            if (parts.firstOrNull()?.equals("invite", ignoreCase = true) == true) return ParsedTemplate.Err("invite")
            if (parts.size == 2 && (parts[0].equals("template", ignoreCase = true) || parts[0].equals("templates", ignoreCase = true)) && TEMPLATE_CODE_RE.matches(parts[1])) {
                return ParsedTemplate.Ok(parts[1])
            }
            if (parts.size == 4 && parts[0].equals("api", ignoreCase = true) && parts[1].equals("guilds", ignoreCase = true) && parts[2].equals("templates", ignoreCase = true) && TEMPLATE_CODE_RE.matches(parts[3])) {
                return ParsedTemplate.Ok(parts[3])
            }
            if (parts.size == 5 && parts[0].equals("api", ignoreCase = true) && parts[2].equals("guilds", ignoreCase = true) && parts[3].equals("templates", ignoreCase = true) && TEMPLATE_CODE_RE.matches(parts[4])) {
                return ParsedTemplate.Ok(parts[4])
            }
        }
        return ParsedTemplate.Err("invalid")
    }

    fun isDiscordTemplateUrl(input: String): Boolean {
        val s = input.trim()
        if (!s.contains('/') && !s.contains('.')) return false
        return parseDiscordTemplate(s) is ParsedTemplate.Ok
    }

    private val NEXA_TEMPLATE_CODE_RE = Regex("^[A-Za-z0-9]{8,32}$")

    /** Whether a host is this Nexa's: the one the app talks to, or nexa.pm. */
    private fun ownHost(host: String): Boolean {
        val h = host.lowercase()
        val mine = runCatching { java.net.URI(Nexa.api.baseUrl).host?.lowercase() }.getOrNull()
        return h == mine || h == "nexa.pm" || h.endsWith(".nexa.pm")
    }

    /** lib/nexaTemplate.ts: a Nexa server template link (https://nexa.pm/template/<code>) or a bare code. */
    fun parseNexaTemplate(input: String): ParsedTemplate {
        val s = input.trim()
        if (s.isEmpty()) return ParsedTemplate.Err("empty")
        if (!s.contains('/') && !s.contains('.')) {
            return if (NEXA_TEMPLATE_CODE_RE.matches(s)) ParsedTemplate.Ok(s) else ParsedTemplate.Err("invalid")
        }
        val uri = try { java.net.URI(if (s.contains("://")) s else "https://$s") } catch (_: Exception) { return ParsedTemplate.Err("invalid") }
        val host = uri.host ?: return ParsedTemplate.Err("invalid")
        val parts = (uri.rawPath ?: "").split('/').filter { it.isNotEmpty() }
        if (!ownHost(host) || parts.size != 2 || parts[0] != "template" || !NEXA_TEMPLATE_CODE_RE.matches(parts[1])) return ParsedTemplate.Err("invalid")
        return ParsedTemplate.Ok(parts[1])
    }

    /** Whether input is a Nexa template link (rather than a bare code). */
    fun isNexaTemplateUrl(input: String): Boolean {
        val s = input.trim()
        return (s.contains('/') || s.contains('.')) && parseNexaTemplate(s) is ParsedTemplate.Ok
    }

    data class TemplateChannelPreview(
        val id: Int,
        val type: String,
        val name: String,
        val parentIndex: Int,
        val isPrivate: Boolean,
    )

    data class TemplateRolePreview(
        val name: String,
        val color: Int,
    )

    data class TemplatePreview(
        val code: String,
        val name: String,
        val description: String,
        val serverName: String,
        val channels: List<TemplateChannelPreview>,
        val roles: List<TemplateRolePreview>,
    )

    /** source is "discord" or "nexa"; both previews have the same shape. */
    suspend fun previewTemplate(source: String, code: String): TemplatePreview {
        val res = Nexa.api.get("/servers/templates/$source/$code") as JsonObject
        fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.takeIf { it.isString }?.content ?: ""
        fun JsonObject.n(k: String) = (this[k] as? JsonPrimitive)?.content?.toIntOrNull() ?: 0
        fun JsonObject.b(k: String) = (this[k] as? JsonPrimitive)?.content == "true"
        val chans = ((res["channels"] as? JsonArray) ?: JsonArray(emptyList())).mapNotNull { el ->
            val o = el as? JsonObject ?: return@mapNotNull null
            TemplateChannelPreview(o.n("id"), o.s("type"), o.s("name"), o.n("parent_index"), o.b("private"))
        }
        val roles = ((res["roles"] as? JsonArray) ?: JsonArray(emptyList())).mapNotNull { el ->
            val o = el as? JsonObject ?: return@mapNotNull null
            TemplateRolePreview(o.s("name"), o.n("color"))
        }
        return TemplatePreview(
            code = res.s("code"),
            name = res.s("name"),
            description = res.s("description"),
            serverName = res.s("server_name"),
            channels = chans,
            roles = roles,
        )
    }

    data class Preview(
        val code: String,
        /** The server it leads to, or the group (see [group]). */
        val id: String?,
        val name: String,
        val iconId: String?,
        val bannerId: String?,
        val description: String,
        val members: Int?,
        val online: Int?,
        val inviter: PublicUser?,
        val group: Boolean,
        val channelName: String?,
        val ageGate: Boolean = false,
    )

    suspend fun preview(code: String): Preview {
        val res = Nexa.api.get("/invites/$code${ageQuery(code)}") as JsonObject
        val inviter = res["inviter"]?.let { runCatching { wireJson.decodeFromJsonElement(PublicUser.serializer(), it) }.getOrNull() }
        fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.takeIf { it.isString }?.content
        fun JsonObject.n(k: String) = (this[k] as? JsonPrimitive)?.content?.toIntOrNull()
        if ((res["age_gate"] as? JsonPrimitive)?.content == "true") return Preview(code, null, t("nsfw.warningTitle"), null, null, "", null, null, null, false, null, ageGate = true)
        val server = res["server"] as? JsonObject
        if (server != null) {
            return Preview(
                code, server.s("id"), server.s("name") ?: "", server.s("icon_id"), server.s("banner_id"), server.s("description") ?: "",
                server.n("member_count"), server.n("online_count"), inviter, group = false,
                channelName = (res["channel"] as? JsonObject)?.s("name"),
            )
        }
        val group = res["group"] as? JsonObject ?: throw IllegalStateException("invite")
        return Preview(code, group.s("id"), group.s("name") ?: "", group.s("icon_id"), null, "", group.n("member_count"), null, inviter, group = true, channelName = null)
    }

    /** acceptInvite: joins, keeps the link's key and grants it to this account, and says where to go. */
    suspend fun accept(code: String, fragment: String): Route {
        val res = Nexa.auth.guarded("join") { captcha -> Nexa.api.request("POST", "/invites/$code${ageQuery(code)}", captcha = captcha) } as JsonObject
        val users = (res["users"] as? JsonArray)?.let { runCatching { wireJson.decodeFromJsonElement(ListSerializer(PublicUser.serializer()), it) }.getOrNull() } ?: emptyList()
        Nexa.users.upsert(users)
        res["channel"]?.let { el ->
            val ch = wireJson.decodeFromJsonElement(Channel.serializer(), el)
            Nexa.channels.upsert(listOf(ch))
            parseFragment(fragment)?.let { Nexa.keys.adopt(ch.id, it.epoch, it.root) }
            return Route.Dm(ch.id)
        }
        val view = res["server"]?.let { wireJson.decodeFromJsonElement(ServerView.serializer(), it) } ?: throw IllegalStateException(t("invite.couldNotAcceptTheInvite"))
        Nexa.channels.upsert(view.channels)
        Nexa.servers.put(view)
        parseFragment(fragment)?.let { Nexa.keys.adopt(view.id, it.epoch, it.root) }
        val first = view.channels.filter { it.type == "text" }.minByOrNull { it.position }
        return Route.Server(view.id, first?.id)
    }

    /** An invite just made: its code, when it runs out (null for never) and how many times it may be used (0 for no limit). */
    data class Made(val code: String, val expiresAt: Long?, val maxUses: Int)

    /** A new link for a server channel or a group; max age in seconds (0 never), max uses (0 unlimited). */
    suspend fun create(channelId: String, maxAge: Int, maxUses: Int): Made {
        val res = Nexa.api.request("POST", "/channels/$channelId/invites", buildJsonObject {
            put("max_age", maxAge)
            put("max_uses", maxUses)
        }) as JsonObject
        return Made(
            (res["code"] as JsonPrimitive).content,
            (res["expires_at"] as? JsonPrimitive)?.content?.toLongOrNull(),
            (res["max_uses"] as? JsonPrimitive)?.content?.toIntOrNull() ?: 0,
        )
    }

    // -----------------------------------------------------------------------
    // lib/invitePreviews.ts: what an invite in a message leads to, kept a few
    // minutes, and asked again a little after a failure

    sealed interface Entry {
        data object Loading : Entry
        data class Ready(val preview: InvitePreview, val at: Long) : Entry
        data class Invalid(val at: Long) : Entry
        data class Failed(val at: Long) : Entry
    }

    private const val FRESH_MS = 5 * 60_000L
    private const val RETRY_MS = 30_000L

    val byCode = mutableStateMapOf<String, Entry>()
    private val inflight = HashSet<String>()

    private fun due(e: Entry?, now: Long): Boolean = when (e) {
        null -> true
        Entry.Loading -> false
        is Entry.Ready -> now - e.at > FRESH_MS
        is Entry.Invalid -> now - e.at > FRESH_MS
        is Entry.Failed -> now - e.at > RETRY_MS
    }

    fun load(code: String, force: Boolean = false) {
        synchronized(inflight) { if (code in inflight) return }
        val generation = accountGeneration
        val cur = byCode[code]
        if (!force && !due(cur, System.currentTimeMillis())) return
        if (cur !is Entry.Ready) byCode[code] = Entry.Loading
        synchronized(inflight) { inflight += code }
        Nexa.scope.launch {
            try {
                val p = Nexa.api.request("GET", "/invites/${java.net.URLEncoder.encode(code, "UTF-8")}${ageQuery(code)}", InvitePreview.serializer())
                if (generation == accountGeneration) byCode[code] = Entry.Ready(p, System.currentTimeMillis())
            } catch (e: Exception) {
                if (generation == accountGeneration) byCode[code] = when {
                    e is ApiError && e.status == 404 -> Entry.Invalid(System.currentTimeMillis())
                    cur is Entry.Ready -> cur.copy(at = System.currentTimeMillis())
                    else -> Entry.Failed(System.currentTimeMillis())
                }
            } finally {
                if (generation == accountGeneration) synchronized(inflight) { inflight -= code }
            }
        }
    }
}
