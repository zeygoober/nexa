package fish.nexa.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import coil3.compose.AsyncImage
import fish.nexa.Nexa
import fish.nexa.media.Media
import fish.nexa.net.ApiError
import fish.nexa.net.MeUser
import fish.nexa.net.Session
import fish.nexa.ui.I18n
import fish.nexa.ui.Time
import fish.nexa.ui.auth.ErrorBox
import fish.nexa.ui.auth.usernameProblem
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.AvatarColor
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxCheckbox
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.Spinner
import fish.nexa.ui.components.Txt
import fish.nexa.ui.describeError
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.rgbColor
import fish.nexa.ui.theme.sunken
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import java.text.NumberFormat

/** Runs [block] on the main thread from anywhere, so a dialog's state changes where it is drawn. */
internal fun onMain(block: suspend () -> Unit) {
    Nexa.scope.launch(Dispatchers.Main) { block() }
}

/** profileTheme(): two colours as a 135° gradient, one as itself, none as the person's own colour. */
fun profileBrush(id: String, primary: Long?, secondary: Long?): Brush = when {
    primary != null && secondary != null -> Brush.linearGradient(listOf(rgbColor(primary), rgbColor(secondary)), start = androidx.compose.ui.geometry.Offset.Zero, end = androidx.compose.ui.geometry.Offset.Infinite)
    primary != null -> androidx.compose.ui.graphics.SolidColor(rgbColor(primary))
    else -> androidx.compose.ui.graphics.SolidColor(AvatarColor.of(id))
}

/** 3.1 My Account: who you are, your password and recovery key, and deleting the account. */
@Composable
fun AccountPage(openPage: (String, String?) -> Unit) {
    val me =Nexa.users.me.collectAsState().value ?: return
    val presence by Nexa.presence.byId.collectAsState()
    PageTitle(t("settings.account.myAccount"))
    AccountCard(me, presence[me.id]?.status ?: "offline", Modifier.settingAnchor("account")) { openPage("profiles", null) }

    SettingsSeparator()
    SectionHeading(t("settings.account.passwordAndAuthentication"), Modifier.settingAnchor("password"), first = true)
    Hint(t("settings.account.yourPasswordProtectsYourAccountAnd"))
    @OptIn(ExperimentalLayoutApi::class)
    FlowRow(Modifier.fillMaxWidth().padding(top = 14.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        NxButton(t("settings.account.changePassword"), { openPasswordSheet() })
        NxButton(
            if (me.hasRecovery) t("settings.account.generateANewRecoveryKey") else t("settings.account.setUpARecoveryKey"),
            { openRecoverySheet() },
            kind = ButtonKind.SECONDARY,
        )
    }
    Hint(
        if (me.hasRecovery) t("settings.account.aRecoveryKeyIsSetGenerating") else t("settings.account.withoutARecoveryKeyAForgotten"),
        Modifier.padding(top = 6.dp),
    )

    SettingsSeparator()
    SectionHeading(t("settings.account.accountRemoval"), first = true)
    DangerBox(null, t("settings.account.deletingYourAccountIsPermanentYour"), t("settings.account.deleteAccount"), { openDeleteSheet() }, anchor = "deleteAccount")
}

/** The card at the top: banner, picture, names, and the two names' Edit buttons. */
@Composable
private fun AccountCard(me: MeUser, status: String, modifier: Modifier, editProfile: () -> Unit) {
    @Suppress("UNUSED_VARIABLE") val mediaAnimationPreferences = fish.nexa.Nexa.settings.doc.collectAsState().value
    val c = Nx.colors
    val density = LocalDensity.current
    SettingsCard(modifier) {
        Box(Modifier.fillMaxWidth().aspectRatio(2.5f).background(profileBrush(me.id, me.accentColor, me.accentColor2))) {
            if (me.bannerId != null) AsyncImage(
                model = Media.bannerUrl(me.bannerId, 600, density.density),
                imageLoader = Media.loader,
                contentDescription = null,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
            )
            Box(
                Modifier.fillMaxSize().background(
                    Brush.verticalGradient(0f to Color.White.copy(alpha = 0.06f), 0.45f to Color.Transparent, 1f to Color.Black.copy(alpha = 0.26f)),
                ),
            )
        }
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            // the picture rises into the banner by 42 dp, ringed in the card's own colour
            Box(
                Modifier
                    .layout { m, cons ->
                        val p = m.measure(cons)
                        val lift = 42.dp.roundToPx()
                        layout(p.width, p.height - lift) { p.place(0, -lift) }
                    }
                    .background(c.bgSurface, CircleShape)
                    .padding(6.dp),
            ) { Avatar(me.id, me.name, me.avatarId, 80.dp, status = status) }
            Column(Modifier.weight(1f).padding(bottom = 2.dp)) {
                fish.nexa.ui.chat.PersonName(me.id, me.name, nameStyle = me.nameStyle, style = Nx.type.heading.copy(fontSize = Nx.type.sp(18f), fontWeight = FontWeight(700)), color = c.textStrong, maxLines = 1)
                Txt(me.username, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted, maxLines = 1)
            }
        }
        Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 8.dp), horizontalArrangement = Arrangement.End) {
            NxButton(t("settings.account.editUserProfile"), editProfile, size = ButtonSize.SM)
        }
        Column(
            Modifier
                .padding(16.dp)
                .fillMaxWidth()
                .sunken(Radii.Md, c)
                .ring(Radii.Md, c.borderSubtle)
                .padding(horizontal = 14.dp, vertical = 4.dp),
        ) {
            FieldRow(t("settings.account.displayName"), me.displayName.ifEmpty { me.username }, Modifier.padding(vertical = 10.dp), personId = me.id) {
                NxButton(t("pickers.stickerPicker.edit"), { openNameSheet(username = false) }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
            }
            Box(Modifier.fillMaxWidth().size(1.dp).background(c.borderSubtle))
            FieldRow(t("settings.account.username"), me.username, Modifier.padding(vertical = 10.dp)) {
                NxButton(t("pickers.stickerPicker.edit"), { openNameSheet(username = true) }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
            }
        }
    }
}

