#!/usr/bin/env bash
# Full quality gate for dohping: gofmt, go vet, race tests, then the static
# analyzers (golangci-lint, staticcheck, gosec, govulncheck) when installed.
#
# One definition of "green" everywhere: run locally, run by CI, and called
# by release.sh before any build. A red gate refuses to release. The SAME
# file, with args for environment-specific behaviour when required:
#   (no args)  local / release.sh — optional tools that aren't installed
#              print SKIP (not FAIL), so a dev box without every analyzer
#              still gets a meaningful green.
#   --ci       CI runner — a missing optional tool is a FAIL (the workflow
#              installs every analyzer, so a SKIP here means the release
#              path silently lost a check; that must be loud).
#
# Exit 0 = green. Non-zero = red.
set -uo pipefail

CI_MODE=0
for arg in "$@"; do
  case "$arg" in
    --ci) CI_MODE=1 ;;
    *) echo "check: unknown argument '$arg' (expected --ci)" >&2; exit 2 ;;
  esac
done

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
# The debug build is a separate compile unit: the real debugx lives only
# under -tags debug (release ships the inert stub). Test BOTH so the
# debug implementation stays green and release stays compiled-out.
run "go vet -tags debug" go vet -tags debug ./...
run "go test -race -tags debug" go test -race -count=1 -tags debug ./...

# Static analyzers. Locally they SKIP (not FAIL) when absent so a dev box
# without every analyzer still gets a meaningful green; with --ci a missing
# tool is a FAIL because the runner is expected to have provisioned it and
# a SKIP would mean the release path silently lost a check.
# golangci-lint (broad style/correctness sweep) and staticcheck (deep
# analysis) are complementary, NOT redundant: golangci-lint's bundled
# staticcheck is only a reimplementation of staticcheck's rules — the
# standalone binary is the authoritative tool (its author explicitly does
# not support the bundled reimplementation). Both run; CI installs both.
for tool in golangci-lint staticcheck gosec govulncheck; do
  if command -v "$tool" >/dev/null 2>&1; then
    case "$tool" in
      golangci-lint) run "$tool" golangci-lint run ;;
      gosec)         run "$tool" gosec -quiet ./... ;;
      *)             run "$tool" "$tool" ./... ;;
    esac
  elif [ "$CI_MODE" -eq 1 ]; then
    echo "check: $tool ... FAIL (required in --ci mode, not installed)"
    failures=$((failures + 1))
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
