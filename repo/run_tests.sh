#!/usr/bin/env bash
# Run every available test via Docker:
#   * Go unit + integration tests (backend) against a throwaway Postgres.
#   * Vitest unit + component tests (frontend).
#
# Exit codes are aggregated so the script returns 0 iff every suite
# passed. Intermediate output is streamed live; the final section
# prints a condensed summary.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$here"

# Pick the compose command (plugin vs. legacy).
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
else
  DC="docker-compose"
fi

COMPOSE_FILE="docker-compose.test.yml"
FAILURES=0

cleanup() {
  $DC -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "[tests] building test images…"
$DC -f "$COMPOSE_FILE" build --progress=plain

echo "[tests] starting Postgres for the backend integration tests…"
$DC -f "$COMPOSE_FILE" up -d db

# Wait for db to report healthy.
echo -n "[tests] waiting for db healthcheck "
for i in $(seq 1 60); do
  state=$($DC -f "$COMPOSE_FILE" ps --format '{{.State}}' db 2>/dev/null || echo "")
  if [ "$state" = "running" ]; then
    if $DC -f "$COMPOSE_FILE" exec -T db pg_isready -U oops -d oops_test >/dev/null 2>&1; then
      echo "(ready)"
      break
    fi
  fi
  sleep 1
  echo -n "."
done

echo ""
echo "================================================================"
echo "  Backend test suite (go test -race -cover ./...)"
echo "================================================================"
if ! $DC -f "$COMPOSE_FILE" run --rm backend-test; then
  FAILURES=$((FAILURES + 1))
  echo "[tests] backend suite FAILED"
else
  echo "[tests] backend suite PASSED"
fi

echo ""
echo "================================================================"
echo "  Frontend test suite (vitest run)"
echo "================================================================"
if ! $DC -f "$COMPOSE_FILE" run --rm frontend-test; then
  FAILURES=$((FAILURES + 1))
  echo "[tests] frontend suite FAILED"
else
  echo "[tests] frontend suite PASSED"
fi

echo ""
echo "================================================================"
if [ "$FAILURES" -eq 0 ]; then
  echo "  All test suites PASSED"
  echo "================================================================"
  exit 0
else
  echo "  ${FAILURES} test suite(s) FAILED"
  echo "================================================================"
  exit 1
fi
