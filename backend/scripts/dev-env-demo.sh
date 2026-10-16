#!/usr/bin/env bash
#
# Local launcher for the agent-built practice environment demo (#192).
#
# Starts the API with CODEGYM_ENV_DEMO=true under Doppler (codegym/dev):
# a request from /practice/new builds the project with opencode on this
# machine, verifies it, promotes it, and opens a practice session.
#
# Requirements on this machine:
#   - Go, plus JDK 21 and Maven on PATH for Spring Boot builds (the agent
#     picks the Java version per project; builds have targeted 17 and 21).
#   - The pinned opencode binary (fetched below on first run).
#
# The registry is in memory: restarting this script gives a cold start, so
# the first request for a technology always builds from scratch, and
# repeated requests reuse the promoted environment.
#
# Usage (from the backend directory):
#   ./scripts/dev-env-demo.sh
#
# Environment overrides:
#   CODEGYM_PORT              backend listen port (default 8080)
#   CODEGYM_ENV_DEMO_TIMEOUT  per-build limit (default 40m)
set -euo pipefail

backend_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
repo_dir="$(dirname -- "${backend_dir}")"
cd "${backend_dir}"

# The pipeline shells out to Windows java.exe with POSIX-style classpaths,
# which requires Git Bash's automatic path conversion. A globally exported
# MSYS_NO_PATHCONV=1 (common for Docker work) makes java receive the literal
# POSIX path and die with ClassNotFoundException for the Maven launcher.
# Unset it for the server process tree only; the interactive terminal keeps
# its setting.
unset MSYS_NO_PATHCONV
unset MSYS2_ARG_CONV_EXCL

# Pinned opencode is fetched on first run, unless the caller already points
# at a working binary (e.g. Windows, where the pinned asset matrix has no
# entry yet and chmod cannot set the exec bit on noacl mounts). A native
# opencode.exe next to the pinned path also satisfies the gate; Windows
# resolves it when the backend invokes the extensionless path.
if [[ ! -x "${repo_dir}/spikes/opencode/.bin/opencode" && ! -x "${repo_dir}/spikes/opencode/.bin/opencode.exe" && -z "${CODEGYM_OPENCODE_BINARY:-}" ]]; then
  "${repo_dir}/spikes/opencode/download-opencode.sh"
fi
for tool in go java mvn doppler; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "dev-env-demo: ${tool} is required on PATH" >&2
    exit 2
  fi
done

export CODEGYM_ENV_DEMO=true
# Spring Boot resolves dependencies from Maven Central during the build
# instead of requiring a pre-seeded offline cache.
export CODEGYM_ALLOW_UNCACHED_SETUP=1
export PORT="${CODEGYM_PORT:-8080}"

exec doppler run -p codegym -c dev -- go run ./cmd/server
