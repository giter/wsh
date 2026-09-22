package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

func newTestServer(web fs.FS) *Server {
	pool := sshclient.NewPool(nil)
	return NewServer(storage.NewMemoryStore(), pool, sshclient.NewTunnelManager(pool), web)
}

// TestServesBuiltFrontend checks that the embedded frontend is wired up the way
// the Go server expects: "/" serves index.html and every asset it references is
// reachable under "/static/". This guards the Vite `base: "/static/"` setting
// against silent breakage.
func TestServesBuiltFrontend(t *testing.T) {
	sub := os.DirFS("../../web")
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		t.Skipf("frontend build not present (run `bun run build` in frontend/): %v", err)
	}

	h := newTestServer(sub).Handler()

	// "/" must serve index.html.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	page := rec.Body.String()
	if !regexp.MustCompile(`id="root"`).MatchString(page) {
		t.Errorf("index.html does not contain the React root element")
	}

	// Every /static/... asset referenced by index.html must be served.
	assets := regexp.MustCompile(`/static/[^"]+`).FindAllString(page, -1)
	if len(assets) == 0 {
		t.Fatal("index.html references no /static/ assets")
	}
	for _, asset := range assets {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, asset, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", asset, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s returned an empty body", asset)
		}
	}
}

// TestFrontendNotBuilt makes sure a binary built without the frontend explains
// itself instead of returning a bare 404 in the app window.
func TestFrontendNotBuilt(t *testing.T) {
	h := newTestServer(fstest.MapFS{"placeholder.txt": {Data: []byte("x")}}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET / = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bun run build") {
		t.Errorf("error page should point at the frontend build, got: %s", rec.Body.String())
	}
}
