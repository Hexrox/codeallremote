# M0 review tasks

## M0-01 — Cross-document consistency review

Compare every domain term, lifecycle state, event type and endpoint across `docs/`, ADRs and tasks. Record conflicts and resolve them before M1 implementation starts.

## M0-02 — Confirm technology baseline

Review ADR-008 against team capabilities and homelab constraints. If changed, amend the ADR and update task assumptions; do not silently substitute a stack during implementation.

## M0-03 — Produce implementation manifest

Create a manifest mapping each M1 task to its input documents, expected modules, tests, migrations and required acceptance evidence.

## M0-04 — Approve M1 start

Sign off the M0→M1 gate only when the fake-adapter end-to-end scenario can be described entirely using existing contracts.

