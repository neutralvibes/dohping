# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `SPECIFICATION.md` (contract), `DECISIONS.md`
(rationale), `CHECKPOINT.md` (gate status), `PROGRESS.md` (timeline). `README.md`
is the user-facing doc.

**Status: build complete, all phases green. v0.1.3 LIVE and correct on GitHub.**
v0.1.1 and v0.1.2 also live (v0.1.2 carries a mislabel note — see below).
Release pipeline now ends with enforced gates. The rig INDEX is auto-derived.
No release work outstanding.

---

## The v0.1.2 / v0.1.3 release saga (READ FIRST — 2026-08-26)

**v0.1.2 SHIPPED MISLABELED, then fixed forward as v0.1.3.**
- v0.1.2 (PR #7 merged → tag → CI draft → user published): the version constant
  `internal/version/version.go` was never bumped (still `0.1.1`), so the release
  carried `dohping_0.1.1_*` archives and binaries reporting 0.1.1, on a v0.1.2
  tag. Published before the drift was caught (user released the draft in good
  faith; connection drops throughout).
- **v0.1.2 is kept live** with a release-body note: assets are mislabeled, use
  v0.1.3. NOT deleted — honest record, no disappearing history.
- **v0.1.3** (PR #9 → tag → CI draft → user published): correct constant,
  correctly-named `dohping_0.1.3_*` archives, binary verified `dohping 0.1.3`.
  Live at github.com/neutralvibes/dohping/releases/tag/v0.1.3.
- One wrinkle: the v0.1.3 release was initially published UNTAGGED (the tag
  wasn't live on origin when the user hit publish — it had been pushed but the
  connection dropped before it registered). Fixed by attaching the release to
  the v0.1.3 tag. Lesson: ALWAYS confirm the tag is live on origin before
  saying a draft is ready.

**The structural fixes this produced (all merged):**
- `scripts/release.sh` now REFUSES to build a release when the release tag and
  the version constant disagree (hard error, exit 1).
- `scripts/release-gate.sh` (NEW, in PUBLIC_ITEMS, runs in CI as a "Release
  gate" step after the quality gate): fails when version-constant != tag,
  CHANGELOG lacks the entry, tag not live on origin, release not a DRAFT, or
  draft assets misnamed / binary --version != version.
- `PUBLISH-CHECKLIST.md` now references the enforced script + the rig-INDEX
  regeneration as a required publish step.

**The real lesson (user's own words):** the markdown checklist was never the
problem — it listed every gate in plain words. The failures happened because it
was NOT READ and followed. A script is a backstop, not a substitute for the
habit: READ PUBLISH-CHECKLIST.md and run it before every push and every "ready
to publish". Never rationalize a skipped procedure as a tooling gap.

---

## Rig INDEX — now auto-derived

The rig's `served/projects/dohping/INDEX.txt` is a build ledger. It went stale
because updating it was a manual chore with no trigger. Fixed:
- `update-index.sh` in the served dir REGENERATES the index from the actual
  files: runs `--version` on the linux builds (cross-platform binaries show `?`
  — can't execute them here, but their sha is recorded), hashes every binary,
  separates plain SHIPPED / versioned test / release archives.
- Regenerating it is now a **required publish step** in PUBLISH-CHECKLIST
  (agent runs it as part of any rig publish). The index is derived from
  reality, not memory.

---

## Current release pipeline (proven, follow for v0.1.4+)

1. Changes as PRs → user merges (agent never pushes/merges main).
2. **`bash scripts/release-gate.sh`** runs in CI as a Release-gate step (after
   the quality gate): version-constant vs tag, CHANGELOG entry. A drift fails
   the PR before merge — what would have caught v0.1.2.
3. **Bump `internal/version/version.go`** to the release version FIRST and add
   the CHANGELOG entry — this is the step that was skipped for v0.1.2.
4. Tag = trigger: `git tag -a vX.Y.Z -m msg <main>` + push. **Confirm the tag
   is live on origin** (git ls-remote) before anything else. CI gates, then the
   release job builds `dist/*` via `scripts/release.sh`, attaches as DRAFT.
5. **Pre-publish: `bash scripts/release-gate.sh --tag vX.Y.Z --release-url <draft>`**
   — confirms tag live on origin + release is a DRAFT + assets correctly
   named/versioned.
6. **Regenerate the rig INDEX** (update-index.sh) as part of the publish.
7. User reviews + publishes. Agent never publishes.
8. Redo: delete draft + tag (local + remote), re-tag on new main.
`release.sh`: `dohping_<ver>_<os>_<arch>.tar.gz`/`.zip`, plain binary inside,
README+CHANGELOG+LICENSE, SHA256SUMS. Test from CLEAN tree (`rm -rf dist`).
`go tool dist list` = ground truth (darwin/windows have no 32-bit arm).

---

## CI gate — unified (#90/#91)

- **#90**: govulncheck was absent from the CI gate (local `check.sh` ran it; CI
  never installed it — release path never scanned). Fixed: CI installs
  `govulncheck@v1.7.0` pinned (= local) and runs it in the gate.
- **#91**: gate DUPLICATED (check.sh vs inline ci.yml drifted). FIX: CI Quality
  gate = `bash scripts/check.sh --ci`. One definition everywhere; args for
  environment behaviour: no-args SKIPs missing tools, `--ci` FAILs on them.
- Plus the new **Release-gate step** (`scripts/release-gate.sh`) from #94.
- CI-only steps (fresh build, debug-compiled-out, PTY probe, release matrix)
  stay BELOW the gate, explicitly not part of the green definition.

---

## Session lessons (user's points — carry forward)

1. Read the file/repo state BEFORE advising. LICENSE holder was never blank
   (`github.com/neutralvibes`, aa88d0b) — I claimed otherwise from stale memory.
2. Never assert a tool is in the release gate without reading the workflow that
   guards it (govulncheck).
3. Don't re-derive settled decisions. "Propose ≠ implement" and "no
   front-running" apply.
4. When the user says "get rid of X," do EXACTLY X — don't expand scope.
5. Position matters as much as heading level (a Linux section after Windows
   reads as part of Windows).
6. No em-dashes in public prose. Brief > verbose in changelogs.
7. Never blame browser cache without proof (the ETag fix proved the real cause).
8. Don't park runnable features as "unverified-forever (probe if asked)" — if
   it runs on this box, verify it myself, unprompted. "Tests for everything" is
   the standing standard. (DECISIONS #92)
9. Checklists are ENFORCED or they are decoration. (DECISIONS #94/#95)
10. The markdown checklist was never the problem — it was not READ and
    followed. A script is a backstop, not a substitute for the habit. Never
    rationalize a skipped procedure as a tooling gap. (user: "There is nothing
    wrong with markdown that had no real gates if you fucking read it")
11. PRs/commits must read like the user wrote them — terse, human, no process
    metadata. If it reads like a machine, halve it and rewrite. (user: "you
    write a PR that reads like a fucking computer")

---

## Windows — full picture

- **#87 ACCEPTED**: color in classic cmd.exe/PowerShell (ENABLE_VIRTUAL_TERMINAL_
  PROCESSING via internal/console); ASCII spinner fallback where block glyphs
  unavailable; `-n` shortcut. Verified by user on real hardware.
- **#88**: Defender flags Windows build `Trojan:Win32/Wacatac.C!ml` — ML false
  positive (known on Go binaries; even unchanged old bytes re-flagged when model
  updated). Not resolvable in code. Mitigations: `-s -w` strip (all platforms)
  + README Defender note. Signing off the table (cost). Ship with the note.

## Rig + SPA lessons (file-serve-rig skill holds detail)

- SPA downloads route non-media via `/raw/` (plain path hits SPA catch-all).
- Cache: `no-store` + `Pragma: no-cache` + `Expires: 0` on every response.
- **ETag trap**: overwriting a served file can leave Caddy serving new bytes
  with OLD ETag/Last-Modified → browser gets false 304 and shows stale forever.
  Fix: `touch` the file after copy, verify `curl -I -H 'If-None-Match: <old>'`
  returns 200.
- The real dohping area is `served/projects/dohping/`, NOT root `served/dohping/`.

---

## Project conventions

- Spec-first; every change traces to spec/report in DECISIONS.md; gates re-run;
  CHECKPOINT.md rewritten at boundaries. Commit at every stage/fix; ASK if
  ambiguous. User's direct experimental evidence = ground truth. Genuine pushback
  + sober voice (say what pays AND what doesn't). Don't commit taste decisions
  without asking. Public prose: plain human phrasing, NO em-dashes.

## Environment (critical — read before running anything)

```sh
export GOROOT=/home/hermes/.hermes/go-toolchain/go
export GOCACHE=/home/hermes/.hermes/go-cache
export PATH="$GOROOT/bin:$PATH"        # ORDER MATTERS
```
go1.26.5 linux/amd64, module `dohping`. Deps: x/net v0.58.0, x/term v0.45.0.
Linters at `~/go/bin/`. **gosec from prebuilt tarball, NEVER `go install gosec@latest`**
(compiles LLM-SDK deps, OOM, #63). govulncheck v1.7.0 at ~/go/bin. Sandbox ICMP:
0 caps → raw EPERM, /bin/ping works → tier 3 exists (settled, don't re-litigate).

## Publishing to the user's laptop

- Rig is the ONLY delivery path (bare MEDIA: lines don't reach laptop in
  remote-gateway). Root: `/home/hermes/.hermes/user/rig/served/`.
- `https://files.hermes.home/projects/dohping/` (auth hermes / rig pass).
- **VERSIONED FILENAMES**: every build also publishes as
  `dohping-<os>-<arch>-<sha8>` + INDEX.txt. Plain name = latest SHIPPED;
  Windows renames same-name re-downloads ("(1)"). Tell the user the sha.
- Publish after rebuild: `cp dist/* <served>/projects/dohping/`, verify via
  `/raw/` → 200 + sha served-vs-dist, **then touch the files** (ETag trap),
  **then regenerate the INDEX** (update-index.sh).
- GitHub assets use PLAIN names. Full rig knowledge: skill `file-serve-rig`.

## Test/debug workflow

- Deterministic down: `192.0.2.1`. TCP mode for CI (no ICMP caps).
- PTY capture: `script -qec` (sandbox PTY ONLCR hides column bugs — RENDER
  through termScreen/noOnlcrScreen and assert the screen, not escape lists).
- Animation: drive `Tick()` with injected clock. Resize: termScreen emulator
  (resize vs reflowResize). Real-PTY: `python3 scripts/pty-resize-probe.py`
  (fresh build first — stale-build trap). Scenarios: window /
  window-same-band / window-subfloor / plain / **quit** (sends `q` to the pty
  master, asserts exit 0 — the interactive q-quit contract, #93).
- **PASTE BLINDNESS**: user's terminal copies wrapped lines as ONE logical line;
  pastes never prove no-wrap. User's screen = evidence; emulator + PTY = proof.
- Resize forensics: `DOHPING_DEBUG=<path> dohping --window HOST`. Ask for the
  log, never widths. Regression tests accompany every acceptance fix.

## Key file map

```
cmd/dohping/main.go          entry
internal/cli/                flags, validation, help
internal/app/                Main, run loop, signals, summary
internal/ping/               Probe iface, ICMP (3-tier + pingcmd), TCP
internal/state/              engine, hysteresis, RTT stats
internal/output/             Layout, plain Display, Window (reclaim)
internal/theme/              ANSI color rules
internal/logx/               text/JSON log files
internal/debugx/             DOHPING_DEBUG (#74)
internal/signalx/            SIGINT/SIGTERM, SIGWINCH (unix/windows)
internal/console/            Windows VT enable + glyph support (#87)
scripts/release.sh           cross-compile + SHA256SUMS (strips ALL targets; tag/constant guard)
scripts/release-gate.sh      ENFORCED release checklist (version/changelog/tag/draft/assets)
scripts/pty-resize-probe.py  real-PTY resize scenarios (incl. quit)
scripts/check.sh             SINGLE gate: gofmt/vet/race(g2)/lint/staticcheck/gosec/govulncheck; --ci mode
scripts/publish-github.sh    derive public snapshot (publish/) — see below
build-docs/                  INTERNAL, tracked, never published
publish/                     INTERNAL derived public repo (gitignored)
assets/                      dohping-demo.gif (hero, in PUBLIC_ITEMS)
docs/                        terminal-rendering.md (public)
LICENSE                      MIT (public) — holder = github.com/neutralvibes (set)
dist/                        release artifacts
```

## GitHub publish structure

- **`build-docs/`** tracked in private repo, never published. **`publish/`**
  (gitignored) = separate git repo with exactly PUBLIC_ITEMS (`README.md LICENSE
  go.mod go.sum cmd internal docs assets
  scripts/{release.sh,release-gate.sh,check.sh,pty-resize-probe.py}
  .github/workflows/ci.yml CHANGELOG.md`), derived by publish-github.sh: clean
  tree required, asserts nothing forbidden + committed list == contract EXACTLY,
  commits snapshot named after private sha. NEVER pushes alone — agent adds
  origin, pushes branch, opens PR; user merges.
- GitHub side: fine-grained token scoped to dohping + branch protection on main.
  Token needs `workflow` scope to push `.github/workflows/`.
- Commit-message hygiene: public commits read as the user wrote them.
  Gate: `build-docs/PUBLISH-CHECKLIST.md` before EVERY push + pre-commit hook.
- Rulesets, not classic branch protection. Final config (proven on repotest):
  **Admin bypass "For pull requests only"** — force-push/direct push rejected,
  owner's PR merges allowed, non-admin gated by count=1. dohping action PENDING
  (user UI). Token can't edit rulesets (403).

## Held plan: reusable terminal test rig (2026-08-18, HELD)

**Do NOT build without explicit go.** CI-ready pty-probe.py
(`--binary/--scenario/--cols/--rows`), CI workflow template, go-cli-development
skill pointer. Non-goals: no Go-module extraction of termScreen/noOnlcrScreen,
no plugin framework/YAML/classes, no interactive-input injection. CI: TCP mode;
Windows CI = unit only.

## Pending / parked

- **dohping ruleset** switch (user UI action) — still open.
- **Held**: reusable terminal test rig (above).
- **Rig cleanup** (user's call): superseded `dohping-windows-amd64-14338619.exe`,
  dead `dohping-demo-synthetic-20260821.gif`, and the rig still carries v0.1.0-era
  plain builds (dohping-linux-amd64 reports 0.1.0) that predate the v0.1.2/0.1.3
  releases — the rig has NOT been updated with the released v0.1.3 binaries yet.
- **Unverified-forever candidates**: darwin/windows binaries (never run here).
- The three local candidates (`-p tcp`, `--log-file`, `--timestamp-format rfc3339`)
  were verified live 2026-08-26 (#92) — no longer parked.
