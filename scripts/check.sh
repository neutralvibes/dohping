#!/usr/bin/env bash
# Full quality gate for dohping: gofmt, go vet, race tests, then the static
# analyzers (golangci-lint, staticcheck, gosec, govulncheck) when installed.
#
# One definition of "green" everywhere: run locally, run by CI (gosec is
# pre-installed there), and called by release.sh before any build. A red
# gate refuses to release. Optional tools that aren't installed print SKIP
# (not FAIL), so the same script works on a stock runner.
#
# Exit 0 = green. Non-zero = red.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Tools may live on PATH or in ~/go/bin (sandbox layout).
export PATH="$HOME/go/bin:$PATH"

failures=0

run() {  # run <label> <cmd...>
  local label="$1"; shift
  local log
  log="$(mktemp)"
  echo -n "check: $label ... "
  if "$@" >"$log" 2>&1; then
    echo "PASS"
  else
    echo "FAIL"
    echo "--- $label output (tail) ---"
    tail -20 "$log"
    echo "---"
    failures=$((failures + 1))
  fi
  rm -f "$log"
}

go_files="$(git ls-files '*.go')"
if [ -z "$go_files" ]; then
  echo "gate: RED (no tracked .go files — wrong directory?)" >&2
  exit 2
fi

# gofmt must be clean on every tracked file (no output = clean).
run "gofmt" bash -c "test -z \"\$(gofmt -l $go_files)\""
run "go vet" go vet ./...
run "go test -race" go test -race -count=1 ./...

# Static analyzers — SKIP (not FAIL) when absent so stock CI runners with
# only the pre-installed gosec still get a meaningful green.
for tool in golangci-lint staticcheck gosec govulncheck; do
  if command -v "$tool" >/dev/null 2>&1; then
    case "$tool" in
      golangci-lint) run "$tool" golangci-lint run ;;
      gosec)         run "$tool" gosec -quiet ./... ;;
      *)             run "$tool" "$tool" ./... ;;
    esac
  else
    echo "check: $tool ... SKIP (not installed)"
  fi
done

echo
if [ "$failures" -eq 0 ]; then
  echo "gate: GREEN"
  exit 0
fi
echo "gate: RED ($failures failure(s))" >&2
exit "$failures"
