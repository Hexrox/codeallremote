package io.codeallremote.car.android.ui.pairing

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.codeallremote.car.android.net.CarRestClient
import io.codeallremote.car.android.net.CarResult
import io.codeallremote.car.android.store.DeviceKeyStore
import io.codeallremote.car.android.store.SecureTokenStore
import io.codeallremote.car.android.store.ServerAccount
import io.codeallremote.car.android.store.ServerAccountStore
import io.codeallremote.car.android.store.newServerAccountId
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class PairingViewModel(
    private val accounts: ServerAccountStore,
    private val tokens: SecureTokenStore,
    private val deviceKey: DeviceKeyStore,
    private val restFactory: (String) -> CarRestClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(PairingUiState())
    val uiState: StateFlow<PairingUiState> = _uiState.asStateFlow()

    private val _baseUrl = MutableStateFlow("https://")
    val baseUrl: StateFlow<String> = _baseUrl.asStateFlow()

    private val _deviceName = MutableStateFlow("Android")
    val deviceName: StateFlow<String> = _deviceName.asStateFlow()

    fun onBaseUrlChange(v: String) {
        _baseUrl.value = v
    }

    fun onDeviceNameChange(v: String) {
        _deviceName.value = v
    }

    fun requestChallenge() {
        // http:// is accepted only for private-VPN homelab hosts allow-listed in
        // network_security_config (ADR-011); any other host stays TLS-only.
        if (!_baseUrl.value.startsWith("http://") && !_baseUrl.value.startsWith("https://")) {
            _uiState.value = _uiState.value.copy(error = "Server URL must start with http:// or https://")
            return
        }
        _uiState.value = _uiState.value.copy(error = null)
        viewModelScope.launch {
            val rest = restFactory(_baseUrl.value.trim())
            when (val r = rest.createPairChallenge()) {
                is CarResult.Ok -> _uiState.value =
                    PairingUiState(phase = PairingPhase.Challenge, challenge = r.value.code)
                is CarResult.Err -> _uiState.value = _uiState.value.copy(error = r.message)
            }
        }
    }

    fun confirmPair(code: String) {
        if (code.isBlank()) {
            _uiState.value = _uiState.value.copy(error = "Pairing code is required")
            return
        }
        _uiState.value = _uiState.value.copy(phase = PairingPhase.Submitting, error = null)
        viewModelScope.launch {
            val pub = try {
                deviceKey.publicKeyBase64()
            } catch (e: Exception) {
                _uiState.value = _uiState.value.copy(error = "Could not generate device key")
                return@launch
            }
            val rest = restFactory(_baseUrl.value.trim())
            when (val r = rest.pairDevice(code.trim(), _deviceName.value.trim(), pub)) {
                is CarResult.Ok -> {
                    val base = _baseUrl.value.trim()
                    // Replace any previous account for the same server so re-pairing
                    // does not accumulate stale accounts/tokens.
                    accounts.accounts.first().filter { it.baseUrl == base }.forEach {
                        tokens.remove(it.id)
                        accounts.remove(it.id)
                    }
                    val id = newServerAccountId()
                    val host = runCatching { java.net.URI(base).host }
                        .getOrNull() ?: base
                    accounts.add(
                        ServerAccount(
                            id = id,
                            displayName = host,
                            baseUrl = _baseUrl.value.trim(),
                            deviceId = r.value.deviceId,
                            pairedAt = java.time.Instant.now().toString(),
                        )
                    )
                    tokens.putToken(id, r.value.accessToken)
                    _uiState.value = _uiState.value.copy(phase = PairingPhase.Paired, error = null)
                }
                is CarResult.Err -> _uiState.value =
                    _uiState.value.copy(phase = PairingPhase.Challenge, error = r.message)
            }
        }
    }
}
