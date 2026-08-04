#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
EVIDENCE_DIR="${SCRIPT_DIR}/isolation-evidence"
SESSION_TRANSCRIPT="${EVIDENCE_DIR}/session-transcript.jsonl"
SESSION_EVENTS="${EVIDENCE_DIR}/session-events.jsonl"
REACH_EVENTS="${EVIDENCE_DIR}/reach-events.jsonl"

for evidence_file in "${SESSION_TRANSCRIPT}" "${SESSION_EVENTS}" "${REACH_EVENTS}"; do
  if [[ ! -s "${evidence_file}" ]]; then
    echo "Missing evidence: ${evidence_file}; run ./run-isolation.sh first." >&2
    exit 2
  fi
done

jq -sr '
  ["invocation", "session_id", "reference_canary_in_body", "session_id_string_in_body"],
  (
    group_by(.label)[] |
    [
      .[0].label,
      .[0].headers["X-Session-Id"][0],
      ([.[].body | contains("CANARY_REFERENCE_9f3a2b")] | any),
      ([.[].body | test("ses_[A-Za-z0-9]+") ] | any)
    ]
  ) | @tsv
' "${SESSION_TRANSCRIPT}" >"${EVIDENCE_DIR}/session-analysis.tsv"

jq -sr '
  ["invocation", "session_id"],
  (
    group_by(.invocation)[] |
    [.[0].invocation, ([.[].sessionID | select(. != null)] | unique | join(","))]
  ) | @tsv
' "${SESSION_EVENTS}" >"${EVIDENCE_DIR}/event-session-ids.tsv"

jq -sr '
  ["invocation", "tool", "status", "command_exit", "output_or_error"],
  (
    .[] |
    select(.type == "tool_use") |
    [
      .invocation,
      .part.tool,
      .part.state.status,
      (.part.state.metadata.exit // ""),
      (.part.state.output // .part.state.error // "")
    ]
  ) | @tsv
' "${REACH_EVENTS}" >"${EVIDENCE_DIR}/reach-results.tsv"

echo "Wrote:"
printf '%s\n' \
  "${EVIDENCE_DIR}/session-analysis.tsv" \
  "${EVIDENCE_DIR}/event-session-ids.tsv" \
  "${EVIDENCE_DIR}/reach-results.tsv"
