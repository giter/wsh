package storage

import "testing"

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
