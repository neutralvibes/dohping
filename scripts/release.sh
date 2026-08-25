#!/usr/bin/env bash
# Build reproducible dohping release packages for all supported targets.
#
# Reproducible: -trimpath + no VCS stamping; the same source + Go version
# yields byte-identical binaries. Version is injected via ldflags.
#
# Packaging: each target ships as a compressed archive named
#   dohping_<version>_<os>_<arch>.tar.gz    (unix: gzip, exec bit set)
#   dohping_<version>_<os>_<arch>.zip       (windows: plain dohping.exe)
# with the PLAIN binary name inside (no os/arch suffix, no version) so a
# download extracts to a ready-to-run `dohping` / `dohping.exe`. A
# SHA256SUMS over the archives accompanies them.
#
# Usage: ./scripts/release.sh [output-dir]   (default: dist/)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$(cd "${1:-dist}" && pwd)"  # absolute — packaging runs from a staging dir

# The quality gate runs first — a red gate refuses to build a release.
# (gofmt, vet, race tests, gosec + the other analyzers when installed.)
# The gate script is a dev-tree convenience; in a published tree (CI)
# the workflow's own quality-gate step already ran, so absence of the
# script is not an error — skip with a warning.
if [ -x "$ROOT/scripts/check.sh" ]; then
  "$ROOT/scripts/check.sh"
else
  echo "release: scripts/check.sh not present (dev-only) — skipping local gate; CI gate already ran" >&2
fi

VERSION="${DOHPING_VERSION:-$(grep -m1 'Version = ' "$ROOT/internal/version/version.go" | sed -E 's/.*"([^"]+)".*/\1/')}"
GOROOT_BIN="${GOROOT:-$(go env GOROOT)}/bin"

rm -rf "$OUT"
mkdir -p "$OUT"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
)

for target in "${targets[@]}"; do
  set -- $target
  os="$1"; arch="$2"
  bin="dohping"
  if [ "$os" = "windows" ]; then bin="dohping.exe"; fi

  echo "building $os/$arch"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    "$GOROOT_BIN/go" build -trimpath -buildvcs=false \
    -ldflags "-X dohping/internal/version.Version=$VERSION" \
    -o "$STAGE/$bin" "$ROOT/cmd/dohping"

  base="dohping_${VERSION}_${os}_${arch}"
  if [ "$os" = "windows" ]; then
    # Zip with the system `zip` (present on the CI runner and dev boxes).
    (cd "$STAGE" && zip -q "$OUT/$base.zip" "$bin")
    echo "packaged $OUT/$base.zip"
  else
    # The archive must preserve the executable bit so a download extracts
    # to a runnable binary without a manual chmod.
    (cd "$STAGE" && tar czf "$OUT/$base.tar.gz" "$bin")
    echo "packaged $OUT/$base.tar.gz"
  fi
done

(cd "$OUT" && sha256sum dohping_* > SHA256SUMS)
echo "checksums:"
cat "$OUT/SHA256SUMS"
echo "release ready in $OUT (version $VERSION)"
