package fish.nexa.ui.markdown

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Whether a link is to a social post, whose card is drawn as a post
 * (lib/postLinks.ts): Send waits longer for such a link's card, since the
 * server reads a post more slowly than a page's tags. The lists are the
 * server's (internal/unfurl/post.go).
 */
object PostLinks {
    /** Services, and the "fixer" domains people swap into a link, under any subdomain. */
    private val HOSTS = setOf(
        "x.com", "twitter.com", "fxtwitter.com", "fixupx.com", "vxtwitter.com", "fixvx.com", "twittpr.com", "nitter.net", "xcancel.com",
        "bsky.app", "fxbsky.app", "bskx.app", "bskye.app", "vxbsky.app",
        "tiktok.com", "tnktok.com", "tfxktok.com", "vxtiktok.com", "kktiktok.com", "tiktxk.com", "tiktokez.com",
        "instagram.com", "ddinstagram.com", "kkinstagram.com", "instagramez.com", "eeinstagram.com", "vxinstagram.com", "uuinstagram.com",
        "reddit.com", "rxddit.com", "vxreddit.com", "redditez.com", "rxyddit.com",
        "threads.net", "threads.com", "fixthreads.net", "vxthreads.net",
        "youtube.com", "youtube-nocookie.com", "koutube.com",
    )

    /** Services under their exact name only: a subdomain of these is something else (i.redd.it is a picture). */
    private val EXACT_HOSTS = setOf("redd.it", "t.me", "telegram.me", "telegram.dog", "fx-t.me", "txlegram.me", "txlegram.org", "youtu.be", "koutu.be")

    /** A Mastodon post is known by the shape of its address, on whatever server. */
    private val MASTODON = Regex("^/(?:@[A-Za-z0-9_]{1,64}(?:@[A-Za-z0-9.-]{1,255})?/[0-9]{10,24}|users/[A-Za-z0-9_]{1,64}/statuses/[0-9]{10,24})/?$")

    fun isPostLink(link: String): Boolean {
        val u = WebUrl.parse(link) ?: return false
        val host = u.hostname.lowercase().removeSuffix(".")
        if (host in EXACT_HOSTS) return true
        val labels = host.split(".")
        if (labels.size in 2..4 && labels.takeLast(2).joinToString(".") in HOSTS) return true
        return MASTODON.containsMatchIn(u.pathname)
    }
}

/**
 * Remote pictures, GIFs and videos in link previews, made safe and able to
 * load (lib/mediaLinks.ts). Every address a preview loads is one this server
 * signed for its media proxy, with a deadline of a day; a preview stored in
 * an encrypted message cannot be renewed by the server, so a stale link is
 * signed again here (POST /proxy/refresh, a batch at a time) before anything
 * loads it. Anything else remote is not loaded at all.
 */
