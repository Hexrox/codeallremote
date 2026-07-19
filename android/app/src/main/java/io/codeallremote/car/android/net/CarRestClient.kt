package io.codeallremote.car.android.net

import io.codeallremote.car.android.net.dto.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.serializer
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException

/**
 * Typed CAR REST client over OkHttp.
 *
 * Design rules from the spec:
 * - Base path /api/v1; UTF-8 JSON.
 * - Authenticated writes require Idempotency-Key (generated client-side,
 *   retained by the caller until outcome is known).
 * - No request/response bodies are logged (may contain secrets).
 * - Transport failures are surfaced as [CarResult.Code.Transport], NOT
 *   treated as success.
 */
class CarRestClient(
    @PublishedApi internal val httpClient: OkHttpClient,
    private val baseUrl: String,           // e.g. "https://car.example.invalid"
    private val tokenProvider: () -> String?,
    @PublishedApi internal val json: Json = CarJson,
) {
    @PublishedApi internal val mediaJson = "application/json; charset=utf-8".toMediaType()

    suspend fun listSessions(): CarResult<SessionListResponse> =
        get("/api/v1/sessions")

    suspend fun createSession(req: CreateSessionRequest): CarResult<SessionSnapshot> =
        post("/api/v1/sessions", req, requireIdempotency = true)

    suspend fun getSession(id: String): CarResult<SessionSnapshot> =
        get("/api/v1/sessions/$id")

    suspend fun startRun(sessionId: String): CarResult<StartRunResponse> =
        postEmpty("/api/v1/sessions/$sessionId/runs", requireIdempotency = true)

    suspend fun submitPrompt(sessionId: String, text: String): CarResult<Unit> =
        postVoid("/api/v1/sessions/$sessionId/prompts", SubmitPromptRequest(text), requireIdempotency = true)

    suspend fun interrupt(sessionId: String): CarResult<Unit> =
        postEmptyVoid("/api/v1/sessions/$sessionId/interrupt", requireIdempotency = true)

    suspend fun getEvents(sessionId: String, after: Long, limit: Int = 100): CarResult<EventsResponse> =
        get("/api/v1/sessions/$sessionId/events?after=$after&limit=$limit")

    suspend fun getApproval(id: String): CarResult<ApprovalResponse> =
        get("/api/v1/approvals/$id")

    suspend fun decideApproval(id: String, decision: String, reason: String?): CarResult<ApprovalResponse> =
        post("/api/v1/approvals/$id/decision", DecisionRequest(decision, reason), requireIdempotency = true)

    // --- Pairing (some endpoints unauthenticated) ---

    suspend fun createPairChallenge(): CarResult<PairChallengeResponse> =
        postEmpty("/api/v1/pair", requireIdempotency = false, auth = false)

    suspend fun pairDevice(code: String, name: String, pubKey: String): CarResult<PairDeviceResponse> =
        post("/api/v1/pair/$code", PairDeviceRequest(name, pubKey), requireIdempotency = false, auth = false)

    suspend fun getMe(): CarResult<MeResponse> = get("/api/v1/me")

    // --- HTTP plumbing ---

    private suspend inline fun <reified T> get(path: String): CarResult<T> =
        withContext(Dispatchers.IO) {
            val req = buildRequest(path).get().build()
            execute(req) { resp -> decodeBody(resp) }
        }

    private suspend inline fun <reified Req, reified Res> post(
        path: String, body: Req, requireIdempotency: Boolean, auth: Boolean = true,
    ): CarResult<Res> = withContext(Dispatchers.IO) {
        val builder = buildRequest(path, auth)
        if (requireIdempotency) builder.header("Idempotency-Key", Idempotency.newKey())
        val payload = json.encodeToString(serializer(), body).toRequestBody(mediaJson)
        val req = builder.post(payload).build()
        execute(req) { resp -> decodeBody(resp) }
    }

    private suspend inline fun <reified Res> postEmpty(
        path: String, requireIdempotency: Boolean, auth: Boolean = true,
    ): CarResult<Res> = withContext(Dispatchers.IO) {
        val builder = buildRequest(path, auth)
        if (requireIdempotency) builder.header("Idempotency-Key", Idempotency.newKey())
        val req = builder.post(EMPTY_BODY.toRequestBody(mediaJson)).build()
        execute(req) { resp -> decodeBody(resp) }
    }

    private suspend inline fun <reified Req> postVoid(
        path: String, body: Req, requireIdempotency: Boolean, auth: Boolean = true,
    ): CarResult<Unit> = withContext(Dispatchers.IO) {
        val builder = buildRequest(path, auth)
        if (requireIdempotency) builder.header("Idempotency-Key", Idempotency.newKey())
        val payload = json.encodeToString(serializer(), body).toRequestBody(mediaJson)
        val req = builder.post(payload).build()
        execute(req) { _ -> Unit }
    }

    private suspend inline fun postEmptyVoid(
        path: String, requireIdempotency: Boolean, auth: Boolean = true,
    ): CarResult<Unit> = withContext(Dispatchers.IO) {
        val builder = buildRequest(path, auth)
        if (requireIdempotency) builder.header("Idempotency-Key", Idempotency.newKey())
        val req = builder.post(EMPTY_BODY.toRequestBody(mediaJson)).build()
        execute(req) { _ -> Unit }
    }

    @PublishedApi
    internal fun buildRequest(path: String, auth: Boolean = true): Request.Builder {
        val b = Request.Builder().url("${baseUrl.trimEnd('/')}$path")
        if (auth) tokenProvider()?.let { b.header("Authorization", "Bearer $it") }
        return b
    }

    @PublishedApi
    internal inline fun <reified T> decodeBody(resp: Response): T {
        val text = resp.body?.string().orEmpty()
        return if (text.isBlank()) {
            // Unit-typed or 204 response with empty body: decode via serializer
            // which handles Unit/objects; fallback handled by caller.
            json.decodeFromString(serializer<T>(), "{}")
        } else {
            json.decodeFromString(serializer(), text)
        }
    }

    @PublishedApi
    internal inline fun <reified T> execute(req: Request, parse: (Response) -> T): CarResult<T> {
        val resp = try {
            httpClient.newCall(req).execute()
        } catch (e: IOException) {
            return CarResult.err(CarResult.Code.Transport, "network error: ${e.message}")
        }
        resp.use {
            return if (it.isSuccessful) {
                CarResult.ok(parse(it))
            } else {
                val errBody = runCatching {
                    json.decodeFromString(ApiError.serializer(), it.body?.string().orEmpty())
                }.getOrNull()
                val code = errBody?.code?.let(::serverCodeToClient)
                    ?: when (it.code) {
                        401 -> CarResult.Code.Unauthorized
                        403 -> CarResult.Code.Forbidden
                        404 -> CarResult.Code.NotFound
                        409 -> CarResult.Code.Conflict
                        else -> CarResult.Code.Internal
                    }
                CarResult.err(code, errBody?.message ?: "HTTP ${it.code}", it.code)
            }
        }
    }

    companion object {
        private val EMPTY_BODY = ""
    }
}
