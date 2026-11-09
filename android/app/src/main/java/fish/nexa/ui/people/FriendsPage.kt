package fish.nexa.ui.people

import fish.nexa.ui.profile.copyBotIdItem
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.People
import fish.nexa.data.RequestNote
import fish.nexa.net.PublicUser
import fish.nexa.net.Relationship
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.IconButton
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.Pick
import fish.nexa.ui.components.PickAction
import fish.nexa.ui.components.PickBar
import fish.nexa.ui.components.PickMark
import fish.nexa.ui.components.TabStrip
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.dialogs.Dialogs
import fish.nexa.ui.layers.MenuItem
import fish.nexa.ui.layers.openMenu
import fish.nexa.ui.nav.Nav
import fish.nexa.ui.nav.Route
import fish.nexa.ui.profile.ProfileLinks
import fish.nexa.ui.profile.StatusLine
import fish.nexa.ui.profile.callWith
import fish.nexa.ui.profile.confirmBlockPeople
import fish.nexa.ui.profile.confirmRemoveFriend
import fish.nexa.ui.profile.confirmRemoveFriends
import fish.nexa.ui.profile.copyWithToast
import fish.nexa.ui.profile.hasStatusLine
import fish.nexa.ui.profile.openBlockDialog
import fish.nexa.ui.profile.openDmWith
import fish.nexa.ui.shell.NavButton
import fish.nexa.ui.shell.OwnerMarks
import fish.nexa.ui.shell.openProfile
import fish.nexa.ui.shell.presenceWord
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.boxShadow
import fish.nexa.ui.theme.brandGradient
import fish.nexa.ui.theme.dangerGradient
import fish.nexa.ui.theme.positiveGradient
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.ringGlow
import fish.nexa.ui.theme.shadowLow
import fish.nexa.ui.theme.sheen
import fish.nexa.ui.theme.sunken
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.launch

private enum class Tab { ONLINE, ALL, PENDING, BLOCKED, ADD }

private val PRESENCE_ORDER = mapOf("online" to 0, "idle" to 1, "dnd" to 2)

/** sortRelationships: by name in the reader's language; Online by who can answer soonest; requests to answer first. */
private fun sorted(list: List<Relationship>, tab: Tab): List<Relationship> {
    val collator = java.text.Collator.getInstance(fish.nexa.ui.I18n.locale).apply { strength = java.text.Collator.PRIMARY }
    val byName = Comparator<Relationship> { a, b ->
        collator.compare(a.user.displayName, b.user.displayName).takeIf { it != 0 } ?: collator.compare(a.user.username, b.user.username)
    }
    return list.sortedWith { a, b ->
        if (tab == Tab.ONLINE) {
            val d = (PRESENCE_ORDER[Nexa.presence.statusOf(a.user.id)] ?: 3) - (PRESENCE_ORDER[Nexa.presence.statusOf(b.user.id)] ?: 3)
            if (d != 0) return@sortedWith d
        }
        if (tab == Tab.PENDING && a.type != b.type) return@sortedWith if (a.type == "incoming") -1 else 1
        byName.compare(a, b)
    }
}

/**
 * FriendsPage.tsx on a phone: the title row with Add Friend at its end, the
 * tab strip under it, then the search, the list and what it is empty of. A
 * new account sees how to begin instead.
 */
