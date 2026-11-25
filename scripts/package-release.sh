#!/bin/sh
# Builds the release archives and SHA256SUMS. Usage: package-release.sh VERSION OUTDIR
set -eu
[ "$#" -eq 2 ] || {
    echo "usage: $0 VERSION OUTDIR" >&2
    exit 2
}
version=$1
echo "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' || {
    echo "bad version $version, want something like v1.2.3" >&2
    exit 2
}
cd "$(dirname "$0")/.."
mkdir -p "$2"
out=$(cd "$2" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go test ./...
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${target%/*} arch=${target#*/}
    name="syscall_${version}_${os}_${arch}"
    dir="$tmp/$name"
    mkdir -p "$dir/dataset"
    export CGO_ENABLED=0 GOOS="$os" GOARCH="$arch"
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$dir/syscall" .
    python3 scripts/collect-licenses.py "$dir/THIRD_PARTY_LICENSES.txt"
    unset GOOS GOARCH
    cp README.md NOTICE.md LICENSE "$dir/"
    cp dataset/README.md dataset/provenance.json "$dir/dataset/"
    tar -czf "$out/$name.tar.gz" -C "$tmp" "$name"
done
cd "$out"
if command -v sha256sum >/dev/null; then sha256sum syscall_*.tar.gz; else shasum -a 256 syscall_*.tar.gz; fi >SHA256SUMS
