# Android state machines

## Connection

```text
disconnected -> connecting -> authenticated -> live
       ^             |             |           |
       |             v             v           v
       +-------- retry/backoff <- expired <- transport_lost
```

The UI may show cached data in `disconnected`, but commands remain disabled until `authenticated` or `live` according to their safety requirements.

## Approval UI

```text
loading -> pending -> submitting -> resolved
                    \-> conflict/expired
```

Submitting is not reversible from the UI. The server response determines the final state.

## Session projection

Session projections use `unknown`, `starting`, `active`, `waiting_approval`, `interrupted`, `completed` and `failed`. Unknown future states render a safe diagnostic card and retain the raw event for debugging; they must not be treated as completed.

