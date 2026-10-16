#!/usr/bin/env bash
# Builds the release archives of a version: one per platform with the
# binary, README.md, LICENSE and CHANGELOG.md, plus SHA256SUMS.
#
# Usage: tools/release/build.sh <version tag, e.g. v0.4.0> <output dir>
set -euo pipefail

tag=${1:?version tag, e.g. v0.4.0}
out=${2:?output directory}
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "not a version tag: $tag" >&2; exit 1; }

root=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for target in windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os=${target%/*} arch=${target#*/}
  name=fbrescue_${tag}_${os}_${arch}
  bin=fbrescue
  [[ $os == windows ]] && bin=fbrescue.exe
  mkdir "$work/$name"
  (cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$tag" \
    -o "$work/$name/$bin" ./cmd/fbrescue)
  cp "$root/README.md" "$root/LICENSE" "$root/CHANGELOG.md" "$work/$name/"
  if [[ $os == windows ]]; then
    (cd "$work/$name" && zip -q -X "$out/$name.zip" ./*)
  else
    tar -C "$work/$name" -czf "$out/$name.tar.gz" --owner=0 --group=0 .
  fi
done

(cd "$out" && sha256sum fbrescue_"${tag}"_* > SHA256SUMS)
cat "$out/SHA256SUMS"
