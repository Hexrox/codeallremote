# M1 demonstration script

## Setup

Use a temporary registered workspace and deterministic fake CLI. No provider credentials or public network are needed.

## Scenario

1. Start CAR with an empty database.
2. Register the temporary workspace.
3. Create a Claude adapter session.
4. Start a run and observe `run.started`.
5. Submit a prompt and observe ordered output chunks.
6. Emit an approval request from the fake CLI.
7. Read approval details and deny it.
8. Observe `approval.resolved` and run interruption/completion.
9. Disconnect the client and restart CAR.
10. Reconnect with the last event cursor and verify snapshot/replay.

## Evidence

The agent records API responses, event sequence, final session state, audit entries and test command output. A successful demo is not a substitute for the full failure-path suite, but it proves the M1 end-to-end contract.

