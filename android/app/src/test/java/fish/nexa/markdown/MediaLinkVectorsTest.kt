package fish.nexa.markdown

import fish.nexa.ui.markdown.LinkPreviews
import fish.nexa.ui.markdown.MediaLinks
import fish.nexa.ui.markdown.PostLinks
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import kotlinx.serialization.json.longOrNull
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.BeforeClass
import org.junit.Test
import java.util.Collections

/** Post links, media proxy links (lib/postLinks.ts, lib/mediaLinks.ts) and the composer's link previews. */
class MediaLinkVectorsTest {
    companion object {
        @BeforeClass
        @JvmStatic
        fun assets() = Vectors.useAssets()
    }

    private fun JsonObject.s(k: String): String? = this[k]?.takeIf { it !is JsonNull }?.jsonPrimitive?.contentOrNull

    @Test
    fun postLinks() {
        val cases = Vectors["posts"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("isPostLink", cases) { c ->
            val got = PostLinks.isPostLink(c.s("url")!!)
            if (got == c["value"]!!.jsonPrimitive.boolean) null else "url ${c.s("url")}: web ${c["value"]} kotlin $got"
        }
    }

    @Test
    fun mediaLinks() = runBlocking {
        val now = Vectors["generated"].jsonObject["now"]!!.jsonPrimitive.long
        val asked = Collections.synchronizedList(ArrayList<List<String>>())
        val links = MediaLinks("https://nexa.pm", this, { batch -> asked += batch; emptyMap() }, { now })
        val cases = Vectors["media"].jsonArray.map { it.jsonObject }
        Vectors.expectAll("isProxied / proxyDeadline / mediaKey / mediaSrc", cases) { c ->
            val url = c.s("url")!!
            val src = links.src(url)
            val kind = when {
                src.pending -> "pending"
                src.url == null -> "blocked"
                src.url == url -> "usable"
                else -> "renewed"
            }
            val ok = links.isProxied(url) == c["proxied"]!!.jsonPrimitive.boolean &&
                links.proxyDeadline(url) == c["deadline"]!!.jsonPrimitive.longOrNull &&
                links.mediaKey(url) == c.s("key") &&
                kind == c.s("src")
            if (ok) null else "url ${Vectors.show(url)}: web $c kotlin proxied ${links.isProxied(url)} deadline ${links.proxyDeadline(url)} key ${links.mediaKey(url)} src $kind"
        }
        // the stale ones were asked for in one batch, and refused (no fresh link came back)
        delay(200)
        assertTrue(asked.flatten().isNotEmpty())
        for (u in asked.flatten()) assertEquals("blocked once refused", null, links.src(u).url)
    }

    @Test
    fun linkPreviewsKeepTheFragmentOnTheDevice() = runBlocking {
        data class Card(val url: String)
        val requested = Collections.synchronizedList(ArrayList<String>())
        val gate = CompletableDeferred<Unit>()
        val previews = LinkPreviews(this, "https://nexa.pm/channels/@me", { url: String ->
            requested += url
            if (url.contains("slow")) gate.await()
            if (url.contains("fail")) throw IllegalStateException("no")
            Card(url)
        }, { it.url }, { c, u -> c.copy(url = u) })
        val got = previews.embedsFor(
            listOf("https://nexa.pm/invite/abc#k=1.key", "https://example.com/a", "https://nexa.pm/note/abcdefghijklmnopq#key", "https://fail.example/x", "https://slow.example/y"),
            waitMs = 300,
        )
        assertEquals(listOf("https://nexa.pm/invite/abc", "https://example.com/a", "https://fail.example/x", "https://slow.example/y"), requested.toList())
        // the invite's key goes back on the card; the note link and the failure have none; the slow one missed the wait
        assertEquals(listOf("https://nexa.pm/invite/abc#k=1.key", "https://example.com/a"), got.map { it.url })
        gate.complete(Unit)
        delay(50)
        assertEquals(listOf("https://slow.example/y"), previews.embedsFor(listOf("https://slow.example/y"), 300).map { it.url })
        // a failure is forgotten, so the next edit asks again
        previews.start("https://fail.example/x")
        delay(50)
        assertEquals(2, requested.count { it == "https://fail.example/x" })
    }
}
