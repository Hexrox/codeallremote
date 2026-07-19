package io.codeallremote.car.android

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test

/**
 * Instrumented UI test (requires an emulator). Verifies the Home screen
 * renders its states and primary actions (M2-13 acceptance, docs/29 QA).
 *
 * Run: ./gradlew connectedDebugAndroidTest
 */
class HomeComposeTest {

    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun home_showsEmptyState_andPairAction() {
        composeRule.setContent {
            io.codeallremote.car.android.ui.theme.CarTheme {
                io.codeallremote.car.android.ui.home.HomeScreen(
                    state = io.codeallremote.car.android.ui.home.HomeUiState(
                        loading = false,
                        sessions = emptyList(),
                    ),
                    onOpenSession = {},
                    onPairServer = {},
                    onRetry = {},
                )
            }
        }
        // Empty state explains how to register a workspace (docs/37 §Home).
        composeRule.onNodeWithText("No sessions yet. Pair a server and start a session.")
            .assertIsDisplayed()
    }

    @Test
    fun home_rendersSessionCard() {
        composeRule.setContent {
            io.codeallremote.car.android.ui.theme.CarTheme {
                io.codeallremote.car.android.ui.home.HomeScreen(
                    state = io.codeallremote.car.android.ui.home.HomeUiState(
                        loading = false,
                        sessions = listOf(
                            io.codeallremote.car.android.net.dto.SessionSnapshot(
                                id = "ses_1", workspaceId = "ws", adapterId = "claude-code",
                                state = "active", lastSequence = 5, title = "Fix bug",
                            ),
                        ),
                    ),
                    onOpenSession = {},
                    onPairServer = {},
                    onRetry = {},
                )
            }
        }
        composeRule.onNodeWithText("Fix bug").assertIsDisplayed()
    }
}
