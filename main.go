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
	pool := ssh.NewPool()
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
	log.Printf("FyneShell 服务已启动：%s", url)

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
		Name: "FyneShell",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "FyneShell",
		// On Windows/Linux the menu is per-window: this option makes the
		// window display the application menu set via app.Menu.Set (macOS
		// ignores it and always shows the global menu bar).
		UseApplicationMenu: true,
		Width:              1200,
		Height:             800,
		MinWidth:           800,
		MinHeight:          600,
		URL:                url,
		DevToolsEnabled:    true, // enable the "开发者工具" menu item
	})
	win.Center()

	app.Menu.Set(buildMenu(app, srv, win))

	app.OnShutdown(shutdown)

	if err := app.Run(); err != nil {
		log.Fatalf("wails: %v", err)
	}
}

// buildMenu constructs the native menu bar (文件 / 视图 / 帮助). Items that
// act on the web UI push a server.UIMsg through the server's WebSocket
// channel; the frontend reacts in RPC.handlePush.
func buildMenu(app *application.App, srv *server.Server, win application.Window) *application.Menu {
	menu := app.NewMenu()

	// macOS: standard application menu (About / Quit live there).
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}

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
	if runtime.GOOS != "darwin" { // macOS already has Quit in the app menu
		fileMenu.Add("退出").
			SetAccelerator("CmdOrCtrl+Q").
			OnClick(func(ctx *application.Context) {
				app.Quit()
			})
	}

	optionsMenu := menu.AddSubmenu("选项")
	optionsMenu.Add("选项…").
		SetAccelerator("CmdOrCtrl+Shift+,").
		OnClick(func(ctx *application.Context) {
			srv.NotifyAll(server.UIMsg{Type: server.UIOpenSettings})
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
	helpMenu.Add("关于 FyneShell").OnClick(func(ctx *application.Context) {
		app.Dialog.Info().
			SetTitle("关于 FyneShell").
			SetMessage("FyneShell\n\n基于 Go + Wails v3 的跨平台 SSH 客户端。\n终端渲染：xterm.js").
			Show()
	})
	helpMenu.Add("开发者工具").OnClick(func(ctx *application.Context) {
		win.OpenDevTools()
	})

	return menu
}

// waitSignal blocks until the process receives a termination signal.
func waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}
