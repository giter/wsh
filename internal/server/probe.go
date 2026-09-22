package server

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Node status probe.
//
// While a session is open, a low-frequency goroutine samples the host's load,
// memory and disk. It deliberately uses its own SSH channel (a one-shot
// `Session`) so the interactive PTY is never disturbed, and it only runs for
// connections that are actually in use.

// probeInterval is how often a host is sampled. Targets are servers, not
// dashboards, so a coarse interval is enough and keeps the load negligible.
const probeInterval = 15 * time.Second

// probeTimeout bounds one sampling command.
const probeTimeout = 8 * time.Second

// probeCommand is read-only and POSIX-ish: it works on any Linux with /proc.
// The "---" markers make the sections independent of line counts.
const probeCommand = `LC_ALL=C sh -c 'cat /proc/loadavg 2>/dev/null; echo ---; grep -E "^(MemTotal|MemAvailable):" /proc/meminfo 2>/dev/null; echo ---; df -P / 2>/dev/null | tail -n 1; echo ---; nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo 2>/dev/null'`

// HostStats is one sample of a host's resource usage.
type HostStats struct {
	ConnID string `json:"connId"`
	// Load1/5/15 are the raw 1/5/15-minute load averages.
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	// Cores is the CPU count, used to turn load into a percentage.
	Cores int `json:"cores"`
	// CPUPercent approximates utilisation as load1/cores, which is the standard
	// way to read load on Linux (load counts runnable + uninterruptible tasks).
	CPUPercent  float64 `json:"cpuPercent"`
	MemTotalKB  int64   `json:"memTotalKb"`
	MemUsedKB   int64   `json:"memUsedKb"`
	MemPercent  float64 `json:"memPercent"`
	DiskTotalKB int64   `json:"diskTotalKb"`
	DiskUsedKB  int64   `json:"diskUsedKb"`
	DiskPercent float64 `json:"diskPercent"`
	// At is the sample time in Unix milliseconds.
	At int64 `json:"at"`
	// Error is set when the sample failed; the UI shows it instead of numbers.
	Error string `json:"error,omitempty"`
}

// probeManager owns the sampling goroutines, one per connection in use.
type probeManager struct {
	srv *Server

	mu     sync.Mutex
	refs   map[string]int           // connection ID -> open sessions
	stops  map[string]chan struct{} // connection ID -> stop signal
	latest map[string]HostStats     // last sample per connection
}

func newProbeManager(srv *Server) *probeManager {
	return &probeManager{
		srv:    srv,
		refs:   make(map[string]int),
		stops:  make(map[string]chan struct{}),
		latest: make(map[string]HostStats),
	}
}

// acquire registers one open session for a connection, starting the sampler on
// the first one.
func (p *probeManager) acquire(connID string) {
	if connID == "" || strings.HasPrefix(connID, "adhoc-") {
		// Ad-hoc sessions own their client and are not pooled, so there is
		// nothing stable to sample against.
		return
	}
	p.mu.Lock()
	p.refs[connID]++
	start := p.refs[connID] == 1
	var stop chan struct{}
	if start {
		stop = make(chan struct{})
		p.stops[connID] = stop
	}
	p.mu.Unlock()

	if start {
		go p.loop(connID, stop)
	}
}

// release drops one session, stopping the sampler when the last one closes.
func (p *probeManager) release(connID string) {
	if connID == "" || strings.HasPrefix(connID, "adhoc-") {
		return
	}
	p.mu.Lock()
	if p.refs[connID] > 0 {
		p.refs[connID]--
	}
	if p.refs[connID] == 0 {
		delete(p.refs, connID)
		if stop, ok := p.stops[connID]; ok {
			close(stop)
			delete(p.stops, connID)
		}
		delete(p.latest, connID)
	}
	p.mu.Unlock()
}

// snapshot returns the most recent sample per connection, so a freshly loaded
// window shows values without waiting for the next tick.
func (p *probeManager) snapshot() map[string]HostStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]HostStats, len(p.latest))
	for k, v := range p.latest {
		out[k] = v
	}
	return out
}

func (p *probeManager) remember(st HostStats) {
	p.mu.Lock()
	p.latest[st.ConnID] = st
	p.mu.Unlock()
}

// hostStatsMsg is the push envelope for a sample. The `type` field is what the
// frontend's RPC dispatcher keys on.
type hostStatsMsg struct {
	Type  string    `json:"type"`
	Stats HostStats `json:"stats"`
}

