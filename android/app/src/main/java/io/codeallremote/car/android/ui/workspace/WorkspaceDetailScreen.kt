package io.codeallremote.car.android.ui.workspace

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.codeallremote.car.android.R
import io.codeallremote.car.android.net.dto.SessionSnapshot

/**
 * Workspace detail (docs/37 §Workspace detail, M2-14 acceptance).
 *
 * Shows workspace display name, health, adapter capabilities, recent sessions,
 * changed-file summaries. The client does NOT expose arbitrary filesystem
 * browsing; file views are scoped to server-approved workspace results.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun WorkspaceDetailScreen(
    name: String,
    adapters: List<String>,
    sessions: List<SessionSnapshot>,
    onOpenSession: (String) -> Unit,
    onBack: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            AdapterChips(adapters)
            if (sessions.isEmpty()) {
                EmptySessions()
            } else {
                LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(sessions, key = { it.id }) { s ->
                        Card(onClick = { onOpenSession(s.id) }, modifier = Modifier.fillMaxWidth()) {
                            Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                Text(s.title ?: s.id, style = MaterialTheme.typography.titleMedium)
                                Text("State: ${s.state}", style = MaterialTheme.typography.bodySmall)
                            }
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AdapterChips(adapters: List<String>) {
    FlowRow(
        modifier = Modifier.fillMaxWidth().padding(12.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        adapters.forEach { a ->
            AssistChip(
                onClick = {},
                label = {
                    Text(a, modifier = Modifier.semantics { contentDescription = "Adapter $a" })
                },
            )
        }
    }
}

@Composable
private fun EmptySessions() {
    Box(Modifier.fillMaxSize(), contentAlignment = androidx.compose.ui.Alignment.Center) {
        Text(
            "No sessions yet for this workspace.",
            style = MaterialTheme.typography.bodyMedium,
        )
    }
}
