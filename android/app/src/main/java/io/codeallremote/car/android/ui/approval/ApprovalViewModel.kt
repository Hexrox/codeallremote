package io.codeallremote.car.android.ui.approval

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.codeallremote.car.android.data.CarRepository
import io.codeallremote.car.android.net.CarResult
import io.codeallremote.car.android.net.dto.ApprovalResponse
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * Drives the approval detail screen (docs/37 §Approval detail, M2-06).
 *
 * - Fetches the current approval; a resolved/expired approval opens to its
 *   final state, NOT actionable controls.
 * - Approve/deny are submitted exactly once; the UI's "submitting" state is
 *   irreversible until the server responds (server is idempotent).
 * - Duplicate requests are safe: a second tap while submitting is ignored.
 */
class ApprovalViewModel(
    private val approvalId: String,
    private val repo: CarRepository,
) : ViewModel() {

    private val _approval = MutableStateFlow<ApprovalResponse?>(null)
    val approval: StateFlow<ApprovalResponse?> = _approval.asStateFlow()

    private val _loading = MutableStateFlow(true)
    val loading: StateFlow<Boolean> = _loading.asStateFlow()

    private val _submitting = MutableStateFlow(false)
    val submitting: StateFlow<Boolean> = _submitting.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    init { load() }

    fun load() {
        _loading.value = true
        viewModelScope.launch {
            when (val r = repo.getApproval(approvalId)) {
                is CarResult.Ok -> {
                    _approval.value = r.value
                    // If the approval is no longer pending, controls stay hidden
                    // and the final state is shown.
                }
                is CarResult.Err -> _error.value = r.message
            }
            _loading.value = false
        }
    }

    fun decide(decision: String, reason: String? = null) {
        // Duplicate-safe: ignore if already submitting.
        if (_submitting.value) return
        _submitting.value = true
        viewModelScope.launch {
            when (val r = repo.decideApproval(approvalId, decision, reason)) {
                is CarResult.Ok -> _approval.value = r.value
                is CarResult.Err -> {
                    // A late/duplicate decision returns the final state;
                    // reload so the UI reflects the server's truth.
                    if (r.code == CarResult.Code.ExpiredApproval ||
                        r.code == CarResult.Code.Conflict) load()
                    else _error.value = r.message
                }
            }
            _submitting.value = false
        }
    }
}
