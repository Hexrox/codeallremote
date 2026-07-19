package io.codeallremote.car.android.ui.settings

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import io.codeallremote.car.android.R

/**
 * Settings screen (docs/37 §Settings, M2-16 acceptance).
 *
 * - Biometric approval requirement, notification privacy, cache clearing.
 * - Clearing cache never revokes server authorization accidentally.
 * - No token, provider credential, or raw secret is displayed here.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    biometricRequired: Boolean,
    notificationPrivacyPrivate: Boolean,
    onBiometricChange: (Boolean) -> Unit,
    onNotificationPrivacyChange: (Boolean) -> Unit,
    onClearCache: () -> Unit,
    onBack: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Settings") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            SectionTitle("Security")
            SwitchRow(
                label = "Require biometric for approvals",
                checked = biometricRequired,
                onChange = onBiometricChange,
                description = "Require device biometric before approve/deny",
            )

            HorizontalDivider()
            SectionTitle("Privacy")
            SwitchRow(
                label = "Private notification previews",
                checked = notificationPrivacyPrivate,
                onChange = onNotificationPrivacyChange,
                description = "Hide content in lock-screen notifications",
            )

            HorizontalDivider()
            SectionTitle("Cache")
            ListItem(
                headlineContent = { Text("Clear cached transcripts and diffs") },
                supportingContent = {
                    Text(
                        "Clears local caches only. Does not revoke server access.",
                        style = MaterialTheme.typography.bodySmall,
                    )
                },
                trailingContent = {
                    OutlinedButton(onClick = onClearCache) { Text("Clear") }
                },
            )

            HorizontalDivider()
            Text(
                "Tokens and provider credentials are never shown here.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.outline,
            )
        }
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        modifier = Modifier.padding(top = 8.dp, bottom = 4.dp),
    )
}

@Composable
private fun SwitchRow(label: String, checked: Boolean, onChange: (Boolean) -> Unit, description: String) {
    ListItem(
        modifier = Modifier.semantics { contentDescription = description },
        headlineContent = { Text(label) },
        trailingContent = {
            Switch(checked = checked, onCheckedChange = onChange)
        },
    )
}
