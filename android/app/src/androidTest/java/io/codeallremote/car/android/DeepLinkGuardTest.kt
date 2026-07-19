package io.codeallremote.car.android

import android.content.Intent
import android.net.Uri
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Instrumented test for deep-link validation (requires an emulator/Robolectric).
 * Deep links must resolve only when scheme/host/segments are well-formed
 * (docs/18 §Navigation model).
 */
@RunWith(AndroidJUnit4::class)
class DeepLinkGuardTest {

    @Test
    fun approval_deep_link_parses() {
        val intent = Intent().setData(Uri.parse("car://approval/srv1/apr1"))
        val t = DeepLinkGuard.validate(intent)
        assertEquals("srv1", t?.serverId)
        assertEquals("approval", t?.kind)
        assertEquals("apr1", t?.resourceId)
    }

    @Test
    fun session_deep_link_parses() {
        val intent = Intent().setData(Uri.parse("car://session/srv1/ses1"))
        val t = DeepLinkGuard.validate(intent)
        assertEquals("session", t?.kind)
        assertEquals("ses1", t?.resourceId)
    }

    @Test
    fun non_car_scheme_rejected() {
        val intent = Intent().setData(Uri.parse("https://example.com/session/srv1/ses1"))
        assertNull(DeepLinkGuard.validate(intent))
    }

    @Test
    fun missing_resource_rejected() {
        val intent = Intent().setData(Uri.parse("car://session/srv1"))
        assertNull(DeepLinkGuard.validate(intent))
    }
}
