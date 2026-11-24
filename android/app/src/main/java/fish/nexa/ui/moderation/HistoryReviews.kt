package fish.nexa.ui.moderation

import androidx.compose.foundation.layout.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.crypto.Envelope
import fish.nexa.crypto.MessagePayload
import fish.nexa.data.OwnerMessages
import fish.nexa.net.Message
import fish.nexa.net.wireJson
import fish.nexa.ui.Time
import fish.nexa.ui.components.*
import fish.nexa.ui.media.Attachments
import fish.nexa.ui.theme.Nx
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerialName
import kotlinx.serialization.json.*

@Serializable
private data class Review(
    val id: String, val reason: String, val status: String,
    @SerialName("channel_id") val channel: String,
    @SerialName("owner_id") val owner: String,
    @SerialName("first_id") val first: String,
    @SerialName("first_approved") val firstApproved: Boolean,
    @SerialName("second_approved") val secondApproved: Boolean,
    @SerialName("expires_at") val expires: Long,
    @SerialName("include_files") val files: Boolean,
    val items: List<JsonObject>,
)

private const val OwnerReviews = "/moderation/history-reviews"
private const val MemberReviews = "/users/@me/history-reviews"

@Composable
fun HistoryReviews(owner: Boolean = false) {
    val scope = rememberCoroutineScope()
    var version by remember { mutableIntStateOf(0) }
    var before by remember { mutableStateOf("") }
    var reviews by remember { mutableStateOf(emptyList<Review>()) }
    var error by remember { mutableStateOf("") }
    var expanded by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    LaunchedEffect(version, before, owner) {
        error = ""
        reviews = emptyList()
        try {
            val path = if (owner) OwnerReviews else MemberReviews
            reviews = Nexa.api.get("$path?before=$before").jsonArray.map { wireJson.decodeFromJsonElement(Review.serializer(), it) }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) { error = e.message.orEmpty() }
    }
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (owner) {
            NxButton("Request historical messages", { expanded = !expanded })
            if (expanded) {
                var channel by remember { mutableStateOf("") }
                var ids by remember { mutableStateOf("") }
                var reason by remember { mutableStateOf("") }
                var files by remember { mutableStateOf(false) }
                Txt("Both people in a direct message must approve. Long selections are divided into batches of 50 for approval.")
                NxTextField(channel, { channel = it }, label = "DM channel ID")
                NxTextField(ids, { ids = it }, label = "Message IDs", hint = "Leave empty for all earlier messages in this DM.")
                NxTextField(reason, { reason = it.take(500) }, label = "Investigation reason")
                NxCheckbox(files, { files = it }, "Include attachments")
                NxButton("Request approval", {
                    busy = true
                    scope.launch {
                        try {
                            Nexa.api.request("POST", OwnerReviews, buildJsonObject {
                                put("channel_id", channel.trim())
                                put("reason", reason)
                                put("include_files", files)
                                put("message_ids", JsonArray(ids.trim().split(Regex("[,\\s]+")).filter(String::isNotEmpty).map(::JsonPrimitive)))
                            })
                            expanded = false
                            before = ""
                            version++
                        } catch (e: CancellationException) { throw e }
                        catch (e: Exception) { error = e.message.orEmpty() }
                        finally { busy = false }
                    }
                }, enabled = !busy && reason.isNotBlank() && channel.isNotBlank())
            }
        }
        for (review in reviews) key(review.id, version) { ReviewCard(review, owner) { version++ } }
        if (reviews.size == 50) NxButton("Older requests", { before = reviews.last().id })
        if (before.isNotEmpty()) NxButton("Newest requests", { before = "" })
        if (error.isNotEmpty()) Txt(error)
        if (reviews.isNotEmpty() || error.isNotEmpty()) NxButton("Refresh requests", { version++ }, kind = ButtonKind.GHOST)
    }
}

@Composable
private fun ReviewCard(r: Review, owner: Boolean, refresh: () -> Unit) {
    val scope = rememberCoroutineScope()
    val me = Nexa.users.me.value?.id ?: return
    var entries by remember { mutableStateOf(emptyList<Pair<Message, MessagePayload>>()) }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }
    fun act(action: String) {
        if (busy) return
        busy = true
        error = ""
        scope.launch {
            try {
                when (action) {
                    "view", "preview" -> {
                        val raw = if (owner) Nexa.api.get("$OwnerReviews/${r.id}/messages") else Nexa.api.request("POST", "$MemberReviews/${r.id}/preview")
                        entries = raw.jsonArray.map {
                            val m = wireJson.decodeFromJsonElement(Message.serializer(), it)
                            val p = if (owner) OwnerMessages.read(m) else requireNotNull(Nexa.messages.decrypt(m).payload) { "This device could not open the selection." }
                            m to if (r.files) p else p.copy(att = null)
                        }
                    }
                    "share" -> {
                        require(entries.size == r.items.size)
                        val boxes = buildJsonObject {
                            for ((m, p) in entries) {
                                val env = requireNotNull(Envelope.from(m.ciphertext))
                                val copy = OwnerMessages.attach(env, r.owner, m.authorId, m.channelId, p, me)
                                put(m.id, copy.getValue("owner").jsonObject.getValue("box"))
                            }
                        }
                        Nexa.api.request("POST", "$MemberReviews/${r.id}/share", boxes)
                        refresh()
                    }
                    else -> { Nexa.api.request("POST", "$MemberReviews/${r.id}/$action"); refresh() }
                }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { error = e.message.orEmpty() }
            finally { busy = false }
        }
    }
    Column(Modifier.fillMaxWidth().modCard(Nx.colors).padding(14.dp), verticalArrangement = Arrangement.spacedBy(5.dp)) {
        Txt(r.reason)
        Txt("Channel ${r.channel} · Receiving owner ${r.owner} · ${r.items.size} messages")
        Txt(if (r.files) "Includes attachments" else "Without attachments")
        Txt("Only the receiving owner can open the copy. Access expires ${java.util.Date(r.expires)}. Withdrawal ends access here. Downloaded copies cannot be recalled.")
        Txt("${r.status} · ${listOf(r.firstApproved,r.secondApproved).count { it }} of 2 approvals")
        for ((m,p) in entries) {
            Txt("Message ${m.id} · Author ${m.authorId} · ${Time.timestamp(m.createdAt)}", color = Nx.colors.textMuted)
            ReviewedPayload(p)
        }
        if (owner && r.status=="shared") NxButton("Read approved messages", { act("view") }, enabled = !busy)
        if (!owner && r.status=="pending") {
            NxButton("Inspect selection", { act("preview") }, enabled = !busy)
            val approved = if (me==r.first) r.firstApproved else r.secondApproved
            if (entries.isNotEmpty() && !approved) NxButton("Approve this selection", { act("approve") }, enabled = !busy)
            if (entries.isNotEmpty() && r.firstApproved && r.secondApproved) NxButton("Send approved copy to owner", { act("share") }, enabled = !busy)
        }
        if (!owner && r.status!="cancelled") NxButton("Decline or withdraw", { act("withdraw") }, enabled = !busy, kind = ButtonKind.GHOST)
        if (error.isNotEmpty()) Txt(error)
    }
}

@Composable
internal fun ReviewedPayload(p: MessagePayload) {
    listOfNotNull(p.fwdComment, p.t, p.sticker?.name, p.poll?.question).forEach { Txt(it) }
    p.poll?.answers?.forEach { Txt(it.text) }
    p.att?.let { Attachments(it, null, null) }
    p.embeds?.forEach { Txt(it.toString()) }
}