// loop samples immediately and then on a ticker until stopped.
func (p *probeManager) loop(connID string, stop chan struct{}) {
	sample := func() {
		st := p.sampleOnce(connID)
		p.remember(st)
		p.srv.NotifyAll(hostStatsMsg{Type: "host.stats", Stats: st})
	}

	sample()
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			sample()
		}
	}
}

// sampleOnce runs the probe command over a dedicated channel and parses it.
func (p *probeManager) sampleOnce(connID string) HostStats {
	st := HostStats{ConnID: connID, At: time.Now().UnixMilli()}

	conn := p.srv.store.Connection(connID)
	if conn == nil {
		st.Error = "连接已不存在"
		return st
	}
	client, err := p.srv.pool.Get(conn, nil)
	if err != nil {
		st.Error = "无法连接：" + err.Error()
		return st
	}
	sess, err := client.NewSession()
	if err != nil {
		st.Error = "无法打开采样通道：" + err.Error()
		return st
	}
	defer sess.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := sess.Output(probeCommand)
		done <- result{out: out, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			// The command itself may exit non-zero on a minimal host while still
			// producing usable output, so parse whatever came back first.
			if len(r.out) == 0 {
				st.Error = "采样失败：" + r.err.Error()
				return st
			}
		}
		parsed, perr := parseProbeOutput(string(r.out))
		if perr != nil {
			st.Error = perr.Error()
			return st
		}
		parsed.ConnID = connID
		parsed.At = st.At
		return parsed
	case <-time.After(probeTimeout):
		_ = sess.Signal(ssh.SIGKILL)
		st.Error = "采样超时"
		return st
	}
}

// parseProbeOutput turns the probe command's output into stats. It is pure so it
// can be tested against real outputs from different distributions.
func parseProbeOutput(out string) (HostStats, error) {
	var st HostStats
	sections := strings.Split(out, "---")
	if len(sections) < 3 {
		return st, fmt.Errorf("采样输出不完整")
	}

	// Section 1: load average, e.g. "0.52 0.58 0.59 1/234 5678".
	loadFields := strings.Fields(strings.TrimSpace(sections[0]))
	if len(loadFields) >= 3 {
		st.Load1, _ = strconv.ParseFloat(loadFields[0], 64)
		st.Load5, _ = strconv.ParseFloat(loadFields[1], 64)
		st.Load15, _ = strconv.ParseFloat(loadFields[2], 64)
	}

	// Section 2: MemTotal / MemAvailable in kB. Used memory is derived from the
	// two, because MemAvailable (not MemFree) is what reflects real pressure.
	var memAvailableKB int64
	for _, line := range strings.Split(sections[1], "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			st.MemTotalKB = value
		case "MemAvailable":
			memAvailableKB = value
		}
	}
	if st.MemTotalKB > 0 && memAvailableKB > 0 {
		st.MemUsedKB = st.MemTotalKB - memAvailableKB
	}

	// Section 3: `df -P /` last line, e.g. "/dev/sda1 41152716 1234 5678 12% /".
	dfFields := strings.Fields(strings.TrimSpace(sections[2]))
	if len(dfFields) >= 4 {
		st.DiskTotalKB, _ = strconv.ParseInt(dfFields[1], 10, 64)
		st.DiskUsedKB, _ = strconv.ParseInt(dfFields[2], 10, 64)
	}

	// Section 4: CPU count.
	if len(sections) >= 4 {
		if n, err := strconv.Atoi(strings.TrimSpace(sections[3])); err == nil && n > 0 {
			st.Cores = n
		}
	}

	return finishStats(st), nil
}

// finishStats derives the percentages that depend on more than one field.
func finishStats(st HostStats) HostStats {
	if st.Cores > 0 {
		st.CPUPercent = st.Load1 / float64(st.Cores) * 100
		if st.CPUPercent > 100 {
			st.CPUPercent = 100
		}
	}
	if st.MemTotalKB > 0 {
		if st.MemUsedKB < 0 {
			st.MemUsedKB = 0
		}
		st.MemPercent = float64(st.MemUsedKB) / float64(st.MemTotalKB) * 100
	}
	if st.DiskTotalKB > 0 {
		st.DiskPercent = float64(st.DiskUsedKB) / float64(st.DiskTotalKB) * 100
	}
	return st
}
