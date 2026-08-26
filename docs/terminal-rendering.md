# Terminal rendering in dohping

This page explains how dohping draws to the terminal, why it is more involved than ordinary CLI output, and the reasoning behind the current approach. You do not need it to use dohping. It is aimed at people who might maintain or extend the code in `internal/output/`.

## The problem

dohping updates a live line, or a fixed block of lines, in place while it runs. In theory that is simple: write a carriage return, overwrite the line, repeat. In practice terminals get in the way.

### Reflowing terminals

Modern terminals (Windows Terminal, Terminal.app, iTerm2, and most Linux emulators) reflow text when the window is resized. If a line was 100 cells wide and the terminal shrinks to 60 columns, the emulator re-wraps the existing text: the tail of the line moves down to the next row, and the cursor position the program thinks it has is now wrong.

The program cannot observe this. The terminal does not send an event saying "I just moved your line down two rows." It only reports the new width, not where the previously written text ended up.

### ConPTY lies about cursor position

On Windows the terminal stack includes ConPTY (Console Pseudo-Terminal). When a program queries the cursor position with DSR/CPR (Device Status Report / Cursor Position Report), ConPTY answers from its internal buffer state, which is not the rendered position on screen after a reflow. Any strategy built on "ask the terminal where I am, then move from there" is therefore unreliable on Windows Terminal and WSL.

