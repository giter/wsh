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
- **快速连接** — type `ssh://user@host:port` in the address bar and hit
  Enter for a temporary session, or save it as a connection
- **Jump-host chains** — a connection can route through multiple bastion hosts
  (`local → jump A → jump B → target`); loops or missing hops fail before dialing
- **Resource gauges** — while a session is open, CPU (load), memory and disk are
  sampled on the target host over a dedicated SSH channel and shown as a mini
  dashboard in the manager pane (never blocks the interactive PTY)
- **AI reasoning pane** (right side) — third pane of the three-column layout:
  streaming chain-of-thought cards, error root-cause analysis (RCA), a
  **shadow-run** confirmation panel for yellow-zone commands, and hints for
  blocked ones. Collapsible; toggled with `Ctrl/Cmd+Shift+A`. Long sessions
  auto-collapse old cards to one-line summaries (hover highlights the related
  output in the terminal; failed generations can be retried)
- **Smart Input** (below the terminal) — one box, two tracks: type shell
  commands directly, or describe an intent in natural language
  ("find what's using port 8080" → a `lsof -i :8080` preview). Input is
  safety-classified live and highlighted green/yellow/red; red-zone commands
  disable Enter. `Ctrl+Enter` generates and runs; `Esc` is a universal stop
- **Local safety engine** — AST-based (`mvdan.cc/sh/v3`), not regex: recursive
  root deletes, `dd of=/dev/sda`, `mkfs`, fork bombs are hard-blocked (red);
  firewall flushes, service stops, bulk deletes and privilege escalation
  (`sudo`/`doas`/`su`) require confirmation (yellow); read-only commands run
  directly (green). Approved commands can be allowlisted permanently.
  Judgement is fully local and instant — no LLM involved
- **Sanitization gateway** — terminal context sent to the AI is masked locally
  first: IPv4/IPv6 → `[IP_MASKED]`; passwords / tokens / API keys / JWT /
  private-key blocks → `[SENSITIVE_DATA]`
- **AI inference (optional)** — OpenAI-compatible APIs or local Ollama (fully
  offline); the API key is stored encrypted. With nothing configured every AI
  feature stays off and the terminal is unaffected; model-proposed commands are
  always re-checked by the local engine before execution
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
       ├─ internal/safety   local AST safety engine (mvdan.cc/sh/v3)
       ├─ internal/sanitize local sanitization gateway before AI upload
       ├─ internal/ai       OpenAI-compatible / Ollama streaming client
       └─ local HTTP + WebSocket service (bound to 127.0.0.1 only)
            └─ React frontend (go:embed'ed into the binary, runs offline)
                 ├─ session window
                 │    ├─ left: docked manager (tree + properties + gauges)
                 │    ├─ center: xterm.js terminal + Smart Input bar
                 │    └─ right: AI reasoning pane (CoT / RCA / shadow-run)
                 └─ file transfer / tunnels / options / keys windows
```

All windows share one backend; cross-window actions are broadcast over RPC
(e.g. `session.open`). The frontend is React + Vite (sources in `frontend/`,
build output in `web/`, embedded with `go:embed` — no Node needed at runtime);
Wails only provides the window shell.

Key frontend sources:

```
frontend/src/
  lib/rpc.js               WebSocket RPC client (requests + push subscriptions)
  lib/intent.js            dual-track input classifier (shell vs natural language)
  state/store.jsx          global state (settings, connections, tabs, panes,
                           reason cards, host sampling)
  components/              pages and components (session tree, terminal,
                           SmartInput.jsx, ReasonPane.jsx, SFTP, tunnels, keys…)
```

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
  (`Ctrl/Cmd+2`), port tunnels (`Ctrl/Cmd+3`), AI reason pane
  show/hide (`Ctrl/Cmd+Shift+A`)
- **Help** — about, developer tools

Tips:

- Single-click a connection to select it, double-click to open a terminal tab;
  the search box filters by name/host/user
- Quick connect accepts `ssh://user@host:port`, `user@host:port`, `host:port`,
  `user@host` and bare `host`
- Failed logins (no password / wrong password) prompt for a password and retry

## AI settings (optional)

Configured under **Options → AI inference**; everything works without it
(terminal, safety engine, jump hosts and gauges don't depend on it):

| Provider                | Base URL example                                                      | Model example |
| ----------------------- | --------------------------------------------------------------------- | ------------- |
| OpenAI-compatible       | `https://api.openai.com/v1` (or self-hosted one-api / vLLM / LocalAI) | `gpt-4o-mini` |
| Ollama (local, offline) | `http://127.0.0.1:11434`                                              | `qwen2.5:7b`  |

- The API key is AES-GCM encrypted like passwords and never echoed back; leave
  it empty for local Ollama
- **Auto root-cause analysis** — when terminal output shows `FATAL`,
  `Segmentation fault`, `Permission denied`, OOM, etc., an RCA request is made
  automatically
- **Don't send terminal context** — requests contain only your question
- Uploads are always sanitized (IP / passwords / tokens / JWT / key blocks),
  and model-proposed commands are re-classified by the local engine — the
  model's own risk assessment is never trusted

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for the
development setup and pull request process. UI copy lives in the i18n
dictionaries under `frontend/src/lib/i18n/`.

## Security

Please report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
