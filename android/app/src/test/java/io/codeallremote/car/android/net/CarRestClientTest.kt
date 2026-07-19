package io.codeallremote.car.android.net

import io.codeallremote.car.android.net.dto.CreateSessionRequest
import io.codeallremote.car.android.net.dto.SessionSnapshot
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Before
import org.junit.Test

/**
 * JVM unit tests for the REST client against MockWebServer. No device, no
 * real network, no provider credentials (docs/23 §Determinism).
 */
class CarRestClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: CarRestClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = CarRestClient(
            httpClient = OkHttpClient(),
            baseUrl = server.url("/").toString().trimEnd('/'),
            tokenProvider = { "test-token" },
        )
    }

    @After
    fun tearDown() { server.shutdown() }

    @Test
    fun `createSession sends idempotency key and parses snapshot`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(201).setBody(SESSION_BODY))
        val res = client.createSession(CreateSessionRequest("ws_1", "claude-code", "T"))
        assertNotNull(res)
        require(res is CarResult.Ok)
        assertEquals("ses_01", res.value.id)
        assertEquals("created", res.value.state)

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/api/v1/sessions", req.path)
        assertNotNull(req.getHeader("Idempotency-Key"))
        assertEquals("Bearer test-token", req.getHeader("Authorization"))
    }

    @Test
    fun `getSession parses snapshot`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody(SESSION_BODY))
        val res = client.getSession("ses_01")
        require(res is CarResult.Ok)
        assertEquals("ses_01", res.value.id)
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/api/v1/sessions/ses_01", req.path)
    }

    @Test
    fun `listSessions parses array`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"sessions":[$SESSION_BODY]}"""))
        val res = client.listSessions()
        require(res is CarResult.Ok)
        assertEquals(1, res.value.sessions.size)
    }

    @Test
    fun `startRun returns 202 with body`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(202).setBody("""{"run_id":"run_01","state":"active","message":"accepted"}"""))
        val res = client.startRun("ses_01")
        require(res is CarResult.Ok)
        assertEquals("run_01", res.value.runId)
        val req = server.takeRequest()
        assertNotNull(req.getHeader("Idempotency-Key"))
    }

    @Test
    fun `submitPrompt accepts 202 with empty body`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(202).setBody(""))
        val res = client.submitPrompt("ses_01", "do the thing")
        require(res is CarResult.Ok)
    }

    @Test
    fun `error maps to errcat code`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(409).setBody(
            """{"code":"conflict","message":"State conflict.","request_id":"req_1"}"""
        ))
        val res = client.createSession(CreateSessionRequest("ws_1", "claude-code"))
        require(res is CarResult.Err)
        assertEquals(CarResult.Code.Conflict, res.code)
        assertEquals(409, res.httpStatus)
    }

    @Test
    fun `401 maps to unauthorized`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401).setBody(
            """{"code":"unauthorized","message":"Authentication required.","request_id":"req_1"}"""
        ))
        val res = client.listSessions()
        require(res is CarResult.Err)
        assertEquals(CarResult.Code.Unauthorized, res.code)
    }

    @Test
    fun `transport failure is not silent`() = runBlocking {
        server.shutdown() // force connection failure
        val res = client.getSession("ses_01")
        require(res is CarResult.Err)
        assertEquals(CarResult.Code.Transport, res.code)
    }

    @Test
    fun `getEvents parses events response`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody(EVENTS_BODY))
        val res = client.getEvents("ses_01", after = 0)
        require(res is CarResult.Ok)
        assertEquals(2, res.value.events.size)
        assertEquals(2L, res.value.nextAfter)
    }

    companion object {
        private val SESSION_BODY = """
            {"id":"ses_01","workspace_id":"ws_01","adapter_id":"claude-code","state":"created","last_sequence":1,"title":"T"}
        """.trimIndent()

        private val EVENTS_BODY = """
            {"events":[
              {"type":"session.created","message_id":"m1","session_id":"ses_01","sequence":1,"payload":{}},
              {"type":"run.started","message_id":"m2","session_id":"ses_01","sequence":2,"payload":{"run_id":"run_01"}}
            ],"next_after":2,"resync_required":false}
        """.trimIndent()
    }
}
