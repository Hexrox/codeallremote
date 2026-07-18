# Protocol fixtures

## Required fixture sets

- session creation and `run.started`;
- streamed output split across multiple chunks;
- approval requested, approved, denied and expired;
- interrupt racing with completion;
- reconnect with contiguous replay;
- cursor-expired resync;
- adapter diagnostic and unsupported capability;
- malformed envelope and unknown additive field.

## Fixture rules

Fixtures contain synthetic IDs, fake paths and fake commands. They are shared by server contract tests and Android parsing tests. A fixture change that alters a durable event requires a protocol-version review.

