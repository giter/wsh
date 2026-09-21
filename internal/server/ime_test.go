package server

import (
	"encoding/json"
	"testing"
)

// TestAppActionImeLatin covers the action behind the passphrase prompt: the
// window name must reach the desktop shell, which is the only side that can
// switch the OS input method to English.
func TestAppActionImeLatin(t *testing.T) {
	s := newTestServer(nil)
	var gotWindow string
	s.SetAppActions(AppActions{
		SetLatinInput: func(window string) { gotWindow = window },
	})

	if _, err := s.handleAppAction(nil, json.RawMessage(`{"action":"ime-latin","window":"main"}`)); err != nil {
		t.Fatalf("handleAppAction: %v", err)
	}
	if gotWindow != "main" {
		t.Errorf("window should be passed through, got %q", gotWindow)
	}
}

// TestAppActionImeLatinHeadless checks the headless case: without a window there
// is no IME to switch, so the action reports itself unavailable instead of
// panicking on a nil callback.
func TestAppActionImeLatinHeadless(t *testing.T) {
	s := newTestServer(nil)

	if _, err := s.handleAppAction(nil, json.RawMessage(`{"action":"ime-latin","window":"main"}`)); err == nil {
		t.Fatal("expected ime-latin to be unavailable without a window")
	}
}
