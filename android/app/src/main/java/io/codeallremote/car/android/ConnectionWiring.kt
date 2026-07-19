package io.codeallremote.car.android

import android.content.Context
import io.codeallremote.car.android.data.CarClient
import io.codeallremote.car.android.store.HybridCursorStore
import io.codeallremote.car.android.store.PersistedCursorPersistence
import io.codeallremote.car.android.store.PersistedCursorStore
import io.codeallremote.car.android.store.SecureTokenStore
import io.codeallremote.car.android.store.ServerAccount

private fun localServerAccount(): ServerAccount =
    ServerAccount(
        id = "local",
        displayName = "CAR (local)",
        baseUrl = "https://car.example.invalid",
        deviceId = "android",
        pairedAt = ""
    )

// buildCarClientFor supplies the foreground CarConnectionService with a CarClient
// for a server id. For now only the local placeholder account resolves; unknown
// ids return null so the service stops rather than connecting to nothing.
fun buildCarClientFor(context: Context, serverId: String): CarClient? {
    val account = if (serverId == "local") localServerAccount() else return null
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
