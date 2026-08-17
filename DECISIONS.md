# dohping Decision Log

PRIVATE — project-internal reasoning record. Do not commit to public repositories (polish public, reasoning private).

Format: date | decision | rationale

## 2026-08-16

| # | Decision | Rationale |
|---|---|---|
| 1 | Position as a better interactive ping: compact stateful display for short sessions (minutes–days), not a long-term monitor | Real need: the information people actually want from ping, without scroll buffers of results; long-term monitoring is crowded (smokeping/Prometheus) |
| 2 | Probe abstraction with two implementations: ICMP (default) + TCP connect (`--probe tcp[:port]`, default 443); TCP lands in Phase 2, not Phase 6 | ICMP needs CAP_NET_RAW on Linux and is often firewall-filtered; TCP is stdlib-only, no privileges, loopback-testable |
| 3 | TCP semantics: established/refused → up, timeout → down, DNS/routing → error | Refused proves host presence (answers "no"); a silently dropped SYN is indistinguishable from unreachable — same limitation as ICMP, documented |
| 4 | Flag conflict policy: mutually opposed explicit flags error with exit 2 (`--no-window`×`--window-lines`, `--no-color`×`--color=always`, `--no-live`×`--live=on`); `NO_COLOR`×`--color=always` is NOT a conflict (env overrides flag) | No silent ambiguity; user-stated: "conflicting flags error out with message" |
| 5 | Interactive `q`/`Q` quit: same graceful shutdown path as SIGINT/SIGTERM, exit 0, TTY-only | Natural way to end an interactive session; exit 0 = deliberate completion, alongside `--count` |
| 6 | `--count` is core scope (Phase 1 parse + Phase 4 shutdown); it is the only other exit-0 path and enables scripted use | Was listed in the CLI table but missing from phases and tests — gap closed |
| 7 | `--no-window` is a real flag; `--window-lines` implies `--window` (compatible, not a conflict) | Tests referenced it while the options table didn't list it — parity fix |
| 8 | IPv6 in scope: ICMPv6, `dohping ::1` test, `[::1]` bracketing in logs | 2026 baseline for a professional tool; previously unmentioned anywhere |
| 9 | Column width policy: HOST computed once at startup (min 15, max 40, truncate with `…`), DURATION capped at `99d+` | Single host per run → width never changes mid-run; 100-day runs are out of scope by design |
| 10 | Short flags: `-V` (version), `-p` (probe), `-w` (window), `-d`/`-u` (down/up-after); `-v` deliberately avoided | `-v` conventionally means verbose in other tools; `-d`/`-u` is a memorable pair |
| 11 | Soak test scoped to 1 hour (12–24h dropped); exit codes `130`/`143` documented as Unix-only | Tool targets short interactive sessions; extended monitoring explicitly out of scope |
| 12 | Window mode remains Phase 5 (last core phase); `q`-quit works there; plain-line-in-scrollback is the default experience | Risk ordering unchanged; window mode is the compact-dashboard differentiator, not the default |
| 13 | Telegram notifications for build progress: watchdog cron (5 min) + append-only `PROGRESS.md` contract; notify on start, per-phase completion (with duration + gate), blockers, final summary; staleness alert if >2h quiet | User asked to be informed over Telegram; phase-granularity keeps it low-noise; watchdog is session-independent so it survives chat restarts |
| 14 | Build checkpoints: `CHECKPOINT.md` state file (phase, status, completed, next_action, notes) rewritten at phase boundaries and discrete chunks; pickup = read checkpoint + re-run current gate; gates remain ground truth | User asked for interruption-safe pickup; separates state record (CHECKPOINT.md) from notification feed (PROGRESS.md); "checkpoint is a map, gates are the terrain" |
| 15 | Name `dohping` = Homer Simpson "D'oh!" play, pronounced "dopping"; chosen for memorability — a stoner or knowledgeable admin should love it; accepted tradeoff: overlaps DNS-over-HTTPS (DoH) in search results | User-stated intent 2026-08-16; playful memorability is a feature for the target audience, the DoH collision is a fair price |

## 2026-08-16 (build session)

