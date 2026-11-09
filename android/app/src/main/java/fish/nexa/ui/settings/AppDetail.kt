package fish.nexa.ui.settings

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.crypto.Bytes
import fish.nexa.data.AppCommands
import fish.nexa.data.Apps
import fish.nexa.data.P
import fish.nexa.net.AppCommand
import fish.nexa.net.InstallParams
import fish.nexa.net.IntegrationTypeConfig
import fish.nexa.net.OwnedApp
import fish.nexa.net.wireJson
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.UnderlineTabs
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.media.HookTags
import fish.nexa.ui.server.ServerApi
import fish.nexa.ui.server.openImageCrop
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.boxShadow
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.sheen
import fish.nexa.ui.theme.sunken
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import java.net.URLEncoder

private enum class AppTab { GENERAL, BOT, INSTALL, OAUTH, COMMANDS }

/** The OAuth2 scopes an app can ask for, with what each lets it do, for the link builder. */
private fun scopeList(): List<Pair<String, String>> = listOf(
    "bot" to t("apps.scope.bot"),
    "applications.commands" to t("apps.scope.commands"),
    "identify" to t("apps.scope.identify"),
    "email" to t("apps.scope.email"),
    "guilds" to t("apps.scope.guilds"),
    "guilds.join" to t("apps.scope.guildsJoin"),
    "guilds.members.read" to t("apps.scope.membersRead"),
    "connections" to t("apps.scope.connections"),
    "applications.commands.permissions.update" to t("apps.scope.commandPermissions"),
    "webhook.incoming" to t("apps.scope.webhook"),
)

private fun enc(s: String) = URLEncoder.encode(s, "UTF-8").replace("+", "%20")

/** The link that adds an app, as its settings make it. */
fun installLink(protocolId: String, scopes: List<String>? = null, permissions: String? = null, integrationType: Int? = null, redirect: String? = null): String {
    val q = ArrayList<String>()
    q += "client_id=" + enc(protocolId)
    if (permissions != null && permissions != "0") q += "permissions=" + enc(permissions)
    if (integrationType != null) q += "integration_type=$integrationType"
    if (!scopes.isNullOrEmpty()) q += "scope=" + enc(scopes.joinToString(" "))
    if (redirect != null) {
        q += "redirect_uri=" + enc(redirect)
        q += "response_type=code"
    }
    return portalBase() + "/oauth2/authorize?" + q.joinToString("&")
}

/** A picture picked and cropped, as image data the server takes. */
private fun dataUrl(bytes: ByteArray, type: String) = "data:$type;base64," + Bytes.b64(bytes)

/**
 * One app in the Developer Portal (web components/settings/portal/
 * AppDetail.tsx): who it is at the top (its picture, name and the id
 * libraries know it by), then its settings by tab. Changes wait in a draft
 * until saved; a new token, a new secret and deleting the app ask for the
 * account's password first.
 */
@Composable
fun AppDetail(app: OwnedApp, onBack: () -> Unit, onChange: (OwnedApp) -> Unit, onDeleted: () -> Unit) {
    val c = Nx.colors
    val form = remember(app.id) { PortalEditing.Form(app, onChange) }
    DisposableEffect(form) {
        PortalEditing.form = form
        onDispose { if (PortalEditing.form === form) PortalEditing.form = null }
    }
    // what was saved elsewhere on this page (a new secret) comes in without losing the draft
    LaunchedEffect(app) { form.app = app }
    var tab by remember { mutableStateOf(AppTab.GENERAL) }
    val shown = form.app
    val name = form.str("name", shown.name)
    Column {
        Row(
            Modifier
                .padding(bottom = 14.dp)
                .clip(Radii.Xs)
                .pressable(remember { MutableInteractionSource() }, onClick = onBack)
                .padding(start = 4.dp, end = 8.dp, top = 6.dp, bottom = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon("ChevronLeft", c.textMuted, size = 18.dp, modifier = Modifier.graphicsLayer { if (fish.nexa.ui.I18n.rtl) scaleX = -1f })
            Txt(t("apps.portal.allApps"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textMuted)
        }
        // the tabs sit 10dp under it (Portal.module.css .head, Designer.module.css .scopeTabs)
        Row(Modifier.padding(bottom = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(16.dp)) {
            Box(Modifier.boxShadow(RoundedCornerShape(22.dp), 10f, 24f, c.shadeAt(0.8f), spread = -12f).ring(RoundedCornerShape(22.dp), c.panelEdge).clip(RoundedCornerShape(22.dp))) {
                AppPicture(shown, 64, square = true)
            }
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                    Txt(name, style = Nx.type.title.copy(fontSize = Nx.type.sp(22f), fontWeight = FontWeight(700)), color = c.textStrong, maxLines = 1, modifier = Modifier.semantics { heading() })
                    HookTags(null, app = true)
                }
                IdChip(t("apps.portal.applicationId"), shown.protocolId)
            }
        }
        UnderlineTabs(
            listOf(
                AppTab.GENERAL to t("apps.portal.tab.general"),
                AppTab.BOT to t("apps.portal.tab.bot"),
                AppTab.INSTALL to t("apps.portal.tab.install"),
                AppTab.OAUTH to t("apps.portal.tab.oauth"),
                AppTab.COMMANDS to t("apps.portal.tab.commands"),
            ),
            tab, name, Modifier.padding(bottom = 18.dp),
        ) { tab = it }
        Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
            when (tab) {
                AppTab.GENERAL -> GeneralTab(form, onDeleted)
                AppTab.BOT -> BotTab(form)
                AppTab.INSTALL -> InstallTab(form)
                AppTab.OAUTH -> OAuthTab(form, onChange)
                AppTab.COMMANDS -> CommandsTab(shown)
            }
        }
    }
}

