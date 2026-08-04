#!/usr/bin/env bash
set -euo pipefail

ISOLATION_SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ISOLATION_ROOT="${OPENCODE_ISOLATION_ROOT:-/tmp/opencode-isolation-$$}"
ISOLATION_HOME="${ISOLATION_ROOT}/home"
ISOLATION_TREE="${ISOLATION_ROOT}/tree"
ISOLATION_SESSION_WORKSPACE="${ISOLATION_ROOT}/session-workspace"
ISOLATION_EVIDENCE_DIR="${ISOLATION_SCRIPT_DIR}/isolation-evidence"
ISOLATION_OPENCODE_BIN="${ISOLATION_SCRIPT_DIR}/.bin/opencode"
ISOLATION_RECORDER_BIN="${ISOLATION_ROOT}/recorder"
ISOLATION_CONFIG="${ISOLATION_HOME}/.config/opencode/opencode.json"
ISOLATION_PORT_FILE="${ISOLATION_ROOT}/recorder.port"
ISOLATION_DUMMY_KEY="sk-isolation-not-a-real-key"
ISOLATION_TIMEOUT_SECONDS="${OPENCODE_ISOLATION_TIMEOUT_SECONDS:-45}"

isolation_recorder_pid=""
isolation_port=""

isolation_stop_recorder() {
  if [[ -n "${isolation_recorder_pid}" ]]; then
    kill "${isolation_recorder_pid}" 2>/dev/null || true
    wait "${isolation_recorder_pid}" 2>/dev/null || true
    isolation_recorder_pid=""
  fi
}

isolation_cleanup() {
  isolation_stop_recorder
}

isolation_require_tools() {
  local tool_name
  for tool_name in go jq rg sqlite3 timeout; do
    if ! command -v "${tool_name}" >/dev/null 2>&1; then
      echo "Missing required tool: ${tool_name}" >&2
      exit 2
    fi
  done
  if [[ ! -x "${ISOLATION_OPENCODE_BIN}" ]]; then
    echo "Missing pinned binary ${ISOLATION_OPENCODE_BIN}; run ./download-opencode.sh first." >&2
    exit 2
  fi
}

isolation_prepare_scratch() {
  mkdir -p \
    "${ISOLATION_HOME}/.config/opencode" \
    "${ISOLATION_HOME}/.local/state/opencode" \
    "${ISOLATION_HOME}/go-cache" \
    "${ISOLATION_TREE}/reference" \
    "${ISOLATION_TREE}/tests" \
    "${ISOLATION_TREE}/spec" \
    "${ISOLATION_SESSION_WORKSPACE}" \
    "${ISOLATION_EVIDENCE_DIR}" \
    "${ISOLATION_ROOT}/events" \
    "${ISOLATION_ROOT}/logs"

  if [[ ! -d "${ISOLATION_SCRIPT_DIR}/.bin/home-template/.config/opencode/node_modules/@opencode-ai/plugin" ]]; then
    echo "Missing prebuilt opencode runtime; rerun ./download-opencode.sh." >&2
    exit 2
  fi
  if [[ ! -d "${ISOLATION_HOME}/.config/opencode/node_modules/@opencode-ai/plugin" ]]; then
    cp -R "${ISOLATION_SCRIPT_DIR}/.bin/home-template/.config/opencode/." "${ISOLATION_HOME}/.config/opencode/"
  fi

  printf '%s\n' 'CANARY_REFERENCE_9f3a2b' >"${ISOLATION_TREE}/reference/solution.py"
  printf '%s\n' 'CANARY_TESTS_7c1d4e' >"${ISOLATION_TREE}/tests/test_solution.py"
  printf '%s\n' 'CANARY_SPEC_4b8e01' >"${ISOLATION_TREE}/spec/spec.md"
  printf '%s\n' 'CANARY_HOME_2d5f6a' >"${ISOLATION_HOME}/.local/state/opencode/isolation-canary.txt"

  HOME="${ISOLATION_HOME}" git init -q "${ISOLATION_TREE}"
  HOME="${ISOLATION_HOME}" git init -q "${ISOLATION_SESSION_WORKSPACE}"

  (
    cd "${ISOLATION_SCRIPT_DIR}/recorder"
    HOME="${ISOLATION_HOME}" GOCACHE="${ISOLATION_HOME}/go-cache" go build -o "${ISOLATION_RECORDER_BIN}" .
  )
}

