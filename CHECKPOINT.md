# dohping build checkpoint
updated: 2026-08-17T03:30+01:00
phase: 6
phase_status: complete
completed: phase1, phase2, phase3, phase4, phase5, phase6
next_action: none — build complete (user acceptance fixes shipped)
notes: User acceptance fixes (DECISIONS #50-52): (1) consecutive errors no longer re-enter error state — one line, duration updates (EventProbeError); (2) permission-class probe errors abort with exit 3 + hint (incl. ping "Operation not permitted" wrapped as EPERM); (3) MIN/MAX/AVG/FAILS left-aligned under headers (up line now byte-identical to spec §7.4). Window-mode corrections (2026-08-17): #53 window renders IN PLACE on the normal terminal — fixed-size block (header + last N events + live line), cursor-up + per-row clear-to-EOL only; NO alternate screen, NO cursor-home, NO clear-to-end-of-screen; Enter/Exit no-ops; block stays visible after exit. #54 explicit column reset: redraw emits `\x1b[<n>A\r` and rows are `\r\n`-separated (CUU preserves column; bare LF doesn't reset it — user's terminal showed fragmented rows). Regression test renders the stream through a mini terminal emulator and asserts the visible screen grid. All gates re-verified: 7/7 race-clean, vet/gofmt clean. dist/ rebuilt (linux-amd64 sha 999026ce…).
