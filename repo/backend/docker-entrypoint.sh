#!/bin/sh
# Backend entrypoint:
#   1. Wait for Postgres to accept connections.
#   2. Apply migrations/0001_init.sql once (idempotent — detected via
#      presence of the `users` table).
#   3. Run the seed binary (idempotent — skips rows that already exist).
#   4. Exec the server.
#
# All steps fail loud on an unexpected error so the container restart
# policy reports a clear failure.
set -eu

if [ -z "${DATABASE_URL:-}" ]; then
  echo "DATABASE_URL is required"
  exit 1
fi

# Parse host/port out of the DSN with a tiny sed so we can use pg_isready
# without pulling in a heavier parser.
host=$(echo "$DATABASE_URL" | sed -E 's#.*@([^:/]+).*#\1#')
port=$(echo "$DATABASE_URL" | sed -E 's#.*@[^:]+:([0-9]+)/.*#\1#')

echo "[entrypoint] waiting for postgres at ${host}:${port} …"
i=0
until pg_isready -h "$host" -p "$port" -q; do
  i=$((i+1))
  if [ "$i" -gt 60 ]; then
    echo "[entrypoint] postgres did not become ready within 60s"
    exit 1
  fi
  sleep 1
done
echo "[entrypoint] postgres is up"

# Apply the migration only on the first run. We detect the first run by
# the absence of the `users` table; PG_APPLIED=1 forces a reapply for
# dev workflows.
if [ "${PG_APPLY:-auto}" = "force" ] || ! psql "$DATABASE_URL" -tAc \
      "SELECT 1 FROM information_schema.tables WHERE table_name='users'" | grep -q 1; then
  echo "[entrypoint] applying migrations/0001_init.sql"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f /app/migrations/0001_init.sql
else
  echo "[entrypoint] schema already present; skipping migration"
fi

# Seed the users + demo region. Idempotent: SeedDeployment skips the
# admin row when it already exists, SeedDemoUsers skips each user
# individually, ReplaceRegions always resets the demo polygon.
echo "[entrypoint] running seed"
/app/seed

echo "[entrypoint] starting server on ${LISTEN_ADDR:-:8080}"
exec /app/server