isolation_write_config() {
  local recorder_port="$1"
  local policy="$2"
  local tree_real
  tree_real="$(CDPATH= cd -- "${ISOLATION_TREE}" && pwd -P)"

  if [[ "${policy}" == "permissive" ]]; then
    jq -n \
      --arg base_url "http://127.0.0.1:${recorder_port}/v1" \
      --arg api_key "${ISOLATION_DUMMY_KEY}" \
      '{
        "$schema": "https://opencode.ai/config.json",
        "autoupdate": false,
        "share": "disabled",
        "model": "codegym-relay/spike-model",
        "provider": {
          "codegym-relay": {
            "npm": "@ai-sdk/openai-compatible",
            "name": "CodeGym local isolation recorder",
            "options": {"baseURL": $base_url, "apiKey": $api_key},
            "models": {
              "spike-model": {
                "name": "Isolation spike model",
                "limit": {"context": 32768, "output": 4096}
              }
            }
          }
        }
      }' >"${ISOLATION_CONFIG}"
    return
  fi

  jq -n \
    --arg base_url "http://127.0.0.1:${recorder_port}/v1" \
    --arg api_key "${ISOLATION_DUMMY_KEY}" \
    --arg spec_glob "${tree_real}/spec/*" \
    '{
      "$schema": "https://opencode.ai/config.json",
      "autoupdate": false,
      "share": "disabled",
      "model": "codegym-relay/spike-model",
      "provider": {
        "codegym-relay": {
          "npm": "@ai-sdk/openai-compatible",
          "name": "CodeGym local isolation recorder",
          "options": {"baseURL": $base_url, "apiKey": $api_key},
          "models": {
            "spike-model": {
              "name": "Isolation spike model",
              "limit": {"context": 32768, "output": 4096}
            }
          }
        }
      },
      "agent": {
        "tests-isolated": {
          "description": "Generate tests from the shared spec without reading reference artifacts",
          "mode": "primary",
          "permission": {
            "*": "deny",
            "bash": "deny",
            "edit": "deny",
            "glob": "deny",
            "grep": "deny",
            "task": "deny",
            "webfetch": "deny",
            "websearch": "deny",
            "read": {
              "*": "deny",
              "test_solution.py": "allow",
              "spec/spec.md": "allow",
              "../spec/spec.md": "allow"
            },
            "external_directory": {
              "*": "deny",
              ($spec_glob): "allow"
            }
          }
        }
      }
    }' >"${ISOLATION_CONFIG}"
}

isolation_start_recorder() {
  local label="$1"
  local transcript_file="$2"
  local tool_name="${3:-}"
  local tool_arguments="{}"
  local recorder_log="${ISOLATION_ROOT}/logs/${label}-recorder.log"
  local -a recorder_args

  if [[ "$#" -ge 4 ]]; then
    tool_arguments="$4"
  fi

  : >"${ISOLATION_PORT_FILE}"
  recorder_args=(
    --port 0
    --port-file "${ISOLATION_PORT_FILE}"
    --transcript "${transcript_file}"
    --label "${label}"
    --responses-api
  )
  if [[ -n "${tool_name}" ]]; then
    recorder_args+=(
      --tool-call-id "call_isolation_${label}"
      --tool-call-name "${tool_name}"
      --tool-call-arguments "${tool_arguments}"
    )
  fi

  "${ISOLATION_RECORDER_BIN}" "${recorder_args[@]}" >"${recorder_log}" 2>&1 &
  isolation_recorder_pid=$!

  local ready_attempt
  for ready_attempt in $(seq 1 100); do
    if [[ -s "${ISOLATION_PORT_FILE}" ]]; then
      break
    fi
    if ! kill -0 "${isolation_recorder_pid}" 2>/dev/null; then
      echo "Recorder failed for ${label}:" >&2
      sed -n '1,120p' "${recorder_log}" >&2
      return 1
    fi
    sleep 0.05
  done
  if [[ ! -s "${ISOLATION_PORT_FILE}" ]]; then
    echo "Recorder did not become ready for ${label}." >&2
    return 1
  fi
  isolation_port="$(tr -d '\r\n' <"${ISOLATION_PORT_FILE}")"
}

isolation_run_opencode() {
  local label="$1"
  local policy="$2"
  local working_dir="$3"
  local events_file="$4"
  local stderr_file="$5"
  local prompt="$6"
  shift 6
  local -a run_args=(
    run
    --format json
    --auto
    --dir "${working_dir}"
    -m codegym-relay/spike-model
  )

  if [[ "${policy}" == "locked" ]]; then
    run_args+=(--agent tests-isolated)
  fi
  if [[ "$#" -gt 0 ]]; then
    run_args+=("$@")
  fi
  run_args+=("${prompt}")

  set +e
  env -i \
    HOME="${ISOLATION_HOME}" \
    PATH="${PATH}" \
    TMPDIR="/tmp" \
    OPENCODE_CONFIG="${ISOLATION_CONFIG}" \
    OPENCODE_DISABLE_AUTOUPDATE=1 \
    OPENCODE_DISABLE_MODELS_FETCH=1 \
    OPENCODE_DISABLE_DEFAULT_PLUGINS=1 \
    OPENCODE_DISABLE_LSP_DOWNLOAD=1 \
    OPENCODE_DISABLE_CLAUDE_CODE=1 \
    OPENCODE_ENABLE_EXA=0 \
    OPENCODE_AUTO_SHARE=0 \
    timeout --signal=TERM --kill-after=5s "${ISOLATION_TIMEOUT_SECONDS}s" \
      "${ISOLATION_OPENCODE_BIN}" "${run_args[@]}" \
      >"${events_file}" 2>"${stderr_file}"
  isolation_run_exit=$?
  set -e
}
