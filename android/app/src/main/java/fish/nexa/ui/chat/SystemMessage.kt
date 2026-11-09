package fish.nexa.ui.chat

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.layout
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.crypto.PollPayload
import fish.nexa.data.ChatMessage
import fish.nexa.ui.Icon
import fish.nexa.ui.Time
import fish.nexa.ui.nav.Nav
import fish.nexa.ui.nav.Route
import fish.nexa.ui.shell.Names
import fish.nexa.ui.shell.openProfile
import fish.nexa.ui.components.pressable
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx

/**
 * SystemMessage.tsx: a line about the conversation itself (someone joined,
 * left, pinned something, a poll closed), never grouped and without a menu.
 * Names open the person; "a message" and "View Poll" jump to what they name.
 */
@Composable
fun SystemRow(cm: ChatMessage, ctx: ListCtx) {
    val c = Nx.colors
    val m = cm.m
    val sid = ctx.server?.server?.id
    val actor = m.authorId
    val target = m.mentions.firstOrNull()
    val icon = when (m.type) {
        "system_member_remove" -> "ArrowLeftFromLine"
        "system_name_change" -> "Pencil"
        "system_icon_change" -> "Image"
        "system_owner_change", "system_server_transfer" -> "Crown"
        "system_pin" -> "Pin"
        "system_poll_result" -> "ChartBar"
        "system_call" -> "Phone"
        "system_thread" -> "MessageSquareText"
        "system_thread_remove" -> "ArrowLeftFromLine"
        else -> "ArrowRightToLine"
    }
    // Someone's own "joined the server" line is theirs to take down, and so is it
    // for whoever may delete messages there, as with any message in the channel.
    val removable = m.type == "system_join" && canDelete(m)
    val inlineWidth = rememberInlineWidth()
    val maxWidth = with(androidx.compose.ui.platform.LocalDensity.current) { inlineWidth.pixels.toDp() }
    Box(
        Modifier
            .fillMaxWidth()
            .padding(top = 17.dp)
            .then(
                if (removable) Modifier.pressable(remember { androidx.compose.foundation.interaction.MutableInteractionSource() }, onLongClick = {
                    fish.nexa.ui.layers.openMenu(listOf(fish.nexa.ui.layers.MenuItem.Action(t("chat.messageMenu.deleteMessage"), danger = true) { confirmDelete(cm) }))
                }) {} else Modifier,
            )
            .heightIn(min = 26.dp)
            .padding(start = if (ctx.selecting) 98.dp else 64.dp, end = 16.dp, top = 4.dp, bottom = 4.dp)
            .then(inlineWidth.modifier),
    ) {
        val nameStyle = SpanStyle(color = c.textStrong, fontWeight = FontWeight(500))
        val names = personInlines(Nx.type.body.copy(fontSize = Nx.type.sp(15.5f), lineHeight = Nx.type.sp(22f), fontWeight = FontWeight(500)), c.textStrong, maxWidth, sid)
        val linkStyle = TextLinkStyles(SpanStyle(color = c.link))
        // a closed poll's question and answers: from the list, or fetched when the list does not hold the poll (usePollOf.ts)
        val personPoll = m.type == "system_poll_result" && m.webhookId == null
        val held = if (personPoll) m.replyToId?.let {
            (Nexa.messages.get(ctx.channel.id).byId[it] ?: (fish.nexa.data.ReplyTargets.byId.value[it] as? fish.nexa.data.ReplyTarget.Found)?.cm)?.payload?.poll
        } else null
        val fetched by produceState<PollPayload?>(null, m.replyToId, held == null) {
            val id = m.replyToId
            if (personPoll && held == null && id != null) value = FetchedPolls.load(ctx.channel.id, id)
        }
        val poll = held ?: fetched

        /** Fills a translated sentence, putting a tappable name or link where each {placeholder} is. */
        fun fill(template: String, parts: Map<String, AnnotatedString.Builder.() -> Unit>): AnnotatedString = buildAnnotatedString {
            var i = 0
            val rx = Regex("\\{(\\w+)\\}")
            for (match in rx.findAll(template)) {
                append(template.substring(i, match.range.first))
                val fn = parts[match.groupValues[1]]
                if (fn != null) fn() else append(match.value)
                i = match.range.last + 1
            }
            append(template.substring(i))
        }

        fun person(id: String?): AnnotatedString.Builder.() -> Unit = {
            if (id == null) append(t("chat.systemMessage.someone"))
            else names.append(this, id, Names.member(sid, id)) { openProfile(id) }
        }

        fun jumpLink(label: String, to: String?): AnnotatedString.Builder.() -> Unit = {
            if (to == null) append(label)
            else withLink(LinkAnnotation.Clickable("jump-$to", styles = linkStyle) { ChatUi.jumpTo(ctx.channel.id, to) }) { append(label) }
        }

        // a thread's name in its notice: the way into it
        fun threadLink(threadId: String, name: String): AnnotatedString.Builder.() -> Unit = {
            withLink(LinkAnnotation.Clickable("thread-$threadId", styles = linkStyle) { if (sid != null) Nav.go(Route.Server(sid, threadId)) }) { append(name) }
        }

        val self = target == null || target == actor
        val text: AnnotatedString = when (m.type) {
            "system_thread" -> {
                val th = m.thread
                val name = th?.let { Nexa.channels.get(it.id)?.name ?: it.name }
                if (th != null && !name.isNullOrEmpty()) fill(t("chat.systemMessage.startedThread"), mapOf("actor" to person(actor), "thread" to threadLink(th.id, name)))
                else fill(t("chat.systemMessage.startedThreadGone"), mapOf("actor" to person(actor)))
            }
            "system_thread_add" -> fill(t("chat.systemMessage.addedToThread"), mapOf("actor" to person(actor), "target" to person(target)))
            "system_thread_remove" -> fill(t("chat.systemMessage.removedFromThread"), mapOf("actor" to person(actor), "target" to person(target)))
            "system_name_change" -> fill(t("chat.systemMessage.changedName"), mapOf("actor" to person(actor)))
            "system_icon_change" -> fill(t("chat.systemMessage.changedIcon"), mapOf("actor" to person(actor)))
            "system_member_add" -> if (self) fill(t("chat.systemMessage.joinedGroup"), mapOf("actor" to person(actor)))
            else fill(t("chat.systemMessage.addedToGroup"), mapOf("actor" to person(actor), "target" to person(target)))
            "system_member_remove" -> if (self) fill(t("chat.systemMessage.leftGroup"), mapOf("actor" to person(actor)))
            else fill(t("chat.systemMessage.removedFromGroup"), mapOf("actor" to person(actor), "target" to person(target)))
            "system_owner_change" -> if (self) fill(t("chat.systemMessage.tookOwnership"), mapOf("actor" to person(actor)))
            else fill(t("chat.systemMessage.madeOwner"), mapOf("actor" to person(actor), "target" to person(target)))
            "system_server_transfer" -> fill(t("chat.systemMessage.serverTransferred"), mapOf("actor" to person(actor), "target" to person(target)))
            "system_pin" -> fill(t("chat.systemMessage.pinned"), mapOf("actor" to person(actor), "message" to jumpLink(t("chat.systemMessage.aMessage"), m.replyToId)))
            "system_join" -> fill(t("chat.systemMessage.joined"), mapOf("actor" to person(actor)))
            // a poll's closing line speaks for whoever asked: the webhook that posted it, or the person
            "system_poll_result" -> if (m.webhookId != null) {
                val hook = fish.nexa.ui.markdown.Webhook.of(cm.hook)
                hookPollResult(hook, ::fill, hook?.let { h -> { withStyle(nameStyle) { append(h.name) } } } ?: person(actor), jumpLink(t("chat.systemMessage.viewPoll"), m.replyToId))
            } else pollResult(cm, poll, ::fill, person(actor), jumpLink(t("chat.systemMessage.viewPoll"), m.replyToId))
            else -> fill(t("chat.systemMessage.didSomething"), mapOf("actor" to person(actor)))
        }
        val full = buildAnnotatedString {
            append(text)
            withStyle(SpanStyle(fontSize = Nx.type.sp(12f), color = c.textMuted)) { append("  " + Time.timestamp(m.createdAt)) }
        }
        Icon(icon, c.textMuted, size = 18.dp, modifier = Modifier.padding(top = 2.dp).offsetStart(if (ctx.selecting) -44 else -40))
        BasicText(full, inlineContent = names.content, style = Nx.type.body.copy(fontSize = Nx.type.sp(15.5f), lineHeight = Nx.type.sp(22f), color = c.textMuted))
    }
}

