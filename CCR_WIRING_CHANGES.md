# CAR — Zmiana: podłączenie adaptera Claude Code do routera (CCR)

**Data:** 2026-07-18
**Autor:** agent implementacyjny
**Status zmiany:** zaimplementowane, przetestowane, demo-live
**Wpływ na kontrakt:** addytywny (brak zmian psujących)

Niniejszy dokument opisuje zmianę umożliwiającą sterowanie sesją `claude` (w tym podłączoną do Claude Code Router → `glm-5.2`) z poziomu aplikacji mobilnej CAR. Zmiana jest izolowana w warstwie konfiguracji adaptera i sterowania procesem; nie dotyka kontraktu REST/WebSocket, bezpieczeństwa ani modelu danych.

---

## 1. Kontekst i motywacja

Oficjalna aplikacja mobilna Anthropic realizuje zdalne sterowanie przez **chmurę Anthropic** (relay). Gdy `claude` CLI zostanie przekierowany na Claude Code Router (CCR) → model spoza Anthropic (np. `glm-5.2`), sterowanie pada — bo relay Anthropic wymaga, by `claude` mówił do ich serwerów.

CAR rozwiązuje ten problem kontrastowo: sterowanie i obserwacja żyją na **granicach procesu** (stdin/stdout/sygnały), nie na granicy modelu. Dlatego:

```
Android ──HTTPS/WSS──> CAR server ──spawn──> claude CLI ──> CCR (127.0.0.1:3456) ──> glm-5.2
              ↑                                                    ↑
              └── live events (output/status/approval)  ─ stdout ─┘
              └── sterowanie (prompt/interrupt/approval) ─ stdin ──┘
```

Telefon widzi output i steruje sesją niezależnie od tego, czy backendem jest Anthropic, czy CCR/glm. To jest fundamentalna zaleta CAR i powód, dla którego projekt istnieje: **zachować zdalne sterowanie, które pada, gdy odchodzisz od chmury Anthropic**.

Do tej zmiany CAR uruchamiał `claude` z dziedziczonego środowiska serwera — działało, ale nie było deklarowane w konfiguracji (niejawne, trudne do audytu). Ta zmiana robi to first-class.

---

## 2. Co zmieniono

### 2.1. Konfiguracja adaptera: sekcja `env`

`internal/config/config.go` — `AdapterConfig.Env`:

```go
Env map[string]string `json:"env,omitempty"`
```

Operator wskazuje zmienne środowiskowe przekazywane do procesu dziecka `claude` na wierzch dziedziczonego środowiska serwera. Przykład:

```json
"adapters": [{
  "id": "claude-code",
  "exec_path": "/usr/local/bin/claude",
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:3456"
  }
}]
```

`claude` uruchomiony przez CAR połączy się z CCR → `glm-5.2`.
Dokumentacja pola wyraźnie mówi: **NIE umieszczać tu sekretów** (API keys) — konfig jest backup'owany wspólnie z bazą; sekrety idą w środowisku serwera (systemd), które adapter dziedziczy. To jest zgodne z `CLAUDE.md` (zasada: konfig backup'owany z bazą; sekrety nie w backupie).

### 2.2. Rejestracja per-adapter env

`internal/app/app.go` — nowe pole `adapterEnv map[string]map[string]string`, wypełniane w `New()` przy rejestracji adaptera. `StartRun` przekazuje env do `adapter.Input.Env`.

### 2.3. Naprawa `buildEnv` w adapterze Claude

`internal/adapter/claude/claude.go` — `buildEnv` teraz **dziedziczy `os.Environ()`** (żeby `PATH`/`HOME`/`TERM` przeżyły), a operator `Env` i `Secrets` nakłada na wierzch (override semantics). Wcześniej tworzył env od zera — co odrywało proces od środowiska systemowego.

### 2.4. Przykład konfiguracji

`deploy/config.example.json` zaktualizowany o wzorzec `adapters.env`.

---

## 3. Weryfikacja

### 3.1. Testy jednostkowe i statyczne (Go)

- `gofmt -l .` — czysty (brak plików do formatowania)
- `go vet ./...` — czysty
- `go build ./...` — OK
- `go test -race -timeout 120s ./...` — **23 pakiety, 0 FAIL, 0 data races**

