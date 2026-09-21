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

	"sshclient/internal/server"
	ssh "sshclient/ssh"
	"sshclient/storage"
)

//go:embed all:web
var webFS embed.FS

func main() {
	noOpen := flag.Bool("no-open", false, "start the server without opening the window (headless)")
	flag.Parse()

	store, err := storage.NewStore()
	if err != nil {
		log.Printf("warning: cannot load config: %v", err)
		store = &storage.Store{}
	}
	pool := ssh.NewPool(store.KeyMaterial)
	tm := ssh.NewTunnelManager(pool)

	// Bind to an ephemeral localhost port so nothing conflicts and the
	// service is only reachable from this machine.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
	win.Center()

	if !frameless {
		app.Menu.Set(buildMenu(app, srv, win, url, store))
	}

	// Let the web UI's own title bar drive native behaviour.
	srv.SetAppActions(server.AppActions{
		OpenSettings: func() {
			application.InvokeAsync(func() {
				openConfigWindow(app, url, "settings", "settings", "选项", 560, 660, isDarkTheme(store))
			})
		},
		OpenKeys: func() {
			application.InvokeAsync(func() {
				openConfigWindow(app, url, "keys", "keys", "密钥管理", 820, 680, isDarkTheme(store))
			})
		},
		Quit: func() { application.InvokeAsync(func() { app.Quit() }) },
		OpenDevTools: func() {
			application.InvokeAsync(func() { win.OpenDevTools() })
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
	})

	app.OnShutdown(shutdown)

	if err := app.Run(); err != nil {
		log.Fatalf("wails: %v", err)
	}
}

// buildMenu constructs the native menu bar used on macOS, where a global app
// menu is expected. Items that act on the web UI push a server.UIMsg through the
// server's WebSocket channel (the frontend reacts in RPC.handlePush);
// configuration features open in their own dedicated windows. Windows and Linux
// use the in-app menu bar in web/menu.js instead.
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
	fileMenu.Add("连接管理…").
		SetAccelerator("CmdOrCtrl+M").
		OnClick(func(ctx *application.Context) {
			srv.NotifyAll(server.UIMsg{Type: server.UINavigate, Page: "connections"})
		})
	fileMenu.AddSeparator()
	// Quit lives in the application menu role on macOS.

	optionsMenu := menu.AddSubmenu("选项")
	optionsMenu.Add("选项…").
		SetAccelerator("CmdOrCtrl+Shift+,").
		OnClick(func(ctx *application.Context) {
			openConfigWindow(app, baseURL, "settings", "settings", "选项", 560, 660, isDarkTheme(store))
		})
	optionsMenu.Add("密钥管理…").
		SetAccelerator("CmdOrCtrl+Shift+K").
		OnClick(func(ctx *application.Context) {
			openConfigWindow(app, baseURL, "keys", "keys", "密钥管理", 820, 680, isDarkTheme(store))
		})

	viewMenu := menu.AddSubmenu("视图")
	viewMenu.Add("连接").
		SetAccelerator("CmdOrCtrl+1").
		OnClick(func(ctx *application.Context) {
			srv.NotifyAll(server.UIMsg{Type: server.UINavigate, Page: "home"})
		})
	viewMenu.Add("文件传输").
		SetAccelerator("CmdOrCtrl+2").
		OnClick(func(ctx *application.Context) {
			srv.NotifyAll(server.UIMsg{Type: server.UINavigate, Page: "sftp"})
		})
	viewMenu.Add("端口隧道").
		SetAccelerator("CmdOrCtrl+3").
		OnClick(func(ctx *application.Context) {
			srv.NotifyAll(server.UIMsg{Type: server.UINavigate, Page: "tunnels"})
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

// openConfigWindow opens (or focuses) a dedicated configuration window. The web
// frontend renders the matching config page for the given hash route, so each
// configuration feature runs in its own native window instead of a modal in the
// main window.
func openConfigWindow(app *application.App, baseURL, name, route, title string, width, height int, dark bool) {
	if existing, ok := app.Window.GetByName(name); ok {
		existing.Show()
		existing.Focus()
		return
	}
	// Configuration windows are frameless too, so they match the app theme.
	frameless := runtime.GOOS != "darwin"
	opts := application.WebviewWindowOptions{
		Name:      name,
		Title:     title,
		Frameless: frameless,
		URL:       baseURL + "/?win=" + name + "#" + route,
		Width:     width, Height: height,
		MinWidth:  380,
		MinHeight: 420,
		// Configuration windows keep the chrome minimal: no menu bar, so the
		// per-window menu is not duplicated on Windows/Linux.
		UseApplicationMenu: false,
		BackgroundColour:   appBackground(dark),
		Windows:            windowsChrome(dark),
	}
	if frameless {
		opts.Windows.DisableMenu = true
	}
	w := app.Window.NewWithOptions(opts)
	w.Center()
	w.Focus()
}

// isDarkTheme reports whether the saved app theme is dark (the default).
func isDarkTheme(store *storage.Store) bool {
	return store.Settings().Theme != "light"
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
