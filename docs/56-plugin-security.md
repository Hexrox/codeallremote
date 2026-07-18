# Plugin security guidelines

## Trust levels

MVP local plugins are operator-installed trusted components. They still receive only registered workspace and core interfaces. Future downloaded plugins must be treated as untrusted until sandboxing, signing, update policy and review are specified.

## Prohibited plugin behavior

- opening an undocumented network listener;
- reading credentials or unrelated home directories;
- writing outside declared artifact/workspace boundaries;
- bypassing approval or authorization;
- emitting events that claim actions not observed by the adapter.

## Review evidence

Every plugin provides manifest, capabilities, required permissions, version compatibility, test fixtures, shutdown behavior and a security contact/owner. A plugin is disabled on manifest mismatch or failed self-check.

