package fish.nexa.ui.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.ChatMessage
import fish.nexa.data.P
import fish.nexa.net.ApiError
import fish.nexa.net.ReactionRole
import fish.nexa.net.Role
import fish.nexa.net.wireJson
import fish.nexa.ui.Icon
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.layers.Sheet
import fish.nexa.ui.pickers.Pickers
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.net.URLEncoder

/** What a role given to anyone who reacts may not carry (api/reaction_roles.go). */
private const val UNSAFE = P.ADMINISTRATOR or P.MANAGE_SERVER or P.MANAGE_ROLES or P.MANAGE_CHANNELS or P.KICK_MEMBERS or P.BAN_MEMBERS or
    P.MODERATE_MEMBERS or P.MANAGE_MESSAGES or P.MANAGE_WEBHOOKS or P.MENTION_EVERYONE or P.MANAGE_NICKNAMES or P.MANAGE_THREADS or
    P.MOVE_MEMBERS or P.MUTE_MEMBERS or P.DEAFEN_MEMBERS or P.VIEW_AUDIT_LOG

private fun colourOf(r: Role): Color? = if (r.color != 0L) Color(0xFF000000 or r.color) else null

private suspend fun send(method: String, cm: ChatMessage, emoji: String, body: JsonObject?): List<ReactionRole> {
    val path = "/channels/${cm.channelId}/messages/${cm.id}/reaction-roles/" + URLEncoder.encode(emoji, "UTF-8").replace("+", "%20")
    val answer = Nexa.api.request(method, path, body) as JsonObject
    return wireJson.decodeFromJsonElement(ListSerializer(ReactionRole.serializer()), answer["reaction_roles"]!!)
}

/** What each reaction on one message gives, for those who manage roles (web ReactionRolesModal). */
fun openReactionRoles(cm: ChatMessage) = fish.nexa.ui.dialogs.open("reactionRoles") { layer, _ ->
    val c = Nx.colors
    val serverId = Nexa.channels.get(cm.channelId)?.serverId
    val st = Nexa.servers.byId.collectAsState().value[serverId]
    var list by remember { mutableStateOf(cm.m.reactionRoles) }
    var emoji by remember { mutableStateOf<String?>(null) }
    var roleId by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    val offered = remember(st) {
        val meId = Nexa.users.meId
        if (st == null || meId == null) emptyList()
        else {
            val (highest, isOwner) = Nexa.servers.memberHighest(st, meId)
            Nexa.servers.rolesByPosition(st).filter { r ->
                r.id != serverId && r.managedBy == null && ((r.permissions.toLongOrNull() ?: 0L) and UNSAFE) == 0L && (isOwner || r.position < highest)
            }
        }
    }
    fun run(what: suspend () -> Unit) {
        busy = true
        Nexa.scope.launch {
            try {
                what()
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                Nexa.ui.toast((e as? ApiError)?.message ?: t("settings.privacy.couldNotSave"), "error")
            } finally {
                busy = false
            }
        }
    }
    Sheet(layer, title = t("chat.reactionRoles.title"), maxWidth = 460.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Txt(t("chat.reactionRoles.hint"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(19.6f)), color = c.textMuted)
            if (list.isEmpty()) {
                Txt(
                    t("chat.reactionRoles.empty"),
                    style = Nx.type.body.copy(fontSize = Nx.type.sp(14f)),
                    color = c.textMuted,
                    modifier = Modifier.fillMaxWidth().background(c.bgSurface, Radii.Sm).padding(horizontal = 14.dp, vertical = 12.dp),
                )
            } else Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                for (rr in list) {
                    val role = st?.roles?.get(rr.roleId) ?: continue
                    Row(
                        Modifier.fillMaxWidth().heightIn(min = 44.dp).background(c.bgSurface, Radii.Sm).padding(start = 12.dp, end = 4.dp, top = 4.dp, bottom = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        EmojiGlyph(rr.emoji, 22.dp)
                        colourOf(role)?.let { Box(Modifier.size(10.dp).background(it, CircleShape)) }
                        Txt(role.name, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(500)), color = c.text, maxLines = 1, modifier = Modifier.weight(1f))
                        Box(
                            Modifier.size(40.dp).pressable(remember { MutableInteractionSource() }) { if (!busy) run { list = send("DELETE", cm, rr.emoji, null) } }
                                .semantics { contentDescription = t("chat.reactionRoles.remove", "role" to role.name) },
                            contentAlignment = Alignment.Center,
                        ) { Icon("X", c.textMuted, size = 16.dp) }
                    }
                }
            }
            if (offered.isEmpty()) Txt(t("chat.reactionRoles.noRoles"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(19.6f)), color = c.textMuted)
            else Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(
                    Modifier.size(44.dp).background(c.bgSurface, Radii.Sm).pressable(remember { MutableInteractionSource() }) {
                        Pickers.open(Pickers.Opener(listOf(Pickers.Tab.EMOJI), serverId, draggable = false, closeOnEmoji = true, title = t("chat.reactionRoles.chooseEmoji"), onEmoji = { p ->
                            emoji = p.custom?.let { (if (it.animated) "a" else "") + ":" + it.name + ":" + it.blobId }
                                ?: p.packEmoji?.let { (if (it.animated) "a" else "") + ":" + it.name + ":" + it.blobId } ?: p.text
                        }))
                    },
                    contentAlignment = Alignment.Center,
                ) {
                    val e = emoji
                    if (e != null) EmojiGlyph(e, 22.dp) else Icon("SmilePlus", c.textMuted, size = 20.dp)
                }
                val chosen = offered.firstOrNull { it.id == roleId }
                Row(
                    Modifier.weight(1f).heightIn(min = 44.dp).background(c.bgSurface, Radii.Sm).pressable(remember { MutableInteractionSource() }) {
                        openPick(offered) { roleId = it }
                    }.padding(horizontal = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Txt(chosen?.name ?: t("chat.reactionRoles.chooseRole"), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f)), color = if (chosen != null) c.text else c.textMuted, maxLines = 1, modifier = Modifier.weight(1f))
                    Icon("ChevronDown", c.textMuted, size = 16.dp)
                }
                NxButton(t("chat.reactionRoles.add"), {
                    val e = emoji
                    val r = roleId
                    if (e != null && r != null) run {
                        list = send("PUT", cm, e, buildJsonObject { put("role_id", r) })
                        emoji = null
                        roleId = null
                    }
                }, size = ButtonSize.TOUCH, icon = "Plus", enabled = emoji != null && roleId != null, loading = busy)
            }
        }
    }
}

private fun openPick(roles: List<Role>, onPick: (String) -> Unit) = fish.nexa.ui.dialogs.open("reactionRolePick") { layer, close ->
    val c = Nx.colors
    Sheet(layer, title = t("chat.reactionRoles.chooseRole"), maxWidth = 460.dp) {
        for (r in roles) {
            Row(
                Modifier.fillMaxWidth().heightIn(min = 48.dp).pressable(remember { MutableInteractionSource() }) { onPick(r.id); close() },
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                Box(Modifier.size(10.dp).background(colourOf(r) ?: c.textMuted, CircleShape))
                Txt(r.name, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f)), color = c.text, maxLines = 1)
            }
        }
    }
}
