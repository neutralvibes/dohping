# PUBLISH-CHECKLIST.md — the gate before every push to GitHub

Consult this before every publish (the publish script's footer reminds you).
It grows over time — add items when we learn what matters.

## Hard gates (mechanical — enforced, not optional)

Run `scripts/release-gate.sh` — the ENFORCED version of this checklist. It
fails the pipeline (non-zero exit) when any hard gate is red; the markdown
below is the human-readable record of what it enforces. Do not publish on a
RED gate.

- [ ] `scripts/release-gate.sh --tag vX.Y.Z` exits 0 (version-constant
      matches the tag, CHANGELOG has the entry, tag is live on origin)
- [ ] CI is GREEN on the PR before merge (gate + gate-windows jobs); a red
      CI is a merge blocker
- [ ] `publish/` derived by `bash scripts/publish-github.sh` from a CLEAN
      private tree; the script's own assertions passed (forbidden-file scan,
      exact public file list)
- [ ] `publish/` pre-commit hook is active (refuses commits whose author is
      Hermes Agent / hermes@hermes.home — the hook is installed by the
      publish script on repo init)
- [ ] Build docs absent from the pushed branch:
      `git -C publish ls-files | grep -cE 'HANDOFF|DECISIONS|SPECIFICATION|CHECKPOINT|LAUNCH|Phases|PROGRESS'` → 0

## Judgment gates (human-looking output)

- [ ] Commit author = the user's identity:
      `git -C publish log -1 --format='%an <%ae>'` → user's name +
      `<ID>+<username>@users.noreply.github.com`. NEVER the agent.
- [ ] Commit message reads like the user wrote it:
      - one terse line; no conventional-commit prefixes (feat:/fix:/chore:)
      - no semicolon clause-chains, no parenthetical scope annotation
      - no process metadata (private shas, UTC timestamps, "snapshot of")
      - release commits are version-driven ("Initial release", "v0.1.0",
        "v0.1.1: fix window-mode resize reclaim")
      - when in doubt: write it as the user would type it, then halve it
- [ ] Releases are tagged — annotated tag (v0.1.0, ...) on every RELEASE, not
      on every push/publish; untagged releases are a tell
- [ ] Release assets are plain-named (`dohping-<os>-<arch>`); the build sha
      lives in the release notes + SHA256SUMS, never in public asset
      filenames — sha-prefixed names are rig/local-testing only
      (decided 2026-08-19)
- [ ] LICENSE holder line filled (first publish only)
- [ ] PR opened; description lists exactly what's inside; user reviews the
      diff and merges. The agent never merges anything.

## Traceability (private only, never public)

- [ ] PUBLISH-LOG.md appended: date · publish commit · private sha · version

## DECISIONS — all resolved 2026-08-19

- First-commit wording: **"Initial release"** + annotated tag **v0.1.0**.
- Merge policy: **squash** (user chose it on the repotest rehearsal).
- Tags: annotated, per RELEASE (not per publish).
- Release assets plain-named; the build sha only in release notes/SHA256SUMS.
- LICENSE holder: **`github.com/neutralvibes`** (owner-path form, no protocol).
