package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"sshclient/internal/server"
	ssh "sshclient/ssh"
	"sshclient/storage"
)

//go:embed all:web
var webFS embed.FS

func main() {
	noOpen := flag.Bool("no-open", false, "start the server without opening the window (headless)")
	portFlag := flag.Int("port", 0, "fixed local port to listen on (0 = pick a free one)")
	flag.Parse()

	store, err := storage.NewStore()
	if err != nil {
		log.Printf("warning: cannot load config: %v", err)
		store = &storage.Store{}
	}
	pool := ssh.NewPool(store.KeyMaterial)
	tm := ssh.NewTunnelManager(pool)

	// Bind to a localhost port so nothing conflicts and the service is only
	// reachable from this machine. A fixed port is useful during frontend
	// development (`vite` proxies the RPC socket to it).
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *portFlag))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	// Wrap the listener so we can bind it directly (avoids a race between
	// Listen and http.Serve).
	webSub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}
	if _, err := fs.Stat(webSub, "index.html"); err != nil {
		log.Printf("警告：二进制内没有前端资源，请先运行 `cd frontend && bun run build` 再重新编译")
	}

	srv := server.NewServer(store, pool, tm, webSub)
	handler := srv.Handler()

	httpSrv := &http.Server{Handler: handler}
	go func() {
		// Serve on the already-bound listener.
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	log.Printf("wsh 服务已启动：%s", url)

	shutdown := func() {
		srv.Close()
		_ = httpSrv.Close()
	}

	// Headless mode: serve only, no GUI window (useful for debugging).
	if *noOpen {
		log.Printf("无窗口模式运行，Ctrl+C 退出")
		waitSignal()
		shutdown()
		return
	}

	// GUI mode: Wails v3 hosts the existing web frontend in a native window
	// by loading the local server URL. The frontend and the WebSocket RPC
	// layer are untouched; Wails only provides the window shell (plus the
	// cross-platform webview and desktop chrome: tray, menus, dialogs, ...).
	app := application.New(application.Options{
		Name: "wsh",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Windows/Linux run frameless and draw their own title bar (web/chrome.js)
	// so the whole window matches the app theme; macOS keeps the native frame
	// and the global menu.
	frameless := runtime.GOOS != "darwin"

	mainOpts := application.WebviewWindowOptions{
		Name:               "main",
		Title:              "wsh",
		Frameless:          frameless,
		UseApplicationMenu: !frameless,
		Width:              1200,
		Height:             800,
		MinWidth:           800,
		MinHeight:          600,
		Zoom:               zoomFor(store),
		// The "?win=" param tells the frontend which window it is, so its title
		// bar can drive the right window over RPC.
		URL:              url + "?win=main",
		DevToolsEnabled:  true, // enable the "开发者工具" menu item
		BackgroundColour: appBackground(isDarkTheme(store)),
		Windows:          windowsChrome(isDarkTheme(store)),
	}
	if frameless {
		mainOpts.Windows.DisableMenu = true
	}
	win := app.Window.NewWithOptions(mainOpts)
	applyWindowIcon(win)
	win.Center()

	// The main window is the app: closing it ends the process, even though the
	// tool windows (file transfer, tunnels, options, keys) are windows of their
	// own.
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		app.Quit()
	})

	if !frameless {
		app.Menu.Set(buildMenu(app, srv, win, url, store))
	}

	// Let the web UI's own title bar drive native behaviour. The session manager
	// is a docked panel of the session window, so it needs no action of its own.
	srv.SetAppActions(server.AppActions{
		OpenSettings: func() {
			application.InvokeAsync(func() {
				openToolWindow(app, url, toolWindow{
					Name: "settings", Title: "选项",
					Width: 560, Height: 660, MinWidth: 380, MinHeight: 420,
					Dark: isDarkTheme(store),
					Zoom: zoomFor(store),
				})
			})
		},
		OpenKeys: func() {
			application.InvokeAsync(func() {
				openToolWindow(app, url, toolWindow{
					Name: "keys", Title: "密钥管理",
					Width: 820, Height: 680, MinWidth: 380, MinHeight: 420,
					Dark: isDarkTheme(store),
					Zoom: zoomFor(store),
				})
			})
		},
		OpenSftp: func() {
			application.InvokeAsync(func() {
				openToolWindow(app, url, toolWindow{
					Name: "sftp", Title: "文件传输",
					Width: 900, Height: 620, MinWidth: 560, MinHeight: 400,
					Dark: isDarkTheme(store),
					Zoom: zoomFor(store),
				})
			})
		},
		OpenTunnels: func() {
			application.InvokeAsync(func() {
				openToolWindow(app, url, toolWindow{
					Name: "tunnels", Title: "端口隧道",
					Width: 760, Height: 560, MinWidth: 480, MinHeight: 360,
					Dark: isDarkTheme(store),
					Zoom: zoomFor(store),
				})
			})
		},
		Quit: func() { application.InvokeAsync(func() { app.Quit() }) },
		OpenDevTools: func() {
			application.InvokeAsync(func() { win.OpenDevTools() })
		},
		// SetZoom applies the web UI's zoom to one window. Each window zooms itself
		// when it applies settings, so the zoom follows the user across windows
		// without the backend tracking it per window.
		SetZoom: func(name string, factor float64) {
			application.InvokeAsync(func() {
				// GetByName returns false once a window has been closed, so a stale
				// request can never touch a destroyed window.
				if w, ok := app.Window.GetByName(name); ok {
					w.SetZoom(factor)
				}
			})
		},
		WindowControl: func(name, action string) interface{} {
			var result interface{}
			application.InvokeSync(func() {
				// GetByName returns false once a window has been closed, so a
				// stale request can never touch a destroyed window.
				w, ok := app.Window.GetByName(name)
				if !ok {
					return
				}
				switch action {
				case "minimise":
					w.Minimise()
				case "toggle-maximise":
					w.ToggleMaximise()
				case "close":
					w.Close()
				case "is-maximised":
					result = w.IsMaximised()
				}
			})
			return result
		},
		// SetLatinInput leaves the OS input method in its English mode so a
		// passphrase prompt is typed as ASCII; the IME is per-window state that
		// only the native side can reach.
		SetLatinInput: func(name string) {
			application.InvokeAsync(func() {
				if w, ok := app.Window.GetByName(name); ok {
					switchToLatinInput(w.NativeWindow())
				}
			})
		},
	})

	app.OnShutdown(shutdown)

	if err := app.Run(); err != nil {
		log.Fatalf("wails: %v", err)
	}
}

