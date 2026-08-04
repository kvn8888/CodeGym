#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
SPIKE_HOME="${OPENCODE_AZURE_SPIKE_HOME:-/tmp/opencode-spike-azure}"
SPIKE_WORKSPACE="${SPIKE_HOME}/workspace"
PORT_FILE="${SPIKE_HOME}/recorder.port"
TRANSCRIPT="${SCRIPT_DIR}/azure-transcript.jsonl"
EVENTS="${SCRIPT_DIR}/azure-events.jsonl"
STDERR_LOG="${SCRIPT_DIR}/azure-opencode.stderr.log"
EXIT_CODE_FILE="${SCRIPT_DIR}/azure-exit-code.txt"
OPENCODE_BIN="${SCRIPT_DIR}/.bin/opencode"
recorder_pid=""

cleanup() {
  if [[ -n "${recorder_pid}" ]]; then kill "${recorder_pid}" 2>/dev/null || true; fi
  if [[ -n "${recorder_pid}" ]]; then wait "${recorder_pid}" 2>/dev/null || true; fi
}
trap cleanup EXIT INT TERM

if [[ ! -x "${OPENCODE_BIN}" ]]; then
  echo "Missing pinned opencode binary; run ./download-opencode.sh." >&2
  exit 2
fi
mkdir -p "${SPIKE_HOME}/.config/opencode" "${SPIKE_HOME}/go-cache" "${SPIKE_WORKSPACE}"
if [[ ! -d "${SPIKE_HOME}/.config/opencode/node_modules/@opencode-ai/plugin" ]]; then
  cp -R "${SCRIPT_DIR}/.bin/home-template/.config/opencode/." "${SPIKE_HOME}/.config/opencode/"
fi
HOME="${SPIKE_HOME}" git init -q "${SPIKE_WORKSPACE}"
: >"${TRANSCRIPT}"
: >"${EVENTS}"
: >"${STDERR_LOG}"
: >"${EXIT_CODE_FILE}"
: >"${PORT_FILE}"

(
  cd "${SCRIPT_DIR}/recorder"
  HOME="${SPIKE_HOME}" GOCACHE="${SPIKE_HOME}/go-cache" go build -o "${SPIKE_HOME}/recorder" .
)
"${SPIKE_HOME}/recorder" --port 0 --port-file "${PORT_FILE}" --transcript "${TRANSCRIPT}" --responses-api &
recorder_pid=$!
for _ in $(seq 1 100); do
  [[ -s "${PORT_FILE}" ]] && break
  kill -0 "${recorder_pid}" 2>/dev/null || exit 1
  sleep 0.05
done
port="$(tr -d '\r\n' <"${PORT_FILE}")"

cat >"${SPIKE_HOME}/.config/opencode/opencode.json" <<JSON
{
  "\$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "share": "disabled",
  "model": "azure/spike-model",
  "provider": {
    "azure": {
      "options": {
        "baseURL": "http://127.0.0.1:${port}/openai/deployments",
        "apiKey": "sk-spike-not-a-real-key",
        "apiVersion": "2024-10-21",
        "useCompletionUrls": true
      },
      "models": {
        "spike-model": {
          "name": "Azure protocol spike",
          "options": { "useCompletionUrls": true }
        }
      }
    }
  },
  "permission": { "*": "deny" }
}
JSON

set +e
(
  cd "${SPIKE_WORKSPACE}"
  env -i \
  HOME="${SPIKE_HOME}" \
  PATH="${PATH}" \
  TMPDIR="/tmp" \
  OPENCODE_CONFIG="${SPIKE_HOME}/.config/opencode/opencode.json" \
  OPENCODE_DISABLE_AUTOUPDATE=1 \
  OPENCODE_DISABLE_MODELS_FETCH=1 \
  OPENCODE_DISABLE_DEFAULT_PLUGINS=1 \
  OPENCODE_DISABLE_LSP_DOWNLOAD=1 \
  OPENCODE_DISABLE_CLAUDE_CODE=1 \
  OPENCODE_AUTO_SHARE=0 \
  timeout --signal=TERM --kill-after=5s 60s \
    "${OPENCODE_BIN}" run --format json --auto -m azure/spike-model "Reply DONE."
) >"${EVENTS}" 2>"${STDERR_LOG}"
run_exit=$?
set -e
printf '%s\n' "${run_exit}" >"${EXIT_CODE_FILE}"
echo "azure probe exit code: ${run_exit}; requests: $(wc -l <"${TRANSCRIPT}" | tr -d ' ')"
exit "${run_exit}"
