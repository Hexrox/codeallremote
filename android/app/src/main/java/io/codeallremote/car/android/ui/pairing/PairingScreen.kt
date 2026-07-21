package io.codeallremote.car.android.ui.pairing

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardOptions
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
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.codeallremote.car.android.R

/**
 * Pairing screen (docs/15 §Pairing flow, M2-10 acceptance).
 *
 * - Single-use, short-lived challenge code entered manually or scanned.
 * - Device generates a key locally; only the public key + name go to the server.
 * - The access token returned is stored in SecureTokenStore, never in general prefs.
 * - Removing a server later does NOT delete remote sessions/data.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PairingScreen(
    state: PairingUiState,
    serverBaseUrl: String,
    deviceName: String,
    onBaseUrlChange: (String) -> Unit,
    onDeviceNameChange: (String) -> Unit,
    onRequestChallenge: () -> Unit,
    onConfirmPair: (String) -> Unit,
    onBack: () -> Unit,
) {
    var code by remember { mutableStateOf("") }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.home_pair_server)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier.fillMaxSize().padding(padding).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = serverBaseUrl,
                onValueChange = onBaseUrlChange,
                label = { Text("Server URL (http(s)://...)") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("server_url"),
            )
            OutlinedTextField(
                value = deviceName,
                onValueChange = onDeviceNameChange,
                label = { Text("Device name") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("device_name"),
            )

            Button(
                onClick = onRequestChallenge,
                enabled = serverBaseUrl.startsWith("http://") || serverBaseUrl.startsWith("https://"),
                modifier = Modifier.fillMaxWidth(),
            ) { Text("Get pairing challenge") }

            if (state.challenge != null) {
                ChallengeCard(state.challenge)
                OutlinedTextField(
                    value = code,
                    onValueChange = { code = it },
                    label = { Text("Pairing code") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii),
                    modifier = Modifier.fillMaxWidth().testTag("pairing_code"),
                )
                Button(
                    onClick = { onConfirmPair(code) },
                    enabled = code.isNotBlank() && state.phase != PairingPhase.Submitting,
                    modifier = Modifier.fillMaxWidth(),
                ) { Text("Pair device") }
            }

            if (state.error != null) {
                Text(state.error, color = MaterialTheme.colorScheme.error)
            }
            if (state.phase == PairingPhase.Paired) {
                Text(
                    "Paired. You can now open sessions.",
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.semantics { contentDescription = "Paired successfully" },
                )
            }
        }
    }
}

@Composable
private fun ChallengeCard(code: String) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            Text("Enter this code on the server's trusted admin UI:", style = MaterialTheme.typography.bodySmall)
            Spacer(Modifier.height(8.dp))
            Text(
                code,
                style = MaterialTheme.typography.titleMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.semantics { contentDescription = "Pairing code" },
            )
            Text("Expires in minutes.", style = MaterialTheme.typography.labelSmall)
        }
    }
}

enum class PairingPhase { Idle, Challenge, Submitting, Paired }

data class PairingUiState(
    val phase: PairingPhase = PairingPhase.Idle,
    val challenge: String? = null,
    val error: String? = null,
)