| # | Decision | Rationale |
|---|---|---|
| 15 | Module path `dohping` (no remote; local-only tool) | LAUNCH.md says ask if not obvious — it is obvious for a private tool; one-line change if the user prefers a namespaced path |
| 16 | CLI parsing via stdlib `flag`: short+long names registered as aliases to the same variable; `fs.Visit` tracks explicitly-set flags | Zero dependencies; aliases idiomatic; Visit separates default from explicit (needed for `--count 0` and conflict detection) |
| 17 | `--window` × `--no-window` is also a usage error (exit 2) | §16.4 enumerates three conflicts, but its principle is "mutually opposed explicit flags = usage error, never silent ambiguity"; window+no-window are literally opposed |
| 18 | Explicit `--count 0` rejected (must be ≥ 1); the 0 default means unlimited | A 0-probe run is a no-op; explicit rejection beats silent weirdness |
| 19 | `--timestamp-format` accepts exactly `HH:MM:SS` (default) or `rfc3339` | Spec §10.4 names both display and log formats; a closed set keeps validation strict |
| 20 | Phase 1 valid run (`dohping HOST`) prints "monitoring not yet implemented", exits 1 | Honest placeholder until Phase 2; valid-run behavior is outside the Phase 1 gate |
| 21 | ICMP socket open is two-tier: privileged raw (`ip4:icmp`/`ip6:ipv6-icmp`) → unprivileged ping socket (`udp4`/`udp6`, ping_group_range); both denied → operational error with permission guidance, exit 3 | Verified in this container: raw ICMP is blocked for our process (no CAP_NET_RAW in the netns-owning userns; seccomp filter). `/bin/ping` works only via sandbox elevation our process does not inherit. Spec §19 requires clear permission errors, never host-down |
| 22 | TCP timeout→down tested against TEST-NET-1 (192.0.2.1, RFC 5737): routed via the container gateway, silently dropped → `i/o timeout` | Deterministic down-state testing without privileges; gateway behavior verified empirically (1.2s dial timeout observed) |
| 23 | State engine: on a status-changing success, transition runs BEFORE the RTT is folded into stats, so the triggering probe is the first sample of the new up state; `transition()` resets stats so down-state recovery RTTs never leak into the next up state | Spec §5.3 (stats per current status) + Phase 2 test 6 (clean reset); naive add-then-transition lost the first sample |
| 24 | `FAILS` = the hysteresis consecutive-failure counter (increments on each failed probe, resets on success) | Literal reading of §5.4 "consecutive failed probes during current down state"; with default `--up-after 1` a success flips to up, so FAILS never shows 0-while-down in the default config |
| 25 | TCP `ECONNRESET` on dial treated as `up` (peer answered with RST — proof of presence), same class as refused | Same presence argument as §4.1 refused semantics; firewalls that RST-filter still prove the host's stack answers |
| 26 | Probe loop: fixed-rate ticker (interval between starts), probes serialized (never overlapping), results after ctx cancellation dropped (loop checks `ctx.Err()` before feeding the engine) | §4.2 rules; a probe that outlives the interval delays the next start (back-to-back), so `--down-after N` ≈ `N × timeout` when failing, as documented |
| 27 | Probe results during shutdown are silently dropped, not fed to the engine | A cancelled probe is not evidence about the host; the display/log finalize on the shutdown path instead |

## 2026-08-16 (Phase 3)

