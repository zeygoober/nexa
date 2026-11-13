package fish.nexa.data

import fish.nexa.Nexa
import fish.nexa.ui.t
import java.util.Locale
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

/**
 * The Secret badges an app reports (store.AppFoundBadges; web lib/secretBadges.ts);
 * the server earns Hopeful and Lucky itself. Nothing on screen says one was
 * found: the Badge Index shows it next time.
 */
object SecretBadges {
    /** How long a padlock in the Badge Index is held for Curious. */
    const val CURIOUS_HOLD_MS = 10_000L

    private val reported = mutableSetOf<String>()

    /** Reports "curious" or "nosy" found, once per run; answers whether it is new to the account. */
    suspend fun report(badge: String): Boolean {
        synchronized(reported) { if (!reported.add(badge)) return false }
        return try {
            val res = Nexa.api.request("POST", "/users/@me/badges/found", buildJsonObject { put("badge", badge) }) as? JsonObject
            res?.get("new")?.jsonPrimitive?.booleanOrNull == true
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            synchronized(reported) { reported.remove(badge) }
            false
        }
    }

    /** Whether a Settings search is the word Nosy listens for: English, or this language's. */
    fun isSecretWord(query: String): Boolean {
        val q = query.trim().lowercase(Locale.ROOT)
        return q == "secret" || q == t("badges.secretWord").lowercase(Locale.ROOT)
    }

    /** Settings search: Nosy, reported in the background. */
    fun searched(query: String) {
        if (isSecretWord(query)) Nexa.scope.launch { report("nosy") }
    }
}
