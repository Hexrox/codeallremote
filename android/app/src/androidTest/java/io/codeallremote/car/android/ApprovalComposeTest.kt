package io.codeallremote.car.android

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import io.codeallremote.car.android.net.dto.ApprovalResponse
import org.junit.Rule
import org.junit.Test

/**
 * Instrumented UI test (requires an emulator). Verifies the Approval detail
 * screen renders a pending approval with its actions, and the empty state.
 */
class ApprovalComposeTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun approval_pending_showsCategoryAndActions() {
        composeRule.setContent {
            io.codeallremote.car.android.ui.theme.CarTheme {
                io.codeallremote.car.android.ui.approval.ApprovalDetailScreen(
                    approval = ApprovalResponse(
                        id = "ap1",
                        sessionId = "s1",
                        state = "pending",
                        category = "file_write",
                        expiresAt = "2026-07-20T00:00:00Z",
                    ),
                    loading = false,
                    submitting = false,
                    onApprove = {},
                    onDeny = {},
                    onBack = {},
                )
            }
        }
        composeRule.onNodeWithText("Approval required").assertIsDisplayed()
        composeRule.onNodeWithText("file_write").assertIsDisplayed()
        composeRule.onNodeWithText("Approve").assertIsDisplayed()
        composeRule.onNodeWithText("Deny").assertIsDisplayed()
    }

    @Test
    fun approval_null_showsTitle() {
        composeRule.setContent {
            io.codeallremote.car.android.ui.theme.CarTheme {
                io.codeallremote.car.android.ui.approval.ApprovalDetailScreen(
                    approval = null,
                    loading = false,
                    submitting = false,
                    onApprove = {},
                    onDeny = {},
                    onBack = {},
                )
            }
        }
        composeRule.onNodeWithText("Approval required").assertIsDisplayed()
    }
}
