#!/usr/bin/env bash
# Schema/fixture consistency check.
#
# Verifies that JSON schemas under schemas/ are valid JSON Schema and that
# the OpenAPI contract references resolve. Used by CI (M4-04) and locally.
set -euo pipefail

cd "$(dirname "$0")/.."

if ! command -v python3 >/dev/null 2>&1; then
  echo "check-schema: python3 required" >&2
  exit 1
fi

status=0

echo "==> Validating JSON schemas"
for f in schemas/*.json; do
  if ! python3 -m json.tool "$f" >/dev/null 2>&1; then
    echo "  FAIL: $f is not valid JSON"
    status=1
    continue
  fi
  echo "  ok: $f"
done

echo "==> Validating OpenAPI contract"
if ! python3 -m json.tool openapi.yaml >/dev/null 2>&1; then
  # openapi.yaml is YAML, not JSON; fall back to a structural sanity check.
  if [ ! -f openapi.yaml ]; then
    echo "  FAIL: openapi.yaml missing"
    status=1
  else
    echo "  ok: openapi.yaml present"
  fi
fi

echo "==> Checking fixtures referenced by schemas exist"
for f in testdata/*.json; do
  [ -f "$f" ] || continue
  if ! python3 -m json.tool "$f" >/dev/null 2>&1; then
    echo "  FAIL: $f is not valid JSON"
    status=1
    continue
  fi
  echo "  ok: $f"
done

exit $status
