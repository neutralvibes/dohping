# dohping — State of Play (handoff for a new chat)

Read order: this file first, then `LAUNCH.md` (build brief), `SPECIFICATION.md`
(the contract), `DECISIONS.md` (54 entries — each fix's rationale), `CHECKPOINT.md`
(gate status), `PROGRESS.md` (timeline). `README.md` is the user-facing doc.

**Status: build complete. All 6 phases green. User acceptance fixes #50–54 shipped
and user-verified for window mode (2026-08-17). The user is mid-testing; expect
more acceptance reports.**

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
- **Three-tier ICMP**: raw socket → ping socket → system `ping`. Verified live:
  `1.1.1.1` up ~8ms, `::1` up (ICMPv6), `192.0.2.1` down (deterministic timeout).
- **Error handling**: consecutive errors = one line, duration updates; permission
  problems abort exit 3 + hint (never misreported as host-down).
- **Signals**: SIGINT→130, SIGTERM→143, q-quit→0, `--count`→0.
- **Cross-compile matrix**: linux/darwin/windows × amd64/arm64 in `dist/`,
  reproducible via `scripts/release.sh`.
- **Gates**: 7/7 packages race-clean (`go test -race -count=1 ./...`),
  gofmt/vet clean. golangci-lint + staticcheck were run at Phase 6 (installed
  in ~/go/bin) — re-run them if the toolchain box changes.

## 3. The five acceptance fixes (DECISIONS #50–54, all shipped)

| # | Fix | User report it answered |
|---|---|---|
| 50 | Consecutive errors don't re-enter error state; `EventProbeError` updates live line duration in place | "error status repeated on a new line" |
| 51 | Permission-class probe errors abort exit 3 + hint (all tiers, incl. ping exit-2 wrapped as EPERM) | "ping can tell you there is a priv problem straight away" |
| 52 | MIN/MAX/AVG/FAILS left-aligned under headers | "MIN, MAX and AVG columns are not in place" |
| 53 | Window mode in place on normal terminal, no alt-screen/clear | "nothing about clearing the screen... at place at the screen" |
| 54 | Explicit column reset: `\x1b[<n>A\r` on redraw, `\r\n` between rows | fragmented rows (`348.00 348.00 348.00` / repeated headers) |

## 4. Pending / next actions

- **User is still testing** — no outstanding agent tasks. If the user reports
  another acceptance issue: reproduce, fix, add regression test, re-run gates,
  rebuild `dist/` via `scripts/release.sh`, republish to the rig (§6), record
  DECISIONS + CHECKPOINT entries.
- Areas the user has NOT explicitly verified yet (candidates to probe if asked):
  TCP probe mode (`-p tcp`), `--log-file` output, `--timestamp-format rfc3339`,
  terminal-resize behavior in window mode, the darwin/windows binaries (built
  but never run on those OSes — Windows signal codes are documented as
  closest-conventional, spec §18).

## 5. Environment (critical — read before running anything)

```sh
export GOROOT=/home/hermes/.hermes/go-toolchain/go
export GOCACHE=/home/hermes/.hermes/go-cache
export PATH="$GOROOT/bin:$PATH"        # ORDER MATTERS: GOROOT before PATH export
```

- go1.26.5 linux/amd64, module `dohping` (local-only, no remote).
- Deps: golang.org/x/net v0.58.0 (icmp), golang.org/x/term v0.45.0.
- Linters at `~/go/bin/golangci-lint`, `~/go/bin/staticcheck` (from Phase 6).
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
- Current published linux-amd64 sha: `e8be5cfdc2c8…` (2026-08-17, acceptance round 5: tick refreshes DURATION #61).
- Full rig knowledge: skill `file-serve-rig`.

## 7. Test/debug workflow that works

- Deterministic `down` tests: `192.0.2.1` (TEST-NET routes out here and times
  out deterministically).
- PTY capture: `script -qec "<cmd>" /tmp/cap.txt` — BUT the sandbox PTY applies
  ONLCR (`\n`→`\r\n`), which HID the #54 column bug. To prove terminal output,
  RENDER the stream through a terminal emulator and assert the visible screen
  (the `termScreen` in `internal/output/window_test.go` does this) — escape-list
  assertions are not enough.
- Regression tests must accompany every acceptance fix (that's the established
  pattern, #50–54 all have them).

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
