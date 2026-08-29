#!/usr/bin/env bash
# Open (or re-confirm) the Defender false-positive tracking issue for a
# release. Called from the CI release job after the draft is attached.
#
# The Microsoft submission itself is a manual human act (the WDSI portal is
# a web form); this script only guarantees the tracking issue exists so the
# reminder cannot be forgotten. Mechanical, never gating on the submission.
#
# The issue body is rendered from
# .github/ISSUE_TEMPLATE/ms-defender-submission.md — the single source of
# truth for the issue shape; this script fills its {{ placeholders }}.
#
# Env: GH_TOKEN (required), VERSION (tag ref), COMMIT (sha).
set -euo pipefail

VER="${VERSION#v}"
ARCHIVE="dohping_${VER}_windows_amd64.zip"
SHA256="$(sha256sum "dist/${ARCHIVE}" | cut -d' ' -f1)"
SHORT_COMMIT="$(echo "${COMMIT}" | cut -c1-12)"
TEMPLATE=".github/ISSUE_TEMPLATE/ms-defender-submission.md"
BODY_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE"' EXIT

# Strip the template's frontmatter (everything through the second --- line),
# then fill the placeholders.
awk 'BEGIN{n=0} /^---$/{n++; next} n>=2' "$TEMPLATE" \
  | sed \
      -e "s|{{ version }}|${VER}|g" \
      -e "s|{{ commit }}|${SHORT_COMMIT}|g" \
      -e "s|{{ artifact }}|dohping.exe|g" \
      -e "s|{{ archive }}|${ARCHIVE}|g" \
      -e "s|{{ sha256 }}|${SHA256}|g" \
      -e "s|{{ detection }}|Trojan:Win32/Wacatac.B!ml|g" \
      > "$BODY_FILE"

# The label must exist for gh issue create to attach it.
gh label create ms-defender-submission \
  --description "Windows Defender false-positive submission pending" \
  --force >/dev/null 2>&1 || true

# One tracking issue per release; skip if one is already open.
if ! gh issue list --label ms-defender-submission --state open \
    --json number --jq 'length' | grep -q '^[1-9]'; then
  gh issue create \
    --title "Defender false-positive submission: ${VERSION} windows binary" \
    --label ms-defender-submission \
    --body-file "$BODY_FILE"
fi
