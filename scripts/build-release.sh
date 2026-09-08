#!/bin/sh
# Build the four v0.1 release binaries, checksums.txt, and build-info.txt.
# Official publishes come from .github/workflows/release.yml on a v* tag.
# Do not upload locally built artifacts as a GitHub Release.
set -eu

VERSION="${VERSION:-}"
COMMIT="${COMMIT:-}"
if [ -z "$VERSION" ]; then
	echo "build-release: VERSION is required (example: VERSION=v0.1.0)" >&2
	exit 1
fi
if [ -z "$COMMIT" ]; then
	COMMIT=$(git rev-parse HEAD)
fi

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

module="github.com/luxavr/agent-undo/internal/version.Version"
ldflags="-X ${module}=${VERSION}"
mkdir -p dist
rm -f dist/agent-undo_* checksums.txt build-info.txt

targets="darwin/arm64 darwin/amd64 linux/arm64 linux/amd64"
for pair in $targets; do
	os=${pair%/*}
	arch=${pair#*/}
	name="agent-undo_${VERSION}_${os}_${arch}"
	echo "building ${name}" >&2
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="$ldflags" -o "dist/${name}" ./cmd/agent-undo
done

native_os=$(go env GOOS)
native_arch=$(go env GOARCH)
native="dist/agent-undo_${VERSION}_${native_os}_${native_arch}"
if [ -x "$native" ]; then
	got=$("$native" version | tr -d '\n')
	if [ "$got" != "$VERSION" ]; then
		echo "build-release: version: got '$got' want '$VERSION'" >&2
		exit 1
	fi
	executed="${native_os}/${native_arch} (version check only)"
else
	executed="none (host ${native_os}/${native_arch} is not a release target)"
fi

go_ver=$(go version)
{
	echo "version: ${VERSION}"
	echo "commit: ${COMMIT}"
	echo "go: ${go_ver}"
	echo "cgo: 0"
	echo "trimpath: true"
	echo "ldflags: -X ${module}=${VERSION}"
	echo "targets:"
	for pair in $targets; do
		echo "  ${pair}"
	done
	echo "executed_on_build_host: ${executed}"
	echo "note: darwin/* and linux/arm64 are cross-compiled unless this host matches."
	echo "note: test execution remains ci.yml (ubuntu-latest, macos-latest)."
} > build-info.txt

(cd dist && sha256sum agent-undo_* 2>/dev/null || shasum -a 256 agent-undo_*) > checksums.txt
if command -v sha256sum >/dev/null 2>&1; then
	sha256sum build-info.txt >> checksums.txt
else
	shasum -a 256 build-info.txt >> checksums.txt
fi

echo "wrote dist/, checksums.txt, build-info.txt" >&2
