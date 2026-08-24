#!/usr/bin/env bash
# Build reproducible dohping release binaries for all supported targets.
#
# Reproducible: -trimpath + no VCS stamping; the same source + Go version
# yields byte-identical binaries. Version is injected via ldflags.
#
# Usage: ./scripts/release.sh [output-dir]   (default: dist/)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-dist}"

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

mkdir -p "$OUT"
rm -f "$OUT"/dohping-* "$OUT/SHA256SUMS"

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
  ext=""
  if [ "$os" = "windows" ]; then ext=".exe"; fi
  outfile="$OUT/dohping-${os}-${arch}${ext}"
  echo "building $os/$arch -> $outfile"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    "$GOROOT_BIN/go" build -trimpath -buildvcs=false \
    -ldflags "-X dohping/internal/version.Version=$VERSION" \
    -o "$outfile" "$ROOT/cmd/dohping"
done

(cd "$OUT" && sha256sum dohping-* > SHA256SUMS)
echo "checksums:"
cat "$OUT/SHA256SUMS"
echo "release ready in $OUT (version $VERSION)"
