package io.codeallremote.car.android.data

import io.codeallremote.car.android.net.CarRestClient
import io.codeallremote.car.android.net.CarWsClient
import io.codeallremote.car.android.net.CursorStore
import io.codeallremote.car.android.store.ServerAccount
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/**
 * A bound CAR client for one server account: REST + WS sharing the same
 * OkHttp instance, cursors, and token provider.
 *
 * The token provider reads from SecureTokenStore so tokens are never held in
 * plain fields longer than necessary.
 */
class CarClient(
    val account: ServerAccount,
    private val tokenProvider: () -> String?,
    private val deviceIdProvider: () -> String,
    private val cursorStore: CursorStore,
    httpClient: OkHttpClient = defaultHttpClient(),
    json: Json = io.codeallremote.car.android.net.CarJson,
) {
    val rest: CarRestClient = CarRestClient(httpClient, account.baseUrl, tokenProvider, json)
    val ws: CarWsClient = CarWsClient(httpClient, account.baseUrl, tokenProvider, deviceIdProvider, cursorStore, json)

    companion object {
        fun defaultHttpClient(): OkHttpClient = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .pingInterval(20, TimeUnit.SECONDS) // WS keepalive
            // No body logging interceptor: payloads may contain secrets.
            .build()
    }
}
