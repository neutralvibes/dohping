# Changelog

All notable changes to dohping are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- README note explaining the Windows Defender `Trojan:Win32/Wacatac.C!ml`
  false positive and how to handle it: verify the source, add a Defender
  exclusion when compiling locally, or choose "Allow on device" in Windows
  Security's protection history.

### Changed
- Windows release builds strip the symbol table and debug info (`-s -w`):
  the binary is smaller and the antivirus false positive is less likely.
- ICMP probing now escalates at probe time, not just socket-open time. When
  an unprivileged socket opens but every probe fails with a non-permission
  error, dohping falls through to the system `ping` command in the same
  probe call. The tool now works anywhere `ping` works (including on a
  Raspberry Pi that ships `/bin/ping` with a setuid bit) instead of showing
  a bare error and needing `sudo`.

## [0.1.1] - 2026-08-25

### Added
- ARM support. dohping now ships for 32-bit ARM (armv6 for the original
  Raspberry Pi and Pi Zero, armv7 for newer 32-bit boards) and 64-bit ARM
  (arm64) on every platform that supports them, so it runs on Raspberry Pi
  and other ARM devices.
- Release archives carry the docs with the binary: `README.md`,
  `CHANGELOG.md`, and `LICENSE`.

## [0.1.0] - 2026-08-24

### Added
- Initial release. Single-host ping/monitoring CLI.
- Plain line mode (default) and fixed auto-scrolling window mode.
- ICMP probing with three-tier fallback (raw socket, unprivileged ping
  socket, system ping) and TCP connect probing.
- Up/down/error states with hysteresis and RTT statistics.
- CSV or JSON log output to a file, independent of quiet mode.
- Liveness animation, resize handling for reflowing terminals, predictable
  exit codes, interactive `q` quit.

[0.1.1]: https://github.com/neutralvibes/dohping/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/neutralvibes/dohping/releases/tag/v0.1.0
