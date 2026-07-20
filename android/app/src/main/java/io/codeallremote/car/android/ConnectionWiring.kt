package io.codeallremote.car.android

import android.content.Context
import io.codeallremote.car.android.data.CarClient
import io.codeallremote.car.android.store.HybridCursorStore
import io.codeallremote.car.android.store.PersistedCursorPersistence
import io.codeallremote.car.android.store.PersistedCursorStore
import io.codeallremote.car.android.store.SecureTokenStore
import io.codeallremote.car.android.store.ServerAccountStore
import kotlinx.coroutines.runBlocking

/**
 * Supplies the foreground CarConnectionService with a CarClient for a server id.
 * Resolves the paired account from ServerAccountStore; unknown ids return null
 * so the service stops rather than connecting to nothing.
 */
fun buildCarClientFor(context: Context, serverId: String): CarClient? {
    val account = runBlocking { ServerAccountStore(context).get(serverId) } ?: return null
    val tokens = SecureTokenStore(context)
    val persisted = PersistedCursorStore(context)
    val cursors = HybridCursorStore(PersistedCursorPersistence(persisted))
    return CarClient(
        account = account,
        tokenProvider = { tokens.getToken(account.id) },
        deviceIdProvider = { account.deviceId },
        cursorStore = cursors
    )
}
