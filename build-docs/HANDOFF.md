# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `LAUNCH.md` (build brief), `SPECIFICATION.md`
(the contract), `DECISIONS.md` (78 entries — each fix's rationale), `CHECKPOINT.md`
(gate status), `PROGRESS.md` (timeline). `README.md` is the user-facing doc.
`SPEC-window-resize-reclaim.md` is the design for the shipped resize behavior.

**Status: build complete. All 6 phases green. Window-mode resize is RESOLVED —
the reflow-aware in-place reclaim is SHIPPED (DECISIONS #78; dist sha
8dcce21a…, byte-reproduced via `release.sh`).**

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

Full per-fix history: DECISIONS.md (78 entries, each with rationale).

## 4. Pending / next actions

- **Publish-prep TODO (user-flagged 2026-08-18):** user to get their GitHub
  numeric ID for the no-reply commit email (`<ID>+<username>@users.noreply.github.com`).
  Deliberate opsec: public commits must not advertise agent involvement —
  commit identity reads as the project owner. Then: set
  `git -C publish config user.name` / `user.email` to the user's identity
  (the publish script only sets identity on repo init, so it won't overwrite),
  **amend the baseline snapshot commit `fefce20`** (currently authored
  "Hermes Agent <hermes@hermes.home>") BEFORE the first push — nothing pushed
  yet, so the rewrite is free — and fill the LICENSE holder line with the
  same name. Public history should then carry zero agent trace.

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
- **README is the only public doc.** It must never reference build-docs/
  files (scrubbed 2026-08-18; development section now points at release.sh +
  pty-resize-probe.py).
- **LICENSE = MIT**, holder line still `<YOUR NAME HERE>` — user must fill
  before first push. Publish commits inherit the private repo's git identity
  (Hermes Agent <hermes@hermes.home>) unless the user overrides in publish/.
- Pitfalls paid for: bare `dohping` in the public .gitignore matches the
  `cmd/dohping/` DIRECTORY (root-anchor: `/dohping`); `.gitignore` must not
  appear twice in the expected list; a nested repo is staged as a 160000
  gitlink unless the parent ignores `publish/`.
- When the held §10 CI workflow is built, add `.github/workflows` to
  PUBLIC_ITEMS in the script.