/** The icon sits in the avatar gutter: 24 dp from the row's start edge (54 while picking). */
private fun Modifier.offsetStart(dp: Int): Modifier = this.layout { measurable, constraints ->
    val p = measurable.measure(constraints)
    layout(0, p.height) { p.placeRelative((dp * density).toInt(), 0) }
}

/** Polls read from messages the list does not hold, by message id, each fetched once (usePollOf.ts loadPoll). */
private object FetchedPolls {
    private val polls = java.util.concurrent.ConcurrentHashMap<String, PollPayload>()
    /** Gone, or out of reach, stays so; a moment offline does not. */
    private val unreachable = java.util.concurrent.ConcurrentHashMap.newKeySet<String>()

    suspend fun load(channelId: String, messageId: String): PollPayload? {
        polls[messageId]?.let { return it }
        if (messageId in unreachable) return null
        return try {
            val m = Nexa.api.get("/channels/$channelId/messages/$messageId", fish.nexa.net.Message.serializer())
            kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.Default) { Nexa.messages.decrypt(m) }.payload?.poll?.also { polls[messageId] = it }
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (e: fish.nexa.net.ApiError) {
            if (e.status == 404 || e.status == 403) unreachable += messageId
            null
        } catch (_: Exception) {
            null
        }
    }
}

