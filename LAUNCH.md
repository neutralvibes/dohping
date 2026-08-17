# dohping — Build Launch Note

**Purpose:** hands a fresh Hermes session everything needed to build `dohping` phase by phase, each phase verified against its acceptance tests with real tool output. Read this entire note and the three project docs in full before writing any code.

**Session contract:** spec-first, results-judged. The spec's tests are the contract. Never fabricate output — run real commands and paste real results.

---

## 1. Mission

Build `dohping` (Go ping/status utility) per `SPECIFICATION.md`, following `Phases.md` in order (1 → 6). Each phase is **done only when its acceptance tests pass with real output**. No skipping, no gold-plating, no out-of-scope features.

## 2. Where everything lives

- Project root: `/home/hermes/.hermes/user/projects/dohping/`
  - `SPECIFICATION.md` — the contract. **Read fully first.**
  - `Phases.md` — implementation plan; per-phase deliverables, tests, acceptance gates.
  - `ENGINEERING_PRINCIPLES.md` — house standard (layering, testability, probe contract, output rules, CLI contract, dependencies).
  - `DECISIONS.md` — **PRIVATE** reasoning log. Append a dated entry for every design decision made during the build. Never commit it anywhere public. No git repo exists — do not create one unless the user asks.
  - `LAUNCH.md` — this file.
- Code output goes in the project root (scaffold `cmd/dohping`, `internal/app`, `internal/cli`, `internal/ping`, `internal/state`, `internal/output`, `internal/theme`, `internal/logx`, `internal/signalx`, `internal/version` per spec §20.2).
- Original attachment copies: `/home/hermes/.hermes/attachments/` (project folder is canonical now).

## 3. Environment facts (verified)

- **Go toolchain already installed:** `GOROOT=/home/hermes/.hermes/go-toolchain/go` (go1.26.5), `GOCACHE=/home/hermes/.hermes/go-cache`. **NEVER download or install another toolchain.** Export `PATH="$GOROOT/bin:$PATH"` if needed.
- **Real ICMP works from this container** (verified 2026-08-16: `ping 1.1.1.1` ≈ 8.3 ms). Real probing, down-state testing (e.g. unroutable targets), and the 1-hour soak are all doable here.
- **Dependencies:** `golang.org/x/net` (ICMP), `golang.org/x/term` (raw-mode key reading, terminal detection). `go mod download` has network.
- Hermes Python venv is sealed — if Python tooling is ever needed, use `tools.lazy_deps.install_specs`, not pip.
- Layout rule: everything under `/home/hermes/.hermes/user/` — never create new top-level directories.
- `hermes-dashboard.service` is the live chat transport — never kill/restart it mid-session.

## 4. Process rules (user's — treat as hard)

1. **Spec-first.** Where the spec is silent or ambiguous: check `DECISIONS.md`; if still unresolved, **ASK THE USER** — never silently invent design decisions.
2. **Phases in order, no skipping.** Phase N is done only when its acceptance tests pass.
3. **Verification is mandatory and real:** `go build ./...`, `go vet ./...`, `go test -race ./...`, `gofmt -l .`, `golangci-lint run` (if available), plus the manual acceptance commands from Phases.md (`--help`, `--version`/`-V`, missing host → exit 2, invalid values → exit 2, conflict flags → exit 2, `--count` → exit 0, piped-output hygiene, `q`-quit, signal tests). Paste real output into the chat.
4. **DECISIONS.md:** append `date | decision | rationale` for every design decision made during the build; add a one-line note for non-trivial bug fixes.
5. **No git init** unless the user asks.
6. **Clutter-free:** remove staging copies once deployed; never delete project files without asking.
7. The user may steer mid-build — absorb corrections without drama, don't re-litigate settled decisions (§8).
8. **Partnership style:** genuine pushback welcome; say plainly what does and doesn't pay; never assume preferences — ask.

## 5. Error handling & retry policy

Most build errors are deterministic — the fix is to read the error and change something, never to blind-retry. Triage every failure into one of three buckets:

**1. Deterministic** (compile errors, vet failures, most test failures):
- Fix, then re-run. Each attempt must change something based on reading the error. Blind re-runs are forbidden.
- After any fix, re-run the FULL phase gate (not just the failing test) — a fix can break something else.

**2. Transient** (network hiccups, `go mod download`, proxy timeouts):
- Retry up to 3 times with short backoff (≈2s, 5s, 10s). If still failing after 3, report the evidence and ask the user — do not keep retrying.

