#!/usr/bin/env bash
# Bring up the full stack (db + backend + frontend) in the background
# and print the URL a human operator should open. Safe to re-run; it
# rebuilds images, applies migrations, and reseeds idempotently.
#
# Exit codes:
#   0  stack is up and backend /api/health returned 200
#   1  docker compose or the health-check never succeeded within the
#      configured window
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$here"

# Copy .env on first run so every default value from .env.example
# applies. We never overwrite an existing .env.
if [ ! -f .env ]; then
  cp .env.example .env
  echo "[start] copied .env.example -> .env"
fi

# Refuse to launch with the historical placeholder ENC_KEYS value. A
# real deployment MUST supply a random key (see `backend/cmd/keygen`).
# Dev mode can still run with ENC_KEYS unset.
placeholder="1:0101010101010101010101010101010101010101010101010101010101010101"
enc_line="$(grep -E '^ENC_KEYS=' .env || true)"
if [ "$enc_line" = "ENC_KEYS=${placeholder}" ]; then
  echo "[start] ENC_KEYS still uses the well-known placeholder value."
  echo "[start] Generate a fresh key with:  docker compose run --rm --entrypoint /app/keygen backend"
  echo "[start] Refusing to start with a publicly-known key."
  exit 1
fi

# Prefer `docker compose` (plugin) but fall back to `docker-compose`
# (legacy standalone) so either install layout works.
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
else
  DC="docker-compose"
fi

# Fail fast if the Docker daemon is unreachable. Without this the
# script would blunder into `compose up` and emit a noisy stack trace
# instead of a one-line diagnostic.
if ! docker info >/dev/null 2>&1; then
  echo "[start] docker daemon is not reachable — start Docker and retry."
  exit 1
fi

# Load env so we can compute the backend/frontend ports before anything
# binds them. `.env` is never a security boundary here; it only carries
# defaults for docker compose substitution.
# shellcheck disable=SC1091
set -a; . ./.env; set +a
BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"

# Build first so image-build failures are visible with their own exit
# status rather than hidden behind the detached `up`. This is the
# biggest source of "it just silently didn't come up" flakiness on
# cold clones — a syntax or dependency error during `go build` in the
# backend Dockerfile would otherwise look like a health-check timeout.
echo "[start] building images…"
if ! $DC build; then
  echo "[start] image build failed — fix the error above and re-run."
  exit 1
fi

echo "[start] starting the stack…"
if ! $DC up -d; then
  echo "[start] compose up failed — check the output above."
  exit 1
fi

# Poll /api/health. We allow a generous window because the first start
# on a fresh machine also runs the DB init + schema migration + seed,
# which can take a while on slow disks. If the backend container
# exits or enters Restarting during the wait, surface that immediately
# instead of burning the full window — a crashloop is not worth
# waiting out.
echo "[start] waiting for backend at http://localhost:${BACKEND_PORT}/api/health …"
attempts=90       # 90 × 2s = 180s soft cap
interval=2
ok=0
for i in $(seq 1 "$attempts"); do
  # Did the backend container give up?
  state="$($DC ps --format '{{.Service}} {{.State}}' 2>/dev/null | awk '$1=="backend"{print $2}')"
  case "$state" in
    exited|dead)
      echo ""
      echo "[start] backend container is in state '${state}' — it crashed on startup."
      echo "[start] last 80 lines of backend logs:"
      $DC logs --tail=80 backend || true
      exit 1
      ;;
  esac
  if curl -fsS "http://localhost:${BACKEND_PORT}/api/health" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep "$interval"
  # Dot-per-attempt progress so a human watching can tell it's alive.
  if [ "$((i % 5))" -eq 0 ]; then
    printf '.'
  fi
done
echo ""

if [ "$ok" -ne 1 ]; then
  echo "[start] backend did not become healthy within $((attempts * interval))s"
  echo "[start] final container state:"
  $DC ps || true
  echo "[start] last 80 lines of backend logs:"
  $DC logs --tail=80 backend || true
  exit 1
fi

cat <<EOF

==============================================
  Unified Offline Operations Portal is up
==============================================
  Frontend:      http://localhost:${FRONTEND_PORT}
  Backend API:   http://localhost:${BACKEND_PORT}/api
  Health probe:  http://localhost:${BACKEND_PORT}/api/health

  Stop the stack with:  docker compose down -v

  Seeded login (see README for the full table):
    admin / AdminTest123!

==============================================
EOF
