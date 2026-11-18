package fish.nexa.ui

import fish.nexa.crypto.WeakKdfError
import fish.nexa.net.ApiError

/** A sentence for whatever went wrong (web/src/lib/errors.ts describeError). */
fun describeError(err: Throwable?, fallback: String? = null): String {
    if (err is WeakKdfError) return t("auth.theServerAskedToProtectYourPassword")
    if (err is ReadableError) return err.message ?: fallback ?: t("modals.confirm.somethingWentWrong")
    if (err is ApiError) {
        when (err.code) {
            "network" -> return t("offline.couldNotReachTheServer")
            "timeout" -> return t("errors.theServerTookTooLong")
            "aborted" -> return t("errors.thatWasCancelledBeforeItFinished")
            "bad_response" -> return t("errors.theServerSAnswerMadeNo")
            "rate_limited" -> return t("errors.youAreDoingThatTooQuickly")
            "owner_access_refused" -> return t("errors.ownerAccessRefused")
            "owner_active" -> return t("errors.ownerActive")
            "too_many_uploads" -> return t("errors.tooManyUploads")
            "upload_quota" -> return t("errors.uploadQuota")
            "no_space" -> return t("errors.noSpace")
            "storage_budget" -> return t("errors.storageBudget")
            "gems_short" -> return t("errors.gemsShort")
            "gems_held" -> return t("errors.gemsHeld")
            "gems_no_collectibles" -> return t("errors.gemsNoCollectibles")
            "gems_fuse" -> return t("errors.gemsFuse")
            "trade_gems_held" -> return t("errors.tradeGemsHeld")
            "trade_gems_linked" -> return t("errors.tradeGemsLinked")
        }
        if (!err.code.startsWith("http_") && !err.message.isNullOrBlank()) return err.message!!
        return fallback ?: err.message ?: t("modals.confirm.somethingWentWrong")
    }
    return fallback ?: t("modals.confirm.somethingWentWrong")
}

/** An error whose message was written for the person reading it. */
class ReadableError(message: String) : Exception(message)