**3. Flaky** (timing-dependent tests, race-detector oddities):
- Re-run exactly once to confirm. Two consecutive failures = a real bug: fix it. Never dismiss a failure as "flaky" without that evidence — and a confirmed flake is a bug in the test or the code, not a shrug.

**Anti-grind escalation — the definition of "grinding":**
- 3 consecutive attempts on the same problem with an UNCHANGED failure mode = no progress = stop. Escalate: summarize what was tried and the evidence, then either ask the user, simplify the approach, or offer a model swap for that chunk.
- Progress means the failure mode changed (different error, further through the suite). Same error three times is the trigger — not wall-clock — but if one problem has consumed ~20 minutes of attempts, apply the same trigger.
- Never silently keep hammering. Stopping with an honest status report is a success, not a failure.

**Session safety (crash-recovery):**
- The acceptance gates are the source of truth for phase completion — not memory of earlier success. If a session is interrupted or the user steers mid-phase, re-run the phase gate from scratch before declaring the phase done. The checkpoint protocol (§11) makes pickup concrete.
- When a sub-agent chunk fails: retrieve its partial output, fix and integrate yourself, re-verify. Don't re-dispatch the same task blindly — children are leaf-only and their claims need verification anyway (§6).

## 6. Model & delegation (user-approved)

- Run as the configured fast/cheap model — that is the deliberate cost-effective choice for this work: the spec is test-complete, so the test suite (race detector, golden tests, acceptance commands) is the correctness safety net, not model strength.
- **Sub-agents and orchestration are allowed** (user-stated). Use `delegate_task` for parallelizable chunks: per-package unit tests, golden-test authoring, static-analysis passes, cross-compile matrix. The main agent orchestrates and integrates all results.
- **Children are leaf-only** (`delegation.max_spawn_depth=1`): children cannot spawn further, and they inherit the parent model unless `delegation.model` is pinned in config.
- **Verify sub-agent claims yourself** (paths, test output) — a child's "tests pass" is a self-report, not proof.
- If a phase genuinely needs heavier reasoning and the model hits the anti-grind trigger (§5), tell the user and offer a model swap (`hermes model`) rather than continuing to thrash.

## 7. Phase quick-reference (acceptance gates)

| Phase | Gate |
|---|---|
| 1 — Foundation/CLI | Flags parse incl. all shorts; `--help` complete; `--version`/`-V` exit 0; missing host exit 2; invalid values exit 2; conflicts exit 2; build/vet/test/gofmt clean |
| 2 — Probe + state engine | Deterministic tests w/ fake pinger + fake clock; hysteresis (`--down-after`/`--up-after`); RTT min/max/avg; stats reset on state change; per-state duration; `error` state; race detector clean; TCP semantics vs loopback (refused = up, timeout = down, DNS/route = error) |
| 3 — Plain display/color | Header + `--no-header`; stable fixed-width columns; live-update in TTY only; finalization on status change; non-TTY hygiene (no ANSI/CR); `--quiet`; color rules incl. `NO_COLOR` override and `TERM=dumb`; golden tests incl. host-width (IPv6) and `99d+` guard |
| 4 — Logging/signals | Log create + append; text/JSON formats; quiet still logs; no ANSI in logs; SIGINT → 130, SIGTERM → 143 graceful (finalize + flush); `q`-quit → 0; `--count` → 0; log-file errors exit non-zero; `[::1]` bracketing |
| 5 — Window mode | `--window` / `--window-lines N` (implies window); rolling window; live current line; non-TTY fallback + stderr warning; resize; terminal restore; `q`-quit works; PTY tests |
| 6 — Hardening/release | Real ICMP + TCP verification (`127.0.0.1`, `::1`, external host); permission-error hints (CAP_NET_RAW); 1h soak; cross-compile matrix (linux amd64/arm64, darwin amd64/arm64, windows amd64); vet/staticcheck/golangci-lint/gosec/govulncheck; coverage on state + formatting; docs; reproducible release |

## 8. Already settled — do not re-litigate

