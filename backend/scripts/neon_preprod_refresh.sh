#!/usr/bin/env bash
# Refresh the Neon preprod child branch from its production parent, then wait
# for CodeGym Staging to become healthy again.
#
# What it does:
#   1. POSTs a "reset from parent" restore to the Neon API for the preprod
#      branch (source = production branch, restored to head).
#   2. Polls the Neon API until the preprod branch reports current_state=ready.
#   3. Polls the staging /ready endpoint until it returns 2xx.
#
# Required env (see backend/.env.example; values live in Doppler `stg` only):
#   NEON_API_KEY, NEON_PROJECT_ID, NEON_PREPROD_BRANCH_ID, NEON_PROD_BRANCH_ID
#
# Optional env:
#   NEON_API_BASE_URL (default https://console.neon.tech/api/v2)
#   CODEGYM_STAGING_BASE_URL (default https://codegym-staging.onrender.com)
#   DRY_RUN (default false; when true, validate config and print the planned
#     action without calling Neon)
#   NEON_POLL_INTERVAL_SECONDS (default 15), NEON_POLL_TIMEOUT_SECONDS (default 900)
#   READY_POLL_INTERVAL_SECONDS (default 15), READY_POLL_TIMEOUT_SECONDS (default 600)
#   Poll interval/timeout values must be positive integers in seconds.
#
# Usage:
#   doppler run -p codegym -c stg -- ./scripts/neon_preprod_refresh.sh
#   DRY_RUN=true ./scripts/neon_preprod_refresh.sh
#
# Never prints API keys, connection strings, or other secret values.
set -euo pipefail

neon_api_base_url="${NEON_API_BASE_URL:-https://console.neon.tech/api/v2}"
neon_api_base_url="${neon_api_base_url%/}"

staging_base_url="${CODEGYM_STAGING_BASE_URL:-https://codegym-staging.onrender.com}"
staging_base_url="${staging_base_url%/}"

dry_run="${DRY_RUN:-false}"
neon_poll_interval="${NEON_POLL_INTERVAL_SECONDS:-15}"
neon_poll_timeout="${NEON_POLL_TIMEOUT_SECONDS:-900}"
ready_poll_interval="${READY_POLL_INTERVAL_SECONDS:-15}"
ready_poll_timeout="${READY_POLL_TIMEOUT_SECONDS:-600}"

log() {
  printf '%s\n' "$1"
}

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

# Validate required configuration before any destructive request.
missing=()
if [[ -z "${NEON_API_KEY:-}" ]]; then
  missing+=("NEON_API_KEY")
fi
if [[ -z "${NEON_PROJECT_ID:-}" ]]; then
  missing+=("NEON_PROJECT_ID")
fi
if [[ -z "${NEON_PREPROD_BRANCH_ID:-}" ]]; then
  missing+=("NEON_PREPROD_BRANCH_ID")
fi
if [[ -z "${NEON_PROD_BRANCH_ID:-}" ]]; then
  missing+=("NEON_PROD_BRANCH_ID")
fi
if [[ "${#missing[@]}" -gt 0 ]]; then
  fail "missing required env vars: ${missing[*]}"
fi

if [[ "${NEON_PREPROD_BRANCH_ID}" == "${NEON_PROD_BRANCH_ID}" ]]; then
  fail "refusing to run: NEON_PREPROD_BRANCH_ID and NEON_PROD_BRANCH_ID are equal (preprod branch id: ${NEON_PREPROD_BRANCH_ID})"
fi

# Poll intervals/timeouts feed shell arithmetic and sleep, so reject empty,
# non-numeric, or non-positive values before any destructive request.
require_positive_int() {
  local name="$1"
  local value="$2"
  case "${value}" in
    '' | *[!0-9]*)
      fail "${name} must be a positive integer in seconds."
      ;;
  esac
  if (( 10#${value} <= 0 )); then
    fail "${name} must be a positive integer in seconds."
  fi
}

require_positive_int "NEON_POLL_INTERVAL_SECONDS" "${neon_poll_interval}"
require_positive_int "NEON_POLL_TIMEOUT_SECONDS" "${neon_poll_timeout}"
require_positive_int "READY_POLL_INTERVAL_SECONDS" "${ready_poll_interval}"
require_positive_int "READY_POLL_TIMEOUT_SECONDS" "${ready_poll_timeout}"

restore_url="${neon_api_base_url}/projects/${NEON_PROJECT_ID}/branches/${NEON_PREPROD_BRANCH_ID}/restore"
branch_url="${neon_api_base_url}/projects/${NEON_PROJECT_ID}/branches/${NEON_PREPROD_BRANCH_ID}"

case "${dry_run}" in
  1 | true | yes | on)
    log "DRY RUN: configuration valid."
    log "DRY RUN: would POST ${restore_url} with source_branch_id=${NEON_PROD_BRANCH_ID} (production parent, head)."
    log "DRY RUN: would then poll ${branch_url} until current_state=ready."
    log "DRY RUN: would then poll ${staging_base_url}/ready until healthy."
    exit 0
    ;;
