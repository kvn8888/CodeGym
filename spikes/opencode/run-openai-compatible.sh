#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
RECORDER_DIR="${SCRIPT_DIR}/recorder"
SPIKE_HOME="${OPENCODE_SPIKE_HOME:-/tmp/opencode-spike}"
SPIKE_WORKSPACE="${SPIKE_HOME}/workspace"
PORT_FILE="${SPIKE_HOME}/recorder.port"
TRANSCRIPT="${SCRIPT_DIR}/transcript.jsonl"
EVENTS="${SCRIPT_DIR}/events.jsonl"
TOOL_LOG="${SCRIPT_DIR}/tool-invocations.jsonl"
EXIT_CODE_FILE="${SCRIPT_DIR}/exit-code.txt"
EGRESS_LOG="${SCRIPT_DIR}/egress-lsof.txt"
STDERR_LOG="${SCRIPT_DIR}/opencode.stderr.log"
RECORDER_LOG="${SCRIPT_DIR}/recorder.log"
OPENCODE_BIN="${SCRIPT_DIR}/.bin/opencode"
DUMMY_KEY="sk-spike-not-a-real-key"

recorder_pid=""
monitor_pid=""
cleanup() {
  if [[ -n "${monitor_pid}" ]]; then kill "${monitor_pid}" 2>/dev/null || true; fi
  if [[ -n "${recorder_pid}" ]]; then kill "${recorder_pid}" 2>/dev/null || true; fi
  if [[ -n "${monitor_pid}" ]]; then wait "${monitor_pid}" 2>/dev/null || true; fi
  if [[ -n "${recorder_pid}" ]]; then wait "${recorder_pid}" 2>/dev/null || true; fi
}
trap cleanup EXIT INT TERM

if [[ ! -x "${OPENCODE_BIN}" ]]; then
  echo "Missing ${OPENCODE_BIN}; run ./download-opencode.sh first." >&2
  exit 2
fi
if ! command -v timeout >/dev/null 2>&1; then
  echo "GNU timeout is required (on macOS: brew install coreutils)." >&2
  exit 2
fi

# Every persistent opencode path resolves beneath this disposable HOME. The Go
# cache is also redirected there so the recorder build does not touch ~/Library.
mkdir -p "${SPIKE_HOME}/.config/opencode" "${SPIKE_HOME}/go-cache" "${SPIKE_WORKSPACE}/.opencode/tools"
if [[ ! -d "${SPIKE_HOME}/.config/opencode/node_modules/@opencode-ai/plugin" ]]; then
  if [[ ! -d "${SCRIPT_DIR}/.bin/home-template/.config/opencode/node_modules/@opencode-ai/plugin" ]]; then
    echo "Missing prebuilt custom-tool runtime; rerun ./download-opencode.sh." >&2
    exit 2
  fi
  cp -R "${SCRIPT_DIR}/.bin/home-template/.config/opencode/." "${SPIKE_HOME}/.config/opencode/"
fi
if [[ ! -d "${SPIKE_WORKSPACE}/.opencode/node_modules/@opencode-ai/plugin" ]]; then
  cp -R "${SCRIPT_DIR}/.bin/home-template/.config/opencode/." "${SPIKE_WORKSPACE}/.opencode/"
fi
cp "${SCRIPT_DIR}/.opencode/tools/report_progress.ts" "${SPIKE_WORKSPACE}/.opencode/tools/report_progress.ts"
HOME="${SPIKE_HOME}" git init -q "${SPIKE_WORKSPACE}"
: >"${TRANSCRIPT}"
: >"${EVENTS}"
: >"${TOOL_LOG}"
: >"${EXIT_CODE_FILE}"
: >"${EGRESS_LOG}"
: >"${STDERR_LOG}"
: >"${RECORDER_LOG}"
: >"${PORT_FILE}"

(
  cd "${RECORDER_DIR}"
  HOME="${SPIKE_HOME}" GOCACHE="${SPIKE_HOME}/go-cache" go build -o "${SPIKE_HOME}/recorder" .
)
"${SPIKE_HOME}/recorder" \
  --port 0 \
  --port-file "${PORT_FILE}" \
  --transcript "${TRANSCRIPT}" \
  --scripted-tool-call \
  --responses-api \
  >"${RECORDER_LOG}" 2>&1 &
recorder_pid=$!

for _ in $(seq 1 100); do
  if [[ -s "${PORT_FILE}" ]]; then break; fi
  if ! kill -0 "${recorder_pid}" 2>/dev/null; then
    echo "Recorder exited before becoming ready:" >&2
    sed -n '1,120p' "${RECORDER_LOG}" >&2
    exit 1
  fi
  sleep 0.05
done
if [[ ! -s "${PORT_FILE}" ]]; then
  echo "Recorder did not become ready within 5 seconds." >&2
  exit 1
fi
port="$(tr -d '\r\n' <"${PORT_FILE}")"

cat >"${SPIKE_HOME}/.config/opencode/opencode.json" <<JSON
{
  "\$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "share": "disabled",
  "model": "codegym-relay/spike-model",
  "provider": {
    "codegym-relay": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "CodeGym local recording relay",
      "options": {
        "baseURL": "http://127.0.0.1:${port}/v1",
        "apiKey": "${DUMMY_KEY}"
      },
      "models": {
        "spike-model": {
          "name": "Protocol spike model",
          "limit": { "context": 32768, "output": 4096 }
        }
      }
    }
  },
  "permission": {
    "*": "deny",
    "report_progress": "allow"
  }
}
JSON

# Best-effort socket sampling. This catches established TCP/UDP sockets owned by
# a process whose command is opencode; it is not a packet capture or DNS audit.
(
  while kill -0 "$$" 2>/dev/null; do
    lsof -nP -a -c opencode -i 2>/dev/null >>"${EGRESS_LOG}" || true
    sleep 0.05
  done
) &
monitor_pid=$!

echo "Recorder: http://127.0.0.1:${port}/v1"
echo "Running opencode $(${OPENCODE_BIN} --version) with isolated HOME=${SPIKE_HOME}"

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
  OPENCODE_ENABLE_EXA=0 \
  OPENCODE_AUTO_SHARE=0 \
  OPENCODE_SPIKE_TOOL_LOG="${TOOL_LOG}" \
  timeout --signal=TERM --kill-after=5s 60s \
    "${OPENCODE_BIN}" run --format json --auto -m codegym-relay/spike-model \
    "Call the report_progress tool with step_id 'environment' and label 'Setting up', then reply DONE."
) >"${EVENTS}" 2>"${STDERR_LOG}"
run_exit=$?
set -e

printf '%s\n' "${run_exit}" >"${EXIT_CODE_FILE}"
echo "opencode exit code: ${run_exit}"
echo "Transcript requests: $(wc -l <"${TRANSCRIPT}" | tr -d ' ')"
echo "Events: $(wc -l <"${EVENTS}" | tr -d ' ')"
echo "Tool invocations: $(wc -l <"${TOOL_LOG}" | tr -d ' ')"
exit "${run_exit}"