/** Picks a square picture, cropped, as image data; Remove takes it away. */
@Composable
private fun PicturePick(label: String, round: Boolean, picked: String?, canRemove: Boolean, fallback: @Composable () -> Unit, onPick: (String) -> Unit, onRemove: () -> Unit) {
    val c = Nx.colors
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val pick = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            val f = runCatching { ServerApi.read(context, uri) }.getOrNull() ?: return@launch
            openImageCrop(f, 1f, round = round, title = t("apps.portal.cropImage"), maxSize = 512) { r -> onPick(dataUrl(r.bytes, r.type)) }
        }
    }
    val shape = if (round) CircleShape else RoundedCornerShape(28.dp)
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(16.dp)) {
        Box(Modifier.size(96.dp).clip(shape).background(c.bgInput).border(1.dp, c.borderSubtle, shape), contentAlignment = Alignment.Center) {
            val bitmap = remember(picked) {
                picked?.substringAfter("base64,", "")?.takeIf { it.isNotEmpty() }?.let { b ->
                    runCatching { Bytes.unb64(b).let { android.graphics.BitmapFactory.decodeByteArray(it, 0, it.size) }?.asImageBitmap() }.getOrNull()
                }
            }
            if (bitmap != null) Image(bitmap, null, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize()) else fallback()
        }
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            NxButton(label, { pick.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH, icon = "ImagePlus")
            if (canRemove) NxButton(t("apps.portal.removeImage"), onRemove, kind = ButtonKind.GHOST, size = ButtonSize.TOUCH)
        }
    }
}

/** A tick and its words, the whole line its target. */
@Composable
private fun CheckLine(on: Boolean, label: String, sub: String? = null, onClick: () -> Unit) {
    val c = Nx.colors
    Row(
        Modifier
            .heightIn(min = 44.dp)
            .clip(Radii.Sm)
            .pressable(remember(label) { MutableInteractionSource() }, role = Role.Checkbox, onClick = onClick)
            .semantics { selected = on }
            .padding(end = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        fish.nexa.ui.components.TickBox(on)
        Txt(label, style = Nx.type.body.copy(fontSize = Nx.type.sp(14.5f)), color = c.text)
        if (sub != null) Txt(sub, style = Nx.type.code.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted)
    }
}

/** A line of what something is, and the button that changes it. */
@Composable
private fun RowLine(hint: String, button: String, onClick: () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Txt(hint, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), lineHeight = Nx.type.sp(18f)), color = Nx.colors.textMuted)
        NxButton(button, onClick, kind = ButtonKind.SECONDARY)
    }
}

