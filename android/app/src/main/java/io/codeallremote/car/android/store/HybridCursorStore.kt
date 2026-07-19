package io.codeallremote.car.android.store

import io.codeallremote.car.android.net.CursorStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import java.util.concurrent.ConcurrentHashMap

/**
 * A [CursorStore] that persists cursors across process death.
 *
 * Strategy: keep the in-memory map as the source of truth for fast reads
 * (the WS hot path), and mirror writes to [CursorPersistence] (DataStore via
 * PersistedCursorPersistence) asynchronously. On startup, [hydrate] loads the
 * persisted values for known sessions.
 *
 * This keeps the WS dedup hot path allocation-free while guaranteeing a
 * process crash does not lose the cursor and force a full resync
 * (docs/18 §Event handling).
 */
class HybridCursorStore(
    private val persistence: CursorPersistence,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) : CursorStore() {

    private val mirror = ConcurrentHashMap<String, Long>()

    /** Eagerly load a session's persisted cursor into the in-memory map. */
    suspend fun hydrate(sessionId: String) {
        val seq = persistence.get(sessionId)
        if (seq > 0) {
            mirror[sessionId] = seq
            super.set(sessionId, seq)
        }
    }

    override fun get(sessionId: String): Long {
        val cached = mirror[sessionId]
        if (cached != null) return cached
        return super.get(sessionId)
    }

    override fun advanceIfContiguous(sessionId: String, sequence: Long): Boolean {
        val advanced = super.advanceIfContiguous(sessionId, sequence)
        if (advanced) {
            mirror[sessionId] = sequence
            persistAsync(sessionId, sequence)
        }
        return advanced
    }

    override fun set(sessionId: String, sequence: Long) {
        super.set(sessionId, sequence)
        mirror[sessionId] = sequence
        persistAsync(sessionId, sequence)
    }

    override fun forget(sessionId: String) {
        super.forget(sessionId)
        mirror.remove(sessionId)
        scope.launch { persistence.forget(sessionId) }
    }

    private fun persistAsync(sessionId: String, sequence: Long) {
        scope.launch { persistence.set(sessionId, sequence) }
    }
}