esac

log "Requesting Neon reset of preprod branch ${NEON_PREPROD_BRANCH_ID} from production branch ${NEON_PROD_BRANCH_ID}."

restore_body="$(python3 - "${NEON_PROD_BRANCH_ID}" <<'PY'
import json
import sys

print(json.dumps({"source_branch_id": sys.argv[1]}))
PY
)"

restore_response="$(curl -fsS --max-time 60 \
  -X POST "${restore_url}" \
  -H "accept: application/json" \
  -H "authorization: Bearer ${NEON_API_KEY}" \
  -H "content-type: application/json" \
  --data-binary "${restore_body}")" || fail "Neon restore request failed for preprod branch ${NEON_PREPROD_BRANCH_ID}."

operation_summary="$(python3 - "${restore_response}" <<'PY' 2>/dev/null || true
import json
import sys

try:
    payload = json.loads(sys.argv[1])
except (ValueError, IndexError):
    print("unparsed-restore-response")
    sys.exit(0)
operations = payload.get("operations", [])
states = [f"{op.get('action', '?')}={op.get('status', '?')}" for op in operations if isinstance(op, dict)]
print(", ".join(states) if states else "restore-accepted")
PY
)"
log "Neon restore accepted for preprod branch ${NEON_PREPROD_BRANCH_ID} (operations: ${operation_summary})."

log "Waiting for preprod branch ${NEON_PREPROD_BRANCH_ID} to become ready."
branch_deadline=$((SECONDS + neon_poll_timeout))
while true; do
  branch_response=""
  if branch_response="$(curl -fsS --max-time 30 \
    "${branch_url}" \
    -H "accept: application/json" \
    -H "authorization: Bearer ${NEON_API_KEY}")"; then
    branch_state="$(python3 - "${branch_response}" <<'PY' 2>/dev/null || true
import json
import sys

try:
    payload = json.loads(sys.argv[1])
except (ValueError, IndexError):
    print("unknown")
    sys.exit(0)
branch = payload.get("branch", {})
print(branch.get("current_state", "unknown") if isinstance(branch, dict) else "unknown")
PY
)"
    if [[ "${branch_state}" == "ready" ]]; then
      log "Preprod branch ${NEON_PREPROD_BRANCH_ID} is ready."
      break
    fi
    log "Preprod branch state is '${branch_state}'; waiting ${neon_poll_interval}s."
  else
    log "Neon branch poll request failed; waiting ${neon_poll_interval}s."
  fi
  if [[ "${SECONDS}" -ge "${branch_deadline}" ]]; then
    fail "timed out after ${neon_poll_timeout}s waiting for preprod branch ${NEON_PREPROD_BRANCH_ID} to become ready."
  fi
  sleep "${neon_poll_interval}"
done

log "Waiting for staging to become healthy at ${staging_base_url}/ready."
ready_deadline=$((SECONDS + ready_poll_timeout))
while true; do
  if curl -fsS --max-time 15 "${staging_base_url}/ready" >/dev/null; then
    log "Staging is healthy at ${staging_base_url}/ready."
    break
  fi
  log "Staging /ready not healthy yet; waiting ${ready_poll_interval}s."
  if [[ "${SECONDS}" -ge "${ready_deadline}" ]]; then
    fail "timed out after ${ready_poll_timeout}s waiting for staging /ready at ${staging_base_url}."
  fi
  sleep "${ready_poll_interval}"
done

log "Preprod refresh complete: branch ${NEON_PREPROD_BRANCH_ID} reset from production and staging is healthy."
