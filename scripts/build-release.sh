#!/bin/sh

set -eu

export LC_ALL=C
export LANG=C

VERSION=${VERSION:-}
case "$VERSION" in
  v[0-9]*)
    version_body=${VERSION#v}
    case "$version_body" in
      *[!0-9A-Za-z._-]*)
        echo "invalid release version: $VERSION" >&2
        exit 1
        ;;
    esac
    ;;
  *)
    echo "usage: VERSION=v0.1.0 scripts/build-release.sh" >&2
    exit 1
    ;;
esac

root_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dist_dir="$root_dir/dist"
binary_name=vibe-pushover

cd "$root_dir"

rm -rf "$dist_dir"
mkdir -p "$dist_dir"

build_archive() {
  goos=$1
  goarch=$2
  extension=$3
  archive_type=$4
  build_dir="$dist_dir/build-${goos}-${goarch}"
  output="$build_dir/$binary_name$extension"
  archive="$dist_dir/${binary_name}_${VERSION}_${goos}_${goarch}.${archive_type}"

  mkdir -p "$build_dir"
  echo "Building $goos/$goarch..."
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$output" \
    ./cmd/vibe-pushover

  if [ "$archive_type" = "tar.gz" ]; then
    tar -C "$build_dir" -czf "$archive" "$binary_name$extension"
  else
    (cd "$build_dir" && zip -q "$archive" "$binary_name$extension")
  fi
  rm -rf "$build_dir"
}

build_archive darwin amd64 "" tar.gz
build_archive darwin arm64 "" tar.gz
build_archive linux amd64 "" tar.gz
build_archive linux arm64 "" tar.gz
build_archive windows amd64 .exe zip
build_archive windows arm64 .exe zip

cp "$root_dir/install.sh" "$dist_dir/install.sh"
(
  cd "$dist_dir"
  if command -v shasum >/dev/null 2>&1; then
    LC_ALL=C LANG=C shasum -a 256 install.sh vibe-pushover_*.tar.gz vibe-pushover_*.zip > SHA256SUMS
  else
    LC_ALL=C LANG=C sha256sum install.sh vibe-pushover_*.tar.gz vibe-pushover_*.zip > SHA256SUMS
  fi
)

echo "Release assets written to $dist_dir"
