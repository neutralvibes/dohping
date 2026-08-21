#!/usr/bin/env python3
"""Record a dohping terminal session as an asciinema v2 cast file.

Used to produce the README hero demo GIF (rendered with `agg`). The
recording is a real PTY run of the built binary — timestamps come from
the wall clock, so the resulting GIF shows genuine live behaviour.

Usage:
  python3 scripts/record-demo-cast.py BINARY CASTFILE [recorder opts] [-- binary args...]

Example (the README hero):
  python3 scripts/record-demo-cast.py /tmp/dohping-demo /tmp/demo.cast \
      --send-keys-after 8 q -- --window 1.1.1.1

Everything after the first bare `--` is passed to the binary as-is.
"""
import argparse
import fcntl
import json
import os
import pty
import select
import struct
import sys
import termios
import time


def set_size(fd, cols, rows):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def main():
    ap = argparse.ArgumentParser(add_help=True)
    ap.add_argument("binary")
    ap.add_argument("castfile")
    ap.add_argument("--cols", type=int, default=80)
    ap.add_argument("--rows", type=int, default=24)
    ap.add_argument("--send-keys-after", nargs="+", action="append", default=[],
                    metavar=("SEC", "KEYS"),
                    help="inject KEYS at SEC wall-clock offset (repeatable)")
    args, rest = ap.parse_known_args()
    if rest and rest[0] == "--":
        rest = rest[1:]
    bin_args = rest

    key_events = []
    for pair in args.send_keys_after:
        if len(pair) != 2:
            sys.exit("--send-keys-after needs SEC KEYS")
        key_events.append((float(pair[0]), pair[1]))
    key_events.sort()

    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.execvp(args.binary, [args.binary] + bin_args)
        os._exit(127)

    set_size(fd, args.cols, args.rows)

    start = time.monotonic()
    events = []
    ki = 0
    status = None

    while True:
        now = time.monotonic() - start
        while ki < len(key_events) and now >= key_events[ki][0]:
            os.write(fd, key_events[ki][1].encode())
            ki += 1

        r, _, _ = select.select([fd], [], [], 0.05)
        if r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                data = b""
            if data:
                events.append((round(time.monotonic() - start, 6), "o",
                               data.decode(errors="replace")))
        wpid, status = os.waitpid(pid, os.WNOHANG)
        if wpid == pid:
            # Drain anything still buffered.
            while True:
                r, _, _ = select.select([fd], [], [], 0.05)
                if not r:
                    break
                try:
                    data = os.read(fd, 65536)
                except OSError:
                    break
                if not data:
                    break
                events.append((round(time.monotonic() - start, 6), "o",
                               data.decode(errors="replace")))
            break
        if status is None:
            time.sleep(0.01)

    header = {
        "version": 2,
        "width": args.cols,
        "height": args.rows,
        "timestamp": int(time.time()),
        "env": {"TERM": "xterm-256color", "SHELL": "/bin/sh"},
    }
    with open(args.castfile, "w", encoding="utf-8") as f:
        f.write(json.dumps(header) + "\n")
        for t, typ, data in events:
            f.write(json.dumps([t, typ, data]) + "\n")

    exit_code = os.waitstatus_to_exitcode(status) if status is not None else "?"
    print(f"wrote {args.castfile}: {len(events)} events, "
          f"{round(events[-1][0], 1) if events else 0}s, exit={exit_code}")


if __name__ == "__main__":
    main()
