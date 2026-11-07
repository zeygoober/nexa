package fish.nexa.markdown

import fish.nexa.ui.markdown.AppLinks
import fish.nexa.ui.markdown.Markdown
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import org.junit.BeforeClass
import org.junit.Test

/** Links to the app's own pages (lib/appLinks.ts, lib/inviteLink.ts), against the web. */
class LinkVectorsTest {
    companion object {
        @BeforeClass
        @JvmStatic
        fun assets() = Vectors.useAssets()

        const val HERE = "https://nexa.pm/channels/@me"
    }

    private fun JsonObject.s(k: String): String? = this[k]?.takeIf { it !is JsonNull }?.jsonPrimitive?.contentOrNull

    private fun describe(link: AppLinks.AppLink?): String = when (link) {
        null -> "null"
        is AppLinks.AppLink.Invite -> "invite ${link.code} ${link.hash}"
        is AppLinks.AppLink.Note -> "note ${link.token} ${link.hash}"
        is AppLinks.AppLink.Profile -> "profile ${link.username}"
        is AppLinks.AppLink.Route -> "route ${link.path}"
    }

    private fun describe(web: JsonElement?): String {
        val o = web?.takeIf { it !is JsonNull }?.jsonObject ?: return "null"
        return when (o.s("kind")) {
            "invite" -> "invite ${o.s("code")} ${o.s("hash")}"
            "note" -> "note ${o.s("token")} ${o.s("hash")}"
            "profile" -> "profile ${o.s("username")}"
            else -> "route ${o.s("path")}"
        }
    }

    @Test
    fun appLinks() {
        val cases = Vectors["links"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("appLink", cases) { c ->
            val got = describe(AppLinks.appLink(c.s("href")!!, HERE))
            val want = describe(c["link"])
            if (got == want) null else "href ${c.s("href")}: web $want kotlin $got"
        }
    }

    @Test
    fun messageTargets() {
        val cases = Vectors["targets"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("messageTarget", cases) { c ->
            val got = AppLinks.messageTarget(c.s("path")!!)
            val want = c["target"]!!.takeIf { it !is JsonNull }?.jsonObject?.let { it.s("channelId")!! to it.s("messageId")!! }
            if (got == want) null else "path ${c.s("path")}: web $want kotlin $got"
        }
    }

    @Test
    fun cards() {
        val cases = Vectors["cards"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("inviteLinksIn / noteLinksIn / profileLinksIn", cases) { c ->
            val nodes = Markdown.parse(c.s("src")!!)
            val invites = AppLinks.inviteLinksIn(nodes, HERE).map { listOf(it.code, it.hash, it.url) }
            val notes = AppLinks.noteLinksIn(nodes, HERE).map { listOf(it.token, it.hash, it.url) }
            val profiles = AppLinks.profileLinksIn(nodes, HERE).map { listOf(it.username, it.url) }
            val wInvites = c["invites"]!!.jsonArray.map { it.jsonObject }.map { listOf(it.s("code"), it.s("hash"), it.s("url")) }
            val wNotes = c["notes"]!!.jsonArray.map { it.jsonObject }.map { listOf(it.s("token"), it.s("hash"), it.s("url")) }
            val wProfiles = c["profiles"]!!.jsonArray.map { it.jsonObject }.map { listOf(it.s("username"), it.s("url")) }
            if (invites == wInvites && notes == wNotes && profiles == wProfiles) null
            else "src ${c.s("src")}\nweb    $wInvites $wNotes $wProfiles\nkotlin $invites $notes $profiles"
        }
    }

    @Test
    fun invites() {
        val shown = Vectors["invites"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("shownInvite", shown) { c ->
            val got = AppLinks.shownInvite(c.s("url")!!, "https://nexa.pm")
            if (got == c.s("shown")) null else "url ${c.s("url")}: web ${c.s("shown")} kotlin $got"
        }
        val fragments = Vectors["fragments"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("parseInviteFragment", fragments) { c ->
            val got = AppLinks.parseInviteFragment(c.s("hash")!!)?.let { it.epoch to it.root.joinToString("") { b -> "%02x".format(b) } }
            val want = c["key"]!!.takeIf { it !is JsonNull }?.jsonObject?.let { it["epoch"]!!.jsonPrimitive.long to it.s("root")!! }
            if (got == want) null else "hash ${c.s("hash")}: web $want kotlin $got"
        }
    }
}
