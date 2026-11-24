package fish.nexa.ui.moderation

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.crypto.MessagePayload
import fish.nexa.data.OwnerMessages
import fish.nexa.net.Message
import fish.nexa.net.wireJson
import fish.nexa.ui.Time
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.Txt
import fish.nexa.ui.media.Attachments
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.jsonArray

@Composable
fun OwnerMessagesTab() {
    var before by remember { mutableStateOf("") }
    var attempt by remember { mutableIntStateOf(0) }
    var page by remember { mutableStateOf<List<Pair<Message, MessagePayload?>>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var failed by remember { mutableStateOf(false) }
    LaunchedEffect(before, attempt) {
        loading = true
        failed = false
        page = emptyList()
        try {
            val rows = Nexa.api.get("/moderation/messages?before=$before").jsonArray
            page = withContext(Dispatchers.Default) { rows.map {
                val m = wireJson.decodeFromJsonElement(Message.serializer(), it)
                m to runCatching { OwnerMessages.read(m) }.getOrNull()
            } }
        } catch (e: CancellationException) { throw e }
        catch (_: Exception) { failed = true }
        finally { loading = false }
    }
    val c = Nx.colors
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PanelPad, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { HistoryReviews(owner = true) }
        when {
            loading -> item { LoadingRow() }
            failed -> item { LoadFailed({ attempt++ }, false) }
            page.isEmpty() -> item { EmptyState("ScrollText", t("moderation.noMatches")) }
        }
        items(page, key = { it.first.id }) { (m, payload) ->
            Column(Modifier.fillMaxWidth().modCard(c).padding(14.dp)) {
                Txt(t("ownerMessages.source", "author" to m.authorId, "channel" to m.channelId) + " · " + Time.timestamp(m.createdAt), color = c.textMuted)
                if (payload == null) Txt(t("ownerMessages.cannotOpen"))
                else ReviewedPayload(payload)
            }
        }
        if (page.size == 50) item { NxButton(t("moderation.trades.loadMore"), { before = page.last().first.id }) }
        if (before.isNotEmpty()) item { NxButton(t("ownerMessages.newest"), { before = "" }) }
    }
}
