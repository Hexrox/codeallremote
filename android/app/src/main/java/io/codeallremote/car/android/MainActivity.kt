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
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
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
import io.codeallremote.car.android.ui.home.HomeViewModel
import io.codeallremote.car.android.ui.navigation.CarNavHost
import io.codeallremote.car.android.ui.theme.CarTheme

/**
 * Single-activity host. Deep links are validated by the navigation graph
 * before private content is rendered (docs/18 §Navigation model). On launch it
 * also wires the background connection service (foreground WS + notifications).
 */
class MainActivity : ComponentActivity() {

    private val homeViewModel: HomeViewModel by viewModels {
        homeViewModelFactory(applicationContext, localServerAccount())
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
        // arrive while the app is backgrounded. The unpaired local placeholder
        // (empty pairedAt) does not start a service — that avoids a doomed
        // reconnect loop against a placeholder host before pairing exists.
        CarConnectionService.clientProvider = { ctx, serverId -> buildCarClientFor(ctx, serverId) }
        val account = localServerAccount()
        if (account.pairedAt.isNotEmpty()) {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
            ) {
                notifPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
            ContextCompat.startForegroundService(
                this,
                Intent(this, CarConnectionService::class.java).putExtra(CarConnectionService.EXTRA_SERVER_ID, account.id)
            )
        }

        setContent {
            CarTheme {
                val state = homeViewModel.state.collectAsState()
                CarNavHost(homeState = state.value)
            }
        }
    }

    // In production this reads the chosen server from the server-account store;
    // for the initial wiring we surface a deterministic local placeholder so the
    // UI is exercised before a real pairing completes.
    private fun localServerAccount(): ServerAccount = ServerAccount(
        id = "local",
        displayName = "CAR (local)",
        baseUrl = "https://car.example.invalid",
        deviceId = "android",
        pairedAt = "",
    )
}

/**
 * Builds a HomeViewModel bound to a CarClient for the given server account.
 *
 * Tokens are read from SecureTokenStore so they never sit in plain fields.
 */
fun homeViewModelFactory(
    context: android.content.Context,
    account: ServerAccount,
): ViewModelProvider.Factory = viewModelFactory {
    initializer<HomeViewModel> {
        val tokens = SecureTokenStore(context)
        // Persistent cursor store: in-memory hot path mirrored to DataStore.
        val persisted = PersistedCursorStore(context)
        val cursors = HybridCursorStore(PersistedCursorPersistence(persisted))
        val client = CarClient(
            account = account,
            tokenProvider = { tokens.getToken(account.id) },
            deviceIdProvider = { account.deviceId },
            cursorStore = cursors,
        )
        val repo = CarRepository(
            rest = client.rest,
            cursorStore = PersistedCursorStore(context),
        )
        // Begin WS connection only while the user is actively viewing; the
        // client itself manages backoff/reconnect (docs/19 §Battery).
        HomeViewModel(repo).also { it.refresh() }
    }
}

// Suppress unused-parameter lint for context/account in the initializer.
@Suppress("UNUSED_PARAMETER")
fun unusedRef(context: android.content.Context, account: ServerAccount): ViewModel? = null
