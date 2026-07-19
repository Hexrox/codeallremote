package io.codeallremote.car.android.net

import java.util.concurrent.ConcurrentHashMap

/**
 * Persists the highest contiguous sequence per session.
 *
 * Per docs/14-websocket-protocol.md §Reconnect: the client stores the last
 * contiguous sequence and sends it in hello on reconnect. Events are applied
 * idempotently and the cursor only advances on a contiguous run.
 *
 * This base implementation is in-memory (fast WS hot path). A persisted
 * implementation (HybridCursorStore) mirrors writes to DataStore so a process
 * crash does not force a full resync — see the store package.
 */
open class CursorStore {
    protected val cursors = ConcurrentHashMap<String, Long>()

    /** The highest contiguous sequence recorded for a session (0 if none). */
    open fun get(sessionId: String): Long = cursors[sessionId] ?: 0L

    /**
     * Advances the cursor only if [sequence] is the next contiguous value.
     * Returns true if the cursor advanced (event is new and in-order).
     */
    open fun advanceIfContiguous(sessionId: String, sequence: Long): Boolean {
        val current = cursors[sessionId] ?: 0L
        if (sequence != current + 1) return false
        cursors[sessionId] = sequence
        return true
    }

    /** Sets an absolute cursor (after a REST replay / resync). */
    open fun set(sessionId: String, sequence: Long) {
        cursors[sessionId] = sequence
    }

    open fun forget(sessionId: String) {
        cursors.remove(sessionId)
    }

    fun snapshot(): Map<String, Long> = cursors.toMap()
}
