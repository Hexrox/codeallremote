package io.codeallremote.car.android.ui.navigation

/**
 * Stable deep-link routes (docs/18 §Navigation model).
 *
 * Deep links from notifications validate server, token and resource
 * authorization before displaying private content.
 */
object Routes {
    const val HOME = "home"

    // /servers/{serverId}/...
    const val SERVER = "servers/{serverId}"
    fun server(serverId: String) = "servers/$serverId"

    const val WORKSPACE = "servers/{serverId}/workspaces/{workspaceId}"
    fun workspace(serverId: String, workspaceId: String) =
        "servers/$serverId/workspaces/$workspaceId"

    const val SESSION = "servers/{serverId}/sessions/{sessionId}"
    fun session(serverId: String, sessionId: String) =
        "servers/$serverId/sessions/$sessionId"

    const val APPROVAL = "servers/{serverId}/approvals/{approvalId}"
    fun approval(serverId: String, approvalId: String) =
        "servers/$serverId/approvals/$approvalId"

    const val PAIR = "servers/{serverId}/pair"
    fun pair(serverId: String) = "servers/$serverId/pair"

    // car:// deep-link scheme (manifest).
    const val DEEP_LINK_SCHEME = "car"
    const val DEEP_LINK_HOST_SESSION = "session"
    const val DEEP_LINK_HOST_APPROVAL = "approval"

    /** Build a car:// deep link for a notification (identifiers only). */
    fun approvalDeepLink(serverId: String, approvalId: String) =
        "$DEEP_LINK_SCHEME://$DEEP_LINK_HOST_APPROVAL/$serverId/$approvalId"

    fun sessionDeepLink(serverId: String, sessionId: String) =
        "$DEEP_LINK_SCHEME://$DEEP_LINK_HOST_SESSION/$serverId/$sessionId"
}
