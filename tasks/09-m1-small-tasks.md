# M1 decomposed tasks

## M1-08 — Workspace registration policy

Define canonical path validation, allowed adapters and workspace display metadata. Add tests for nonexistent paths, symlinks escaping the root and duplicate workspace IDs.

## M1-09 — Session snapshot projection

Build a read model that returns lifecycle state, active run, pending approval and last sequence in one transaction. Verify it remains correct after restart.

## M1-10 — Event cursor repository

Implement replay by `(session_id, after_sequence)` with stable ordering and a clear retention boundary. Test a missing/expired cursor and concurrent appends.

## M1-11 — Audit writer

Record actor, device, action, target, outcome and timestamp for prompts, interrupts, approval decisions, pairing and revocation. Redact sensitive context before persistence.

## M1-12 — Graceful shutdown reconciliation

On shutdown, stop accepting new runs, request adapter drains, persist outcomes and mark unresolved processes for recovery on next startup.

