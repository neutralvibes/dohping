#!/usr/bin/env bash
# publish-github.sh — derive the PUBLIC repo snapshot into publish/.
#
# publish/ is a SEPARATE git repository containing exactly the files that
# should reach GitHub. It is a DERIVED ARTIFACT (like dist/): this script
# wipes its working tree (keeping .git so remotes/tags/identity survive),
# copies the public file list, asserts nothing extra snuck in, and commits
# a snapshot tied to the private commit it came from.
#
# The private build-process docs (build-docs/) and local state (dist/,
# .notify-state) NEVER go here — the file list below is the contract, and
# the forbidden-file + exact-list assertions fail the run if it is broken.
#
# This script itself is intentionally NOT published: it is housekeeping for
# the private working tree (it knows about publish/ and build-docs/).
#
# This script never pushes. After a run, publishing happens from the user's
# machine (they hold the credentials):
#   git -C publish remote add origin git@github.com:YOU/dohping.git   # once
#   git -C publish push -u origin master                              # each publish
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUB="$ROOT/publish"

# --- The contract: what is public -----------------------------------------
PUBLIC_ITEMS=(
  README.md
  LICENSE
  .gitignore
  go.mod
  go.sum
  cmd
  internal
  scripts/release.sh
  scripts/pty-resize-probe.py
)

# Files that must NEVER appear in the published tree (checked by name).
FORBIDDEN=(
  HANDOFF.md DECISIONS.md CHECKPOINT.md PROGRESS.md LAUNCH.md
  SPECIFICATION.md SPEC-window-resize-reclaim.md Phases.md
  ENGINEERING_PRINCIPLES.md
  build-docs dist .notify-state publish
)

# The public repo's own .gitignore (independent of the working tree's).
PUB_GITIGNORE='dohping
dohping.exe
dist/
*.test
'

cd "$ROOT"

# A clean working tree is required: a publish must match a committed state.
if [ -n "$(git status --porcelain)" ]; then
  echo "ERROR: working tree not clean — commit or stash before publishing" >&2
  exit 1
fi

# Rebuild the publish working tree, keeping .git (remotes/tags survive).
mkdir -p "$PUB"
find "$PUB" -mindepth 1 ! -path "$PUB/.git" ! -path "$PUB/.git/*" -exec rm -rf {} +
if [ ! -d "$PUB/.git" ]; then
  git -C "$PUB" init -q -b master
  # Inherit the private repo's author identity when it has one.
  priv_name="$(git config user.name || true)"
  priv_email="$(git config user.email || true)"
  [ -n "$priv_name" ] && git -C "$PUB" config user.name "$priv_name" || true
  [ -n "$priv_email" ] && git -C "$PUB" config user.email "$priv_email" || true
fi

# Copy the public items.
for item in "${PUBLIC_ITEMS[@]}"; do
  if [ -d "$item" ]; then
    mkdir -p "$PUB/$item"
    cp -r "$item"/. "$PUB/$item"/
  else
    cp "$item" "$PUB/$item"
  fi
done
printf '%s' "$PUB_GITIGNORE" > "$PUB/.gitignore"

# Assertion 1: nothing forbidden in the tree.
violations=()
for f in "${FORBIDDEN[@]}"; do
  if [ -e "$PUB/$f" ]; then violations+=("$f"); fi
done
if [ "${#violations[@]}" -gt 0 ]; then
  echo "ERROR: forbidden file(s) found in publish tree: ${violations[*]}" >&2
  exit 1
fi

# Assertion 2: the committed file list is EXACTLY the contract (no more, no less).
git -C "$PUB" add -A
actual="$(git -C "$PUB" ls-files | sort)"
expected="$({
  for item in "${PUBLIC_ITEMS[@]}"; do
    if [ -d "$item" ]; then
      (cd "$item" && find . -type f | sed 's|^\./||' | while read -r f; do echo "$item/$f"; done)
    else
      echo "$item"
    fi
  done
  echo ".gitignore"
} | sort)"
if [ "$actual" != "$expected" ]; then
  echo "ERROR: published file list deviates from the contract:" >&2
  diff <(echo "$expected") <(echo "$actual") >&2 || true
  exit 1
fi

# Commit the snapshot (no-op when nothing changed since the last one).
priv_sha="$(git rev-parse --short HEAD)"
if git -C "$PUB" diff --cached --quiet; then
  echo "publish/: no changes since last snapshot"
else
  git -C "$PUB" commit -qm "Publish snapshot of private master @ $priv_sha ($(date -u +%Y-%m-%dT%H:%M:%SZ))"
fi

echo
echo "publish/ ready: $(git -C "$PUB" ls-files | wc -l) file(s), $(git -C "$PUB" log --oneline | wc -l) commit(s)"
echo "  latest: $(git -C "$PUB" log -1 --format='%h %s')"
echo
echo "To publish (on YOUR machine — this script never pushes):"
echo "  git -C $PUB remote add origin git@github.com:YOU/dohping.git   # once"
echo "  git -C $PUB push -u origin master                              # each publish"
