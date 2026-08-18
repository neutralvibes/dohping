# SPEC — Window-mode resize: reflow-aware in-place reclaim

Status: **APPROVED + SHIPPED 2026-08-18 (DECISIONS #78).** Built as
DECISIONS #75, rejected on a stale-binary report (#76), properly retested
(#77: "holds up really well but is not faultless") and promoted after B's
direct rejection ("0014967e is out… This one should be set as the shipped
build"). Shipped linux-amd64 = 8dcce21a…, byte-reproduced via release.sh.
Non-reflowing terminals: the crossing reclaim is a documented limitation
(§4, option A). Author: Hermes. Date: 2026-08-18.
Read with: DECISIONS #70–78 (the history), SPECIFICATION.md §8.

---

## 1. Requirement (the contract, unchanged)

SPECIFICATION.md §8.5:

> Window mode should redraw only the fixed display region. **It should not
> rely on normal terminal scrollback.** … clear stale characters … handle
> terminal resize gracefully.

User constraints (2026-08-18, binding):

- The block stays where it is — anchored where it first appears. **No
  bottom-of-screen pinning.**
- The column trim (DECISIONS #71) stays: "the trimming was good."
- Plain view is untouched, including its approved freeze-and-restart
  (DECISIONS #69).
- A resize should leave the screen looking like nothing happened.

## 2. Why the current behavior fails the requirement

The freeze-and-restart resize path (DECISIONS #67, window mode) does the
opposite of §8.5: on a crossing it **abandons the block**, restarts below
it, and leaves the old rendering in scrollback. The user's test of round
15/16 produced **three blocks on screen** for one drag session — copies
accumulate. That is a spec violation, not a cosmetic quirk.

Round 15's defer (DECISIONS #73) made it measurably worse, in the user's
own words ("now it seems to wrap nearly every time… frozen blocks take
very little effort now") and in their own debug log: the freeze at
`77→75 rows 11→12` fired because the on-screen frame was **stale** (last
rendered at width 97, 79-cell lines) and wrapped at 75. Pre-#73, every
SIGWINCH repainted at the new width, so the frame tracked the drag and
above the floor the freeze was effectively unreachable (the round-13
claim "freeze reachable only below 46" was true then). The defer
suppressed mid-drag repaints to fix a transient shift and thereby made
the frame stale, which moved the freeze back above the floor and left a
visibly wrapped stale frame on screen at every pause. The user's
perception is accurate, and the cause is #73.

## 3. Design: reflow-aware in-place reclaim

### 3.1 The one assumption (and the evidence for it)

On a **reflowing** terminal (Windows Terminal, Terminal.app, iTerm2), a
width change re-wraps the on-screen content, and **the cursor follows its
content**: after the reflow, the cursor sits at the end of the last line
the app wrote, re-wrapped. The app therefore knows the anchor *without*
CPR: it is `R − 1` rows above the cursor, where R is the reflowed span.

Evidence it holds on the user's terminal: in the round-16 log, the
freeze-restart at 35.300 emitted `\r\n` + a fresh block that the
screenshot shows correctly below the frozen block — the restart landed
relative to the *reflowed* frame, so the cursor was where reflow put it.
(What broke in #66 was CPR *reports* — ConPTY's answer to a query —
not the physical cursor.)

The app computes R exactly: it holds the rendered rows of the last
completed frame (its own text, ANSI-stripped cell widths) and the new
width, so

```
R = Σ physicalRows(cellWidth(row), tw) over lastRows
```

This is the same sum observeResize already computes today for the
freeze decision (`rows 11→12` in the log — the machinery exists).

### 3.2 The reclaim (replaces the freeze, above the floor)

On a width change that would cross (R ≠ lastPhysRows), instead of
freezing:

1. **Still write nothing mid-reflow.** The settle hold (300 ms, clock
   restarted by every further change) stays — #73's safety is kept. The
   stale frame is visibly re-wrapped by the terminal during the drag;
   that is terminal behavior and unavoidable without writing mid-reflow.
2. **On settle, reclaim in place:**
   - Walk back `R − 1` rows from the cursor (the reflowed frame's top).
   - Write the new frame (N rows, trimmed at the new width, per-row
     `\x1b[K`) — this overwrites the reflowed stale rendering.
   - If `R > N` (block lost rows in the reflow), clear the `R − N` stale
     rows *below* the new block (`\x1b[1B\r\x1b[K` each), then return the
     cursor (`\x1b[<R−N>A`). Same-band case: R = N — byte-identical to
     today's in-place path.
   - If `N > R` (theoretical; block grew rows), write N rows from the
     anchor; rows below the old content are fresh (nothing is below the
     block during a run).
   - Bookkeeping: `lastPhysRows = N`, `lastRows = new frame`, `started`
     stays true, no CRLF, no restart, no frozen copy.
3. **The freeze survives only as the below-floor fallback** (below the
   46-cell essentials floor, where lines wrap by design and a wrapped
   block can exceed the screen: also when `R > th` — the reflowed span
   cannot fit one screen, so the anchor is unknowable). Those cases keep
   today's freeze → restart-below behavior unchanged.
4. `Finalize` reclaims like a settle (walk back R at the current width,
   write the final frame) — the block never restarts on a fresh row even
   at shutdown.

### 3.3 What the user sees

- During a drag: the block holds (no writes; the terminal re-wraps the
  stale rendering — transient, expected).
- ~300 ms after the drag stops: one clean repaint at the new width. The
  wrapped stale frame is gone. **No frozen copies, no persistent wrap,
  no dead-then-jump.**
- Repeated drags: still one block, always at its anchor.

## 4. The detection problem (the honest hard part)

The reclaim is correct **only on reflowing terminals**. On a
non-reflowing terminal (xterm, Linux console, tmux panes, Windows
conhost) the frame does *not* re-wrap — it occupies `lastPhysRows` rows
at its old rendering, and walking back `R − 1` (R ≠ lastPhysRows) lands
mid-frame: deterministic, permanent interleaving, **not** self-correcting.

The app cannot distinguish the two worlds: the only query that would
(CPR) is exactly the thing ConPTY lies about (#18725). The freeze was
the one behavior that is merely-suboptimal in both worlds.

Resolution options for the user (decision required, §7):

- **A. Reclaim, reflow-world assumed.** Correct on the user's terminal
  (WT). Non-reflowing terminals are documented as out of scope for
  crossing resizes (they keep the current freeze behavior, i.e. the
  pre-change status quo for them). The user's real-terminal test is the
  acceptance gate; the debug log records the reclaim for correlation.
- **B. Revert #73 (the defer) instead.** Restores the round-13/14 state
  the user accepted ("works much better" + the #72 lived-with note):
  fresh frames during drags, freeze reachable only below the floor,
  transient mid-reflow shifts return. Zero risk, zero new machinery, but
  the mid-reflow shift the user reported in round 14 comes back.
- **C. Keep current (post-#73).** The state the user calls worse.

## 5. Scope

- `internal/output/window.go` — the crossing path: freeze → reclaim
  (above floor), freeze kept below floor / R > th. Same-band path
  unchanged. `Finalize` switch from force-restart to reclaim.
- `internal/output/display.go` (plain) — **untouched**.
- `internal/app/app.go` — untouched (winch/tick already drive Redraw).
- `internal/debugx` — new decision line in the resize tag
  (`→ reclaim R=12 N=11 tw=75`), new `reclaimed phys=… (was …)` line in
  the redraw tag, so the user's screen test correlates with the log.
- Tests:
  - `termScreen` emulator gains a **reflowing** resize mode (re-wrap the
    buffer at the new width, cursor follows content) — the model for the
    user's terminal. Existing resize tests keep the non-reflowing mode.
  - Regression: crossing above the floor with reflow-mode emulator →
    exactly ONE block, no CRLF restart, no frozen copy; same-band with
    reflow mode → unchanged; below-floor crossing → freeze (unchanged);
    non-reflowing emulator crossing → freeze (unchanged, the option-A
    documented limitation); Finalize-mid-crossing reclaims.
  - PTY probe: existing scenarios unchanged (none crosses above the
    floor; `window-subfloor` 40→70 exercises the retained freeze).
  - The user's real-terminal drag test with `DOHPING_DEBUG` is the
    acceptance gate (option A).

## 6. Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| ConPTY physical cursor does not follow reflow | Low (evidence §3.1) | One-row transient shift, self-correcting next repaint — the #72 floor; user's screen test detects it; log correlates |
| Non-reflowing crossing regression (option A only) | Certain, if option A and such a terminal is used | Documented limitation; freeze retained for those cases by keeping the crossing freeze behind... **not detectable** — see §4. Honest: option A means non-reflowing crossings break |
| Emulator reflow model diverges from WT reality | Moderate | The user's screen test is the gate, not the emulator |

## 7. Decision requested

1. A (reclaim, reflow-world assumed — recommended: it is the only design
   that satisfies §8.5 on your terminal, and the log + your screen give
   us a testable gate), B (revert the defer), or C (keep current)?
2. If A: the below-floor fallback keeps today's freeze — confirm the
   46-cell floor as the boundary.
