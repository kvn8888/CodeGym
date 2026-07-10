#!/usr/bin/env bash
set -euo pipefail

api_base_url="${CODEGYM_API_BASE_URL:-http://127.0.0.1:8080}"
api_base_url="${api_base_url%/}"

dev_user_id="${CODEGYM_DEV_USER_ID:-smoke-user}"
dev_workspace_id="${CODEGYM_DEV_WORKSPACE_ID:-personal-smoke}"

if [[ -n "${CODEGYM_DEV_AUTH_TOKEN:-}" ]]; then
  bearer_token="${CODEGYM_DEV_AUTH_TOKEN}"
else
  bearer_token="dev:${dev_user_id}:${dev_workspace_id}"
fi

run_id="${CODEGYM_SMOKE_RUN_ID:-memory-smoke-$(date -u +%Y%m%dT%H%M%SZ)-$$}"

request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"

  if [[ -n "${body}" ]]; then
    curl -fsS \
      -X "${method}" \
      "${api_base_url}${path}" \
      -H "Authorization: Bearer ${bearer_token}" \
      -H "Content-Type: application/json" \
      --data-binary "${body}"
  else
    curl -fsS \
      -X "${method}" \
      "${api_base_url}${path}" \
      -H "Authorization: Bearer ${bearer_token}"
  fi
}

printf 'Checking backend readiness at %s\n' "${api_base_url}"
curl -fsS "${api_base_url}/ready" >/dev/null

event_body="$(python3 - "${run_id}" <<'PY'
import json
import sys

run_id = sys.argv[1]
print(json.dumps({
    "source": "system",
    "type": "memory_api_checked",
    "summary": "Backend smoke test verified memory event and profile APIs.",
    "payload": {
        "script": "backend-memory-smoke",
        "run_id": run_id,
        "schema_version": 1,
    },
}))
PY
)"

create_response="$(request POST /api/v1/memory/events "${event_body}")"
event_id="$(python3 - "${create_response}" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
event = payload.get("data")
if not isinstance(event, dict) or not event.get("id"):
    raise SystemExit("POST /api/v1/memory/events did not return data.id")
print(event["id"])
PY
)"

list_response="$(request GET /api/v1/memory/events)"
python3 - "${list_response}" "${event_id}" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
event_id = sys.argv[2]
events = payload.get("data")
if not isinstance(events, list):
    raise SystemExit("GET /api/v1/memory/events did not return a data array")
if not any(isinstance(event, dict) and event.get("id") == event_id for event in events):
    raise SystemExit(f"Created memory event {event_id} was not returned by list events")
PY

profile_response="$(request GET /api/v1/memory/profile)"
python3 - "${profile_response}" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
profile = payload.get("data")
if not isinstance(profile, dict):
    raise SystemExit("GET /api/v1/memory/profile did not return an object in data")
if not isinstance(profile.get("summary"), str):
    raise SystemExit("Memory profile response is missing summary")
PY

printf 'Memory smoke passed: created and listed %s, then fetched profile.\n' "${event_id}"
