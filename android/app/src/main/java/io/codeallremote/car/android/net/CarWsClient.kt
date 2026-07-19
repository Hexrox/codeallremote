package io.codeallremote.car.android.net

import io.codeallremote.car.android.net.dto.*
import kotlinx.coroutines.*
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.*
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.*

/**
 * Connection state machine (docs/38 §Connection):
 *
 *   disconnected -> connecting -> authenticated -> live
 *         ^             |             |           |
 *         |             v             v           v
 *         +-------- retry/backoff <- expired <- transport_lost
 */
sealed interface ConnectionState {
    data object Disconnected : ConnectionState
    data object Connecting : ConnectionState
    data object Authenticated : ConnectionState
    data object Live : ConnectionState
    data class Failed(val transient: Boolean, val code: CarResult.Code) : ConnectionState
}

/** A live event delivered to the UI, already deduplicated by the cursor store. */
data class LiveEvent(
    val sessionId: String,
    val sequence: Long,
    val type: String,
    val payload: Map<String, JsonElement>,
    val occurredAt: String?,
)

/**
 * CAR WebSocket client.
 *
 * Responsibilities (docs/14 `WebSocket protocol`, M2-02 acceptance):
 *  - hello/welcome handshake carrying device ID + per-session cursors;
 *  - replay available events then switch to live delivery;
 *  - deduplicate by (session_id, sequence) via the cursor store;
 *  - on `resync_required`, emit a [WsSignal.ResyncRequired] and pause live
 *    application for that session until a snapshot is fetched;
 *  - backpressure: the server closes us (close code 4001) when we are slow;
 *    we reconnect from the cursor with bounded, jittered backoff.
 */
class CarWsClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
    private val tokenProvider: () -> String?,
    private val deviceIdProvider: () -> String,
    private val cursorStore: CursorStore,
    private val json: Json = CarJson,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    /** Signals the UI consumes (in addition to the live-event flow). */
    sealed interface WsSignal {
        data class Welcome(val serverTime: String?) : WsSignal
        data class ResyncRequired(val sessionId: String, val after: Long) : WsSignal
        data class Connected(val state: ConnectionState) : WsSignal
        data object SlowClientDisconnected : WsSignal
        data object Closed : WsSignal
    }

    private val _signals = MutableSharedFlow<WsSignal>(extraBufferCapacity = 64)
    val signals: SharedFlow<WsSignal> = _signals.asSharedFlow()

    private val _events = MutableSharedFlow<LiveEvent>(extraBufferCapacity = 256)
    val events: SharedFlow<LiveEvent> = _events.asSharedFlow()

    private val _state = MutableStateFlow<ConnectionState>(ConnectionState.Disconnected)
    val state: StateFlow<ConnectionState> = _state.asStateFlow()

    private var loopJob: Job? = null
    private var currentSocket: WebSocket? = null

    /** Begin connecting and stay connected until [stop] is called. */
    fun start() {
        if (loopJob?.isActive == true) return
        loopJob = scope.launch { connectLoop() }
    }

    fun stop() {
        loopJob?.cancel()
        loopJob = null
        currentSocket?.cancel()
        currentSocket = null
        _state.value = ConnectionState.Disconnected
        _signals.tryEmit(WsSignal.Closed)
    }

    private suspend fun connectLoop() {
        var attempt = 0
        while (currentCoroutineContext().isActive) {
            try {
                connectOnce()
                attempt++
                _state.value = ConnectionState.Disconnected
            } catch (c: CancellationException) {
                throw c
            } catch (e: Throwable) {
                attempt++
                _state.value = ConnectionState.Failed(transient = true, code = CarResult.Code.Transport)
            }
            val delayMs = backoffMs(attempt)
            try {
                delay(delayMs)
            } catch (c: CancellationException) {
                throw c
            }
        }
    }

    private suspend fun connectOnce() {
        val token = tokenProvider() ?: run {
            _state.value = ConnectionState.Failed(transient = false, code = CarResult.Code.Unauthorized)
            return
        }
        val wsScheme = baseUrl.replaceFirst("https://", "wss://").replaceFirst("http://", "ws://")
        val url = "$wsScheme/api/v1/ws?token=$token"
        _state.value = ConnectionState.Connecting

        val socket = client.newWebSocket(
            Request.Builder().url(url).build(),
            CarWsListener(deviceIdProvider, cursorStore, _signals, _state, _events, json, scope),
        )
        currentSocket = socket
        _state.value = ConnectionState.Live
        try {
            awaitCancellation()
        } catch (c: CancellationException) {
            socket.cancel()
            throw c
        }
    }

    private fun backoffMs(attempt: Int): Long {
        val capped = (1L shl minOf(attempt, 5)) * 1000L  // 1s,2s,4s..32s cap
        val jitter = (Math.random() * 500).toLong()
        return minOf(capped + jitter, 32_000L)
    }
}

private class CarWsListener(
    private val deviceIdProvider: () -> String,
    private val cursorStore: CursorStore,
    private val signals: MutableSharedFlow<CarWsClient.WsSignal>,
    private val state: MutableStateFlow<ConnectionState>,
    private val events: MutableSharedFlow<LiveEvent>,
    private val json: Json,
    private val scope: CoroutineScope,
) : WebSocketListener() {

    override fun onOpen(webSocket: WebSocket, response: Response) {
        state.value = ConnectionState.Authenticated
        val hello = Hello(
            deviceId = deviceIdProvider(),
            cursors = cursorStore.snapshot().map { Cursor(it.key, it.value) },
        )
        webSocket.send(json.encodeToString(Hello.serializer(), hello))
        signals.tryEmit(CarWsClient.WsSignal.Connected(ConnectionState.Live))
    }

    override fun onMessage(webSocket: WebSocket, text: String) {
        scope.launch { handleMessage(text) }
    }

    override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        // 4001 = backpressure: we were slow. Reconnect from cursor (backoff).
        if (code == 4001) signals.tryEmit(CarWsClient.WsSignal.SlowClientDisconnected)
        signals.tryEmit(CarWsClient.WsSignal.Closed)
    }

    override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        state.value = ConnectionState.Failed(transient = true, code = CarResult.Code.Transport)
        signals.tryEmit(CarWsClient.WsSignal.Closed)
    }

    private suspend fun handleMessage(raw: String) {
        val env = runCatching {
            json.decodeFromString(WsEnvelope.serializer(), raw)
        }.getOrNull() ?: return

        when (env.type) {
            "welcome" -> signals.emit(
                CarWsClient.WsSignal.Welcome((env.payload["server_time"] as? JsonPrimitive)?.content),
            )
            "resync_required" -> {
                val sid = env.sessionId ?: return
                val after = env.sequence ?: 0L
                cursorStore.set(sid, after)
                signals.emit(CarWsClient.WsSignal.ResyncRequired(sid, after))
            }
            else -> {
                val sid = env.sessionId ?: return
                val seq = env.sequence ?: return
                // Deduplicate: only forward the next contiguous event. A gap
                // means we paused live application; a REST replay fills it.
                if (cursorStore.advanceIfContiguous(sid, seq)) {
                    events.emit(
                        LiveEvent(
                            sessionId = sid,
                            sequence = seq,
                            type = env.type,
                            payload = env.payload,
                            occurredAt = env.occurredAt,
                        ),
                    )
                }
            }
        }
    }
}
