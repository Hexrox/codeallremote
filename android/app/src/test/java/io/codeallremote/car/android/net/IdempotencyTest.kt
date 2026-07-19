package io.codeallremote.car.android.net

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class IdempotencyTest {
    @Test
    fun `keys are unique`() {
        val a = Idempotency.newKey()
        val b = Idempotency.newKey()
        assertNotEquals(a, b)
    }

    @Test
    fun `keys are within server bounds`() {
        val key = Idempotency.newKey()
        // Server requires 8-200 chars (docs/13).
        assertTrue(key.length in 8..200)
    }

    @Test
    fun `keys are hex`() {
        val key = Idempotency.newKey()
        assertTrue(key.matches(Regex("^[0-9a-f]+$")))
    }
}

class CarResultTest {
    @Test
    fun `serverCodeToClient maps known codes`() {
        assertEquals(CarResult.Code.Unauthorized, serverCodeToClient("unauthorized"))
        assertEquals(CarResult.Code.Conflict, serverCodeToClient("conflict"))
        assertEquals(CarResult.Code.ExpiredApproval, serverCodeToClient("expired_approval"))
        assertEquals(CarResult.Code.CursorExpired, serverCodeToClient("cursor_expired"))
        assertEquals(CarResult.Code.AdapterUnavailable, serverCodeToClient("adapter_unavailable"))
    }

    @Test
    fun `unknown code falls back to internal`() {
        assertEquals(CarResult.Code.Internal, serverCodeToClient("totally_unknown"))
    }
}
