# CI remediation — hosted GitHub Actions failures on `main`

## Context

The first hosted run of both workflows on `main` (commit `414287c`) failed:
`ci` and `android` both red. Local gates were reported green, so every
failure below is a **CI/environment or determinism defect that local runs
did not surface** — exactly the evidence gap `tasks/26-release-approval-blocker.md`
requires closing before `in_review → accepted`.

Consequence for release: the `instrumented (emulator)` job — the standing
release blocker — was **skipped**, because it `needs: gate` and the Android
`gate` job failed first. There is still zero evidence for the release blocker.

Do not mark this file complete until all three gates are green in hosted CI
and the emulator job has actually executed. Do not change any public contract.

Evidence (run `414287c`):
- ci run 29651487113 — jobs `dependency vulnerability scan` + `format / vet / test / schema / docs` failed.
- android run 29651487115 — job `kotlin / lint / unit / apk` failed; `instrumented (emulator)` skipped.

---

## CI-01 — Bump the Go toolchain to a patched release (govulncheck)

**Symptom.** `dependency vulnerability scan` job fails, exit code 3.

**Root cause.** `.github/workflows/ci.yml` pins `GO_VERSION: "1.24.1"` and
`go.mod` declares `go 1.24.1`. `govulncheck` reports **27 standard-library
vulnerabilities** reachable from our code in go1.24.1, e.g.:
- `GO-2025-3749` — `crypto/x509` via `http.Server.Serve` (`internal/server/server.go:230`).
- `GO-2025-3563` — `net/http/internal` via `carctl` restore (`cmd/carctl/restore.go:195`).

These are fixed in later Go patch releases; local passes because the
developer machine runs a newer patched Go (1.26.x).

**Fix.** Raise `GO_VERSION` in `.github/workflows/ci.yml` (env at line 17,
consumed at lines 29/95/115) and the `go` directive in `go.mod` to a current
patched release that clears these advisories (align CI with the local
toolchain). Keep server and Android JDK settings unchanged.

**Acceptance.** `govulncheck` job green; `go build ./...` and `go test -race
./...` still green on the bumped toolchain.

---

## CI-02 — Fix the Android `-Werror` gate misconfiguration (unblocks the release blocker)

**Symptom.** `compileDebugKotlin` fails: *"Problem configuring task
:app:compileDebugKotlin from command line."* → Android `gate` red →
`instrumented (emulator)` skipped.

**Root cause.** `.github/workflows/android.yml:57` runs
`./gradlew compileDebugKotlin --no-daemon -Werror`. **`-Werror` is not a
Gradle flag**; Gradle parses it as an unknown per-task command-line option
and aborts before compiling. The previously reported "BUILD SUCCESSFUL …
-Werror" was therefore never a real warnings-as-errors gate.

**Fix.** Remove `-Werror` from the Gradle invocation, and enable
warnings-as-errors in the build script instead — in
`android/app/build.gradle.kts` set `allWarningsAsErrors = true` inside the
existing `kotlinOptions` block (alongside `jvmTarget = "17"`, line 42), or the
equivalent `compilerOptions { allWarningsAsErrors.set(true) }`. Then any real
Kotlin warning fails the compile through configuration, not a bogus CLI flag.

**Acceptance.** `gate` job compiles green; `instrumented (emulator)` job is no
longer skipped and actually executes `./gradlew connectedDebugAndroidTest
--no-daemon`, running all six methods (`HomeComposeTest` ×2,
`DeepLinkGuardTest` ×4). Record the run URL and green result in
`REVIEWER_REPORT.md` and `tasks/25-post-review-closure.md`.

---

## CI-03 — Fix non-deterministic `TestApp_ReplayWithCursor`

**Symptom.** `--- FAIL: TestApp_ReplayWithCursor`:
`app_test.go:269: expected 3 events after cursor 1, got 4`. Passes locally,
fails on the hosted runner.

**Root cause (to confirm).** A replay-from-cursor observes an extra event
under the CI scheduler — an ordering/race between cursor replay and an
asynchronous publish in `internal/app`. This violates the project's
determinism guarantee for event replay.

**Fix.** Investigate event emission and cursor replay in `internal/app`;
ensure the count of events after a cursor is deterministic (e.g. the replay
snapshot and the live publish path do not double-count, and replay is
sequenced against publication). Do not paper over it with a sleep or a
loosened assertion.

**Acceptance.** `go test -race -count=20 ./internal/app/` is stably green;
the regression is covered so the double-count cannot silently return.

---

## CI-04 — Android Lint `MissingClass` false positive blocks the gate

**Discovered on run `b3a9fa8`**, after CI-02 fixed the Kotlin compile. The
Android `gate` job now passes compile + warnings-as-errors but fails at the
`Lint` step, so `instrumented (emulator)` is still skipped.

