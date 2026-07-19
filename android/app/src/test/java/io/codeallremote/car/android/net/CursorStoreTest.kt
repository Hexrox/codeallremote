package io.codeallremote.car.android.net

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CursorStoreTest {

    @Test
    fun `cursor starts at zero`() {
        val store = CursorStore()
        assertEquals(0L, store.get("ses-1"))
    }

    @Test
    fun `advances only on contiguous sequence`() {
        val store = CursorStore()
        // First event must be sequence 1.
        assertTrue(store.advanceIfContiguous("ses-1", 1L))
        // Out-of-order or duplicate is rejected.
        assertFalse(store.advanceIfContiguous("ses-1", 1L)) // duplicate
        assertFalse(store.advanceIfContiguous("ses-1", 5L)) // gap
        // Contiguous next advances.
        assertTrue(store.advanceIfContiguous("ses-1", 2L))
        assertEquals(2L, store.get("ses-1"))
    }

    @Test
    fun `sessions are independent`() {
        val store = CursorStore()
        assertTrue(store.advanceIfContiguous("ses-1", 1L))
        assertTrue(store.advanceIfContiguous("ses-1", 2L))
        // ses-2 starts fresh.
        assertEquals(0L, store.get("ses-2"))
        assertTrue(store.advanceIfContiguous("ses-2", 1L))
        assertEquals(2L, store.get("ses-1"))
        assertEquals(1L, store.get("ses-2"))
    }

    @Test
    fun `set overrides after replay`() {
        val store = CursorStore()
        store.set("ses-1", 42L)
        assertEquals(42L, store.get("ses-1"))
        // Next contiguous is 43.
        assertTrue(store.advanceIfContiguous("ses-1", 43L))
    }

    @Test
    fun `forget removes a session`() {
        val store = CursorStore()
        store.set("ses-1", 5L)
        store.forget("ses-1")
        assertEquals(0L, store.get("ses-1"))
    }

    @Test
    fun `snapshot is a copy`() {
        val store = CursorStore()
        store.set("ses-1", 3L)
        val snap = store.snapshot()
        store.set("ses-1", 4L)
        assertEquals(3L, snap["ses-1"])
    }
}
