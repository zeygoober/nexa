package fish.nexa.ui.moderation

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.data.ModApp
import fish.nexa.data.Moderation
import fish.nexa.ui.Icon
import fish.nexa.ui.Time
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.components.pressable
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private const val PAGE = 48

/**
 * Every app on Nexa (web BotsTab.tsx): its picture, name and description, who
 * owns it, where it is added and whether it reads every message it can see.
 * Its bot is an account, moderated from its own page like anyone's.
 */
@Composable
fun BotsTab() {
    val scope = rememberCoroutineScope()
    var q by remember { mutableStateOf("") }
    var sort by remember { mutableStateOf("newest") }
    val apps = remember { mutableStateListOf<ModApp>() }
    var loaded by remember { mutableStateOf(false) }
    var total by remember { mutableStateOf<Int?>(null) }
    var more by remember { mutableStateOf(false) }
    var loading by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    var request by remember { mutableStateOf(0) }

    fun load(offset: Int) {
        val id = ++request
        loading = true
        failed = false
        scope.launch {
            try {
                val r = Moderation.apps(q, sort, offset, PAGE)
                if (id != request) return@launch
                if (offset == 0) apps.clear()
                apps.addAll(r.apps.filter { x -> apps.none { it.id == x.id } })
                total = r.total
                more = r.more
                loaded = true
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                if (id == request) failed = true
            } finally {
                if (id == request) loading = false
            }
        }
    }
    val list = rememberLazyListState()
    // a new search or order starts the list again, from its top (typing waits a moment)
    LaunchedEffect(q, sort) {
        request++
        more = false
        if (q.isNotEmpty()) delay(250)
        loaded = false
        apps.clear()
        list.scrollToItem(0)
        load(0)
    }
    val nearEnd by remember { derivedStateOf { list.layoutInfo.visibleItemsInfo.lastOrNull()?.index?.let { it >= list.layoutInfo.totalItemsCount - 4 } == true } }
    LaunchedEffect(nearEnd, more, loading, failed) { if (nearEnd && more && !loading && !failed && loaded) load(apps.size) }

    LazyColumn(Modifier.fillMaxSize(), state = list, contentPadding = PanelPad, verticalArrangement = Arrangement.spacedBy(18.dp)) {
        item("tools") {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    SearchField(q, { q = it }, t("moderation.bots.search"), Modifier.weight(1f))
                    total?.let { Txt(t("moderation.bots.total", "count" to it), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = Nx.colors.textMuted, maxLines = 1) }
                }
                SortField(listOf("newest" to t("moderation.sort.newest"), "oldest" to t("moderation.sort.oldest"), "servers" to t("moderation.bots.sortServers"), "name" to t("moderation.sort.name")), sort) { sort = it }
            }
        }
        when {
            !loaded && !failed -> item("loading") { LoadingRow() }
            failed && apps.isEmpty() -> item("failed") { LoadFailed({ load(0) }, loading) }
            apps.isEmpty() -> item("empty") {
                EmptyState("Bot", if (q.isNotEmpty()) t("moderation.noMatches") else t("moderation.bots.none")) {
                    if (q.isNotEmpty()) NxButton(t("moderation.clearFilters"), { q = "" }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
                }
            }
            else -> items(apps, key = { it.id }) { a -> AppCard(a) { changed -> apps[apps.indexOfFirst { it.id == changed.id }] = changed } }
        }
        if (loading && loaded) item("more") { LoadingRow() }
    }
}

/** `.card` for an app: a banner of its colour, its picture on it, its name, owner, description, reach, and Moderate. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AppCard(a: ModApp, onChange: (ModApp) -> Unit) {
    val c = Nx.colors
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf(false) }
    val owner = person(a.ownerId)
    val bot = person(a.id)
    val open = { goPanel("users/${a.id}") }
    Box(Modifier.fillMaxWidth().modCard(c, if (a.disabled) "banned" else null).pressable(remember { MutableInteractionSource() }) { open() }.semantics { contentDescription = a.name }) {
        Column(Modifier.fillMaxWidth()) {
            CardBanner(null, profileBrush(null, null, a.id), 88.dp)
            Column(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 44.dp, bottom = 16.dp), verticalArrangement = Arrangement.spacedBy(7.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Txt(a.name, style = Nx.type.name.copy(fontSize = Nx.type.sp(17.5f), fontWeight = FontWeight(690)), color = c.textStrong, maxLines = 1, modifier = Modifier.weight(1f, fill = false))
                    fish.nexa.ui.media.AppTag()
                }
                // the one thing about an app the staff most need to see at a glance
                if (a.readsAll) Row(
                    Modifier.background(c.warning.copy(alpha = 0.14f), RoundedCornerShape(999.dp)).padding(horizontal = 8.dp, vertical = 2.dp),
                    verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp),
                ) {
                    Icon("Eye", c.textWarning, size = 13.dp)
                    Txt(t("moderation.bots.readsAll"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(600)), color = c.textWarning, maxLines = 1)
                }
                OwnerChip(a.ownerId, t("moderation.servers.ownedBy", "name" to fish.nexa.ui.chat.PERSON_SLOT), ownerName = nameOf(owner)) { goPanel("users/${a.ownerId}") }
                Txt(
                    if (a.description.isNotEmpty()) previewText(a.description) else t("moderation.servers.noDescription"),
                    style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13.5f), lineHeight = Nx.type.sp(18.9f), fontStyle = if (a.description.isEmpty()) FontStyle.Italic else FontStyle.Normal),
                    color = if (a.description.isEmpty()) c.textPlaceholder else c.textSubtle, maxLines = 2,
                    modifier = Modifier.padding(top = 2.dp).heightIn(min = 37.8.dp),
                )
                FlowRow(Modifier.padding(top = 4.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    MetaItem("Server", t("apps.portal.inServers", "count" to a.servers))
                    MetaItem("UserRound", t("apps.portal.onAccounts", "count" to a.accounts))
                    MetaItem("CalendarDays", Time.shortDate(a.createdAt))
                }
                Row(Modifier.padding(top = 8.dp).fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                NxButton(t("moderation.moderate"), open, Modifier.weight(1f), icon = "Gavel", size = ButtonSize.TOUCH)
                if (a.listed || a.hidden) NxButton(
                    t(if (a.hidden) "moderation.bots.allow" else "moderation.bots.hide"),
                    {
                        val hide = !a.hidden
                        busy = true
                        scope.launch {
                            try {
                                Moderation.setAppDiscovery(a.id, hide)
                                onChange(a.copy(hidden = hide, listed = !hide && a.public))
                            } catch (e: Exception) {
                                if (e is kotlinx.coroutines.CancellationException) throw e
                                Nexa.ui.toast(t("moderation.failed"), "error")
                            } finally {
                                busy = false
                            }
                        }
                    },
                    Modifier.weight(1f), kind = ButtonKind.SECONDARY, size = ButtonSize.TOUCH, loading = busy,
                )
                }
                if (a.disabled) Txt(t("moderation.bots.disabled"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f)), color = c.textMuted, modifier = Modifier.padding(top = 6.dp))
            }
        }
        Box(Modifier.padding(start = 14.dp, top = 44.dp).background(cardFill(c), RoundedCornerShape(30)).padding(5.dp)) {
            Avatar(a.id, a.name, a.iconId ?: bot?.avatarId, 64.dp, icon = a.iconId != null, shape = RoundedCornerShape(30))
        }
    }
}
