# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `LAUNCH.md` (build brief), `SPECIFICATION.md`
(the contract), `DECISIONS.md` (63 entries — each fix's rationale), `CHECKPOINT.md`
(gate status), `PROGRESS.md` (timeline). `README.md` is the user-facing doc.

**Status: build complete. All 6 phases green. Acceptance rounds 1–5 shipped
(DECISIONS #50–61, 2026-08-17), the run-duration docs decision (#62), and the
gosec security round (#63). User-verified so far: window mode in place, flags on
either side of HOST, bare-seconds interval/timeout, clean exit summary, and the
liveness animation. The user is mid-testing; expect more acceptance reports.**

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
- **Flags on either side of HOST** (`dohping google.com -c 5` == `-c 5 google.com`)
  — GNU-style permutation (#55). `--` still terminates flags.
- **`-i`/`-t` accept bare seconds** (`-i 5` = 5s, ping convention) plus full
  duration strings (`500ms`, `1m30s`) (#57). Zero/negative still rejected.
- **Exit summary is a clean left-aligned block** at column 0 even on terminals
  without ONLCR — explicit `\r` per line, `\r\n` terminators (#56).
- **Liveness animation**: live line shows a rising bar `▁▃▅▇` in the DURATION
  field's padding (column 47, floats between DURATION and MIN). Advances on a
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

## 4. Pending / next actions

- **User is still testing** — no outstanding agent tasks. If the user reports
  another acceptance issue: reproduce, fix, add regression test, re-run gates,
  rebuild `dist/` via `scripts/release.sh`, republish to the rig (§6), record
  DECISIONS + CHECKPOINT entries, commit.
- **Animation is user-testing territory**: the rising-bar placement (col 47) and
  the 1-second ticker were both corrected after user reports — if placement or
  cadence comes up again, verify against the rendered-screen tests first.
- Areas the user has NOT explicitly verified yet (candidates to probe if asked):
  TCP probe mode (`-p tcp`), `--log-file` output (now 0600 — user may notice),
  `--timestamp-format rfc3339`, terminal-resize behavior in window mode, the
  darwin/windows binaries (built but never run on those OSes — Windows signal
  codes are documented as closest-conventional, spec §18).
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
- Current published linux-amd64 sha: `09fc4d339cb3…` (2026-08-17, gosec round: log 0600 + G104 fix #63).
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
- Regression tests must accompany every acceptance fix (that's the established
  pattern, #50–63 all have them).

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
