package io.codeallremote.car.android.ui.changes

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/**
 * Changes and files screen (docs/37 §Changes and files, M2-15 acceptance).
 *
 * - Partial/stale change data is clearly labelled (never claim a diff is
 *   complete when the server reports partial/stale).
 * - Paths render as server-provided display values, not local filesystem paths.
 * - Large output remains usable on a phone with a selectable text fallback.
 */
@Composable
fun ChangesScreen(files: List<ChangedFile>, partial: Boolean, onSelectFallback: () -> Unit) {
    Column(Modifier.fillMaxSize()) {
        if (partial) {
            PartialBanner()
        }
        LazyColumn(modifier = Modifier.weight(1f).testTag("changes_list")) {
            items(files, key = { it.path }) { f ->
                FileRow(f)
            }
            item { TextFallbackLink(onSelectFallback) }
        }
    }
}

@Composable
private fun PartialBanner() {
    Surface(color = MaterialTheme.colorScheme.tertiaryContainer) {
        Text(
            "Partial / stale snapshot — diff may be incomplete",
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            style = MaterialTheme.typography.bodySmall,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun FileRow(f: ChangedFile) {
    ListItem(
        headlineContent = { Text(f.path, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        supportingContent = {
            Text("${f.operation} · +${f.additions} −${f.deletions}")
        },
        trailingContent = {
            // Color is never the sole carrier of meaning; label the status.
            Text(
                f.status,
                style = MaterialTheme.typography.labelSmall,
                modifier = Modifier.semantics { contentDescription = "File status ${f.status}" },
            )
        },
    )
    HorizontalDivider()
}

@Composable
private fun TextFallbackLink(onClick: () -> Unit) {
    TextButton(onClick = onClick, modifier = Modifier.padding(16.dp)) {
        Text("View as selectable plain text")
    }
}

data class ChangedFile(
    val path: String,            // server-provided display value
    val operation: String,       // create/modify/delete
    val additions: Int = 0,
    val deletions: Int = 0,
    val status: String,         // pending/saved/stale
)

/** Selectable plain-text diff fallback (a11y: terminal output selectable). */
@Composable
fun DiffFallback(text: String) {
    SelectionContainer {
        Text(
            text,
            modifier = Modifier.fillMaxSize().testTag("diff_fallback").padding(12.dp),
        )
    }
}
