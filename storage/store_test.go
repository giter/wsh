package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestStore returns a store with a temp path so the mutating helpers can
// persist without touching the real config directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{path: t.TempDir() + "/config.json", settings: defaultSettings()}
}

// idsOf returns the connection IDs of one group in display order.
func idsOf(s *Store, folderID string) []string {
	var out []string
	for _, c := range s.Connections() {
		if c.FolderID == folderID {
			out = append(out, c.ID)
		}
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMoveConnection covers the drag-and-drop move: into a folder, out of it,
// and reordering inside one group.
func TestMoveConnection(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"a", "b", "c"} {
		if err := s.AddConnection(&Connection{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddFolder(&Folder{ID: "f1", Name: "生产"}); err != nil {
		t.Fatal(err)
	}

	// a, b, c are ungrouped in insertion order.
	if got := idsOf(s, ""); !equalIDs(got, []string{"a", "b", "c"}) {
		t.Fatalf("initial ungrouped order = %v", got)
	}

	// Move b into f1 at the end.
	if err := s.MoveConnection("b", "f1", -1); err != nil {
		t.Fatal(err)
	}
	if got := idsOf(s, "f1"); !equalIDs(got, []string{"b"}) {
		t.Fatalf("f1 = %v, want [b]", got)
	}
	if got := idsOf(s, ""); !equalIDs(got, []string{"a", "c"}) {
		t.Fatalf("ungrouped = %v, want [a c]", got)
	}

	// Move a into f1 at index 0, so it lands before b.
	if err := s.MoveConnection("a", "f1", 0); err != nil {
		t.Fatal(err)
	}
	if got := idsOf(s, "f1"); !equalIDs(got, []string{"a", "b"}) {
		t.Fatalf("f1 = %v, want [a b]", got)
	}

	// Move a back out to the ungrouped group at index 1.
	if err := s.MoveConnection("a", "", 1); err != nil {
		t.Fatal(err)
	}
	if got := idsOf(s, ""); !equalIDs(got, []string{"c", "a"}) {
		t.Fatalf("ungrouped = %v, want [c a]", got)
	}

	// An out-of-range index appends instead of failing.
	if err := s.MoveConnection("a", "f1", 99); err != nil {
		t.Fatal(err)
	}
	if got := idsOf(s, "f1"); !equalIDs(got, []string{"b", "a"}) {
		t.Fatalf("f1 = %v, want [b a]", got)
	}

	if err := s.MoveConnection("missing", "f1", 0); err == nil {
		t.Error("moving an unknown connection should fail")
	}
}

// TestReorderConnections checks that an explicit order is applied and that IDs
// missing from the request keep a stable place at the end.
func TestReorderConnections(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := s.AddConnection(&Connection{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.ReorderConnections("", []string{"c", "a"}); err != nil {
		t.Fatal(err)
	}
	if got := idsOf(s, ""); !equalIDs(got, []string{"c", "a", "b", "d"}) {
		t.Fatalf("order = %v, want [c a b d]", got)
	}

	// An unknown ID in the list is ignored rather than erroring.
	if err := s.ReorderConnections("", []string{"d", "zzz", "b"}); err != nil {
		t.Fatal(err)
	}
	if got := idsOf(s, ""); !equalIDs(got, []string{"d", "b", "c", "a"}) {
		t.Fatalf("order = %v, want [d b c a]", got)
	}
}

// TestReorderFolders checks folder ordering.
func TestReorderFolders(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"f1", "f2", "f3"} {
		if err := s.AddFolder(&Folder{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.ReorderFolders([]string{"f3", "f1", "f2"}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range s.Folders() {
		got = append(got, f.ID)
	}
	if !equalIDs(got, []string{"f3", "f1", "f2"}) {
		t.Fatalf("folders = %v, want [f3 f1 f2]", got)
	}
}

// TestMoveConnectionSurvivesReload checks that the arrangement is persisted, not
// just kept in memory: the tree must look the same after a restart.
func TestMoveConnectionSurvivesReload(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"a", "b"} {
		if err := s.AddConnection(&Connection{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddFolder(&Folder{ID: "f1", Name: "生产"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveConnection("b", "f1", -1); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(reloaded, "f1"); !equalIDs(got, []string{"b"}) {
		t.Fatalf("after reload f1 = %v, want [b]", got)
	}
}

// TestDeleteConnectionClearsJumpReferences keeps a saved profile from pointing
// at a bastion that no longer exists.
func TestDeleteConnectionClearsJumpReferences(t *testing.T) {
	s := NewMemoryStore()

	bastion := &Connection{ID: "bastion", Name: "bastion", Host: "b", Port: 22, User: "u"}
	target := &Connection{ID: "target", Name: "target", Host: "t", Port: 22, User: "u", JumpHostIDs: []string{"bastion"}}
	other := &Connection{ID: "other", Name: "other", Host: "o", Port: 22, User: "u", JumpHostIDs: []string{"other-bastion", "bastion"}}

	for _, c := range []*Connection{bastion, target, other} {
		if err := s.AddConnection(c); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.DeleteConnection("bastion"); err != nil {
		t.Fatal(err)
	}

	if got := s.Connection("bastion"); got != nil {
		t.Fatalf("deleted connection should be gone, got %+v", got)
	}
	if got := s.Connection("target").JumpHostIDs; len(got) != 0 {
		t.Fatalf("target should have no jump hosts left, got %v", got)
	}
	// Unrelated hops survive.
	if got := s.Connection("other").JumpHostIDs; len(got) != 1 || got[0] != "other-bastion" {
		t.Fatalf("unrelated hop was dropped: %v", got)
	}
}

func TestMemoryStoreDoesNotPersist(t *testing.T) {
	s := NewMemoryStore()
	if err := s.AddConnection(&Connection{ID: "x", Name: "x", Host: "h", Port: 22, User: "u"}); err != nil {
		t.Fatalf("an in-memory store must not try to write to disk: %v", err)
	}
	if err := s.UpdateSettings(Settings{Theme: "light", AIProvider: "ollama"}); err != nil {
		t.Fatalf("UpdateSettings on an in-memory store: %v", err)
	}
	if got := s.Settings().AIProvider; got != "ollama" {
		t.Fatalf("settings should still be updated in memory, got %q", got)
	}
}

// TestLoadStoreLoadsSettingsWithoutConfigFile covers a fresh install where the
// user has only ever changed global options: config.json does not exist yet, but
// settings.json does. Those options (including the AI provider) must still load
// instead of silently falling back to defaults.
func TestLoadStoreLoadsSettingsWithoutConfigFile(t *testing.T) {
	dir := t.TempDir()
	// UserConfigDir reads XDG_CONFIG_HOME on Unix and AppData on Windows, so
	// point both at the temp dir to keep the test hermetic on either platform.
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)

	wshDir := filepath.Join(dir, "wsh")
	if err := os.MkdirAll(wshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	seed := `{"theme":"light","ai_provider":"openai","ai_model":"gpt-4o-mini"}`
	if err := os.WriteFile(filepath.Join(wshDir, "settings.json"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := LoadStore(filepath.Join(wshDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Settings()
	if got.Theme != "light" || got.AIProvider != "openai" || got.AIModel != "gpt-4o-mini" {
		t.Fatalf("settings.json was ignored without config.json: %+v", got)
	}
}

func TestSettingsDefaults(t *testing.T) {
	s := NewMemoryStore()
	if got := s.Settings().Theme; got != "dark" {
		t.Fatalf("default theme = %q, want dark", got)
	}
	// AI is off by default: the terminal must work with no provider configured.
	if got := s.Settings().AIProvider; got != "" {
		t.Fatalf("AI should be unconfigured by default, got %q", got)
	}
}