@Composable
fun FriendsPage() {
    val c = Nx.colors
    var tab by rememberSaveable { mutableStateOf(Tab.ONLINE) }
    var query by rememberSaveable { mutableStateOf("") }
    val rels by Nexa.relationships.byId.collectAsState()
    val presences by Nexa.presence.byId.collectAsState()
    val users by Nexa.users.byId.collectAsState()
    @Suppress("UNUSED_VARIABLE") val touch = presences.size + users.size
    val q = query.trim().lowercase().removePrefix("@")
    val all = rels.values.map { r -> r.copy(user = users[r.user.id] ?: r.user) }
    fun match(r: Relationship) = q.isEmpty() || r.user.displayName.lowercase().contains(q) || r.user.username.contains(q)
    val list = sorted(
        when (tab) {
            Tab.ONLINE -> all.filter { it.type == "friend" && Nexa.presence.statusOf(it.user.id) != "offline" && match(it) }
            Tab.ALL -> all.filter { it.type == "friend" && match(it) }
            Tab.PENDING -> all.filter { (it.type == "incoming" || it.type == "outgoing") && match(it) }
            Tab.BLOCKED -> all.filter { it.type == "blocked" && match(it) }
            Tab.ADD -> emptyList()
        },
        tab,
    )
    // nobody yet, once the session has said so (not while it is still connecting)
    val ready by Nexa.session.ready.collectAsState()
    val alone = ready && rels.isEmpty()
    val pendingCount = rels.values.count { it.type == "incoming" }
    val picking = Pick.picking("friends")
    val order = list.map { it.user.id }

    // another tab is another list; whoever left this one is let go of
    LaunchedEffect(tab) { if (Pick.list == "friends") Pick.clear() }
    LaunchedEffect(order) { if (Pick.list == "friends") Pick.drop(Pick.ids.filter { it !in order }) }

    val tabLabel = when (tab) {
        Tab.ONLINE -> t("shell.user.online")
        Tab.ALL -> t("friends.all")
        Tab.PENDING -> t("friends.pending")
        Tab.BLOCKED -> t("friends.blocked")
        Tab.ADD -> ""
    }

    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            FriendsHeader(tab, pendingCount) { tab = it }
            if (tab == Tab.ADD) {
                Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(start = 16.dp, end = 12.dp, top = 12.dp, bottom = 16.dp)) { AddFriend() }
            } else {
                LazyColumn(Modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 16.dp, end = 12.dp, top = 12.dp, bottom = if (picking) 150.dp else 16.dp)) {
                    item("search") { SearchField(query) { query = it } }
                    item("section") {
                        Row(Modifier.fillMaxWidth().padding(bottom = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            Txt(
                                "$tabLabel (${list.size})",
                                style = Nx.type.label.copy(fontSize = Nx.type.sp(12.5f), fontWeight = FontWeight(700)),
                                color = c.textMuted,
                                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
                            )
                            if (list.isNotEmpty() && !picking) Txt(
                                t("pick.select"),
                                style = Nx.type.label.copy(fontWeight = FontWeight(600)),
                                color = c.textMuted,
                                modifier = Modifier.clip(Radii.Xs).pressable(remember { MutableInteractionSource() }) { Pick.start("friends") }.padding(horizontal = 12.dp, vertical = 10.dp),
                            )
                        }
                    }
                    if (list.isEmpty()) item("empty") {
                        if (alone && q.isEmpty() && (tab == Tab.ONLINE || tab == Tab.ALL)) FirstRun { tab = Tab.ADD }
                        else Empty(tab, query.trim()) { query = "" }
                    }
                    items(list, key = { it.user.id }) { r -> FriendRow(r, picking) }
                }
            }
        }
        if (picking && tab != Tab.ADD) {
            val (bar, menu) = friendsBulk(Pick.ids.toList())
            PickBar(t("pick.friendsBar"), bar + PickAction("MoreHorizontal", t("pick.more")) { openMenu(menu, t("pick.count", "count" to Pick.ids.size)) }, Modifier.align(Alignment.BottomCenter))
        }
    }
}

