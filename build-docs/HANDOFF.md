# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `LAUNCH.md` (build brief), `SPECIFICATION.md`
(the contract), `DECISIONS.md` (78 entries — each fix's rationale), `CHECKPOINT.md`
(gate status), `PROGRESS.md` (timeline). `README.md` is the user-facing doc.
`SPEC-window-resize-reclaim.md` is the design for the shipped resize behavior.

**Status: build complete. All 6 phases green. Window-mode resize is RESOLVED —
the reflow-aware in-place reclaim is SHIPPED (DECISIONS #78; dist sha
8dcce21a…, byte-reproduced via `release.sh`).**

**2026-08-23: plain-mode state-change clobber FIXED (DECISIONS #79).**
`printFinalized` marked the row above the live line with `-` (the #68 resize
artifact) on EVERY status change, not just resizes — the header was replaced
by `-` on the first flip and history never accumulated ("only shows 2 lines
at a time"). Caught by the user's host up/down script for the README GIF.
Fix + `TestDisplayStatusChangeKeepsHistory` committed; dist sha
`0ac183e1…`.

**PUBLISH-GATE STATUS (2026-08-21 session): in progress — PR #1 open, CI
being nursed to green. Read the "SESSION 2026-08-21" section below FIRST if
resuming mid-fix.**

---

## SESSION 2026-08-21 — publish gate + CI fixing marathon (READ FIRST if resuming)

**Where the publish stands:**
- **PR #1 open**: https://github.com/neutralvibes/dohping/pull/1 (branch `publish-initial`). Push gate EXECUTED — the token's **Workflows** permission was added (user action), push no longer refused server-side.
- **CI has run 4 times, all red so far** — each run caught a real defect. Down to the LAST issue (golangci-lint version). **Windows job is GREEN** as of the run on `b3a4f34` (WSAECONNREFUSED fix). Linux red only on golangci-lint.
- Resume point: the in-flight golangci-lint v2.13.1 lint fixes (below) are NOT yet committed.

**Fixes shipped this session (private master, all committed, `check.sh` GREEN locally):**
1. `5fca079` — CI runs the quality gate INLINE (`ci.yml`) instead of `scripts/check.sh`. **check.sh stays PRIVATE** — it references the sandbox and internal process; the user challenged publishing it and was right. This is a deliberate architecture decision: public CI self-contained, private dev gate private.
2. `454cdef` — `scripts/record-demo-cast.py` (asciinema cast recorder for the README demo GIF; dev tool, NOT in PUBLIC_ITEMS).
3. `793f8c2` — debugx timestamp test made timezone-independent (UTC emits `Z`, not `+01:00`); POSIX-only signal + ping tests split behind `//go:build !windows` (`platform_test.go`, `ping_posix_test.go`).
4. `cf0c3ee` — **WSAECONNREFUSED classification**: on Windows a refused TCP dial surfaces as WSAECONNREFUSED (10061); stdlib `syscall.ECONNREFUSED` is an invented value (APPLICATION_ERROR+22) that never matches. Platform-split `isRefused` helper (`refused_posix.go`/`refused_windows.go`) + `tcp_windows_test.go` regression.
5. `9007c87` — **scrubbed ALL decision references from the public tree** (user directive: no DECISIONS #N / HANDOFF / SPEC-window refs in any file that reaches GitHub — they were in `icmp.go`, `app.go`, `debugx.go`, tests, `pty-resize-probe.py`, `ci.yml`). Verified zero matches across the published file list. `grep -rn 'DECISIONS|HANDOFF|SPECIFICATION|SPEC-window|...' <public files>` → clean. KEEP it clean.

**IN FLIGHT — golangci-lint version bump (the current fixing work):**
- Problem: ci.yml pins **golangci-lint v1.64.8**, built with go1.24 → fails on the Go 1.26.5 module: *"the Go language version (go1.24) used to build golangci-lint is lower than the targeted Go version (1.26.5)"*.
- Decision (user-approved): bump to **v2.13.1** (built with go1.27.0; checksum-verified) and **FIX all 27 issues** the stricter v2 default set flags (23 errcheck + 4 staticcheck QF) — not disable them. **DONE — committed `da861a1`** (v2.13.1 reports 0 issues, gate green, Windows clean).
- **Scrub scope decision (user correction 2026-08-21):** "there is nothing wrong with comments per-se." The `(DECISIONS #N)` / SPEC refs in `internal/output/*` (95 refs) are KEPT for v0.1.0 — the surrounding text carries the technical rationale; a blind regex pass mangled gofmt alignment + ASCII art and was reverted. A surgical per-comment scrub of `internal/output/*` is a documented FOLLOW-UP, not in the CI-green critical path. The worst leaks (HANDOFF §, SPEC-window filenames, "sandbox" in app/ping/debugx/ci.yml/pty-probe) are already scrubbed (`9007c87`).
- Local v2 binary: `/tmp/golangci-v2/golangci-lint-2.13.1-linux-amd64/golangci-lint` (note: installer checksum flaked once — download tarball + verify against checksums.txt manually if it recurs).
- **NEXT ACTION: commit the HANDOFF update, re-derive `publish/`, push, WATCH CI TO GREEN.** (README v2 + GIF workstream still parked until CI is green.)
- Fix progress: `internal/app/app.go` DONE (all `fmt.Fprintf`/`Fprintln`/`Close` → `_, _ =` / `defer func(){ _ = pr.Close() }()`).
- **REMAINING lint fixes:**
  - `internal/cli/help.go:11` — `fmt.Fprint` unchecked
  - `internal/output/display.go` — several `fmt.Fprint`/`Fprintln`/`Fprintf` unchecked (lines ~184, 198, 213, 217, 251, 266, 269, 307, 325, 340)
  - `internal/logx/logx.go:100` — QF1003 tagged-switch suggestion; `logx_test.go` — `l.Close`/`l2.Close` unchecked
  - `internal/app/app_test.go:111`, `internal/output/format.go:334`, `internal/output/window_test.go:78` — QF1001 De Morgan's law (rewrite `!(a && b)` → `!a || !b`)
  - `internal/app/run_integration_test.go` — `ln.Close`, `c.Close`, `pr.Close` unchecked
  - `internal/ping/ping_test.go` — `c.Close`, `ln.Close`; `ping_posix_test.go` — `pr.Close` unchecked
- **After fixes**: `check.sh` green (its golangci-lint is the local v1 binary — run the v2 binary manually to prove the CI gate; consider bumping local too), commit, re-derive `publish/`, push, **WATCH CI TO GREEN — never report done on a red/unknown CI** (the session's core lesson).

**README v2 + demo GIF (PARKED 2026-08-21 — user called a break; DO NOT resume without explicit go):**
- Draft: `build-docs/README-v2-draft.md` (private, untracked) → finished README v2 now at repo root `README.md` + copied to the rig at `served/projects/dohping/README.md` (+ README.html rendered via md2html.py, img src fixed to ./dohping-demo.gif).
- Demo GIF: the rig's `dohping-demo.gif` is a SYNTHETIC composition (`scripts/compose-hero-demo.py` — asciinema cast with column-exact frames, no real host/PTY). User's final verdict: **"Still not good. Idea is right, previous lessons lost."** — the one-host state-change concept was right but execution drifted from earlier corrections. PARKED. Do not iterate on the GIF without the user asking.
- User's hard-won demo requirements (see `demo-craft` skill, which captures them): default first/no flags, shortcuts look easy, state change is the story, no summary block, small tight terminal, compose don't record, verify frames by looking.
- GIF pipeline tools: agg at `~/hermes/user/tools/bin/agg`; `scripts/compose-hero-demo.py` (synthetic), `scripts/record-hero-demo.py` (PTY recorder, two-session + listener variant), `scripts/record-shell-demo.py` (general shell recorder).

**Session lessons (the user's points — carry forward):**
1. **Read a file in full BEFORE asking whether it should be public.** "Should X be published?" is answered by reading X + the contract, not by asking.
2. **Never report done without CI green.** I pushed 3× without watching CI; the user had to point at it. Verify the PUBLISHED tree (not just private) and watch the CI run to completion.
3. **No decision/process references in public files.** DECISIONS #N, HANDOFF §, SPEC-window, sandbox-process detail — all scrubbed; the publish structure exists to keep them private.
4. **golangci-lint v2 default set is stricter** (errcheck on by default) — v1.64.8 was silently permissive. The version pin must match the module's Go version.
5. **Windows gotcha**: `syscall.ECONNREFUSED` is an invented APPLICATION_ERROR value; the real refused error is `WSAECONNREFUSED` (10061) via `golang.org/x/sys/windows`.
6. Windows binaries/tests were never run on real Windows before this — the CI windows job is the first real validation.

---

## Resize behavior — the current contract (window mode)

- The block writes NOTHING while the width is moving (300 ms settle; every
  further change restarts the clock).
- **Above the essentials floor (~46 cols):** every width change — same-band
  OR crossing — settles to exactly ONE clean block in place. A crossing
  RECLAIMS: the terminal reflowed the on-screen frame to
  R = Σ physicalRows(cellWidth(row), tw) and the cursor followed its content,
  so the block's top is exactly R−1 rows above the cursor — walk back R−1,
  overwrite with the fresh trimmed frame, clear R−N stale rows. No frozen
  copy, no restart, no scrollback reliance (SPECIFICATION.md §8.5).
- **Below the floor** (the fresh frame itself wraps; its reflowed anchor is
  unknowable) and when R > terminal height: the freeze → restart-below
  fallback (one frozen copy stays in scrollback).
- **Plain view: UNCHANGED** — its freeze-and-restart with the `-` mark stays
  user-approved (DECISIONS #69).
- The column trim (#71) stays: below 79 cells the rightmost columns drop
  (FAILS→AVG→MAX→MIN) so the line keeps fitting.

**Accepted residual (documented in README):** mid-drag the terminal re-wraps
the stale on-screen rendering (transient, unavoidable without writing
mid-reflow); a settle repaint can land one row off if ConPTY's physical cursor
is off after the reflow — self-corrects on the next repaint. Non-reflowing
terminals (xterm, tmux, the sandbox PTY): the crossing reclaim is a documented
limitation (the reflow/non-reflow world is undetectable — CPR lies on ConPTY,
microsoft/terminal#18725; the reclaim chooses the user's reflowing world).

## How it got here (landmarks; full rationale in DECISIONS.md)

| Round | Fix | Outcome |
|---|---|---|
| 9–11 (#67–69) | Freeze-and-restart after CPR re-anchor (#66) failed on ConPTY | plain view user-approved (#69) |
| 12 (#70) | Conditional freeze — only when the reflow would move the block | superseded by the reclaim |
| 13 (#71) | Column trim (the user's idea) | kept |
| 15 (#73) | Defer same-band repaints | REVERTED — stale frames moved the freeze above the floor |
| 16 (#74) | `DOHPING_DEBUG` debug-log facility (the evidence path) | kept |
| 17 (#75) | Reflow-aware in-place reclaim | SHIPPED after retest |
| 18–19 (#76–78) | B elected on a STALE-BINARY report, then rejected on direct test; reclaim promoted | reclaim shipped (#78) |

The #76 rejection was the unversioned-filename trap (Windows renames
same-name re-downloads) — fixed in §6. The debug log (#74) remains the
evidence path for any future resize report.

---

## 1. What it is

`dohping` — a single-host ping/monitoring CLI (Go 1.26.5), "D'oh!"/Homer play,
pronounced "dopping". Monitors one host with ICMP (raw → unprivileged ping socket
→ system `ping` fallback) or TCP connect, tracks up/down/error state with
hysteresis and RTT stats, renders plain-line or fixed window mode, optional
text/JSON logging, interactive `q` quit, predictable exit codes.

Project dir: `/home/hermes/.hermes/user/projects/dohping` — **git repo** (baseline
`1e4a96e`, identity `Hermes Agent <hermes@hermes.home>`, branch `master`). Commit at
every stage/acceptance fix; use git to reverse changes when required (user's
stated expectation — the build process was delegated with git for reversibility
and stage-marking; if in doubt, ASK, don't assume).

## 2. Verified working (user-confirmed or gate-proven)

- **Plain line mode**: header, columns, live line updates in place, finalization
  on status change, non-TTY hygiene. Up line byte-identical to spec §7.4 example.
- **Window mode** (`--window`, `--window-lines N`) — user-confirmed 2026-08-17
  (#53/#54): renders IN PLACE on the normal terminal, no alternate screen, no
  screen clearing, block stays after exit, summary below. **Resize: reclaim
  shipped (#78)** — above the 46-cell floor every width change settles to one
  clean block (same-band and crossings alike); below the floor the conditional
  freeze → restart below (one frozen copy). Proven in-unit through the REFLOWING
  emulator model (`reflowResize`) and the real-PTY probe (all 4 scenarios PASS).
- **Flags on either side of HOST** (`dohping google.com -c 5` == `-c 5 google.com`)
  — GNU-style permutation (#55). `--` still terminates flags.
- **`-i`/`-t` accept bare seconds** (`-i 5` = 5s, ping convention) plus full
  duration strings (`500ms`, `1m30s`) (#57). Zero/negative still rejected.
- **Exit summary is a clean left-aligned block** at column 0 even on terminals
  without ONLCR — explicit `\r` per line, `\r\n` terminators (#56).
- **Liveness animation**: rising bar `▁▃▅▇` in the DURATION field's padding
  (column 45 at the HOST-15 minimum), advancing on a fixed 1-SECOND timer
  independent of probe cadence; the tick ALSO refreshes DURATION from the wall
  clock (#59–61). Finalized/history/piped lines stay plain (spec §7.4
  byte-identical).
- **Timestamp at default intensity** (was dim) (#58).
- **Three-tier ICMP**: raw socket → ping socket → system `ping`. Verified live:
  `1.1.1.1` up ~8ms, `::1` up (ICMPv6), `192.0.2.1` down (deterministic timeout).
- **Error handling**: consecutive errors = one line, duration updates; permission
  problems abort exit 3 + hint (never misreported as host-down).
- **Signals**: SIGINT→130, SIGTERM→143, q-quit→0, `--count`→0.
- **Cross-compile matrix**: linux/darwin/windows × amd64/arm64 in `dist/`,
  reproducible via `scripts/release.sh` (verified: round-17 source reproduces
  the shipped sha byte-for-byte).
- **Gates**: 7/7 packages race-clean (`go test -race -count=1 ./...`),
  gofmt/vet clean, golangci-lint + staticcheck + **gosec 2.28.0** all clean
  (gosec round #63: log file now 0600; justified `#nosec`; #74 added G703 to
  the debugx annotation).

## 3. Acceptance fixes worth remembering

| # | Fix | User report it answered |
|---|---|---|
| 53 | Window mode in place on normal terminal, no alt-screen/clear | "nothing about clearing the screen... at place at the screen" |
| 54 | Explicit column reset: `\x1b[<n>A\r` on redraw, `\r\n` between rows | fragmented rows (`348.00 348.00 348.00` / repeated headers) |
| 64 | Resize handled, platform-split: HOST elastic (content-fit, min 15 max 40, `…`), rune-based cell math, re-measure every redraw, PHYSICAL-row cursor math, SIGWINCH fast path / Windows 1s-tick self-heal | "Not handling terminal resize - breaks output" + "must be handled based on platform" |
| 65 | Plain live line width-aware (lastPhysRows walk-back + defensive clear); stale-clear resets column first | "plain mode also needs to be width aware for the current live line only" + user's lastPhysRows design |
| 66 | DSR/CPR re-anchor — **FAILED** on ConPTY (#18725), replaced by #67 | "It can creep up and does not clear the rest on wrap" |
| 67–69 | Freeze-and-restart (both views); `-` mark + STATUS→STATE/`?` (line 81→79 cells) | plain view user-approved ("much nicer visual") |
| 71 | WINDOW-MODE COLUMN TRIM: rightmost columns drop below 79 cells so the line fits instead of wrapping (down to essentials, 46 cells) | "My terminal doesn't live in any band, I am testing so use it accordingly" |
| 74 | `DOHPING_DEBUG=<path>` debug-log facility (`internal/debugx`, 0600, RFC3339-ms `[tag]` lines) — the app's own width telemetry | "Have you even seen a terminal tell you the width you are resizing to?" + "we should have had a facility for a debug logger already, just only enabled by code or ENV" |
| 75/78 | REFLOW-AWARE IN-PLACE RECLAIM (shipped): crossings above the floor reclaim in place — one block, no frozen copy (SPEC-window-resize-reclaim.md) | "It is neither a defense or true. It is clear not what was required" (§8.5: no scrollback reliance) |
| 76–77 | B episode: wrongly rejected on a stale binary, properly retested, promoted | "0014967e is out… This one should be set as the shipped build" |
| 79 | Plain-mode state-change clobber: `resizeMarkAbove` fired on EVERY finalize (not just resizes), replacing the header/finalized history with `-`; now gated on a real resize. **In wear-test 2026-08-23: "reasonably solid so far through a number of state changes"** | "prints a blank line with '-', losing the header. It also only shows 2 lines at a time" |

Full per-fix history: DECISIONS.md (79 entries, each with rationale).

## 4. Pending / next actions

- **Publish sequence — EXECUTED up to the push gate (2026-08-19, user "Go"):**
  - DONE: identity set (`neutralvibes` / `26578830+neutralvibes@users.noreply.github.com`);
    baseline snapshot amended (`--amend --reset-author`) from `fefce20`
    (Hermes Agent) → `88f887d` "Initial release" (user identity, author
    AND committer); rebased onto `origin/main` (`bf528a7` "Initial commit");
    LICENSE conflict resolved KEEPING OURS (`github.com/neutralvibes` —
    the earlier `git checkout --ours` grabbed the seed's plain
    "neutralvibes"; fixed with sed before the amend); commit message
    reworded per checklist ("Initial release", no process metadata);
    publish branch `publish-initial` created from the amended master.
  - **BLOCKED: `git push` → `refusing to allow a Personal Access Token to
    create or update workflow .github/workflows/ci.yml without workflow
    scope`.** The token lacks the Workflows permission — GitHub refuses
    the push SERVER-SIDE; no agent workaround. **User action: fine-grained
    token → Repository permissions → Workflows: Read and write on dohping,
    save.** Then resume: `git -C publish push -u origin publish-initial`
    → open PR (squash policy) → CI's first real run → tag v0.1.0 + release
    with plain assets + SHA256SUMS.
  - Previous state (for the record): identity TODO from 2026-08-18;
    decisions all resolved 2026-08-19 (first-commit "Initial release",
    tag v0.1.0, LICENSE holder `github.com/neutralvibes`, squash policy,
    plain asset names). The rebase WILL conflict on LICENSE — keep ours
    (now proven).

 - **CI un-held and BUILT (2026-08-19, user: "CI tests must be setup to run"):**
  - `.github/workflows/ci.yml` (tracked in the PRIVATE repo; added to
    PUBLIC_ITEMS so publish/ carries it) — gate job on ubuntu: gofmt, vet,
    race tests, gosec (prebuilt tarball v2.28.0, never `go install`,
    DECISIONS #63), fresh-build + all 4 PTY probe scenarios (TCP mode — no
    ICMP caps on runners), release matrix + SHA256SUMS; gate-windows job:
    unit tests + vet only (probe is POSIX). Runs on push to main + PRs.
  - `scripts/check.sh` (new): the shared gate — gofmt, vet, `go test -race`,
    then golangci-lint/staticcheck/gosec/govulncheck when installed
    (SKIP, not FAIL, when absent). `release.sh` now calls it first — a red
    gate refuses to build. Verified locally: gate GREEN (7/7), probes 4/4
    PASS, release.sh matrix rebuilt with linux-amd64 sha STILL `8dcce21a…`
    (byte-identical to the shipped build).
  - BEFORE the first push: the fine-grained token must have the **Workflows
    permission** (pushing `.github/workflows/` is rejected without it —
    Contents RW alone is not enough). User to confirm/add in GitHub token
    settings.
  - Reusable rig generalization (pty-probe.py, §10) REMAINS HELD — dohping
    CI uses the dohping-specific probe as-is.
  - **Debug facility is BUILD-TAGGED (2026-08-19):** debugx real impl +
    its forensics tests live only under `-tags debug`; release builds ship
    an inert stub (debugx_stub.go — DOHPING_DEBUG ignored, nothing can
    enable logging). CI has a dedicated step asserting a release binary
    writes no debug log; check.sh tests BOTH paths (`go test -race ./...`
    + `-tags debug`). Debug binary: `go build -tags debug`. README updated.

- **dohping setup finalisation (from repotest lessons, 2026-08-19) —
 checklist, user-paced** (user: one thing at a time; they'll be clear
 what dohping needs once the repotest procedure is digested):
 - [ ] Ruleset "main": switch Admin bypass from "always" to **"for pull
       requests only"** — the tested final config (push layer enforced
       mechanically, merges frictionless, no deadlock). User UI action:
       token cannot edit rulesets (PUT → 403).
 - [ ] Keep approval count=1 as the future gate; **CI/status checks will
       be the real mechanical reviewer** once a test suite exists (CI is
       a separate matter, user-flagged; connects to the held §10 CI-ready
       plan).
 - [ ] Optional: tag ruleset (no force-update, no deletion) to protect
       release tags.
 - [ ] Then the publish-prep sequence above (identity → amend fefce20 →
       rebase onto origin/main → derive publish/ → branch + PR → user
       squash-merge → tag v0.1.0 → release with plain assets + SHA256SUMS).

 - **Resize: RESOLVED and SHIPPED (#78).** If a resize report comes back, the
  evidence path is the debug log (`DOHPING_DEBUG=<path> dohping --window HOST`
  — tags `winch`/`tick`/`resize`/`redraw`, incl. `reclaim in place (R=… N=…
  tw=…)` and `reclaimed tw=… phys=… (was …)`) plus the reflow-emulator tests —
  never ask the user for widths or paste geometry (paste blindness, §7).
- **HELD PLAN — reusable terminal test rig** (idea user-approved 2026-08-18,
  build explicitly on hold until user says go): full plan in §10.
- **Unverified-forever candidates** (probe if asked): TCP probe mode (`-p tcp`),
  `--log-file` output (now 0600), `--timestamp-format rfc3339`, the
  darwin/windows binaries (built but never run on those OSes — Windows signal
  codes are closest-conventional, spec §18; Windows resize relies on the 1s
  tick, never run there either).
- Proposed but not done: adding gosec to `scripts/release.sh` as an automatic
  gate (user hasn't answered the offer).

## 5. Environment (critical — read before running anything)

```sh
export GOROOT=/home/hermes/.hermes/go-toolchain/go
export GOCACHE=/home/hermes/.hermes/go-cache
export PATH="$GOROOT/bin:$PATH"        # ORDER MATTERS: GOROOT before PATH export
```

- go1.26.5 linux/amd64, module `dohping` (local-only, no remote).
- Deps: golang.org/x/net v0.58.0 (icmp), golang.org/x/term v0.45.0.
- Linters at `~/go/bin/golangci-lint`, `~/go/bin/staticcheck`, `~/go/bin/gosec` (gosec 2.28.0, added 2026-08-17).
- **Install gosec from the prebuilt GitHub release tarball, NEVER `go install gosec@latest`** — that compiles the Anthropic/OpenAI SDKs (gosec's LLM feature deps) and OOM'd the 4GB sandbox (DECISIONS #63).
- **Sandbox ICMP reality**: our processes have 0 effective caps → raw sockets
  EPERM; `/bin/ping` works via sandbox elevation → that's why tier 3 exists.
  Do NOT re-litigate; it's settled and user-verified.
- Build: `go build ./cmd/dohping`. Release: `bash scripts/release.sh` → `dist/`
  (5 binaries + SHA256SUMS, reproducible).

## 6. Publishing to the user's laptop

- Rig is the ONLY delivery path (bare MEDIA: lines don't reach the laptop in
  remote-gateway mode). Serve root: `/home/hermes/.hermes/user/rig/served/`.
- Published at `https://files.hermes.home/dohping/` (basic auth: user `hermes`,
  password in Hermes memory — re-share only if user asks).
- **VERSIONED FILENAMES (2026-08-18 lesson)**: every build is ALSO published as
  `dohping-<os>-<arch>-<sha8>` plus an `INDEX.txt` build history. The plain
  name is the latest SHIPPED build. Windows renames same-name re-downloads
  ("(1)"), which once caused a stale-binary test to wrongly reject a build —
  never rely on the plain name alone for a test round; tell the user the exact
  versioned filename (or the sha) to download. Current: plain name AND
  `dohping-linux-amd64-8dcce21a` = shipped reclaim; `dohping-linux-amd64-0014967e`
  = REJECTED B, retained as a versioned artifact.
- GitHub release assets use PLAIN names (`dohping-<os>-<arch>`) — the sha
  prefix is a rig/local-testing convention only (decided 2026-08-19; see
  PUBLISH-CHECKLIST.md judgment gates).
- Publish step after a rebuild: `cp dist/* /home/hermes/.hermes/user/rig/served/dohping/`
  then verify `curl -sku hermes:<pass> -o /dev/null -w "%{http_code}" \
  https://files.hermes.home/dohping/dohping-linux-amd64` → 200, and compare
  `sha256sum` served-vs-dist.
- Current published linux-amd64 sha: `8dcce21a73fa…` (2026-08-18, round #78:
  reflow-aware reclaim SHIPPED; served = dist, verified byte-identical over TLS).
- Full rig knowledge: skill `file-serve-rig`.

## 7. Test/debug workflow that works

- Deterministic `down` tests: `192.0.2.1` (TEST-NET routes out here and times
  out deterministically).
- PTY capture: `script -qec "<cmd>" /tmp/cap.txt` — BUT the sandbox PTY applies
  ONLCR (`\n`→`\r\n`), which HID the #54 column bug. To prove terminal output,
  RENDER the stream through a terminal emulator and assert the visible screen
  (the `termScreen` in `internal/output/window_test.go` does this) — escape-list
  assertions are not enough.
- The user's terminal does NOT apply ONLCR: exit-summary drift (#56) and the
  window column bug (#54) both only reproduced when rendering through a
  no-ONLCR emulator (`noOnlcrScreen` in `internal/app/app_test.go`). Any new
  terminal-output change should be asserted through BOTH the plain termScreen
  and, where line-column drift is possible, the no-ONLCR one.
- Animation tests: the live-line frame lives at column 47 and advances on the
  1-second `Tick()` (driven by the app loop's timer, not probe events) — unit
  tests drive `Tick()` directly with an injected clock.
- Resize tests: inject a terminal width via `newTestWindowResizable`, render
  through `termScreen` (DECAWM autowrap). The emulator models BOTH worlds:
  `resize()` = non-reflowing (xterm-style, existing tests) and `reflowResize()`
  = reflowing (logical-line re-wrap, cursor follows content — the user's
  terminal; the reclaim tests). Above the 46-cell floor: ANY width change
  (same-band or crossing: 60→55, 60→100, 120→79, the user's 97→75, 40→100,
  45→60, drag 55→65→60) settles to exactly ONE block. Below the floor:
  same-band wraps in place (40→45), crossings freeze → two blocks (60→40).
- **Real-PTY resize proof**: `python3 scripts/pty-resize-probe.py` spawns the
  built binary in a pty and renders the capture through a VT emulator. Scenarios:
  `window` (60→100 — in place, one header), `window-same-band` (60→55 — one
  block), `window-subfloor` (50→40 — SETTLES BELOW the floor: crossing freeze →
  two headers; the non-reflowing sandbox pty cannot exercise an above-floor
  reclaim), `plain` (fixed 60-col pty — live line anchored at row 2). Run it
  against a FRESH build (`rm -f` the output first — stale-build trap; the
  probe's binary path is hardcoded to /tmp/dohping-test).
- **PASTE BLINDNESS (2026-08-18 lesson)**: the user's terminal (Windows
  Terminal → WSL2) copies a WRAPPED line as one logical line — no newline at
  the wrap point — so a pasted line of N cells does NOT prove it fit in N
  columns. Never conclude "no wrap / no reflow" from a user's paste geometry;
  the user's own screen is the evidence (they send screenshots now). Pastes
  are weak evidence; the emulator tests and PTY probe are the proof.
- **Resize forensics via the debug log (DECISIONS #74/#75)**: for the user's
  real-terminal drag tests, the evidence is `DOHPING_DEBUG=<path> dohping
  --window HOST` — the app logs its own width observations (`winch`, `tick`,
  `resize` with the crossing decision, `redraw` with `reclaim in place (R=… N=…
  tw=…)` and `reclaimed tw=… phys=… (was …)`). No terminal displays the width
  during a drag, so this log is the only record of the sweep; ask the user for
  it instead of widths or screenshots. Tags and format: `internal/debugx` +
  README "Debug logging".
- Regression tests must accompany every acceptance fix (that's the established
  pattern, #50–78 all have them).

## 8. Project conventions (user's way of working)

- **Spec-first, no drive-by fixes**: every change traces to the spec or an
  explicit user report, recorded in `DECISIONS.md` with rationale, and gates are
  re-run. `CHECKPOINT.md` rewritten at phase/acceptance boundaries.
- **Git discipline**: commit at every stage and acceptance fix (baseline
  `1e4a96e`); `dist/` and `.notify-state` are ignored. If a step is genuinely
  ambiguous, ASK — the user delegated the build but expects questions, not
  assumptions.
- User wants genuine pushback and the sober voice: say what pays AND what doesn't.
- Do not commit taste decisions to memory/vault/skills without asking.
- User's direct experimental evidence is ground truth — don't re-litigate settled
  findings with indirect logs.
- **Public-facing prose (README, docs/): plain human phrasing, NO em-dashes
  ('—').** An em-dash is an AI giveaway (user's convention; see the humanizer
  skill). Applies to everything that reaches GitHub, including docs/.

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
scripts/pty-resize-probe.py  real-PTY resize scenarios (window/same-band/subfloor/plain)
scripts/publish-github.sh    derive the PUBLIC repo snapshot (publish/) — see §11
build-docs/                  INTERNAL build-process docs — TRACKED in the private
                             repo (git mv kept them; gitignore never applied to
                             tracked files) but NEVER published: publish/ derives
                             only from the contract list. Files: HANDOFF/DECISIONS/
                             CHECKPOINT/PROGRESS/LAUNCH/SPECIFICATION/
                             SPEC-window-resize-reclaim/Phases/ENGINEERING_PRINCIPLES
publish/                     INTERNAL derived PUBLIC repo (gitignored, regenerated by
                             scripts/publish-github.sh) — GitHub gets exactly this
LICENSE                      MIT (public) — holder line still a placeholder
dist/                        release artifacts (5 binaries + sums)
```

## 10. Held plan: reusable terminal test rig (2026-08-18, HELD)

**Status: user approved the idea ("we will do"), explicitly HELD until a
go-ahead. Do NOT build this without the user saying go.**

**Why it exists:** 3 of the user's last 5 projects were command-line based.
The rig lessons (render the stream through an emulator and assert the visible
screen, never escape-list assertions; ONLCR traps; real-PTY resize proof) cost
many acceptance rounds on dohping — they should carry forward, not be re-paid
per project. Hard constraint the user added: the asset must be CI-ready — the
project will live on GitHub, so anyone must be able to run the suite with a
stock toolchain + stock Python, no agent, no sandbox.

**Deliverables (one pass, ~1h total):**

1. `~/.hermes/user/tools/scripts/pty-probe.py` — generalize
   `scripts/pty-resize-probe.py` (which stays in this repo as the dohping-
   specific instance):
   - `--binary PATH` (fixes the hardcoded `/tmp/dohping-test`), `--scenario NAME`,
     optional `--cols/--rows/--duration/--resize-to`.
   - `SCENARIOS` dict: `name -> fn(binary, opts) -> bool`. dohping's scenarios
     ship as examples: `window`, `window-same-band`, `window-subfloor` and
     `plain`.
   - Core untouched: `TermScreen` emulator, `pty.fork` capture, TIOCSWINSZ
     injection, `RESULT: PASS/FAIL` + exit 0/1.
   - stdlib-only (`pty`/`fcntl`/`termios`/`select`), POSIX — runs on
     Linux/macOS CI runners; deterministic (fixed durations, no interactivity).
2. `~/.hermes/user/tools/scripts/ci-terminal-probe.yml` — GitHub Actions
   workflow template (copy into `.github/workflows/`, adjust build step +
   scenario names): `rm -f` fresh build (stale-build trap), `go test -race ./...`,
   `go vet`, gofmt check, probe scenarios, release matrix
   (linux/darwin/windows × amd64/arm64 + SHA256SUMS). gosec via the securego
   action or prebuilt tarball — NEVER `go install gosec@latest` (compiles the
   LLM-SDK deps, OOM'd the sandbox, DECISIONS #63).
3. `go-cli-development` skill: 3-line pointer to the parked harness.

**Explicit non-goals (anti-over-engineering, agreed with user):** no Go-module
extraction of `termScreen`/`noOnlcrScreen` (they stay dohping-internal);
no plugin framework/YAML/classes — the dict of functions is the ceiling;
no interactive-input injection hook (known future seam only, one comment in
the file, for driving things like a `q`-quit).

**CI notes:** probe uses TCP mode (`-p tcp`) so no ICMP caps are needed on
runners; ICMP tiers stay unit-tested with mocks (runners have no raw sockets
either — same constraint as the sandbox). Windows CI runs unit tests only
(probe is POSIX); the emulator-grid unit tests cover the platform matrix.

**Trigger:** user says go → build per this plan, commit in the tools/ repo
(it is a git repo, branch `main`), repoint this HANDOFF §7 to the parked
harness.

---

## 11. GitHub publish structure (2026-08-18)

The repo is destined for GitHub. Two layers keep the internal build docs out:

- **`build-docs/`**: all build-process docs — HANDOFF, DECISIONS,
  CHECKPOINT, PROGRESS, LAUNCH, SPECIFICATION, SPEC-window-resize-reclaim,
  Phases, ENGINEERING_PRINCIPLES. They are TRACKED in the private repo
  (versioned + reversible like everything else — `git mv` kept them tracked,
  and gitignore never applies to tracked files; the `build-docs/` gitignore
  line was added then REMOVED as misleading). They never reach GitHub because
  `publish/` derives only from the PUBLIC_ITEMS contract list — the private
  repo is never pushed, only `publish/` is. Old history contains them up to
  `fa83a67`, which is equally fine for the same reason.
- **`publish/`** (gitignored): a SEPARATE git repo containing exactly the
  public files, derived by `scripts/publish-github.sh`. GitHub gets only this.
  The script: requires a clean working tree, rebuilds the tree (keeps `.git`
  so remotes survive), copies the PUBLIC_ITEMS contract, asserts (a) nothing
  forbidden (build docs, dist, .notify-state, publish itself) and (b) the
  committed file list equals the contract EXACTLY, then commits a snapshot
  named after the private sha. Idempotent (no-op if nothing changed). The
  script NEVER pushes on its own — pushing is agent-handled OUTSIDE the
  script: the agent adds the origin remote (git + PAT credential helper;
  `gh` NOT installed on the sandbox), pushes publish/ master to a branch,
  and opens a PR; the USER approves the merge to main. First publish will
  need the repo URL + token from the user; token must include `workflow`
  scope when the §10 CI workflow (which pushes .github/workflows) ships.
- **GitHub side (set by user 2026-08-18):** fine-grained token scoped to the
  dohping repo only (contents write, PRs read/write, issues read, actions
  read) + branch protection on `main`: require PR + 1 approval, block
  force-pushes, and *do not fail branches at creation* — no CI workflow
  exists yet, so no required checks until one has run. When the §10 CI
  workflow ships: run it once on a branch, THEN add its checks as required
  status checks (requiring a never-run check deadlocks every PR).
- **Commit-message hygiene (2026-08-18):** public commits must read as the
  user wrote them — no agent tells (semicolon clause-chains, conventional-
  commit prefixes, process metadata like private shas/timestamps). The gate
  is `build-docs/PUBLISH-CHECKLIST.md` (consult before EVERY push; the
  publish script's footer reminds) + a pre-commit hook in publish/.git that
  HARD-REFUSES commits whose email is missing or hermes@hermes.home (script
  installs it on init; currently also installed manually). Traceability
  between public and private commits lives in `build-docs/PUBLISH-LOG.md`
  (private) — never in public messages.
- **LICENSE = MIT**, holder line still `<YOUR NAME HERE>` — user must fill
  before first push. Publish commits inherit the private repo's git identity
  (Hermes Agent <hermes@hermes.home>) unless the user overrides in publish/.
- Pitfalls paid for: bare `dohping` in the public .gitignore matches the
  `cmd/dohping/` DIRECTORY (root-anchor: `/dohping`); `.gitignore` must not
  appear twice in the expected list; a nested repo is staged as a 160000
  gitlink unless the parent ignores `publish/`.
- When the held §10 CI workflow is built, add `.github/workflows` to
  PUBLIC_ITEMS in the script.

---

## 12. Rehearsal run — neutralvibes/repotest (2026-08-19, ALL STEPS GREEN)

Full dry-run of the publish flow on a throwaway repo before touching dohping.
Repo: `neutralvibes/repotest` (public, created with LICENSE seed → main born
at creation with the user's "Initial commit"). Scratch: `~/.hermes/user/tmp/
repotest/` (teardown pending).

| Step | Result |
|---|---|
| Agent-authored first commit (simulating `fefce20`) | ✓ then amended |
| `git commit --amend --reset-author` → user identity | ✓ author AND committer both reset; tree scan zero agent trace |
| Push branch (`rehearsal-v0.1.0`) via git + credential store | ✓ |
| Open PR via REST API | ✗ 422 "no history in common with main" — branch and seeded main are unrelated roots |
| Rebase branch onto origin/main (resolve seed overlap) | ✓ `GIT_EDITOR=true git rebase --continue` needed (no editor in non-interactive shell); keep the SEED's files on conflict |
| PR via API (history now shared) | ✓ PR #1 |
| User squash-merge | ✓ main = "Initial release (#1)" |
| Annotated tag `v0.1.0` (tagger neutralvibes) + push | ✓ |
| Release via API + asset upload (plain names) | ✓ `hello.sh` + `SHA256SUMS`; sha in release notes |
| Branch protection via API | ✗ 403 — token has no Administration scope → user sets in UI |
| Force-push test (rule INACTIVE) | ✗ force-push to main succeeded — the rule existed but was never enabled |
| Force-push test (rule ENABLED) | ⚠️ force-push to main STILL succeeded — admin bypass ("include administrators" off); remote only WARNS "Changes must be made through a pull request" |
| Direct FF push to main (rule enabled) | ⚠️ succeeded — same admin bypass |
| Merge without approval via API | ⚠️ succeeded ("Pull Request successfully merged", PR #2) — admin bypass covers merges too; the 1-approval requirement is unsatisfiable solo (self-approval blocked) |

**Lessons for dohping's first publish (all in §4):**
1. GitHub repo-creation LICENSE/README seed = root commit on main; a
   separately-rooted publish branch CANNOT PR (422). Fetch + rebase onto
   origin/main before opening the PR.
2. Keep the seed's files on conflict where sensible; on LICENSE the user
   chose the OWNER-PATH holder (`github.com/neutralvibes`, no protocol) →
   the first rebase WILL conflict on LICENSE: keep ours over the seed's
   plain form.
3. Squash-merge makes main read as one versioned commit per PR — user chose
   it here; CONFIRMED 2026-08-19 as dohping's merge policy.
4. Release assets upload fine via `uploads.github.com` with the fine-grained
   token; plain names work (no Windows-style rename server-side).
5. Amend-first is verified safe: `--amend --reset-author` rewrites both
   identities before anything is pushed.
6. **Protection mechanism = RULESETS, not classic branch protection.**
   Both repos run rulesets whose bypass list names "Admin role,
   bypass_mode: always". REFINED MODEL (2026-08-19, after user challenge):
   **bypass ≠ admin.** The token has NO Administration permission (admin
   APIs 403) — it is not admin-capable. But GitHub resolves the token's
   actor as the repo-admin role for ENFORCEMENT (repo API reports
   `permissions.admin: True`; rulesets report `current_user_can_bypass:
   always`), so it auto-bypasses the rulesets — push-time rules
   (pull_request, non_fast_forward) are not enforced against it, with only
   a warning. Verification tool: GET /rulesets/{id} →
   `current_user_can_bypass`. Configs verified 2026-08-19:
   - repotest "test rule" (active): deletion, non_fast_forward,
     pull_request with required_approving_review_count=**0** → merges need
     no approval for anyone (that alone explains the unapproved merge).
   - dohping "main" (active): deletion, non_fast_forward, pull_request
     with required_approving_review_count=**1** + dismiss-stale-reviews;
     allowed_merge_methods incl. squash. Matches the intended config.
7. **Protection guards contributors, not the owner's token.** For non-admin
   contributors the rulesets are fully enforced (PR + 1 approval, no
   force-push, no delete); the agent's token bypasses (Admin-role bypass
   actor). LEVER: removing "Admin role" from the bypass list makes the
   ruleset bind the token too — but binds the owner as well; on a solo
   repo with count=1 and self-approval blocked, the owner can deadlock
   merging their own PRs. The real protection for dohping main stays
   procedural: agent never pushes/merges main (documented flow), pre-commit
   identity hook, user oversight. Keep count=1 as a future contributor
   gate — user's call (2026-08-19).
8. **Bypass removal EXECUTED + PROVEN on repotest (2026-08-19). FINAL
   CONFIG = Admin bypass "For pull requests only"** (bypass_mode:
   `pull_request`; `current_user_can_bypass: pull_requests_only`). Full
   matrix, all proven behaviorally on repotest:
   - bypass "always" (original): force-push, direct push, unapproved merge
     ALL allowed (warning only)
   - bypass none + count=1: all three REJECTED — but the owner deadlocks on
     their own PRs (self-approval blocked → disable/merge/re-enable dance)
   - **bypass "pull_request" + count=1 (FINAL):** force-push REJECTED,
     direct push REJECTED, owner's own PR merges ALLOWED (no dance);
     non-admin contributors still gated by count=1. The push layer is
     mechanically enforced; merge authority is procedural (the admin PR
     bypass covers the token too — accepted residual).
   The token CANNOT edit rulesets (PUT /rulesets → 403) — the escape hatch
   is exclusively the user's. Ruleset conditions = `~DEFAULT_BRANCH` only →
   feature-branch pushes (incl. the first-publish branch) unaffected.
   dohping action pending: switch its Admin bypass from "always" to
   "for pull requests only" (same as repotest).
