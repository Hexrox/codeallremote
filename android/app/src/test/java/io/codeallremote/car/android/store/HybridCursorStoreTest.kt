package io.codeallremote.car.android.store

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class HybridCursorStoreTest {

    private fun store() = HybridCursorStore(
        InMemoryCursorPersistence(),
        TestScope(StandardTestDispatcher()),
    )

    @Test
    fun `advance contiguous persists`() = runTest {
        val s = store()
        assertTrue(s.advanceIfContiguous("ses-1", 1L))
        assertTrue(s.advanceIfContiguous("ses-1", 2L))
        // Mirror reflects the in-memory value synchronously.
        assertEquals(2L, s.get("ses-1"))
    }

    @Test
    fun `gap rejected`() = runTest {
        val s = store()
        assertTrue(s.advanceIfContiguous("ses-1", 1L))
        assertFalse(s.advanceIfContiguous("ses-1", 5L)) // gap
        assertEquals(1L, s.get("ses-1"))
    }

    @Test
    fun `forget removes`() = runTest {
        val s = store()
        s.set("ses-1", 10L)
        s.forget("ses-1")
        assertEquals(0L, s.get("ses-1"))
    }

    @Test
    fun `hydrate loads persisted value`() = runTest {
        val persistence = InMemoryCursorPersistence()
        persistence.set("ses-1", 42L) // a previously-seen cursor from a prior run
        val s = HybridCursorStore(persistence, TestScope(StandardTestDispatcher()))
        s.hydrate("ses-1")
        assertEquals(42L, s.get("ses-1"))
    }

    @Test
    fun `persistence mirror written on advance`() = runTest {
        val persistence = InMemoryCursorPersistence()
        val scope = TestScope(StandardTestDispatcher())
        val s = HybridCursorStore(persistence, scope)
        s.advanceIfContiguous("ses-1", 1L)
        scope.advanceUntilIdle()
        assertEquals(1L, persistence.snapshot()["ses-1"])
    }
}
