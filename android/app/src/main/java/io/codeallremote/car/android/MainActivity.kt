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
import kotlinx.coroutines.runBlocking

/**
 * Single-activity host. Deep links are validated by the navigation graph
 * before private content is rendered (docs/18 §Navigation model). On launch it
 * also wires the background connection service (foreground WS + notifications).
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

        // Wire the background connection service: supply it a client factory and
        // start it only for a really-paired account, so events and approvals
        // arrive while the app is backgrounded. If no server is paired, the
        // service is not started — that avoids a doomed reconnect loop against
        // a placeholder host before pairing exists.
        CarConnectionService.clientProvider = { ctx, serverId -> buildCarClientFor(ctx, serverId) }

        // Resolve the active paired server asynchronously; start the foreground
        // service only if one exists.
        lifecycleScope.launch {
            val account = ServerAccountStore(applicationContext).accounts.first().firstOrNull()
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
 * Builds a HomeViewModel bound to a CarClient for the active paired server.
 *
 * The active account is the first account in ServerAccountStore (all persisted
 * accounts are paired). Tokens are read from SecureTokenStore so they never sit
 * in plain fields. When no server is paired, a throwaway repo is used and the
 * empty (no-server) state is rendered via [HomeViewModel.showNoServer].
 */
fun homeViewModelFactory(context: android.content.Context): ViewModelProvider.Factory = viewModelFactory {
    initializer<HomeViewModel> {
        val account = runBlocking { ServerAccountStore(context).accounts.first().firstOrNull() }
        val persisted = PersistedCursorStore(context)
        val cursors = HybridCursorStore(PersistedCursorPersistence(persisted))
        if (account == null) {
            // No paired server: build a throwaway repo and render the empty state.
            // refresh() is never called, so the dummy base URL is never used.
            val dummy = ServerAccount(id = "none", displayName = "none", baseUrl = "https://none.invalid", deviceId = "none", pairedAt = "")
            val tokens = SecureTokenStore(context)
            val client = CarClient(dummy, { tokens.getToken(dummy.id) }, { dummy.deviceId }, cursors)
            HomeViewModel(CarRepository(client.rest, PersistedCursorStore(context))).also { it.showNoServer() }
        } else {
            val tokens = SecureTokenStore(context)
            val client = CarClient(account, { tokens.getToken(account.id) }, { account.deviceId }, cursors)
            HomeViewModel(CarRepository(client.rest, PersistedCursorStore(context))).also { it.refresh() }
        }
    }
}
