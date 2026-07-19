package io.codeallremote.car.android.store

/**
 * Suspend persistence backend for cursors. [PersistedCursorStore] is the
 * DataStore implementation; tests supply an in-memory fake.
 */
interface CursorPersistence {
    suspend fun get(sessionId: String): Long
    suspend fun set(sessionId: String, sequence: Long)
    suspend fun forget(sessionId: String)
}

/**
 * In-memory [CursorPersistence] for JVM tests (no Context/DataStore needed).
 */
class InMemoryCursorPersistence : CursorPersistence {
    private val map = mutableMapOf<String, Long>()
    override suspend fun get(sessionId: String): Long = map[sessionId] ?: 0L
    override suspend fun set(sessionId: String, sequence: Long) { map[sessionId] = sequence }
    override suspend fun forget(sessionId: String) { map.remove(sessionId) }
    fun snapshot(): Map<String, Long> = map.toMap()
}

/** Adapt PersistedCursorStore (DataStore-backed) to [CursorPersistence]. */
class PersistedCursorPersistence(private val store: PersistedCursorStore) : CursorPersistence {
    override suspend fun get(sessionId: String): Long = store.get(sessionId)
    override suspend fun set(sessionId: String, sequence: Long) = store.set(sessionId, sequence)
    override suspend fun forget(sessionId: String) = store.forget(sessionId)
}
