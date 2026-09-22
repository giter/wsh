<h1 align="center">wsh</h1>

<p align="center">
  A cross-platform SSH client built with Go + Web.<br/>
  English | <a href="README.zh-CN.md">中文</a>
</p>

> [!NOTE]
> Screenshots welcome — the UI is an Xshell-style desktop app: a docked session
> manager on the left, multi-tab terminal sessions, and separate windows for
> file transfer, tunnels, keys and options.

## Features

- **Session manager** — docked side panel with a folder/connection tree and a
  properties pane; connections can be dragged into/out of folders and reordered
- **Multi-tab terminals** — xterm.js rendering with automatic PTY resize; tabs
  are renameable and quick-connect sessions can be saved as connections
- **Quick connect** — type `ssh://user@host:port` in the address bar and hit
  Enter for a temporary session, or save it as a connection
- **Key management** — import private keys (stored encrypted), copy the public
  key / fingerprint, prefer key auth with password fallback; passphrases stay
  in memory unless you opt in
- **Encrypted storage** — passwords are AES-GCM encrypted with a machine-local
  key before being persisted
- **SFTP file transfer** — dual-pane local/remote browser with upload/download
- **Port tunnels** — local/remote forwarding rules with start/stop
- **Multi-window** — file transfer, tunnels, options and key manager each run
  in their own native window
- **Frameless UI** — self-drawn title bar and menus on Windows/Linux; system
  chrome on macOS

## Architecture

```
Wails v3 native window (cross-platform WebView container)
  └─ Go backend (SSH / SFTP / tunnel / storage logic)
       └─ local HTTP + WebSocket service (bound to 127.0.0.1 only)
            └─ React frontend (go:embed'ed into the binary, runs offline)
                 ├─ session window (docked manager + quick connect + tabs)
                 └─ file transfer / tunnels / options / keys windows
```

All windows share one backend; cross-window actions are broadcast over RPC
(e.g. `session.open`). The frontend is React + Vite (sources in `frontend/`,
build output in `web/`, embedded with `go:embed` — no Node needed at runtime);
Wails only provides the window shell.

> `web/` is a build artifact and **not committed**. Build the frontend before
> `go build`, otherwise the window shows a "frontend not built" notice.
> `./run.sh` and `./build.sh` do this automatically.

## Building

Requires **Go 1.27+**, [bun](https://bun.sh) and a platform WebView runtime
(not pure Go — CGO is involved):

- **Windows** — WebView2 Runtime is built into Win10/11; cross-compiling needs mingw-w64
- **Linux** — `libwebkit2gtk-4.1-dev`, `libgtk-3-dev`, `build-essential`, `pkg-config`
- **macOS** — Xcode Command Line Tools (WKWebView is built in)

```bash
# Linux (Debian/Ubuntu dependencies)
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config

# Build frontend + backend and launch (native window)
./run.sh

# Manual: frontend first, then the backend
cd frontend && bun install && bun run build && cd ..
go build -o wsh .
```

### Frontend development (HMR)

```bash
./dev.sh        # backend on :17777 + Vite dev server on :5173
```

Open <http://localhost:5173> — frontend edits hot-reload without rebuilding
Go. Vite proxies `/ws` to the backend.

### Windows (.exe)

```bash
sudo apt install gcc-mingw-w64-x86-64   # cross toolchain (on Linux)
./build.sh                              # frontend + wsh.exe
```

### macOS (.app / .dmg)

macOS builds **must run on macOS** (CGO links Cocoa/WebKit; no
cross-compiling):

```bash
xcode-select --install       # first time only
./build.sh darwin            # wsh.app
./build.sh dmg               # + wsh.dmg distributable image
```

## Usage

On start the app serves a local API on `127.0.0.1` (random port by default,
`-port` to pin, `-no-open` to skip opening a window) and loads the UI in a
native Wails window. Configuration lives in the system config directory under
`wsh/config.json` (passwords and keys stored encrypted); the machine key is
`secret.key` next to it.

Menu bar (self-drawn inside the title bar):

- **File** — new connection (`Ctrl/Cmd+N`), quit (`Ctrl/Cmd+Q`)
- **Options** — options (`Ctrl/Cmd+Shift+,`), key manager (`Ctrl/Cmd+Shift+K`)
- **View** — zoom in/out/reset, session manager (`Ctrl/Cmd+1`), file transfer
  (`Ctrl/Cmd+2`), port tunnels (`Ctrl/Cmd+3`)
- **Help** — about, developer tools

Tips:

- Single-click a connection to select it, double-click to open a terminal tab;
  the search box filters by name/host/user
- Quick connect accepts `ssh://user@host:port`, `user@host:port`, `host:port`,
  `user@host` and bare `host`
- Failed logins (no password / wrong password) prompt for a password and retry

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for the
development setup and pull request process. UI copy lives in the i18n
dictionaries under `frontend/src/lib/i18n/`.

## Security

Please report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
