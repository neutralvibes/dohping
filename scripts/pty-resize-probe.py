#!/usr/bin/env python3
"""Real-PTY proof harness for dohping's terminal-output rounds.

Scenarios (argv[1]):
  window (default): spawns dohping --window in a 60x24 pty, resizes the pty
    to 100x24 mid-run (SIGWINCH + TIOCSWINSZ), renders the capture through
    a VT emulator and prints the visible screen — proves the block repaints
    IN PLACE across the resize: with column trimming the line never wraps
    at these widths, so the reflow cannot move it and exactly ONE block
    remains on screen (no frozen duplicate).
  window-subfloor: same setup but 50x24 → 40x24 mid-run — a width change
    INTO the below-floor wrap zone (the line fits one row at 50, wraps at
    40): the fresh frame itself wraps at the settled width, so the block
    FREEZES and restarts on a fresh row below the frozen rendering — the
    reclaim's below-floor fallback (two blocks).
  window-same-band: 60 → 55 mid-run — both widths keep the trimmed line in
    one row; repaints in place, one block.
  plain: spawns PLAIN live mode in a fixed 60x24 pty (below the 81-cell
    minimum, so every line wraps) and asserts the live line stays anchored
    across many redraws — earlier code walked it down one row per redraw,
    leaving stale fragments.

Run against a FRESH build (rm -f the binary first — stale-build trap):
  python3 scripts/pty-resize-probe.py [window|plain]
"""
import fcntl
import os
import pty
import re
import select
import signal
import struct
import sys
import termios
import time


def set_winsize(fd, rows, cols):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


class TermScreen:
    def __init__(self, rows, cols):
        self.rows, self.cols = rows, cols
        self.cells = [[None] * cols for _ in range(rows)]
        self.r = self.c = 0

    def feed(self, s):
        i = 0
        while i < len(s):
            ch = s[i]
            if ch == "\x1b":
                if i + 1 < len(s) and s[i + 1] == "[":
                    j = i + 2
                    while j < len(s) and not ("\x40" <= s[j] <= "\x7e"):
                        j += 1
                    if j >= len(s):
                        break
                    final = s[j]
                    n = 1
                    params = s[i + 2:j].split(";")
                    if params and params[0].isdigit() and int(params[0]) > 0:
                        n = int(params[0])
                    if final == "A":
                        self.r = max(0, self.r - n)
                    elif final == "B":
                        self.r = min(self.rows - 1, self.r + n)
                    elif final == "K":
                        for cc in range(self.c, self.cols):
                            self.cells[self.r][cc] = None
                    elif final == "J":
                        for rr in range(self.r, self.rows):
                            for cc in range(self.cols):
                                self.cells[rr][cc] = None
                    i = j + 1
                    continue
                i += 1
                continue
            if ch == "\r":
                self.c = 0
            elif ch == "\n":
                self.r = min(self.rows - 1, self.r + 1)
            elif ord(ch) < 0x20:
                pass
            else:
                if self.c >= self.cols:  # DECAWM autowrap
                    self.r = min(self.rows - 1, self.r + 1)
                    self.c = 0
                if 0 <= self.r < self.rows and 0 <= self.c < self.cols:
                    self.cells[self.r][self.c] = ch
                self.c += 1
            i += 1

    def line(self, r):
        cells = self.cells[r]
        end = len(cells)
        while end > 0 and cells[end - 1] is None:
            end -= 1
        return "".join(cells[:end])

    def resize(self, cols):
        """Non-reflowing width change (like the pty itself): existing cells
        keep their positions; only new writes use the new width."""
        if cols > self.cols:
            for i in range(len(self.cells)):
                self.cells[i] = self.cells[i] + [None] * (cols - self.cols)
        elif cols < self.cols:
            for i in range(len(self.cells)):
                self.cells[i] = self.cells[i][:cols]
        self.cols = cols

    def dump(self):
        return "\n".join(f"{r:2}|{self.line(r)}" for r in range(self.rows))


