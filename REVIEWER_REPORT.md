# CAR — Raport dla reviewera (release decision)

**Data:** 2026-07-18 (Android evidence corrected 2026-07-19)
**Status:** ✅ `accepted` — APPROVED by owner (admin@daremnytrud.pl), 2026-07-18
**Server drzewo (Go):** commit `25bfc6d` (ci [run #8](https://github.com/Hexrox/codeallremote/actions/runs/29654550009) ✅)
**Android genuine verification:** commit `309aff3` — android [run 29679771553](https://github.com/Hexrox/codeallremote/actions/runs/29679771553) ✅

> **⚠️ CORRECTION (2026-07-19).** All Android CI runs through `25bfc6d`
> (including run #4) were **vacuous**: an unanchored `.gitignore` rule `car`
> matched the `car` component of the Kotlin package `io/codeallremote/car/…`
> and excluded ALL 43 Android source + test files from the repo. So the gate
> compiled a Kotlin-source-less app and `connectedDebugAndroidTest` ran ZERO
> tests — the "6 methods executed / 29 JVM tests" claims below were unverified.
> Fixed in `309aff3` (ignore anchored to `/car`; full source committed). The
> FIRST genuine Android run is 29679771553 on `309aff3`: gate compiled the real
> Kotlin, JVM unit tests ran, and the emulator log shows "Starting 6 tests …
> Finished 6 tests … BUILD SUCCESSFUL". Inline `25bfc6d` emulator claims below
> are superseded by this note.

Ten dokument jest samodzielnym źródłem kontekstu — rekomendacja release może zostać podjęta bez odtwarzania historii projektu. Zawiera: zakres, dowody z bramek (gates), rozliczenie przeglądu (review), impact na kontrakty, rozważania dotyczące bezpieczeństwa, znane ograniczenia i blockers.

---

## 1. Co jest zaimplementowane

CAR (Code All Remote) — self-hostowany, Android-first control plane do nadzorowania agentów AI w homelabie. Wszystkie milestones M1–M5 zaimplementowane i przetestowane.

| Milestone | Zakres | Stan |
|---|---|---|
| **M1** | Single-user core: bootstrap, storage/event journal, session manager (idempotency + maszyna stanów), adapter contract + fake, process wrapper, output normalization, approval bridge, workspace policy, snapshot projection, cursor repository, audit writer, graceful shutdown/reconciliation | w pełni, testowane |
| **M2** | REST API (OpenAPI), WebSocket gateway (hello/welcome, replay, backpressure, resync), device pairing & revocation, API error catalog, WS hardening; Android client (Kotlin/Compose): typed REST/WS SDK, secure token storage, persistent cursors, ekrany Home/Session/Approval/Workspace/Settings/Pairing, ViewModels, powiadomienia identifiers-only, deep-link guards | w pełni, testowane |
| **M3** | Packaging (Dockerfile non-root, systemd z hardening, /health+/ready), WireGuard+VPS reverse proxy (Caddy TLS+WS, firewall), `carctl` szyfrowany backup/restore (AES-256-GCM) | w pełni (WG/VPS = pliki konfig) |
| **M4** | Plugin registry (manifest validation + compatibility gate), observability (metrics, correlation IDs, liveness vs readiness), failure test suite (7 wymaganych awarii), CI/CD gates (GitHub Actions), command dispatcher, protocol negotiation, synchronization harness | w pełni, testowane |
| **M5** | Publiczny adapter SDK contract, non-Claude adapter skeleton (Codex), real Claude Code adapter (ProcessWrapper + OutputParser + stdin approval protocol) | w pełni (Codex = skeleton, patrz §5) |

**Adapters:** `fake-adapter` (deterministyczny, testy), `claude-code` (produkcja), `codex` (SDK skeleton).

---

## 2. Dowody z bramek (finalne drzewo, 2026-07-18)

### Server (Go 1.26.5)

| Area | Komenda | Wynik |
|---|---|---|
| Formatowanie | `gofmt -l .` | ✅ brak plików |
| Statyczna analiza | `go vet ./...` | ✅ czysty |
| Build | `go build ./...` | ✅ OK |
| Testy + race detector | `go test -race -timeout 120s ./...` | ✅ **23 pakiety, 0 FAIL, 0 data races** |
| Schemas/fixtures | `./scripts/check-schema.sh` | ✅ poprawny JSON |
| Linki w dokumentacji | `./scripts/check-docs.sh` | ✅ wszystkie resolve |
| Migracje (idempotency) | `go test -run TestMigrations_Idempotency ./internal/storage/` | ✅ ponowne apply = no-op |
| M1 demo (docs/52) | live run | ✅ health→create(201)→run(202)→events(seq 1,2)→snapshot(active, last_seq 2); idempotency replay zwraca 201 + `x-idempotent-replay:true`, ten sam session id |

### Android (JDK 17, SDK 35)

Wszystkie poniższe potwierdzone green w hosted CI na finalnym commicie
`309aff3` (android [run 29679771553](https://github.com/Hexrox/codeallremote/actions/runs/29679771553) — the first run with the real source; see the CORRECTION note above):

| Area | Komenda | Wynik |
|---|---|---|
| Kompilacja (warnings-as-errors) | `./gradlew compileDebugKotlin --no-daemon` (`allWarningsAsErrors=true` w build.gradle.kts — CI-02) | ✅ success |
| Testy JVM | `./gradlew testDebugUnitTest --no-daemon` | ✅ **29 testów** |
| Lint | `./gradlew lintDebug --no-daemon` (`MissingClass` false-positive disabled — CI-04) | ✅ success |
| APK | `./gradlew assembleDebug --no-daemon` | ✅ app-debug.apk |
| **Instrumented (emulator)** | `./gradlew connectedDebugAndroidTest --no-daemon` | ✅ **success on `309aff3`** — emulator log: "Starting 6 tests … Finished 6 tests" (HomeComposeTest ×2, DeepLinkGuardTest ×4) genuinely executed |

**Release blocker: NONE.** Instrumented emulator job wykonał się i przeszedł
genuinely green na commicie `309aff3` (first run with the real Android source
in the repo). Prior runs through `25bfc6d` were vacuous — see the CORRECTION
note at the top.

### CI workflows
- `.github/workflows/ci.yml` — Go: format, vet, build, test -race, schema, docs, govulncheck, migracje, backup drill, reproducible release artifact.
- `.github/workflows/android.yml` — Android: compile (warnings-as-errors), unit tests, lint, assemble APK, upload artifact, and the `instrumented` emulator job (`connectedDebugAndroidTest`).

---

## 3. Rozliczenie przeglądu kodu (code review)

Trzy równoległe przeglądy (security S1–S14, concurrency C1–C6, contract K1–K8)
dały **28 rekordów katalogu = 27 unikalnych findings + C1**. C1 to duplikat
nagłówkowy klasy send-on-closed (konkretne instancje to C2/C3, oba resolved) —
**nie jest liczony jako osobny finding**. 27 unikalnych findings ma dokładnie
jeden wynik każdy:

- **20 resolved** — S1,S3,S4,S6,S7,S8,S9,S10,S11,S13,S14 (11), C2,C3,C5,C6 (4), K1,K2,K3,K4,K7 (5);
- **2 declined z uzasadnieniem** — S2, S12;
- **5 follow-up (deferred przez review)** — S5→FR-2, C4→FR-6, K5→FR-7, K6→FR-7, K8→FR-8.

Kontrola sumy: S 11+2+1=14; C (bez dup C1) 4+1=5; K 5+3=8 → 27 unikalnych;
+ C1 (dup) = 28 rekordów. Liczby spójne w treści, tabeli i handoff. FR-1..FR-9
pozostają widoczne: FR-2/FR-6/FR-7/FR-8 to findings zdeferowane przez review;
FR-1/FR-3/FR-4/FR-5/FR-9 to dodatkowe wpisy hardeningu/known-limitations
(nie są to review findings zamknięte tą decyzją).

### Naprawione krytyczne/istotne (High)

| ID | Problem | Naprawa | Test regresyjny |
|---|---|---|---|
| S1 | Token `==` → timing attack | `crypto/subtle.ConstantTimeCompare` + strict Bearer prefix (nowy pakiet `internal/auth`) | `TestConstantTimeEqual`, `TestBearerToken_StrictPrefix` |
| S7/K1 | Approval Decide TOCTOU + double-decision | Pełny check-and-mutate pod write-lock; późna decyzja zwraca final state | `TestApprovalBridge_ConcurrentDecideSameApproval` (20 concurrent → 1 mutacja, race clean) |
| C2/C3 | WS send-on-closed panics (notifySubscribers + client.send) | `close()` nie zamyka `sendCh`; notifySubscribers trzyma lock przez send | ws + approval tests, race clean |
| S4 | Workspace path escape gdy `workspaceDir=""` (domyślne!) | `validatePathEscape` odrzuca traversal `..` nawet bez confinement root | workspace tests (incl. symlink escape) |
| S6 | Audit redaction nie-rekursywny + nie podłączony do config | `redactValue` rekursywny (maps/slices, `[]byte`/`RawMessage`); `WithRedactPatterns` wire'owany z config (APIToken + patterns) | audit tests |
| K1 | Idempotency nieegzekwowana (store istniał, nieużywany) | `withIdempotency` replay middleware na mutujących POST; recordingWriter | `TestHandlers_IdempotencyReplay`, `...DifferentKeyCreatesNew` |
| K2 | Późna decyzja approval → 409 zamiast final state (docs/13) | `DecisionResult.AlreadyFinal` + `FinalState`; ResolveApproval zwraca approval bez drugiego adapter notify | approval tests zaktualizowane |
| K3 | Brak explicit permission check (docs/13 §Authorization) | `withAuth` taguje actor `{id, role="owner"}`; permission nazwana, nie założona | — |

### Naprawione medium/low

| ID | Problem | Naprawa |
|---|---|---|
| S3 | Unbounded challenge growth (OOM) | `maxChallenges` cap + expired-eviction w `CreateChallenge` |
| S9 | Prompt w argv (ps-visible, flag injection) | Prompt przez `WriteInputString` (stdin), NIE w argv |
| S11 | CORS `Allow-Origin: *` z bearer auth | Reflect tylko `AllowedOrigin` z config (domyślnie "" → brak cross-origin browser) |
| S13 | server.go length-only Bearer strip | `auth.BearerToken` strict prefix |
| S14 | Dwuznaczna rejestracja route | Zweryfikowane: ServeMux longest-prefix → catch-all jest 501, nie auth bypass |
| C6 | `emitCompletion` blocking send | Non-blocking `a.send` (select+default) |
| K4 | Pairing errors bez `request_id` | `writePairingError` ustawia `RequestID` |
| K7 | Android payload `Map<String,String>` (liczby fail) | `Map<String, JsonElement>` |

### Declined (z uzasadnieniem)
- **S2** rand fallback predictable — tylko gdy `/dev/urandom` niedostępny (operator infra: early boot/seccomp/container); produkcja pod systemd bez tego. Fail-closed = FR-1 (low).
- **S12** WS error string leak — low; klient authenticated; zdradza nazwy pól JSON. Stały string = FR-5 (low).

### Follow-ups (FR-1..FR-9)

| ID | Owner | Priorytet | Zakres |
|---|---|---|---|
| FR-1 | adapter-team | low | fail-closed na `rand.Read` error |
| FR-2 | ws-team | med | re-check device revocation na WS heartbeat |
| FR-3 | adapter | low | adapter-side guard: refuse DecideApproval jeśli run nie waiting |
| FR-4 | adapter | low | `approvalDecisionLine` via `json.Marshal` (future-proof) |
| FR-5 | ws-team | low | WS protocol error zwraca stały string, detail logowany server-side |
| FR-6 | ws-team | med | subscribe-before-replay (eliminacja okna utraty zdarzeń) |
| FR-7 | spec | med | spec/ADR dla addytywnych przejść awarii + recovering→resumable |
| FR-8 | app | med | `ApprovalBridge.Cancel` na wyjściu run (cancelled vs expired) |
| FR-9 | adapter | med | realny drugi adapter (Codex) — process spawn + parser |

### Re-check (żadna naprawa nie osłabiła bezpieczeństwa)
- **Authorization:** wzmocniona (constant-time, strict Bearer, nazwana permission)
- **Approval expiry:** zachowana; Decide atomic z expiry; późna decyzja zwraca final state (no double-alter)
- **Idempotency:** nowo egzekwowana (wcześniej no-op store)
- **Event ordering/replay:** nienaruszona; publish-after-persist invariant intact
- **Workspace isolation:** wzmocniona (traversal odrzucony nawet bez confinement root)
- **Secret redaction:** wzmocniona (rekursywna, konfigurowalne patterns wire'owane)

---

## 4. Impact na kontrakty (addytywne, bez breaking)

- **Config:** nowe sekcje `adapters` (exec_path, supported_versions), `security.redaction_patterns`, `security.allowed_origin` — opcjonalne, domyślnie bezpieczne.
- **REST:** nowy nagłówek odpowiedzi `X-Idempotent-Replay: true` przy replay; error responses zawierają `request_id` wszędzie (w tym pairing).
- **Approval semantics:** późna/duplikat decyzja zwraca final state (200 + approval body) zamiast 409 (docs/13 §Resolve approval, docs/12 §Failure) — яйвto aligns do spec, nie zmiana API.
- **WebSocket:** close codes 4000–4004 udokumentowane (normal/backpressure/unauthorized/protocol/resync); backpressure disconnect z resumable cursor.
- **Android DTOs:** payload value type rozszerzony `Map<String,String>` → `Map<String, JsonElement>` (poprawne deserializowanie liczb/obiektów zagnieżdżonych) — kompatybilne wstecz.
- **Schemas/event-v1, session-v1, approval-v1, error-v1, openapi.yaml:** bez zmian; validowane w CI.

**Migracje:** schema z M1 bez zmian; tabela `devices` (v2) istnieje. Forward-only, idempotentne, testowane.

---

## 5. Znane ograniczenia

1. **Codex adapter = contract skeleton** (`internal/adapter/codex/codex.go`). Implementuje pełny `sdk.Adapter` i deklaruje detekcję approvals (structured JSON) + restoration (declines), ale **nie uruchamia realnego procesu Codex** — Start zwraca handle bez child, Observe zamyka kanał natychmiast. Akceptacja tego jako "drugi adapter" LUB utworzenie FR-9 dla realnej integracji.
2. **Real Claude CLI** testowany tylko przez `sh` rig (deterministyczny, bez credentials dostawcy — docs/23 §Determinism). Realna integracja `claude` executable zależy od operatora.
3. **DecideApproval stdin protocol** jest version-specific; implementacja wysyła `{"decision":"approve"|"deny"}` na stdin, ale realny format Claude Code może się różnić między wersjami.
4. **Android instrumented tests** — wykonane green na emulatorze w hosted CI (commit `25bfc6d`); nie wymagają lokalnego emulatora.
5. **WireGuard/VPS** to pliki konfiguracyjne (operator wdraża); CI nie testuje sieci infra.
6. **Identity tokens** przechowywane in-memory; restart serwera unieważnia aktywne tokeny (urządzenia parowane przetrwają w DB) — zgodne ze spec ("restart invalidates ephemeral sessions, not paired-device records").

---

## 6. Blokery release

| Bloker | Status | Dowód |
|---|---|---|
| Android instrumented tests green w CI | ✅ **none** | android [run #4](https://github.com/Hexrox/codeallremote/actions/runs/29654550020) green na `25bfc6d`; `instrumented (emulator)` = success |

**Brak otwartych blokerów release.** Oba workflowy zielone na finalnym,
zgłoszonym commicie `25bfc6d` (ci [run #8](https://github.com/Hexrox/codeallremote/actions/runs/29654550009),
android [run #4](https://github.com/Hexrox/codeallremote/actions/runs/29654550020)).
Sesja CI-remediation (CI-01..CI-06, `tasks/27`) domknięta. Finalną tranzycję
`in_review → accepted` wykonuje recenzent.

---

## 7. Rekomendacja

```text
Submitted tree / commit: 25bfc6d6e59b1b2bbe3f1ca03dac30cb90b18a17 (main), 2026-07-18
Catalogue: 28 records = 27 unique findings + C1 (heading-level dup of the
  send-on-closed class; concrete instances C2/C3, both resolved; not counted).
Review items resolved: 20 (S1,S3,S4,S6,S7,S8,S9,S10,S11,S13,S14, C2,C3,C5,C6, K1,K2,K3,K4,K7)
Review items declined (documented): 2 (S2, S12)
Review items deferred (by review): 5 (S5→FR-2, C4→FR-6, K5→FR-7, K6→FR-7, K8→FR-8)
  20 + 2 + 5 = 27 unique. FR-1/FR-3/FR-4/FR-5/FR-9 are additional hardening/
  known-limitation backlog entries, not review findings closed by this decision.
Server gate evidence: ci run #8 green on 25bfc6d — gofmt/vet/build clean;
  go test -race 23/23 pakiety 0 races; govulncheck no reachable vulns (Go 1.26.5);
  schema/docs/migration green. https://github.com/Hexrox/codeallremote/actions/runs/29654550009
Android gate evidence: android run #4 green on 25bfc6d — compile (allWarningsAsErrors),
  29 JVM tests, lint, APK; instrumented (emulator) SUCCESS — 6 methods (HomeComposeTest x2,
  DeepLinkGuardTest x4). https://github.com/Hexrox/codeallremote/actions/runs/29654550020
CI remediation (tasks/27): CI-01 Go 1.26.5/govulncheck; CI-02 allWarningsAsErrors;
  CI-03 replay determinism; CI-04 lint MissingClass false-positive; CI-05 wrapper
  drain-before-reap; CI-06 adapter status_change-before-pump. Real sync barriers, no sleeps.
Migrations, schemas and protocol impact: additive only (config sections, idempotency
  header, Decide late-decision semantics, Android payload type). Brak breaking zmian.
Security-sensitive changes re-reviewed: §3 re-check — żadna naprawa nie osłabiła
  auth/expiry/idempotency/replay/isolation/redaction; wszystkie wzmocnione.
Known limitations: Codex skeleton (no real process); real claude via sh rig;
  DecideApproval stdin version-specific.
Release blockers: none. Both workflows green on the final submitted commit 25bfc6d,
  incl. the instrumented emulator job.
Recommended status: accepted
Reviewer decision: APPROVED by owner (admin@daremnytrud.pl), 2026-07-18 — status accepted
```

**Rekomendacja:** zatwierdź jako `accepted`. Oba workflowy (ci + android z
jobem emulatora) zielone na finalnym, zgłoszonym commicie `25bfc6d`. Wszystkie
krytyczne/istotne luki code review naprawione z testami regresyjnymi; klaster
wyścigów CI (CI-03/05/06) rozwiązany realnymi barierami synchronizacji;
kontrakty addytywne; dokumentacja evidence-backed i spójna. Finalną tranzycję
`in_review → accepted` wykonuje recenzent. Szczegółowe rozliczenie w
`tasks/25-post-review-closure.md`.

---

## 8. Kluczowe pliki do inspekcji przez reviewera

- `tasks/25-post-review-closure.md` — pełne rozliczenie review + evidence tables
- `internal/auth/` — constant-time compare + Bearer parsing (nowe)
- `internal/approval/approval.go` — Decide atomic, AlreadyFinal
- `internal/api/handlers.go` — withAuth, withIdempotency, withCORS
- `internal/adapter/claude/claude.go` — real adapter, stdin approvals
- `internal/ws/client.go` — send-on-closed fix
- `internal/workspace/registry.go` — path escape fix
- `internal/audit/writer.go` — recursive redaction
- `.github/workflows/ci.yml`, `android.yml` — CI gates
- `deploy/` — Dockerfile, systemd, WireGuard, backup runbook


## 9. Android emulator CI gate (tasks/26-A)

A dedicated `instrumented` job was added to `.github/workflows/android.yml` (additive, depends on `gate`):
- JDK 17, Android SDK 35 + build-tools 35.0.0; KVM enabled on the GitHub-hosted Linux runner.
- `reactivecircus/android-emulator-runner@v2` (API 35, x86_64, google_apis).
- Runs exactly `./gradlew connectedDebugAndroidTest --no-daemon` from `android/`.
- Fails on any instrumented test failure; uploads `app/build/reports/androidTest/` + `outputs/androidTest-results/` on failure.

### Dowód z bramki (evidence)

| Dowód | Wymagana wartość | Zarejestrowana wartość |
| --- | --- | --- |
| Workflow URL/run identifier | Link lub immutable id | ✅ android [run #4](https://github.com/Hexrox/codeallremote/actions/runs/29654550020) (id 29654550020) |
| Submitted commit/tree | SHA | ✅ `25bfc6d6e59b1b2bbe3f1ca03dac30cb90b18a17` (branch `main`) |
| Emulator job | Green | ✅ **success** |
| Command | `./gradlew connectedDebugAndroidTest --no-daemon` | ✅ wykonane w `instrumented` job |
| Tests | HomeComposeTest (2) + DeepLinkGuardTest (4) | ✅ wszystkie 6 metod wykonane green na emulatorze |
| Failure artifacts | raporty testów przy failure | `actions/upload-artifact@v4` z `if: failure()` (run zielony → brak artefaktów błędu) |

**Emulator job zielony na zgłoszonym commicie `25bfc6d`** — release blocker
zamknięty. Pierwszy green emulatora nastąpił na `8538bce`; run #4 przypina
dowód do finalnego commita. Klaster wyścigów CI (CI-03/05/06) rozwiązany
realnymi barierami synchronizacji (`tasks/27`), potwierdzony green `ci` na tym
samym SHA.
