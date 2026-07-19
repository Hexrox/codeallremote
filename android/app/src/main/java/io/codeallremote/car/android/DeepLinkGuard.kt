package io.codeallremote.car.android

import android.content.Intent
import android.net.Uri
import io.codeallremote.car.android.store.SecureTokenStore

/**
 * Validates an incoming deep link before private content is rendered
 * (docs/18 §Navigation model: "Deep links from notifications must validate
 * the server, token and resource authorization before displaying private
 * content").
 *
 * A deep link referencing a server we have no token for is downgraded to the
 * home screen rather than rendering private session/approval content — the
 * user is re-prompted to reauthenticate.
 */
object DeepLinkGuard {

    /** @return the validated (serverId, kind, resourceId) or null if untrusted. */
    fun validate(intent: Intent?): DeepLinkTarget? {
        val uri = intent?.data ?: return null
        if (uri.scheme != "car") return null
        val kind = uri.host ?: return null
        val segments = uri.pathSegments
        if (segments.size < 2) return null
        val serverId = segments[0]
        val resourceId = segments[1]
        return DeepLinkTarget(serverId = serverId, kind = kind, resourceId = resourceId)
    }

    /**
     * Confirms the device still holds a token for the deep-link's server. If
     * not, the caller routes to home instead of the private destination.
     */
    fun isAuthorized(context: android.content.Context, target: DeepLinkTarget?): Boolean {
        if (target == null) return false
        val store = SecureTokenStore(context)
        return store.getToken(target.serverId) != null
    }
}

data class DeepLinkTarget(
    val serverId: String,
    val kind: String,        // "session" or "approval"
    val resourceId: String,
)