def capture(args, cols, rows, scr, resize_to=None, duration=7.0):
    """Run dohping in a pty of the given size; optionally resize mid-run.
    scr is a TermScreen fed incrementally and renders the final state.
    Returns (raw_bytes, exit_code)."""
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.execv("/tmp/dohping-test", ["dohping"] + args)
        os._exit(127)

    set_winsize(fd, rows, cols)
    buf = b""
    t0 = time.time()
    resized = False
    while True:
        r, _, _ = select.select([fd], [], [], 0.2)
        if r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                break
            if not data:
                break
            buf += data
            scr.feed(data.decode("utf-8", "replace"))
        now = time.time() - t0
        if not resized and resize_to is not None and now > 3.0:
            # Mid-run resize. SIGWINCH goes to the pty's foreground
            # process group (dohping); the emulator follows (non-reflowing,
            # like the pty itself).
            set_winsize(fd, rows, resize_to)
            scr.resize(resize_to)
            resized = True
        if now > duration:
            break

    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    # Drain briefly so the exit summary arrives.
    end = time.time() + 1.5
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.2)
        if r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                break
            if not data:
                break
            buf += data
            scr.feed(data.decode("utf-8", "replace"))
    _, status = os.waitpid(pid, 0)
    return buf, os.waitstatus_to_exitcode(status)


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "window"
    rows = 24
    common = ["-i", "1", "-p", "tcp", "--no-color", "1.1.1.1"]

    if mode == "window-same-band":
        # 60 → 55 mid-run: same wrap band (12 physical rows at both
        # widths), so the block must repaint in place — exactly ONE header
        # on the final screen, no frozen duplicate.
        scr = TermScreen(rows, 60)
        buf, code = capture(["--window"] + common, 60, rows, scr, resize_to=55)
        print("=== visible screen at final width (55 cols) ===")
        print(scr.dump())
        print(f"=== exit status: {code} ===")
        headers = [r for r in range(rows) if scr.line(r).startswith("TIME")]
        live = [r for r in range(rows) if re.match(r"^\d{2}:\d{2}:\d{2}", scr.line(r))]
        ok = len(headers) == 1 and len(live) == 1 and live[0] > headers[0]
        text = buf.decode("utf-8", "replace")
        # A standalone restart CRLF at a frame start would begin "\r\n"
        # right after the block's own row CRLFs; count them loosely and
        # require zero restart markers in the windowed frame stream: the
        # screen assertion above is the strong one (one header only).
        print(f"header rows: {headers} (want exactly 1 — no frozen duplicate)")
        print(f"timestamp rows: {live}")
        print(f"restart CRLF sequences: {text.count(chr(13)+chr(10))}")
        print("RESULT: " + ("PASS — same-band resize repainted in place" if ok else "FAIL — block froze/duplicated"))
        return

    if mode == "plain":
        # Fixed 60-col pty (below the 81-cell minimum → every line wraps).
        # Many redraws happen over the run; the live line must never drift.
        scr = TermScreen(rows, 60)
        buf, code = capture(["--no-window"] + common, 60, rows, scr, duration=6.0)
        print("=== visible screen (plain live, fixed 60 cols) ===")
        print(scr.dump())
        ts = re.compile(r"^\d{2}:\d{2}:\d{2}")
        anchors = [r for r in range(rows) if ts.match(scr.line(r))]
        ok = len(anchors) == 1 and anchors[0] == 2
        print(f"=== exit status: {code} ===")
        print(f"timestamp rows: {anchors} (want exactly [2] — header wraps to rows 0-1)")
        print("RESULT: " + ("PASS — live line anchored" if ok else "FAIL — drifted/fragmented"))
        return

    if mode == "window-subfloor":
        # 50 → 40 mid-run. The line fits one row at 50; at 40 it wraps
        # (below the essentials floor) — the fresh frame itself wraps at
        # the settled width, so the block FREEZES then restarts below the
        # frozen rendering: the reclaim's below-floor fallback
        # (two blocks on screen).
        scr = TermScreen(rows, 50)
        buf, code = capture(["--window"] + common, 50, rows, scr, resize_to=40)
        print("=== visible screen at final width (40 cols) ===")
        print(scr.dump())
        print(f"=== exit status: {code} ===")
        headers = [r for r in range(rows) if scr.line(r).startswith("TIME")]
        fresh = [r for r in range(rows) if re.match(r"^\d{2}:\d{2}:\d{2}", scr.line(r))]
        ok = len(headers) == 2 and headers[1] > headers[0] and any(r > headers[1] for r in fresh)
        print(f"header rows: {headers} (want exactly 2: frozen + fresh below)")
        print(f"timestamp rows: {fresh}")
        print("RESULT: " + ("PASS — sub-floor crossing froze then restarted below" if ok else "FAIL — block not cleanly restarted"))
        return

    # window mode: 60 → 100 mid-run. The line is trimmed to fit at 60
    # and never wraps at either width, so the reflow cannot
    # move the block: it must repaint IN PLACE — exactly ONE block on the
    # final screen (no frozen duplicate).
    scr = TermScreen(rows, 60)
    buf, code = capture(["--window"] + common, 60, rows, scr, resize_to=100)
    print("=== visible screen at final width (100 cols) ===")
    print(scr.dump())
    print(f"=== exit status: {code} ===")
    # Structural sanity on the visible screen: exactly one block header
    # and a live line below it.
    headers = [r for r in range(rows) if scr.line(r).startswith("TIME")]
    fresh = [r for r in range(rows) if re.match(r"^\d{2}:\d{2}:\d{2}", scr.line(r))]
    ok = len(headers) == 1 and len(fresh) == 1 and fresh[0] > headers[0]
    print(f"header rows: {headers} (want exactly 1 — repainted in place)")
    print(f"timestamp rows: {fresh}")
    print("RESULT: " + ("PASS — block repainted in place across the resize" if ok else "FAIL — block froze/duplicated"))
    # Structural sanity on the raw stream.
    text = buf.decode("utf-8", "replace")
    print(f"bytes captured: {len(buf)}")
    upseqs = re.findall(r"\x1b\[(\d+)A", text)
    print(f"cursor-up sequences: {len(upseqs)} (last: {upseqs[-1] if upseqs else 'none'})")
    print(f"shrink-clear sequences (ESC[1B CR ESC[K): {text.count(chr(27)+'[1B'+chr(13)+chr(27)+'[K')}")
    print(f"restart CRLF sequences: {text.count(chr(13)+chr(10))}")


if __name__ == "__main__":
    main()
