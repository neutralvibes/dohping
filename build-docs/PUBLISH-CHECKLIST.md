# PUBLISH-CHECKLIST.md — the gate before every push to GitHub

Consult this before every publish (the publish script's footer reminds you).
It grows over time — add items when we learn what matters.

## Hard gates (mechanical — enforced, not optional)

- [ ] publish/ derived by `bash scripts/publish-github.sh` from a CLEAN
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
- [ ] Releases are tagged (annotated v0.1.0, ...) — untagged releases are a tell
- [ ] LICENSE holder line filled (first publish only)
- [ ] PR opened; description lists exactly what's inside; user reviews the
      diff and merges. The agent never merges anything.

## Traceability (private only, never public)

- [ ] PUBLISH-LOG.md appended: date · publish commit · private sha · version

## OPEN DECISIONS — parked 2026-08-18 (user decides fresh, NOT the agent)

- [ ] First-commit wording: "Initial release" + tag v0.1.0, or the message
      itself is "v0.1.0"?
- [ ] Annotated version tags on every publish — yes/no?
