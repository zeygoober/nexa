package fish.nexa

import fish.nexa.ui.profile.Activities
import org.junit.Assert.*
import org.junit.Test

class ActivityLinkTest {
    @Test fun onlyCanonicalCredentialFreeWebLinksLeaveTheApp() {
        for (raw in listOf("javascript:alert(1)", "intent://example.com", "file:///data/private", "https://nexa.pm@evil.example/", "https://user:password@example.com/", "not a url")) assertNull(raw, Activities.destination(raw))
        val link = Activities.destination("https://EVIL.example:8443/account")!!
        assertEquals("evil.example:8443", link.host)
        assertEquals("https://evil.example:8443/account", link.href)
    }
}
