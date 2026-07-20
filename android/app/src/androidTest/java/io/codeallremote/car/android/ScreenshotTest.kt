package io.codeallremote.car.android

import android.graphics.Bitmap
import android.util.Log
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.test.platform.app.InstrumentationRegistry
import io.codeallremote.car.android.net.dto.SessionSnapshot
import io.codeallremote.car.android.ui.home.HomeScreen
import io.codeallremote.car.android.ui.home.HomeUiState
import io.codeallremote.car.android.ui.theme.CarTheme
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import java.io.File
import java.io.FileOutputStream

class ScreenshotTest {

    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun captureHome() {
        val state = HomeUiState(
            loading = false,
            sessions = listOf(
                SessionSnapshot(
                    id = "ses_1",
                    workspaceId = "main",
                    adapterId = "claude-code",
                    state = "active",
                    lastSequence = 12,
                    title = "Fix auth bug",
                ),
                SessionSnapshot(
                    id = "ses_2",
                    workspaceId = "main",
                    adapterId = "claude-code",
                    state = "completed",
                    lastSequence = 34,
                    title = "Refactor stream-json parser",
                ),
                SessionSnapshot(
                    id = "ses_3",
                    workspaceId = "web",
                    adapterId = "claude-code",
                    state = "waiting_approval",
                    lastSequence = 7,
                    title = "Update dependencies",
                ),
            ),
        )

        composeRule.setContent {
            CarTheme {
                HomeScreen(
                    state = state,
                    onOpenSession = {},
                    onPairServer = {},
                    onRetry = {},
                )
            }
        }

        composeRule.waitForIdle()

        val bmp = composeRule.onRoot().captureToImage().asAndroidBitmap()

        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val f = File(ctx.filesDir, "home.png")
        FileOutputStream(f).use { bmp.compress(Bitmap.CompressFormat.PNG, 100, it) }
        Log.i("CAR_SHOT", f.absolutePath)

        assertTrue(f.exists())
    }
}
