# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Bilingual UI (English / 中文) with a language selector in Options; the
  `auto` setting follows the system locale.
- Bilingual documentation: `README.md` (English) and `README.zh-CN.md` (Chinese).

## [1.0.0] - 2026-09-22

### Added

- Session manager (dockable, folder tree with drag & drop) and multi-tab
  xterm.js terminals
- Quick connect bar supporting `ssh://user@host:port` style addresses
- Connection management with test, edit and delete; key-based login preferred
  over password with automatic fallback
- Key manager: submit private keys (stored encrypted), show public key and
  fingerprint, optional remembered passphrases (AES-GCM encrypted at rest)
- Passwords encrypted with a machine-local key before being persisted
- SFTP file transfer window (dual-pane browser, upload/download)
- Local/remote port forwarding manager with start/stop
- Frameless self-drawn title bar and menus on Windows/Linux; system chrome on
  macOS
- Windows (.exe via mingw cross-compile), macOS (.app / .dmg) and Linux builds

[Unreleased]: https://github.com/giter/wsh/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/giter/wsh/releases/tag/v1.0.0
