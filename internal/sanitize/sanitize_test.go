package sanitize

import (
	"strings"
	"testing"
)

// TestMaskRedacts is the "must be masked" half of the contract.
func TestMaskRedacts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ipv4",
			in:   "connect failed to 192.168.1.100 port 22",
			want: "connect failed to " + IPMask + " port 22",
		},
		{
			name: "ipv4 with port",
			in:   "listening on 10.0.0.5:8080",
			want: "listening on " + IPMask + ":8080",
		},
		{
			name: "two ips in one line",
			in:   "1.1.1.1 -> 2.2.2.2",
			want: IPMask + " -> " + IPMask,
		},
		{
			name: "ipv6 full",
			in:   "peer 2001:0db8:0000:0000:0000:0000:0000:0001 up",
			want: "peer " + IPMask + " up",
		},
		{
			name: "ipv6 compressed",
			in:   "peer fe80::1%eth0 up",
			want: "peer " + IPMask + "%eth0 up",
		},
		{
			name: "ipv6 loopback",
			in:   "listening on ::1",
			want: "listening on " + IPMask,
		},
		{
			name: "password flag",
			in:   "mysql -h db --password=hunter2 -u root",
			want: "mysql -h db --password=" + Sensitive + " -u root",
		},
		{
			name: "password flag space",
			in:   "deploy --password hunter2 --verbose",
			want: "deploy --password " + Sensitive + " --verbose",
		},
		{
			name: "env secret",
			in:   "API_KEY=abcd1234efgh EXPORTED=1",
			want: "API_KEY=" + Sensitive + " EXPORTED=1",
		},
		{
			name: "quoted secret",
			in:   `password="p@ss word" tail`,
			want: `password=` + Sensitive + ` tail`,
		},
		{
			name: "bearer token",
			in:   "Authorization: Bearer abc.def.ghi",
			want: "Authorization: Bearer " + Sensitive,
		},
		{
			name: "jwt",
			in:   "cookie java=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dBjftJeZ4CVP",
			want: "cookie java=" + Sensitive,
		},
		{
			name: "aws key",
			in:   "using AKIAIOSFODNN7EXAMPLE for upload",
			want: "using " + Sensitive + " for upload",
		},
		{
			name: "connection string",
			in:   "dsn postgres://admin:s3cret@db.internal:5432/app",
			want: "dsn postgres://admin:" + Sensitive + "@db.internal:5432/app",
		},
		{
			name: "private key block",
			in:   "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk=\n-----END OPENSSH PRIVATE KEY-----\ndone",
			want: Sensitive + "\ndone",
		},
		{
			name: "client secret",
			in:   "client_secret=zzz999",
			want: "client_secret=" + Sensitive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mask(tt.in); got != tt.want {
				t.Fatalf("Mask(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestMaskPreserves is the "must survive untouched" half: over-masking is a
// correctness bug, because it strips the context the LLM needs to diagnose.
func TestMaskPreserves(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"iso timestamp", "2026-09-21T18:00:12 err: out of memory"},
		{"log timestamp", "Sep 21 18:00:12 host sshd[123]: Accepted password"},
		{"three part version", "nginx/1.18.0 (Ubuntu)"},
		{"version with suffix", "server version 8.0.34-0ubuntu0.22.04.1"},
		{"cpp scope symbol", "error: std::array<int, 3> has no member"},
		{"cpp deque symbol", "note: see std::deque<T> for details"},
		{"mac address stays", "link/ether aa:bb:cc:dd:ee:ff brd ff:ff:ff:ff:ff:ff"},
		{"ip inside path", "writing /var/log/1.2.3.4.log"},
		{"duplicated ip fragment", "release 1.2.3.4.5 build"},
		{"percentages", "cpu 99.9% mem 12.1%"},
		{"date only", "2026.09.21 12:00"},
		{"empty", ""},
		{"plain chinese log", "服务 nginx 已启动，监听端口 80"},
		{"grep command", "grep -rn 'timeout' /etc/nginx/nginx.conf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mask(tt.in); got != tt.in {
				t.Fatalf("Mask(%q) modified the text:\n got: %q", tt.in, got)
			}
		})
	}
}

// TestMaskIsStable guards the invariant that makes Mask safe to apply twice
// (e.g. once on the excerpt and once on the assembled prompt).
func TestMaskIsStable(t *testing.T) {
	in := "login to 10.1.2.3 with password=s3cret and AKIAIOSFODNN7EXAMPLE"
	once := Mask(in)
	if twice := Mask(once); twice != once {
		t.Fatalf("Mask is not idempotent:\n once: %q\ntwice: %q", once, twice)
	}
	if !strings.Contains(once, IPMask) || !strings.Contains(once, Sensitive) {
		t.Fatalf("expected both placeholders in %q", once)
	}
}

func TestMaskBytes(t *testing.T) {
	got := string(MaskBytes([]byte("host 172.16.0.1")))
	if got != "host "+IPMask {
		t.Fatalf("MaskBytes = %q", got)
	}
	if MaskBytes(nil) != nil {
		t.Fatal("MaskBytes(nil) should return nil")
	}
}

func TestValidIPv4(t *testing.T) {
	cases := map[string]bool{
		"1.2.3.4":         true,
		"255.255.255.255": true,
		"256.1.1.1":       false,
		"1.2.3":           false,
		"1.2.3.4.5":       false,
	}
	for in, want := range cases {
		m := in
		// Reconstruct the boundary arguments the way applyRule would for a
		// standalone token.
		got := validIPv4(m, "", "")
		if in == "1.2.3.4.5" {
			// The regex itself matches only the first four octets, whose
			// following byte is a dot, so the boundary check rejects it.
			got = validIPv4("1.2.3.4", "", ".")
		}
		if got != want {
			t.Errorf("validIPv4(%q) = %v, want %v", in, got, want)
		}
	}
}
