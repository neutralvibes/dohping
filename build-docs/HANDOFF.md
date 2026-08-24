# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `SPECIFICATION.md` (the contract),
`DECISIONS.md` (fix rationale), `CHECKPOINT.md` (gate status),
`PROGRESS.md` (timeline), `SPEC-window-resize-reclaim.md` (shipped resize
design). `README.md` is the user-facing doc.

**Status: build complete. All 6 phases green. Resize resolved (#78 shipped),
state-change clobber fixed + accepted (#79). README v2 with hero GIF baked in
(2026-08-24). Publish gate: PR #1 open, CI nursed green, final push/PR still
user-approved + parked.**

---

## SESSION 2026-08-24 — README v2 + hero GIF shipped (READ FIRST if resuming)

**The demo GIF is DONE and baked in.** The parked synthetic GIF is dead. The
working hero is a REAL recording of a flapping LAN host (`192.168.1.182`, up →
down → up → down, FAILS climbing), default invocation `dohping HOST`, no flags,
no summary block. Fixed and cropped: 1458×580 window → tight 1390×310 content
box (kills the void + window chrome), palette-quantized to 28KB, original
timing kept (38 frames, 25.71s).

- Repo: `assets/dohping-demo.gif` (sha `62c1bdf4…`); README v2 line 7 references
  it as `assets/dohping-demo.gif` (relative, GitHub-portable).
- Rig preview: `served/projects/dohping/assets/dohping-demo.gif`; the rig README
  uses the ABSOLUTE path `/projects/dohping/assets/dohping-demo.gif` because the
  SPA's markdown renderer resolves relative image refs against the `/raw/` data
  channel (which serves listings/text, NOT media) — a relative ref breaks there.
  This rig/repo divergence is deliberate; the repo form carries to GitHub.
- `README.html` on the rig is DEAD and removed (2026-08-24): the rig's SPA
  renders markdown itself, md2html.py is not used for it anymore.
- `docs/terminal-rendering.md` (the README-linked resize internals doc) is now
  git-tracked. Old synthetic GIF removed everywhere ("old stuff should die").
- Dev-only demo pipeline scripts (`scripts/{compose,record}-*.py`,
  `record-shell-demo.py`) + `build-docs/README-v2-draft.md` are gitignored —
  private dev tools, never published, and they were tripping the publish gate's
  clean-tree requirement.
- `assets` added to `PUBLIC_ITEMS` in `scripts/publish-github.sh`. Publish gate
  re-run clean: 52 files, both assertions pass, `publish/assets/dohping-demo.gif`
  byte-identical to repo. Working tree clean.

**Publish gate residual (still parked, user-approved):** re-derive `publish/`
(done above), push branch, open PR, **WATCH CI TO GREEN** — never report done on
a red/unknown CI. README v2's assets ref needs `assets/` in PUBLIC_ITEMS (done).
No decision/process references in any file that reaches GitHub — keep the scrub
intact.

---

## Session lessons (user's points — carry forward)

1. Read a file in full BEFORE asking whether it should be public.
2. Never report done without CI green; verify the PUBLISHED tree, not just private.
3. No DECISIONS #N / HANDOFF / SPEC-window / sandbox-process refs in public files.
4. golangci-lint v2 default set is stricter than v1.64.8; pin must match the
   module's Go version.
5. Windows gotcha: `syscall.ECONNREFUSED` is invented (APPLICATION_ERROR+22);
   real refused is `WSAECONNREFUSED` (10061) via `golang.org/x/sys/windows`.
6. Demo craft (see `demo-craft` skill): default first, shortcuts look easy,
   state change is the story, no summary block, small tight frame, compose don't
   record, verify frames by LOOKING. A 200 or a rendered file ≠ correct.
7. The rig SPA resolves markdown image refs against `/raw/`; use absolute public
   paths for images on the rig, relative `assets/` paths for GitHub.

---

## Resize behavior — the current contract (window mode)

- Nothing writes while the width is moving (300 ms settle; further changes
  restart the clock).
- Above the essentials floor (~46 cols): every width change settles to exactly
  ONE clean block in place. Crossings RECLAIM via reflow-aware in-place
  repaint (SPEC-window-resize-reclaim.md). No frozen copy, no scrollback
  reliance.
- Below the floor / when R > terminal height: freeze → restart-below fallback.
- Plain view unchanged: freeze-and-restart with the `-` resize mark (user-approved, #69).
- Column trim (#71): below 79 cells the rightmost columns drop (FAILS→AVG→MAX→MIN).
- Accepted residual (documented in README): mid-drag transient rewraps; settle
  can land one row off if ConPTY's physical cursor is off after reflow —
  self-corrects next repaint. Non-reflowing terminals (xterm/tmux/sandbox PTY)
  can't exercise the crossing reclaim (CPR lies on ConPTY, microsoft/terminal#18725).

## How it got here (landmarks; rationale in DECISIONS.md)

| Round | Fix | Outcome |
|---|---|---|
| 9–11 (#67–69) | Freeze-and-restart after CPR re-anchor (#66) failed on ConPTY | plain view user-approved (#69) |
| 13 (#71) | Column trim (user's idea) | kept |
| 15 (#73) | Defer same-band repaints | REVERTED |
| 16 (#74) | `DOHPING_DEBUG` debug-log facility (evidence path) | kept |
| 17 (#75) | Reflow-aware in-place reclaim | SHIPPED after retest |
| 18–19 (#76–78) | B elected on stale binary, rejected on direct test; reclaim promoted | reclaim shipped (#78) |
| 20 (#79) | Plain-mode state-change clobber (resize `-` fired on every finalize) | FIXED + ACCEPTED |

The #76 rejection was the unversioned-filename trap (Windows renames same-name
re-downloads) — fixed in §6. The debug log (#74) remains the evidence path for
any future resize report.

---

## 1. What it is

A single-host ping/monitoring CLI (Go 1.26.5), "D'oh!"/Homer play, pronounced
"dopping". Monitors one host with ICMP (raw → unprivileged ping socket → system
`ping` fallback) or TCP connect, tracks up/down/error with hysteresis and RTT
stats, renders plain-line or fixed window mode, optional text/JSON logging,
interactive `q` quit, predictable exit codes.

Project dir: `/home/hermes/.hermes/user/projects/dohping` — git repo (identity
`Hermes Agent <hermes@hermes.home>`, branch `master`). Commit at every
stage/acceptance fix; use git to reverse. If in doubt, ASK, don't assume.

## 2. Verified working (user-confirmed or gate-proven)

- Plain line mode: header, columns, live line in place, finalization, non-TTY
  hygiene. Byte-identical to spec §7.4 example.
- Window mode: in place on the normal terminal, no alt-screen/clear, block
  stays after exit, summary below. Resize reclaim shipped (#78), proven in-unit
  through the reflowing-emulator model + real-PTY probe (4 scenarios PASS).
- Flags on either side of HOST (GNU-style permutation, #55); `--` terminates.
- `-i`/`-t` bare seconds plus full durations (#57); zero/negative rejected.
- Clean left-aligned exit summary even without ONLCR (#56).
- Liveness bar `▁▃▅▇` on a 1s ticker independent of probe cadence (#59–61).
- Three-tier ICMP verified live (`1.1.1.1` up, `::1` up, `192.0.2.1` down).
- Errors: consecutive errors = one line; permission problems exit 3 + hint.
- Signals: SIGINT→130, SIGTERM→143, q→0, `--count`→0.
- Cross-compile matrix (5 binaries) reproducible via `scripts/release.sh`.
- Gates: 7/7 race-clean, gofmt/vet, golangci-lint/staticcheck/gosec clean.

## 3. Acceptance fixes worth remembering (full history: DECISIONS.md)

| # | Fix | User report |
|---|---|---|
| 53 | Window mode in place, no alt-screen/clear | "nothing about clearing the screen..." |
| 54 | Explicit column reset on redraw | fragmented rows |
| 64 | Resize handled, platform-split (HOST elastic, rune math, SIGWINCH / 1s tick) | "Not handling terminal resize" |
| 65 | Plain live line width-aware | "plain mode also needs to be width aware" |
| 66 | DSR/CPR re-anchor | FAILED on ConPTY → replaced by #67 |
| 67–69 | Freeze-and-restart; `-` mark + STATE/`?` | plain view approved |
| 71 | Window-mode column trim below 79 cells | "My terminal doesn't live in any band" |
| 74 | `DOHPING_DEBUG` debug-log facility | "we should have had a facility for a debug logger" |
| 75/78 | Reflow-aware in-place reclaim | "It is neither a defense or true..." |
| 76–77 | B episode: stale-binary rejection, retest, promoted | "0014967e is out..." |
| 79 | State-change clobber gated on real resize | "only shows 2 lines at a time" |

## 4. Pending / next actions

- **Publish sequence (parked at push, 2026-08-19 "Go"):** identity set
  (`26578830+neutralvibes@users.noreply.github.com`), baseline amended, rebased
  onto `origin/main`, LICENSE conflict resolved (keep ours), branch
  `publish-initial` created. Push gate EXECUTED (Workflows permission added).
  CI nursed to green (golangci-lint v2.13.1, all 27 issues fixed, `da861a1`).
  NEXT: re-derive `publish/`, push `publish-initial`, PR, tag v0.1.0 + release
  with plain assets + SHA256SUMS. **README v2 assets ref now covered.**
- **dohping ruleset (2026-08-19):** switch Admin bypass from "always" to "for
  pull requests only" (proven config on repotest). User UI action — token can't
  edit rulesets (PUT → 403).
- **Held plan — reusable terminal test rig (§8, 2026-08-18):** user approved,
  explicitly HELD until go. CI-ready pty-probe + workflow template + skill pointer.
- **Unverified-forever candidates** (probe if asked): `-p tcp`, `--log-file`
  output, `--timestamp-format rfc3339`, darwin/windows binaries (never run on
  those OSes).

## 5. Environment (critical — read before running anything)

```sh
export GOROOT=/home/hermes/.hermes/go-toolchain/go
export GOCACHE=/home/hermes/.hermes/go-cache
export PATH="$GOROOT/bin:$PATH"        # ORDER MATTERS
```

- go1.26.5 linux/amd64, module `dohping` (local-only). Deps: x/net v0.58.0, x/term v0.45.0.
- Linters at `~/go/bin/{golangci-lint,staticcheck,gosec}`. **Install gosec from
  the prebuilt release tarball, NEVER `go install gosec@latest`** (compiles the
  LLM-SDK deps, OOM'd the sandbox, #63).
- Sandbox ICMP: 0 effective caps → raw sockets EPERM; `/bin/ping` works via
  sandbox elevation → tier 3 exists. Settled, user-verified — don't re-litigate.
- Build: `go build ./cmd/dohping`. Release: `bash scripts/release.sh` → `dist/`.

## 6. Publishing to the user's laptop

- Rig is the ONLY delivery path (bare MEDIA: lines don't reach the laptop in
  remote-gateway mode). Serve root: `/home/hermes/.hermes/user/rig/served/`.
- `https://files.hermes.home/dohping/` (basic auth: user `hermes`, password in
  Hermes memory — re-share only if user asks).
- **VERSIONED FILENAMES (2026-08-18 lesson):** every build also publishes as
  `dohping-<os>-<arch>-<sha8>` plus `INDEX.txt` history. Plain name = latest
  SHIPPED build; Windows renames same-name re-downloads ("(1)") — never rely on
  the plain name alone for a test round; tell the user the versioned name or sha.
- Publish after rebuild: `cp dist/* <served>/dohping/` then verify
  `curl -sku hermes:<pass> -o /dev/null -w "%{http_code}" https://files.hermes.home/dohping/dohping-linux-amd64` → 200 and sha served-vs-dist.
- Current shipped linux-amd64: #79-fixed source, dist sha `0ac183e1…` (rig
  carries current builds only; no versioned artifacts since 2026-08-23 reorg).
- GitHub release assets use PLAIN names; sha prefix is a rig/local convention.
- Full rig knowledge: skill `file-serve-rig`.

## 7. Test/debug workflow that works

- Deterministic `down`: `192.0.2.1`. TCP mode for CI (no ICMP caps on runners).
- PTY capture: `script -qec` — but the sandbox PTY applies ONLCR which hides
  column bugs. RENDER through a terminal emulator and assert the visible screen
  (`termScreen` in `internal/output/window_test.go`); escape-list assertions are
  not enough. Also assert through `noOnlcrScreen` (`internal/app/app_test.go`)
  where line-column drift is possible.
- Animation tests drive `Tick()` directly with an injected clock (frame at col 47).
- Resize tests: inject width via `newTestWindowResizable`, render through
  `termScreen`. The emulator models both worlds: `resize()` (non-reflowing,
  xterm) and `reflowResize()` (reflowing, logical-line re-wrap — the user's
  terminal). Above the 46-cell floor ANY width change settles to ONE block;
  below the floor same-band wraps in place, crossings freeze → two blocks.
- Real-PTY proof: `python3 scripts/pty-resize-probe.py` (fresh build first —
  `rm -f /tmp/dohping-test`; stale-build trap). Scenarios: window, window-same-band,
  window-subfloor, plain.
- **PASTE BLINDNESS (2026-08-18):** the user's terminal (Windows Terminal →
  WSL2) copies a wrapped line as ONE logical line — pastes never prove no-wrap.
  The user's screen is the evidence (screenshots); emulator tests + PTY probe
  are the proof.
- Resize forensics: `DOHPING_DEBUG=<path> dohping --window HOST` logs its own
  width telemetry (`winch`/`tick`/`resize`/`redraw`). Ask the user for the log,
  never widths or paste geometry.
- Regression tests accompany every acceptance fix (established pattern, #50–79).

## 8. Project conventions (user's way of working)

- Spec-first, no drive-by fixes; every change traces to spec/report, recorded in
  DECISIONS.md; gates re-run; CHECKPOINT.md rewritten at boundaries.
- Git discipline: commit at every stage/fix. If genuinely ambiguous, ASK.
- Genuine pushback + sober voice: say what pays AND what doesn't.
- Don't commit taste decisions to memory/vault/skills without asking.
- User's direct experimental evidence is ground truth — don't re-litigate settled
  findings with indirect logs.
- **Public-facing prose (README, docs/): plain human phrasing, NO em-dashes.**
  An em-dash is an AI giveaway (humanizer skill). Applies to everything that
  reaches GitHub.

## 9. Key file map

```
cmd/dohping/main.go          entry
internal/cli/                flag parse, validation, conflicts, help
internal/app/                Main, run loop, signals, key reader, summary
internal/ping/               Probe iface, ICMP (3-tier incl. pingcmd.go), TCP
internal/state/              engine, hysteresis, RTT stats, events
internal/output/             Layout/format, plain Display, Window (in-place + reclaim)
internal/theme/              role-based ANSI color rules
internal/logx/               text/JSON log files
internal/debugx/             optional DOHPING_DEBUG diagnostic logger (#74)
internal/signalx/            SIGINT/SIGTERM listen, SIGWINCH (unix/windows split)
scripts/release.sh           cross-compile + SHA256SUMS
scripts/pty-resize-probe.py  real-PTY resize scenarios
scripts/publish-github.sh    derive the PUBLIC repo snapshot (publish/) — see §10
build-docs/                  INTERNAL build-process docs — TRACKED, never published
publish/                     INTERNAL derived PUBLIC repo (gitignored) — GitHub gets exactly this
assets/                      dohping-demo.gif (hero, committed, in PUBLIC_ITEMS)
docs/                        terminal-rendering.md (public, README-linked)
LICENSE                      MIT (public) — holder line still a placeholder
dist/                        release artifacts (5 binaries + sums)
```

## 10. GitHub publish structure

Two layers keep internal build docs out:
- **`build-docs/`**: all build-process docs, TRACKED in the private repo but
  never published — `publish/` derives only from PUBLIC_ITEMS. The private repo
  is never pushed.
- **`publish/`** (gitignored): a separate git repo with exactly the public
  files, derived by `scripts/publish-github.sh`: requires a clean tree, copies
  PUBLIC_ITEMS (`README.md LICENSE go.mod go.sum cmd internal docs assets
  scripts/release.sh scripts/pty-resize-probe.py .github/workflows/ci.yml`),
  asserts nothing forbidden + the committed list equals the contract EXACTLY,
  commits a snapshot named after the private sha. NEVER pushes on its own —
  agent adds origin, pushes to a branch, opens a PR; user approves the merge.
- GitHub side (set 2026-08-18): fine-grained token scoped to dohping (contents
  write, PRs RW, issues read, actions read) + branch protection on main (PR + 1
  approval, no force-push). Token needs `workflow` scope to push `.github/workflows/`.
- Commit-message hygiene: public commits read as the user wrote them — no agent
  tells. Gate: `build-docs/PUBLISH-CHECKLIST.md` (consult before EVERY push) +
  a pre-commit hook in publish/.git refusing bad identity.
- LICENSE = MIT, holder line still `<YOUR NAME HERE>` — user must fill before
  first push.
- Pitfalls paid: bare `dohping` in .gitignore matches the cmd DIRECTORY
  (root-anchor: `/dohping`); .gitignore must not appear twice; a nested repo is
  staged as a gitlink unless the parent ignores `publish/`.

## 11. Rehearsal run — neutralvibes/repotest (2026-08-19, ALL STEPS GREEN)

Full dry-run of the publish flow on a throwaway repo before touching dohping.
Key results: agent-authored first commit amended (`--amend --reset-author` →
user identity, author AND committer); push branch + PR via REST (branch must
share history with main — rebase onto origin/main first, keep the seed's files
on conflict); user squash-merge; annotated tag v0.1.0; release + plain-named
assets via `uploads.github.com`; branch protection via API → 403 (no Admin
scope → user sets in UI).

**Protection mechanism = RULESETS, not classic branch protection.**
- Bypass ≠ admin: the token has no Administration permission (admin APIs 403)
  but GitHub resolves its actor as repo-admin for ENFORCEMENT, so it auto-bypasses
  rulesets with only a warning. Verify via `GET /rulesets/{id}` →
  `current_user_can_bypass`.
- **FINAL CONFIG (proven behaviorally on repotest): Admin bypass "For pull
  requests only"** (`bypass_mode: pull_request`). force-push REJECTED, direct
  push REJECTED, owner's own PR merges ALLOWED (no self-approval deadlock);
  non-admin contributors still gated by count=1. dohping action pending: switch
  its Admin bypass from "always" to "for pull requests only" (user UI action).
- Protection guards contributors, not the owner's token; real protection for
  main stays procedural (agent never pushes/merges main, pre-commit identity
  hook, user oversight).
- The token CANNOT edit rulesets (403) — the escape hatch is the user's.
  Ruleset conditions = `~DEFAULT_BRANCH` only → feature-branch pushes unaffected.

---

## Held plan: reusable terminal test rig (2026-08-18, HELD)

**Do NOT build without the user saying go.** Why: 3 of the last 5 projects were
CLI-based; the rig lessons cost many acceptance rounds on dohping and should
carry forward. Must be CI-ready (stock toolchain + stock Python, no agent, no
sandbox). Deliverables: generalized `pty-probe.py` (`--binary/--scenario/
--cols/--rows`), CI workflow template, `go-cli-development` skill pointer.
Non-goals (agreed): no Go-module extraction of termScreen/noOnlcrScreen, no
plugin framework/YAML/classes (dict of functions is the ceiling), no
interactive-input injection hook. CI notes: probe uses TCP mode (no ICMP caps);
Windows CI runs unit tests only.
