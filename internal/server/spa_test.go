package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// TestServesBuiltFrontend checks that the embedded frontend is wired up the way
// the Go server expects: "/" serves index.html and every asset it references is
// reachable under "/static/". This guards the Vite `base: "/static/"` setting
// against silent breakage.
func TestServesBuiltFrontend(t *testing.T) {
	sub := os.DirFS("../../web")
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		t.Skipf("frontend build not present (run `bun run build` in frontend/): %v", err)
	}

	pool := sshclient.NewPool(nil)
	srv := NewServer(&storage.Store{}, pool, sshclient.NewTunnelManager(pool), sub)
	h := srv.Handler()

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
