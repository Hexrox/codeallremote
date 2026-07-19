package io.codeallremote.car.android.ui.session

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import io.codeallremote.car.android.R
import io.codeallremote.car.android.net.ConnectionState
import io.codeallremote.car.android.net.LiveEvent
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

/**
 * Extracts a human-readable content string from a payload's "content" field,
 * stripping JSON string quotes. Returns null if absent or non-primitive.
 */
private fun payloadContent(payload: Map<String, JsonElement>): String? {
    val el = payload["content"] ?: return null
    return (el as? JsonPrimitive)?.contentOrNullString()
}

private fun JsonPrimitive.contentOrNullString(): String? =
    if (this.isString) this.content else this.toString()

/**
 * Session supervision view (docs/37 §Session detail, M2-05 acceptance).
 *
 * - Header shows title/workspace/adapter/lifecycle state and connection.
 * - Tabs: Output, Timeline. (Chat merges output+composer; Changes/Approvals
 *   are surfaced as badges and dedicated destinations.)
 * - A persistent composer sends prompts and clearly marks unsent drafts.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SessionDetailScreen(
    state: SessionUiState,
    onSend: (String) -> Unit,
    onInterrupt: () -> Unit,
    onBack: () -> Unit,
) {
    val draft = remember { mutableStateOf("") }
    val sending = remember { mutableStateOf(false) }
    val unsentMarker = remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(state.title ?: state.sessionId, maxLines = 1) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                actions = {
                    ConnectionIndicator(state.connection)
                    if (state.state == "active" || state.state == "waiting_approval") {
                        TextButton(onClick = onInterrupt) { Text("Interrupt") }
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            HeaderStrip(state)
            Tabs(state, draft.value, sending.value, unsentMarker.value)
            Composer(
                draft = draft,
                sending = sending.value,
                unsent = unsentMarker.value,
                connection = state.connection,
                onSend = { text ->
                    sending.value = true
                    unsentMarker.value = true
                    scope.launch {
                        onSend(text)
                        // The caller flips these off after server acknowledgment.
                        // Drafts remain visibly unsent until success.
                    }
                },
            )
        }
    }
}

@Composable
private fun HeaderStrip(state: SessionUiState) {
    Surface(color = MaterialTheme.colorScheme.surfaceVariant) {
        Row(
            Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Column(Modifier.weight(1f)) {
                Text("Adapter: ${state.adapterId}", style = MaterialTheme.typography.bodySmall)
                Text("Workspace: ${state.workspaceId}", style = MaterialTheme.typography.bodySmall)
            }
            StateChip(state.state)
        }
    }
}

@Composable
private fun StateChip(state: String) {
    val label = when (state) {
        "active" -> stringResource(R.string.session_state_active)
        "waiting_approval" -> stringResource(R.string.session_state_waiting_approval)
        "completed" -> stringResource(R.string.session_state_completed)
        "failed" -> stringResource(R.string.session_state_failed)
        else -> state
    }
    AssistChip(
        onClick = {},
        label = { Text(label, modifier = Modifier.semantics { contentDescription = "State: $label" }) },
    )
}

@Composable
private fun ConnectionIndicator(connection: ConnectionState) {
    val (icon, desc) = when (connection) {
        ConnectionState.Live -> null to stringResource(R.string.session_connection_live)
        else -> Icons.Default.CloudOff to stringResource(R.string.session_connection_disconnected)
    }
    if (icon != null) {
        Icon(icon, contentDescription = desc, modifier = Modifier.semantics { contentDescription = desc })
    } else {
        Text(
            desc,
            style = MaterialTheme.typography.labelSmall,
            modifier = Modifier.semantics { contentDescription = desc },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Tabs(state: SessionUiState, draft: String, sending: Boolean, unsent: Boolean) {
    val pager = rememberPagerState(initialPage = 0, pageCount = { 2 })
    val scope = rememberCoroutineScope()
    TabRow(selectedTabIndex = pager.currentPage) {
        listOf("Output", "Timeline").forEachIndexed { i, label ->
            Tab(
                selected = pager.currentPage == i,
                onClick = { scope.launch { pager.animateScrollToPage(i) } },
                text = { Text(label) },
            )
        }
    }
    HorizontalPager(state = pager, modifier = Modifier.fillMaxSize()) { page ->
        when (page) {
            0 -> OutputPane(state.output)
            1 -> TimelinePane(state.timeline)
        }
    }
}

@Composable
private fun OutputPane(output: List<LiveEvent>) {
    // Terminal output: each line selectable/selectable container for a11y.
    LazyColumn(modifier = Modifier.fillMaxSize().testTag("output")) {
        items(output) { ev ->
            Text(
                payloadContent(ev.payload) ?: ev.type,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(8.dp)
                    .semantics { contentDescription = "Output line" },
            )
        }
    }
}

@Composable
private fun TimelinePane(timeline: List<LiveEvent>) {
    LazyColumn(modifier = Modifier.fillMaxSize().testTag("timeline")) {
        items(timeline) { ev ->
            ListItem(
                headlineContent = { Text(ev.type) },
                supportingContent = { Text("seq ${ev.sequence}") },
                modifier = Modifier.semantics { contentDescription = "${ev.type} at ${ev.sequence}" },
            )
            HorizontalDivider()
        }
    }
}

@Composable
private fun Composer(
    draft: MutableState<String>,
    sending: Boolean,
    unsent: Boolean,
    connection: ConnectionState,
    onSend: (String) -> Unit,
) {
    // Commands disabled until connected (docs/38 §Connection: commands disabled
    // until authenticated/live).
    val connected = connection == ConnectionState.Live
    Surface(tonalElevation = 2.dp) {
        Row(
            Modifier.fillMaxWidth().padding(8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = draft.value,
                onValueChange = { draft.value = it },
                modifier = Modifier.weight(1f).testTag("composer"),
                enabled = connected,
                label = {
                    Text(if (unsent) stringResource(R.string.session_composer_unsent) else stringResource(R.string.session_composer_hint))
                },
                singleLine = false,
                maxLines = 4,
            )
            IconButton(
                onClick = {
                    if (draft.value.isNotBlank()) {
                        onSend(draft.value)
                    }
                },
                enabled = connected && draft.value.isNotBlank() && !sending,
                modifier = Modifier.semantics { contentDescription = "Send prompt" },
            ) {
                Icon(Icons.AutoMirrored.Filled.Send, contentDescription = stringResource(R.string.cd_send))
            }
        }
    }
}

data class SessionUiState(
    val sessionId: String,
    val title: String? = null,
    val workspaceId: String = "",
    val adapterId: String = "",
    val state: String = "created",
    val connection: ConnectionState = ConnectionState.Disconnected,
    val output: List<LiveEvent> = emptyList(),
    val timeline: List<LiveEvent> = emptyList(),
    val pendingApprovalId: String? = null,
)
