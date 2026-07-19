package io.codeallremote.car.android.ui.session

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.codeallremote.car.android.data.CarRepository
import io.codeallremote.car.android.net.CarResult
import io.codeallremote.car.android.net.ConnectionState
import io.codeallremote.car.android.net.LiveEvent
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch

/**
 * Drives the session supervision screen (docs/37 §Session detail, M2-05).
 *
 * - Derives the projection from REST snapshot + live events.
 * - Persists the prompt draft (device-local, never sent until user presses
 *   send) and marks it unsent until the server acknowledges.
 * - On reconnect, replays via REST to fill gaps before live delivery resumes.
 */
class SessionViewModel(
    private val sessionId: String,
    private val repo: CarRepository,
    private val connection: StateFlow<ConnectionState>,
    private val liveEvents: SharedFlow<LiveEvent>,
) : ViewModel() {

    private val _draft = MutableStateFlow("")
    val draft: StateFlow<String> = _draft.asStateFlow()

    private val _sending = MutableStateFlow(false)
    val sending: StateFlow<Boolean> = _sending.asStateFlow()

    private val _unsent = MutableStateFlow(false)
    val unsent: StateFlow<Boolean> = _unsent.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    init {
        // Re-derive state from the repository projection.
        viewModelScope.launch {
            repo.sessions.collect { snaps ->
                val s = snaps[sessionId] ?: return@collect
                projection.value = projection.value.copy(
                    title = s.title,
                    workspaceId = s.workspaceId,
                    adapterId = s.adapterId,
                    state = s.state,
                    pendingApprovalId = s.pendingApprovalId,
                    sessionId = sessionId,
                )
            }
        }
        viewModelScope.launch {
            connection.collect { projection.value = projection.value.copy(connection = it) }
        }
        // Apply live events to timeline/output and projection.
        viewModelScope.launch {
            liveEvents.collect { ev ->
                if (ev.sessionId != sessionId) return@collect
                repo.applyLiveEvent(ev)
                val tl = repo.timeline.value[sessionId].orEmpty()
                projection.value = projection.value.copy(
                    timeline = tl,
                    output = tl.filter { it.type == "run.output" || it.type == "run.prompt" },
                )
            }
        }
        load()
    }

    private val projection = MutableStateFlow(
        SessionUiState(sessionId = sessionId, connection = ConnectionState.Disconnected)
    )
    val state: StateFlow<SessionUiState> = projection.asStateFlow()

    fun load() {
        viewModelScope.launch {
            when (val r = repo.loadSession(sessionId)) {
                is CarResult.Ok -> {
                    val s = r.value
                    projection.value = projection.value.copy(
                        title = s.title, workspaceId = s.workspaceId,
                        adapterId = s.adapterId, state = s.state,
                        pendingApprovalId = s.pendingApprovalId,
                    )
                    // Seed timeline from a replay (fills any gap before live).
                    val replay = repo.replay(sessionId, 0)
                    if (replay is CarResult.Ok) {
                        val tl = repo.timeline.value[sessionId].orEmpty()
                        projection.value = projection.value.copy(
                            timeline = tl,
                            output = tl.filter { it.type == "run.output" || it.type == "run.prompt" },
                        )
                    }
                }
                is CarResult.Err -> _error.value = r.message
            }
        }
    }

    fun updateDraft(text: String) { _draft.value = text }

    fun sendPrompt() {
        val text = _draft.value
        if (text.isBlank()) return
        _sending.value = true
        _unsent.value = true
        viewModelScope.launch {
            when (repo.submitPrompt(sessionId, text)) {
                is CarResult.Ok -> {
                    // Server acknowledged: clear the draft, mark sent.
                    _draft.value = ""
                    _unsent.value = false
                }
                is CarResult.Err -> {
                    // Draft remains visibly unsent (docs/18 §Prompt drafts).
                    // Keep the text so the user can retry.
                }
            }
            _sending.value = false
        }
    }

    fun interrupt() {
        viewModelScope.launch { repo.interrupt(sessionId) }
    }

    /** Called after a resync signal to fetch a fresh snapshot + replay. */
    fun resync() {
        viewModelScope.launch { repo.replay(sessionId, 0) }
    }
}
