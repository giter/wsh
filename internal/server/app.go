package server

import (
	"encoding/json"
	"fmt"
)

// AppActions are desktop-shell callbacks the web UI can invoke over RPC to drive
// native behaviour (opening configuration windows, quitting, DevTools, window
// controls). They are unset in headless mode, where those actions are
// unavailable.
type AppActions struct {
	OpenSettings func()
	OpenKeys     func()
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
	case "quit":
		return nil, run(s.actions.Quit)
	case "devtools":
		return nil, run(s.actions.OpenDevTools)
	default:
		return nil, fmt.Errorf("未知操作：%s", p.Action)
	}
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