// ---- Dialog A: a new display name or username ----

private fun openNameSheet(username: Boolean) {
    val me = Nexa.users.me.value ?: return
    val ctl = SheetCtl(t(if (username) "settings.account.changeYourUsername" else "settings.account.changeYourDisplayName"))
    var value by mutableStateOf(if (username) me.username else me.displayName)
    var error by mutableStateOf<String?>(null)
    var busy by mutableStateOf(false)
    fun submit() {
        if (busy) return
        val next = if (username) value.trim().lowercase() else value.trim()
        val problem = if (username) usernameProblem(next, canBeShort = next == me.username || me.shortNameCredits > 0) else if (next.isEmpty()) t("settings.account.displayNameCannotBeEmpty") else null
        if (problem != null) {
            error = problem
            return
        }
        busy = true
        ctl.closable = false
        onMain {
            try {
                patchMe(buildJsonObject { put(if (username) "username" else "display_name", next) })
                Nexa.ui.toast(t("settings.account.saved"), "success")
                ctl.close()
            } catch (e: Exception) {
                error = describeError(e)
            } finally {
                busy = false
                ctl.closable = true
            }
        }
    }
    openSettingsSheet("account-name", ctl, footer = {
        NxButton(t("chat.editMessage.cancel"), { it.close() }, kind = ButtonKind.GHOST, enabled = !busy)
        NxButton(t("modals.serverNotifications.done"), { submit() }, loading = busy)
    }) {
        val focus = remember { FocusRequester() }
        LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
        NxTextField(
            value,
            {
                value = it.take(32)
                error = null
            },
            modifier = Modifier.focusRequester(focus),
            label = t(if (username) "settings.account.username" else "settings.account.displayName"),
            error = error,
            hint = if (username && me.shortNameCredits > 0) t("settings.account.shortNameHint") else null,
            imeAction = ImeAction.Done,
            onIme = { submit() },
            capitalize = !username,
        )
    }
}

// ---- Dialog B: a new password ----

