#!/usr/bin/env bash
#
# Starts a Cloudflare quick tunnel to the local CodeGym backend so a Daytona
# build sandbox can reach the agent relay at /api/v1/agent-relay/v1.
#
# A sandbox cannot reach 127.0.0.1, so during local development the relay must
# be exposed publicly. Quick tunnels need no account: cloudflared prints a
# random https URL, this script writes it to a file, and the backend reads
# that file to configure sandbox builds. Stop with Ctrl-C.
#
# Usage (from the backend directory, in its own terminal):
#   ./scripts/dev-relay-tunnel.sh
#
# Environment overrides:
#   CODEGYM_PORT              backend port to expose (default 8080)
#   CODEGYM_TUNNEL_URL_FILE   where to write the public URL
#                             (default: <repo>/.codegym/agent-relay-url)
set -euo pipefail

backend_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
repo_dir="$(dirname -- "${backend_dir}")"
port="${CODEGYM_PORT:-8080}"
url_file="${CODEGYM_TUNNEL_URL_FILE:-${repo_dir}/.codegym/agent-relay-url}"

find_cloudflared() {
  if command -v cloudflared >/dev/null 2>&1; then
    command -v cloudflared
    return 0
  fi
  for candidate in \
    "/c/Program Files (x86)/cloudflared/cloudflared.exe" \
    "/c/Program Files/cloudflared/cloudflared.exe" \
    "$HOME/AppData/Local/Microsoft/WinGet/Links/cloudflared.exe"; do
    if [[ -x "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 1
}

if ! cloudflared_bin="$(find_cloudflared)"; then
  echo "dev-relay-tunnel: cloudflared not found." >&2
  echo "Install it (winget install --id Cloudflare.cloudflared) and retry." >&2
  exit 2
fi

mkdir -p "$(dirname -- "${url_file}")"
# Clear any stale URL so the backend never reads yesterday's tunnel.
: >"${url_file}"

echo "dev-relay-tunnel: exposing http://127.0.0.1:${port} via a quick tunnel"
echo "dev-relay-tunnel: public URL will be written to ${url_file}"

# cloudflared logs the assigned URL on stderr. Tee it, extract the URL, and
# write it once so the backend can pick it up.
"${cloudflared_bin}" tunnel --no-autoupdate --url "http://127.0.0.1:${port}" 2>&1 |
  while IFS= read -r line; do
    printf '%s\n' "${line}"
    if [[ "${line}" =~ (https://[a-z0-9-]+\.trycloudflare\.com) ]]; then
      if [[ ! -s "${url_file}" ]]; then
        printf '%s\n' "${BASH_REMATCH[1]}" >"${url_file}"
        echo "dev-relay-tunnel: relay base is ${BASH_REMATCH[1]}/api/v1/agent-relay/v1"
      fi
    fi
  done
