#!/usr/bin/env bash
# Bring up the full stack (db + backend + frontend) in the background
# and print the URL a human operator should open. Safe to re-run; it
# rebuilds images, applies migrations, and reseeds idempotently.
#
# Exit codes:
#   0  stack is up and backend /api/health returned 200
#   1  docker compose or the health-check never succeeded within the
#      configured window
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$here"

# Copy .env on first run so every default value from .env.example
# applies. We never overwrite an existing .env.
if [ ! -f .env ]; then
  cp .env.example .env
  echo "[start] copied .env.example -> .env"
fi

# Prefer `docker compose` (plugin) but fall back to `docker-compose`
# (legacy standalone) so either install layout works.
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
else
  DC="docker-compose"
fi

echo "[start] building and starting the stack…"
$DC up --build -d

# Work out where the backend is exposed so we can poll /api/health.
# shellcheck disable=SC1091
set -a; . ./.env; set +a
BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"

echo "[start] waiting for backend at http://localhost:${BACKEND_PORT}/api/health …"
ok=0
for i in $(seq 1 60); do
  if curl -fsS "http://localhost:${BACKEND_PORT}/api/health" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 2
done

if [ "$ok" -ne 1 ]; then
  echo "[start] backend did not become healthy within 120s"
  echo "[start] last 50 lines of backend logs:"
  $DC logs --tail=50 backend || true
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
