package server

import (
	"testing"
)

// realProbeOutput is the shape a typical Linux host returns for probeCommand.
const realProbeOutput = `0.52 0.58 0.59 1/234 5678
---
MemTotal:       16316560 kB
MemAvailable:    4089640 kB
---
/dev/sda1       41152716  12345678   9876543  56% /
---
8
`

func TestParseProbeOutput(t *testing.T) {
	st, err := parseProbeOutput(realProbeOutput)
	if err != nil {
		t.Fatal(err)
	}
	if st.Load1 != 0.52 || st.Load5 != 0.58 || st.Load15 != 0.59 {
		t.Fatalf("load parsing wrong: %+v", st)
	}
	if st.Cores != 8 {
		t.Fatalf("cores = %d, want 8", st.Cores)
	}
	// load1 / cores * 100 = 6.5%
	if st.CPUPercent < 6.4 || st.CPUPercent > 6.6 {
		t.Fatalf("cpu percent = %v, want ~6.5", st.CPUPercent)
	}
	if st.MemTotalKB != 16316560 {
		t.Fatalf("mem total = %d", st.MemTotalKB)
	}
	if st.MemUsedKB != 16316560-4089640 {
		t.Fatalf("mem used = %d, want the total minus available", st.MemUsedKB)
	}
	if st.MemPercent < 74 || st.MemPercent > 76 {
		t.Fatalf("mem percent = %v, want ~75", st.MemPercent)
	}
	if st.DiskTotalKB != 41152716 || st.DiskUsedKB != 12345678 {
		t.Fatalf("disk parsing wrong: %+v", st)
	}
	if st.DiskPercent < 29 || st.DiskPercent > 31 {
		t.Fatalf("disk percent = %v, want ~30", st.DiskPercent)
	}
}

// TestParseProbeOutputMinimal covers a container-style host where df and the
// load average are present but nproc is missing (the fallback path).
func TestParseProbeOutputMinimal(t *testing.T) {
	out := "0.00 0.01 0.05 0/100 1234\n---\nMemTotal:        1000000 kB\nMemAvailable:     250000 kB\n---\noverlay          1000000   500000   500000  50% /\n---\n"
	st, err := parseProbeOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Cores != 0 {
		t.Fatalf("cores = %d, want 0 when nproc is unavailable", st.Cores)
	}
	if st.CPUPercent != 0 {
		t.Fatalf("cpu percent should stay 0 without a core count, got %v", st.CPUPercent)
	}
	if st.MemPercent != 75 {
		t.Fatalf("mem percent = %v, want 75", st.MemPercent)
	}
	if st.DiskPercent != 50 {
		t.Fatalf("disk percent = %v, want 50", st.DiskPercent)
	}
}

func TestParseProbeOutputRejectsPartial(t *testing.T) {
	if _, err := parseProbeOutput("garbage"); err == nil {
		t.Fatal("expected an error for output without the section markers")
	}
}

// TestFinishStatsClampsLoad keeps the percentage display sane on a loaded box
// where load can exceed the core count.
func TestFinishStatsClampsLoad(t *testing.T) {
	st := finishStats(HostStats{Load1: 40, Cores: 4})
	if st.CPUPercent != 100 {
		t.Fatalf("cpu percent = %v, want it clamped to 100", st.CPUPercent)
	}
}

// TestProbeManagerRefCounting checks the sampler is started once and stopped
// when the last session for a connection closes.
func TestProbeManagerRefCounting(t *testing.T) {
	srv := newTestServer(nil)
	pm := srv.probes

	pm.acquire("c1")
	pm.acquire("c1")
	pm.mu.Lock()
	if pm.refs["c1"] != 2 {
		t.Fatalf("refs = %d, want 2", pm.refs["c1"])
	}
	if _, ok := pm.stops["c1"]; !ok {
		t.Fatal("a sampler should be running for c1")
	}
	pm.mu.Unlock()

	pm.release("c1")
	pm.mu.Lock()
	if pm.refs["c1"] != 1 {
		t.Fatalf("refs = %d, want 1", pm.refs["c1"])
	}
	pm.mu.Unlock()

	pm.release("c1")
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.refs["c1"]; ok {
		t.Fatal("refs entry should be gone")
	}
	if _, ok := pm.stops["c1"]; ok {
		t.Fatal("sampler should be stopped after the last session closed")
	}
}

// TestProbeSkipsAdhocSessions documents that quick-connect sessions (which own a
// non-pooled client) are not sampled.
func TestProbeSkipsAdhocSessions(t *testing.T) {
	srv := newTestServer(nil)
	srv.probes.acquire("adhoc-123")
	srv.probes.mu.Lock()
	defer srv.probes.mu.Unlock()
	if len(srv.probes.refs) != 0 {
		t.Fatalf("ad-hoc sessions should not be probed: %+v", srv.probes.refs)
	}
}
