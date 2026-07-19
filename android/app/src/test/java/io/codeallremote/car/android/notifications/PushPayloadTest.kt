package io.codeallremote.car.android.notifications

import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * JVM tests for the push payload (M2-19 notification privacy review).
 *
 * The payload must contain ONLY identifiers and a generic category — no
 * command text, filenames, transcripts, or credentials.
 */
class PushPayloadTest {

    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    @Test
    fun `payload round-trips with identifiers only`() {
        val p = PushPayload(
            serverId = "srv_1",
            resourceId = "apr_1",
            category = PushCategory.approval_requested,
            nonce = "n1",
        )
        val s = json.encodeToString(PushPayload.serializer(), p)
        // Assert no sensitive content keys exist.
        listOf("text", "command", "filename", "diff", "token", "secret").forEach {
            assertTrue("payload must not contain $it", !s.contains("\"$it\""))
        }
        val decoded = json.decodeFromString(PushPayload.serializer(), s)
        assertEquals(p, decoded)
    }

    @Test
    fun `generic title and body carry no resource content`() {
        val secretResourceId = "SECRET_RES_123"
        val p = PushPayload("s", secretResourceId, PushCategory.approval_requested, "n")
        val title = p.genericTitle()
        val body = p.genericBody()
        assertNotEquals("", title)
        // Body must not leak the actual resource id.
        assertTrue("body leaks resource id", !body.contains(secretResourceId))
        assertTrue("title leaks resource id", !title.contains(secretResourceId))
    }

    @Test
    fun `each category has a generic message`() {
        PushCategory.values().forEach { cat ->
            val p = PushPayload("s", "res", cat, "n")
            assertTrue(p.genericTitle().isNotBlank())
            assertTrue(p.genericBody().isNotBlank())
        }
    }

    @Test
    fun `additive unknown fields are ignored`() {
        val raw = """{"serverId":"s","resourceId":"r","category":"approval_requested","nonce":"n","futureField":42}"""
        val decoded = json.decodeFromString(PushPayload.serializer(), raw)
        assertEquals("s", decoded.serverId)
        assertEquals(PushCategory.approval_requested, decoded.category)
    }
}
