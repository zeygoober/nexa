package fish.nexa.ui.discovery

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.ui.Icon
import fish.nexa.ui.components.Avatar
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.ring
import fish.nexa.ui.theme.topEdge
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** A bot on Discover (web api/discovery.ts ListedApp); its id is its bot's. */
@Serializable
private data class ListedApp(
    val id: String,
    val name: String = "",
    val description: String = "",
    @SerialName("icon_id") val iconId: String? = null,
    @SerialName("avatar_id") val avatarId: String? = null,
    val tags: List<String> = emptyList(),
    val servers: Int = 0,
    @SerialName("reads_all") val readsAll: Boolean = false,
)

@Serializable
private data class ListedApps(
    val bots: List<ListedApp> = emptyList(),
    @SerialName("has_more") val hasMore: Boolean = false,
    @SerialName("next_offset") val nextOffset: Int? = null,
)

/** Discover's bots (BotDiscovery.tsx): the public apps whose developers listed them. */
@Composable
internal fun BotDiscovery() {
    val c = Nx.colors
    var q by remember { mutableStateOf("") }
    var sort by remember { mutableStateOf("popular") }
    var list by remember { mutableStateOf<List<ListedApp>?>(null) }
    var next by remember { mutableStateOf<Int?>(null) }
    var failed by remember { mutableStateOf(false) }
    var reload by remember { mutableIntStateOf(0) }
    var generation by remember { mutableIntStateOf(0) }
    val scope = rememberCoroutineScope()
    suspend fun load(offset: Int) {
        val mine = ++generation
        failed = false
        try {
            val parts = buildList {
                add("sort=$sort")
                add("limit=24")
                if (q.isNotBlank()) add("q=" + java.net.URLEncoder.encode(q.trim(), "UTF-8"))
                if (offset > 0) add("offset=$offset")
            }
            val page = Nexa.api.get("/discovery/bots?" + parts.joinToString("&"), ListedApps.serializer())
            if (mine != generation) return
            list = if (offset > 0 && list != null) list!! + page.bots else page.bots
            next = if (page.hasMore) page.nextOffset else null
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            if (mine == generation) failed = true
        }
    }
    LaunchedEffect(q, sort, reload) {
        ++generation
        if (q.isNotEmpty()) delay(300)
        load(0)
    }
    LazyColumn(
        Modifier.fillMaxSize().navigationBarsPadding(),
        contentPadding = PaddingValues(start = 12.dp, end = 12.dp, top = 14.dp, bottom = 32.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        item("lead") {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Txt(t("discovery.bots.heading"), style = Nx.type.title.copy(fontSize = Nx.type.sp(26f), lineHeight = Nx.type.sp(29f)), color = c.textStrong)
                Txt(t("discovery.bots.lead"), style = Nx.type.body.copy(fontSize = Nx.type.sp(15f)), color = c.textMuted)
            }
        }
        item("search") { fish.nexa.ui.server.SearchBox(q, { q = it }, t("discovery.bots.search")) }
        item("sorts") { SortStrip(sort, listOf("popular", "newest"), { t("discovery.packs.sort.$it") }, t("discovery.bots.sortLabel")) { sort = it } }
        val l = list
        when {
            failed && l == null -> item("failed") {
                Column(Modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Txt(t("discovery.bots.failed"), style = Nx.type.body, color = c.textMuted, align = TextAlign.Center)
                    NxButton(t("chat.messageList.tryAgain"), { reload += 1 }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
                }
            }
            l == null -> item("loading") { Txt(t("discovery.packs.loading"), style = Nx.type.body, color = c.textMuted, align = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(24.dp)) }
            l.isEmpty() -> item("empty") {
                Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 40.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Icon("Bot", c.textMuted, size = 36.dp)
                    Txt(t(if (q.isNotBlank()) "discovery.bots.noMatches" else "discovery.bots.none"), style = Nx.type.heading, color = c.textStrong, align = TextAlign.Center)
                    if (q.isBlank()) Txt(t("discovery.bots.noneHint"), style = Nx.type.body, color = c.textMuted, align = TextAlign.Center, modifier = Modifier.widthIn(max = 360.dp))
                }
            }
            else -> items(l, key = { it.id }) { BotCard(it) }
        }
        next?.let { offset ->
            item("more") {
                Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                    NxButton(t("discovery.packs.more"), { scope.launch { load(offset) } }, kind = ButtonKind.SECONDARY, size = ButtonSize.SM)
                }
            }
        }
    }
}

/** A bot on Discover: what it is, whether it reads everything, and the way to add it (the consent page says the rest). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun BotCard(a: ListedApp) {
    val c = Nx.colors
    val shape = Radii.Lg
    val pill = RoundedCornerShape(999.dp)
    Column(
        Modifier.fillMaxWidth().ring(shape, c.borderSubtle).clip(shape).background(c.bgRaised, shape).topEdge(shape, c.edgeLight).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Avatar(a.id, a.name, a.iconId ?: a.avatarId, 56.dp, icon = a.iconId != null, shape = RoundedCornerShape(30))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Txt(a.name, style = Nx.type.name.copy(fontSize = Nx.type.sp(17f), fontWeight = FontWeight(700)), color = c.textStrong, maxLines = 1)
                Txt(t("apps.portal.inServers", "count" to a.servers), style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(13f)), color = c.textMuted, maxLines = 1)
            }
        }
        if (a.description.isNotEmpty()) Txt(a.description, style = Nx.type.bodySmall.copy(fontSize = Nx.type.sp(14f), lineHeight = Nx.type.sp(20f)), color = c.textSubtle, maxLines = 3)
        if (a.tags.isNotEmpty()) FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            for (tag in a.tags) Txt(tag, style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(600)), color = c.textSubtle, maxLines = 1, modifier = Modifier.background(c.bgActive, pill).padding(horizontal = 8.dp, vertical = 3.dp))
        }
        if (a.readsAll) Row(Modifier.background(c.warning.copy(alpha = 0.14f), pill).padding(horizontal = 8.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
            Icon("Eye", c.textWarning, size = 13.dp)
            Txt(t("moderation.bots.readsAll"), style = Nx.type.hint.copy(fontSize = Nx.type.sp(12f), fontWeight = FontWeight(600)), color = c.textWarning, maxLines = 1)
        }
        NxButton(t("apps.profile.addApp"), { fish.nexa.ui.apps.openAuthorize("client_id=${a.id}", fromApp = true) }, Modifier.fillMaxWidth(), icon = "Plus", size = ButtonSize.TOUCH)
    }
}
