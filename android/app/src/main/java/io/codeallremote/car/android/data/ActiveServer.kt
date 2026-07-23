package io.codeallremote.car.android.data

import android.content.Context
import io.codeallremote.car.android.store.HybridCursorStore
import io.codeallremote.car.android.store.PersistedCursorPersistence
import io.codeallremote.car.android.store.PersistedCursorStore
import io.codeallremote.car.android.store.SecureTokenStore
import io.codeallremote.car.android.store.ServerAccountStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking

object ActiveServer {
    private var client: CarClient? = null
    private var repo: CarRepository? = null
    private var boundId: String? = null

    @Synchronized
    fun bind(context: Context): Boolean {
        val app = context.applicationContext
        // Pick the most recently paired account, not the oldest: re-pairing adds
        // a new account, and older accounts may hold tokens the server no longer
        // honors (device removed) -> "invalid or expired token".
        val account = runBlocking {
            ServerAccountStore(app).accounts.first().maxByOrNull { it.pairedAt }
        } ?: return false
        if (boundId == account.id && client != null && repo != null) return true
        // Account changed: stop the previous shared socket before rebinding.
        client?.ws?.stop()
        val tokens = SecureTokenStore(app)
        val cursors = HybridCursorStore(PersistedCursorPersistence(PersistedCursorStore(app)))
        val c = CarClient(account, { tokens.getToken(account.id) }, { account.deviceId }, cursors)
        client = c
        repo = CarRepository(c.rest, PersistedCursorStore(app))
        boundId = account.id
        c.ws.start()
        return true
    }
    fun client(): CarClient? = client
    fun repo(): CarRepository? = repo
}
