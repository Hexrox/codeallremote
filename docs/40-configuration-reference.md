# Configuration reference

## Rules

Configuration is loaded from a local file and environment-specific secret store. Environment variables override non-secret file values only when documented. Unknown keys fail validation to prevent typos becoming unsafe defaults.

## Sections

- `server`: bind address, port, public URL and shutdown timeout.
- `storage`: database path, artifact path and retention policy.
- `security`: token lifetime, pairing expiry and redaction patterns.
- `adapters`: executable paths, supported versions and workspace policy.
- `notifications`: push provider identifiers, never private keys in plain text.
- `observability`: log level, metrics bind address and sampling.

## Safe defaults

Bind to loopback/local interface, deny unregistered workspaces, require approval for externally visible actions, redact common token formats, and disable debug payload logging. Production configuration must explicitly opt into any broader exposure.

