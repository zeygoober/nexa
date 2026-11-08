package fish.nexa.ui.moderation

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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import fish.nexa.data.ModAlt
import fish.nexa.data.ModUser
import fish.nexa.data.Moderation
import fish.nexa.ui.Icon
import fish.nexa.ui.Time
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.describeError
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx

/** "Alt Accounts: 5" (AltAccounts.tsx `AltBadge`) on an account that shares an address with others; opens the list. */
@Composable
fun AltBadge(u: ModUser) {
    // alt accounts are for admins and above
    if (u.altCount <= 0 || !fish.nexa.data.StaffRanks.can(fish.nexa.data.StaffCan.SEE_ALTS_AND_CALLS)) return
    val label = t("moderation.alts.badge", "count" to u.altCount)
    Box(
        Modifier
            .heightIn(min = 34.dp)
            .pressable(remember { MutableInteractionSource() }) { openAlts(u) }
            .semantics(mergeDescendants = true) { role = Role.Button; contentDescription = label },
        contentAlignment = Alignment.Center,
    ) { Flag(label, "alt", "Link2") }
}

/**
 * Every account seen at one of the same addresses as this one, with how many
 * they share (AltAccountsModal). People on one network show here too, so the
 * sheet says so.
 */
fun openAlts(u: ModUser) {
    var alts by mutableStateOf<List<ModAlt>?>(null)
    var error by mutableStateOf<String?>(null)
    openModSheet(
        "alts",
        top = { close ->
            ModSheetTop(t("moderation.alts.title", "name" to u.displayName), close, titleWords = fish.nexa.ui.chat.PersonWords(t("moderation.alts.title", "name" to fish.nexa.ui.chat.PERSON_SLOT), listOf(u.id to u.displayName)), leading = { Avatar(u.id, u.displayName, u.avatarId, 40.dp) }) {
                Txt("@" + u.username, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f)), color = Nx.colors.textMuted, maxLines = 1)
            }
        },
        foot = { close ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) { NxButton(t("moderation.alts.close"), close, kind = ButtonKind.GHOST) }
        },
    ) { close ->
        val c = Nx.colors
        LaunchedEffect(Unit) {
            try {
                alts = Moderation.alts(u.id)
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                error = describeError(e, t("moderation.loadFailed"))
            }
        }
        NoteBox(t("moderation.alts.explain"))
        error?.let { e -> DangerBox { Txt(e, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text) } }
        val list = alts
        when {
            list == null && error == null -> LoadingRow()
            list != null && list.isEmpty() -> Txt(t("moderation.alts.none"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
            list != null -> Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                for (a in list) AltRow(a) {
                    close()
                    goPanel("users/${a.user.id}")
                }
            }
        }
    }
}

/** `.altRow`: one account, how many addresses it shares and when it was last at one. */
@Composable
private fun AltRow(a: ModAlt, onClick: () -> Unit) {
    val c = Nx.colors
    val shape = RoundedCornerShape(14.dp)
    Row(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .pressable(remember { MutableInteractionSource() }, onClick = onClick)
            .heightIn(min = 52.dp)
            .padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Avatar(a.user.id, a.user.displayName, a.user.avatarId, 36.dp)
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                fish.nexa.ui.chat.PersonName(a.user.id, a.user.displayName, style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), fontWeight = FontWeight(650)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f, fill = false))
                if (a.user.ban != null) Icon("Ban", c.danger, size = 13.dp)
                if (a.user.mute != null) Icon("MessageSquareOff", c.warning, size = 13.dp)
            }
            Txt("@" + a.user.username, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, maxLines = 1)
        }
        Column(horizontalAlignment = Alignment.End) {
            Txt(t("moderation.alts.shared", "count" to a.sharedIps), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f), fontWeight = FontWeight(650)), color = c.textWarning, maxLines = 1)
            Txt(t("moderation.alts.lastShared", "when" to Time.relative(a.lastSharedAt)), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted, maxLines = 1)
        }
    }
}

/**
 * What only the owner can read about an account (web AccountDetailsModal).
 * Asking for it writes a line in the log, so it is asked for only when this
 * sheet opens, never with the page.
 */
