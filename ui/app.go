package ui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ssh "sshclient/ssh"
	"sshclient/storage"
)

// App is the top-level application shell.
type App struct {
	app     fyne.App
	win     fyne.Window
	store   *storage.Store
	pool    *ssh.Pool
	tunnels *ssh.TunnelManager

	stack    *fyne.Container
	viewList []*fyne.Container

	// live terminal sessions, keyed by a unique tab id
	sessionMu sync.Mutex
	sessions  map[string]*sessionEntry
}

type sessionEntry struct {
	session *ssh.TerminalSession
}

// NewApp builds the application and shows the main window.
func NewApp() {
	a := app.NewWithID("com.fyneshell.client")
	a.Settings().SetTheme(&AppTheme{})

	store, err := storage.NewStore()
	if err != nil {
		store = &storage.Store{}
	}
	pool := ssh.NewPool()
	tm := ssh.NewTunnelManager(pool)

	ui := &App{
		app:      a,
		store:    store,
		pool:     pool,
		tunnels:  tm,
		sessions: make(map[string]*sessionEntry),
	}

	w := a.NewWindow("FyneShell")
	w.Resize(fyne.NewSize(1100, 720))
	ui.win = w

	ui.buildUI()

	w.SetOnClosed(func() {
		ui.closeAllSessions()
		tm.StopAll()
		pool.CloseAll()
		_ = store.Save()
	})

	w.ShowAndRun()
}

// buildUI composes the nav rail and the content stack.
func (ui *App) buildUI() {
	connView := ui.newConnectionsView()
	transferView := ui.newTransferView()
	tunnelView := ui.newTunnelsView()

	// Hide everything except the first view initially; the rail controls
	// which one is visible.
	ui.viewList = []*fyne.Container{connView, transferView, tunnelView}
	objects := make([]fyne.CanvasObject, len(ui.viewList))
	for i, v := range ui.viewList {
		objects[i] = v
	}
	ui.stack = container.NewStack(objects...)

	ui.win.SetContent(container.NewBorder(
		nil, nil, ui.buildRail(), nil, ui.stack,
	))
	ui.selectView(0)
}

// buildRail creates the vertical navigation rail.
func (ui *App) buildRail() fyne.CanvasObject {
	labels := []string{"连接", "传输", "隧道"}
	icons := []fyne.Resource{theme.ComputerIcon(), theme.UploadIcon(), theme.StorageIcon()}

	buttons := make([]*widget.Button, 0, 3)
	for i := 0; i < 3; i++ {
		idx := i
		b := widget.NewButtonWithIcon(labels[i], icons[i], func() {
			ui.selectView(idx)
		})
		b.Alignment = widget.ButtonAlignLeading
		buttons = append(buttons, b)
	}

	inner := container.NewVBox(
		widget.NewLabel(""),
		buttons[0],
		buttons[1],
		buttons[2],
		layout.NewSpacer(),
	)
	return container.NewPadded(container.NewVBox(inner))
}

// selectView makes only the given view visible within the stack.
func (ui *App) selectView(index int) {
	if ui.stack == nil {
		return
	}
	for i, view := range ui.viewList {
		if i == index {
			view.Show()
		} else {
			view.Hide()
		}
	}
	ui.stack.Refresh()
}

func (ui *App) addSession(id string, s *ssh.TerminalSession) {
	ui.sessionMu.Lock()
	defer ui.sessionMu.Unlock()
	ui.sessions[id] = &sessionEntry{session: s}
}

func (ui *App) removeSession(id string) {
	ui.sessionMu.Lock()
	defer ui.sessionMu.Unlock()
	delete(ui.sessions, id)
}

func (ui *App) closeAllSessions() {
	ui.sessionMu.Lock()
	defer ui.sessionMu.Unlock()
	for _, e := range ui.sessions {
		e.session.Close()
	}
	ui.sessions = make(map[string]*sessionEntry)
}
