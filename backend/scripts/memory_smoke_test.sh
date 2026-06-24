#!/usr/bin/env bash
# Smoke-test the memory API against a running CodeGym backend.
#
# Prerequisites:
#   1. Backend running with Neon/Postgres (Doppler recommended):
#        cd backend
#        doppler run -p codegym -c dev -- go run ./cmd/server
#   2. Doppler dev config provides NEON_CONNECTION_STRING (or DATABASE_URL).
#
# Auth (pick one):
#   - Static token (when the server has CODEGYM_DEV_AUTH_TOKEN set):
#       export CODEGYM_SMOKE_AUTH_TOKEN="$CODEGYM_DEV_AUTH_TOKEN"
#   - Dev bearer shape (default when no static token is configured):
#       export CODEGYM_SMOKE_AUTH_TOKEN="dev:smoke-test:personal-smoke"
#
# Optional overrides:
#   CODEGYM_BASE_URL          Backend origin (default: http://127.0.0.1:8080)
#   CODEGYM_SMOKE_AUTH_TOKEN  Bearer token sent on protected routes
#   CODEGYM_SMOKE_TENANT_ID   X-CodeGym-Tenant-ID header (optional)
#
# Exits non-zero when readiness, event creation, listing, or profile fetch fails.

set -euo pipefail

BASE_URL="${CODEGYM_BASE_URL:-http://127.0.0.1:8080}"
AUTH_TOKEN="${CODEGYM_SMOKE_AUTH_TOKEN:-${CODEGYM_DEV_AUTH_TOKEN:-dev:smoke-test:personal-smoke}}"
TENANT_HEADER=()
if [[ -n "${CODEGYM_SMOKE_TENANT_ID:-}" ]]; then
  TENANT_HEADER=(-H "X-CodeGym-Tenant-ID: ${CODEGYM_SMOKE_TENANT_ID}")
fi

MARKER="smoke-$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
API_PREFIX="${BASE_URL%/}/api/v1"

fail() {
  echo "memory smoke test: $*" >&2
  exit 1
}

request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local expected="${4:-200}"
  local tmp
  tmp="$(mktemp)"
  local status

  if [[ -n "$body" ]]; then
    status="$(
      curl -sS -o "$tmp" -w "%{http_code}" -X "$method" \
        -H "Authorization: Bearer ${AUTH_TOKEN}" \
        -H "Content-Type: application/json" \
        "${TENANT_HEADER[@]}" \
        --data "$body" \
        "${API_PREFIX}${path}"
    )"
  else
    status="$(
      curl -sS -o "$tmp" -w "%{http_code}" -X "$method" \
        -H "Authorization: Bearer ${AUTH_TOKEN}" \
        "${TENANT_HEADER[@]}" \
        "${API_PREFIX}${path}"
    )"
  fi

  if [[ "$status" != "$expected" ]]; then
    echo "response body:" >&2
    cat "$tmp" >&2 || true
    rm -f "$tmp"
    fail "${method} ${path} expected HTTP ${expected}, got ${status}"
  fi

  cat "$tmp"
  rm -f "$tmp"
}

echo "memory smoke test: checking ${BASE_URL}/ready"
ready_status="$(
  curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL%/}/ready" || true
)"
if [[ "$ready_status" != "200" ]]; then
  fail "/ready returned HTTP ${ready_status}; start the backend with Doppler + Neon first"
fi

event_body="$(cat <<EOF
{
  "source": "smoke_test",
  "type": "memory_api_checked",
  "summary": "Memory smoke test marker ${MARKER}",
  "payload": {
    "marker": "${MARKER}",
    "check": "memory_smoke_test"
  }
}
EOF
)"

echo "memory smoke test: recording event"
created="$(request POST /memory/events "$event_body" 201)"
event_id="$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"
if [[ -z "$event_id" ]]; then
  fail "created event response did not include data.id"
fi

echo "memory smoke test: listing events"
listed="$(request GET /memory/events)"
if ! printf '%s' "$listed" | python3 -c "import json,sys; data=json.load(sys.stdin)['data']; marker='${MARKER}'; sys.exit(0 if any(evt.get('id')=='${event_id}' or marker in evt.get('summary','') for evt in data) else 1)"; then
  fail "created event ${event_id} was not found in GET /memory/events"
fi

echo "memory smoke test: fetching profile"
profile="$(request GET /memory/profile)"
summary="$(printf '%s' "$profile" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["summary"])')"
if [[ -z "$summary" ]]; then
  fail "GET /memory/profile returned an empty summary"
fi

echo "memory smoke test: ok (event ${event_id})"