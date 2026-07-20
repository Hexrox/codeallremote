package io.codeallremote.car.android.ui.navigation

import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import androidx.navigation.navDeepLink
import io.codeallremote.car.android.ui.approval.ApprovalDetailScreen
import io.codeallremote.car.android.ui.changes.ChangesScreen
import io.codeallremote.car.android.ui.home.HomeScreen
import io.codeallremote.car.android.ui.home.HomeUiState
import io.codeallremote.car.android.ui.pairing.PairingScreen
import io.codeallremote.car.android.ui.session.SessionDetailScreen
import io.codeallremote.car.android.ui.session.SessionUiState
import io.codeallremote.car.android.ui.server.ServerDetailScreen
import io.codeallremote.car.android.ui.settings.SettingsScreen
import io.codeallremote.car.android.ui.workspace.WorkspaceDetailScreen

/**
 * Single root navigation graph with stable deep links (docs/18 §Navigation).
 *
 * Deep links from notifications validate the server/token/resource before
 * displaying private content (the activity does that prior to rendering).
 */
@Composable
fun CarNavHost(homeState: HomeUiState) {
    val nav = rememberNavController()
    NavHost(navController = nav, startDestination = Routes.HOME) {
        composable(Routes.HOME) {
            HomeScreen(
                state = homeState,
                onOpenSession = { id -> nav.navigate(Routes.session("local", id)) },
                onPairServer = { nav.navigate("pair") },
                onRetry = { },
            )
        }
        composable(Routes.SESSION,
            arguments = listOf(
                navArgument("serverId") { type = NavType.StringType },
                navArgument("sessionId") { type = NavType.StringType },
            ),
            deepLinks = listOf(navDeepLink { uriPattern = "${Routes.DEEP_LINK_SCHEME}://${Routes.DEEP_LINK_HOST_SESSION}/{serverId}/{sessionId}" }),
        ) { entry ->
            val sessionId = entry.arguments?.getString("sessionId").orEmpty()
            val context = LocalContext.current
            val bound = remember(sessionId) { io.codeallremote.car.android.data.ActiveServer.bind(context) }
            val repo = io.codeallremote.car.android.data.ActiveServer.repo()
            val client = io.codeallremote.car.android.data.ActiveServer.client()
            if (bound && repo != null && client != null) {
                val vm = viewModel<io.codeallremote.car.android.ui.session.SessionViewModel>(
                    key = "session_$sessionId",
                    factory = viewModelFactory {
                        initializer {
                            io.codeallremote.car.android.ui.session.SessionViewModel(sessionId, repo, client.ws.state, client.ws.events)
                        }
                    },
                )
                SessionDetailScreen(
                    state = vm.state.collectAsState().value,
                    onSend = { text -> vm.updateDraft(text); vm.sendPrompt() },
                    onInterrupt = vm::interrupt,
                    onBack = { nav.popBackStack() },
                )
            } else {
                SessionDetailScreen(
                    state = SessionUiState(sessionId = sessionId),
                    onSend = {},
                    onInterrupt = {},
                    onBack = { nav.popBackStack() },
                )
            }
        }
        composable(Routes.APPROVAL,
            arguments = listOf(
                navArgument("serverId") { type = NavType.StringType },
                navArgument("approvalId") { type = NavType.StringType },
            ),
            deepLinks = listOf(navDeepLink { uriPattern = "${Routes.DEEP_LINK_SCHEME}://${Routes.DEEP_LINK_HOST_APPROVAL}/{serverId}/{approvalId}" }),
        ) { entry ->
            val approvalId = entry.arguments?.getString("approvalId").orEmpty()
            val context = LocalContext.current
            val bound = remember(approvalId) { io.codeallremote.car.android.data.ActiveServer.bind(context) }
            val repo = io.codeallremote.car.android.data.ActiveServer.repo()
            if (bound && repo != null) {
                val vm = viewModel<io.codeallremote.car.android.ui.approval.ApprovalViewModel>(
                    key = "approval_$approvalId",
                    factory = viewModelFactory {
                        initializer { io.codeallremote.car.android.ui.approval.ApprovalViewModel(approvalId, repo) }
                    },
                )
                ApprovalDetailScreen(
                    approval = vm.approval.collectAsState().value,
                    loading = vm.loading.collectAsState().value,
                    submitting = vm.submitting.collectAsState().value,
                    onApprove = { vm.decide("approve") },
                    onDeny = { vm.decide("deny") },
                    onBack = { nav.popBackStack() },
                )
            } else {
                ApprovalDetailScreen(
                    approval = null,
                    loading = false,
                    submitting = false,
                    onApprove = {},
                    onDeny = {},
                    onBack = { nav.popBackStack() },
                )
            }
        }
        composable("workspace/{workspaceId}") { entry ->
            val wid = entry.arguments?.getString("workspaceId").orEmpty()
            WorkspaceDetailScreen(
                name = wid,
                adapters = listOf("claude-code"),
                sessions = emptyList(),
                onOpenSession = { id -> nav.navigate(Routes.session("local", id)) },
                onBack = { nav.popBackStack() },
            )
        }
        composable("workspace/{workspaceId}/changes") { entry ->
            val wid = entry.arguments?.getString("workspaceId").orEmpty()
            ChangesScreen(
                files = emptyList(),
                partial = false,
                onSelectFallback = { nav.navigate("workspace/$wid/changes/text") },
            )
        }
        composable("workspace/{workspaceId}/changes/text") {
            // Selectable plain-text fallback route.
            ChangesFallbackRoute()
        }
        composable("settings") {
            SettingsScreen(
                biometricRequired = true,
                notificationPrivacyPrivate = true,
                onBiometricChange = {},
                onNotificationPrivacyChange = {},
                onClearCache = {},
                onBack = { nav.popBackStack() },
            )
        }
        composable("pair") {
            val context = androidx.compose.ui.platform.LocalContext.current
            val vm = androidx.lifecycle.viewmodel.compose.viewModel<io.codeallremote.car.android.ui.pairing.PairingViewModel>(
                factory = androidx.lifecycle.viewmodel.viewModelFactory {
                    initializer {
                        io.codeallremote.car.android.ui.pairing.PairingViewModel(
                            accounts = io.codeallremote.car.android.store.ServerAccountStore(context.applicationContext),
                            tokens = io.codeallremote.car.android.store.SecureTokenStore(context.applicationContext),
                            deviceKey = io.codeallremote.car.android.store.DeviceKeyStore(),
                            restFactory = { url ->
                                io.codeallremote.car.android.net.CarRestClient(
                                    io.codeallremote.car.android.data.CarClient.defaultHttpClient(), url, { null },
                                )
                            },
                        )
                    }
                },
            )
            val ui = vm.uiState.collectAsState()
            val url = vm.baseUrl.collectAsState()
            val name = vm.deviceName.collectAsState()
            PairingScreen(
                state = ui.value,
                serverBaseUrl = url.value,
                deviceName = name.value,
                onBaseUrlChange = vm::onBaseUrlChange,
                onDeviceNameChange = vm::onDeviceNameChange,
                onRequestChallenge = vm::requestChallenge,
                onConfirmPair = vm::confirmPair,
                onBack = { nav.popBackStack() },
            )
        }
        composable("server") {
            ServerDetailScreen(
                account = io.codeallremote.car.android.store.ServerAccount(
                    id = "local", displayName = "CAR", baseUrl = "https://", deviceId = "dev", pairedAt = "",
                ),
                connectionLive = false,
                onRemoveServer = {},
                onRevokeDevice = {},
                onBack = { nav.popBackStack() },
            )
        }
    }
}

@Composable
private fun ChangesFallbackRoute() {
    io.codeallremote.car.android.ui.changes.DiffFallback(text = "Selectable diff output (placeholder).")
}
