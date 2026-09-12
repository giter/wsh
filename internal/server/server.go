package server

import (
	"encoding/json"
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

	sessionsMu sync.Mutex
	sessions   map[string]*WebSession

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
		clients:  make(map[*wsClient]struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
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
		http.ServeFileFS(w, r, sub, "index.html")
	})

	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

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
