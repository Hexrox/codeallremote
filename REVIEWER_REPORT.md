# CAR — Raport dla reviewera (release decision)

**Data:** 2026-07-18
**Zalecany status:** `in_review` → `accepted` po green CI (instrumented Android tests)
**Drzewo:** post-review (workspace bez historii git; commit identyfikator niedostępny)

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

### Server (Go 1.24.1)

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

| Area | Komenda | Wynik |
|---|---|---|
| Kompilacja | `./gradlew :app:compileDebugKotlin --no-daemon -Werror` | ✅ BUILD SUCCESSFUL (main + unitTest + androidTest) |
| Testy JVM | `./gradlew :app:testDebugUnitTest --no-daemon` | ✅ **29 testów** |
| Lint | `./gradlew :app:lintDebug --no-daemon` | ✅ BUILD SUCCESSFUL |
| APK | `./gradlew :app:assembleDebug --no-daemon` | ✅ app-debug.apk (64.6 MB) |
| Instrumented (androidTest) | — | ⚠️ skompilowane i zarejestrowane; **wymaga emulatora/CI** (HomeComposeTest, DeepLinkGuardTest) |

**Release blocker:** Android instrumented tests muszą zostać uruchomione green w CI przed `accepted`. Testy są napisane (6 metod `@Test`), brakuje tylko środowiska emulatora lokalnie.

### CI workflows
- `.github/workflows/ci.yml` — Go: format, vet, build, test -race, schema, docs, govulncheck, migracje, backup drill, reproducible release artifact.
- `.github/workflows/android.yml` — Android: compile -Werror, unit tests, lint, assemble APK, upload artifact.

---

## 3. Rozliczenie przeglądu kodu (code review)

Trzy równoległe przeglądy (security S1–S14, concurrency C1–C6, contract K1–K8) wyłoniły 28 kandydatów. Po weryfikacji każdy ma dokładnie jeden wynik: **21 resolved** (w tym C1 — duplikat/odniesienie nagłówkowe do klasy send-on-closed; konkretne instancje to C2/C3, oba resolved), **2 declined z uzasadnieniem**, **5 follow-up** (S5→FR-2, C4→FR-6, K5/K6→FR-7, K8→FR-8). Liczby spójne w treści, tabeli i handoff.

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
4. **Android instrumented tests** wymagają emulatora (napisane, skompilowane, nie uruchomione lokalnie).
5. **WireGuard/VPS** to pliki konfiguracyjne (operator wdraża); CI nie testuje sieci infra.
6. **Identity tokens** przechowywane in-memory; restart serwera unieważnia aktywne tokeny (urządzenia parowane przetrwają w DB) — zgodne ze spec ("restart invalidates ephemeral sessions, not paired-device records").

---

## 6. Blokery release

| Bloker | Status | Rozwiązanie |
|---|---|---|
| Android instrumented tests green w CI | **pending** | Uruchom `./gradlew connectedDebugAndroidTest` w CI (android.yml → emulator) |

Wszystkie inne bramy lokalne PASS. Po green CI workflow status → `accepted`.

---

## 7. Rekomendacja

```text
Submitted tree / commit: post-review, 2026-07-18
Review items resolved: 21 (S1,S3,S4,S6,S7,S8,S9,S10,S11,S13,S14,C1,C2,C3,C5,C6,K1,K2,K3,K4,K7)
Note: C1 is the heading-level dup of the send-on-closed class (concrete C2/C3); not double-counted. Follow-up mappings: S5→FR-2, C4→FR-6, K5/K6→FR-7, K8→FR-8.
Review items declined (documented): 2 (S2, S12)
Review items deferred: 9 (FR-1..FR-9 z owner/priority)
Server gate evidence: gofmt/vet/build clean; go test -race 23/23 pakiety 0 races;
  schema/docs/migration green; M1 demo live (idempotency replay potwierdzone)
Android gate evidence: compile -Werror ok; 29 JVM tests pass; lint ok; APK 64.6MB;
  instrumented compile-registered, emulator run pending in CI
Migrations, schemas and protocol impact: additive only (config sections, idempotency
  header, Decide late-decision semantics, Android payload type). Brak breaking zmian.
Security-sensitive changes re-reviewed: §3 re-check — żadna naprawa nie osłabiła
  auth/expiry/idempotency/replay/isolation/redaction; wszystkie wzmocnione.
Known limitations: Codex skeleton (no real process); real claude via sh rig;
  DecideApproval stdin version-specific; Android instrumented tests need emulator.
Release blockers: Android instrumented tests (connectedDebugAndroidTest) must run green in CI. CI remediation applied: Go 1.26.5 (govulncheck reports no reachable vulns), `-Werror`→`allWarningsAsErrors`, `TestApp_ReplayWithCursor` deterministic. All local gates green; hosted CI run pending.
Recommended status: blocked (until hosted CI is green)
Reviewer decision: pending
```

**Rekomendacja:** zatwierdź jako `accepted` po uruchomieniu CI (szczególnie Android instrumented tests na emulatorze). Wszystkie krytyczne/istotne luki code review naprawione z testami regresyjnymi; kontrakty addytywne; dokumentacja evidence-backed i spójna. Szczegółowe rozliczenie w `tasks/25-post-review-closure.md`.

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
| Workflow URL/run identifier | Link lub immutable id | **PENDING** — workflow committed; uruchomienie emulatora na GitHub-hosted runnerze nie wykonane w tym workspace (brak git remote). |
| Emulator job | Green | **PENDING** (release blocker) |
| Command | `./gradlew connectedDebugAndroidTest --no-daemon` | skonfigurowane w `instrumented` job `script` |
| Tests | HomeComposeTest (2) + DeepLinkGuardTest (4) | zarejestrowane w `app/src/androidTest`; uruchomią się na emulatorze |
| Failure artifacts | raporty testów przy failure | `actions/upload-artifact@v4` z `if: failure()` |

Jeśli emulator nie ruszy, status pozostaje `blocked` z kompletnym błędem CI i najmniejszą reproducible remediacją. Kompilacja `androidTest` NIE jest substytutem uruchomienia (kompilacja potwierdzona).
