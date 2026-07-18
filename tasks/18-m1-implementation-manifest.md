# M1 implementation manifest

| Task | Inputs | Main outputs | Required evidence |
| --- | --- | --- | --- |
| M1-01 | architecture, guidelines | server bootstrap/config/health | invalid config and shutdown tests |
| M1-02 | domain, storage | migrations/repositories/event journal | transaction and restart tests |
| M1-03 | lifecycle, protocol | state machine/idempotency | transition and race tests |
| M1-04 | adapter ADR/contract | adapter interface/fake adapter | signal mapping contract tests |
| M1-05 | wrapper spec/security | owned process wrapper | fake CLI process tests |
| M1-06 | adapter/protocol | normalized Claude signals | parser fixtures and degraded mode |
| M1-07 | approvals | approval bridge | approve/deny/expiry races |
| M1-08 | security | workspace policy | path/symlink policy tests |
| M1-09 | domain | snapshot projection | restart/reconstruction test |
| M1-10 | protocol | cursor replay | gap/retention tests |
| M1-11 | security | audit writer | redaction and actor tests |
| M1-12 | lifecycle | shutdown/recovery | controlled restart test |

## Ordering constraints

Implement M1-01 and M1-02 first. M1-03 depends on persistence. M1-04 can proceed with a fake adapter after the domain types exist. M1-05/M1-06/M1-07 depend on the adapter boundary. M1-08–M1-12 may proceed in parallel only where repository interfaces are stable.

## M1 exit

Run `docs/52-m1-demo-script.md`, attach evidence for every manifest row and pass the M0→M1/M1→M2 gates.

