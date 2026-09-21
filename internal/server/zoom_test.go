package server

import (
	"encoding/json"
	"testing"
)

// TestAppActionZoom covers the zoom action behind Ctrl +/-/ and the options
// window: the window name and factor must reach the desktop shell verbatim, since
// only the shell can zoom a webview.
func TestAppActionZoom(t *testing.T) {
	s := newTestServer(nil)
	var gotWindow string
	var gotFactor float64
	s.SetAppActions(AppActions{
		SetZoom: func(window string, factor float64) {
			gotWindow, gotFactor = window, factor
		},
	})

	if _, err := s.handleAppAction(nil, json.RawMessage(`{"action":"zoom","window":"main","zoom":1.5}`)); err != nil {
		t.Fatalf("handleAppAction: %v", err)
	}
	if gotWindow != "main" {
		t.Errorf("window should be passed through, got %q", gotWindow)
	}
	if gotFactor != 1.5 {
		t.Errorf("factor should be passed through, got %v", gotFactor)
	}
}

// TestAppActionZoomHeadless checks the headless case: with no webview the action
// reports itself unavailable rather than panicking on a nil callback.
func TestAppActionZoomHeadless(t *testing.T) {
	s := newTestServer(nil)

	if _, err := s.handleAppAction(nil, json.RawMessage(`{"action":"zoom","window":"main","zoom":1.5}`)); err == nil {
		t.Fatal("expected zoom to be unavailable without a window")
	}
}

// TestAppActionUnknownStillRejected guards the default arm, so adding zoom did not
// turn unknown actions into silent no-ops.
func TestAppActionUnknownStillRejected(t *testing.T) {
	s := newTestServer(nil)

	if _, err := s.handleAppAction(nil, json.RawMessage(`{"action":"nope"}`)); err == nil {
		t.Fatal("expected an unknown action to be rejected")
	}
}