/** The header: the menu button, the title, the report button; the tab strip below it. */
@Composable
private fun FriendsHeader(tab: Tab, pending: Int, onTab: (Tab) -> Unit) {
    val c = Nx.colors
    Column(
        Modifier
            .fillMaxWidth()
            .boxShadow(RoundedCornerShape(0.dp), 8f, 16f, c.shadeAt(0.9f), spread = -14f)
            .background(c.bgRaised)
            .background(sheen(c))
            .padding(horizontal = 18.dp),
    ) {
        Row(Modifier.fillMaxWidth().height(52.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            NavButton()
            Row(Modifier.weight(1f), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(9.dp)) {
                Icon("Users", c.textBrand, size = 22.dp)
                Txt(t("sidebar.dMSidebar.friends"), style = Nx.type.headerTitle.copy(fontSize = Nx.type.sp(16.5f), fontWeight = FontWeight(680)), color = c.textStrong, maxLines = 1)
                // after the title, as the web's header orders it
                IconButton("Lightbulb", t("suggestions.suggest"), { fish.nexa.ui.bugs.openBugReport("friends", "suggestion") }, tint = c.icon, size = 40.dp, iconSize = 22.dp)
                IconButton("Bug", t("bugs.report"), { fish.nexa.ui.bugs.openBugReport("friends") }, tint = c.icon, size = 40.dp, iconSize = 22.dp)
            }
            AddTab(selected = tab == Tab.ADD) { onTab(Tab.ADD) }
        }
        TabStrip(
            listOf(
                Tab.ONLINE to t("shell.user.online"),
                Tab.ALL to t("friends.all"),
                Tab.PENDING to (t("friends.pending") + if (pending > 0) " ($pending)" else ""),
                Tab.BLOCKED to t("friends.blocked"),
            ),
            selected = tab,
            label = t("sidebar.dMSidebar.friends"),
            modifier = Modifier.padding(bottom = 10.dp),
            onSelect = onTab,
        )
    }
}

/** Add Friend: the one action among the tabs, lit in the accent; chosen, a tinted chip. */
@Composable
private fun AddTab(selected: Boolean, onClick: () -> Unit) {
    val c = Nx.colors
    val shape = RoundedCornerShape(9.dp)
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier
            .graphicsLayer { val s = if (pressed) 0.97f else 1f; scaleX = s; scaleY = s }
            .height(38.dp)
            .then(
                if (selected) Modifier.ring(shape, c.brand.copy(alpha = 0.4f)).background(c.brand.copy(alpha = 0.16f), shape)
                else Modifier.boxShadow(shape, 4f, 12f, c.brand.copy(alpha = 0.8f), spread = -4f).background(brandGradient(c), shape).topEdge(shape, Color.White.copy(alpha = 0.22f)),
            )
            .clip(shape)
            .pressable(interaction, role = Role.Tab, onClick = onClick)
            .semantics { this.selected = selected }
            .padding(horizontal = 12.dp),
        contentAlignment = Alignment.Center,
    ) {
        Txt(t("profile.profileCard.addFriend"), style = Nx.type.button.copy(fontSize = Nx.type.sp(14f), fontWeight = FontWeight(650)), color = if (selected) c.textBrand else c.onBrand, maxLines = 1)
    }
}

@Composable
private fun SearchField(query: String, onChange: (String) -> Unit) {
    val c = Nx.colors
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val shape = Radii.Sm
    Box(
        Modifier
            .padding(bottom = 20.dp)
            .fillMaxWidth()
            .height(42.dp)
            .then(if (focused) Modifier.ringGlow(shape, c) else Modifier)
            .sunken(shape, c)
            .border(1.dp, if (focused) c.focusRing.copy(alpha = 0.75f) else c.borderSubtle, shape),
        contentAlignment = Alignment.CenterStart,
    ) {
        BasicTextField(
            query, onChange,
            Modifier.fillMaxWidth().padding(start = 14.dp, end = 42.dp).semantics { contentDescription = t("friends.searchFriends") },
            textStyle = Nx.type.input.copy(fontSize = Nx.type.sp(15.5f), color = c.text),
            singleLine = true,
            cursorBrush = SolidColor(c.focusRing),
            interactionSource = interaction,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search, capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false),
            decorationBox = { inner ->
                Box(contentAlignment = Alignment.CenterStart) {
                    if (query.isEmpty()) Txt(t("chat.chat.search"), style = Nx.type.input.copy(fontSize = Nx.type.sp(15.5f)), color = c.textPlaceholder)
                    inner()
                }
            },
        )
        Icon("Search", c.textMuted, size = 20.dp, modifier = Modifier.align(Alignment.CenterEnd).padding(end = 13.dp))
    }
}

@Composable
private fun Empty(tab: Tab, query: String, onClear: () -> Unit) {
    val c = Nx.colors
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 64.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Icon("Users", c.textMuted, size = 48.dp)
        val text = if (query.isNotEmpty()) t("friends.nobodyHereMatches", "query" to query) else when (tab) {
            Tab.ONLINE -> t("friends.nobodyIsOnlineRightNow")
            Tab.ALL -> t("friends.noFriendsYet")
            Tab.PENDING -> t("friends.thereAreNoPendingFriendRequests")
            else -> t("friends.youHaveNotBlockedAnyone")
        }
        Txt(text, style = Nx.type.body, color = c.textMuted, align = TextAlign.Center)
        if (query.isNotEmpty()) NxButton(t("friends.clearTheSearch"), onClear, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
    }
}

