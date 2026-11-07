package fish.nexa.data

import fish.nexa.Nexa
import fish.nexa.ui.markdown.Markdown
import fish.nexa.ui.markdown.Node
import fish.nexa.ui.markdown.WebUrl
import java.text.Normalizer
import java.util.regex.Pattern

/**
 * The checks of AutoMod's rules that are not lists of words
 * (web/src/lib/automod/checks.ts): links, invites, personal details,
 * capitals, emoji, files and the signs of spam. Each says what it found, or
 * null.
 */
object AutomodChecks {
    /** Hosts that are this app's own, where links and invites are Nexa's. */
    val OWN_HOSTS = listOf("nexa.pm")

    private const val TLDS = "com|net|org|io|gg|co|me|tv|app|dev|xyz|ru|cn|uk|de|fr|info|biz|link|ly|gift|fish|lol|site|online|shop|store|club|top|live|stream|fun|click|win|pro|cc|to|su|in|us|ca|eu|nl|br|jp|kr|au|es|it|pl|tk|ml|ga|cf|gq|ws"
    private val URL_RE = Pattern.compile("\\bhttps?://[^\\s<>()]+|\\b(?:www\\.)?(?:[a-z0-9-]+\\.)+(?:$TLDS)\\b(?:/[^\\s<>()]*)?", Pattern.CASE_INSENSITIVE or Pattern.UNICODE_CASE)
    private val TRAILING = Regex("[.,!?;:'\"]+$")
    private val HAS_SCHEME = Regex("^https?://", RegexOption.IGNORE_CASE)

    class FoundLink(val url: String, val host: String, val path: String)

    private fun parse(raw: String): WebUrl? = WebUrl.parse(if (HAS_SCHEME.containsMatchIn(raw)) raw else "http://$raw")

    /** The links in a message, with their hosts. */
    fun linksIn(text: String): List<FoundLink> {
        val out = ArrayList<FoundLink>()
        val m = URL_RE.matcher(text)
        while (m.find()) {
            val raw = m.group().replace(TRAILING, "")
            val u = parse(raw) ?: continue
            out += FoundLink(raw, u.hostname.lowercase().removeSuffix("."), u.pathname)
        }
        return out
    }

    /** Whether a host is a domain or one of its subdomains. */
    fun hostIs(host: String, domain: String): Boolean {
        val d = domain.removePrefix("*.")
        return host == d || host.endsWith(".$d")
    }

    private fun ownHost(host: String): Boolean {
        val here = runCatching { WebUrl.parse(Nexa.api.baseUrl)?.hostname }.getOrNull()
        return OWN_HOSTS.any { hostIs(host, it) } || (!here.isNullOrEmpty() && host == here)
    }

    /** links: every link but the allowed sites, or only the listed ones. Nexa's own links are always fine. */
    fun checkLinks(text: String, c: AutomodConfig): String? {
        for (l in linksIn(text)) {
            if (ownHost(l.host)) continue
            if (c.mode == "list") {
                if ((c.domains ?: emptyList()).any { hostIs(l.host, it) }) return l.url
            } else if ((c.allow ?: emptyList()).none { hostIs(l.host, it) }) return l.url
        }
        return null
    }

    private val INVITE_PATHS = Regex("^/(?:invite|i)/[A-Za-z0-9_-]+/?$")

    /** invites: invite links, to servers here or anywhere else people gather. */
    fun checkInvites(text: String): String? {
        for (l in linksIn(text)) {
            val path = l.path
            if (ownHost(l.host) && INVITE_PATHS.matches(path)) return l.url
            if (hostIs(l.host, "discord.gg") || ((hostIs(l.host, "discord.com") || hostIs(l.host, "discordapp.com")) && path.startsWith("/invite/"))) return l.url
            if (hostIs(l.host, "t.me") || hostIs(l.host, "chat.whatsapp.com") || hostIs(l.host, "revolt.chat") || hostIs(l.host, "guilded.gg")) return l.url
        }
        return null
    }

