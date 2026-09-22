# Contributing to wsh

Thank you for considering a contribution! This document explains how to set up
a development environment and submit changes.

[English](CONTRIBUTING.md) | [中文](docs/CONTRIBUTING.zh-CN.md)

## Development setup

Requirements: **Go 1.27+**, [bun](https://bun.sh) (frontend builds), and the
platform WebView runtime:

- **Windows** — WebView2 Runtime is built into Win10/11; cross-compiling needs mingw-w64
- **Linux (Debian/Ubuntu)** — `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config`
- **macOS** — Xcode Command Line Tools

```bash
# Frontend + backend with hot reload (Vite on :5173, backend on :17777)
./dev.sh

# Full build (frontend + native binary)
./run.sh
```

See [README.md](README.md) for the full architecture and build matrix.

## How to submit changes

1. Fork the repository and create a topic branch from `main`
   (`feat/my-feature`, `fix/my-bugfix`).
2. Make your change, keeping the style of the surrounding code.
3. Add or update tests when the change affects behavior.
4. Run the test suite and the frontend build before pushing:

   ```bash
   go test ./...
   cd frontend && bun install && bun run build
   ```

5. Open a pull request using the provided template and link any related issues.

## Guidelines

- Keep pull requests focused; one feature or fix per PR.
- UI copy must go through the i18n dictionaries (`frontend/src/lib/i18n/*.js`);
  do not hardcode user-visible strings in components.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
  (e.g. `fix(sftp): handle empty remote directories`).
- Backend Go code is formatted with `gofmt`.

## Reporting issues

Use the issue templates. For security vulnerabilities, please follow
[SECURITY.md](SECURITY.md) instead of opening a public issue.

## License

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE).
