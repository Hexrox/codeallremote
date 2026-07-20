package io.codeallremote.car.android.ui.approval

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import io.codeallremote.car.android.data.CarRepository
import io.codeallremote.car.android.net.CarRestClient
import io.codeallremote.car.android.store.PersistedCursorStore
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ApprovalViewModelTest {

    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun newRepo(): CarRepository {
        val rest = CarRestClient(
            OkHttpClient(),
            server.url("/").toString(),
            { "test-token" }
        )
        return CarRepository(rest, PersistedCursorStore(ApplicationProvider.getApplicationContext()))
    }

    private fun waitUntil(timeoutMs: Long = 5000, cond: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (!cond() && System.currentTimeMillis() < deadline) {
            Thread.sleep(25)
        }
        assertTrue("condition not met within timeout", cond())
    }

    @Test
    fun load_fetchesPendingApproval() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"ap1","session_id":"s1","state":"pending","category":"file_write","expires_at":"2026-07-20T00:00:00Z"}"""
            )
        )
        // Build the repo off the main thread: MockWebServer.url() does a reverse
        // DNS lookup (getCanonicalHostName), which StrictMode forbids on main.
        val repo = newRepo()
        lateinit var vm: ApprovalViewModel
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            vm = ApprovalViewModel("ap1", repo)
        }
        waitUntil { !vm.loading.value && vm.approval.value != null }
        assertEquals("pending", vm.approval.value?.state)
        assertEquals("file_write", vm.approval.value?.category)
    }

    @Test
    fun decide_approve_updatesState() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"ap1","session_id":"s1","state":"pending","category":"file_write","expires_at":"2026-07-20T00:00:00Z"}"""
            )
        )
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"ap1","session_id":"s1","state":"approved","category":"file_write","expires_at":"2026-07-20T00:00:00Z"}"""
            )
        )
        // Build the repo off the main thread: MockWebServer.url() does a reverse
        // DNS lookup (getCanonicalHostName), which StrictMode forbids on main.
        val repo = newRepo()
        lateinit var vm: ApprovalViewModel
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            vm = ApprovalViewModel("ap1", repo)
        }
        waitUntil { !vm.loading.value && vm.approval.value != null }
        InstrumentationRegistry.getInstrumentation().runOnMainSync { vm.decide("approve") }
        waitUntil { vm.approval.value?.state == "approved" }
        assertEquals("approved", vm.approval.value?.state)
    }
}
