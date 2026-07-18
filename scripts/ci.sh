#!/usr/bin/env bash
# Local CI emulator: runs the same gate sequence as the GitHub Actions workflow.
# Use before pushing to catch failures early.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> gofmt (format check)"
if [ -n "$(gofmt -l . | grep -v '^vendor/')" ]; then
  echo "FAIL: files need formatting:"
  gofmt -l . | grep -v '^vendor/'
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> go build"
go build ./...

echo "==> unit + contract + integration tests (race)"
go test -race -timeout 120s ./...

echo "==> schema/fixture consistency"
./scripts/check-schema.sh

echo "==> doc link validation"
./scripts/check-docs.sh

echo "==> migration reproducibility (render + reapply)"
# A migration must be idempotent; re-running migrate on an already-migrated
# DB is a no-op. The storage package tests cover this; ensure they ran.
go test -run 'TestMigrations_Idempotency' ./internal/storage/

echo "ALL GATES PASSED"
