package fish.nexa.ui.moderation

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import fish.nexa.Nexa
import fish.nexa.data.ManageChannel
import fish.nexa.data.ManageMember
import fish.nexa.data.ManageRole
import fish.nexa.data.ManageRule
import fish.nexa.data.ManageView
import fish.nexa.data.ModServer
import fish.nexa.data.Moderation
import fish.nexa.data.Perms
import fish.nexa.net.PublicUser
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.ToggleLine
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.server.OverwriteGrid
import fish.nexa.ui.server.RoleGrid
import fish.nexa.ui.server.SelectField
import fish.nexa.ui.settings.openColourPicker
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.add
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.addJsonObject

private const val DAY = 24L * 3600 * 1000
private val CHANNEL_KINDS = listOf("text", "voice", "category", "announcement", "forum", "media", "link")

private data class ManageTarget(val id: String, val type: String, val name: String, val allow: String, val deny: String)

private sealed class ManageField {
    data class Text(val key: String, val label: String, val max: Int, val required: Boolean = false, val area: Boolean = false, val shownWhen: ((Map<String, Any>) -> Boolean)? = null) : ManageField()
    data class Number(val key: String, val label: String, val min: Int, val max: Int) : ManageField()
    data class Select(val key: String, val label: String, val options: List<Pair<String, String>>) : ManageField()
    data class Toggle(val key: String, val label: String) : ManageField()
    data class Colour(val key: String, val label: String) : ManageField()
    data class Grid(val key: String, val label: String) : ManageField()
    data class Over(val channelType: String, val targets: List<ManageTarget>) : ManageField()
}

private data class ManageRequest(
    val action: String? = null,
    val channelId: String? = null,
    val roleId: String? = null,
    val userId: String? = null,
    val targetId: String? = null,
    val webhookId: String? = null,
    val appId: String? = null,
    val ruleId: String? = null,
    val code: String? = null,
    val data: JsonObject? = null,
)

/** One owner's action, as the sheet asks for it: what it is, what it needs and who will hear of it. */
private class ManageAsk(
    val action: String,
    val sees: List<String>,
    val target: String? = null,
    val confirm: String? = null,
    val danger: Boolean = false,
    val fields: List<ManageField> = emptyList(),
    val initial: Map<String, Any> = emptyMap(),
    val request: (Map<String, Any>) -> ManageRequest = { ManageRequest() },
)

private fun personOf(u: PublicUser) = u.displayName.ifEmpty { u.username }
private fun hexOf(color: Long) = "#" + color.toString(16).padStart(6, '0')
private fun colourNumber(hex: Any?) = hex.toString().removePrefix("#").toLongOrNull(16) ?: 0L
private fun permOf(v: Any?) = v.toString().toLongOrNull() ?: 0L
private fun label(key: String) = t("moderation.manage.$key")
private fun wordsOf(v: Any?) = v.toString().split("\n").map { it.trim() }.filter { it.isNotEmpty() }

/**
 * Looking after a server in its owner's absence, for Nexa's owner only (web
 * ManageServerModal). Nothing here reads a message or touches a key. Every change
 * takes a reason, is written to both logs under this account, and says first who
 * will hear of it.
 */