/** The sentence after "…has closed.": nobody voted, a tie, or who won. A winner whose words cannot be read goes unsaid. */
private fun verdictText(total: Int, victor: Int, votes: Int, answer: String?): String = when {
    total == 0 -> t("chat.systemMessage.pollNoVotes")
    victor == 0 -> t("chat.systemMessage.pollTie")
    answer == null -> ""
    else -> t("chat.systemMessage.pollWinner", "answer" to answer, "percent" to Math.round(votes * 100.0 / total))
}

/**
 * How a person's poll went. Its line carries only numbers, since the server
 * never saw what the poll asked; the words come from the poll's own message.
 */
private fun pollResult(
    cm: ChatMessage,
    poll: PollPayload?,
    fill: (String, Map<String, AnnotatedString.Builder.() -> Unit>) -> AnnotatedString,
    actor: AnnotatedString.Builder.() -> Unit,
    view: AnnotatedString.Builder.() -> Unit,
): AnnotatedString {
    val question = poll?.question
    val head = if (question != null) fill(t("chat.systemMessage.pollClosed"), mapOf("actor" to actor, "question" to { withStyle(SpanStyle(fontWeight = FontWeight(600))) { append(question) } }))
    else fill(t("chat.systemMessage.pollClosedUntitled"), mapOf("actor" to actor))
    val outcome = cm.m.pollResult?.let { o ->
        val a = poll?.answers?.getOrNull(o.victorAnswerId - 1)
        verdictText(o.totalVotes, o.victorAnswerId, o.victorAnswerVotes, a?.let { if (it.emoji != null && it.emoji.id == null) "${it.emoji.name} ${it.text}".trim() else it.text })
    } ?: ""
    return closedLine(head, outcome, view)
}

/** How a webhook's poll went, from the result its closing carries (the poll_result embed). */
private fun hookPollResult(
    hook: fish.nexa.ui.markdown.Webhook.Content?,
    fill: (String, Map<String, AnnotatedString.Builder.() -> Unit>) -> AnnotatedString,
    actor: AnnotatedString.Builder.() -> Unit,
    view: AnnotatedString.Builder.() -> Unit,
): AnnotatedString {
    val fields = hook?.embeds?.firstOrNull { it.type == "poll_result" }?.fields.orEmpty().associate { it.name to it.value }
    val question = fields["poll_question_text"] ?: ""
    val total = fields["total_votes"]?.toIntOrNull() ?: 0
    val votes = fields["victor_answer_votes"]?.toIntOrNull() ?: 0
    val winner = fields["victor_answer_text"]
    val emoji = fields["victor_answer_emoji_name"]
    val head = fill(t("chat.systemMessage.pollClosed"), mapOf("actor" to actor, "question" to { withStyle(SpanStyle(fontWeight = FontWeight(600))) { append(question) } }))
    val outcome = when {
        total == 0 -> verdictText(0, 0, 0, null)
        winner != null -> verdictText(total, 1, votes, if (emoji != null) "$emoji $winner" else winner)
        else -> verdictText(total, 0, 0, null)
    }
    return closedLine(head, outcome, view)
}

private fun closedLine(head: AnnotatedString, outcome: String, view: AnnotatedString.Builder.() -> Unit) = buildAnnotatedString {
    append(head)
    if (outcome.isNotEmpty()) append(" $outcome")
    append(" ")
    view()
}