fun openAccountDetails(u: ModUser) {
    var data by mutableStateOf<fish.nexa.data.ModAccount?>(null)
    var error by mutableStateOf<String?>(null)
    val title = t("moderation.account.title", "name" to u.displayName)
    openModSheet(
        "account",
        top = { close ->
            ModSheetTop(title, close, titleWords = fish.nexa.ui.chat.PersonWords(t("moderation.account.title", "name" to fish.nexa.ui.chat.PERSON_SLOT), listOf(u.id to u.displayName)), leading = { Avatar(u.id, u.displayName, u.avatarId, 40.dp) }) {
                Txt("@" + u.username, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f)), color = Nx.colors.textMuted, maxLines = 1)
            }
        },
        foot = { close ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) { NxButton(t("moderation.alts.close"), close, kind = ButtonKind.GHOST) }
        },
    ) {
        val c = Nx.colors
        LaunchedEffect(Unit) {
            try {
                data = Moderation.account(u.id)
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                error = describeError(e, t("moderation.loadFailed"))
            }
        }
        NoteBox(t("moderation.account.note"))
        error?.let { e -> DangerBox { Txt(e, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.text) } }
        val d = data
        if (d == null && error == null) LoadingRow()
        if (d != null) {
            val sub = d.subscription
            Column(Modifier.fillMaxWidth().background(c.bgInput, RoundedCornerShape(10.dp)).padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                AccountFact(t("moderation.account.joined"), Time.shortDate(d.createdAt))
                AccountFact(t("moderation.account.plan"), fish.nexa.ui.support.planName(d.plan))
                AccountFact(
                    t("moderation.account.subscription"),
                    if (sub != null) "${fish.nexa.ui.support.planName(sub.plan)} (${sub.status.replace('_', ' ')})" else t("moderation.account.noSubscription"),
                    sub?.takeIf { it.periodEnd > 0 }?.let { t(if (it.cancelAtEnd) "moderation.account.ends" else "moderation.account.renews", "when" to Time.shortDate(it.periodEnd)) },
                )
            }
            Txt(t("moderation.account.noEmail"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted)
            AccountList(t("moderation.account.addresses")) {
                if (d.addresses.isEmpty()) Txt(t("moderation.account.noAddresses"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
                for (a in d.addresses) AccountRecord(null, a.ip, t("moderation.account.seen", "first" to Time.shortDate(a.firstSeenAt), "last" to Time.relative(a.lastSeenAt)))
            }
            AccountList(t("moderation.account.sessions")) {
                if (d.sessions.isEmpty()) Txt(t("moderation.account.noSessions"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted)
                for (s in d.sessions) AccountRecord(fish.nexa.ui.settings.describeDevice(s.userAgent).label, s.ip, t("moderation.account.session", "first" to Time.shortDate(s.createdAt), "last" to Time.relative(s.lastSeenAt)))
            }
        }
    }
}

/** A small heading with its records close under it, the way the web's dialog sets them. */
@Composable
private fun AccountList(title: String, body: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Txt(title.uppercase(), Modifier.padding(start = 2.dp, bottom = 6.dp), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(700), letterSpacing = 0.5.sp), color = Nx.colors.textMuted)
        body()
    }
}

@Composable
private fun AccountFact(label: String, value: String, small: String? = null) {
    val c = Nx.colors
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Txt(label, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(650)), color = c.textMuted)
        Txt(value, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textStrong)
        small?.let { Txt(it, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted) }
    }
}

/** One address or device, its address in the code face, with a button that copies it. */
@Composable
private fun AccountRecord(name: String?, ip: String, line: String) {
    val c = Nx.colors
    val context = androidx.compose.ui.platform.LocalContext.current
    val copy = t("moderation.account.copyIp")
    Row(Modifier.fillMaxWidth().background(c.bgInput, RoundedCornerShape(10.dp)).padding(start = 12.dp, top = 8.dp, bottom = 8.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
            name?.let { Txt(it, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(600)), color = c.textStrong) }
            Txt(ip, style = Nx.type.bodySmall.copy(fontFamily = Nx.type.mono, fontSize = Nx.type.sp(13f)), color = c.text)
            Txt(line, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12.5f)), color = c.textMuted)
        }
        Box(
            Modifier.size(44.dp).clip(RoundedCornerShape(10.dp))
                .pressable(remember { MutableInteractionSource() }) { fish.nexa.ui.settings.copyAndSay(context, ip, t("moderation.copied")) }
                .semantics { role = Role.Button; contentDescription = copy },
            contentAlignment = Alignment.Center,
        ) { Icon("Copy", c.textMuted, size = 18.dp) }
    }
}
