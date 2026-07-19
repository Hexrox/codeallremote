package io.codeallremote.car.android.ui.server

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.codeallremote.car.android.R
import io.codeallremote.car.android.store.ServerAccount

/**
 * Server detail (docs/37 §Server detail, M2-13 acceptance).
 *
 * Shows name, version, connection status, paired-device identity, workspace
 * list. Destructive actions (remove server, revoke device) require
 * confirmation and NEVER delete homelab data.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServerDetailScreen(
    account: ServerAccount,
    connectionLive: Boolean,
    onRemoveServer: () -> Unit,
    onRevokeDevice: () -> Unit,
    onBack: () -> Unit,
) {
    var showRemove = remember { androidx.compose.runtime.mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(account.displayName, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            ListItem(
                headlineContent = { Text("Connection") },
                supportingContent = {
                    Text(if (connectionLive) "Live" else "Disconnected")
                },
                modifier = Modifier.semantics { contentDescription = "Connection status" },
            )
            ListItem(
                headlineContent = { Text("Device") },
                supportingContent = { Text(account.deviceId) },
                modifier = Modifier.testTag("device_identity"),
            )
            ListItem(
                headlineContent = { Text("Server URL") },
                supportingContent = { Text(account.baseUrl) },
            )

            Spacer(Modifier.height(8.dp))
            // Destructive: confirmation required; never deletes homelab data.
            OutlinedButton(
                onClick = { showRemove.value = true },
                modifier = Modifier.fillMaxWidth(),
            ) { Text("Remove server (local only)") }

            if (showRemove.value) {
                AlertDialog(
                    onDismissRequest = { showRemove.value = false },
                    title = { Text("Remove server?") },
                    text = { Text("This forgets the local account and token. Remote sessions and data are NOT deleted.") },
                    confirmButton = {
                        TextButton(onClick = { showRemove.value = false; onRemoveServer() }) { Text("Remove") }
                    },
                    dismissButton = {
                        TextButton(onClick = { showRemove.value = false }) { Text(stringResource(R.string.action_cancel)) }
                    },
                )
            }
        }
    }
}
