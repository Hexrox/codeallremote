package io.codeallremote.car.android.ui.home

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.codeallremote.car.android.data.CarRepository
import io.codeallremote.car.android.net.CarResult
import io.codeallremote.car.android.net.LiveEvent
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * Drives the Home screen: loads sessions, reflects live state-changed / run
 * events into the derived projection (docs/39 §Home sessions).
 *
 * State is derived from the repository's server-authoritative snapshot; the
 * ViewModel holds no duplicated business logic.
 */
class HomeViewModel(private val repo: CarRepository) : ViewModel() {

    private val _state = MutableStateFlow(HomeUiState(loading = true))
    val state: StateFlow<HomeUiState> = _state.asStateFlow()

    init {
        // Reflect live events into the projection.
        viewModelScope.launch {
            repo.timeline.collect { _ -> /* sessions projection already shared */ }
        }
    }

    fun refresh() {
        _state.value = _state.value.copy(loading = true, error = null)
        viewModelScope.launch {
            when (val r = repo.refreshSessions()) {
                is CarResult.Ok -> {
                    val list = repo.sessions.value.values.sortedByDescending { it.lastSequence }
                    _state.value = HomeUiState(
                        loading = false,
                        sessions = list,
                        lastUpdated = System.currentTimeMillis(),
                    )
                }
                is CarResult.Err -> _state.value = _state.value.copy(
                    loading = false,
                    error = r.message,
                )
            }
        }
    }

    /** Surface a live event on the home projection (e.g. run completed). */
    fun onLiveEvent(ev: LiveEvent) {
        viewModelScope.launch { repo.applyLiveEvent(ev) }
    }

    /**
     * Render the empty (no paired server) state instead of a loading skeleton.
     * Used at startup when no server account exists yet, so Home shows the
     * "pair a server" call to action rather than a doomed request.
     */
    fun showNoServer() {
        _state.value = HomeUiState(loading = false, sessions = emptyList())
    }
}