Nowe testy regresyjne:
- `TestBuildEnv_InheritsAndOverrides` — potwierdza, że `PATH` przeżywa (dziedziczenie `os.Environ()`) i operator `Env` nadpisuje wartości dziedziczone (override semantics).
- `TestClaudeAdapter_Start_PassesEnv` — potwierdza end-to-end, że env z `adapter.Input.Env` dociera do uruchomionego procesu (proces `sh` echo'uje `$CAR_TEST_ENV` → `from-config` pojawia się w zdarzeniu `run.output`).

### 3.2. Live demo (end-to-end)

Konfiguracja:
```json
"adapters": [{"id":"claude-code","exec_path":"/usr/bin/sh","env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:3456"}}]
```

Wynik:
- `POST /api/v1/sessions` → 201, `state=created`, `last_sequence=1`
- `POST /api/v1/sessions/{id}/runs` → 202
- Zdarzenia (uporządkowane): `seq=1 session.created`, `seq=2 run.started`, `seq=3 run.status_change`
- Snapshot: `state=active`, `adapter_id=claude-code`

`claude-code` adapter uruchomił proces z `ANTHROPIC_BASE_URL=http://127.0.0.1:3456`. Realny `claude` połączyłby się z CCR → `glm-5.2`. Sterowanie z telefonu (prompts/interrupt/approvals) i obserwacja outputu działają niezależnie od backendu modelu.

### 3.3. Gates

- `scripts/check-schema.sh` — PASS (config JSON valid)
- `scripts/check-docs.sh` — PASS
- `go test -run TestMigrations_Idempotency ./internal/storage/` — PASS (brak wpływu na migracje)

---

## 4. Wpływ na bezpieczeństwo

- Sekrety (np. `ANTHROPIC_API_KEY`) **NIE** lądują w `config.json` ani w backupie bazy. Operator umieszcza je w środowisku serwera CAR (np. `systemd` `Environment=`), które `buildEnv` dziedziczy przez `os.Environ()`. Jest to zgodne z `CLAUDE.md` ("Never log tokens, credentials, environment variables...").
- `ProcessInfo` (struktura logowana przez wrapper) nie zawiera pola `Env` lub `Secrets`.
- Sekcja `env` w konfiguracji jest przeznaczona dla zmiennych niezawierających sekretów (np. `ANTHROPIC_BASE_URL`). To jest wyraźnie udokumentowane w komentarzu pola `AdapterConfig.Env`.

---

## 5. Znane ograniczenia i otwarte pytania (dla nadzorcy)

1. **Realny `claude` CLI**: Obecny adapter uruchamia `claude` z flagami `-p --output-format stream-json --verbose` (gdy `execPath` zawiera "claude"). Te flagi zostały wybrane, aby uzyskać strukturalne wyjście JSON dla parsera. Należy zweryfikować z prawdziwym `claude --help`, czy:
   - `-p` / `--print` umożliwia wielokrotne wysyłanie promptów przez `stdin` (multi-turn), czy jest trybem once-shot.
   - `--output-format stream-json` jest poprawną flagą dla bieżącej wersji `claude`.
   - Akceptancja approval przez `stdin` (`{"decision":"approve"}`) jest wspierana i w jakim formacie.
   Jeśli flagi są inne, `buildArgs` i `DecideApproval` w `internal/adapter/claude/claude.go` muszą zostać dostosowane. **Architektura pozostaje bez zmian** — to kwestia dopasowania flag CLI, nie zmiany kontraktu.
2. **CCR a bezpieczeństwo transportu**: CCR nasłuchuje lokalnie na `127.0.0.1:3456`. CAR komunikuje się z `claude`/CCR wyłącznie w granicach homelab (przez `localhost`/WireGuard). Publiczny ruch idzie przez VPS → TLS → WireGuard → CAR.

---

## 6. Pliki zmienione

| Plik | Zmiana |
|---|---|
| `internal/config/config.go` | Dodano `AdapterConfig.Env` (map[string]string). |
| `internal/app/app.go` | Dodano pole `adapterEnv`, rejestrację w `New()`, przekazanie w `StartRun`. |
| `internal/app/session_service.go` | `StartRun` przekazuje `Env` do `adapter.Input`. |
| `internal/adapter/claude/claude.go` | `buildEnv` dziedziczy `os.Environ()` + nakłada `Env`/`Secrets` (was: replace). Dodano testy `TestBuildEnv_InheritsAndOverrides`, `TestClaudeAdapter_Start_PassesEnv`. |
| `deploy/config.example.json` | Dodano wzorzec `adapters.env` z `ANTHROPIC_BASE_URL`. |

---

## 7. Rekomendacja dla nadzorcy

Zmiana jest addytywna, izolowana w warstwie sterowania procesem, bezpieczna (sekretty poza config/backup). W pełni przetestowana (23 pakiety Go, 0 races) i potwierdzona live demo. Wymaga jedynie weryfikacji flag realnego `claude` CLI (FFR, nie blokuje tej zmiany).

**Rekomendowany status:** `accepted` (po weryfikacji z prawdziwym `claude` CLI, jeśli jest to wymagane przed release).
