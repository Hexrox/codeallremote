package io.codeallremote.car.android.ui.session

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import io.codeallremote.car.android.data.CarRepository
import io.codeallremote.car.android.net.CarRestClient
import io.codeallremote.car.android.net.ConnectionState
import io.codeallremote.car.android.store.PersistedCursorStore
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
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
class SessionViewModelTest {

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
        val rest = CarRestClient(OkHttpClient(), server.url("/").toString(), { "test-token" })
        return CarRepository(rest, PersistedCursorStore(ApplicationProvider.getApplicationContext()))
    }

    private fun waitUntil(timeoutMs: Long = 5000, cond: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (!cond() && System.currentTimeMillis() < deadline) {
            Thread.sleep(25)
        }
        assertTrue("condition not met within timeout", cond())
    }

    private fun newVm(repo: CarRepository, sessionId: String): SessionViewModel {
        lateinit var vm: SessionViewModel
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            vm = SessionViewModel(
                sessionId,
                repo,
                MutableStateFlow(ConnectionState.Disconnected),
                MutableSharedFlow()
            )
        }
        return vm
    }

    @Test
    fun load_populatesSnapshotAndTimeline() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"ses1","workspace_id":"ws","adapter_id":"claude-code","state":"active","last_sequence":5,"title":"Fix bug"}"""
            )
        )
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"events":[{"type":"run.output","message_id":"m1","session_id":"ses1","sequence":6,"payload":{"text":"hello"}}],"next_after":6,"resync_required":false,"has_more":false}"""
            )
        )
        val repo = newRepo()
        val vm = newVm(repo, "ses1")
        waitUntil { vm.state.value.state == "active" && vm.state.value.timeline.isNotEmpty() }
        assertEquals("Fix bug", vm.state.value.title)
        assertEquals("active", vm.state.value.state)
        assertEquals("ws", vm.state.value.workspaceId)
        assertEquals(1, vm.state.value.timeline.size)
        assertEquals("run.output", vm.state.value.output.first().type)
    }

    @Test
    fun sendPrompt_clearsDraftOnSuccess() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"ses1","workspace_id":"ws","adapter_id":"claude-code","state":"active","last_sequence":5,"title":"Fix bug"}"""
            )
        )
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"events":[],"next_after":0,"resync_required":false,"has_more":false}"""
            )
        )
        server.enqueue(MockResponse().setResponseCode(200).setBody(""))
        val repo = newRepo()
        val vm = newVm(repo, "ses1")
        waitUntil { vm.state.value.state == "active" }
        // Ensure both load() requests (snapshot + events) have been consumed
        // before the prompt POST, so the POST gets the empty ack (3rd response).
        waitUntil { server.requestCount >= 2 }
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            vm.updateDraft("hello there")
            vm.sendPrompt()
        }
        waitUntil { vm.draft.value.isEmpty() && !vm.unsent.value }
        assertEquals("", vm.draft.value)
        assertTrue(!vm.unsent.value)
    }
}