    private val EMAIL_RE = Pattern.compile("[\\p{L}\\p{N}._%+-]+@[\\p{L}\\p{N}-]+(?:\\.[\\p{L}\\p{N}-]+)*\\.\\p{L}{2,}")
    private val PHONE_RE = Pattern.compile("(?:^|[^\\d])(\\+?\\d[\\d\\s().-]{8,18}\\d)(?!\\d)")
    private val IPV4_RE = Pattern.compile("\\b(?:(?:25[0-5]|2[0-4]\\d|1?\\d?\\d)\\.){3}(?:25[0-5]|2[0-4]\\d|1?\\d?\\d)\\b")
    private val IPV6_RE = Pattern.compile("\\b(?:[0-9a-f]{1,4}:){4,7}[0-9a-f]{1,4}\\b", Pattern.CASE_INSENSITIVE)
    private val CARD_RE = Pattern.compile("(?:\\d[ -]?){12,18}\\d")
    private val NOT_DIGIT = Regex("\\D")
    private val GROUPING = Regex("[\\s().+-]")

    private fun luhn(digits: String): Boolean {
        var sum = 0
        var double = false
        for (i in digits.length - 1 downTo 0) {
            var d = digits[i] - '0'
            if (double) {
                d *= 2
                if (d > 9) d -= 9
            }
            sum += d
            double = !double
        }
        return sum % 10 == 0
    }

    /** personal: email addresses, phone numbers, IP addresses and card numbers. */
    fun checkPersonal(text: String, c: AutomodConfig): String? {
        if (c.emails == true) {
            val m = EMAIL_RE.matcher(text)
            if (m.find()) return m.group()
        }
        if (c.cards == true) {
            val m = CARD_RE.matcher(text)
            while (m.find()) {
                val digits = m.group().replace(NOT_DIGIT, "")
                if (digits.length in 13..19 && luhn(digits)) return m.group()
            }
        }
        if (c.phones == true) {
            val m = PHONE_RE.matcher(text)
            while (m.find()) {
                val num = m.group(1) ?: continue
                val digits = num.replace(NOT_DIGIT, "")
                // a number grouped as phone numbers are, not a date, an amount or an id
                if (digits.length in 10..15 && GROUPING.containsMatchIn(num) && !luhn(digits)) return num
            }
        }
        if (c.ips == true) {
            val m4 = IPV4_RE.matcher(text)
            if (m4.find()) return m4.group()
            val m6 = IPV6_RE.matcher(text)
            if (m6.find()) return m6.group()
        }
        return null
    }

    private val TOKENS = Regex("<[@#&:a][^>]*>")
    private val LINKS = Regex("https?://\\S+")

    /** caps: a message mostly in capitals, past a length. */
    fun checkCaps(text: String, c: AutomodConfig): String? {
        // mentions, emoji and links are not shouting
        val plain = text.replace(TOKENS, "").replace(LINKS, "")
        var letters = 0
        var upper = 0
        var i = 0
        while (i < plain.length) {
            val cp = plain.codePointAt(i)
            when (Character.getType(cp).toByte()) {
                Character.UPPERCASE_LETTER -> {
                    letters++
                    upper++
                }
                Character.LOWERCASE_LETTER -> letters++
            }
            i += Character.charCount(cp)
        }
        val min = c.minLength ?: 10
        val percent = c.percent ?: 70
        return if (letters >= min && upper * 100.0 / letters >= percent) "${Math.round(upper * 100.0 / letters)}%" else null
    }

    private fun countEmoji(nodes: List<Node>): Int {
        var n = 0
        for (node in nodes) {
            n += when (node) {
                is Node.UnicodeEmoji, is Node.Emoji -> 1
                is Node.Details -> countEmoji(node.summary + node.children)
                is Node.Container -> countEmoji(node.children)
                is Node.ListBlock -> node.items.sumOf { countEmoji(it) }
                else -> 0
            }
        }
        return n
    }

