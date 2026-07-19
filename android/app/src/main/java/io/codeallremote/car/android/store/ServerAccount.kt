package io.codeallremote.car.android.store

import kotlinx.serialization.Serializable

/**
 * A registered CAR server record.
 *
 * The access token is NOT stored here — it lives in [SecureTokenStore]
 * (Android EncryptedSharedPreferences). This record holds only non-secret
 * metadata so it can live in plain DataStore without risk.
 */
@Serializable
data class ServerAccount(
    val id: String,            // opaque client-generated UUID
    val displayName: String,
    val baseUrl: String,       // https://host
    val deviceId: String,      // paired device ID
    val pairedAt: String,
)

/**
 * Pairing result that becomes a [ServerAccount] + a secret stored separately.
 */
@Serializable
data class PairingResult(
    val account: ServerAccount,
    val accessToken: String,
)
