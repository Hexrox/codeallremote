package io.codeallremote.car.android.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Color is never the sole carrier of meaning (docs/58 §Required behavior):
// icons + content descriptions accompany each state color.

private val LightColors = lightColorScheme(
    primary = Color(0xFF2E5C8A),
    error = Color(0xFFB3261E),
    tertiary = Color(0xFF8A5A00), // warning/awaiting
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF6FA8DC),
    error = Color(0xFFF2B8B5),
    tertiary = Color(0xFFFFD180),
)

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