| # | Decision | Rationale |
|---|---|---|
| 28 | Column layout: TIME(8)+2sp, HOST(15–40)+1sp, STATUS(7)+1sp, DURATION(14)+1sp, MIN/MAX/AVG(7, right)+1sp, FAILS(8, right); header labels left-aligned; trailing whitespace trimmed | Column starts match the spec §7.4 header literal exactly (verified against it); the spec's own data examples are internally inconsistent (header DURATION@34 vs data @35), so goldens encode the self-consistent layout — "accurate enough, not completely accurate" |
| 29 | DURATION left-aligned in its 14-wide field (matches spec examples); numeric fields right-aligned; RTT always `%.2f` ms | §10.1 stable columns; §10.2 consistent decimals |
| 30 | Event contract refined: `ev.Stats` = post-event stats of the CURRENT state; `ev.PrevStats` = stats of the ended state (StatusChange/Error) | The display finalizes the old line from PrevStats/Duration and seeds the new line from Stats — the status-triggering RTT is the first sample of the new up state (Phase 2 decision 23 made this possible) |
| 31 | Live updates redraw per probe event with `\r` + line + `\x1b[K`; TIME stays the state-start time | §7.2; ESC[K is cursor hygiene for overwrites, not color — NO_COLOR/color rules disable colors only, never the live mechanism |
| 32 | `--color=always` does NOT force color on non-TTY stdout | Spec §11.3 rule 3 is unconditional: stdout-not-a-terminal disables color regardless of mode |
| 33 | IPv6 TCP to 2001:4860:4860::8888 yields `error` state in this container (no v6 route) | Spec §4.1: routing failure → error, never down; also confirms the error state renders correctly |

## 2026-08-16 (Phase 4)

| # | Decision | Rationale |
|---|---|---|
| 34 | Logs record each finalized status period (initial `unknown` skipped) plus the current state at shutdown; text per §14.4, JSON per §14.5 with RTT rounded to 2 decimals | One entry per real state period; spec examples show 2-decimal JSON values |
| 35 | Interactive quit uses raw-mode stdin (x/term): `q`/`Q` → exit 0, byte 0x03 (Ctrl-C with ISIG off) → exit 130, terminal restored on exit/EOF; piped stdin → no key handling | Instant `q` like mtr/htop while keeping the 130 exit-code contract; raw mode's ISIG-off is handled by mapping 0x03 explicitly |
| 36 | Exit summary printed only when stdout is a terminal; piped/scripted output stays clean | §2.5 predictable scripted behavior — a trailing summary would break parsers; "graceful exit" summary test interpreted as interactive |
| 37 | Log-file open failure exits 1 (general error) with a clear message | CLI is valid; it's an I/O error, not usage — exit 2 reserved for parse/validation |
| 38 | Log entries are fsync'd after each write | §14.6 "flushed reliably": a completed event must survive a crash; volume is tiny |

## 2026-08-16 (Phase 5)

| # | Decision | Rationale |
|---|---|---|
| 39 | Window mode renders via full redraw per event on the alternate screen: cursor home + header + bounded history + live line + clear-to-end | At probe cadence (default 1s) full redraw is flicker-free; per-redraw height re-read makes resize handling automatic; simpler and more robust than partial diffing |
| 40 | `--window-lines N` budgets N data lines: N-1 finalized history + 1 live; the header adds one row; terminal height < needed → visible lines reduced (min 1), never an error | Spec §8.3/§8.5 "reduce the number of visible lines"; matches the §8.7 example (header + 4 rows under --window-lines 5) |
| 41 | SIGWINCH (Unix) triggers an immediate redraw; Windows gets a no-op channel | Resize must repaint even between probe events; `//go:build` split keeps cross-compile clean |
| 42 | Full redraw writes `\x1b[H` + lines + `\x1b[J`; alt screen entered with `\x1b[?1049h`, left with `\x1b[?1049l` (exactly once each, verified in PTY capture) | §8.5 cursor positioning without flicker; clean terminal restore on every exit path |

## 2026-08-16 (Phase 6)

| # | Decision | Rationale |
|---|---|---|
| 43 | 1h soak runs TCP-loopback (privilege-free, deterministic); ICMP soak is impossible in this sandbox (no CAP_NET_RAW, ping_group_range excludes our gid — decision 21) and is documented as such | The soak's purpose is leak/crash detection, which the loopback path exercises identically |
| 44 | Release via `scripts/release.sh`: 5 targets, `-trimpath -buildvcs=false`, version injected via ldflags, SHA256SUMS; verified byte-identical across two builds | §Phase 6 test 10 reproducible release; no git repo → documented build process + checksums |
| 45 | govulncheck DONE (user approved): 0 reachable vulnerabilities; x/net v0.58.0 + x/term v0.45.0 clean; the 8 reported findings are all stdlib (go1.26.5) issues fixed in go1.26.6, none reachable. golangci-lint DONE (user approved): 4 errcheck findings fixed (all in test files), clean now. gosec is the only remaining deferred tool | Dependency supply-chain certified; stdlib findings are an environment toolchain bump (go1.26.6), not a code change — LAUNCH.md forbids re-downloading the toolchain |
| 46 | Coverage: state 100%, output 94.3%, cli 99.1%, logx 90%, ping 36.3% — the ICMP probe paths are untestable in this sandbox (permission contract verified instead, decision 21) | Spec requires high coverage on state + formatting (met); ping's gap is environmental, not deliberate |
| 47 | External verification: `--probe tcp 1.1.1.1` → up, ~8.7 ms RTT; `::1` loopback up; TEST-NET-1 down; IPv6 Google DNS → error (no v6 route) | Real-world up/down/error states all exercised end to end |
| 48 | ICMP network behavior verified via the `ping` oracle (sandbox-elevated binary): v4 loopback, v6 loopback, external 1.1.1.1 all 0% loss; dohping TCP RTT (8.03 ms) matches ping's ICMP RTT (8.04 ms) on the same target | Our process cannot open raw/ping sockets (decision 21), but the oracle proves ICMP works in this environment and cross-validates dohping's RTT measurement path; dohping's own ICMP socket contract (permission error, hint, exit 3) remains the verified path |
| 49 | ICMP probe is now THREE tiers: raw socket → unprivileged ping socket → system `ping` command (iputils-oriented parser). Verified live in this sandbox: `dohping 1.1.1.1` up at 8.09 ms, `::1` up (ICMPv6 via `-6`), `192.0.2.1` down (ping exit 1). The permission-error path is preserved for when ALL tiers fail (test forces it via PATH) | User direction: "you can use ping" — the default probe must WORK in restricted environments where the sandbox elevates /bin/ping but our process has no socket privileges. Trade-off: the parser is Linux-iputils-oriented; macOS/Windows prefer the socket tiers (macOS ICMP is unprivileged; Windows admin-only raw) |

## 2026-08-17 (user acceptance fixes)

| # | Decision | Rationale |
|---|---|---|
| 50 | Consecutive operational errors do NOT re-enter the error state: first error transitions (EventError), subsequent errors emit EventProbeError and only update the live line's duration | User report: `error` status repeated on a new line every probe; a status must not repeat — one line, duration updating as usual |
| 51 | Permission-class errors at PROBE time (from any tier, incl. ping exit-2 "Operation not permitted") abort the run with exit 3 and the guidance hint instead of probing in error state forever; ping-tier stderr permission messages are wrapped with syscall.EPERM so IsPermissionError catches them | User report: "ping can tell you there is a priv problem straight away, this should error out"; a privilege problem is permanent, never a host condition |
| 52 | MIN/MAX/AVG/FAILS values are LEFT-aligned in their fields, starting directly under the header labels | User report: "MIN, MAX and AVG columns are not in place" — right-aligned values drifted from their headers; left-alignment makes the up line byte-identical to the spec §7.4 example and matches the header exactly (all fields are fixed-format, so nothing is lost) |
| 53 | Window mode renders IN PLACE on the normal terminal: a fixed-size block (header + last N events + live line) rewritten with cursor-up + per-row clear-to-EOL only. NO alternate screen, NO cursor-home, NO clear-to-end-of-screen. Enter/Exit are no-ops; the block stays visible after exit, and the exit summary prints below it | User correction: "There is nothing about clearing the screen. It should display exactly as it does now except only show x number of the last events at place at the screen." — §8 never asks to clear or take over the screen; the alt-screen swap (DECISIONS #39) and cursor-home repaint (#42) were implementation choices, not spec requirements. Padding rows keep the block height constant so it never grows into the terminal or relies on scrollback |
| 54 | In-place window redraws must reset the column explicitly: cursor-up is emitted as `\x1b[<n>A\r` (CR after CUU — CUU preserves the column, so without CR every redraw starts mid-line and leaves stale fragments), and rows are separated by `\r\n` (bare LF moves down but does NOT reset the column; relying on the terminal's ONLCR translation masked this in the sandbox PTY but not on the user's terminal) | User report: `dohping --window-lines 5 google.com` showed repeated headers and fragments like `348.00  348.00  348.00` on each row — the classic symptom of writing at a preserved cursor column. Fixed by explicit column reset; regression test renders the stream through a mini terminal emulator (termScreen) and asserts the VISIBLE screen grid, which escape-stream assertions cannot catch |