@Composable
private fun GeneralTab(form: PortalEditing.Form, onDeleted: () -> Unit) {
    val c = Nx.colors
    val app = form.app
    var tag by remember { mutableStateOf("") }
    val icon = form.draft["icon"]
    val tags = (form.draft["tags"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull } ?: app.tags
    val types = (form.draft["event_webhooks_types"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull } ?: app.eventWebhooksTypes
    fun addTag() {
        val x = tag.trim()
        tag = ""
        if (x.isEmpty() || tags.size >= 5 || x in tags) return
        form.set("tags", JsonArray((tags + x.take(20)).map { JsonPrimitive(it) }))
    }
    fun toggleType(type: String) = form.set("event_webhooks_types", JsonArray((if (type in types) types - type else types + type).map { JsonPrimitive(it) }))
    PicturePick(
        t("apps.portal.uploadIcon"), round = false,
        picked = (icon as? JsonPrimitive)?.contentOrNull,
        canRemove = if (icon == null) app.iconId != null else icon !is JsonNull,
        // its icon, or what stands in for one: its bot's avatar or its letters (AppDetail.tsx)
        fallback = {
            val iconId = if (icon is JsonNull) null else app.iconId
            Avatar(app.id, form.str("name", app.name), iconId ?: app.bot.avatarId, 96.dp, icon = iconId != null, shape = fish.nexa.ui.components.SQUARE_AVATAR)
        },
        onPick = { form.set("icon", JsonPrimitive(it)) },
        onRemove = { if (app.iconId != null) form.set("icon", JsonNull) else form.draft.remove("icon") },
    )
    NxTextField(form.str("name", app.name), { form.set("name", JsonPrimitive(it.take(32))) }, label = t("apps.portal.name"), required = true, hint = t("apps.portal.nameHint"), capitalize = true)
    NxTextField(
        form.str("description", app.description), { form.set("description", JsonPrimitive(it.take(400))) },
        label = t("apps.portal.description"), hint = t("apps.portal.descriptionHint"), singleLine = false, minLines = 3, capitalize = true, autoCorrect = true,
    )
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Txt(t("apps.portal.tags"), style = Nx.type.label, color = c.textSubtle)
        FlowRow(
            Modifier.fillMaxWidth().sunken(Radii.Sm, c).border(1.dp, c.borderSubtle, Radii.Sm).padding(6.dp).heightIn(min = 32.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
            itemVerticalAlignment = Alignment.CenterVertically,
        ) {
            for (x in tags) Row(
                Modifier.ring(RoundedCornerShape(999.dp), c.borderSubtle).background(c.bgSurface, RoundedCornerShape(999.dp)).padding(start = 10.dp, end = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                Txt(x, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), fontWeight = FontWeight(550)), color = c.text)
                Box(
                    Modifier.size(32.dp).clip(CircleShape).pressable(remember(x) { MutableInteractionSource() }) { form.set("tags", JsonArray((tags - x).map { JsonPrimitive(it) })) }
                        .semantics { contentDescription = t("apps.portal.removeTag", "tag" to x) },
                    contentAlignment = Alignment.Center,
                ) { Icon("X", c.icon, size = 13.dp) }
            }
            if (tags.size < 5) Box(Modifier.heightIn(min = 32.dp).padding(horizontal = 6.dp), contentAlignment = Alignment.CenterStart) {
                if (tag.isEmpty()) Txt(t("apps.portal.addTag"), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f)), color = c.textPlaceholder)
                BasicTextField(
                    tag, { v -> if (v.endsWith(",")) { tag = v.dropLast(1); addTag() } else tag = v.take(20) },
                    textStyle = Nx.type.body.copy(fontSize = Nx.type.sp(15f), color = c.text),
                    cursorBrush = SolidColor(c.text),
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { addTag() }),
                    modifier = Modifier.semantics { contentDescription = t("apps.portal.addTag") },
                )
            }
        }
        Txt(t("apps.portal.tagsHint"), style = Nx.type.hint, color = c.textMuted)
    }
    SectionHeading(t("apps.portal.interactions"))
    NxTextField(
        form.str("interactions_url", app.interactionsUrl), { form.set("interactions_url", JsonPrimitive(it.trim())) },
        label = t("apps.portal.endpoint"), hint = t("apps.portal.endpointHint"), placeholder = "https://", keyboardType = KeyboardType.Uri, mono = true, monoSize = 13f,
    )
    IdChip(t("apps.portal.publicKey"), app.verifyKey)
    SectionHeading(t("apps.portal.links"))
    NxTextField(form.str("terms_url", app.termsUrl), { form.set("terms_url", JsonPrimitive(it.trim())) }, label = t("apps.portal.terms"), placeholder = "https://", keyboardType = KeyboardType.Uri)
    NxTextField(form.str("privacy_url", app.privacyUrl), { form.set("privacy_url", JsonPrimitive(it.trim())) }, label = t("apps.portal.privacy"), placeholder = "https://", keyboardType = KeyboardType.Uri)
    SectionHeading(t("apps.portal.events"))
    Hint(t("apps.portal.eventsHint"), Modifier.padding(bottom = 4.dp))
    NxTextField(
        form.str("event_webhooks_url", app.eventWebhooksUrl), { form.set("event_webhooks_url", JsonPrimitive(it.trim())) },
        label = t("apps.portal.eventsUrl"), placeholder = "https://", keyboardType = KeyboardType.Uri, mono = true, monoSize = 13f,
    )
    ToggleRow(t("apps.portal.eventsOn"), form.bool("event_webhooks_enabled", app.eventWebhooksEnabled), { form.set("event_webhooks_enabled", JsonPrimitive(it)) })
    FlowRow(horizontalArrangement = Arrangement.spacedBy(18.dp)) {
        CheckLine("APPLICATION_AUTHORIZED" in types, t("apps.portal.eventAdded")) { toggleType("APPLICATION_AUTHORIZED") }
        CheckLine("APPLICATION_DEAUTHORIZED" in types, t("apps.portal.eventRemoved")) { toggleType("APPLICATION_DEAUTHORIZED") }
    }
    SectionHeading(t("apps.portal.dangerZone"))
    DangerBox(t("apps.portal.deleteApp"), t("apps.portal.deleteHint"), t("apps.portal.deleteApp"), onClick = {
        openPasswordConfirm(t("apps.portal.deleteNamed", "name" to app.name), t("apps.portal.deleteBody"), t("apps.portal.deleteApp"), danger = true) { key ->
            Apps.remove(app.id, key)
            AppCommands.forget()
            onDeleted()
        }
    })
}

