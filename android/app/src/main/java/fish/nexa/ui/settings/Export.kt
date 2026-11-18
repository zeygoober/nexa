package fish.nexa.ui.settings

import android.net.Uri
import fish.nexa.Nexa
import fish.nexa.data.DecryptFail
import fish.nexa.net.ApiError
import fish.nexa.net.Channel
import fish.nexa.net.Message
import fish.nexa.net.wireJson
import fish.nexa.ui.ReadableError
import fish.nexa.ui.t
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.put
import java.io.OutputStream
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import java.util.zip.CRC32
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

/**
 * lib/export.ts: everything the account holds, read through the ordinary
 * API, decrypted on this device and written as a zip of JSON files (stored,
 * not compressed, as the web writes it), straight into the file the person
 * chose rather than held in memory.
 */
object DataExport {
    /** The passcode verifier belongs in the sealed settings, never a plain export. */
    internal fun publicSettings(settings: JsonObject): JsonObject = JsonObject(settings.filterKeys { it != "appLock" })
    private const val REQUEST_GAP_MS = 200L
    private const val PAGE = 100
    private const val MAX_PAGES = 10_000
    private const val MAX_ENTRIES = 65_535
    private const val TRIES = 6

    private val pretty = Json { prettyPrint = true; prettyPrintIndent = "  " }
    private var lastRequest = 0L

    fun fileName(): String {
        val f = SimpleDateFormat("yyyy-MM-dd", Locale.ROOT).apply { timeZone = TimeZone.getTimeZone("UTC") }
        return "evilion-chat-export-${f.format(Date())}.zip"
    }

    private fun safeName(s: String): String = s.replace(Regex("[^A-Za-z0-9._-]+"), "_").take(60).ifEmpty { "channel" }

    /** Requests are spaced 200 ms apart across the whole export; busy or failed ones are tried again. */
    private suspend fun paced(path: String): JsonElement {
        var attempt = 0
        while (true) {
            val wait = lastRequest + REQUEST_GAP_MS - System.currentTimeMillis()
            if (wait > 0) delay(wait)
            lastRequest = System.currentTimeMillis()
            try {
                return Nexa.api.get(path)
            } catch (e: ApiError) {
                val retry = e.status == 429 || e.status == 0 || e.status >= 500
                attempt += 1
                if (!retry || attempt >= TRIES) throw e
                val ms = minOf(maxOf((e.retryAfter ?: 0) * 1000L, 1000L shl attempt), 30_000L)
                delay(ms)
            }
        }
    }

    private class Zip(out: OutputStream) {
        val zip = ZipOutputStream(out)
        var entries = 0
        var bytes = 0L
        val time = System.currentTimeMillis()

        fun put(name: String, data: ByteArray) {
            entries += 1
            bytes += data.size
            if (entries > MAX_ENTRIES || bytes > 0xFFFFFFFFL) throw ReadableError(t("export.tooLarge"))
            val e = ZipEntry(name)
            e.method = ZipEntry.STORED
            e.size = data.size.toLong()
            e.compressedSize = data.size.toLong()
            e.crc = CRC32().apply { update(data) }.value
            e.time = time
            zip.putNextEntry(e)
            zip.write(data)
            zip.closeEntry()
        }

        fun json(name: String, value: JsonElement) = put(name, pretty.encodeToString(JsonElement.serializer(), value).toByteArray())
    }

    private fun iso(ms: Long): String {
        val f = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'", Locale.ROOT).apply { timeZone = TimeZone.getTimeZone("UTC") }
        return f.format(Date(ms))
    }

    private fun row(m: Message): JsonObject {
        val dec = Nexa.messages.decrypt(m)
        val p = dec.payload
        val hookText = (dec.hook?.get("content") as? JsonPrimitive)?.content
        return buildJsonObject {
            put("id", m.id)
            put("author_id", m.authorId)
            put("type", m.type)
            put("created_at", iso(m.createdAt))
            put("edited_at", m.editedAt?.let { JsonPrimitive(iso(it)) } ?: JsonNull)
            put("reply_to_id", m.replyToId?.let { JsonPrimitive(it) } ?: JsonNull)
            put("text", (p?.t ?: hookText)?.let { JsonPrimitive(it) } ?: JsonNull)
            put("attachments", buildJsonArray {
                p?.att?.forEach { a -> add(buildJsonObject { put("name", a.name); put("type", a.type); put("size", a.size) }) }
            })
            put("sticker", p?.sticker?.let { s ->
                buildJsonObject {
                    put("id", s.id)
                    s.blob?.let { put("blob", it) }
                    s.name?.let { put("name", it) }
                    s.w?.let { put("w", it) }
                    s.h?.let { put("h", it) }
                    s.animated?.let { put("animated", it) }
                    s.format?.let { put("format", it) }
                }
            } ?: JsonNull)
            put("embeds", JsonArray(p?.embeds ?: emptyList()))
            put("reactions", wireJson.encodeToJsonElement(m.reactions))
            put("decrypted", dec.error == null)
            put("error", when (dec.error) {
                DecryptFail.NO_KEY -> JsonPrimitive("no_key")
                DecryptFail.BAD -> JsonPrimitive("bad")
                DecryptFail.INVALID -> JsonPrimitive("invalid")
                null -> JsonNull
            })
        }
    }

