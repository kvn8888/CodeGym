#!/usr/bin/env bash
#
# Local development launcher for the #191 browser practice sample with
# DURABLE saves across backend restarts.
#
# What it does:
#   1. Ensures a dedicated local Postgres cluster exists under a persistent
#      data directory (reused across restarts, never a temp dir).
#   2. Starts that cluster if it is not running and creates the
#      codegym_practice_dev database if missing.
#   3. Starts the backend with DATABASE_URL pointed at that database and
#      CODEGYM_PRACTICE_SAMPLE=true, so /practice saves persist.
#
# What it never touches:
#   - The default :5432 Postgres, any Neon/SHARED/production database, or
#     existing authentication settings. Cluster port 55434, database
#     codegym_practice_dev, and local trust auth are dedicated to this
#     launcher. Stop the database with:
#       pg_ctl -D "${CODEGYM_DEV_DATA_DIR:-$HOME/.codegym-dev}/pg-practice" stop
#
# Usage (from the backend directory):
#   ./scripts/dev-practice-sample.sh
#
# Environment overrides:
#   CODEGYM_DEV_DATA_DIR  persistent parent dir (default $HOME/.codegym-dev)
#   CODEGYM_DEV_PG_PORT   cluster port (default 55434)
#   CODEGYM_PORT          backend listen port (default 8080)
#   PG_BIN                directory holding initdb/pg_ctl/psql (default: PATH,
#                         then /Library/PostgreSQL/17/bin)
set -euo pipefail
unset NEON_CONNECTION_STRING

PORT="${CODEGYM_DEV_PG_PORT:-55434}"
DATADIR="${CODEGYM_DEV_DATA_DIR:-$HOME/.codegym-dev}/pg-practice"
DB="codegym_practice_dev"

resolve_pg_bin() {
  # A client-only libpq directory is not enough: the postgres server
  # binary must be present.
  if [ -n "${PG_BIN:-}" ] && [ -x "$PG_BIN/postgres" ]; then
    echo "$PG_BIN"
    return 0
  fi
  for candidate in "$(command -v initdb 2>/dev/null && dirname "$(command -v initdb)")" "/Library/PostgreSQL/17/bin"; do
    if [ -n "$candidate" ] && [ -x "$candidate/postgres" ] && [ -x "$candidate/pg_ctl" ]; then
      echo "$candidate"
      return 0
    fi
  done
  echo "dev-practice-sample: postgres server binaries not found; set PG_BIN to a full Postgres bin directory" >&2
  return 1
}

PG_BIN_DIR="$(resolve_pg_bin)"
if [ -n "$PG_BIN_DIR" ]; then
  export PATH="$PG_BIN_DIR:$PATH"
fi

if [ ! -f "$DATADIR/PG_VERSION" ]; then
  mkdir -p "$(dirname "$DATADIR")"
  initdb -D "$DATADIR" -U postgres --auth=trust -E UTF8 >/dev/null
  echo "dev-practice-sample: initialized dedicated cluster at $DATADIR"
fi

if ! pg_ctl -D "$DATADIR" status >/dev/null 2>&1; then
  pg_ctl -D "$DATADIR" -l "$DATADIR/server.log" -o "-p $PORT -k /tmp" start >/dev/null
  echo "dev-practice-sample: started dedicated Postgres on 127.0.0.1:$PORT"
fi

if ! psql -h /tmp -p "$PORT" -U postgres -d "$DB" -c 'select 1' >/dev/null 2>&1; then
  createdb -h /tmp -p "$PORT" -U postgres "$DB"
  echo "dev-practice-sample: created database $DB"
fi

export DATABASE_URL="postgres://postgres@127.0.0.1:$PORT/$DB?sslmode=disable"
export CODEGYM_PRACTICE_SAMPLE=true
export CODEGYM_PORT="${CODEGYM_PORT:-8080}"
# Explicit so the documented mock-free Vite origin (port 5173) works without
# the user discovering CORS after a failure. Override for a custom origin.
export CODEGYM_CORS_ALLOWED_ORIGINS="${CODEGYM_CORS_ALLOWED_ORIGINS:-http://localhost:5173,http://127.0.0.1:5173}"

# Refuse to start when something already answers on the backend port: a
# stale server would otherwise serve this launch's traffic while the new
# process exits on bind failure.
if (echo >/dev/tcp/127.0.0.1/"$CODEGYM_PORT") 2>/dev/null; then
  echo "dev-practice-sample: refusing to start; 127.0.0.1:$CODEGYM_PORT already answers (stop the stale backend first)" >&2
  exit 1
fi

echo "dev-practice-sample: durable saves via dedicated local Postgres ($DB on 127.0.0.1:$PORT)"
exec go run ./cmd/server