// buildMenu constructs the native menu bar used on macOS, where a global app
// menu is expected. Items that act on the web UI push a server.UIMsg through the
// server's WebSocket channel (the frontend reacts in RPC.handlePush); the tool
// windows (file transfer, tunnels, options, keys) open in their own dedicated
// windows. The session manager is a docked panel of the session window, so it is
// toggled from the app UI (Ctrl+1) instead of being a window here. Windows and
// Linux use the in-app title bar in frontend/src/components/TitleBar.jsx.
func buildMenu(app *application.App, srv *server.Server, win application.Window, baseURL string, store *storage.Store) *application.Menu {
	menu := app.NewMenu()

	// Standard application menu (About / Quit live there).
	menu.AddRole(application.AppMenu)

	fileMenu := menu.AddSubmenu("文件")
	fileMenu.Add("新建连接…").
		SetAccelerator("CmdOrCtrl+N").
		OnClick(func(ctx *application.Context) {
			srv.NotifyAll(server.UIMsg{Type: server.UINewConnection})
		})
	fileMenu.AddSeparator()
	// Quit lives in the application menu role on macOS.

	optionsMenu := menu.AddSubmenu("选项")
	optionsMenu.Add("选项…").
		SetAccelerator("CmdOrCtrl+Shift+,").
		OnClick(func(ctx *application.Context) {
			openToolWindow(app, baseURL, toolWindow{
				Name: "settings", Title: "选项",
				Width: 560, Height: 660, MinWidth: 380, MinHeight: 420,
				Dark: isDarkTheme(store),
			})
		})
	optionsMenu.Add("密钥管理…").
		SetAccelerator("CmdOrCtrl+Shift+K").
		OnClick(func(ctx *application.Context) {
			openToolWindow(app, baseURL, toolWindow{
				Name: "keys", Title: "密钥管理",
				Width: 820, Height: 680, MinWidth: 380, MinHeight: 420,
				Dark: isDarkTheme(store),
			})
		})

	viewMenu := menu.AddSubmenu("视图")
	viewMenu.Add("文件传输").
		SetAccelerator("CmdOrCtrl+2").
		OnClick(func(ctx *application.Context) {
			openToolWindow(app, baseURL, toolWindow{
				Name: "sftp", Title: "文件传输",
				Width: 900, Height: 620, MinWidth: 560, MinHeight: 400,
				Dark: isDarkTheme(store),
			})
		})
	viewMenu.Add("端口隧道").
		SetAccelerator("CmdOrCtrl+3").
		OnClick(func(ctx *application.Context) {
			openToolWindow(app, baseURL, toolWindow{
				Name: "tunnels", Title: "端口隧道",
				Width: 760, Height: 560, MinWidth: 480, MinHeight: 360,
				Dark: isDarkTheme(store),
			})
		})

	helpMenu := menu.AddSubmenu("帮助")
	helpMenu.Add("关于 wsh").OnClick(func(ctx *application.Context) {
		app.Dialog.Info().
			SetTitle("关于 wsh").
			SetMessage("wsh\n\n基于 Go + Wails v3 的跨平台 SSH 客户端。\n终端渲染：xterm.js").
			Show()
	})
	helpMenu.Add("开发者工具").OnClick(func(ctx *application.Context) {
		win.OpenDevTools()
	})

	return menu
}