fun openManageServer(server: ModServer, onChanged: () -> Unit) {
    var view by mutableStateOf<ManageView?>(null)
    var failed by mutableStateOf(false)
    var tab by mutableStateOf("overview")
    var attempt by mutableIntStateOf(0)
    var changed = false
    var refresh by mutableIntStateOf(0)

    openModSheet(
        "manage-server",
        top = { close ->
            val c = Nx.colors
            val v = view
            Column {
                ModSheetTop(t("moderation.manage.title", "name" to (v?.server?.name ?: server.name)), close, leading = { ServerIcon(server.id, v?.server?.iconId ?: server.iconId, server.name, 40.dp) }) {
                    v?.owner?.let { owner ->
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            Txt(t("moderation.manage.owner", "name" to owner.username), Modifier.weight(1f, fill = false), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f)), color = c.textMuted, maxLines = 1)
                            if (v.ownerGone) ManageTag(label("ownerGone"), danger = true)
                        }
                    }
                }
                if (v?.server != null) {
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .drawBehind { drawRect(c.borderSubtle, Offset(0f, size.height - 1.dp.toPx()), Size(size.width, 1.dp.toPx())) }
                            .horizontalScroll(rememberScrollState())
                            .padding(horizontal = 18.dp, vertical = 10.dp),
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        for ((id, text) in listOf(
                            "overview" to t("server.channelSettings.overview"),
                            "channels" to t("moderation.preview.channels"),
                            "roles" to t("moderation.preview.roles"),
                            "members" to t("moderation.preview.members"),
                            "bans" to t("server.bans.bans"),
                            "invites" to t("server.invites.invites"),
                            "apps" to t("apps.menu.apps"),
                            "automod" to t("automod.title"),
                        )) ModChip(text, tab == id, { tab = id }, small = true)
                    }
                }
            }
        },
        foot = { close ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) { NxButton(t("moderation.alts.close"), close, kind = ButtonKind.GHOST) }
        },
    ) { close ->
        // one height, so that changing tab does not move the frame under the finger
        val tall = (LocalConfiguration.current.screenHeightDp * 0.62f).dp
        DisposableEffect(Unit) { onDispose { if (changed) onChanged() } }
        LaunchedEffect(attempt) {
            failed = false
            try {
                view = Moderation.serverManage(server.id)
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                failed = true
            }
        }
        Column(Modifier.fillMaxWidth().heightIn(min = tall), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            val v = view
            when {
                failed -> EmptyState("Settings2", null, t("moderation.loadFailed")) { NxButton(t("moderation.retry"), { attempt++ }, kind = ButtonKind.SECONDARY) }
                v?.server == null -> LoadingRow()
                else -> ManageBody(v, server.id, tab, refresh) { ask ->
                    openManageAction(server.id, ask) { next ->
                        changed = true
                        if (next.gone) {
                            Nexa.ui.toast(label("gone"), "info")
                            close()
                        } else {
                            view = next
                            refresh++
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ColumnScope.ManageBody(view: ManageView, serverId: String, tab: String, refresh: Int, say: (ManageAsk) -> Unit) {
    val c = Nx.colors
    val srv = view.server ?: return
    val ownerName = view.owner?.let { personOf(it) } ?: ""
    val everyoneId = srv.id
    val seesOwner = if (view.ownerDeleted) label("seesNoOwner") else t("moderation.manage.seesOwner", "name" to ownerName)
    fun sees(vararg extra: String) = listOf(seesOwner) + extra.toList()
    val save = t("server.invites.vanitySave")

    val categories = view.channels.filter { it.type == "category" }.sortedBy { it.position }
    val categoryOptions = listOf("" to label("noCategory")) + categories.map { it.id to it.name }
    val roleOrder = view.roles.sortedByDescending { it.position }
    val assignable = roleOrder.filter { it.id != everyoneId && it.managedBy == null }
    fun roleName(r: ManageRole) = if (r.id == everyoneId) "@everyone" else r.name
    fun plain(action: String, target: String? = null, seen: List<String> = sees(), request: (Map<String, Any>) -> ManageRequest = { ManageRequest() }) =
        say(ManageAsk(action, seen, target = target, danger = true, request = request))

    val roleFields = listOf(
        ManageField.Text("name", t("server.roles.roleName"), 100, required = true),
        ManageField.Colour("color", t("badges.staff.colour")),
        ManageField.Toggle("hoist", label("hoist")),
        ManageField.Toggle("mentionable", label("mentionable")),
        ManageField.Grid("permissions", t("server.channelSettings.permissions")),
    )
    fun roleData(v: Map<String, Any>) = buildJsonObject {
        put("name", v["name"].toString().trim())
        put("color", colourNumber(v["color"]))
        put("hoist", v["hoist"] == true)
        put("mentionable", v["mentionable"] == true)
        put("permissions", v["permissions"].toString())
    }

    @Composable
    fun channelRow(ch: ManageChannel, first: Boolean) {
        val parent = ch.parentId?.let { id -> view.channels.firstOrNull { it.id == id } }
        val hasTopic = !ch.private && ch.type != "category"
        val hasNsfw = ch.type != "category" && ch.type != "link"
        val hasSlowmode = ch.type in listOf("text", "announcement", "forum", "media")
        val hasLimit = ch.type == "voice"
        val targets = roleOrder.map { r ->
            val o = ch.overwrites.firstOrNull { it.targetId == r.id }
            ManageTarget(r.id, "role", roleName(r), o?.allow ?: "0", o?.deny ?: "0")
        } + ch.overwrites.filter { it.targetType == "member" }.map { o ->
            ManageTarget(o.targetId, "member", t("moderation.manage.memberEntry", "id" to o.targetId), o.allow, o.deny)
        }
        ManageRow(first, inset = true, lead = {
            if (ch.private) Icon("Lock", c.textMuted, size = 16.dp)
            else fish.nexa.ui.shell.ChannelGlyph(fish.nexa.net.Channel(id = ch.id, type = ch.type, name = ch.name), 18.dp, c.textMuted, cut = c.bgInput)
        }, name = ch.name, tags = {
            if (ch.private) ManageTag(label("private"))
            if (ch.nsfw) ManageTag(t("nsfw.label"))
        }, actions = {
            RowAction(label("edit")) {
                say(ManageAsk(
                    "channel_edit", sees(), target = ch.name, confirm = save,
                    initial = mapOf("name" to ch.name, "topic" to ch.topic, "nsfw" to ch.nsfw, "slowmode_seconds" to ch.slowmodeSeconds, "user_limit" to ch.userLimit),
                    fields = buildList {
                        add(ManageField.Text("name", t("modals.createChannel.channelName"), 100, required = true))
                        if (hasTopic) add(ManageField.Text("topic", label("topic"), 1024, area = true))
                        if (hasNsfw) add(ManageField.Toggle("nsfw", label("channelNsfw")))
                        if (hasSlowmode) add(ManageField.Number("slowmode_seconds", label("slowmode"), 0, 21600))
                        if (hasLimit) add(ManageField.Number("user_limit", label("userLimit"), 0, 99))
                    },
                    request = { v ->
                        ManageRequest(channelId = ch.id, data = buildJsonObject {
                            put("name", v["name"].toString().trim())
                            if (hasTopic) put("topic", v["topic"].toString())
                            if (hasNsfw) put("nsfw", v["nsfw"] == true)
                            if (hasSlowmode) put("slowmode_seconds", v["slowmode_seconds"] as? Int ?: 0)
                            if (hasLimit) put("user_limit", v["user_limit"] as? Int ?: 0)
                        })
                    },
                ))
            }
            if (ch.type != "category") RowAction(label("move")) {
                say(ManageAsk(
                    "channel_move", sees(), target = ch.name, confirm = label("move"),
                    initial = mapOf("parent_id" to (ch.parentId ?: ""), "position" to ch.position),
                    fields = listOf(ManageField.Select("parent_id", t("discovery.settings.category"), categoryOptions), ManageField.Number("position", label("position"), 0, 500)),
                    request = { v ->
                        ManageRequest(channelId = ch.id, data = buildJsonObject {
                            put("parent_id", v["parent_id"].toString().ifEmpty { null })
                            put("position", v["position"] as? Int ?: 0)
                        })
                    },
                ))
            }
            RowAction(t("server.channelSettings.permissions")) {
                say(ManageAsk(
                    "overwrite_set", sees(), target = ch.name, confirm = save,
                    fields = listOf(ManageField.Over(ch.type, targets)),
                    request = { v ->
                        val who = targets.first { it.id == v["target_id"] }
                        if (v["clear"] == true) ManageRequest(action = "overwrite_remove", channelId = ch.id, targetId = who.id)
                        else ManageRequest(channelId = ch.id, targetId = who.id, data = buildJsonObject {
                            put("type", who.type)
                            put("allow", v["allow"].toString())
                            put("deny", v["deny"].toString())
                        })
                    },
                ))
            }
            if (parent != null && !Perms.sameOverwrites(ch.overwrites, parent.overwrites)) RowAction(label("sync")) {
                say(ManageAsk("overwrite_sync", sees(), target = ch.name, request = { ManageRequest(channelId = ch.id) }))
            }
            RowAction(label("delete"), danger = true) { plain("channel_delete", ch.name) { ManageRequest(channelId = ch.id) } }
        })
    }

    @Composable
    fun memberRow(x: ManageMember, first: Boolean) {
        val name = personOf(x.user)
        val mine = assignable.filter { it.id in x.roleIds }
        val others = assignable.filter { it.id !in x.roleIds }
        val isOwner = x.user.id == view.owner?.id
        val isMe = x.user.id == Nexa.users.meId
        fun pick(action: String, list: List<ManageRole>) = say(ManageAsk(
            action, sees(), target = name,
            initial = mapOf("role_id" to (list.firstOrNull()?.id ?: "")),
            fields = listOf(ManageField.Select("role_id", label("role"), list.map { it.id to it.name })),
            request = { v -> ManageRequest(userId = x.user.id, roleId = v["role_id"].toString()) },
        ))
        val sub = "@" + x.user.username + if (mine.isNotEmpty()) " · " + mine.joinToString(", ") { it.name } else ""
        ManageRow(first, lead = { Avatar(x.user.id, name, x.user.avatarId, 32.dp) }, leadWide = true, name = name, sub = sub, tags = {
            if (isOwner) ManageTag(label("ownerTag"))
        }, actions = if (isOwner || isMe) null else ({
            if (others.isNotEmpty()) RowAction(label("giveRole")) { pick("member_role_add", others) }
            if (mine.isNotEmpty()) RowAction(label("takeRole")) { pick("member_role_remove", mine) }
            RowAction(label("remove"), danger = true) {
                plain("member_remove", name, sees(t("moderation.manage.seesRemoved", "name" to name))) { ManageRequest(userId = x.user.id) }
            }
            RowAction(label("ban"), danger = true) {
                plain("member_ban", name, sees(t("moderation.manage.seesBanned", "name" to name))) { ManageRequest(userId = x.user.id) }
            }
            if (view.ownerGone) NxButton(label("makeOwner"), {
                val lines = (if (view.ownerDeleted) emptyList() else listOf(seesOwner)) + t("moderation.manage.seesTransfer", "name" to name)
                say(ManageAsk("transfer", lines, target = name, danger = true, request = { ManageRequest(userId = x.user.id) }))
            }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH)
        }))
    }

    @Composable
    fun ruleRow(r: ManageRule, first: Boolean) {
        ManageRow(first, name = r.name, sub = t("automod.kind.${r.kind}"), tags = {
            if (!r.enabled) ManageTag(label("off"))
            if (r.alerts) ManageTag(label("postsCaught"))
        }, actions = {
            if (!r.alerts) RowAction(label("edit")) {
                val words = r.kind == "words"
                say(ManageAsk(
                    "automod_edit", sees(), target = r.name, confirm = save,
                    initial = mapOf("name" to r.name, "words" to r.words.joinToString("\n")),
                    fields = buildList {
                        add(ManageField.Text("name", t("integrations.name"), 100, required = true))
                        if (words) add(ManageField.Text("words", label("ruleWords"), 20000, area = true))
                    },
                    request = { v ->
                        ManageRequest(ruleId = r.id, data = buildJsonObject {
                            put("name", v["name"].toString().trim())
                            if (words) put("config", buildJsonObject { put("words", buildJsonArray { wordsOf(v["words"]).forEach { add(it) } }) })
                        })
                    },
                ))
            }
            if (r.enabled || !r.alerts) {
                val word = label(if (r.enabled) "turnOff" else "turnOn")
                RowAction(word) {
                    say(ManageAsk("automod_edit", sees(), target = r.name, confirm = word, request = { ManageRequest(ruleId = r.id, data = buildJsonObject { put("enabled", !r.enabled) }) }))
                }
            }
            RowAction(label("delete"), danger = true) { plain("automod_delete", r.name) { ManageRequest(ruleId = r.id) } }
        })
    }

    when (tab) {
        "overview" -> {
            NoteBox(label("note"))
            ManageHead(t("server.channelSettings.overview")) {
                NxButton(t("moderation.manage.a.server_edit"), {
                    say(ManageAsk(
                        "server_edit", sees(), confirm = save,
                        initial = mapOf("name" to srv.name, "description" to (srv.description ?: ""), "system_channel_id" to (srv.systemChannelId ?: ""), "default_notifications" to srv.defaultNotifications.toString(), "nsfw" to srv.nsfw),
                        fields = listOf(
                            ManageField.Text("name", t("integrations.name"), 100, required = true),
                            ManageField.Text("description", t("moderation.servers.description"), 1000, area = true),
                            ManageField.Select("system_channel_id", t("server.serverOverview.systemMessagesChannel"), listOf("" to label("none")) + view.channels.filter { it.type == "text" && !it.private }.map { it.id to it.name }),
                            ManageField.Select("default_notifications", t("server.serverOverview.defaultNotificationSettings"), listOf("0" to t("server.serverOverview.allMessages"), "1" to t("server.serverOverview.onlyMentions"))),
                            ManageField.Toggle("nsfw", label("serverNsfw")),
                        ),
                        request = { v ->
                            ManageRequest(data = buildJsonObject {
                                put("name", v["name"].toString().trim())
                                put("description", v["description"].toString())
                                put("system_channel_id", v["system_channel_id"].toString().ifEmpty { null })
                                put("default_notifications", v["default_notifications"].toString().toIntOrNull() ?: 0)
                                put("nsfw", v["nsfw"] == true)
                            })
                        },
                    ))
                }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH)
            }
            ManageList {
                ManageRow(true, above = t("integrations.name"), name = srv.name)
                ManageRow(false, above = t("moderation.servers.description"), name = srv.description?.ifEmpty { null } ?: label("none"), plainName = true)
                if (srv.iconId != null) ManageRow(false, name = label("iconLabel"), actions = { RowAction(label("remove"), danger = true) { plain("server_icon_clear") } })
                if (srv.bannerId != null) ManageRow(false, name = label("bannerLabel"), actions = { RowAction(label("remove"), danger = true) { plain("server_banner_clear") } })
            }
            Txt(if (view.ownerGone) label("transferHere") else label("transferHint"), Modifier.padding(start = 2.dp), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f), lineHeight = Nx.type.sp(18.9f)), color = c.textMuted)
        }
        "channels" -> {
            fun under(id: String?) = view.channels.filter { it.type != "category" && it.parentId == id }.sortedBy { it.position }
            ManageHead(t("moderation.preview.channels")) {
                NxButton(t("modals.createChannel.createChannel"), {
                    say(ManageAsk(
                        "channel_create", sees(),
                        initial = mapOf("type" to "text", "name" to "", "parent_id" to "", "url" to ""),
                        fields = listOf(
                            ManageField.Select("type", label("channelType"), CHANNEL_KINDS.map { k -> k to if (k == "category") t("discovery.settings.category") else t("modals.createChannel.$k") }),
                            ManageField.Text("name", t("modals.createChannel.channelName"), 100, required = true),
                            ManageField.Select("parent_id", t("discovery.settings.category"), categoryOptions),
                            ManageField.Text("url", label("linkAddress"), 1024, shownWhen = { it["type"] == "link" }),
                        ),
                        request = { v ->
                            ManageRequest(data = buildJsonObject {
                                put("type", v["type"].toString())
                                put("name", v["name"].toString().trim())
                                if (v["parent_id"].toString().isNotEmpty() && v["type"] != "category") put("parent_id", v["parent_id"].toString())
                                if (v["type"] == "link") put("url", v["url"].toString().trim())
                            })
                        },
                    ))
                }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH)
            }
            ManageList {
                if (view.channels.isEmpty()) ManageEmpty("noChannels")
                var first = true
                for (ch in under(null)) { channelRow(ch, first); first = false }
                for (cat in categories) {
                    channelRow(cat, first)
                    first = false
                    for (ch in under(cat.id)) channelRow(ch, false)
                }
            }
        }
        "roles" -> {
            ManageHead(t("moderation.preview.roles")) {
                NxButton(t("server.roles.createRole"), {
                    say(ManageAsk(
                        "role_create", sees(),
                        initial = mapOf("name" to "", "color" to "#99aab5", "hoist" to false, "mentionable" to false, "permissions" to "0"),
                        fields = roleFields,
                        request = { v -> ManageRequest(data = roleData(v)) },
                    ))
                }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH)
            }
            ManageList {
                roleOrder.forEachIndexed { i, r ->
                    ManageRow(i == 0, inset = true, lead = {
                        Box(Modifier.size(12.dp).background(if (r.color != 0L) Color(0xFF000000 or r.color) else c.textMuted, CircleShape))
                    }, name = roleName(r), actions = {
                        RowAction(label("edit")) {
                            say(ManageAsk(
                                "role_edit", sees(), target = roleName(r), confirm = save,
                                initial = mapOf("name" to r.name, "color" to if (r.color != 0L) hexOf(r.color) else "#000000", "hoist" to r.hoist, "mentionable" to r.mentionable, "permissions" to r.permissions),
                                fields = if (r.id == everyoneId) roleFields.filterIsInstance<ManageField.Grid>() else roleFields,
                                request = { v -> ManageRequest(roleId = r.id, data = if (r.id == everyoneId) buildJsonObject { put("permissions", v["permissions"].toString()) } else roleData(v)) },
                            ))
                        }
                        if (r.id != everyoneId && r.managedBy == null) RowAction(label("delete"), danger = true) { plain("role_delete", r.name) { ManageRequest(roleId = r.id) } }
                    })
                }
            }
        }
        "members" -> {
            var query by remember { mutableStateOf("") }
            var members by remember { mutableStateOf<List<ManageMember>?>(null) }
            LaunchedEffect(query, refresh) {
                if (query.isNotEmpty()) delay(250)
                members = try {
                    Moderation.manageMembers(serverId, query.trim())
                } catch (e: Exception) {
                    if (e is kotlinx.coroutines.CancellationException) throw e
                    emptyList()
                }
            }
            NxTextField(query, { query = it }, placeholder = t("server.channelSettings.searchMembers"))
            ManageList {
                val list = members
                when {
                    list == null -> LoadingRow()
                    list.isEmpty() -> ManageEmpty("noMembers")
                    else -> list.forEachIndexed { i, x -> memberRow(x, i == 0) }
                }
            }
        }
        "bans" -> ManageList {
            if (view.bans.isEmpty()) ManageEmpty("noBans")
            view.bans.forEachIndexed { i, b ->
                val name = personOf(b.user)
                ManageRow(i == 0, lead = { Avatar(b.user.id, name, b.user.avatarId, 32.dp) }, leadWide = true, name = name, sub = b.reason.ifEmpty { null }, actions = {
                    RowAction(label("unban")) { say(ManageAsk("member_unban", sees(), target = name, request = { ManageRequest(userId = b.user.id) })) }
                })
            }
        }
        "invites" -> {
            val paused = (srv.invitesPausedUntil ?: 0L) > System.currentTimeMillis()
            val pause = label(if (paused) "resumeInvites" else "pauseInvites")
            ManageHead(t("server.invites.invites")) {
                RowAction(pause) {
                    say(ManageAsk("server_edit", sees(), target = pause, confirm = pause, request = {
                        ManageRequest(data = buildJsonObject { put("invites_paused_until", if (paused) null else System.currentTimeMillis() + DAY) })
                    }))
                }
                if (view.invites.isNotEmpty()) RowAction(t("server.invites.revokeAll"), danger = true) { plain("invites_revoke_all") }
            }
            ManageList {
                if (view.vanity.isNotEmpty()) ManageRow(true, name = view.vanity, code = true, tags = { ManageTag(label("vanity")) }, actions = {
                    RowAction(label("remove"), danger = true) { plain("vanity_clear", view.vanity) }
                })
                view.invites.forEachIndexed { i, x ->
                    ManageRow(i == 0 && view.vanity.isEmpty(), name = x.code, code = true, sub = t("moderation.manage.uses", "count" to x.uses), actions = {
                        RowAction(label("revoke"), danger = true) { plain("invite_revoke", x.code) { ManageRequest(code = x.code) } }
                    })
                }
                if (view.vanity.isEmpty() && view.invites.isEmpty()) ManageEmpty("noInvites")
            }
        }
        "apps" -> {
            ManageHead(t("apps.menu.apps"))
            ManageList {
                if (view.apps.isEmpty()) ManageEmpty("noApps")
                view.apps.forEachIndexed { i, a ->
                    ManageRow(i == 0, lead = { Icon("Plug", c.textMuted, size = 16.dp) }, name = a.name, tags = { if (a.readsAll) ManageTag(label("readsAll")) }, actions = {
                        RowAction(label("remove"), danger = true) { plain("app_remove", a.name) { ManageRequest(appId = a.id) } }
                    })
                }
            }
            ManageHead(t("integrations.webhooks"))
            ManageList {
                if (view.webhooks.isEmpty()) ManageEmpty("noWebhooks")
                view.webhooks.forEachIndexed { i, w ->
                    ManageRow(i == 0, lead = { Icon("Webhook", c.textMuted, size = 16.dp) }, name = w.name, sub = view.channels.firstOrNull { it.id == w.channelId }?.name, actions = {
                        RowAction(label("remove"), danger = true) { plain("webhook_delete", w.name) { ManageRequest(webhookId = w.id) } }
                    })
                }
            }
        }
        "automod" -> {
            ManageHead(t("automod.title")) {
                NxButton(t("moderation.manage.a.automod_create"), {
                    say(ManageAsk(
                        "automod_create", sees(),
                        initial = mapOf("name" to "", "words" to ""),
                        fields = listOf(ManageField.Text("name", t("integrations.name"), 100, required = true), ManageField.Text("words", label("ruleWords"), 20000, required = true, area = true)),
                        request = { v ->
                            ManageRequest(data = buildJsonObject {
                                put("name", v["name"].toString().trim())
                                put("kind", "words")
                                put("config", buildJsonObject { put("words", buildJsonArray { wordsOf(v["words"]).forEach { add(it) } }) })
                                putJsonArray("actions") { addJsonObject { put("type", "block") } }
                            })
                        },
                    ))
                }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH)
            }
            ManageList {
                if (view.automod.isEmpty()) ManageEmpty("noRules")
                view.automod.forEachIndexed { i, r -> ruleRow(r, i == 0) }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ManageHead(title: String, actions: (@Composable () -> Unit)? = null) {
    FlowRow(
        Modifier.fillMaxWidth().heightIn(min = 32.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.Start),
        verticalArrangement = Arrangement.spacedBy(8.dp),
        itemVerticalAlignment = Alignment.CenterVertically,
    ) {
        Txt(title.uppercase(), Modifier.padding(start = 2.dp).weight(1f), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(700), letterSpacing = 0.5.sp), color = Nx.colors.textMuted, maxLines = 2)
        if (actions != null) Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) { actions() }
    }
}

@Composable
private fun ManageList(content: @Composable ColumnScope.() -> Unit) {
    Column(Modifier.fillMaxWidth().background(Nx.colors.bgInput, RoundedCornerShape(14.dp)).padding(6.dp), content = content)
}

@Composable
private fun ManageEmpty(key: String) {
    Box(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 22.dp), contentAlignment = Alignment.Center) {
        Txt(label(key), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f)), color = Nx.colors.textMuted)
    }
}

@Composable
private fun ManageTag(text: String, danger: Boolean = false) {
    val c = Nx.colors
    Box(Modifier.height(20.dp).background(if (danger) c.danger.copy(alpha = 0.16f) else c.bgSurface, RoundedCornerShape(10.dp)).padding(horizontal = 7.dp), contentAlignment = Alignment.Center) {
        Txt(text, style = Nx.type.hint.copy(fontSize = Nx.type.sp(11.5f), fontWeight = FontWeight(650)), color = if (danger) c.textDanger else c.textSubtle, maxLines = 1)
    }
}

/** A row of a list. At a phone's width its buttons go under its name, not in a sliver beside it (`inset` lines them up with the name). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ManageRow(
    first: Boolean,
    name: String,
    inset: Boolean = false,
    lead: (@Composable () -> Unit)? = null,
    leadWide: Boolean = false,
    above: String? = null,
    sub: String? = null,
    plainName: Boolean = false,
    code: Boolean = false,
    tags: (@Composable () -> Unit)? = null,
    actions: (@Composable () -> Unit)? = null,
) {
    val c = Nx.colors
    val line = c.borderSubtle
    Column(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 48.dp)
            .then(if (first) Modifier else Modifier.drawBehind { drawRect(line, Offset.Zero, Size(size.width, 1.dp.toPx())) })
            .padding(horizontal = 8.dp, vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp, Alignment.CenterVertically),
    ) {
        Row(Modifier.fillMaxWidth().heightIn(min = 28.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            if (lead != null) Box(if (leadWide) Modifier else Modifier.size(20.dp), contentAlignment = Alignment.Center) { lead() }
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
                above?.let { Txt(it, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted) }
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(3.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                    Txt(
                        name,
                        style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(if (plainName) 500 else 600), fontFamily = if (code) androidx.compose.ui.text.font.FontFamily.Monospace else null),
                        color = c.textStrong,
                    )
                    tags?.invoke()
                }
                sub?.let { Txt(it, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted) }
            }
        }
        if (actions != null) {
            FlowRow(
                Modifier.fillMaxWidth().padding(start = if (inset) 30.dp else 0.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
                itemVerticalAlignment = Alignment.CenterVertically,
            ) { actions() }
        }
    }
}

/** A small ghost button whose words carry its colour, as the web's `.destroy` does. */
@Composable
private fun RowAction(text: String, danger: Boolean = false, onClick: () -> Unit) {
    val c = Nx.colors
    Box(
        Modifier.height(40.dp).clip(RoundedCornerShape(8.dp)).pressable(remember { MutableInteractionSource() }, onClick = onClick).padding(horizontal = 10.dp),
        contentAlignment = Alignment.Center,
    ) {
        Txt(text, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = if (danger) c.textDanger else c.text, maxLines = 1)
    }
}

/** The sheet that asks for one action's details and reason, and says first who will hear of it (web ManageActionModal). */
private fun openManageAction(serverId: String, ask: ManageAsk, onDone: (ManageView) -> Unit) {
    val overwrite = ask.fields.filterIsInstance<ManageField.Over>().firstOrNull()
    val firstTarget = overwrite?.targets?.firstOrNull()
    val values = mutableStateMapOf<String, Any>().apply {
        putAll(ask.initial)
        if (firstTarget != null) {
            put("target_id", firstTarget.id)
            put("allow", firstTarget.allow)
            put("deny", firstTarget.deny)
            put("clear", false)
        }
    }
    var reason by mutableStateOf("")
    var busy by mutableStateOf(false)
    var error by mutableStateOf<String?>(null)
    val title = label("a.${ask.action}")

    openModSheet(
        "manage-action",
        top = { close ->
            ModSheetTop(title, close, leading = {}) {
                ask.target?.let { Txt(it, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f)), color = Nx.colors.textMuted, maxLines = 2) }
            }
        },
        foot = { close ->
            val scope = rememberCoroutineScope()
            val missing = ask.fields.any { it is ManageField.Text && it.required && values[it.key].toString().isBlank() }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
                NxButton(t("bugs.form.cancel"), close, kind = ButtonKind.GHOST)
                NxButton(ask.confirm ?: title, {
                    if (!busy) {
                        busy = true
                        error = null
                        scope.launch {
                            try {
                                val q = ask.request(values.toMap())
                                val view = Moderation.manage(serverId, buildJsonObject {
                                    put("action", q.action ?: ask.action)
                                    put("reason", reason.trim())
                                    q.channelId?.let { put("channel_id", it) }
                                    q.roleId?.let { put("role_id", it) }
                                    q.userId?.let { put("user_id", it) }
                                    q.targetId?.let { put("target_id", it) }
                                    q.webhookId?.let { put("webhook_id", it) }
                                    q.appId?.let { put("app_id", it) }
                                    q.ruleId?.let { put("rule_id", it) }
                                    q.code?.let { put("code", it) }
                                    q.data?.let { put("data", it) }
                                })
                                Nexa.ui.toast(label("done"), "success")
                                onDone(view)
                                close()
                            } catch (e: Exception) {
                                if (e is kotlinx.coroutines.CancellationException) throw e
                                error = describeError(e, t("moderation.failed"))
                                busy = false
                            }
                        }
                    }
                }, kind = if (ask.danger) ButtonKind.DANGER else ButtonKind.PRIMARY, enabled = reasonOk(reason) && !missing, loading = busy)
            }
        },
    ) {
        val c = Nx.colors
        fun show(v: Any?) = v?.toString() ?: ""
        for (f in ask.fields) {
            when (f) {
                is ManageField.Text -> if (f.shownWhen == null || f.shownWhen.invoke(values)) ManageSection(f.label) {
                    NxTextField(show(values[f.key]), { values[f.key] = it.take(f.max) }, singleLine = !f.area, minLines = if (f.area) 3 else 1)
                }
                is ManageField.Number -> ManageSection(f.label) {
                    NxTextField(show(values[f.key] ?: 0), { text ->
                        values[f.key] = (text.filter { it.isDigit() }.take(6).toIntOrNull() ?: 0).coerceIn(f.min, f.max)
                    }, keyboardType = KeyboardType.Number)
                }
                is ManageField.Select -> ManageSection(f.label) {
                    SelectField(f.options, show(values[f.key]), { values[f.key] = it }, title = f.label)
                }
                is ManageField.Toggle -> ToggleLine(f.label, values[f.key] == true, { values[f.key] = it })
                is ManageField.Colour -> ManageSection(f.label) {
                    val hex = show(values[f.key]).ifEmpty { "#000000" }
                    Row(
                        Modifier.fillMaxWidth().heightIn(min = 44.dp).clip(Radii.Sm).background(c.bgInput, Radii.Sm).pressable(remember { MutableInteractionSource() }) { openColourPicker(f.label, hex, false) { values[f.key] = it } }.padding(horizontal = 12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        Box(Modifier.size(20.dp).background(Color(0xFF000000 or colourNumber(hex)), CircleShape).border(1.dp, c.borderSubtle, CircleShape))
                        Txt(hex, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text)
                    }
                }
                is ManageField.Grid -> ManageSection(f.label) {
                    Column(Modifier.fillMaxWidth().background(c.bgInput, Radii.Sm).padding(horizontal = 10.dp, vertical = 2.dp)) {
                        RoleGrid(permOf(values[f.key]), { values[f.key] = it.toString() }, editable = true, filter = "", grantable = null)
                    }
                }
                is ManageField.Over -> Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    ManageSection(label("appliesTo")) {
                        SelectField(f.targets.map { it.id to it.name }, show(values["target_id"]), { id ->
                            f.targets.firstOrNull { it.id == id }?.let { next ->
                                values["target_id"] = next.id
                                values["allow"] = next.allow
                                values["deny"] = next.deny
                                values["clear"] = false
                            }
                        }, title = label("appliesTo"))
                    }
                    val current = f.targets.firstOrNull { it.id == values["target_id"] }
                    if (current != null && (current.allow != "0" || current.deny != "0")) ToggleLine(label("clearEntry"), values["clear"] == true, { values["clear"] = it })
                    if (values["clear"] != true) Column(Modifier.fillMaxWidth().background(c.bgInput, Radii.Sm).padding(horizontal = 10.dp, vertical = 2.dp)) {
                        OverwriteGrid(permOf(values["allow"]), permOf(values["deny"]), { allow, deny -> values["allow"] = allow.toString(); values["deny"] = deny.toString() }, editable = true, channelType = f.channelType, grantable = null)
                    }
                }
            }
        }
        ReasonSection(reason, { reason = it }, label("reasonPlaceholder"), "", emptyList())
        Column(
            Modifier.fillMaxWidth().background(c.brand.copy(alpha = 0.09f), Radii.Sm).border(1.dp, c.brand.copy(alpha = 0.26f), Radii.Sm).padding(horizontal = 12.dp, vertical = 10.dp),
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Txt(label("sees"), Modifier.padding(bottom = 2.dp), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(12.5f), fontWeight = FontWeight(700)), color = c.text)
            for (line in ask.sees) Txt(line, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), lineHeight = Nx.type.sp(19.6f)), color = c.text)
        }
        error?.let { e -> DangerBox { Txt(e, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text) } }
    }
}

@Composable
private fun ManageSection(label: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SectionLabel(label)
        content()
    }
}
