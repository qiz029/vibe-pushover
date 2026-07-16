#!/bin/sh

set -eu

export LC_ALL=C
export LANG=C

REPOSITORY=${VIBE_PUSHOVER_REPOSITORY:-qiz029/vibe-pushover}
INSTALL_DIR=${VIBE_PUSHOVER_INSTALL_DIR:-"$HOME/.local/bin"}
VERSION=${VIBE_PUSHOVER_VERSION:-}
DOWNLOAD_BASE_URL=${VIBE_PUSHOVER_DOWNLOAD_BASE_URL:-}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "vibe-pushover installer: required command not found: $1" >&2
    exit 1
  fi
}

require_command curl
require_command install
require_command tar

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *)
    echo "vibe-pushover installer: unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *)
    echo "vibe-pushover installer: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if [ -z "$VERSION" ]; then
  if [ -n "$DOWNLOAD_BASE_URL" ]; then
    echo "vibe-pushover installer: VIBE_PUSHOVER_VERSION is required with a custom download base URL" >&2
    exit 1
  fi
  latest_url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPOSITORY/releases/latest")
  VERSION=${latest_url##*/}
fi

case "$VERSION" in
  v[0-9]*)
    version_body=${VERSION#v}
    case "$version_body" in
      *[!0-9A-Za-z._-]*)
        echo "vibe-pushover installer: invalid release version: $VERSION" >&2
        exit 1
        ;;
    esac
    ;;
  *)
    echo "vibe-pushover installer: invalid release version: $VERSION" >&2
    exit 1
    ;;
esac

asset="vibe-pushover_${VERSION}_${os}_${arch}.tar.gz"
if [ -n "$DOWNLOAD_BASE_URL" ]; then
  base_url=${DOWNLOAD_BASE_URL%/}
else
  base_url="https://github.com/$REPOSITORY/releases/download/$VERSION"
fi
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/vibe-pushover-install.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

echo "Downloading vibe-pushover $VERSION for $os/$arch..."
curl -fsSL "$base_url/$asset" -o "$tmp_dir/$asset"
curl -fsSL "$base_url/SHA256SUMS" -o "$tmp_dir/SHA256SUMS"

expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "$tmp_dir/SHA256SUMS")
if [ -z "$expected" ]; then
  echo "vibe-pushover installer: checksum not found for $asset" >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmp_dir/$asset" | awk '{ print $1 }')
elif command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp_dir/$asset" | awk '{ print $1 }')
else
  echo "vibe-pushover installer: shasum or sha256sum is required" >&2
  exit 1
fi

if [ "$actual" != "$expected" ]; then
  echo "vibe-pushover installer: checksum verification failed for $asset" >&2
  exit 1
fi

tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp_dir/vibe-pushover" "$INSTALL_DIR/vibe-pushover"

echo "Installed vibe-pushover $VERSION to $INSTALL_DIR/vibe-pushover"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add $INSTALL_DIR to PATH to run vibe-pushover directly." ;;
esac