// toolWindow describes one of the app's auxiliary windows (file transfer,
// tunnels, options, key manager). Each is its own native window so it can be
// opened, closed and rearranged independently of the terminal sessions, which is
// how Xshell keeps its transfer UI out of the session window. The session
// manager is not one of these: it is docked inside the session window.
type toolWindow struct {
	Name      string
	Title     string
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
	Dark      bool
	// Zoom is the saved UI zoom factor, applied at creation so a new window opens
	// at the user's zoom instead of flashing at 100% first.
	Zoom float64
}

// openToolWindow opens (or focuses) an auxiliary window. The web frontend picks
// the page to render from the "?win=" query parameter, so no route/hash is
// needed and a window keeps its identity even if the hash changes.
func openToolWindow(app *application.App, baseURL string, tw toolWindow) {
	if existing, ok := app.Window.GetByName(tw.Name); ok {
		existing.Show()
		existing.Focus()
		return
	}
	// Tool windows are frameless too, so they match the app theme.
	frameless := runtime.GOOS != "darwin"
	opts := application.WebviewWindowOptions{
		Name:      tw.Name,
		Title:     tw.Title,
		Frameless: frameless,
		URL:       baseURL + "/?win=" + tw.Name,
		Width:     tw.Width,
		Height:    tw.Height,
		MinWidth:  tw.MinWidth,
		MinHeight: tw.MinHeight,
		Zoom:      tw.Zoom,
		// Tool windows keep the chrome minimal: no menu bar, so the per-window
		// menu is not duplicated on Windows/Linux.
		UseApplicationMenu: false,
		BackgroundColour:   appBackground(tw.Dark),
		Windows:            windowsChrome(tw.Dark),
	}
	if frameless {
		opts.Windows.DisableMenu = true
	}
	w := app.Window.NewWithOptions(opts)
	applyWindowIcon(w)
	w.Center()
	w.Focus()
}

// isDarkTheme reports whether the saved app theme is dark (the default).
func isDarkTheme(store *storage.Store) bool {
	return store.Settings().Theme != "light"
}

// zoomFor maps the saved UI size to the webview zoom factor used when a window is
// created (uiBasePx is 100%). Values outside the supported range fall back to
// 100%, matching what the options window accepts.
func zoomFor(store *storage.Store) float64 {
	const (
		uiBasePx = 13
		uiMaxPx  = 32
	)
	px := store.Settings().FontSize
	if px < uiBasePx || px > uiMaxPx {
		px = uiBasePx
	}
	return float64(px) / uiBasePx
}

// appBackground is the app's background colour for the current theme, applied
// to every window so there is no white (or dark) flash before the webview
// paints the themed UI.
func appBackground(dark bool) application.RGBA {
	if dark {
		return application.NewRGBA(0x1b, 0x1b, 0x22, 0xff) // --bg #1b1b22
	}
	return application.NewRGBA(0xf5, 0xf5, 0xf8, 0xff) // --bg #f5f5f8
}

// windowsChrome themes the native window frame so it matches the app's UI
// instead of looking like a stock OS window. Only the Windows backend reads
// these options; other platforms ignore them.
func windowsChrome(dark bool) application.WindowsWindow {
	mode := application.Light
	var custom application.ThemeSettings
	if dark {
		mode = application.Dark
		custom = application.ThemeSettings{
			DarkModeActive: &application.WindowTheme{
				TitleBarColour:  bgr(0x23, 0x23, 0x2d), // --sidebar #23232d
				TitleTextColour: bgr(0xe7, 0xe7, 0xf0), // --text    #e7e7f0
				BorderColour:    bgr(0x34, 0x34, 0x3f), // --border  #34343f
			},
			DarkModeInactive: &application.WindowTheme{
				TitleBarColour:  bgr(0x1b, 0x1b, 0x22), // --bg  #1b1b22
				TitleTextColour: bgr(0x9a, 0x9a, 0xa8), // --dim #9a9aa8
				BorderColour:    bgr(0x34, 0x34, 0x3f),
			},
		}
	}
	return application.WindowsWindow{Theme: mode, CustomTheme: custom}
}

// bgr packs an #RRGGBB colour into the 0x00BBGGRR form that Wails/Windows
// expects for native title-bar theming.
func bgr(r, g, b uint32) *uint32 {
	v := r | g<<8 | b<<16
	return &v
}

// waitSignal blocks until the process receives a termination signal.
func waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}
