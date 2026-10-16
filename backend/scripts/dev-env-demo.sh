#!/usr/bin/env bash
#
# Local launcher for the agent-built practice environment demo (#192).
#
# Starts the API with CODEGYM_ENV_DEMO=true under Doppler (codegym/dev):
# a request from /practice/new builds the project with opencode on this
# machine, verifies it, promotes it, and opens a practice session.
#
# Requirements on this machine:
#   - Go, plus Java 17+ and Maven on PATH for Spring Boot builds.
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

if [[ ! -x "${repo_dir}/spikes/opencode/.bin/opencode" ]]; then
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
