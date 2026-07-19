package io.codeallremote.car.android.notifications

import io.codeallremote.car.android.net.LiveEvent
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Test

class NotificationMapperTest {

    private val serverId = "srv-1"
    private val sessionId = "sess-1"
    private val sequence = 7L

    private fun buildEvent(type: String, approvalId: String? = null): LiveEvent {
        val payload = if (approvalId != null) {
            mapOf("approval_id" to JsonPrimitive(approvalId))
        } else {
            emptyMap()
        }
        return LiveEvent(
            sessionId = sessionId,
            sequence = sequence,
            type = type,
            payload = payload,
            occurredAt = null
        )
    }

    @Test
    fun approvalRequested_mapsToApprovalNotification() {
        val event = buildEvent("approval.requested", approvalId = "ap-1")
        val mapped = NotificationMapper.map(serverId, event)

        assertNotNull(mapped)
        mapped!!
        assertTrue(mapped.isApproval)
        assertEquals(PushCategory.approval_requested, mapped.payload.category)
        assertEquals("ap-1", mapped.payload.resourceId)
        assertEquals("sess-1:7", mapped.payload.nonce)
        assertEquals(serverId, mapped.payload.serverId)
    }

    @Test
    fun approvalRequested_withoutApprovalId_fallsBackToSessionId() {
        val event = buildEvent("approval.requested")
        val mapped = NotificationMapper.map(serverId, event)

        assertNotNull(mapped)
        mapped!!
        assertTrue(mapped.isApproval)
        assertEquals(sessionId, mapped.payload.resourceId)
        assertEquals("sess-1:7", mapped.payload.nonce)
    }

    @Test
    fun runCompleted_mapsToRunCompletedNotification() {
        val event = buildEvent("run.completed")
        val mapped = NotificationMapper.map(serverId, event)

        assertNotNull(mapped)
        mapped!!
        assertFalse(mapped.isApproval)
        assertEquals(PushCategory.run_completed, mapped.payload.category)
        assertEquals(sessionId, mapped.payload.resourceId)
        assertEquals("sess-1:7", mapped.payload.nonce)
    }

    @Test
    fun runFailed_mapsToRunFailedNotification() {
        val event = buildEvent("run.failed")
        val mapped = NotificationMapper.map(serverId, event)

        assertNotNull(mapped)
        mapped!!
        assertFalse(mapped.isApproval)
        assertEquals(PushCategory.run_failed, mapped.payload.category)
        assertEquals(sessionId, mapped.payload.resourceId)
        assertEquals("sess-1:7", mapped.payload.nonce)
    }

    @Test
    fun unknownType_returnsNull() {
        val event = buildEvent("run.output")
        val mapped = NotificationMapper.map(serverId, event)

        assertNull(mapped)
    }
}
