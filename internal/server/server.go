package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// Server is the HTTP/WebSocket backend that bridges the web frontend with the
// SSH, storage, and tunnel layers.
type Server struct {
	store   *storage.Store
	pool    *sshclient.Pool
	tunnels *sshclient.TunnelManager

	web fs.FS // embedded web/ directory

	// actions are desktop-shell callbacks the web UI can trigger over RPC
	// (open config windows, quit, devtools). Unset in headless mode.
	actions AppActions

	sessionsMu sync.Mutex
	sessions   map[string]*WebSession

	// confirms holds one-shot approval tokens for yellow-zone commands
	// (see safety.go).
	confirms *confirmStore

	// probes samples host resources while sessions are open (see probe.go).
	probes *probeManager

	clientsMu sync.Mutex
	clients   map[*wsClient]struct{}

	upgrader websocket.Upgrader
}

// NewServer builds a server over the given backing services.
func NewServer(store *storage.Store, pool *sshclient.Pool, tm *sshclient.TunnelManager, web fs.FS) *Server {
	s := &Server{
		store:    store,
		pool:     pool,
		tunnels:  tm,
		web:      web,
		sessions: make(map[string]*WebSession),
		confirms: newConfirmStore(),
		clients:  make(map[*wsClient]struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	s.probes = newProbeManager(s)
	return s
}

// Handler returns the HTTP mux serving static assets and the WebSocket hub.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// SPA static files. The embed FS is the "web" directory itself.
	sub := s.web
	fileServer := http.FileServer(http.FS(sub))
	// Disable caching so frontend changes appear without a hard refresh.
	mux.Handle("/static/", noCache(http.StripPrefix("/static/", fileServer)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		noCacheHeader(w)
		if _, err := fs.Stat(sub, "index.html"); err != nil {
			// The frontend was never built into the embed FS. Say so plainly
			// instead of returning a bare 404 in the app window.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, frontendNotBuiltPage)
			return
		}
		http.ServeFileFS(w, r, sub, "index.html")
	})

	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

// frontendNotBuiltPage is shown when the embedded frontend is missing, which
// happens when the binary is built without running the frontend build first.
const frontendNotBuiltPage = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>wsh</title></head>
<body style="font-family:system-ui,sans-serif;background:#1b1b22;color:#e7e7f0;padding:40px;line-height:1.7">
<h2>前端未构建</h2>
<p>这个二进制里没有打包前端资源。请先构建前端再重新编译：</p>
<pre style="background:#282834;padding:12px 14px;border-radius:8px">cd frontend &amp;&amp; bun install &amp;&amp; bun run build
go build -o wsh .</pre>
<p>或直接使用 <code>./run.sh</code>（它会先构建前端）。</p>
</body></html>`

// noCache wraps an http.Handler with Cache-Control: no-cache headers.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		noCacheHeader(w)
		h.ServeHTTP(w, r)
	})
}

func noCacheHeader(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// Close tears down every live session and tunnel.
func (s *Server) Close() {
	s.sessionsMu.Lock()
	ss := make([]*WebSession, 0, len(s.sessions))
	for _, ses := range s.sessions {
		ss = append(ss, ses)
	}
	s.sessions = make(map[string]*WebSession)
	s.sessionsMu.Unlock()

	for _, ses := range ss {
		ses.Close()
	}
	s.tunnels.StopAll()
	s.pool.CloseAll()
	_ = s.store.Save()
}

// handleProbeSnapshot returns the most recent resource sample per connection.
func (s *Server) handleProbeSnapshot(c *wsClient, params json.RawMessage) (interface{}, error) {
	return s.probes.snapshot(), nil
}

// ---- WebSocket protocol ----

type wsRequest struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type wsResponse struct {
	ID   int         `json:"id"`
	OK   bool        `json:"ok"`
	Data interface{} `json:"data,omitempty"`
	Err  string      `json:"error,omitempty"`
}

// wsClient tracks a single browser connection and serializes writes.
type wsClient struct {
	server *Server
	conn   *websocket.Conn
	wmu    sync.Mutex // guards conn writes
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	c := &wsClient{server: s, conn: conn}
	s.addClient(c)
	defer s.removeClient(c)
	conn.SetReadLimit(64 << 20) // allow large SFTP payloads

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req wsRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			c.respond(req.ID, false, nil, "bad request: "+err.Error())
			continue
		}
		// Each request runs on its own goroutine; long-running SSH calls
		// must not block the read loop.
		go s.dispatch(c, req)
	}
}

func (s *Server) dispatch(c *wsClient, req wsRequest) {
	handler, ok := router[req.Method]
	if !ok {
		c.respond(req.ID, false, nil, "unknown method: "+req.Method)
		return
	}
	data, err := handler(s, c, req.Params)
	if err != nil {
		c.respond(req.ID, false, nil, err.Error())
		return
	}
	c.respond(req.ID, true, data, "")

	// Connections are owned by the session manager window but listed by every
	// other window (session tree, file transfer, tunnels), so a successful
	// mutation is broadcast to keep them in sync.
	if mutatesConnections[req.Method] {
		s.NotifyAll(UIMsg{Type: UIConnectionsChanged})
	}
}

// mutatesConnections lists the RPC methods that change the saved connections or
// their folders (including their order) and therefore invalidate the other
// windows' views.
var mutatesConnections = map[string]bool{
	"connections.save":    true,
	"connections.delete":  true,
	"connections.move":    true,
	"connections.reorder": true,
	"folders.save":        true,
	"folders.delete":      true,
	"folders.reorder":     true,
}

// respond sends a reply to the browser.
func (c *wsClient) respond(id int, ok bool, data interface{}, errMsg string) {
	msg := wsResponse{ID: id, OK: ok, Data: data, Err: errMsg}
	c.send(msg)
}

// send writes a JSON message, serializing concurrent writes.
func (c *wsClient) send(v interface{}) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_ = c.conn.WriteJSON(v)
}

// register tracks a live terminal session.
func (s *Server) register(ws *WebSession) {
	s.sessionsMu.Lock()
	s.sessions[ws.id] = ws
	s.sessionsMu.Unlock()
}

// unregister removes a live terminal session.
func (s *Server) unregister(id string) {
	s.sessionsMu.Lock()
	delete(s.sessions, id)
	s.sessionsMu.Unlock()
}

// addClient tracks a connected browser client.
func (s *Server) addClient(c *wsClient) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	s.clients[c] = struct{}{}
}

// removeClient drops a disconnected browser client.
func (s *Server) removeClient(c *wsClient) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	delete(s.clients, c)
}

// NotifyAll pushes a message to every connected browser client. It is used by
// the native desktop shell (menus) to drive the web frontend.
func (s *Server) NotifyAll(v interface{}) {
	s.clientsMu.Lock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clientsMu.Unlock()

	for _, c := range clients {
		c.send(v)
	}
}
