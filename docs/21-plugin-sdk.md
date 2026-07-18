# Plugin SDK

## Purpose

The plugin SDK allows CAR to add agent adapters and optional integrations without coupling them to Android, storage or transport. Plugins are server-side components loaded from an explicit allowlist.

## Plugin boundaries

A plugin MAY provide an adapter, capability metadata, configuration schema and health diagnostics. It MUST use CAR interfaces for sessions, events, approvals and artifacts. It MUST NOT open arbitrary listeners, read unregistered workspaces, or bypass authorization.

## Versioning

Every plugin declares `plugin_id`, semantic version, SDK compatibility range and capabilities. Core rejects incompatible plugins before startup. Breaking SDK changes require a new major version and migration note.

## Lifecycle

```text
discover -> validate manifest -> load -> self-check -> ready
                                      \-> degraded / rejected
ready -> drain -> unload
```

Shutdown gives a plugin a bounded drain period. A plugin that fails self-check is visible in diagnostics but cannot create sessions.

## Security

MVP plugins are trusted local binaries/processes installed by the operator. A future sandboxed plugin model must be designed separately; the SDK must not imply that untrusted marketplace code is safe.

