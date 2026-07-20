package io.codeallremote.car.android.ui.pairing

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import io.codeallremote.car.android.store.DeviceKeyStore
import io.codeallremote.car.android.store.SecureTokenStore
import io.codeallremote.car.android.store.ServerAccountStore
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PairingViewModelTest {

    private fun newVm(): PairingViewModel {
        val ctx = ApplicationProvider.getApplicationContext<Context>()
        return PairingViewModel(
            ServerAccountStore(ctx),
            SecureTokenStore(ctx),
            DeviceKeyStore(),
            restFactory = { throw AssertionError("restFactory must not be called in guard tests") }
        )
    }

    @Test
    fun requestChallenge_nonHttps_setsError() {
        val vm = newVm()
        vm.onBaseUrlChange("http://insecure.example")
        vm.requestChallenge()
        assertEquals("Server URL must start with https://", vm.uiState.value.error)
    }

    @Test
    fun confirmPair_blankCode_setsError() {
        val vm = newVm()
        vm.confirmPair("   ")
        assertEquals("Pairing code is required", vm.uiState.value.error)
    }

    @Test
    fun baseUrlAndDeviceName_update() {
        val vm = newVm()
        vm.onBaseUrlChange("https://car.example.test")
        vm.onDeviceNameChange("Pixel 8")
        assertEquals("https://car.example.test", vm.baseUrl.value)
        assertEquals("Pixel 8", vm.deviceName.value)
    }
}
