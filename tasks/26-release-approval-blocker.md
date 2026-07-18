# Release approval blocker — Android instrumentation and evidence reconciliation

## Decision

Project remains **`in_review`**. Do not change any milestone to `accepted`
until both acceptance criteria below are met on the submitted tree:

1. Android instrumented tests run green on an emulator in GitHub Actions.
2. `REVIEWER_REPORT.md` and `tasks/25-post-review-closure.md` contain
   internally consistent accounting of review findings and gate status.

This is a release-readiness task only. Do not add product features or change a
public contract while carrying it out.

## A. Add the missing Android emulator CI gate

The current `.github/workflows/android.yml` explicitly says instrumented tests
are a follow-up, but it has only the `gate` job; no job runs
`connectedDebugAndroidTest`. Add a separate emulator job (it may depend on
`gate`) that:

- uses JDK 17 and Android API/SDK 35;
- provisions a hardware-accelerated Android emulator on the GitHub-hosted
  Linux runner;
- runs exactly `./gradlew connectedDebugAndroidTest --no-daemon` from
  `android/`;
- fails the workflow when any instrumented test fails;
- uploads the relevant test reports on failure; and
- is triggered for the same Android/workflow paths as the existing gate.

Use a maintained emulator action or explicit SDK/AVD setup compatible with
GitHub-hosted runners. Keep the existing compile, JVM-test, lint and APK jobs;
the emulator job is additive, not a replacement.

### Evidence required

Record in `REVIEWER_REPORT.md` and `tasks/25-post-review-closure.md`:

| Evidence | Required value |
| --- | --- |
| Workflow URL/run identifier | Link or immutable run identifier |
| Submitted commit/tree identifier | SHA preferred; explain if unavailable |
| Emulator job | Green |
| Command | `./gradlew connectedDebugAndroidTest --no-daemon` |
| Tests | Both `HomeComposeTest` methods and all four `DeepLinkGuardTest` methods executed |
| Failure artifacts | Test reports available when the job fails |

If the emulator cannot run, leave status `blocked` and include the complete CI
error plus the smallest reproducible remediation. Compilation of the
`androidTest` source set is not a substitute for execution.

## B. Reconcile the review evidence before final sign-off

Correct the report rather than papering over inconsistencies. At the time this
task was created, the report contains these discrepancies:

- It says **28** candidate findings, while the listed IDs are S1–S14 (14),
  C2–C6 (5) and K1–K8 (8): 27 rows. C1 is mentioned in the heading but has no
  row.
- It says **16 resolved**, but the parenthesized resolved-ID list in the final
  handoff has 17 IDs.
- Several rows use `accepted` or `verified clean`; choose and define one
  classification so those rows are not ambiguously counted as fixed.
- `tasks/25-post-review-closure.md` marks its gate-evidence acceptance
  criterion as complete although the only release blocker has not yet run.

For every candidate finding, retain one row with exactly one final outcome:
`resolved`, `declined`, or `follow-up`. Then make the total in the prose,
tables and final handoff agree. If C1 was duplicate/non-actionable, state that
explicitly; otherwise add its disposition. The final handoff must distinguish:

- resolved issues (with a regression test or rationale),
- declined issues (with risk acceptance), and
- deferred issues (FR ID, owner, priority, acceptance criterion).

Until the emulator job has a green result, show the verification/release gate
as **pending**, not passed.

## C. Finish

After A and B, request reviewer sign-off with this concise decision record:

```text
Android instrumented CI run:
Review-accounting reconciliation:
Remaining follow-ups / risk acceptance:
Release blockers: none / list
Recommended status: accepted / blocked
Reviewer decision:
```

Only a green emulator result, a reconciled report, and reviewer approval permit
`in_review` → `accepted`. FR-1 through FR-9 remain visible follow-up work;
they are not silently closed by this release decision.

## D. Immediate execution instruction for the implementation agent

Perform these steps in order. Do not mark this task complete before step 4.

1. **Correct the C1 accounting in both evidence documents.**
   In `REVIEWER_REPORT.md` and `tasks/25-post-review-closure.md`, record C1
   as a catalogue/header duplicate, not as a resolved finding. Use one
   unambiguous summary:

   ```text
   28 catalogue records = 27 unique findings (20 resolved, 2 declined,
   5 follow-up) + C1, a duplicate/non-actionable header reference.
   ```

   Remove C1 from the list of 20 resolved IDs. Explain that C2 and C3 are the
   concrete resolved send-on-closed findings. Keep FR-1..FR-9 visible, but
   distinguish the five findings deferred by review from additional hardening
   backlog entries created from accepted risk/known limitations.

2. **Publish the exact post-review tree to the configured Git remote.**
   Create or update the review branch/PR; do not substitute a local build for
   hosted CI. Record the resulting commit SHA.

3. **Wait for the `android` GitHub Actions workflow.**
   The `instrumented` job must run after `gate` and pass:

   ```bash
   ./gradlew connectedDebugAndroidTest --no-daemon
   ```

   Confirm that all six instrumented methods execute: two in
   `HomeComposeTest` and four in `DeepLinkGuardTest`. If the job fails, use the
   uploaded Android test reports, fix the reported defect/configuration, push
   the fix and rerun CI. Do not change release status on a failed or skipped
   job.

4. **Record final evidence and request sign-off.**
   Add the commit SHA, Actions run URL/ID, green `instrumented` result and test
   count to both evidence documents. Change the release blocker to `none` only
   after that green run. Then request the reviewer decision:

   ```text
   Recommended status: accepted
   Reviewer decision: pending approval
   ```

   The reviewer, not the implementation agent, makes the final
   `in_review` → `accepted` transition.
