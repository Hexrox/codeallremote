package io.codeallremote.car.android.notifications

import io.codeallremote.car.android.net.LiveEvent
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

data class MappedNotification(val payload: PushPayload, val isApproval: Boolean)

object NotificationMapper {
    fun map(serverId: String, event: LiveEvent): MappedNotification? {
        val nonce = "${event.sessionId}:${event.sequence}"
        return when (event.type) {
            "approval.requested" -> {
                val resourceId = event.payload["approval_id"]?.jsonPrimitive?.contentOrNull ?: event.sessionId
                MappedNotification(
                    payload = PushPayload(
                        serverId = serverId,
                        resourceId = resourceId,
                        category = PushCategory.approval_requested,
                        nonce = nonce
                    ),
                    isApproval = true
                )
            }
            "run.completed" -> MappedNotification(
                payload = PushPayload(
                    serverId = serverId,
                    resourceId = event.sessionId,
                    category = PushCategory.run_completed,
                    nonce = nonce
                ),
                isApproval = false
            )
            "run.failed" -> MappedNotification(
                payload = PushPayload(
                    serverId = serverId,
                    resourceId = event.sessionId,
                    category = PushCategory.run_failed,
                    nonce = nonce
                ),
                isApproval = false
            )
            else -> null
        }
    }
}