@Composable
private fun BotTab(form: PortalEditing.Form) {
    val app = form.app
    var token by remember { mutableStateOf<String?>(null) }
    val intents = (form.draft["intents"] as? JsonObject)
    fun intent(key: String, saved: Boolean) = (intents?.get(key) as? JsonPrimitive)?.booleanOrNull ?: saved
    fun setIntent(key: String, on: Boolean) = form.set("intents", JsonObject((intents ?: JsonObject(emptyMap())) + (key to JsonPrimitive(on))))
    val avatar = form.bot["avatar"]
    val name = form.str("name", app.name)
    PicturePick(
        t("apps.portal.uploadAvatar"), round = true,
        picked = (avatar as? JsonPrimitive)?.contentOrNull,
        canRemove = if (avatar == null) app.bot.avatarId != null else avatar !is JsonNull,
        fallback = { Avatar(app.bot.id, name, if (avatar is JsonNull) null else app.bot.avatarId, 96.dp) },
        onPick = { form.bot["avatar"] = JsonPrimitive(it) },
        onRemove = { if (app.bot.avatarId != null) form.bot["avatar"] = JsonNull else form.bot.remove("avatar") },
    )
    NxTextField(
        (form.bot["username"] as? JsonPrimitive)?.contentOrNull ?: app.bot.username,
        { form.bot["username"] = JsonPrimitive(it.lowercase().take(32)) },
        label = t("apps.portal.username"), hint = t("apps.portal.usernameHint"),
    )
    SectionHeading(t("apps.portal.token"))
    val shown = token
    if (shown != null) SecretReveal(shown, t("apps.portal.tokenNote")) { token = null }
    else RowLine(t("apps.portal.tokenHint"), t("apps.portal.resetToken")) {
        openPasswordConfirm(t("apps.portal.resetToken"), t("apps.portal.resetTokenBody"), t("apps.portal.resetToken")) { key -> token = Apps.resetToken(app.id, key) }
    }
    SectionHeading(t("apps.portal.whoCanAdd"))
    ToggleRow(t("apps.portal.public"), form.bool("bot_public", app.botPublic), { form.set("bot_public", JsonPrimitive(it)) }, t("apps.portal.publicHint"))
    val public = form.bool("bot_public", app.botPublic)
    ToggleRow(
        t("apps.portal.discovery"),
        form.bool("discoverable", app.discoverable) && public && !app.discoveryHidden,
        { form.set("discoverable", JsonPrimitive(it)) },
        t(if (app.discoveryHidden) "apps.portal.discoveryHidden" else if (public) "apps.portal.discoveryHint" else "apps.portal.discoveryNeedsPublic"),
        enabled = public && !app.discoveryHidden,
    )
    ToggleRow(t("apps.portal.codeGrant"), form.bool("bot_require_code_grant", app.botRequireCodeGrant), { form.set("bot_require_code_grant", JsonPrimitive(it)) }, t("apps.portal.codeGrantHint"))
    SectionHeading(t("apps.portal.privileged"))
    ToggleRow(t("apps.portal.presence"), intent("presence", app.intents.presence), { setIntent("presence", it) }, t("apps.portal.presenceHint"))
    ToggleRow(t("apps.portal.members"), intent("members", app.intents.members), { setIntent("members", it) }, t("apps.portal.membersHint"))
    SectionHeading(t("apps.glass.title", "app" to name))
    fish.nexa.ui.apps.ReadingGlass(name, intent("message_content", app.intents.messageContent)) { setIntent("message_content", it) }
}