/** The first screen of a new account: the two ways in, and your username to send to someone. */
@Composable
private fun FirstRun(onAdd: () -> Unit) {
    val c = Nx.colors
    Column(Modifier.fillMaxWidth().padding(top = 40.dp, start = 8.dp, end = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        val shape = RoundedCornerShape(24.dp)
        Box(
            Modifier
                .padding(bottom = 20.dp)
                .graphicsLayer { rotationZ = -4f }
                .size(76.dp)
                .boxShadow(shape, 18f, 40f, c.brand.copy(alpha = 0.85f), spread = -14f)
                .background(brandGradient(c), shape)
                .topEdge(shape, Color.White.copy(alpha = 0.24f)),
            contentAlignment = Alignment.Center,
        ) { Icon("Users", c.onBrand, size = 32.dp) }
        Txt(t("friends.firstRunTitle"), style = Nx.type.title.copy(fontSize = Nx.type.sp(24f), fontWeight = FontWeight(690), lineHeight = Nx.type.sp(28.8f)), color = c.textStrong, align = TextAlign.Center, modifier = Modifier.padding(bottom = 8.dp))
        Txt(t("friends.firstRunBody"), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f), lineHeight = Nx.type.sp(21.75f)), color = c.textMuted, align = TextAlign.Center, modifier = Modifier.widthIn(max = 460.dp).padding(bottom = 20.dp))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            NxButton(t("profile.profileCard.addFriend"), onAdd, icon = "UserPlus")
            NxButton(t("friends.createAServer"), { Dialogs.createServer() }, kind = ButtonKind.SECONDARY, icon = "Plus")
        }
        YourUsername(centred = true)
    }
}

/** "Your username is @name. Send it to someone so they can add you." with a copy button. */
@Composable
private fun YourUsername(centred: Boolean) {
    val c = Nx.colors
    val me by Nexa.users.me.collectAsState()
    val username = me?.username ?: return
    FlowRow(
        Modifier.fillMaxWidth().padding(top = 16.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp, if (centred) Alignment.CenterHorizontally else Alignment.Start),
        verticalArrangement = Arrangement.Center,
        itemVerticalAlignment = Alignment.CenterVertically,
    ) {
        Txt(t("friends.yourUsername", "username" to "@$username"), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = c.textMuted, align = if (centred) TextAlign.Center else TextAlign.Start)
        IconButton("Copy", t("friends.copyYourUsername"), { copyWithToast(username, t("chat.groupMemberMenu.copied")) }, tint = c.textMuted, size = 40.dp, iconSize = 14.dp)
    }
}

