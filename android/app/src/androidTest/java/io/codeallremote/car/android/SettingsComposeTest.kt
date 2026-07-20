package io.codeallremote.car.android

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/**
 * Instrumented UI test (requires an emulator). Verifies the Settings screen
 * renders its sections and the Clear action invokes its callback.
 */
class SettingsComposeTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun settings_rendersSections() {
        composeRule.setContent {
            io.codeallremote.car.android.ui.theme.CarTheme {
                io.codeallremote.car.android.ui.settings.SettingsScreen(
                    biometricRequired = true,
                    notificationPrivacyPrivate = false,
                    onBiometricChange = {},
                    onNotificationPrivacyChange = {},
                    onClearCache = {},
                    onBack = {},
                )
            }
        }
        composeRule.onNodeWithText("Settings").assertIsDisplayed()
        composeRule.onNodeWithText("Require biometric for approvals").assertIsDisplayed()
        composeRule.onNodeWithText("Private notification previews").assertIsDisplayed()
        composeRule.onNodeWithText("Clear cached transcripts and diffs").assertIsDisplayed()
        composeRule.onNodeWithText("Tokens and provider credentials are never shown here.").assertIsDisplayed()
    }

    @Test
    fun settings_clearInvokesCallback() {
        var cleared = false
        composeRule.setContent {
            io.codeallremote.car.android.ui.theme.CarTheme {
                io.codeallremote.car.android.ui.settings.SettingsScreen(
                    biometricRequired = true,
                    notificationPrivacyPrivate = false,
                    onBiometricChange = {},
                    onNotificationPrivacyChange = {},
                    onClearCache = { cleared = true },
                    onBack = {},
                )
            }
        }
        composeRule.onNodeWithText("Clear").performClick()
        assertTrue(cleared)
    }
}