private val typesSerializer = MapSerializer(String.serializer(), IntegrationTypeConfig.serializer())

@Composable
private fun InstallTab(form: PortalEditing.Form) {
    val app = form.app
    val types: Map<String, IntegrationTypeConfig> = form.draft["integration_types"]?.let { runCatching { wireJson.decodeFromJsonElement(typesSerializer, it) }.getOrNull() } ?: app.integrationTypes
    val guild = types["0"]
    val user = types["1"]
    val guildParams = guild?.installParams ?: app.installParams ?: InstallParams(listOf("bot", "applications.commands"), "0")
    fun setTypes(next: Map<String, IntegrationTypeConfig>) {
        form.set("integration_types", wireJson.encodeToJsonElement(typesSerializer, next))
        // the older single setting follows the server one, for the libraries and links that read it
        next["0"]?.installParams?.let { form.set("install_params", wireJson.encodeToJsonElement(InstallParams.serializer(), it)) }
    }
    fun toggle(k: String) {
        val next = types.toMutableMap()
        if (next.containsKey(k)) next.remove(k)
        else next[k] = if (k == "0") IntegrationTypeConfig(guildParams) else IntegrationTypeConfig(InstallParams(listOf("applications.commands"), "0"))
        if (next.isEmpty()) return
        setTypes(next)
    }
    fun setGuild(scopes: List<String> = guildParams.scopes, permissions: String = guildParams.permissions) = setTypes(types + ("0" to IntegrationTypeConfig(InstallParams(scopes, permissions))))
    val scopes = guildParams.scopes
    SectionHeading(t("apps.portal.whereAdded"), first = true)
    RadioCard(t("apps.portal.installServers"), guild != null, { toggle("0") }, t("apps.portal.installServersHint"), tick = true)
    RadioCard(t("apps.portal.installAccounts"), user != null, { toggle("1") }, t("apps.portal.installAccountsHint"), tick = true)
    SectionHeading(t("apps.portal.installLink"))
    Hint(t("apps.portal.installLinkHint"), Modifier.padding(bottom = 4.dp))
    GeneratedLink(installLink(app.protocolId))
    if (guild != null) {
        SectionHeading(t("apps.portal.serverDefaults"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(18.dp)) {
            for (sc in listOf("bot", "applications.commands")) CheckLine(sc in scopes, if (sc == "bot") t("apps.scope.bot") else t("apps.scope.commands"), sc) {
                setGuild(scopes = if (sc in scopes) scopes - sc else scopes + sc)
            }
        }
        if ("bot" in scopes) {
            Hint(t("apps.portal.permissionsHint"))
            fish.nexa.ui.apps.PermissionPicker(P.parse(guildParams.permissions), { setGuild(permissions = java.lang.Long.toUnsignedString(it)) })
        }
    }
    SectionHeading(t("apps.portal.customInstall"))
    NxTextField(
        form.str("custom_install_url", app.customInstallUrl), { form.set("custom_install_url", JsonPrimitive(it.trim())) },
        label = t("apps.portal.customInstallUrl"), hint = t("apps.portal.customInstallHint"), placeholder = "https://", keyboardType = KeyboardType.Uri, mono = true, monoSize = 13f,
    )
}

@Composable
private fun OAuthTab(form: PortalEditing.Form, onChange: (OwnedApp) -> Unit) {
    val c = Nx.colors
    val app = form.app
    var secret by remember { mutableStateOf<String?>(null) }
    val redirects = (form.draft["redirect_uris"] as? JsonArray)?.map { (it as? JsonPrimitive)?.contentOrNull ?: "" } ?: app.redirectUris
    fun setRedirects(list: List<String>) = form.set("redirect_uris", JsonArray(list.map { JsonPrimitive(it) }))
    var scopes by remember { mutableStateOf(listOf("bot", "applications.commands")) }
    var perms by remember { mutableLongStateOf(0L) }
    var redirect by remember { mutableStateOf("") }
    var userInstall by remember { mutableStateOf(false) }
    val canUser = app.integrationTypes.containsKey("1")
    val needsRedirect = scopes.any { it != "bot" && it != "applications.commands" }
    val usable = redirects.filter { it.isNotBlank() }
    val link = installLink(
        app.protocolId, scopes,
        permissions = if ("bot" in scopes) java.lang.Long.toUnsignedString(perms) else null,
        integrationType = if (userInstall) 1 else null,
        redirect = if (needsRedirect) redirect.ifEmpty { usable.firstOrNull() ?: "" }.ifEmpty { null } else null,
    )
    IdChip(t("apps.portal.clientId"), app.protocolId)
    SectionHeading(t("apps.portal.clientSecret"))
    val shownSecret = secret
    if (shownSecret != null) SecretReveal(shownSecret, t("apps.portal.secretNote")) { secret = null }
    else RowLine(if (app.hasSecret) t("apps.portal.secretSet") else t("apps.portal.secretNone"), if (app.hasSecret) t("apps.portal.resetSecret") else t("apps.portal.makeSecret")) {
        val label = if (app.hasSecret) t("apps.portal.resetSecret") else t("apps.portal.makeSecret")
        openPasswordConfirm(label, if (app.hasSecret) t("apps.portal.resetSecretBody") else t("apps.portal.makeSecretBody"), label) { key ->
            secret = Apps.resetSecret(app.id, key)
            onChange(app.copy(hasSecret = true))
        }
    }
    SectionHeading(t("apps.portal.redirects"))
    Hint(t("apps.portal.redirectsHint"), Modifier.padding(bottom = 4.dp))
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        redirects.forEachIndexed { i, u ->
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                NxTextField(
                    u, { v -> setRedirects(redirects.mapIndexed { j, x -> if (j == i) v.trim() else x }) },
                    modifier = Modifier.weight(1f), placeholder = "https://", keyboardType = KeyboardType.Uri, mono = true, monoSize = 13f,
                )
                fish.nexa.ui.components.IconButton("Trash2", t("apps.portal.removeRedirect"), { setRedirects(redirects.filterIndexed { j, _ -> j != i }) }, tint = c.icon, iconSize = 17.dp)
            }
        }
        if (redirects.size < 10) NxButton(t("apps.portal.addRedirect"), { setRedirects(redirects + "") }, kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH, icon = "Plus")
    }
    SectionHeading(t("apps.portal.linkBuilder"))
    Hint(t("apps.portal.linkBuilderHint"), Modifier.padding(bottom = 4.dp))
    if (canUser) ToggleRow(t("apps.portal.forAccounts"), userInstall, { on ->
        userInstall = on
        if (on) scopes = scopes.filter { it != "bot" && it != "webhook.incoming" }
    })
    FlowRow(horizontalArrangement = Arrangement.spacedBy(18.dp)) {
        for ((id, label) in scopeList().filter { !(userInstall && (it.first == "bot" || it.first == "webhook.incoming")) }) {
            CheckLine(id in scopes, label) { scopes = if (id in scopes) scopes - id else scopes + id }
        }
    }
    if (needsRedirect) {
        if (usable.isEmpty()) Txt(t("apps.portal.redirectNeeded"), style = Nx.type.bodySmall, color = c.textDanger)
        else fish.nexa.ui.components.SelectField(t("apps.portal.redirect"), usable.map { it to it }, redirect.ifEmpty { usable.first() }) { redirect = it }
    }
    if ("bot" in scopes) fish.nexa.ui.apps.PermissionPicker(perms, { perms = it })
    GeneratedLink(link)
}