class MediaLinks(
    /** The instance's origin ("https://nexa.pm"). */
    private val origin: String,
    private val scope: CoroutineScope,
    /** POST /proxy/refresh with these links: the freshly signed ones by the old address. Throws when it did not get through. */
    private val refresh: suspend (List<String>) -> Map<String, String>,
    private val now: () -> Long = System::currentTimeMillis,
) {
    companion object {
        const val PROXY_PREFIX = "/media/proxy/"
        /** A link this close to its deadline is signed again rather than risked. */
        const val MARGIN_MS = 5 * 60 * 1000L
        private const val BATCH = 100
        private const val GATHER_MS = 30L
        /** After a request that did not get through, how long before its links are asked for again. */
        const val RETRY_MS = 30 * 1000L

        /** A picture that shows nothing, for a tile whose link is still being signed again. */
        const val BLANK_IMAGE = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

        private val UNSAFE = Regex("[\\\\\\s\\u0000-\\u001f\\u007f\\u00A0\\u1680\\u2000-\\u200A\\u2028\\u2029\\u202F\\u205F\\u3000\\uFEFF]")
    }

    /** The path of a link to this server's proxy, or null for anything else. */
    fun proxyPath(url: String): String? {
        if (url.startsWith(PROXY_PREFIX)) return url
        val u = WebUrl.parse(url) ?: return null
        return if (u.origin == origin && u.pathname.startsWith(PROXY_PREFIX)) u.pathname else null
    }

    fun isProxied(url: String): Boolean = proxyPath(url) != null

    /** When a proxy link stops working, in ms; null for one signed before links had deadlines. */
    fun proxyDeadline(url: String): Long? {
        val path = proxyPath(url) ?: return null
        val sig = path.substring(PROXY_PREFIX.length).split("/")[0]
        val m = Regex("^([0-9]{9,12})-").find(sig) ?: return null
        return m.groupValues[1].toLong() * 1000
    }

    /** What a media link points at, the same however often it is signed. */
    fun mediaKey(url: String): String {
        val path = proxyPath(url) ?: return url
        return path.substring(PROXY_PREFIX.length).split("/").getOrNull(1) ?: url
    }

    /** Whether a link can be loaded as it is: this server's own media, a fresh proxy link, or local data. */
    fun usableAsIs(url: String): Boolean {
        if (url.startsWith("data:") || url.startsWith("blob:")) return true
        if (isProxied(url)) {
            val deadline = proxyDeadline(url)
            return deadline != null && deadline - now() > MARGIN_MS
        }
        // a backslash or a control character is how a path that starts with "/" reaches another origin
        if (UNSAFE.containsMatchIn(url)) return false
        val base = WebUrl.parse("$origin/") ?: return false
        return WebUrl.parse(url, base)?.origin == origin
    }

    private val renewed = HashMap<String, String>()
    private val refused = HashSet<String>()
    private val failed = HashMap<String, Long>()
    private val queued = LinkedHashSet<String>()
    private val asking = HashSet<String>()
    private val lock = Mutex()
    private var flushing = false

    private val _version = MutableStateFlow(0)
    /** Bumps whenever a link comes back signed again (or refused): read it to draw again. */
    val version: StateFlow<Int> = _version

    /**
     * What to load for a media link just now: the address itself, a newly
     * signed one, null while one is being asked for (`pending` true), or null
     * for something that must not be loaded (`pending` false).
     */
    data class Src(val url: String?, val pending: Boolean)

    @Synchronized
    fun src(url: String?): Src {
        if (url.isNullOrEmpty()) return Src(null, false)
        if (usableAsIs(url)) return Src(url, false)
        if (!isProxied(url)) return Src(null, false)
        val fresh = renewed[url]
        if (fresh != null && usableAsIs(fresh)) return Src(fresh, false)
        if (url in refused) return Src(null, false)
        val at = failed[url]
        if (at != null && now() - at < RETRY_MS) return Src(null, true)
        ask(url)
        return Src(null, true)
    }

    private fun ask(url: String) {
        if (url in queued || url in asking) return
        queued += url
        if (!flushing) {
            flushing = true
            scope.launch {
                delay(GATHER_MS)
                flush()
            }
        }
    }

    private suspend fun flush() {
        while (true) {
            val batch = synchronized(this) {
                val b = queued.take(BATCH)
                for (u in b) {
                    queued.remove(u)
                    asking += u
                }
                if (b.isEmpty()) flushing = false
                b
            }
            if (batch.isEmpty()) return
            lock.withLock {
                try {
                    val urls = refresh(batch)
                    synchronized(this) {
                        for (url in batch) {
                            val fresh = urls[url]
                            failed.remove(url)
                            if (fresh != null && isProxied(fresh)) renewed[url] = fresh else refused += url
                        }
                    }
                } catch (e: kotlinx.coroutines.CancellationException) {
                    throw e
                } catch (_: Exception) {
                    // offline or rate limited: the links wait, and are asked for again
                    val t = now()
                    synchronized(this) { for (url in batch) failed[url] = t }
                    scope.launch {
                        delay(RETRY_MS)
                        _version.value++
                    }
                } finally {
                    synchronized(this) { for (url in batch) asking.remove(url) }
                    _version.value++
                }
            }
            delay(GATHER_MS)
        }
    }
}