Reference: [microsoft/terminal#18725](https://github.com/microsoft/terminal/issues/18725)

### The naive approach fails

If you write `\r` plus new content on every update, and the terminal has already reflowed your previous output, you write to the wrong row. The result is fractured output: stale fragments, duplicated lines, or text drifting down the screen.

## How dohping reacts to a resize

dohping never tries to track where the terminal moved its text. While the width is moving it freezes its drawing, waits for the width to settle, then picks one of three outcomes:

- Same band: repaint in place. No frozen copy.
- Crossing above the essentials floor: reclaim the block in place.
- Crossing below the essentials floor: freeze, then restart on a fresh row.

The next sections define the terms and explain each outcome.

### The 300 ms settle window

`resizeSettleDelay = 300 * time.Millisecond`

The first observed width change stops all in-place writes and starts the clock. Every further change before the window expires restarts the clock. A resize drag therefore holds the display frozen until the user stops dragging and the width stabilises.

### Same band vs crossing

A width change is classified into one of two groups:

- Same band: every row of the last completed frame occupies the same number of physical rows at the new width as it did at the old width. The reflow cannot have moved the block.
- Crossing: at least one row changes its physical row count, for example a line that fit on one row now wraps onto two. The reflow may have moved the block.

The classification decides what happens after the settle window. A same-band change repaints in place. A crossing above the floor reclaims; a crossing below the floor freezes and restarts.

Even a same-band change waits for the settle window. The terminal is still mid-reflow while the user drags, and a redraw landing at that moment writes to a canvas that is still shifting, which can move the block by a row. So same band waits too; it just does not leave a frozen copy behind.

## Reclaim in place: the normal case above the floor

When a crossing settles above the essentials floor, dohping can recover its block in place. It does not need to know where the terminal moved the text, because the terminal reflows content in place and the cursor follows its content, so the reflowed position is computable from dohping's own data.

1. The last completed frame was rendered at width W1 and occupied R1 physical rows.
2. The terminal reflowed that frame to the new width W2. dohping computes R2, the number of physical rows the same frame would occupy at W2.
3. The top of the reflowed frame is exactly R2 minus 1 rows above the current cursor position.
4. dohping walks back R2 minus 1 rows, overwrites with the fresh frame, and clears any stale rows below it.

No frozen copy, no restart. The block is reclaimed where it already is.

This works only when the fresh frame itself fits without wrapping. That is what the essentials floor guarantees.

### The essentials floor

The minimum meaningful line width is 46 cells (TIME, HOST, STATE, DURATION). Below that even the essential columns wrap. When the terminal is narrower than this floor, the fresh frame wraps by design and its anchor is unknowable, so the reclaim math cannot guarantee a clean result and the freeze fallback applies.

## Freeze and restart: the fallback below the floor

When dohping cannot safely reclaim (a crossing that settles below the essentials floor), it falls back to freeze and restart:

1. Freeze: stop all in-place writes immediately.
2. Wait: hold until the width has been stable for the settle period. This covers resize drags, where the width is changing rapidly.
3. Restart: write a newline and begin drawing on a fresh row below the frozen, reflowed rendering.

The old rendering stays in scrollback as history. The live data continues on a new row. The user sees a brief pause during the drag, then clean output resumes.

This fallback is position independent. It behaves the same on reflowing and non-reflowing terminals and needs no terminal cooperation. It is used only where reclaim cannot be.

## Plain line mode vs window mode

### Plain line mode

- One live line updates in place while the status is unchanged.
- On a status change the line is finalized (printed with a newline) and a new live line begins below it.
- The display tracks how many physical rows the previous live line occupied. Before rewriting it walks back to the true start and clears rows the line no longer uses, for example when the terminal grew and the line no longer wraps.
- Column widths are fixed for the run, because plain mode's history is the terminal's scrollback, which cannot be re-laid-out.

### Window mode

- A fixed block of N data lines plus a header, drawn in place on the normal terminal.
- No alternate screen, no clear-to-end-of-screen. Content above and below the block is untouched.
- Every redraw re-measures the terminal: the width drives the HOST column (expand, retract, truncate) and the height drives the visible line count.
- The block is always the same height, padded with blank rows when there are fewer events, so it never grows into the terminal.
- Cursor math counts physical rows, not logical lines. A wrapped line occupies several rows, and the cursor-up count must account for this.

## Resize detection

### Where resize awareness comes from

1. SIGWINCH (Unix): immediate fast path. The signal handler triggers a tick, which makes the display re-read the width and act.
2. The 1-second tick: the display re-measures the terminal on every tick. This is the only mechanism on Windows (no SIGWINCH), so Windows self-heals within one second.
3. Probe events: every probe result also triggers a redraw, which re-measures.

### Force render

On graceful shutdown (finalize) the display forces a render immediately, without waiting for the settle window. The final block has to land correctly now, not after a delay.

## Why DSR/CPR was removed

An earlier version used DSR/CPR to re-anchor after a resize: query the terminal for the cursor position, compute where the block should be relative to it, and rewrite.

This was abandoned because:

- ConPTY reports unreliable positions on Windows Terminal and WSL. The answer reflects ConPTY's internal buffer state, not the rendered view after reflow.
- It required terminal cooperation that is not universally reliable.
- The reclaim and freeze approach is simpler, provably correct, and works everywhere without querying the terminal.

The CPR machinery was removed entirely. No cursor queries, no re-anchors. The display trusts only its own accounting and the safe fallback of starting fresh.

## Debug forensics

When built with `-tags debug`, the binary includes a diagnostic logger controlled by the `DOHPING_DEBUG` environment variable. It writes a file of timestamped events: resize observations, freeze and restart decisions, repaint actions, and physical row counts.

This is the app's own record of what happened during a resize drag. No terminal shows the widths it passes through during a drag, so the log is the only way to reconstruct the episode.

Example:

```text
2026-08-18T15:14:46.276+01:00 [winch] SIGWINCH received → repaint
2026-08-18T15:14:46.276+01:00 [resize] 60→55 rows 11→11 → defer (in-place after settle)
2026-08-18T15:14:46.276+01:00 [redraw] deferred (settle, 299.9ms left)
2026-08-18T15:14:47.276+01:00 [redraw] defer released → in-place repaint
2026-08-18T15:14:47.276+01:00 [redraw] repainted tw=55 phys=11 (was 11) rows=11
```

Release builds compile the debug facility out entirely. CI verifies this on every build.

## Testing

The Python harness `scripts/pty-resize-probe.py` spawns dohping in a real PTY, resizes it mid-run, and renders the capture through a minimal VT emulator to assert the final screen state.

Scenarios:

- window: 60 to 100 columns mid-run. Asserts the block settles to exactly one clean block on screen.
- window-same-band: 60 to 55 columns. Both widths keep the trimmed line on one row. Asserts an in-place repaint with no frozen copy.
- window-subfloor: 50 to 40 columns. A crossing into the below-floor wrap zone (the line fits one row at 50 and wraps at 40). Asserts freeze and restart below.
- plain: a fixed 60-column PTY, below the minimum so every line wraps. Asserts the live line stays anchored.

These run in CI on every push and pull request.

## Summary of design decisions

| Decision | Rationale |
|---|---|
| Reclaim in place above the floor | The reflowed span is computable from the app's own data; the block stays put, no frozen copy. |
| Freeze and restart below the floor | Below 46 columns the fresh frame wraps and its anchor is unknowable. |
| 300 ms settle window | Covers resize drags; the clock restarts on every change. |
| No DSR/CPR | ConPTY lies about cursor position; position independent strategies are more reliable. |
| Physical row math | Logical line counts are wrong when wrapping changes across resizes. |
| No alternate screen | Normal terminal buffer; content above and below the block is preserved. |
| Debug compiled out | Diagnostics never ship in release binaries by construction. |

## Further reading

- `internal/output/display.go` (plain line mode)
- `internal/output/window.go` (window mode)
- `scripts/pty-resize-probe.py` (real-PTY test harness)
- `.github/workflows/ci.yml` (CI gate, including the PTY scenarios)