/**
 * What an app's bot registered: its commands everywhere and for single servers,
 * as people are offered them. The owner can take one away, or all of them for
 * the bot to register again from nothing.
 */
@Composable
private fun CommandsTab(app: OwnedApp) {
    var list by remember { mutableStateOf<List<AppCommand>?>(null) }
    var failed by remember { mutableStateOf(false) }
    LaunchedEffect(app.id) {
        try {
            list = Apps.commands(app.id)
        } catch (_: Exception) {
            failed = true
        }
    }
    fun shownName(cmd: AppCommand) = if (cmd.type == AppCommands.CMD_SLASH) "/${cmd.name}" else cmd.name
    fun delete(cmd: AppCommand) = fish.nexa.ui.shell.confirm(t("apps.portal.deleteCommand"), t("apps.portal.deleteCommandBody", "name" to shownName(cmd)), t("apps.portal.delete")) {
        Apps.deleteCommand(app.id, cmd.id)
        list = list?.filter { it.id != cmd.id }
        Nexa.ui.toast(t("apps.portal.commandDeleted", "name" to shownName(cmd)), "success")
    }
    val l = list
    when {
        failed -> Hint(t("apps.portal.commandsFailed"))
        l == null -> Unit
        l.isEmpty() -> Hint(t("apps.portal.noCommands"))
        else -> Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Hint(t("apps.portal.commandsHint"))
            NxButton(t("apps.portal.deleteAllCommands"), {
                fish.nexa.ui.shell.confirm(t("apps.portal.deleteAllCommands"), t("apps.portal.deleteAllCommandsBody"), t("apps.portal.delete")) {
                    Apps.deleteAllCommands(app.id)
                    list = emptyList()
                    Nexa.ui.toast(t("apps.portal.commandsDeleted"), "success")
                }
            }, Modifier.padding(bottom = 4.dp), kind = ButtonKind.DANGER, size = ButtonSize.SM)
            for (cmd in l) {
                if (cmd.type == AppCommands.CMD_SLASH) for (leaf in AppCommands.leaves(cmd)) CommandCard("/${leaf.label}", t("apps.portal.kindSlash"), cmd, leaf.description, leaf.options.map { o -> (AppCommands.name(o) + if (o.required) "" else "?") to o.required }, shownName(cmd)) { delete(cmd) }
                else CommandCard(cmd.name, if (cmd.type == AppCommands.CMD_USER) t("apps.portal.kindUser") else t("apps.portal.kindMessage"), cmd, AppCommands.description(cmd), emptyList(), shownName(cmd)) { delete(cmd) }
            }
        }
    }
}