- **Positioning:** a better *interactive* ping for short sessions (minutes–days); NOT a long-term monitor. Display is the product; event log is a bonus.
- **Probe abstraction:** `Probe` interface; ICMP default + TCP connect (`--probe tcp[:port]`, default 443); TCP in Phase 2, not later.
- **TCP semantics:** established/refused → up; timeout → down; DNS/routing → error.
- **Conflict policy:** mutually opposed explicit flags error with exit 2 (`--no-window`×`--window-lines`, `--no-color`×`--color=always`, `--no-live`×`--live=on`); `NO_COLOR` overrides `--color=always` without error.
- **Interactive `q`/`Q` quit:** TTY only, same graceful shutdown path as signals, exit 0.
- **`--count`:** core scope (Phase 1 + 4); the other exit-0 path.
- **Column widths:** HOST computed once at startup (min 15, max 40, truncate `…`); DURATION capped `99d+`.
- **Short flags:** `-h -V -i -t -c -q -l -p -w -d -u`; `-v` deliberately avoided.
- **Soak:** 1 hour; exit codes `130`/`143` documented as Unix-only.
- **IPv6 in scope:** ICMPv6, `dohping ::1` test, `[::1]` in logs.
- **Down-after semantics:** `--down-after N` ≈ `N × timeout` wall-clock when failing (serialized probes) — document, don't change.

## 9. First actions

1. Read `SPECIFICATION.md`, `Phases.md`, `ENGINEERING_PRINCIPLES.md`, and `CHECKPOINT.md` in full.
2. `export PATH="$GOROOT/bin:$PATH"`; confirm `go version` → go1.26.5.
3. Scaffold module (`go mod init` — module path decision: ask the user if not obvious), then execute Phase 1.
4. Append the STARTED line to `PROGRESS.md` (format in §10) and set `CHECKPOINT.md` to phase 1 in_progress (format in §11) before Phase 1.
5. Report real output per gate after each phase; update DECISIONS.md as decisions arise.

## 10. Reporting & notifications (Telegram)

The user is notified on Telegram automatically at key moments. Mechanism: the build session appends one line per milestone to `/home/hermes/.hermes/user/projects/dohping/PROGRESS.md` (append-only); a watchdog cron job (every 5 min) forwards new lines to the user's Telegram. This is session-independent — it survives chat restarts and needs no action from the build session beyond appending lines.

Progress file line format (one line per event; never edit earlier lines):

    2026-08-17T01:05+01:00 | STARTED | build began — phase 1/6
    2026-08-17T01:47+01:00 | COMPLETE | phase=1 duration=42m gate=green
    2026-08-17T02:31+01:00 | BLOCKER | phase=2 needs decision: <one-line summary>
    2026-08-17T05:12+01:00 | DONE | all 6 phases total=4h12m

Rules:

- Append `STARTED` once at the beginning and `DONE` at the very end.
- Append `COMPLETE` at every phase completion: phase number, wall-clock duration for the phase (measure with `date +%s` at start/end), and gate result (`gate=green` or `gate=FAILED <reason>`).
- Append `BLOCKER` immediately when the anti-grind trigger fires (§5) — the user must be alerted without waiting for the chat. Then ask in the chat.
- Phase granularity only: never notify for sub-steps, per-test results, or routine fix loops.
- Keep lines one-line and self-contained (plain text; Telegram-safe).

## 11. Checkpoints & resumption

The build is resumable after any interruption. State lives in `CHECKPOINT.md` in the project root — written by the build session, read on every session start. PROGRESS.md (§10) is the notification feed; CHECKPOINT.md is the state record. Keep them separate.

File format (rewritten, not appended — it reflects CURRENT state):

    # dohping build checkpoint
    updated: 2026-08-17T00:05+01:00
    phase: 2
    phase_status: in_progress
    completed: phase1
    next_action: implement state-engine transitions; run Phase 2 gate
    notes: probe interface + TCP probe done; fake pinger done; hysteresis tests failing

When to update:

- Phase start: `phase=N`, `phase_status=in_progress`, `completed=<list>`, `next_action=phase N goal`.
- Phase gate green: `phase_status=complete`, `next_action=start phase N+1`. (Also append COMPLETE to PROGRESS.md.)
- After any discrete chunk within a phase (probe done, golden tests authored, lint fixed): refresh `next_action` and `notes` — cheap, makes mid-phase pickup precise.
- Any time the session stops (signal, user steer, error): write the checkpoint FIRST, then stop.

Pickup protocol (on every session start or resume):

1. Read `CHECKPOINT.md` — it says where you were.
2. Re-run the current phase's gate (build + tests). The gate says what is TRUE — never trust the checkpoint over the gate. Green → mark complete, proceed. Red → resume from the failing state; the error output plus `next_action`/`notes` tell you exactly where you are.
3. Only then continue (next phase or the failing sub-step).
4. After pickup, refresh the checkpoint's `updated` line.

Rule: the checkpoint is a map; the gates are the terrain. If they disagree, the terrain wins.
