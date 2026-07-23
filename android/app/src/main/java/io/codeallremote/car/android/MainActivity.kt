package io.codeallremote.car.android

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.runtime.collectAsState
import androidx.core.content.ContextCompat
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import io.codeallremote.car.android.data.ActiveServer
import io.codeallremote.car.android.data.CarClient
import io.codeallremote.car.android.data.CarRepository
import io.codeallremote.car.android.notifications.CarConnectionService
import io.codeallremote.car.android.store.HybridCursorStore
import io.codeallremote.car.android.store.PersistedCursorPersistence
import io.codeallremote.car.android.store.PersistedCursorStore
import io.codeallremote.car.android.store.SecureTokenStore
import io.codeallremote.car.android.store.ServerAccount
import io.codeallremote.car.android.store.ServerAccountStore
import io.codeallremote.car.android.ui.home.HomeViewModel
import io.codeallremote.car.android.ui.navigation.CarNavHost
import io.codeallremote.car.android.ui.theme.CarTheme
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * Single-activity host. Deep links are validated by the navigation graph
 * before private content is rendered (docs/18 §Navigation model). On launch it
 * also starts the background connection service (foreground WS + notifications).
 */
class MainActivity : ComponentActivity() {

    private val homeViewModel: HomeViewModel by viewModels {
        homeViewModelFactory(applicationContext)
    }

    private val notifPermissionLauncher =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { /* granted or not; nothing else to do */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Validate the incoming deep link (car:// session/approval) against the
        // server/token before rendering private content; the NavHost handles
        // authorization in addition to this gate.
        DeepLinkGuard.validate(intent)

        // Resolve the active paired server asynchronously; start the foreground
        // service only if one exists. Both the service and Home share the single
        // CarClient/WebSocket owned by ActiveServer.
        lifecycleScope.launch {
            val account = ServerAccountStore(applicationContext).accounts.first().maxByOrNull { it.pairedAt }
            if (account != null && account.pairedAt.isNotEmpty()) {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                    ContextCompat.checkSelfPermission(this@MainActivity, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
                ) {
                    notifPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
                }
                ContextCompat.startForegroundService(
                    this@MainActivity,
                    Intent(this@MainActivity, CarConnectionService::class.java)
                        .putExtra(CarConnectionService.EXTRA_SERVER_ID, account.id),
                )
            }
        }

        setContent {
            CarTheme {
                CarNavHost(homeState = homeViewModel.state.collectAsState().value)
            }
        }
    }
}

/**
 * Builds a HomeViewModel bound to the shared CarRepository from ActiveServer.
 *
 * ActiveServer.bind() starts the shared WebSocket once and returns the shared
 * CarClient/CarRepository. When no server is paired, a throwaway repo is used
 * and the empty (no-server) state is rendered via [HomeViewModel.showNoServer].
 */
fun homeViewModelFactory(context: android.content.Context): ViewModelProvider.Factory = viewModelFactory {
    initializer<HomeViewModel> {
        val bound = ActiveServer.bind(context)
        val repo = ActiveServer.repo()
        if (!bound || repo == null) {
            // No paired server: build a throwaway repo and render the empty state.
            // refresh() is never called, so the dummy base URL is never used.
            val dummy = ServerAccount(id = "none", displayName = "none", baseUrl = "https://none.invalid", deviceId = "none", pairedAt = "")
            val tokens = SecureTokenStore(context)
            val cursors = HybridCursorStore(PersistedCursorPersistence(PersistedCursorStore(context)))
            val client = CarClient(dummy, { tokens.getToken(dummy.id) }, { dummy.deviceId }, cursors)
            HomeViewModel(CarRepository(client.rest, PersistedCursorStore(context))).also { it.showNoServer() }
        } else {
            HomeViewModel(repo).also { it.refresh() }
        }
    }
}