private fun openPasswordSheet() {
    val ctl = SheetCtl(t("settings.account.updateYourPassword"))
    var current by mutableStateOf("")
    var next by mutableStateOf("")
    var again by mutableStateOf("")
    var error by mutableStateOf<String?>(null)
    var busy by mutableStateOf(false)
    fun ready() = current.isNotEmpty() && next.length >= 8 && next == again
    fun submit() {
        if (busy || !ready()) return
        busy = true
        error = null
        ctl.closable = false
        onMain {
            try {
                Nexa.auth.changePassword(current, next)
                Nexa.ui.toast(t("settings.account.passwordChangedOtherDevicesWereLogged"), "success")
                ctl.close()
            } catch (e: Exception) {
                error = describeError(e)
            } finally {
                busy = false
                ctl.closable = true
            }
        }
    }
    openSettingsSheet("account-password", ctl, footer = {
        NxButton(t("chat.editMessage.cancel"), { it.close() }, kind = ButtonKind.GHOST, enabled = !busy)
        NxButton(t("modals.serverNotifications.done"), { submit() }, loading = busy, enabled = ready())
    }) {
        val focus = remember { FocusRequester() }
        LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
        error?.let { ErrorBox(it) }
        NxTextField(current, { current = it }, Modifier.focusRequester(focus), label = t("settings.account.currentPassword"), required = true, password = true)
        NxTextField(next, { next = it }, label = t("settings.account.newPassword"), required = true, password = true, hint = t("settings.account.atLeast8Characters"))
        NxTextField(
            again, { again = it },
            label = t("settings.account.confirmNewPassword"), required = true, password = true,
            error = if (again.isNotEmpty() && again != next) t("settings.account.passwordsDoNotMatch") else null,
            imeAction = ImeAction.Done, onIme = { submit() },
        )
    }
}

// ---- Dialog C: a recovery key, asked for with the password, then shown once ----

private fun openRecoverySheet() {
    val me = Nexa.users.me.value ?: return
    val ctl = SheetCtl(t("settings.account.recoveryKey"))
    var password by mutableStateOf("")
    var error by mutableStateOf<String?>(null)
    var busy by mutableStateOf(false)
    var key by mutableStateOf<String?>(null)
    fun submit() {
        if (busy || password.isEmpty()) return
        busy = true
        error = null
        ctl.closable = false
        onMain {
            try {
                val k = Nexa.auth.newRecoveryKey(password)
                password = ""
                key = k
                // shown once: no swiping it away, only Done
                ctl.title = t("register.saveYourRecoveryKey")
                ctl.footer = false
                ctl.closable = false
            } catch (e: Exception) {
                error = describeError(e)
                ctl.closable = true
            } finally {
                busy = false
            }
        }
    }
    openSettingsSheet("account-recovery", ctl, footer = {
        NxButton(t("chat.editMessage.cancel"), { it.close() }, kind = ButtonKind.GHOST, enabled = !busy)
        NxButton(t("settings.account.generate"), { submit() }, loading = busy, enabled = password.isNotEmpty())
    }) { sheet ->
        val k = key
        if (k == null) {
            val focus = remember { FocusRequester() }
            LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
            error?.let { ErrorBox(it) }
            Hint(t("settings.account.confirmYourPasswordToGenerateA"))
            NxTextField(password, { password = it }, Modifier.focusRequester(focus), label = t("chat.lockedChannel.password"), required = true, password = true, imeAction = ImeAction.Done, onIme = { submit() })
        } else KeyToSave(
            value = k,
            fileName = "nexa-recovery-key-${me.username}.txt",
            fileText = t("register.recoveryKeyFileHeading", "username" to me.username) + "\n\n" + k + "\n\n" + t("register.keepThisSomewhereSafe") + "\n",
            warning = t("settings.account.newKeyWarning"),
            confirm = t("register.iHaveSavedMyRecoveryKey"),
            copyFailed = t("register.copyFailed"),
            downloadLabel = t("register.downloadTxt"),
            onDone = { sheet.close() },
        )
    }
}

/**
 * RecoveryKeyPanel: a key (or codes) shown once, to copy or save as a file,
 * with a warning, a box to tick, and Done only once it is ticked.
 */