@Composable
private fun CommandCard(name: String, kind: String, cmd: AppCommand, description: String, options: List<Pair<String, Boolean>>, deleteName: String, onDelete: () -> Unit) {
    val c = Nx.colors
    val shape = Radii.Md
    Column(
        Modifier.fillMaxWidth().ring(shape, c.borderSubtle).background(c.bgSurface, shape).background(sheen(c), shape).topEdge(shape, c.edgeLight).padding(horizontal = 14.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            FlowRow(Modifier.weight(1f), horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(4.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                Txt(name, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(650)), color = c.textBrand)
                Txt(kind, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(600)), color = c.textMuted)
                if (cmd.guildId != null) Txt(
                    t("apps.portal.onlyIn", "server" to (cmd.guildName ?: cmd.guildId)),
                    style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(600)), color = c.textSubtle,
                    modifier = Modifier.background(c.bgHover, RoundedCornerShape(999.dp)).padding(horizontal = 8.dp, vertical = 1.dp),
                )
            }
            fish.nexa.ui.components.IconButton("Trash2", t("apps.portal.deleteCommandNamed", "name" to deleteName), onDelete, tint = c.textMuted, size = 36.dp, iconSize = 16.dp, modifier = Modifier.padding(start = 6.dp).offset(x = 6.dp, y = 0.dp))
        }
        if (description.isNotEmpty()) Txt(description, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textSubtle)
        if (options.isNotEmpty()) FlowRow(Modifier.padding(top = 2.dp), horizontalArrangement = Arrangement.spacedBy(4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            for ((o, required) in options) Txt(
                o, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = if (required) c.text else c.textMuted,
                modifier = Modifier.background(c.bgHover, RoundedCornerShape(5.dp)).padding(horizontal = 7.dp, vertical = 1.dp),
            )
        }
    }
}
