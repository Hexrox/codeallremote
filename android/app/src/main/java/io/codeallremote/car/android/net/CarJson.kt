package io.codeallremote.car.android.net

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNamingStrategy

/**
 * Shared JSON configuration.
 *
 * - Snake-case mapping: the server uses snake_case JSON fields; Kotlin
 *   properties are camelCase, the strategy bridges them so DTOs stay idiomatic.
 * - Unknown keys ignored: additive protocol fields stay readable by an older
 *   client (docs/34-protocol-versioning.md).
 */
@OptIn(ExperimentalSerializationApi::class)
val CarJson: Json = Json {
    ignoreUnknownKeys = true
    isLenient = true
    encodeDefaults = true
    explicitNulls = false
    namingStrategy = JsonNamingStrategy.SnakeCase
}
