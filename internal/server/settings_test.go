package server

import (
	"encoding/json"
	"testing"
)

// TestSettingsAutoRunDefaultsOn covers the inverted storage of the auto-run
// option: an existing settings.json predates the field, so its zero value has to
// mean "enabled" or every user would silently lose the behaviour on upgrade.
func TestSettingsAutoRunDefaultsOn(t *testing.T) {
	srv := newTestServer(nil)

	out, err := srv.handleGetSettings(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, ok := out.(settingsView)
	if !ok {
		t.Fatalf("unexpected settings view: %T", out)
	}
	if !view.AIAutoRun {
		t.Fatalf("auto-run must default to enabled, got %+v", view)
	}
}

// TestSettingsAutoRunRoundTrip checks that turning it off persists in inverted
// form and is reported back in the positive form the UI reasons about.
func TestSettingsAutoRunRoundTrip(t *testing.T) {
	srv := newTestServer(nil)

	raw, err := json.Marshal(saveSettingsParams{Theme: "dark", AIAutoRun: false})
	if err != nil {
		t.Fatal(err)
	}
	out, err := srv.handleSaveSettings(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	if view, ok := out.(settingsView); !ok || view.AIAutoRun {
		t.Fatalf("the reply should report auto-run as disabled: %+v", out)
	}
	if st := srv.store.Settings(); !st.AINoAutoRun {
		t.Fatalf("disabling auto-run must set the inverted stored flag: %+v", st)
	}

	// And it survives a reload: the flag round-trips through the view.
	back, err := srv.handleGetSettings(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view := back.(settingsView); view.AIAutoRun {
		t.Fatalf("auto-run should still be disabled: %+v", view)
	}
}
