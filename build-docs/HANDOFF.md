# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `SPECIFICATION.md` (contract), `DECISIONS.md`
(rationale), `CHECKPOINT.md` (gate status), `PROGRESS.md` (timeline). `README.md`
is the user-facing doc.

**Status: build complete, all phases green, v0.1.1 LIVE on GitHub** (archives,
CI-built, all platforms incl. ARM; v0.1.0 dropped, never posted). **v0.1.2 is
assembled and unblocked** — the publish sequence below is the only remaining
work. README restructured, CHANGELOG finalized, Windows resolves as ship.

---

## v0.1.2 — current state (READ FIRST)

**Content ready (all committed):**
- README: restructured. Order = Why-not-ping → Quick start (with
  "Eager to try it? Just head to [Installation](#installation).") → What makes
  it different → When to use it → Usage → Examples → **Installation** (moved
  after Examples — section had grown) → Display modes → Logging → Dev → License.
  - Permissions is `####` **under Linux installation** (was after Windows — the
    fix the user pushed on repeatedly; position, not heading level).
  - Defender note is `####` under Windows installation (not `###`).
  - setcap framed as "…and you want to run it without `sudo`".
  - No em-dashes anywhere (public-prose rule).
- CHANGELOG: v0.1.2 entry named (not [Unreleased]), date 2026-08-25, compare
  link added, boilerplate "format is based on Keep a Changelog…" line **removed**
  (user: "that reads like a machine"). Structure = `### Added` + `### Fixed`
  headings (user: do NOT remove these). Entries:
  - Added: Defender README note (brief, ends at "…and how to handle it.");
    `-n` shortcut for `--no-color` (a feature).
  - Fixed: Windows color in cmd/PowerShell + ASCII spinner fallback (#87);
    ICMP fallback to system `ping` when sockets fail (#85/#86); release
    binaries stripped.
- `scripts/release.sh`: **`-s -w` strip now applies to ALL targets** (was
  Windows-only; user: "should have been done as standard", "it is not just
  windows"). Verified clean 8-target build; linux amd64 stripped + runs +
  `--version` OK. Header comment updated.
- **Windows ship-vs-defer: RESOLVED — Windows SHIPS** (user: "windows wont be
  deferred, the README handles it for now"). No signing (cost doesn't pay back).
  README Defender note is the documentation.
- Rig `served/projects/dohping/`: Windows build `dohping-windows-amd64-f6ea1c7d.exe`
  (stripped, verified), plain name aliases it, both release zips, README +
  CHANGELOG (hash-matched to repo), INDEX.txt + SHA256SUMS current.

**NEXT (publish, unblocked):**
1. **Recommend a test PR first** to prove the updated CI (govulncheck install +
   `check.sh --ci`) on a real runner before it guards a release — the workflow
   hasn't run on GitHub yet.
2. Re-derive `publish/` (one commit behind; CHANGELOG in PUBLIC_ITEMS), push
   branch, PR, tag v0.1.2, CI builds matrix + draft, **user publishes**.
3. Ruleset switch (Admin bypass → "for pull requests only") — user UI action.

---

## CI gate — unified (#90/#91, 2026-08-25)

- **#90**: govulncheck was absent from the CI gate (local `check.sh` ran it;
  CI never installed it — release path never scanned). Fixed: CI installs
  `govulncheck@v1.7.0` pinned (= local) via `go install` (no prebuilt tarballs
  exist; first attempt 404'd) and runs it in the gate.
- **#91**: the gate was DUPLICATED (check.sh vs inline ci.yml steps drifted).
  FIX: CI Quality gate = `bash scripts/check.sh --ci`. One definition everywhere;
  args for environment behaviour (user's design): no-args SKIPs missing tools,
  `--ci` FAILs on them (runner must have provisioned everything).
- CI-only steps (fresh build, debug-compiled-out, PTY probe, release matrix)
  stay BELOW the gate, explicitly not part of the green definition.

---

## Windows — full picture (2026-08-25)

- **#87 ACCEPTED**: color in classic cmd.exe/PowerShell (ENABLE_VIRTUAL_TERMINAL_
  PROCESSING via internal/console); ASCII spinner fallback where block glyphs
  unavailable (output.SetFrames + console.SupportsUnicodeGlyphs); `-n` shortcut.
- **#88**: Defender flags Windows build `Trojan:Win32/Wacatac.C!ml` — ML false
  positive (known on Go binaries; even unchanged old bytes re-flagged when model
  updated). Not resolvable in code. Mitigations: `-s -w` strip (all platforms)
  + README Defender note. Signing off the table (cost). Ship with the note.
- Verified by user on real hardware; test build `dohping-windows-amd64-14338619.exe`
  superseded by stripped `f6ea1c7d`.

## Rig + SPA lessons (file-serve-rig skill holds detail)

- SPA downloads route non-media via `/raw/` (plain path hits SPA catch-all).
- Cache: `no-store` + `Pragma: no-cache` + `Expires: 0` on every response;
  SPA `/raw/` fetches pass `cache: 'no-store'`.
- **ETag trap**: overwriting a served file can leave Caddy serving new bytes
  with OLD ETag/Last-Modified → browser gets false 304 and shows stale forever.
  Fix: `touch` the file after copy, verify `curl -I -H 'If-None-Match: <old>'`
  returns 200. (This was the real cause of the "rig still stale" saga.)
- The real dohping area is `served/projects/dohping/`, NOT root `served/dohping/`.

---

## Session lessons (user's points — carry forward)

1. Read the file/repo state BEFORE advising. LICENSE holder was never blank
   (`github.com/neutralvibes`, aa88d0b) — I claimed otherwise from stale memory.
2. Never assert a tool is in the release gate without reading the workflow that
   guards it (govulncheck).
3. Don't re-derive settled decisions ("strip all builds" was already established;
   I still had it Windows-only). "Propose ≠ implement" and "no front-running" apply.
4. When the user says "get rid of X," do EXACTLY X — don't expand scope (I stripped
   Added/Fixed headings when only the boilerplate line was to go; user: NO).
5. Position matters as much as heading level: a Linux section after Windows reads
   as part of Windows (Permissions fix).
6. No em-dashes in public prose. Brief > verbose in changelogs ("information
   should be brief and to the point").
7. Never blame browser cache without proof (the ETag fix proved the real cause).

---

## Release pipeline (proven, follow for v0.1.2+)

1. Changes as PRs → user merges (agent never pushes/merges main).
2. Tag = trigger: `git tag -a vX.Y.Z -m msg <main>` + push. CI gates, then
   release job builds `dist/*` via `scripts/release.sh`, attaches as DRAFT.
3. User reviews + publishes. Agent never publishes.
4. Redo: delete draft + tag (local + remote), re-tag on new main.
`release.sh`: `dohping_<ver>_<os>_<arch>.tar.gz`/`.zip`, plain binary inside,
README+CHANGELOG+LICENSE, SHA256SUMS. Test from CLEAN tree (`rm -rf dist`).
`go tool dist list` = ground truth (darwin/windows have no 32-bit arm).

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
  `/raw/` → 200 + sha served-vs-dist, **then touch the files** (ETag trap).
- GitHub assets use PLAIN names. Full rig knowledge: skill `file-serve-rig`.

## Test/debug workflow

- Deterministic down: `192.0.2.1`. TCP mode for CI (no ICMP caps).
- PTY capture: `script -qec` (sandbox PTY ONLCR hides column bugs — RENDER
  through termScreen/noOnlcrScreen and assert the screen, not escape lists).
- Animation: drive `Tick()` with injected clock. Resize: termScreen emulator
  (resize vs reflowResize). Real-PTY: `python3 scripts/pty-resize-probe.py`
  (fresh build first — stale-build trap).
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
scripts/release.sh           cross-compile + SHA256SUMS (strips ALL targets)
scripts/pty-resize-probe.py  real-PTY resize scenarios
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
  go.mod go.sum cmd internal docs assets scripts/{release.sh,pty-resize-probe.py}
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

- **dohping ruleset** switch (user UI action).
- **Held**: reusable terminal test rig (above).
- **Unverified-forever candidates** (probe if asked): `-p tcp`, `--log-file`
  output, `--timestamp-format rfc3339`, darwin/windows binaries (never run).
- Old rig build `dohping-windows-amd64-14338619.exe` (superseded) + dead
  `dohping-demo-synthetic-20260821.gif` — user's call to remove.
