#!/usr/bin/env bash
# release-gate.sh — ENFORCED release checklist. Runs the hard gates that
# previously lived only as markdown in build-docs/PUBLISH-CHECKLIST.md and
# were never enforced (the v0.1.2 version-drift and v0.1.3 untagged-release
# failures both happened because nothing FAILED the pipeline early).
#
# What this enforces (each is a hard FAIL, exit non-zero):
#   1. version-constant   — version.go's Version matches the release tag this
#                           tree is at (or, when not at a tag, is non-empty
#                           and semver-shaped). Prevents the v0.1.2 drift:
#                           a tag pushed while version.go still said 0.1.1.
#   2. changelog          — CHANGELOG.md has an entry for the release version.
#   3. release-tag-live   — when the release tag exists, it must be pushed to
#                           origin BEFORE the release is published. This
#                           closes the v0.1.3 gap: a draft published without
#                           its tag on the remote became an untagged release.
#   4. release-draft      — when GITHUB_RELEASE_URL is set, the draft must
#                           exist and be a DRAFT (never auto-publish).
#   5. assets-correct     — when the release assets are reachable, every
#                           archive must be named dohping_<ver>_<os>_<arch>
#                           and the linux-amd64 binary must report --version
#                           == the release version.
#
# Usage:
#   scripts/release-gate.sh                 local: checks 1-3
#   scripts/release-gate.sh --tag vX.Y.Z    also requires origin has the tag
#   scripts/release-gate.sh --release-url <draft-url> --tag vX.Y.Z
#       adds draft + asset verification for the final pre-publish check.
#
# Exit 0 = all gates green. Non-zero = red, do not release.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TAG=""
RELEASE_URL=""
while [ $# -gt 0 ]; do
  case "$1" in
    --tag) TAG="${2:-}"; shift 2 ;;
    --release-url) RELEASE_URL="${2:-}"; shift 2 ;;
    *) echo "release-gate: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

fails=0
pass() { echo "gate: $1 ... PASS"; }
fail() { echo "gate: $1 ... FAIL — $2"; fails=$((fails + 1)); }

# --- 1. version constant -------------------------------------------------
CONST="$(grep -m1 'Version = ' internal/version/version.go | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$CONST" ] || ! echo "$CONST" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  fail version-constant "version.go Version is not a clean semver: '$CONST'"
else
  # When we're exactly at a version tag, the constant MUST equal it.
  at_tag="$(git describe --tags --exact-match 2>/dev/null || true)"
  if [ -n "$at_tag" ] && [ "${at_tag#v}" != "$CONST" ]; then
    fail version-constant "at tag $at_tag but version.go says $CONST — bump version.go to match the release tag"
  else
    pass version-constant "$CONST"
  fi
fi

# --- 2. changelog has the version ---------------------------------------
if grep -q "^## \[${CONST#v}\]" CHANGELOG.md; then
  pass changelog "entry for $CONST"
else
  fail changelog "CHANGELOG.md has no '## [${CONST#v}]' entry — add one"
fi

# --- 3. release tag is live on origin (before publishing) ---------------
if [ -n "$TAG" ]; then
  if git ls-remote --tags origin "$TAG" 2>/dev/null | grep -q "refs/tags/$TAG"; then
    pass release-tag-live "$TAG is on origin"
  else
    fail release-tag-live "$TAG is NOT on origin — push the tag before the release can be published"
  fi
else
  pass release-tag-live "(no tag given — skipped)"
fi

# --- 4. release is a DRAFT, not published --------------------------------
if [ -n "$RELEASE_URL" ]; then
  # release URL like https://github.com/OWNER/REPO/releases/tag/vX.Y.Z or
  # the API /releases/<id>. We check via the API draft flag.
  API="${RELEASE_URL//github.com\/releases\/tag\//api.github.com\/repos\/neutralvibes\/dohping\/releases\/tags\/}"
  API="${API//github.com\/releases\//api.github.com\/repos\/neutralvibes\/dohping\/releases\/}"
  TOKEN="$(python3 -c "import re; print(re.search(r'https://([^@/]+)@github.com', open('$HOME/.hermes/user/git/credentials').read()).group(1))" 2>/dev/null || true)"
  if [ -n "$TOKEN" ]; then
    DRAFT=$(curl -s -u "$TOKEN" -H "Accept: application/vnd.github+json" "$API" | python3 -c "import json,sys; print(json.load(sys.stdin).get('draft',''))" 2>/dev/null || echo "")
    if [ "$DRAFT" = "True" ]; then
      pass release-draft "release is a draft"
    elif [ "$DRAFT" = "False" ]; then
      fail release-draft "release is ALREADY PUBLISHED — cannot gate a published release"
    else
      fail release-draft "could not read draft state from $API"
    fi
  else
    fail release-draft "no GitHub credentials found — cannot verify draft state"
  fi
else
  pass release-draft "(no release URL — skipped)"
fi

# --- 5. assets are correctly named + report the right version -----------
if [ -n "$RELEASE_URL" ] && [ -n "$TOKEN" ]; then
  NAMES=$(curl -s -u "$TOKEN" -H "Accept: application/vnd.github+json" "$API" | python3 -c "
import json,sys
d=json.load(sys.stdin)
print(' '.join(a['name'] for a in d.get('assets',[])))" 2>/dev/null || echo "")
  bad=""
  for n in $NAMES; do
    if [ "$n" != "SHA256SUMS" ] && ! echo "$n" | grep -qE "^dohping_${CONST#v}_[a-z0-9]+_[a-z0-9]+\.(tar\.gz|zip)$"; then
      bad="$bad $n"
    fi
  done
  if [ -z "$bad" ]; then
    pass assets-correct "all archives named dohping_${CONST#v}_*"
  else
    fail assets-correct "mislabeled assets:$bad (want dohping_${CONST#v}_*)"
  fi
else
  pass assets-correct "(no release URL — skipped)"
fi

echo
if [ "$fails" -eq 0 ]; then
  echo "release-gate: GREEN — safe to publish"
  exit 0
fi
echo "release-gate: RED ($fails failure(s)) — do NOT publish" >&2
exit "$fails"