**Symptom.** `./gradlew lintDebug --no-daemon` fails:
`AndroidManifest.xml:28: Error: Class referenced in the manifest,
io.codeallremote.car.android.MainActivity, was not found in the project or the
libraries [MissingClass]` → "Lint found errors in the project; aborting build."
(1 error, 41 warnings).

**Root cause.** False positive. `MainActivity` exists and is correct:
`android/app/src/main/java/io/codeallremote/car/android/MainActivity.kt`,
`package io.codeallremote.car.android`, `class MainActivity : ComponentActivity()`.
The manifest reference `.MainActivity` resolves to that class via the module
`namespace`, and the Kotlin compile step already passed. `MissingClass` is a
known Android-lint partial-analysis limitation for Kotlin-declared activities:
the class is not on lint's classpath during `lintAnalyzeDebug` even though it
compiles and ships.

**Fix.** Add a targeted `lint {}` block to `android/app/build.gradle.kts` that
disables only the spurious check, keeping every other lint check and
`abortOnError` active:

```kotlin
lint {
    // MissingClass is a known false positive for the Kotlin ComponentActivity:
    // the class compiles and exists, but lint's partial analysis can't resolve
    // it on its classpath. Do not broaden this to abortOnError=false.
    disable += "MissingClass"
}
```

Do not disable `abortOnError` and do not add a blanket lint-baseline (which
would also freeze the 41 warnings and hide future regressions).

**Acceptance.** `Lint` step green in the Android `gate` job; `gate` passes end
to end so `instrumented (emulator)` runs and executes all six methods.

## CI-05 — Race in ProcessWrapper: `cmd.Wait()` before pipe reads complete

**Discovered on run `8538bce`.** After CI-01..CI-04, the `ci` job failed on a
non-deterministic test (it had passed on `b3a9fa8`; no Go code changed since):

```
--- FAIL: TestProcessWrapper_Output
    wrapper_test.go:131: expected stdout to contain 'stdout_line', got ''
```

This is a **real wrapper bug**, not just a flaky test.

**Root cause.** `internal/wrapper/wrapper.go` reads child output from
`cmd.StdoutPipe()` / `cmd.StderrPipe()` in `readOutput` goroutines, but
`waitForExit()` calls `w.cmd.Wait()` (line ~221) **before** `w.readersWG.Wait()`
(line ~225). The `os/exec` docs are explicit: *"Wait will close the pipe after
seeing the command exit, so most callers need not close it themselves. It is
thus incorrect to call Wait before all reads from the pipe have completed."*
When `cmd.Wait()` closes the read end before `readOutput` has consumed the
buffered output, the reader gets a premature EOF/closed-file and forwards
nothing → empty stdout. On a loaded CI runner the window widens, so it fails
intermittently.

Secondary (latent) defect in the same file: `readOutput` sends with a
non-blocking `select { case ch <- data: default: /* drop */ }`, which silently
drops output if the channel is momentarily full.

**Fix (applied).** Order the drain before the reap: in `waitForExit`, wait for
the reader goroutines to reach EOF (`readersWG.Wait()`) and only then call
`cmd.Wait()` — the pipes reach EOF when the child exits (its stdout/stderr fds
close), independently of the parent calling `Wait()`, so this does not
deadlock. This matches the documented safe StdoutPipe pattern (read to EOF,
then Wait). Channel-close and `done` signalling stay after the reap. This
alone fixes the empty-output flake (the failing test drains both channels and
the cap-100 buffer never dropped the single line).

**Deliberately NOT changed: the non-blocking send stays non-blocking.**
Switching `readOutput` to a blocking send would, combined with the drain-first
reorder, deadlock the reap in production: only stdout is consumed (the claude
adapter drains `OutputChannel`, never `ErrorChannel`), so a child that floods
stderr past the cap-100 `errCh` buffer would block the stderr reader forever,
`readersWG.Wait()` would never return, and the process would never be reaped.
The non-blocking send is retained (with a comment) to keep the reap path live.
Follow-up (out of CI-05 scope): drain `ErrorChannel()` in the adapter or make
`errCh` backpressure explicit, then the send can become lossless.

**Acceptance.** `go test -race -count=50 ./internal/wrapper/` is stably green
(no empty-output failures); full `go test -race ./...` green. No public
contract change — this is internal process plumbing.

## Finish

After CI-01..CI-05, all jobs in both workflows are green on `main` and the
emulator job has executed. Then reconcile the evidence documents per
`tasks/26` §B and request reviewer sign-off. Until then, the
verification/release gate stays **pending**; `in_review → accepted` is not
permitted.
