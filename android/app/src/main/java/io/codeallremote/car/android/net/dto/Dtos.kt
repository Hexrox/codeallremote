package io.codeallremote.car.android.net.dto

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

/**
 * DTOs matching the CAR REST/WebSocket contract (docs/13, docs/14).
 *
 * Server IDs are opaque; clients MUST NOT infer structure.
 * Payloads contain no secrets beyond what the server already redacted.
 */

// --- REST responses ---

@Serializable
data class SessionSnapshot(
    val id: String,
    val workspaceId: String,
    val adapterId: String,
    val state: String,
    val lastSequence: Long,
    val pendingApprovalId: String? = null,
    val title: String? = null,
    val updatedAt: String? = null,
)

@Serializable
data class SessionListResponse(val sessions: List<SessionSnapshot> = emptyList())

@Serializable
data class CreateSessionRequest(
    val workspaceId: String,
    val adapterId: String,
    val title: String? = null,
)

@Serializable
data class StartRunResponse(val runId: String, val state: String, val message: String? = null)

@Serializable
data class SubmitPromptRequest(val text: String)

@Serializable
data class ApprovalResponse(
    val id: String,
    val sessionId: String,
    val state: String,
    val category: String? = null,
    val expiresAt: String,
)

@Serializable
data class DecisionRequest(val decision: String, val reason: String? = null)

@Serializable
data class EventDto(
    val type: String,
    val messageId: String,
    val sessionId: String,
    val sequence: Long,
    val schemaVersion: Int = 1,
    // Payload values may be strings, numbers, booleans, or nested objects
    // (schemas/event-v1.json allows arbitrary object values). Use JsonElement
    // so a numeric run_id or nested payload deserializes correctly instead of
    // failing on a Map<String, String>.
    val payload: Map<String, JsonElement> = emptyMap(),
)

@Serializable
data class EventsResponse(
    val events: List<EventDto> = emptyList(),
    val nextAfter: Long = 0,
    val resyncRequired: Boolean = false,
    val hasMore: Boolean = false,
)

@Serializable
data class ApiError(val code: String, val message: String, val requestId: String? = null)

// --- Pairing ---

@Serializable
data class PairChallengeResponse(val code: String, val expiresAt: String)

@Serializable
data class PairDeviceRequest(val deviceName: String, val devicePubKey: String)

@Serializable
data class PairDeviceResponse(val accessToken: String, val deviceId: String, val expiresAt: String)

@Serializable
data class MeResponse(val user: String, val deviceId: String, val role: String)

// --- WebSocket envelopes ---

@Serializable
data class Hello(
    val type: String = "hello",
    val protocolVersion: Int = 1,
    val deviceId: String,
    val cursors: List<Cursor> = emptyList(),
)

@Serializable
data class Cursor(val sessionId: String, val after: Long)

@Serializable
data class WsEnvelope(
    val type: String,
    val messageId: String? = null,
    val occurredAt: String? = null,
    val sessionId: String? = null,
    val sequence: Long? = null,
    // Payload may contain non-string values (e.g. numeric run_id). Use
    // JsonElement to deserialize any value type safely.
    val payload: Map<String, JsonElement> = emptyMap(),
)