    /** emoji_spam: more emoji in one message than allowed. */
    fun checkEmoji(text: String, c: AutomodConfig): String? {
        val n = if (text.isEmpty()) 0 else countEmoji(Markdown.parse(text))
        return if (n > (c.max ?: 10)) n.toString() else null
    }

    /** attachments: files ending in what the rule lists. */
    fun checkAttachments(names: List<String>, c: AutomodConfig): String? {
        val banned = (c.extensions ?: emptyList()).map { it.lowercase() }.toSet()
        for (name in names) {
            val ext = name.lowercase().substringAfterLast('.', "")
            if (name.contains('.') && ext in banned) return name
        }
        return null
    }

    /** How far apart two words are, in letters to change, add or take away. */
    private fun distance(a: String, b: String): Int {
        val row = IntArray(b.length + 1) { it }
        for (i in 1..a.length) {
            var prev = row[0]
            row[0] = i
            for (j in 1..b.length) {
                val cur = row[j]
                row[j] = minOf(row[j] + 1, row[j - 1] + 1, prev + if (a[i - 1] == b[j - 1]) 0 else 1)
                prev = cur
            }
        }
        return row[b.length]
    }

    private val NUMERIC_HOST = Regex("^\\d+\\.\\d+\\.\\d+\\.\\d+$")
    private val LABEL_SPLIT = Regex("[.-]")

    /** A link that hides where it goes, or passes for somewhere it is not. */
    fun shadyLink(links: List<FoundLink>, presets: AutomodPresets): String? {
        for (l in links) {
            if (ownHost(l.host)) continue
            if (NUMERIC_HOST.matches(l.host) || l.host.contains(':')) return l.url
            if (l.host.split('.').any { it.startsWith("xn--") }) return l.url
            if (presets.shorteners.any { hostIs(l.host, it) }) return l.url
            val folded = AutomodMatch.fold(l.host)
            for ((name, real) in presets.impersonated) {
                if (real.any { hostIs(l.host, it) }) continue
                val labels = folded.split(LABEL_SPLIT)
                if (folded.contains(name) || (name.length >= 5 && labels.any { it.length >= 5 && distance(it, name) == 1 })) return l.url
            }
        }
        return null
    }

    /** Text drowned in marks stacked on its letters. */
    fun zalgo(text: String): String? {
        var marks = 0
        var run = 0
        val s = Normalizer.normalize(text, Normalizer.Form.NFD)
        var i = 0
        while (i < s.length) {
            val cp = s.codePointAt(i)
            val type = Character.getType(cp).toByte()
            if (type == Character.NON_SPACING_MARK || type == Character.ENCLOSING_MARK || type == Character.COMBINING_SPACING_MARK) {
                marks++
                run++
                if (run >= 4) return String(Character.toChars(cp))
            } else run = 0
            i += Character.charCount(cp)
        }
        return if (marks >= 12) marks.toString() else null
    }

    private val UNBROKEN = Pattern.compile("\\S{300,}")
    private val REPEATED = Pattern.compile("(.)\\1{49,}", Pattern.DOTALL)

    /** A wall of text: a great many lines, a run with no spaces, or one character over and over. */
    fun wall(text: String): String? {
        if (text.split('\n').size > 30) return "lines"
        if (UNBROKEN.matcher(text).find()) return "unbroken"
        if (REPEATED.matcher(text).find()) return "repeated"
        return null
    }

    private var scamMatcher: Pair<AutomodPresets, AutomodMatch.Matcher>? = null

    /** A scam's or an advert's usual words. */
    @Synchronized
    fun scam(text: String, presets: AutomodPresets): String? {
        val held = scamMatcher
        val m = if (held != null && held.first === presets) held.second else AutomodMatch.Matcher(presets.spamPhrases, emptyList(), emptyList(), true).also { scamMatcher = presets to it }
        return m.match(text)
    }
}
