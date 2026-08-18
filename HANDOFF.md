# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `LAUNCH.md` (build brief), `SPECIFICATION.md`
(the contract), `DECISIONS.md` (71 entries — each fix's rationale), `CHECKPOINT.md`
(gate status), `PROGRESS.md` (timeline). `README.md` is the user-facing doc.

**Status: build complete. All 6 phases green. Acceptance rounds 1–11 shipped
(DECISIONS #50–69, 2026-08-17/18). Round 8's CPR re-anchor (#66) FAILED the
user's real-terminal test ("still not working") and was replaced in round 9
(#67) by freeze-and-restart: on a width change the displays pause in-place
updates until the width is stable (300ms, restarted by every further change),
then continue on a fresh row below the frozen re-wrapped rendering — the old
rendering stays in scrollback as history. Root cause of the #66 failure:
the user's terminal is Windows Terminal → WSL2 → Debian, and the DSR/CPR
answer comes from ConPTY, whose reflow differs from the rendered view
(microsoft/terminal#18725 — "wildly incorrect cursor positions"). Round 10
(#68) confirmed working on the user's screen ("It is doing it") and added:
the frozen row is marked with a single `-` (safe only when the line was one
row before and after the resize), and STATUS → STATE with `?` for the
never-established status — the minimum line dropped from 81 to 79 cells,
under 80. #67/#68 are position-independent and fully sandbox-tested; the
user's own terminal test was the remaining gate and is now CLOSED for plain
view (DECISIONS #69, 2026-08-18: "the last change for the plain view appears
to be working on resize and provides a much nicer visual" — frozen `-` rows
stay in scrollback, live line continues fresh below; 5 stacked frozen rows
observed under repeated resizing). User-verified so far: window mode in
place, flags on either side of HOST, bare-seconds interval/timeout, clean
exit summary, the liveness animation, the window-mode resize fix, and the
freeze/restart resize behavior (both views, incl. the `-` mark in plain).**
Round 12 (DECISIONS #70, 2026-08-18): the #67 freeze is now CONDITIONAL in
WINDOW mode — it fires only when the reflow would actually MOVE the block
(any row of the last completed frame changes its physical row count at the
new width). The user's window is ~55–60 cols (below the 79-cell floor;
screenshot: AVG 3.92 wraps mid-value), and #67's unconditional trigger
froze on EVERY resize there, stacking a whole frozen block per width
change. Same-band resizes (60→55: 12 rows at both widths) now repaint IN
PLACE — no artifact. Plain display UNCHANGED per user ("plain view is plain
view"). Superseded for window mode by the round-13 trim (below).
Remaining gate: the user's real-terminal test of the same-band window
repaint (built 2026-08-18, dist sha 466b95c0…).
Round 13 (DECISIONS #71, 2026-08-18): WINDOW-MODE COLUMN TRIM — below the
79-cell line minimum the rightmost columns (FAILS→AVG→MAX→MIN) drop so the
line keeps fitting instead of wrapping, down to essentials (46 cells).
User: "my terminal doesn't live in any band, I am testing so use it
accordingly" — the #70 freeze only separates same-band from crossing, and
free testing crosses constantly; trimming eliminates the crossings
themselves. Resize behavior is now UNIFORM: at any width ≥ 46 a resize is
an in-place repaint with zero artifacts; the freeze is reachable only
below 46. Plain untouched. Remaining gate: user's real-terminal test
(built 2026-08-18, dist sha 5d79fd9b…).

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
- **Window mode** (`--window`, `--window-lines N`) — **user-confirmed working
  2026-08-17** after fixes #53/#54: renders IN PLACE on the normal terminal,
  no alternate screen, no screen clearing, block stays after exit, summary below.
  **Resize is now artifact-free at any width ≥ 46 (DECISIONS #71)**: below the
  79-cell line minimum the rightmost columns drop (FAILS→AVG→MAX→MIN) so the
  line never wraps — a resize is always an in-place repaint with no frozen
  block; only below 46 cells (where the essentials line wraps) does the
  conditional freeze (#70) fire. Proven in-unit and in the real-PTY probe
  (`window` 60→100: one block, in place; `window-subfloor` 40→70: freeze).
- **Flags on either side of HOST** (`dohping google.com -c 5` == `-c 5 google.com`)
  — GNU-style permutation (#55). `--` still terminates flags.
- **`-i`/`-t` accept bare seconds** (`-i 5` = 5s, ping convention) plus full
  duration strings (`500ms`, `1m30s`) (#57). Zero/negative still rejected.
- **Exit summary is a clean left-aligned block** at column 0 even on terminals
  without ONLCR — explicit `\r` per line, `\r\n` terminators (#56).
- **Liveness animation**: live line shows a rising bar `▁▃▅▇` in the DURATION
  field's padding (column 45 at the HOST-15 minimum, floats between DURATION
  and MIN). Advances on a
  fixed 1-SECOND timer independent of probe cadence, and the tick ALSO refreshes
  DURATION from the wall clock (#59–61). Finalized/history/piped lines stay
  plain (spec §7.4 byte-identical). Verified in PTY capture at `-i 5`.
- **Timestamp at default intensity** (was dim) (#58).
- **Three-tier ICMP**: raw socket → ping socket → system `ping`. Verified live:
  `1.1.1.1` up ~8ms, `::1` up (ICMPv6), `192.0.2.1` down (deterministic timeout).
- **Error handling**: consecutive errors = one line, duration updates; permission
  problems abort exit 3 + hint (never misreported as host-down).
- **Signals**: SIGINT→130, SIGTERM→143, q-quit→0, `--count`→0.
- **Cross-compile matrix**: linux/darwin/windows × amd64/arm64 in `dist/`,
  reproducible via `scripts/release.sh`.
- **Gates**: 7/7 packages race-clean (`go test -race -count=1 ./...`),
  gofmt/vet clean, golangci-lint + staticcheck + **gosec 2.28.0** all clean
  (gosec round #63: log file now 0600; 2 justified `#nosec`).

## 3. Acceptance fixes (DECISIONS #50–63, all shipped)

| # | Fix | User report it answered |
|---|---|---|
| 50 | Consecutive errors don't re-enter error state; `EventProbeError` updates live line duration in place | "error status repeated on a new line" |
| 51 | Permission-class probe errors abort exit 3 + hint (all tiers, incl. ping exit-2 wrapped as EPERM) | "ping can tell you there is a priv problem straight away" |
| 52 | MIN/MAX/AVG/FAILS left-aligned under headers | "MIN, MAX and AVG columns are not in place" |
| 53 | Window mode in place on normal terminal, no alt-screen/clear | "nothing about clearing the screen... at place at the screen" |
| 54 | Explicit column reset: `\x1b[<n>A\r` on redraw, `\r\n` between rows | fragmented rows (`348.00 348.00 348.00` / repeated headers) |
| 55 | Flags on either side of HOST (GNU-style permutation) | "options/flags should be placeable either side of a ping target" |
| 56 | Live finalize ends `\r\n`; exit summary `\r`-prefixed + `\r\n`-terminated | "once the time is up it loses formatting" |
| 57 | `-i`/`-t` accept bare seconds + duration strings, helpful error hint | "`--interval 5` / `-i 5`: parse error" |
| 58 | Timestamp at default intensity (was dim) | "color selected for time field is a bit dim" |
| 59–61 | Liveness animation: rising bar `▁▃▅▇` at col 47, 1s ticker independent of probe cadence, tick refreshes DURATION | "needs something more visible to show it is working" / "blocks are not appearing in the right place" / "display needs to run every second" / "why doesn't duration update on the same schedule?" |
| 62 | Docs: README "Timing model" note (first probe immediate; duration measured) | "it feels like it should be 5 secs… needs a value + 1" (rejected — see DECISIONS) |
| 63 | gosec 2.28.0 clean: log 0600, TCP close discarded, 2 justified `#nosec` | "we need gosec" |
| 64 | Terminal resize handled, platform-split: HOST elastic column (content-fit, terminal-capped, min 15 max 40, `…`), rune-based cell math, window re-measures every redraw, PHYSICAL-row cursor math (wrapped blocks stay coherent); Unix SIGWINCH fast path, Windows self-heals via the 1s tick | "Not handling terminal resize - breaks output" + "must be handled based on platform" |
| 65 | Plain live line width-aware: same physical-row primitive reduced to one row — lastPhysRows + walk-back + defensive clear (live rewrite AND finalize); HOST stays fixed (scrollback consistency); piped/--no-live untouched; plain mode gains SIGWINCH fast path. Latent fix: stale-clear resets column (`\x1b[1B\r\x1b[K`) — cursor-down preserves the column, so non-blank last rows left stale text (window full-block case too) | "plain mode also needs to be width aware for the current live line only" + user's lastPhysRows design |
| 66 | REFLOWING-terminal re-anchor via DSR/CPR (root fix for "creep up / does not clear the rest on wrap"): on width change, query the cursor (`\x1b[6n` → `\x1b[<r>;<c>R`, app-owned, key reader parses CSI, 150ms timeout, degrade on no-answer); cursor sits at the end of the re-wrapped line → recompute anchor. Discriminator: CPR row == bookkeeping expectation → no reflow → non-reflowing terminals byte-identical to #65. Both displays (plain live + window block — retires the #64 window creep too). x/term Windows MakeRaw sets ENABLE_VIRTUAL_TERMINAL_INPUT (verified) | "It can creep up and does not clear the rest on wrap" + option A chosen ("lets try your recommendation") |
| 70 | Window-mode resize freeze is CONDITIONAL: freeze only when the reflow would move the block (any row of the last completed frame changes its physical row count at the new width — observeResize compares Σ physicalRows(lastRows, newW) vs lastPhysRows); same-band resizes repaint in place, no frozen block left behind. Pending freeze still restarts the settle clock on every further width change (drag behavior unchanged). PLAIN display untouched. User's column-trim idea parked (window mode may be rethought) | "This happens on resize window mode, making it less wider causes it" — user's ~55–60 col terminal stacked a frozen block per width change; #67 froze on every change even when the layout absorbed it (60→55 = 12 rows both widths; same-band lines cannot move in a reflow — the #68 safety reasoning generalized). Screenshot proved AVG 3.92 wrapping mid-value |
| 71 | WINDOW-MODE COLUMN TRIM (the user's original idea): below the 79-cell line minimum the rightmost columns drop (FAILS→AVG→MAX→MIN, header in sync) so the line fits instead of wrapping — down to essentials TIME/HOST/STATE/DURATION (46 cells; the live line's animation frame keeps DURATION untrimmed). HOST retracts first (#64), then columns. Plain untouched. Combined with #70, window-mode resize is now a uniform in-place repaint at any width ≥ 46 — the freeze is only reachable below 46 | "My terminal doesn't live in any band, I am testing so use it accordingly" — #70 only separated same-band from crossing, and free testing crosses constantly; trimming eliminates the crossings themselves. Built after the conditional freeze proved invisible to the user's testing |

## 4. Pending / next actions

- **User confirmed the plain-view resize fix on the real terminal (2026-08-18,
  DECISIONS #69)** — freeze-and-restart reads well ("much nicer visual").
- **Round 13 (#71) shipped and awaiting the user's real-terminal test**:
  window mode trims rightmost columns below 79 cells so the line never
  wraps — resize is now a uniform in-place repaint at any width ≥ 46
  (the user's "my terminal doesn't live in any band" correction drove it;
  #70's conditional freeze is now reachable only below 46). PLAIN display
  deliberately untouched. If the user reports another acceptance issue:
  reproduce, fix, add regression test, re-run gates, rebuild `dist/` via
  `scripts/release.sh`, republish to the rig (§6), record DECISIONS +
  CHECKPOINT entries, commit.
- **HELD PLAN — reusable terminal test rig** (idea user-approved 2026-08-18,
  build explicitly on hold until user says go): full plan in §10.
- **Animation is user-testing territory**: the rising-bar placement (col 47) and
  the 1-second ticker were both corrected after user reports — if placement or
  cadence comes up again, verify against the rendered-screen tests first.
- **Resize is user-testing territory**: the #64/#65/#67/#68 work (HOST column
  retraction/expansion, `…` truncation at min width, below-floor wrap
  staying coherent, plain live line staying anchored, freeze-and-restart with
  the `-` mark) was proven in-unit and in a real PTY
  (scripts/pty-resize-probe.py — `window` and `plain` scenarios), and the
  plain-view behavior is now user-confirmed (#69); window mode's frozen-block
  behavior was confirmed in round 10. If a resize report comes back, re-verify
  against the width-injected window/display tests first.
- **No terminal cooperation is relied on** (#67): freeze-and-restart is
  position-independent and needs no DSR/CPR answer — identical on reflowing
  and non-reflowing terminals. Piped stdin or a dumb terminal degrades to
  no-live/plain behavior. Windows Terminal: the Windows binary is still
  never run; first Windows smoke should resize in window mode to confirm.
- Areas the user has NOT explicitly verified yet (candidates to probe if asked):
  TCP probe mode (`-p tcp`), `--log-file` output (now 0600 — user may notice),
  `--timestamp-format rfc3339`, the darwin/windows binaries (built but never run
  on those OSes — Windows signal codes are documented as closest-conventional,
  spec §18; Windows resize relies on the 1s tick, never run there either).
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
- Publish step after a rebuild: `cp dist/* /home/hermes/.hermes/user/rig/served/dohping/`
  then verify `curl -sku hermes:<pass> -o /dev/null -w "%{http_code}" \
  https://files.hermes.home/dohping/dohping-linux-amd64` → 200.
- Current published linux-amd64 sha: `5d79fd9b7f09…` (2026-08-18, round #71:
  window-mode column trim; served = dist, verified byte-identical over TLS).
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
  tests drive `Tick()` directly with an injected clock; the rendered-screen
  assertions in `display_test.go`/`window_test.go` cover placement and
  history-vs-live separation.
- Resize tests: inject a terminal width via the window's `sizeFn` (helper
  `newTestWindowResizable`), render through `termScreen` (which now simulates
  DECAWM autowrap) and assert the visible grid at 120/79/60/45/40 cols —
  expansion, retraction, `…` truncation, below-floor wrap coherence,
  stale-row clearing. Since #71 the floor is 46 cells (the essentials line;
  the live line's animation frame keeps DURATION untrimmed): at ≥46 a width
  change must repaint in place with exactly ONE block (60→55, 60→100,
  120→79, drag 55→65→60); below 46 the band rules apply — same-band repaints
  (40→45), crossing freezes (40→100, 45→60, drag 40→60→35→60).
- **Real-PTY resize proof**: `python3 scripts/pty-resize-probe.py` spawns the
  built binary in a pty and renders the capture through a VT emulator,
  printing the visible screen + raw-stream forensics (cursor-up counts,
  restart CRLFs). Scenarios: `window` (60→100 mid-run — the line is trimmed
  at 60 so it never wraps: proves the block repaints IN PLACE, exactly one
  header, #71), `window-subfloor` (40→70 mid-run — a genuine wrap-count
  change below the floor: proves the conditional freeze fires and restarts
  below the frozen rendering, two headers, #67/#70), `window-same-band`
  (60→55 — one block), and `plain` (fixed 60-col pty — live line anchored at
  row 2 across a multi-second run). Run it against a FRESH build (`rm -f`
  the output first — stale-build trap; the probe's binary path is hardcoded
  to /tmp/dohping-test).
- **PASTE BLINDNESS (2026-08-18 lesson)**: the user's terminal (Windows
  Terminal → WSL2) copies a WRAPPED line as one logical line — no newline at
  the wrap point — so a pasted line of N cells does NOT prove it fit in N
  columns. Never conclude "no wrap / no reflow" from a user's paste geometry;
  the user's own screen is the evidence (they send screenshots now). Pastes
  are weak evidence; the emulator tests and PTY probe are the proof.
- Regression tests must accompany every acceptance fix (that's the established
  pattern, #50–66 all have them).

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

## 9. Key file map

```
cmd/dohping/main.go          entry
internal/cli/                flag parse, validation, conflicts, help
internal/app/                Main, run loop, signals, key reader, summary
internal/ping/               Probe iface, ICMP (3-tier incl. pingcmd.go), TCP
internal/state/              engine, hysteresis, RTT stats, events
internal/output/             Layout/format, plain Display, Window (in-place)
internal/theme/              role-based ANSI color rules
internal/logx/               text/JSON log files
internal/signalx/            SIGINT/SIGTERM listen, SIGWINCH (unix/windows split)
scripts/release.sh           cross-compile + SHA256SUMS
dist/                        release artifacts (5 binaries + sums)
```

## 10. Held plan: reusable terminal test rig (2026-08-18, HELD)

**Status: user approved the idea ("we will do"), explicitly HELD until a
go-ahead. Do NOT build this without the user saying go.**

**Why it exists:** 3 of the user's last 5 projects were command-line based.
The rig lessons (render the stream through an emulator and assert the visible
screen, never escape-list assertions; ONLCR traps; real-PTY resize proof) cost
five acceptance rounds on dohping — they should carry forward, not be re-paid
per project. Hard constraint the user added: the asset must be CI-ready — the
project will live on GitHub, so anyone must be able to run the suite with a
stock toolchain + stock Python, no agent, no sandbox.

**Deliverables (one pass, ~1h total):**

1. `~/.hermes/user/tools/scripts/pty-probe.py` — generalize
   `scripts/pty-resize-probe.py` (which stays in this repo as the dohping-
   specific instance):
   - `--binary PATH` (fixes the hardcoded `/tmp/dohping-test`), `--scenario NAME`,
     optional `--cols/--rows/--duration/--resize-to`.
   - `SCENARIOS` dict: `name -> fn(binary, opts) -> bool`. dohping's two
     scenarios ship as examples: `window` (freeze-and-restart on resize) and
     `plain` (live line anchored below minimum width).
   - Core untouched: `TermScreen` emulator, `pty.fork` capture, TIOCSWINSZ
     injection, DSR/CPR answering, `RESULT: PASS/FAIL` + exit 0/1.
   - stdlib-only (`pty`/`fcntl`/`termios`/`select`), POSIX — runs on
     Linux/macOS CI runners; deterministic (fixed durations, no interactivity).
2. `~/.hermes/user/tools/scripts/ci-terminal-probe.yml` — GitHub Actions
   workflow template (copy into `.github/workflows/`, adjust build step +
   scenario names): `rm -f` fresh build (stale-build trap), `go test -race ./...`,
   `go vet`, gofmt check, probe `window` + `plain`, release matrix
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