/** A person in a tab: picture, name, how they are or where things stand; the buttons that fit. */
@Composable
private fun FriendRow(rel: Relationship, picking: Boolean) {
    val c = Nx.colors
    val user = rel.user
    val picked = Pick.isPicked("friends", user.id)
    val busy = People.isBusy(user.id)
    val status = Nexa.presence.statusOf(user.id)
    val name = user.displayName.ifEmpty { user.username }
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val note = remember(rel.note) { if (rel.type == "incoming") RequestNote.open(rel.note) else null }
    val shape = Radii.Md

    fun menu() {
        if (Pick.list == "friends" && user.id in Pick.ids && Pick.ids.size > 1) {
            openMenu(friendsBulk(Pick.ids.toList()).second, t("pick.count", "count" to Pick.ids.size))
            return
        }
        if (Pick.list == "friends" && !Pick.mode && Pick.ids.isNotEmpty()) Pick.clear()
        val friend = rel.type == "friend"
        val items = ArrayList<MenuItem>()
        items += MenuItem.Action(t("profile.profile.profile")) { openProfile(user.id) }
        items += MenuItem.Action(t("profile.profileCard.message"), separatorAfter = !friend) { openDmWith(user.id) }
        if (friend) {
            items += MenuItem.Action(t("chat.chat.startVoiceCall"), separatorAfter = true) { callWith(user.id) }
            items += MenuItem.Action(t("profile.profileCard.removeFriend"), danger = true) { confirmRemoveFriend(user) }
        }
        items += if (rel.type != "blocked") MenuItem.Action(t("profile.profileCard.block"), danger = true) { openBlockDialog(user.id) }
        else MenuItem.Action(t("friends.unblock")) { Nexa.scope.launch { People.unblock(user.id) } }
        items += MenuItem.Action(t("profileLink.copy")) { ProfileLinks.copy(user.username) }
        items += MenuItem.Action(t("chat.groupMemberMenu.copyUserId")) { copyWithToast(user.id, t("friends.userIdCopied")) }
        items += copyBotIdItem(user.id, separatorAfter = true)
        items += MenuItem.Action(t("pick.select")) { Pick.start("friends", user.id) }
        openMenu(items)
    }

    Row(
        Modifier
            .padding(bottom = 3.dp)
            .fillMaxWidth()
            .heightIn(min = if (note != null) 62.dp else 64.dp)
            .then(
                when {
                    picked -> Modifier.ring(shape, c.brand.copy(alpha = 0.3f)).background(c.brand.copy(alpha = 0.14f), shape)
                    pressed -> Modifier.ring(shape, c.borderSubtle).background(c.bgHover, shape).background(sheen(c), shape).topEdge(shape, c.edgeLight)
                    else -> Modifier
                },
            )
            .clip(shape)
            .pressable(interaction, onLongClick = ::menu) {
                if (picking) Pick.toggle("friends", user.id) else openProfile(user.id)
            }
            .semantics { if (picking) role = Role.Checkbox }
            .padding(horizontal = 10.dp, vertical = if (note != null) 8.dp else 0.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (picking) PickMark(picked, t("pick.pickNamed", "name" to name))
        Avatar(user.id, name, user.avatarId, 32.dp, status = if (rel.type == "friend") status else null)
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                fish.nexa.ui.chat.PersonName(user.id, name, nameStyle = user.nameStyle, style = Nx.type.body.copy(fontWeight = FontWeight(650), lineHeight = Nx.type.sp(20f)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f, fill = false))
                OwnerMarks(user.id)
                user.serverTag?.takeIf { it.text.isNotEmpty() }?.let { fish.nexa.ui.chat.ServerTagChip(it, small = true) }
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                val subStyle = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(18f))
                Txt("${user.username} · ", style = subStyle, color = c.textMuted, maxLines = 1, modifier = Modifier.widthIn(max = 160.dp))
                if (rel.type == "friend" && hasStatusLine(user.id)) StatusLine(user.id, subStyle, c.textMuted, Modifier.weight(1f))
                else Txt(
                    when (rel.type) {
                        "incoming" -> t("friends.incomingFriendRequest")
                        "outgoing" -> t("friends.outgoingFriendRequest")
                        "blocked" -> t("friends.blocked")
                        else -> presenceWord(status)
                    },
                    style = subStyle, color = c.textMuted, maxLines = 1,
                )
            }
            if (note != null) Row(Modifier.padding(top = 4.dp).height(androidx.compose.foundation.layout.IntrinsicSize.Min)) {
                Box(Modifier.width(2.dp).fillMaxHeight().background(c.border))
                Txt(note, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(18f)), color = c.text, maxLines = 3, modifier = Modifier.padding(start = 8.dp))
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            when (rel.type) {
                "friend" -> {
                    RoundAction("MessageSquare", t("profile.profileCard.message")) { openDmWith(user.id) }
                    RoundAction("Phone", t("chat.chat.startVoiceCall")) { callWith(user.id) }
                    RoundAction("MoreVertical", t("chat.chat.more"), onClick = ::menu)
                }
                "incoming" -> {
                    RoundAction("Check", t("profile.profileCard.acceptFriendRequest"), kind = "ok", enabled = !busy) {
                        Nexa.scope.launch { People.accept(user)?.let { Nav.go(Route.Dm(it.id)) } }
                    }
                    RoundAction("X", t("people.menu.ignoreRequest"), kind = "no", enabled = !busy) { Nexa.scope.launch { People.dropRequest(user.id) } }
                }
                "outgoing" -> RoundAction("X", t("people.menu.cancelRequest"), kind = "no", enabled = !busy) { Nexa.scope.launch { People.dropRequest(user.id) } }
                else -> RoundAction("X", t("friends.unblock"), kind = "no", enabled = !busy) { Nexa.scope.launch { People.unblock(user.id) } }
            }
        }
    }
}

