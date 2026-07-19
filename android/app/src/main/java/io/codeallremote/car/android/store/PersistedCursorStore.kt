package io.codeallremote.car.android.store

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.cursorStore by preferencesDataStore("car_cursors")

/**
 * Per-session cursors persisted across process death (docs/18 §Event handling).
 *
 * The client stores the highest contiguous sequence per session and sends it
 * in hello on reconnect. Persistent storage guarantees a crash does not lose
 * the cursor and force a full resync.
 */
class PersistedCursorStore(private val context: Context) {

    suspend fun get(sessionId: String): Long = context.cursorStore.data.map { prefs ->
        prefs[longPreferencesKey(sessionId)] ?: 0L
    }.first()

    suspend fun set(sessionId: String, sequence: Long) {
        context.cursorStore.edit { it[longPreferencesKey(sessionId)] = sequence }
    }

    suspend fun forget(sessionId: String) {
        context.cursorStore.edit { it.remove(longPreferencesKey(sessionId)) }
    }

    /** Clears all cursors (cache clear; does NOT revoke server auth). */
    suspend fun clearAll() {
        context.cursorStore.edit { it.clear() }
    }
}