    /** Runs the whole export into the document at [uri]; [status] hears each step. */
    suspend fun run(uri: Uri, status: (String) -> Unit) = withContext(Dispatchers.IO) {
        lastRequest = 0L
        val out = Nexa.app.contentResolver.openOutputStream(uri) ?: throw ReadableError(t("settings.privacy.couldNotExportYourData"))
        out.use { stream ->
            val zip = Zip(stream)
            withContext(Dispatchers.Main) { status(t("export.readingProfile")) }
            zip.json("profile.json", paced("/users/@me"))
            zip.json("settings.json", publicSettings(Nexa.settings.doc.value))
            withContext(Dispatchers.Main) { status(t("export.readingFriends")) }
            zip.json("relationships.json", paced("/users/@me/relationships"))
            zip.json("gems.json", paced("/gems/record"))
            val servers = Nexa.servers.byId.value.values.toList()
            zip.json("servers.json", buildJsonArray {
                for (st in servers) {
                    val base = wireJson.encodeToJsonElement(st.server) as JsonObject
                    add(JsonObject(LinkedHashMap(base).apply {
                        put("roles", wireJson.encodeToJsonElement(st.roles.values.toList()))
                        put("me", wireJson.encodeToJsonElement(st.me))
                    }))
                }
            })
            val dms = (paced("/users/@me/channels") as? JsonArray)?.mapNotNull { runCatching { wireJson.decodeFromJsonElement(Channel.serializer(), it) }.getOrNull() } ?: emptyList()
            val text = Nexa.channels.byId.value.values.filter { it.serverId != null && it.type == "text" }
            val all = dms + text
            zip.json("channels.json", JsonArray(all.map { wireJson.encodeToJsonElement(it) }))
            val skipped = ArrayList<String>()
            all.forEachIndexed { i, ch ->
                val name = ch.name.ifEmpty { ch.id }
                withContext(Dispatchers.Main) { status(t("export.readingChannel", "done" to i + 1, "total" to all.size, "name" to name)) }
                val pages = ArrayList<List<Message>>()
                var before: String? = null
                var count = 0
                try {
                    for (page in 0 until MAX_PAGES) {
                        val path = "/channels/${ch.id}/messages?limit=$PAGE" + (before?.let { "&before=$it" } ?: "")
                        val list = (paced(path) as? JsonArray)?.map { wireJson.decodeFromJsonElement(Message.serializer(), it) } ?: emptyList()
                        if (list.isEmpty()) break
                        pages += list
                        count += list.size
                        withContext(Dispatchers.Main) {
                            status(t("export.readingChannelCount", "done" to i + 1, "total" to all.size, "name" to name, "messages" to t("export.messagesCount", "count" to count)))
                        }
                        before = list.first().id
                        if (list.size < PAGE) break
                    }
                } catch (e: ApiError) {
                    if (e.status == 403 || e.status == 404) {
                        val server = ch.serverId?.let { Nexa.servers.get(it)?.server?.name ?: it }
                        skipped += (if (server != null) "$server/${ch.name}" else ch.name.ifEmpty { ch.recipientIds.joinToString("-") }) + " (${ch.id})"
                        return@forEachIndexed
                    }
                    throw e
                }
                val messages = pages.asReversed().flatten().map { row(it) }
                val dir = if (ch.serverId == null) "dm" else safeName(Nexa.servers.get(ch.serverId)?.server?.name ?: ch.serverId)
                zip.json("messages/$dir/${safeName(name)}-${ch.id}.json", buildJsonObject {
                    put("channel", wireJson.encodeToJsonElement(ch))
                    put("messages", JsonArray(messages))
                })
            }
            val readme = buildString {
                append(t("export.readmeFirstLine")).append("\n\n")
                append("Nexa data export\n\n")
                append("Everything here was decrypted on your device. Messages you never held a key for are marked decrypted: false.\n")
                append("Attachments are listed by name; download them from the app while you still have access.\n")
                if (skipped.isNotEmpty()) {
                    append("\nThese conversations could not be read any more, so they are not here:\n")
                    for (s in skipped) append("- ").append(s).append('\n')
                }
            }
            zip.put("README.txt", readme.toByteArray())
            zip.zip.finish()
        }
    }
}
