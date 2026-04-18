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

# If .env still carries the historical placeholder ENC_KEYS value,
# auto-rotate it in place. Anyone with repo access knows the old
# constant, so running with it would make at-rest encryption
# effectively reversible — but rather than bouncing the operator to a
# separate `keygen` invocation, we just mint a fresh 32-byte key here
# and rewrite the line. The user's data on a first run is empty, so
# swapping the key has no recovery cost.
placeholder="1:0101010101010101010101010101010101010101010101010101010101010101"
enc_line="$(grep -E '^ENC_KEYS=' .env || true)"
if [ "$enc_line" = "ENC_KEYS=${placeholder}" ]; then
  echo "[start] ENC_KEYS held the historical placeholder — rotating to a fresh random key."
  new_hex=""
  if command -v openssl >/dev/null 2>&1; then
    new_hex="$(openssl rand -hex 32)"
  elif [ -r /dev/urandom ]; then
    # Portable fallback: 32 bytes from /dev/urandom, hex-encoded.
    new_hex="$(LC_ALL=C od -An -vN32 -tx1 < /dev/urandom | tr -d ' \n')"
  fi
  if [ -z "$new_hex" ] || [ "${#new_hex}" -ne 64 ]; then
    echo "[start] could not generate a 32-byte key (no openssl and /dev/urandom unreadable)."
    echo "[start] Generate one manually with:  docker compose run --rm --entrypoint /app/keygen backend"
    exit 1
  fi
  # Rewrite the ENC_KEYS line in .env without touching anything else.
  # A sibling .env.bak preserves the previous value in case an
  # operator wants to roll back.
  cp .env .env.bak
  # BSD and GNU sed both accept `-i ''` vs `-i` differently; use a
  # portable two-step rewrite instead.
  awk -v newline="ENC_KEYS=1:${new_hex}" '
    /^ENC_KEYS=/ { print newline; next }
    { print }
  ' .env > .env.tmp && mv .env.tmp .env
  echo "[start] wrote a fresh ENC_KEYS to .env (previous value backed up to .env.bak)."
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

# Preflight: confirm the host ports we plan to publish are free. Docker
# will otherwise partially recreate the stack and then fail on the
# `bind` step, leaving containers in a "Recreated" limbo that's hard
# to read. A single clear error up front is much friendlier.
port_in_use() {
  local port="$1"
  # lsof is on macOS by default and most Linux dev boxes. The TCP
  # probe fallback uses /dev/tcp, which bash implements natively.
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1 && return 0 || return 1
  fi
  (exec 3<>/dev/tcp/127.0.0.1/"${port}") 2>/dev/null && { exec 3<&- 3>&-; return 0; } || return 1
}

describe_port_holder() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print "        "$1" (pid "$2")"}' | sort -u
  fi
}

# Pick the next free port starting from the requested value, walking
# up to 50 candidates. Prints the chosen value to stdout; returns
# non-zero if nothing in the range was free.
find_free_port() {
  local start="$1"
  local p="$start"
  local tries=0
  while [ "$tries" -lt 50 ]; do
    if ! port_in_use "$p"; then
      echo "$p"
      return 0
    fi
    p=$((p + 1))
    tries=$((tries + 1))
  done
  return 1
}

# Overwrite a KEY=VALUE line in .env, preserving everything else.
rewrite_env_var() {
  local key="$1" val="$2"
  if grep -qE "^${key}=" .env; then
    awk -v k="${key}" -v v="${val}" '
      $0 ~ "^"k"=" { print k"="v; next }
      { print }
    ' .env > .env.tmp && mv .env.tmp .env
  else
    printf '%s=%s\n' "$key" "$val" >> .env
  fi
}

# First-pass collision check. If a Docker-owned leftover is squatting
# on one of our ports, try `compose down` once to free it. Anything
# still bound is someone else's process (another compose project, a
# host-native server, a running dev tool) — we just step around them
# by picking the next free port and writing it back to .env.
for attempt in 1 2; do
  collision=0
  docker_holder=0
  for port_var in BACKEND_PORT FRONTEND_PORT; do
    port_val="$(eval echo "\${$port_var}")"
    [ -z "$port_val" ] && continue
    if port_in_use "$port_val"; then
      collision=1
      holder="$(describe_port_holder "$port_val")"
      case "$holder" in
        *com.docke*|*docker*) docker_holder=1 ;;
      esac
    fi
  done

  if [ "$collision" -eq 0 ]; then
    break
  fi

  if [ "$attempt" -eq 1 ] && [ "$docker_holder" -eq 1 ]; then
    echo "[start] a Docker process is squatting on one of our ports — running compose down to clear any leftover from a previous run."
    $DC down --remove-orphans >/dev/null 2>&1 || true
    continue
  fi

  # Second pass — remaining collisions are not ours. Hop to the next
  # free port and persist it so future runs are stable.
  for port_var in BACKEND_PORT FRONTEND_PORT; do
    port_val="$(eval echo "\${$port_var}")"
    [ -z "$port_val" ] && continue
    if port_in_use "$port_val"; then
      holder="$(describe_port_holder "$port_val")"
      new_port="$(find_free_port "$((port_val + 1))" || true)"
      if [ -z "$new_port" ]; then
        echo "[start] ${port_var}=${port_val} is in use and no free port found in the next 50; stop the holder and re-run."
        [ -n "$holder" ] && echo "        holder(s): $(echo "$holder" | tr -s '\n ' ' ')"
        exit 1
      fi
      echo "[start] ${port_var}=${port_val} is in use; rotating to ${new_port} and writing it to .env."
      [ -n "$holder" ] && echo "        previous holder: $(echo "$holder" | tr -s '\n ' ' ')"
      rewrite_env_var "$port_var" "$new_port"
      eval "$port_var=${new_port}"
    fi
  done
  break
done

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
  echo "[start] if the error mentioned 'port is already allocated', stop the process on that port"
  echo "        or change BACKEND_PORT / FRONTEND_PORT / DB_PORT in .env and re-run."
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
