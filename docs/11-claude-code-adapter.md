# Claude Code adapter

## Responsibility

The Claude Code adapter translates CAR commands and process output into the CAR domain model. It is the only component permitted to understand Claude Code's launch arguments, terminal conventions and compatibility quirks.

## Contract

```text
validateWorkspace(workspace) -> ValidationResult
start(session, input) -> RunHandle
submitInput(run, prompt) -> Accepted
interrupt(run) -> Accepted
observe(run) -> AsyncStream<AdapterSignal>
decideApproval(run, decision) -> Accepted
recover(session) -> RecoveryResult
capabilities() -> CapabilitySet
```

`AdapterSignal` is mapped into normalized CAR events: output chunks, status changes, approval requests, changed-file hints, diagnostics and completion. Raw terminal data MAY be retained as an artifact but MUST NOT become the only representation of a significant event.

## Wrapper requirements

- Spawn the installed `claude` executable in the workspace's canonical directory.
- Use a PTY only where interactive behavior requires it; retain a noninteractive test path.
- Capture executable version, launch arguments excluding secrets, PID and exit result.
- Kill only the process group owned by the run, never a broad shell or unrelated session.
- Redact configured secret patterns before data reaches events, logs or push notifications.

## Compatibility strategy

The adapter identifies a supported Claude Code version range at start. When parsing confidence is insufficient, it emits `adapter.compatibility_degraded`, preserves raw diagnostics locally, and disables only features it cannot safely normalize. It MUST NOT synthesize an approval or claim that an action was completed.

## Provider boundary

CAR does not route model-provider calls for Claude Code. The user's existing Claude Code configuration remains authoritative. CAR only controls the local CLI process and observes the resulting session.

