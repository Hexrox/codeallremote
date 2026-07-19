package io.codeallremote.car.android.data

import io.codeallremote.car.android.net.*
import io.codeallremote.car.android.net.dto.*
import io.codeallremote.car.android.store.PersistedCursorStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Aggregates REST + live events into a derived per-session projection.
 *
 * Per docs/18 §State ownership: server state (sessions/approvals/cursors) is
 * derived from CAR APIs/events; device state (selected server, drafts,
 * notification prefs) stays local. This repository owns the server-side
 * projection; the UI holds drafts and transient state.
 */
class CarRepository(
    private val rest: CarRestClient,
    private val cursorStore: PersistedCursorStore,
) {
    /** Derived session snapshots keyed by ID (authoritative = server-derived). */
    private val _sessions = MutableStateFlow<Map<String, SessionSnapshot>>(emptyMap())
    val sessions: StateFlow<Map<String, SessionSnapshot>> = _sessions.asStateFlow()

    /** Live events keyed by session (the UI timeline reads these). */
    private val _timeline = MutableStateFlow<Map<String, List<LiveEvent>>>(emptyMap())
    val timeline: StateFlow<Map<String, List<LiveEvent>>> = _timeline.asStateFlow()

    private val applyMutex = Mutex()

    suspend fun refreshSessions(): CarResult<Unit> = when (val r = rest.listSessions()) {
        is CarResult.Ok -> {
            val map = r.value.sessions.associateBy { it.id }
            _sessions.value = map
            CarResult.ok(Unit)
        }
        is CarResult.Err -> r
    }

    suspend fun loadSession(id: String): CarResult<SessionSnapshot> {
        // Also seed the cursor from the snapshot's last_sequence.
        return when (val r = rest.getSession(id)) {
            is CarResult.Ok -> {
                _sessions.value = _sessions.value + (id to r.value)
                cursorStore.set(id, r.value.lastSequence)
                r
            }
            is CarResult.Err -> r
        }
    }

    suspend fun createSession(req: CreateSessionRequest): CarResult<SessionSnapshot> =
        when (val r = rest.createSession(req)) {
            is CarResult.Ok -> {
                _sessions.value = _sessions.value + (r.value.id to r.value)
                r
            }
            is CarResult.Err -> r
        }

    suspend fun startRun(sessionId: String): CarResult<StartRunResponse> = rest.startRun(sessionId)
    suspend fun submitPrompt(sessionId: String, text: String): CarResult<Unit> =
        rest.submitPrompt(sessionId, text)
    suspend fun interrupt(sessionId: String): CarResult<Unit> = rest.interrupt(sessionId)

    suspend fun getApproval(id: String): CarResult<ApprovalResponse> = rest.getApproval(id)
    suspend fun decideApproval(id: String, decision: String, reason: String?): CarResult<ApprovalResponse> =
        rest.decideApproval(id, decision, reason)

    /**
     * Applies a live event to the derived projection. Idempotent by sequence;
     * a gap is detected and the caller requests REST replay.
     */
    suspend fun applyLiveEvent(ev: LiveEvent) = applyMutex.withLock {
        val list = _timeline.value[ev.sessionId].orEmpty()
        if (list.any { it.sequence == ev.sequence }) return@withLock // dedupe
        // Detect gap: if the event is ahead of the next contiguous sequence,
        // we apply it anyway (so the UI sees the latest) but the gap is
        // visible; the WS layer has already triggered replay via resync.
        _timeline.value = _timeline.value + (ev.sessionId to (list + ev))
        // Persist the cursor and (optimistically) update the snapshot state
        // for lifecycle events so the header reflects transitions promptly.
        cursorStore.set(ev.sessionId, ev.sequence)
        when (ev.type) {
            "run.started" -> updateSessionState(ev.sessionId, "active")
            "run.completed" -> updateSessionState(ev.sessionId, "completed")
            "run.interrupted" -> updateSessionState(ev.sessionId, "interrupted")
            "approval.requested" -> updateSessionState(ev.sessionId, "waiting_approval")
        }
    }

    private fun updateSessionState(sessionId: String, state: String) {
        val cur = _sessions.value[sessionId] ?: return
        _sessions.value = _sessions.value + (sessionId to cur.copy(state = state))
    }

    /** Seed the timeline from a REST replay (after reconnect/resync). */
    suspend fun seedTimelineFromReplay(sessionId: String, events: List<EventDto>) = applyMutex.withLock {
        val mapped = events.map { ev ->
            LiveEvent(
                sessionId = ev.sessionId, sequence = ev.sequence,
                type = ev.type, payload = ev.payload.mapValues { it.value },
                occurredAt = null,
            )
        }
        _timeline.value = _timeline.value + (sessionId to mapped)
        if (mapped.isNotEmpty()) cursorStore.set(sessionId, mapped.last().sequence)
    }

    suspend fun replay(sessionId: String, after: Long): CarResult<EventsResponse> =
        rest.getEvents(sessionId, after).alsoOnOk { resp ->
            seedTimelineFromReplay(sessionId, resp.events)
        }

    private inline fun <T> CarResult<T>.alsoOnOk(block: (T) -> Unit): CarResult<T> {
        if (this is CarResult.Ok) block(value)
        return this
    }

    fun clearLocalProjection() {
        _sessions.value = emptyMap()
        _timeline.value = emptyMap()
    }
}
