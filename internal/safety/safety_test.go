package safety

import "testing"

// TestAnalyzeLevels covers the three risk zones, including the bypasses a naive
// substring/regex check misses (see PLAN.md §1.3.4).
func TestAnalyzeLevels(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		level Level
		rule  string // expected rule for the most severe finding ("" = don't care)
	}{
		// ---- 绿区：只读 / 低风险，秒级放行 ----
		{name: "empty", cmd: "", level: LevelSafe},
		{name: "ls", cmd: "ls -la /var/log", level: LevelSafe},
		{name: "cat", cmd: "cat /etc/passwd", level: LevelSafe},
		{name: "grep recursive", cmd: "grep -rn 'foo' /etc/nginx", level: LevelSafe},
		{name: "ps", cmd: "ps aux", level: LevelSafe},
		{name: "tail follow", cmd: "tail -f /var/log/syslog", level: LevelSafe},
		{name: "lsof port", cmd: "lsof -i :8080", level: LevelSafe},
		{name: "ss", cmd: "ss -lntp", level: LevelSafe},
		{name: "find name", cmd: "find /var/log -name '*.log' -mtime +7", level: LevelSafe},
		{name: "systemctl status", cmd: "systemctl status nginx", level: LevelSafe},
		{name: "docker ps", cmd: "docker ps -a", level: LevelSafe},
		{name: "iptables list", cmd: "iptables -L -n", level: LevelSafe},
		{name: "chained read", cmd: "cd /etc && cat hosts | head -20", level: LevelSafe},
		{name: "redirect to dev null", cmd: "echo hello > /dev/null", level: LevelSafe},
		{name: "redirect to temp file", cmd: "echo hello > /tmp/out.txt", level: LevelSafe},
		{name: "rm single file", cmd: "rm /tmp/old.log", level: LevelSafe},
		{name: "rm force file", cmd: "rm -f /tmp/old.log", level: LevelSafe},
		{name: "quoted glob is literal-ish", cmd: `ls "/etc/*"`, level: LevelSafe},
		{name: "version string is not a device", cmd: "echo 1.2.3.4", level: LevelSafe},

		// ---- 红区：不可恢复，硬阻断 ----
		{name: "rm root", cmd: "rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "rm root glob", cmd: "rm -rf /*", level: LevelBlocked, rule: "rm.root"},
		{name: "rm root separated flags", cmd: "rm -r -f /", level: LevelBlocked, rule: "rm.root"},
		{name: "rm root reversed cluster", cmd: "rm -fr /", level: LevelBlocked, rule: "rm.root"},
		{name: "rm root upper R", cmd: "rm -Rf /usr", level: LevelBlocked, rule: "rm.root"},
		{name: "rm etc", cmd: "rm -rf /etc", level: LevelBlocked, rule: "rm.root"},
		{name: "rm etc glob", cmd: "rm -rf /etc/*", level: LevelBlocked, rule: "rm.root"},
		{name: "rm home shortcut", cmd: "rm -rf ~", level: LevelBlocked, rule: "rm.root"},
		{name: "rm home shortcut glob", cmd: "rm -rf ~/*", level: LevelBlocked, rule: "rm.root"},
		{name: "rm dot", cmd: "rm -rf .", level: LevelBlocked, rule: "rm.root"},
		{name: "rm long flags", cmd: "rm --recursive --force /", level: LevelBlocked, rule: "rm.root"},
		{name: "rm after dashdash", cmd: "rm -rf -- /etc", level: LevelBlocked, rule: "rm.root"},
		{name: "rm absolute binary", cmd: "/bin/rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "rm no preserve root", cmd: "rm -rf / --no-preserve-root", level: LevelBlocked, rule: "rm.no-preserve-root"},
		{name: "sudo rm", cmd: "sudo rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "sudo with user flag", cmd: "sudo -u root rm -rf /etc", level: LevelBlocked, rule: "rm.root"},
		{name: "sudo env assignment", cmd: "sudo FOO=1 rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "env wrapper", cmd: "env FOO=bar rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "bash -c quoted", cmd: `bash -c 'rm -rf /'`, level: LevelBlocked, rule: "rm.root"},
		{name: "sh -c double quoted", cmd: `sh -c "rm -rf /"`, level: LevelBlocked, rule: "rm.root"},
		{name: "bash -lc cluster", cmd: `bash -lc 'rm -rf /'`, level: LevelBlocked, rule: "rm.root"},
		{name: "eval", cmd: `eval "rm -rf /"`, level: LevelBlocked, rule: "rm.root"},
		{name: "su -c", cmd: `su -c 'rm -rf /'`, level: LevelBlocked, rule: "rm.root"},
		{name: "nohup shell", cmd: `nohup sh -c 'rm -rf /' &`, level: LevelBlocked, rule: "rm.root"},
		{name: "command substitution", cmd: `echo "$(rm -rf /)"`, level: LevelBlocked, rule: "rm.root"},
		{name: "pipeline xargs", cmd: "echo /tmp | xargs rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "ssh remote", cmd: "ssh deploy@10.0.0.5 rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "ssh with port", cmd: "ssh -p 2222 deploy@10.0.0.5 rm -rf /etc", level: LevelBlocked, rule: "rm.root"},
		{name: "find -exec with root", cmd: `find / -maxdepth 1 -exec rm -rf /etc ;`, level: LevelBlocked, rule: "rm.root"},
		{name: "subshell", cmd: "(cd / && rm -rf /)", level: LevelBlocked, rule: "rm.root"},
		{name: "chained after safe", cmd: "ls /tmp && rm -rf /", level: LevelBlocked, rule: "rm.root"},
		{name: "dd to disk", cmd: "dd if=/dev/zero of=/dev/sda bs=1M", level: LevelBlocked, rule: "dd.device"},
		{name: "dd to nvme", cmd: "dd if=/dev/zero of=/dev/nvme0n1", level: LevelBlocked, rule: "dd.device"},
		{name: "redirect to disk", cmd: "echo 1 > /dev/sda", level: LevelBlocked, rule: "redirect.device"},
		{name: "mkfs ext4", cmd: "mkfs.ext4 /dev/sda1", level: LevelBlocked, rule: "mkfs.device"},
		{name: "mkfs plain", cmd: "mkfs -t ext4 /dev/vda1", level: LevelBlocked, rule: "mkfs.device"},
		{name: "fork bomb", cmd: ":(){ :|:& };:", level: LevelBlocked, rule: "forkbomb"},
		{name: "fork bomb named fn", cmd: "boom(){ boom|boom& }; boom", level: LevelBlocked, rule: "forkbomb"},

		// ---- 黄区：需二次确认 ----
		{name: "rm recursive temp", cmd: "rm -rf /tmp/build", level: LevelCaution, rule: "rm.recursive"},
		{name: "rm recursive relative", cmd: "rm -r node_modules", level: LevelCaution, rule: "rm.recursive"},
		{name: "rm recursive home subdir", cmd: "rm -rf ~/Downloads/tmp", level: LevelCaution, rule: "rm.recursive"},
		{name: "rm bare glob", cmd: "rm -rf *", level: LevelCaution, rule: "rm.glob"},
		{name: "rm glob subpath", cmd: "rm -rf /var/log/*.gz", level: LevelCaution, rule: "rm.glob"},
		{name: "rm dynamic target", cmd: "rm -rf $TARGET", level: LevelCaution, rule: "rm.dynamic"},
		{name: "name containing r is not a flag", cmd: "rm -rf report/", level: LevelCaution, rule: "rm.recursive"},
		{name: "iptables flush", cmd: "iptables -F", level: LevelCaution, rule: "iptables.flush"},
		{name: "nft flush", cmd: "nft flush ruleset", level: LevelCaution, rule: "nft.flush"},
		{name: "ufw disable", cmd: "ufw disable", level: LevelCaution, rule: "ufw.disable"},
		{name: "systemctl stop", cmd: "systemctl stop nginx", level: LevelCaution, rule: "service.change"},
		{name: "systemctl restart", cmd: "systemctl restart sshd", level: LevelCaution, rule: "service.change"},
		{name: "service stop", cmd: "service mysql stop", level: LevelCaution, rule: "service.change"},
		{name: "shutdown", cmd: "shutdown -h now", level: LevelCaution, rule: "power"},
		{name: "reboot", cmd: "reboot", level: LevelCaution, rule: "power"},
		{name: "kill broadcast", cmd: "kill -9 -1", level: LevelCaution, rule: "kill.broadcast"},
		{name: "pkill", cmd: "pkill -9 nginx", level: LevelCaution, rule: "kill.bulk"},
		{name: "crontab remove", cmd: "crontab -r", level: LevelCaution, rule: "crontab.remove"},
		{name: "userdel", cmd: "userdel -r bob", level: LevelCaution, rule: "user.delete"},
		{name: "docker prune", cmd: "docker system prune -a", level: LevelCaution, rule: "docker.prune"},
		{name: "docker rm force", cmd: "docker rm -f web", level: LevelCaution, rule: "docker.rm"},
		{name: "truncate", cmd: "truncate -s 0 app.log", level: LevelCaution, rule: "truncate"},
		{name: "redirect system file", cmd: "echo x > /etc/hosts", level: LevelCaution, rule: "redirect.system"},
		{name: "redirect glob", cmd: "cat a b > logs/*.txt", level: LevelCaution, rule: "redirect.glob"},
		{name: "chmod recursive root", cmd: "chmod -R 777 /", level: LevelCaution, rule: "perm.recursive"},
		{name: "find exec placeholder", cmd: `find / -maxdepth 1 -exec rm -rf {} +`, level: LevelCaution, rule: "rm.dynamic"},
		{name: "find delete", cmd: "find /var/log -name '*.gz' -delete", level: LevelCaution, rule: "find.delete"},
		{name: "fdisk", cmd: "fdisk /dev/sdb", level: LevelCaution, rule: "disk.partition"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Analyze(tt.cmd)
			if got.Level != tt.level {
				t.Fatalf("Analyze(%q).Level = %v (reason %q), want %v; findings: %+v",
					tt.cmd, got.Level, got.Reason, tt.level, got.Findings)
			}
			if tt.rule != "" {
				found := false
				for _, f := range got.Findings {
					if f.Rule == tt.rule {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("Analyze(%q) missing rule %q; findings: %+v", tt.cmd, tt.rule, got.Findings)
				}
			}
		})
	}
}

// TestParseErrorIsSafe documents that incomplete input (as typed by the user)
// is handed to the shell rather than being flagged: the parser would otherwise
// report "blocked" on half-typed quotes.
func TestParseErrorIsSafe(t *testing.T) {
	got := Analyze(`rm -rf "/`)
	if got.ParseError == "" {
		t.Fatal("expected a parse error for unbalanced quote")
	}
	if got.Level != LevelSafe {
		t.Fatalf("level = %v, want safe (parse errors are handed to the shell)", got.Level)
	}
}

func TestClassifyAndHelpers(t *testing.T) {
	if Classify("ls") != LevelSafe {
		t.Error("Classify(ls) should be safe")
	}
	if Classify("rm -rf /") != LevelBlocked {
		t.Error("Classify(rm -rf /) should be blocked")
	}
	if r := Analyze("rm -rf /"); !r.Blocked() || r.NeedsConfirm() {
		t.Errorf("Blocked/NeedsConfirm mismatch: %+v", r)
	}
	if r := Analyze("reboot"); r.Blocked() || !r.NeedsConfirm() {
		t.Errorf("Blocked/NeedsConfirm mismatch: %+v", r)
	}
	if r := Analyze("ls"); r.Blocked() || r.NeedsConfirm() || r.Reason != "" {
		t.Errorf("safe command should carry no reason: %+v", r)
	}
}

// TestFindingDeduplication keeps the UI list short when a rule fires repeatedly
// (e.g. the same command inside a long pipeline).
func TestFindingDeduplication(t *testing.T) {
	got := Analyze("rm -rf / ; rm -rf /")
	n := 0
	for _, f := range got.Findings {
		if f.Rule == "rm.root" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("rm.root reported %d times, want 1: %+v", n, got.Findings)
	}
}

func TestLevelJSON(t *testing.T) {
	b, err := Analyze("rm -rf /").Level.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"blocked"` {
		t.Fatalf("level JSON = %s, want \"blocked\"", b)
	}
}

func TestOversizedCommandIsNotParsed(t *testing.T) {
	got := Analyze("ls " + string(make([]byte, maxCommandLen)))
	if got.ParseError == "" {
		t.Fatal("expected the length guard to report a parse error")
	}
}
