package io.codeallremote.car.android.store

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Stores access tokens for paired servers in Android EncryptedSharedPreferences.
 *
 * Per docs/15: refresh credentials are bound to one device and encrypted at
 * rest. Tokens never appear in general preferences, logs, or DataStore.
 *
 * This is the only place tokens live; removing a server deletes its entry
 * here (which revokes local access; the server revocation is a separate call).
 */
class SecureTokenStore(context: Context) {

    private val prefs = run {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            FILE_NAME,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    fun putToken(serverId: String, token: String) {
        prefs.edit().putString(tokenKey(serverId), token).apply()
    }

    fun getToken(serverId: String): String? = prefs.getString(tokenKey(serverId), null)

    fun remove(serverId: String) {
        prefs.edit().remove(tokenKey(serverId)).apply()
    }

    /** Clears all cached tokens (cache clear does NOT revoke server auth). */
    fun clearAll() {
        prefs.edit().clear().apply()
    }

    private fun tokenKey(serverId: String) = "tok_$serverId"

    companion object {
        private const val FILE_NAME = "car_secrets"
    }
}
