#!/usr/bin/env bash
# FlowForge release build: cross-compile the single binary for the full
# distribution matrix (linux/darwin/windows x amd64/arm64), package archives,
# and emit SHA256SUMS. Pure-Go deps (modernc sqlite, wazero, starlark) mean
# CGO_ENABLED=0 works everywhere.
#
# Usage:  scripts/build.sh [version]        (default: dev)
#
# The embedded UI must be built first (npm --prefix app run build; copy
# app/dist/* to server-go/ui/dist/) — the script warns if the placeholder
# embed is detected.

set -euo pipefail

version="${1:-dev}"
repo="$(cd "$(dirname "$0")/.." && pwd)"
out="$repo/dist"
uidir="$repo/server-go/ui/dist"

if [ ! -d "$uidir/assets" ]; then
  echo "WARNING: server-go/ui/dist has no assets/ — the placeholder UI will be embedded." >&2
  echo "         Build the real UI first: npm --prefix app install && npm --prefix app run build && cp -r app/dist/* server-go/ui/dist/" >&2
fi

targets="
  linux   amd64
  linux   arm64
  darwin  amd64
  darwin  arm64
  windows amd64
  windows arm64
"

mkdir -p "$out"
cd "$repo/server-go"

archives=()
while read -r os arch; do
  [ -z "$os" ] && continue
  name="flowforge-${version}-${os}-${arch}"
  ext=""
  entry="flowforge"
  if [ "$os" = "windows" ]; then ext=".exe"; entry="flowforge.exe"; fi
  echo "==> building $name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${version}" -o "$out/$name$ext" ./cmd/flowforge
  if [ "$os" = "windows" ]; then
    (cd "$out" && zip -q -j "$name.zip" "$name$ext" && rm "$name$ext")
    archives+=("$name.zip")
  else
    (cd "$out" && mv "$name$ext" "$entry" && tar -czf "$name.tar.gz" "$entry" && rm "$entry")
    archives+=("$name.tar.gz")
  fi
done <<< "$targets"

cd "$out"
: > SHA256SUMS
for a in "${archives[@]}"; do
  sha256sum "$a" >> SHA256SUMS
done

echo
echo "Release artifacts in $out"
cat SHA256SUMS