@Composable
internal fun KeyToSave(
    value: String,
    fileName: String,
    fileText: String,
    warning: String,
    confirm: String,
    copyFailed: String,
    downloadLabel: String,
    onDone: () -> Unit,
    display: String = value.replace("-", "-​"),
    copied: String = value,
    copyFor: Long = 2000,
    warningFirst: Boolean = false,
) {
    val c = Nx.colors
    val ctx = LocalContext.current
    var saved by remember { mutableStateOf(false) }
    var didCopy by remember { mutableStateOf(false) }
    LaunchedEffect(didCopy) {
        if (didCopy) {
            kotlinx.coroutines.delay(copyFor)
            didCopy = false
        }
    }
    val save = rememberTextFileSaver()
    if (warningFirst) NoticeBox(warning, c.danger, icon = "Smartphone")
    MonoBox(display, center = !warningFirst)
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        NxButton(
            if (didCopy) t("modals.groupInvite.copied") else t("modals.groupInvite.copy"),
            {
                if (copyText(ctx, copied, sensitive = true)) didCopy = true else Nexa.ui.toast(copyFailed, "error")
            },
            kind = ButtonKind.SECONDARY, block = true, icon = if (didCopy) "Check" else "Copy", modifier = Modifier.weight(1f),
        )
        NxButton(downloadLabel, { save(fileName, fileText) }, kind = ButtonKind.SECONDARY, block = true, icon = "Download", modifier = Modifier.weight(1f))
    }
    if (!warningFirst) NoticeBox(warning, c.warning)
    NxCheckbox(saved, { saved = it }, confirm)
    NxButton(t("modals.serverNotifications.done"), onDone, size = ButtonSize.LG, block = true, enabled = saved)
}

// ---- Dialog D: deleting the account ----

