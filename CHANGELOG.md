# Changelog

All notable changes to dohping are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Release archives now include `README.md`, `CHANGELOG.md`, and `LICENSE`
  alongside the binary.

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

[Unreleased]: https://github.com/neutralvibes/dohping/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/neutralvibes/dohping/releases/tag/v0.1.0
