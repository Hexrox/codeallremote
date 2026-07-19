package io.codeallremote.car.android.ui.home

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Refresh
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
import io.codeallremote.car.android.net.dto.SessionSnapshot
import java.text.DateFormat
import java.util.Date

/**
 * Home screen (docs/37 §Home): connectivity, active sessions, pending
 * approvals, recent activity. Handles loading/empty/stale states.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    state: HomeUiState,
    onOpenSession: (String) -> Unit,
    onPairServer: () -> Unit,
    onRetry: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.home_title)) },
                actions = {
                    IconButton(onClick = onPairServer) {
                        Icon(Icons.Default.Add, contentDescription = stringResource(R.string.home_pair_server))
                    }
                    IconButton(onClick = onRetry) {
                        Icon(Icons.Default.Refresh, contentDescription = stringResource(R.string.action_retry))
                    }
                },
            )
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                state.loading -> LoadingSkeleton()
                state.error != null -> ErrorState(state.error, onRetry)
                state.sessions.isEmpty() -> EmptyState()
                else -> SessionList(state.sessions, state.lastUpdated, onOpenSession)
            }
        }
    }
}

@Composable
private fun SessionList(
    sessions: List<SessionSnapshot>,
    lastUpdated: Long?,
    onOpen: (String) -> Unit,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize().testTag("session_list"),
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (lastUpdated != null) {
            item {
                Text(
                    stringResource(R.string.home_stale, formatTime(lastUpdated)),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.outline,
                    modifier = Modifier.semantics { contentDescription = "Stale marker" },
                )
            }
        }
        items(sessions, key = { it.id }) { s ->
            SessionCard(s) { onOpen(s.id) }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SessionCard(s: SessionSnapshot, onClick: () -> Unit) {
    Card(onClick = onClick, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                s.title ?: s.id,
                style = MaterialTheme.typography.titleMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(s.adapterId, style = MaterialTheme.typography.bodySmall)
            StateBadge(s.state)
        }
    }
}

@Composable
private fun StateBadge(state: String) {
    val (label, container) = when (state) {
        "active" -> stringResource(R.string.session_state_active) to MaterialTheme.colorScheme.primaryContainer
        "waiting_approval" -> stringResource(R.string.session_state_waiting_approval) to MaterialTheme.colorScheme.tertiaryContainer
        "completed" -> stringResource(R.string.session_state_completed) to MaterialTheme.colorScheme.surfaceVariant
        "failed" -> stringResource(R.string.session_state_failed) to MaterialTheme.colorScheme.errorContainer
        "interrupted" -> stringResource(R.string.session_state_interrupted) to MaterialTheme.colorScheme.surfaceVariant
        else -> state to MaterialTheme.colorScheme.surfaceVariant
    }
    // The badge always has a text label; color is secondary (a11y).
    AssistChip(
        onClick = {},
        label = { Text(label, modifier = Modifier.semantics { contentDescription = "State: $label" }) },
        colors = AssistChipDefaults.assistChipColors(containerColor = container),
    )
}

@Composable
private fun LoadingSkeleton() {
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        repeat(3) { SkeletonCard() }
    }
}

@Composable
private fun SkeletonCard() {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Box(Modifier.height(20.dp).fillMaxWidth(0.6f)) { Text("") }
            Box(Modifier.height(16.dp).fillMaxWidth(0.4f)) { Text("") }
        }
    }
}

@Composable
private fun EmptyState() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(
            stringResource(R.string.home_no_sessions),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(24.dp),
        )
    }
}

@Composable
private fun ErrorState(message: String, onRetry: () -> Unit) {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text("Error: $message", color = MaterialTheme.colorScheme.error)
            Spacer(Modifier.height(12.dp))
            Button(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
        }
    }
}

private fun formatTime(epochMs: Long): String =
    DateFormat.getTimeInstance(DateFormat.MEDIUM).format(Date(epochMs))

data class HomeUiState(
    val loading: Boolean = true,
    val sessions: List<SessionSnapshot> = emptyList(),
    val lastUpdated: Long? = null,
    val error: String? = null,
)
