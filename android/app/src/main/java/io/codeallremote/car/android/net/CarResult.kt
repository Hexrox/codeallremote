package io.codeallremote.car.android.net

/**
 * Outcome of a CAR API call. Failures carry the stable errcat code
 * (internal/errcat/catalog.go) so the UI can branch on it rather than prose.
 *
 * Network/transport failures that did not reach the server use [Code.Transport].
 */
sealed interface CarResult<out T> {
    data class Ok<T>(val value: T) : CarResult<T>
    data class Err(
        val code: Code,
        val message: String,
        val httpStatus: Int? = null,
    ) : CarResult<Nothing>

    /** Stable error codes mirrored from the server errcat. */
    enum class Code {
        Unauthorized,
        Forbidden,
        NotFound,
        Conflict,
        ExpiredApproval,
        IdempotencyConflict,
        CursorExpired,
        AdapterUnavailable,
        InvalidInput,
        UnsupportedCommand,
        RateLimited,
        Internal,
        Transport,
    }

    companion object {
        fun <T> ok(v: T): CarResult<T> = Ok(v)
        fun err(code: Code, message: String, httpStatus: Int? = null): CarResult<Nothing> =
            Err(code, message, httpStatus)
    }
}

/** Map a server errcat code string to the client enum. */
fun serverCodeToClient(code: String): CarResult.Code = when (code) {
    "unauthorized" -> CarResult.Code.Unauthorized
    "forbidden" -> CarResult.Code.Forbidden
    "not_found" -> CarResult.Code.NotFound
    "conflict" -> CarResult.Code.Conflict
    "expired_approval" -> CarResult.Code.ExpiredApproval
    "idempotency_conflict" -> CarResult.Code.IdempotencyConflict
    "cursor_expired" -> CarResult.Code.CursorExpired
    "adapter_unavailable" -> CarResult.Code.AdapterUnavailable
    "invalid_input" -> CarResult.Code.InvalidInput
    "unsupported_command" -> CarResult.Code.UnsupportedCommand
    "rate_limited" -> CarResult.Code.RateLimited
    "internal_error" -> CarResult.Code.Internal
    else -> CarResult.Code.Internal
}
