package fish.nexa.ui.server

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import fish.nexa.Nexa
import fish.nexa.data.P
import fish.nexa.data.ServerState
import fish.nexa.ui.layers.Layer
import fish.nexa.ui.nav.Nav
import fish.nexa.ui.nav.Route
import fish.nexa.ui.t

/**
 * ServerSettingsModal.tsx: a server's settings, the pages a person may open
 * (the rest are left out, not greyed), in the web's order and groups. It
 * closes by itself when the server goes (deleted, left, removed).
 */
object ServerSettings {
    /** Opens on the list, or straight on [page] when it is one this person may open. */
    fun open(serverId: String, page: String? = null) {
        openSettingsLayer("serverSettings") { layer, close -> ServerSettingsLayer(layer, serverId, page, close) }
    }

    /** The pages this person may open, in order: id, label, icon, group (0 server, 1 moderation, 2 the danger zone). */
    fun pages(st: ServerState): List<Pair<NavPage, Int>> {
        val s = Nexa.servers
        val owner = st.server.ownerId == Nexa.users.meId
        val webhooks = s.canI(st, P.MANAGE_WEBHOOKS) || Nexa.channels.byId.value.values.any { it.serverId == st.server.id && it.isText && s.canI(st, P.MANAGE_WEBHOOKS, it.id) }
        val all = listOf(
            Triple(NavPage("overview", t("server.channelSettings.overview"), "Settings2"), s.canI(st, P.MANAGE_SERVER), 0),
            Triple(NavPage("tag", t("serverTag.page.title"), "Tag"), s.canI(st, P.MANAGE_SERVER), 0),
            Triple(NavPage("roles", t("server.members.roles"), "Tags"), s.canI(st, P.MANAGE_ROLES), 0),
            Triple(NavPage("emoji", t("modals.customStatus.emoji"), "Smile"), true, 0),
            Triple(NavPage("stickers", t("pickers.expressionPicker.stickers"), "Sticker"), true, 0),
            // its apps for whoever manages the server, its webhooks for whoever manages them somewhere
            Triple(NavPage("integrations", t("integrations.title"), "Webhook"), webhooks || s.canI(st, P.MANAGE_SERVER), 0),
            Triple(NavPage("encryption", t("server.encryption.encryption"), "Lock"), true, 0),
            Triple(NavPage("discovery", t("discovery.title"), "Compass"), s.canI(st, P.MANAGE_SERVER), 0),
            Triple(NavPage("template", t("server.template.title"), "PanelsTopLeft"), s.canI(st, P.MANAGE_SERVER), 0),
            Triple(NavPage("members", t("server.serverSettings.members"), "Users"), true, 1),
            Triple(NavPage("invites", t("server.invites.invites"), "Link"), s.canI(st, P.MANAGE_SERVER), 1),
            Triple(NavPage("bans", t("server.serverSettings.bans"), "Gavel"), s.canI(st, P.BAN_MEMBERS), 1),
            Triple(NavPage("automod", t("automod.title"), "ShieldCheck"), s.canI(st, P.MANAGE_SERVER) || s.canI(st, P.VIEW_AUDIT_LOG), 1),
            Triple(NavPage("safety", t("safety.title"), "ShieldAlert"), s.canI(st, P.MANAGE_SERVER), 1),
            Triple(NavPage("audit", t("server.auditLog.auditLog"), "ScrollText"), s.canI(st, P.VIEW_AUDIT_LOG), 1),
            Triple(NavPage("delete", t("server.deleteServer.deleteServer"), "Trash2", danger = true), owner, 2),
        )
        return all.filter { it.second }.map { it.first to it.third }
    }
}

@Composable
private fun ServerSettingsLayer(layer: Layer, serverId: String, initial: String?, close: () -> Unit) {
    val servers by Nexa.servers.byId.collectAsState()
    val st = servers[serverId]
    // the server went (deleted, left, removed): so do its settings
    LaunchedEffect(st == null) { if (st == null) close() }
    if (st == null) return
    val allowed = ServerSettings.pages(st)
    var page by remember { mutableStateOf(initial?.takeIf { p -> allowed.any { it.first.id == p } } ?: allowed.firstOrNull()?.first?.id ?: "members") }
    val sections = listOf(
        NavSection(st.server.name, allowed.filter { it.second == 0 }.map { it.first }),
        NavSection(t("server.serverSettings.moderation"), allowed.filter { it.second == 1 }.map { it.first }),
        NavSection(null, allowed.filter { it.second == 2 }.map { it.first }),
    )
    SettingsFrame(
        layer,
        t("server.serverSettings.serverSettings"),
        sections,
        page,
        { page = it },
        startOnList = initial == null,
        close = close,
    ) { id ->
        when (id) {
            "overview" -> OverviewPage(serverId)
            "roles" -> RolesPage(serverId)
            "integrations" -> IntegrationsPage(serverId, channelId = null)
            "invites" -> InvitesPage(serverId)
            "bans" -> BansPage(serverId)
            "audit" -> AuditLogPage(serverId)
            "emoji" -> EmojiPage(serverId)
            "stickers" -> ServerStickersPage(serverId)
            "tag" -> ServerTagPage(serverId)
            "members" -> MembersPage(serverId)
            "discovery" -> DiscoverySettingsPage(serverId)
            "template" -> ServerTemplatePage(serverId)
            "safety" -> SafetyPage(serverId) { page = it }
            "automod" -> fish.nexa.ui.automod.AutomodPage(serverId)
            "delete" -> DeleteServerPage(serverId) {
                close()
                Nav.go(Route.Friends, replace = true)
            }
            else -> EncryptionPage(serverId)
        }
    }
}
