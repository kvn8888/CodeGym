#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
VERSION="v1.18.11"
RELEASE_BASE="https://github.com/sst/opencode/releases/download/${VERSION}"

os="$(uname -s)"
arch="$(uname -m)"
case "${os}/${arch}" in
  Darwin/arm64)
    asset="opencode-darwin-arm64.zip"
    sha256="188ff6a716bcd40e33ac62f17f4aec9bd760164fa6a2cde66f779a5db4abc7ce"
    ;;
  Darwin/x86_64)
    asset="opencode-darwin-x64.zip"
    sha256="95953ab2aca4322b90690bf34697cc9b47b6a7c72f78e7c469056fb589124d31"
    ;;
  *)
    echo "Unsupported platform: ${os}/${arch}. Add the pinned ${VERSION} asset and checksum to this script." >&2
    exit 2
    ;;
esac

bin_dir="${SCRIPT_DIR}/.bin"
archive="${bin_dir}/${asset}"
url="${RELEASE_BASE}/${asset}"
mkdir -p "${bin_dir}"

echo "Downloading opencode ${VERSION} for ${os}/${arch}"
echo "Asset: ${url}"
if [[ ! -f "${archive}" ]]; then
  curl --fail --location --proto '=https' --tlsv1.2 --output "${archive}" "${url}"
fi

actual="$(shasum -a 256 "${archive}" | awk '{print $1}')"
if [[ "${actual}" != "${sha256}" ]]; then
  echo "Checksum mismatch: expected ${sha256}, got ${actual}" >&2
  exit 1
fi

unzip -oq "${archive}" -d "${bin_dir}"
chmod 0755 "${bin_dir}/opencode"
echo "Verified SHA-256: ${sha256}"
"${bin_dir}/opencode" --version

# OpenCode initializes its TypeScript custom-tool runtime by installing the
# matching @opencode-ai/plugin package. Do that explicitly during setup so the
# protocol run itself can remain localhost-only, including with a fresh HOME.
config_template="${bin_dir}/home-template/.config/opencode"
if [[ ! -d "${config_template}/node_modules/@opencode-ai/plugin" ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required once to prepare the pinned custom-tool runtime." >&2
    exit 2
  fi
  mkdir -p "${config_template}" "/tmp/opencode-spike-npm-cache"
  HOME="/tmp/opencode-spike-download" npm install \
    --cache "/tmp/opencode-spike-npm-cache" \
    --prefix "${config_template}" \
    --save-exact \
    --no-audit \
    --no-fund \
    "@opencode-ai/plugin@1.18.11"
fi
echo "Prepared pinned custom-tool runtime: @opencode-ai/plugin@1.18.11"
