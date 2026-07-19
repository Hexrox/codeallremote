package io.codeallremote.car.android.net

import java.security.SecureRandom

/**
 * Generates client-side idempotency keys for write commands.
 *
 * Per docs/13-rest-api.md and docs/33-car-protocol.md, every mutating command
 * carries an idempotency key; the client retains it until the server outcome
 * is known so a retry never starts a second run.
 *
 * Keys are opaque to the server; we use 16 bytes of CSPRNG entropy, hex-encoded
 * (32 chars, within the 8-200 char server bound).
 */
object Idempotency {
    private val random = SecureRandom()

    fun newKey(): String {
        val bytes = ByteArray(16)
        random.nextBytes(bytes)
        return bytes.joinToString("") { "%02x".format(it) }
    }
}
