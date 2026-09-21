package server

import (
	"encoding/json"
	"fmt"
)

// AppActions are desktop-shell callbacks the web UI can invoke over RPC to drive
// native behaviour (opening windows, quitting, DevTools, window controls). They
// are unset in headless mode, where those actions are unavailable.
type AppActions struct {
	OpenSettings func()
	OpenKeys     func()
	OpenSftp     func()
	OpenTunnels  func()
	Quit         func()
	OpenDevTools func()
	// WindowControl performs an action on the named window and returns an
	// action-specific result (e.g. the maximise state for "is-maximised").
	WindowControl func(window, action string) interface{}
}

// SetAppActions installs the desktop-shell callbacks. It should be called before
// the app starts serving requests.
func (s *Server) SetAppActions(a AppActions) { s.actions = a }

// handleAppAction runs one of the desktop-shell actions on behalf of the web
// frontend, which owns the custom menu bar on Windows/Linux.
func (s *Server) handleAppAction(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	run := func(fn func()) error {
		if fn == nil {
			return fmt.Errorf("无窗口模式下不可用")
		}
		fn()
		return nil
	}

	switch p.Action {
	case "settings":
		return nil, run(s.actions.OpenSettings)
	case "keys":
		return nil, run(s.actions.OpenKeys)
	case "sftp":
		return nil, run(s.actions.OpenSftp)
	case "tunnels":
		return nil, run(s.actions.OpenTunnels)
	case "quit":
		return nil, run(s.actions.Quit)
	case "devtools":
		return nil, run(s.actions.OpenDevTools)
	default:
		return nil, fmt.Errorf("未知操作：%s", p.Action)
	}
}

// handleOpenSession opens a terminal session in the session window. The tool
// windows (file transfer, tunnels) cannot host terminal tabs, so they ask the
// backend to broadcast the target to every window; the session window picks it
// up and opens the tab.
func (s *Server) handleOpenSession(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ConnID string `json:"connId"`
		Host   string `json:"host"`
		Port   int    `json:"port"`
		User   string `json:"user"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.ConnID == "" && p.Host == "" {
		return nil, fmt.Errorf("缺少连接信息")
	}
	s.NotifyAll(UIMsg{Type: UIOpenSession, ConnID: p.ConnID, Host: p.Host, Port: p.Port, User: p.User})
	return nil, nil
}

// handleWindowControl drives the custom title bar's window buttons on the
// frameless windows (minimise / maximise / close).
func (s *Server) handleWindowControl(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		Window string `json:"window"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if s.actions.WindowControl == nil {
		return nil, fmt.Errorf("无窗口模式下不可用")
	}
	return s.actions.WindowControl(p.Window, p.Action), nil
}