/** A 40 dp lit circle; accept-type ones press to the positive fill, refuse-type to the danger fill. */
@Composable
private fun RoundAction(icon: String, label: String, kind: String? = null, enabled: Boolean = true, onClick: () -> Unit) {
    val c = Nx.colors
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier
            .graphicsLayer { val s = if (pressed) 0.92f else 1f; scaleX = s; scaleY = s; alpha = if (enabled) 1f else 0.55f }
            .size(40.dp)
            .ring(CircleShape, c.borderSubtle)
            .then(
                when {
                    pressed && kind == "ok" -> Modifier.background(positiveGradient(c), CircleShape)
                    pressed && kind == "no" -> Modifier.background(dangerGradient(c), CircleShape)
                    else -> Modifier.background(c.bgSurface, CircleShape).background(sheen(c), CircleShape)
                },
            )
            .topEdge(CircleShape, c.edgeLight)
            .clip(CircleShape)
            .pressable(interaction, enabled = enabled, onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, when {
            pressed && kind == "ok" -> c.onPositive
            pressed && kind == "no" -> c.onDanger
            else -> c.textSubtle
        }, size = 20.dp)
    }
}

/** friendsBulk: what picking several people offers, the first three in the bar. */
private fun friendsBulk(ids: List<String>): Pair<List<PickAction>, List<MenuItem>> {
    val byUser = Nexa.relationships.byId.value
    fun of(type: String): List<PublicUser> = ids.mapNotNull { byUser[it] }.filter { it.type == type }.map { Nexa.users.get(it.user.id) ?: it.user }
    val friends = of("friend")
    val incoming = of("incoming")
    val outgoing = of("outgoing")
    val blocked = of("blocked")
    val blockable = ids.mapNotNull { byUser[it] }.filter { it.type != "blocked" }.map { it.user }
    val bar = ArrayList<PickAction>()
    val menu = ArrayList<MenuItem>()
    fun add(a: PickAction, inBar: Boolean) {
        if (inBar) bar += a
        menu += MenuItem.Action(a.label, danger = a.danger, onClick = a.onClick)
    }
    if (friends.isNotEmpty()) add(PickAction("MessageSquarePlus", t("pick.groupChatWith", "count" to friends.size)) { Dialogs.createDm(initial = friends.map { it.id }) }, true)
    if (incoming.isNotEmpty()) {
        add(PickAction("Check", t("pick.acceptRequests", "count" to incoming.size)) { Nexa.scope.launch { People.acceptMany(incoming); Pick.clear() } }, true)
        add(PickAction("X", t("pick.ignoreRequests", "count" to incoming.size), danger = true) { Nexa.scope.launch { People.dropMany(incoming); Pick.clear() } }, true)
    }
    if (outgoing.isNotEmpty()) add(PickAction("X", t("pick.cancelRequests", "count" to outgoing.size), danger = true) { Nexa.scope.launch { People.dropMany(outgoing); Pick.clear() } }, true)
    if (blocked.isNotEmpty()) add(PickAction("ShieldOff", t("pick.unblockPeople", "count" to blocked.size)) { Nexa.scope.launch { People.unblockMany(blocked); Pick.clear() } }, true)
    if (friends.isNotEmpty()) add(PickAction("UserMinus", t("pick.removeFriends", "count" to friends.size), danger = true) { confirmRemoveFriends(friends) }, true)
    if (blockable.isNotEmpty()) add(PickAction("ShieldOff", t("pick.block", "count" to blockable.size), danger = true) { confirmBlockPeople(blockable) }, false)
    return bar.take(3) to menu
}