private fun openDeleteSheet() {
    val ctl = SheetCtl(t("settings.account.deleteAccount"))
    var password by mutableStateOf("")
    var error by mutableStateOf<String?>(null)
    var busy by mutableStateOf(false)
    fun submit() {
        if (busy || password.isEmpty()) return
        busy = true
        error = null
        ctl.closable = false
        onMain {
            try {
                Nexa.auth.deleteAccount(password)
                ctl.close()
            } catch (e: Exception) {
                error = describeError(e)
                ctl.closable = true
            } finally {
                busy = false
            }
        }
    }
    openSettingsSheet("account-delete", ctl, footer = {
        NxButton(t("chat.editMessage.cancel"), { it.close() }, kind = ButtonKind.GHOST, enabled = !busy)
        NxButton(t("settings.account.deleteAccount"), { submit() }, kind = ButtonKind.DANGER, loading = busy, enabled = password.isNotEmpty())
    }) {
        val c = Nx.colors
        // null while asking, an empty object when the server could not say
        var counts by remember { mutableStateOf<JsonObject?>(null) }
        LaunchedEffect(Unit) {
            counts = try {
                Nexa.api.get("/users/@me/delete-preview") as? JsonObject ?: JsonObject(emptyMap())
            } catch (_: Exception) {
                JsonObject(emptyMap())
            }
        }
        Txt(t("settings.account.purge.title"), style = Nx.type.body.copy(fontWeight = FontWeight(600)), color = c.textStrong)
        val n = counts
        if (n == null) Box(Modifier.fillMaxWidth().padding(vertical = 8.dp), contentAlignment = Alignment.Center) { Spinner(size = 22.dp) }
        else PurgeList(n)
        Txt(t("settings.account.purge.note"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
        error?.let { ErrorBox(it) }
        val focus = remember { FocusRequester() }
        LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
        NxTextField(password, { password = it }, Modifier.focusRequester(focus), label = t("chat.lockedChannel.password"), required = true, password = true, imeAction = ImeAction.Done, onIme = { submit() })
    }
}

/** What deleting takes with it, one row per thing there is any of. */
@Composable
private fun PurgeList(n: JsonObject) {
    val c = Nx.colors
    val fmt = remember(I18n.tag) { NumberFormat.getInstance(I18n.locale) }
    fun num(k: String) = (n[k] as? JsonPrimitive)?.longOrNull ?: 0L
    val rows = buildList {
        if (num("messages") > 0) add(t("settings.account.purge.messages") to fmt.format(num("messages")))
        if (num("attachments") > 0) add(t("settings.account.purge.files") to "${fmt.format(num("attachments"))} · ${Time.bytes(num("attachment_bytes"))}")
        if (num("owned_servers") > 0) add(t("settings.account.purge.ownedServers") to fmt.format(num("owned_servers")))
        if (num("owned_groups") > 0) add(t("settings.account.purge.ownedGroups") to fmt.format(num("owned_groups")))
        if (num("direct_messages") > 0) add(t("settings.account.purge.dms") to fmt.format(num("direct_messages")))
        if (num("notes") > 0) add(t("settings.account.purge.notes") to fmt.format(num("notes")))
        if (num("reactions") > 0) add(t("settings.account.purge.reactions") to fmt.format(num("reactions")))
        if (num("packs") > 0) add(t("settings.account.purge.packs") to fmt.format(num("packs")))
        val leaves = maxOf(0, num("servers") - num("owned_servers")) + maxOf(0, num("groups") - num("owned_groups"))
        if (leaves > 0) add(t("settings.account.purge.leaves") to fmt.format(leaves))
        add(t("settings.account.purge.profile") to "")
    }
    Column(
        Modifier
            .fillMaxWidth()
            .background(c.danger.copy(alpha = 0.06f), Radii.Md)
            .ring(Radii.Md, c.danger.copy(alpha = 0.3f))
            .padding(horizontal = 14.dp, vertical = 6.dp),
    ) {
        rows.forEachIndexed { i, (label, value) ->
            if (i > 0) Box(Modifier.fillMaxWidth().size(1.dp).background(c.borderSubtle))
            Row(Modifier.fillMaxWidth().padding(vertical = 9.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Txt(label, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(19f)), color = c.text, modifier = Modifier.weight(1f))
                if (value.isNotEmpty()) Txt(value, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1)
            }
        }
    }
}

// ---------------------------------------------------------------------------

/** What a signed-in device is, for its name and icon (lib/deviceLabel.ts describeDevice). */
data class DeviceInfo(val label: String, val icon: String)

private val ANDROID_APP = Regex("NexaAndroid/[^\\s(]*\\s*\\(([^)]*)\\)")

fun describeDevice(ua: String): DeviceInfo {
    if (ua.isEmpty()) return DeviceInfo(t("settings.devices.unknownDevice"), "MonitorSmartphone")
    // the Android app says so first, with the phone's model, which is what its owner recognises
    if (ua.contains("NexaAndroid/")) {
        val parts = (ANDROID_APP.find(ua)?.groupValues?.get(1) ?: "").split(";").map { it.trim() }.filter { it.isNotEmpty() }
        val release = parts.firstOrNull { Regex("^Android\\b", RegexOption.IGNORE_CASE).containsMatchIn(it) }
        val model = parts.firstOrNull { it != release && !Regex("^(Linux|U|wv|K)$", RegexOption.IGNORE_CASE).matches(it) }
        return DeviceInfo(t("settings.devices.androidAppOn", "device" to (model ?: release ?: "Android")), "Smartphone")
    }
    // the desktop app is a browser inside, but nobody thinks of it as Chrome
    val browser = when {
        ua.contains("Electron/") -> t("settings.desktop.desktopApp")
        ua.contains("Edg/") -> "Edge"
        ua.contains("OPR/") -> "Opera"
        ua.contains("Chrome/") -> "Chrome"
        ua.contains("Safari/") && !ua.contains("Chrome") -> "Safari"
        ua.contains("Firefox/") -> "Firefox"
        else -> t("settings.devices.browser")
    }
    val os = when {
        ua.contains("iPhone") || ua.contains("iPad") -> "iOS"
        ua.contains("Android") -> "Android"
        ua.contains("Mac OS X") -> "macOS"
        ua.contains("Windows") -> "Windows"
        ua.contains("Linux") -> "Linux"
        else -> ""
    }
    // an Android tablet's browser leaves "Mobile" out of its user agent
    val icon = when {
        ua.contains("iPad") || ua.contains("Tablet") || (ua.contains("Android") && !ua.contains("Mobile")) -> "Tablet"
        Regex("iPhone|iPod|Android|Mobi").containsMatchIn(ua) -> "Smartphone"
        os.isNotEmpty() -> "Monitor"
        else -> "MonitorSmartphone"
    }
    return DeviceInfo(if (os.isNotEmpty()) t("settings.devices.browserOnSystem", "browser" to browser, "system" to os) else browser, icon)
}

/** 3.4 Devices: every signed-in session, this one first, each but this one to log out. */
@Composable
fun DevicesPage() {
    val scope = rememberCoroutineScope()
    var list by remember { mutableStateOf<List<Session>?>(null) }
    var failed by remember { mutableStateOf(false) }
    var sweeping by remember { mutableStateOf(false) }
    var dropping by remember { mutableStateOf<String?>(null) }
    suspend fun load() {
        try {
            list = Nexa.api.get("/auth/sessions", ListSerializer(Session.serializer()))
            failed = false
        } catch (_: Exception) {
            list = null
            failed = true
        }
    }
    LaunchedEffect(Unit) { load() }
    fun drop(id: String) {
        if (dropping != null) return
        dropping = id
        scope.launch {
            try {
                Nexa.api.request("DELETE", "/auth/sessions/$id")
                load()
            } catch (e: Exception) {
                Nexa.ui.toast(describeError(e, t("settings.devices.couldNotLogThatDeviceOut")), "error")
            } finally {
                dropping = null
            }
        }
    }
    val others = list?.count { !it.current } ?: 0
    // logging out one of nine must not look like logging out all nine, so failures are counted out loud
    fun sweep() = fish.nexa.ui.shell.confirm(
        t("settings.devices.logOutAllOtherDevices"),
        t("settings.devices.everyOtherDeviceCountWillHave", "count" to others),
        t("settings.devices.logThemOut"),
    ) {
        sweeping = true
        try {
            var failures = 0
            for (s in list.orEmpty()) {
                if (s.current) continue
                try {
                    Nexa.api.request("DELETE", "/auth/sessions/${s.id}")
                } catch (_: Exception) {
                    failures++
                }
            }
            load()
            if (failures > 0) Nexa.ui.toast(t("settings.devices.someDevicesCouldNotBeLogged", "count" to failures), "error")
            else Nexa.ui.toast(t("settings.devices.everyOtherDeviceIsSignedOut"), "success")
        } finally {
            sweeping = false
        }
    }

    PageTitle(t("settings.devices.devices"))
    Hint(t("settings.devices.everyDeviceThatIsLoggedInto"), Modifier.settingAnchor("sessions"))
    if (others > 0) Box(Modifier.padding(top = 16.dp)) {
        DangerBox(null, t("settings.devices.otherDevicesCount", "count" to others), t("settings.devices.logOutAllOtherDevices"), { sweep() }, loading = sweeping)
    }
    Column(Modifier.fillMaxWidth().padding(top = 16.dp)) {
        when {
            failed -> LoadFailed(t("settings.devices.yourDevicesCouldNotBeLoaded")) { scope.launch { load() } }
            list == null -> Hint(t("chat.pins.loading"))
            else -> for (s in list.orEmpty().sortedWith(compareByDescending<Session> { it.current }.thenByDescending { it.lastSeenAt })) {
                val d = describeDevice(s.userAgent)
                val sub = buildString {
                    if (s.ip.isNotEmpty()) append(s.ip).append(" · ")
                    append(t("settings.devices.signedIn", "date" to Time.dateDivider(s.createdAt)))
                    append(" · ")
                    append(if (s.current) t("settings.devices.activeNow") else t("settings.devices.lastActive", "when" to Time.relative(s.lastSeenAt)))
                }
                DeviceRow(
                    d.label, sub, d.icon, current = s.current,
                    badge = if (s.current) t("settings.devices.thisDevice") else null,
                    actions = if (s.current) null else {
                        { NxButton(t("settings.devices.logOut"), { drop(s.id) }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, loading = dropping == s.id) }
                    },
                )
            }
        }
    }
}

/** LoadError: what could not be loaded, and Try Again. */
@Composable
fun LoadFailed(text: String, retry: () -> Unit) {
    val c = Nx.colors
    Column(
        Modifier
            .fillMaxWidth()
            .background(c.danger.copy(alpha = 0.06f), Radii.Md)
            .ring(Radii.Md, c.danger.copy(alpha = 0.35f))
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Txt(text, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text)
        NxButton(t("chat.messageList.tryAgain"), retry, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
    }
}
