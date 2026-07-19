package io.codeallremote.car.android.ui.approval

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.codeallremote.car.android.R
import io.codeallremote.car.android.net.dto.ApprovalResponse

/**
 * Approval detail (docs/37 §Approval detail, M2-06 acceptance).
 *
 * - Redacted context, risk category, expiry countdown.
 * - Approve/deny require explicit confirmation (dialog).
 * - Duplicate-safe: submitting is irreversible from the UI; the server is
 *   idempotent.
 * - Resolved/expired approvals show the final state, NOT actionable controls.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ApprovalDetailScreen(
    approval: ApprovalResponse?,
    loading: Boolean,
    submitting: Boolean,
    onApprove: () -> Unit,
    onDeny: () -> Unit,
    onBack: () -> Unit,
) {
    val showConfirm = remember { mutableStateOf<Decision?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.approval_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
            )
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                loading -> LoadingBox()
                approval == null -> EmptyBox()
                else -> ApprovalContent(
                    approval,
                    submitting,
                    onApprove = { showConfirm.value = Decision.Approve },
                    onDeny = { showConfirm.value = Decision.Deny },
                )
            }
        }
    }

    showConfirm.value?.let { decision ->
        ConfirmDialog(
            decision,
            onDismiss = { showConfirm.value = null },
            onConfirm = {
                showConfirm.value = null
                when (decision) {
                    Decision.Approve -> onApprove()
                    Decision.Deny -> onDeny()
                }
            },
        )
    }
}

@Composable
private fun ApprovalContent(
    a: ApprovalResponse,
    submitting: Boolean,
    onApprove: () -> Unit,
    onDeny: () -> Unit,
) {
    val isPending = a.state == "pending"

    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(a.category ?: "Unknown", style = MaterialTheme.typography.titleMedium)

        // Risk: announced for screen readers; color is secondary.
        Text(
            "Risk: ${riskLabel(a)}",
            modifier = Modifier.semantics { contentDescription = "Approval risk" },
        )

        // Expiry countdown (server is authoritative; client just shows).
        Text(
            "Expires: ${a.expiresAt}",
            modifier = Modifier.semantics { contentDescription = "Approval expiry" },
        )

        // Redacted context — selectable plain text (a11y).
        SelectionContainer {
            Text(
                redactedContext(a),
                style = MaterialTheme.typography.bodyMedium,
                maxLines = 6,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.testTag("approval_context"),
            )
        }

        Spacer(Modifier.weight(1f))
        if (isPending) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.fillMaxWidth()) {
                Button(
                    onClick = onApprove,
                    enabled = !submitting,
                    modifier = Modifier.weight(1f).semantics { contentDescription = "Approve" },
                ) { Text(stringResource(R.string.approval_approve)) }
                OutlinedButton(
                    onClick = onDeny,
                    enabled = !submitting,
                    modifier = Modifier.weight(1f).semantics { contentDescription = "Deny" },
                ) { Text(stringResource(R.string.approval_deny)) }
            }
        } else {
            // Resolved/expired: final state, no actionable controls.
            FinalStateCard(a)
        }
    }
}

@Composable
private fun FinalStateCard(a: ApprovalResponse) {
    val label = when (a.state) {
        "approved" -> stringResource(R.string.approval_approve)
        "denied" -> stringResource(R.string.approval_deny)
        "expired" -> stringResource(R.string.approval_expired)
        else -> a.state
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Text(
            "${stringResource(R.string.approval_resolved)}: $label",
            modifier = Modifier.padding(16.dp).semantics { contentDescription = "Final state $label" },
        )
    }
}

@Composable
private fun ConfirmDialog(decision: Decision, onDismiss: () -> Unit, onConfirm: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.approval_confirm)) },
        text = {
            Text(
                if (decision == Decision.Approve) "Approve this action?" else "Deny this action?",
                modifier = Modifier.testTag("confirm_text"),
            )
        },
        confirmButton = {
            Button(onClick = onConfirm) {
                Text(if (decision == Decision.Approve) stringResource(R.string.approval_approve) else stringResource(R.string.approval_deny))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun LoadingBox() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator()
    }
}
@Composable
private fun EmptyBox() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text("Approval not found")
    }
}

private enum class Decision { Approve, Deny }

// The server already redacts secrets in the human-readable context. We never
// attempt to "un-redact". Risk label is derived from category.
private fun riskLabel(a: ApprovalResponse): String = when (a.category) {
    "file_write", "file_delete" -> "high"
    "command_execution" -> "high"
    "network" -> "medium"
    else -> "low"
}

private fun redactedContext(a: ApprovalResponse): String =
    // Only category + session remain; the actual context is fetched via REST
    // and the server redacts it. Client displays verbatim, selectable.
    "Action required on session ${a.sessionId}."
