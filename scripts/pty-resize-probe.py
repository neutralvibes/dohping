#!/usr/bin/env python3
"""Real-PTY resize proof for dohping window mode.

Spawns dohping --window in a 60x24 pty, resizes the pty to 100x24 mid-run
(SIGWINCH + TIOCSWINSZ), captures the full stream, then renders it through
a minimal VT emulator (autowrap, CR/LF, CUU/CUD, EL) and prints the visible
screen at the end — the real-binary equivalent of the Go termScreen tests.
"""
import fcntl
import os
import pty
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

    def dump(self):
        return "\n".join(f"{r:2}|{self.line(r)}" for r in range(self.rows))


def main():
    cols, rows = 60, 24
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.execv("/tmp/dohping-test",
                 ["dohping", "--window", "-i", "1", "-p", "tcp", "--no-color", "1.1.1.1"])
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
        now = time.time() - t0
        if not resized and now > 3.0:
            # Mid-run resize: 60 → 100 cols. SIGWINCH goes to the pty's
            # foreground process group (dohping).
            set_winsize(fd, rows, 100)
            resized = True
        if now > 7.0:
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
    _, status = os.waitpid(pid, 0)

    text = buf.decode("utf-8", "replace")
    # Render ONLY the final window state: emulate the resize by replaying
    # the stream on a 100-col screen — the last frame's cursor-up math must
    # have laid the block out for 100 cols.
    scr = TermScreen(rows, 100)
    scr.feed(text)
    print("=== visible screen at final width (100 cols) ===")
    print(scr.dump())
    print(f"=== exit status: {os.waitstatus_to_exitcode(status)} ===")
    # Structural sanity on the raw stream.
    print(f"bytes captured: {len(buf)}")
    import re
    upseqs = re.findall(r"\x1b\[(\d+)A", text)
    print(f"cursor-up sequences: {len(upseqs)} (last: {upseqs[-1] if upseqs else 'none'})")
    print(f"shrink-clear sequences (ESC[1B ESC[K): {text.count(chr(27)+'[1B'+chr(27)+'[K')}")


if __name__ == "__main__":
    main()
