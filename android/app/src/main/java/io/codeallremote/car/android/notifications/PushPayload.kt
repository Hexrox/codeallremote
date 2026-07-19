package io.codeallremote.car.android.notifications

import kotlinx.serialization.Serializable

/**
 * Push notification payload (docs/19 §Push notifications, M2-19 acceptance).
 *
 * Contains ONLY identifiers and a generic category. NEVER command text,
 * filenames, transcripts, or credentials. The app fetches sensitive content
 * only after authenticated app unlock.
 */
@Serializable
data class PushPayload(
    val serverId: String,
    val resourceId: String,        // session or approval ID
    val category: PushCategory,
    val nonce: String,             // dedup nonce
)

@Serializable
enum class PushCategory {
    approval_requested,
    run_completed,
    run_failed,
    security_event,
}

/** Map a category to a generic, secret-free notification title. */
fun PushPayload.genericTitle(): String = when (category) {
    PushCategory.approval_requested -> "Action requires approval"
    PushCategory.run_completed -> "Run completed"
    PushCategory.run_failed -> "Run failed"
    PushCategory.security_event -> "Server security event"
}

/** Map a category to a generic body (no content). */
fun PushPayload.genericBody(): String = when (category) {
    PushCategory.approval_requested -> "Open CAR to review the approval request."
    PushCategory.run_completed -> "A run finished. Open CAR for details."
    PushCategory.run_failed -> "A run failed. Open CAR for details."
    PushCategory.security_event -> "Open CAR to review."
}