/** The Add Friend tab: a username, an optional note sealed to them, what came of it. */
@Composable
private fun AddFriend() {
    val c = Nx.colors
    val scope = rememberCoroutineScope()
    var value by rememberSaveable { mutableStateOf("") }
    var note by rememberSaveable { mutableStateOf("") }
    var noteOpen by rememberSaveable { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    var msg by remember { mutableStateOf<Pair<Boolean, String>?>(null) }

    fun submit() {
        val username = value.trim().removePrefix("@").lowercase()
        if (username.isEmpty() || busy) return
        busy = true
        msg = null
        scope.launch {
            try {
                val (type, user) = People.requestByName(username)
                val name = user.displayName.ifEmpty { user.username }
                msg = true to if (type == "friend") t("friends.youAreNowFriends", "name" to name) else t("friends.friendRequestSent", "name" to name)
                value = ""
                val text = RequestNote.clean(note)
                if (text.isNotEmpty() && type == "outgoing") People.addRequestNote(user, text)
                note = ""
                noteOpen = false
            } catch (e: Exception) {
                msg = false to People.requestError(e)
            } finally {
                busy = false
            }
        }
    }

    Column(Modifier.widthIn(max = 720.dp)) {
        Txt(t("profile.profileCard.addFriend"), style = Nx.type.title.copy(fontSize = Nx.type.sp(22f), fontWeight = FontWeight(690), lineHeight = Nx.type.sp(26.4f)), color = c.textStrong, modifier = Modifier.padding(top = 4.dp, bottom = 8.dp))
        Txt(t("friends.youCanAddFriendsWithTheir"), style = Nx.type.body.copy(fontSize = Nx.type.sp(14.5f), lineHeight = Nx.type.sp(21f)), color = c.textSubtle, modifier = Modifier.padding(bottom = 16.dp))
        val interaction = remember { MutableInteractionSource() }
        val focused by interaction.collectIsFocusedAsState()
        val shape = Radii.Md
        Column(
            Modifier
                .fillMaxWidth()
                .then(if (focused) Modifier.ringGlow(shape, c) else Modifier)
                .sunken(shape, c)
                .border(1.dp, if (focused) c.focusRing.copy(alpha = 0.75f) else c.borderSubtle, shape)
                .padding(start = 12.dp, end = 12.dp, top = 6.dp, bottom = 12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon("UserPlus", c.textMuted, size = 20.dp)
                BasicTextField(
                    value, { value = it },
                    Modifier.weight(1f).height(40.dp).semantics { contentDescription = t("settings.account.username") },
                    textStyle = Nx.type.input.copy(color = c.text),
                    singleLine = true,
                    cursorBrush = SolidColor(c.focusRing),
                    interactionSource = interaction,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Send, capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false),
                    keyboardActions = KeyboardActions(onSend = { submit() }),
                    decorationBox = { inner ->
                        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.CenterStart) {
                            if (value.isEmpty()) Txt(t("friends.addFriendPlaceholder"), style = Nx.type.input, color = c.textPlaceholder, maxLines = 1)
                            inner()
                        }
                    },
                )
            }
            NxButton(t("friends.sendFriendRequest"), ::submit, size = ButtonSize.SM, block = true, loading = busy, enabled = value.isNotBlank())
        }
        if (noteOpen) {
            Column(Modifier.padding(top = 16.dp)) {
                NxTextField(
                    note, { note = it.take(RequestNote.MAX) },
                    label = t("friends.requestNote"),
                    placeholder = t("friends.requestNotePlaceholder"),
                    singleLine = false, minLines = 2, capitalize = true, autoCorrect = true,
                )
                Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    Txt(t("friends.requestNoteHint"), style = Nx.type.hint, color = c.textMuted, modifier = Modifier.weight(1f))
                    Txt("${note.length}/${RequestNote.MAX}", style = Nx.type.hint.copy(fontFeatureSettings = "tnum"), color = c.textMuted)
                }
            }
        } else {
            Box(Modifier.padding(top = 8.dp)) {
                NxButton(t("friends.addANote"), { noteOpen = true }, kind = ButtonKind.GHOST, size = ButtonSize.SM, icon = "MessageSquarePlus")
            }
        }
        msg?.let { (ok, text) ->
            Txt(text, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f)), color = if (ok) c.positive else c.textDanger, modifier = Modifier.padding(top = 8.dp).semantics { liveRegion = if (ok) LiveRegionMode.Polite else LiveRegionMode.Assertive })
        }
        YourUsername(centred = false)
        Spacer(Modifier.height(8.dp))
    }
}
