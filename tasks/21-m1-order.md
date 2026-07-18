# M1 recommended execution order

1. M1-01 server bootstrap and configuration.
2. M1-02 storage and event journal.
3. M1-08 workspace registration policy.
4. M1-03 session manager and idempotency.
5. M1-09 session snapshot projection.
6. M1-10 event cursor repository.
7. M1-11 audit writer.
8. M1-04 adapter boundary and fake adapter.
9. M1-12 graceful shutdown/reconciliation.
10. M1-05 process wrapper.
11. M1-06 output normalization.
12. M1-07 approval bridge.

The order intentionally establishes durable state and a fake adapter before touching Claude Code-specific process behavior.

