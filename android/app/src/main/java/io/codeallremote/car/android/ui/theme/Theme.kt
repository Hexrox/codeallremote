package io.codeallremote.car.android.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.HelpOutline
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.NotificationsActive
import androidx.compose.material.icons.filled.PauseCircle
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import io.codeallremote.car.android.R

// Material 3 has no dedicated "success" role; provide one for completed states.
val SuccessContainerLight = Color(0xFFC7F0D2)
val OnSuccessContainerLight = Color(0xFF04210F)
val SuccessContainerDark = Color(0xFF20512F)
val OnSuccessContainerDark = Color(0xFFB6F0C4)

private val LightColors = lightColorScheme(
    primary = Color(0xFF3B5BDB),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFDDE1FF),
    onPrimaryContainer = Color(0xFF001257),
    secondary = Color(0xFF5A5D72),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFDFE0F9),
    onSecondaryContainer = Color(0xFF171A2C),
    tertiary = Color(0xFF855200),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFFFDEA8),
    onTertiaryContainer = Color(0xFF2B1700),
    error = Color(0xFFBA1A1A),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFFDAD6),
    onErrorContainer = Color(0xFF410002),
    background = Color(0xFFFBFAFF),
    onBackground = Color(0xFF1A1B23),
    surface = Color(0xFFFBFAFF),
    onSurface = Color(0xFF1A1B23),
    surfaceVariant = Color(0xFFE2E1EC),
    onSurfaceVariant = Color(0xFF45464F),
    outline = Color(0xFF767680),
    outlineVariant = Color(0xFFC6C5D0),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF9DB4FF),
    onPrimary = Color(0xFF002780),
    primaryContainer = Color(0xFF1B3FB1),
    onPrimaryContainer = Color(0xFFDDE1FF),
    secondary = Color(0xFFC3C4DD),
    onSecondary = Color(0xFF2C2F42),
    secondaryContainer = Color(0xFF434659),
    onSecondaryContainer = Color(0xFFDFE0F9),
    tertiary = Color(0xFFFFB95C),
    onTertiary = Color(0xFF482100),
    tertiaryContainer = Color(0xFF683E00),
    onTertiaryContainer = Color(0xFFFFDEA8),
    error = Color(0xFFFFB4AB),
    onError = Color(0xFF690005),
    errorContainer = Color(0xFF93000A),
    onErrorContainer = Color(0xFFFFDAD6),
    background = Color(0xFF121318),
    onBackground = Color(0xFFE3E1E9),
    surface = Color(0xFF121318),
    onSurface = Color(0xFFE3E1E9),
    surfaceVariant = Color(0xFF45464F),
    onSurfaceVariant = Color(0xFFC6C5D0),
    outline = Color(0xFF90909A),
    outlineVariant = Color(0xFF45464F),
)

data class SessionVisual(
    val container: Color,
    val content: Color,
    val icon: ImageVector,
    val label: String,
)

@Composable
fun sessionVisual(state: String): SessionVisual {
    val darkTheme = isSystemInDarkTheme()
    return when (state) {
        "active" -> SessionVisual(
            container = MaterialTheme.colorScheme.primaryContainer,
            content = MaterialTheme.colorScheme.onPrimaryContainer,
            icon = Icons.Filled.PlayArrow,
            label = stringResource(R.string.session_state_active),
        )
        "waiting_approval" -> SessionVisual(
            container = MaterialTheme.colorScheme.tertiaryContainer,
            content = MaterialTheme.colorScheme.onTertiaryContainer,
            icon = Icons.Filled.NotificationsActive,
            label = stringResource(R.string.session_state_waiting_approval),
        )
        "completed" -> SessionVisual(
            container = if (darkTheme) SuccessContainerDark else SuccessContainerLight,
            content = if (darkTheme) OnSuccessContainerDark else OnSuccessContainerLight,
            icon = Icons.Filled.CheckCircle,
            label = stringResource(R.string.session_state_completed),
        )
        "failed" -> SessionVisual(
            container = MaterialTheme.colorScheme.errorContainer,
            content = MaterialTheme.colorScheme.onErrorContainer,
            icon = Icons.Filled.ErrorOutline,
            label = stringResource(R.string.session_state_failed),
        )
        "interrupted" -> SessionVisual(
            container = MaterialTheme.colorScheme.surfaceVariant,
            content = MaterialTheme.colorScheme.onSurfaceVariant,
            icon = Icons.Filled.PauseCircle,
            label = stringResource(R.string.session_state_interrupted),
        )
        else -> SessionVisual(
            container = MaterialTheme.colorScheme.surfaceVariant,
            content = MaterialTheme.colorScheme.onSurfaceVariant,
            icon = Icons.AutoMirrored.Filled.HelpOutline,
            label = state,
        )
    }
}

@Composable
fun CarTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        content = content,
    )
}
