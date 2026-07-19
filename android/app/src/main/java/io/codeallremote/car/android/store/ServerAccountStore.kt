package io.codeallremote.car.android.store

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import java.util.UUID

private val Context.serverListStore by preferencesDataStore("car_servers")

/**
 * Persists the non-secret server-account list via DataStore.
 *
 * Removing a server DOES NOT delete remote sessions/data; it only forgets
 * the local account and its token (docs/37 §Server detail).
 */
class ServerAccountStore(private val context: Context) {

    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    val accounts: Flow<List<ServerAccount>> = context.serverListStore.data.map { prefs ->
        prefs[KEY]?.let { json.decodeFromString(ListSerializer(ServerAccount.serializer()), it) }
            ?: emptyList()
    }

    suspend fun add(account: ServerAccount) {
        context.serverListStore.edit { prefs ->
            val list = prefs[KEY]?.let {
                json.decodeFromString(ListSerializer(ServerAccount.serializer()), it)
            } ?: emptyList()
            prefs[KEY] = json.encodeToString(
                ListSerializer(ServerAccount.serializer()),
                (list + account),
            )
        }
    }

    suspend fun remove(serverId: String) {
        context.serverListStore.edit { prefs ->
            val list = prefs[KEY]?.let {
                json.decodeFromString(ListSerializer(ServerAccount.serializer()), it)
            } ?: emptyList()
            prefs[KEY] = json.encodeToString(
                ListSerializer(ServerAccount.serializer()),
                list.filterNot { it.id == serverId },
            )
        }
    }

    suspend fun get(serverId: String): ServerAccount? =
        accounts.first().firstOrNull { it.id == serverId }

    private val KEY = stringPreferencesKey("servers")
}

/** Generate a fresh opaque server account ID. */
fun newServerAccountId(): String = "srv_" + UUID.randomUUID().toString()
