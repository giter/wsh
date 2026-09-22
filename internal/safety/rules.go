package safety

import (
	"path"
	"strings"
)

// wrapper describes a command whose own arguments name another command to run
// (sudo, env, timeout, ssh, xargs, ...). Resolving them is essential: without it
// `sudo rm -rf /` would be reported as a harmless call to "sudo".
type wrapper struct {
	// valueFlags are short/long flags in exact "-x" / "--xyz" form that consume
	// the following argument.
	valueFlags map[string]bool
	// assignArgs marks leading VAR=value arguments as belonging to the wrapper.
	assignArgs bool
	// positional marks wrappers whose first bare argument is a value rather than
	// the command (timeout's duration, ssh's host).
	positional bool
}

func flagSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

var wrappers = map[string]wrapper{
	"sudo":    {valueFlags: flagSet("-u", "-g", "-p", "-C", "-h", "-r", "-t", "-U", "-T", "-A", "-D", "-R"), assignArgs: true},
	"doas":    {valueFlags: flagSet("-u", "-C")},
	"env":     {valueFlags: flagSet("-u", "-C", "-S"), assignArgs: true},
	"nohup":   {},
	"time":    {},
	"nice":    {valueFlags: flagSet("-n")},
	"ionice":  {valueFlags: flagSet("-c", "-n", "-p")},
	"setsid":  {},
	"stdbuf":  {valueFlags: flagSet("-i", "-o", "-e")},
	"timeout": {valueFlags: flagSet("-s", "-k", "--signal", "--kill-after"), positional: true},
	"watch":   {valueFlags: flagSet("-n")},
	"xargs":   {valueFlags: flagSet("-I", "-n", "-P", "-s", "-d", "-a", "-E", "-L", "-R"), assignArgs: true},
	// busybox's first argument is the applet, i.e. the command itself.
	"busybox": {},
	"command": {},
	"builtin": {},
	"exec":    {},
	"ssh":     {valueFlags: flagSet("-p", "-l", "-i", "-o", "-F", "-E", "-b", "-c", "-D", "-J", "-L", "-R", "-m", "-O", "-Q", "-S", "-w", "-W"), positional: true},
}

// resolve peels command wrappers until the command that actually runs is found,
// returning its base name and its remaining arguments. It returns ("", nil) when
// no inner command can be determined (e.g. `sudo -v`) or when an argument is
// dynamic, in which case nothing can be proven statically.
func resolve(name string, args []arg) (string, []arg) {
	for hop := 0; hop < 6; hop++ {
		w, ok := wrappers[name]
		if !ok {
			return name, args
		}
		idx := -1
		// A wrapper that takes a bare positional value still needs to see it
		// before the command (timeout's duration, ssh's host).
		positionalSeen := !w.positional
		for i := 0; i < len(args); i++ {
			ar := args[i]
			if ar.dynamic {
				return "", nil
			}
			t := ar.text
			if w.assignArgs && !strings.HasPrefix(t, "-") && isAssignment(t) {
				continue
			}
			if strings.HasPrefix(t, "-") && t != "-" {
				if base := flagBase(t); w.valueFlags[base] {
					i++ // the flag consumes the next argument
				}
				continue
			}
			if !positionalSeen {
				positionalSeen = true
				continue
			}
			idx = i
			break
		}
		if idx < 0 {
			return "", nil
		}
		name = path.Base(args[idx].text)
		args = args[idx+1:]
	}
	return "", nil
}

// flagBase strips an inline value from a long flag ("--signal=KILL" -> "--signal").
func flagBase(t string) string {
	if i := strings.IndexByte(t, '='); i > 0 {
		return t[:i]
	}
	return t
}

// isAssignment reports whether s looks like a VAR=value argument.
func isAssignment(s string) bool {
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range s[:eq] {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// applyRules classifies one resolved command. Unlisted commands are safe by
// default, which keeps everyday read-only work at zero friction.
func (a *analyzer) applyRules(name string, args []arg) {
	if name == "mkfs" || strings.HasPrefix(name, "mkfs.") {
		a.ruleMkfs(name, args)
		return
	}
	switch name {
	case "rm":
		a.ruleRm(args)
	case "dd":
		a.ruleDd(args)
	case "iptables", "ip6tables", "iptables-legacy", "iptables-nft", "iptables-restore",
		"ip6tables-restore":
		if hasFlag(args, "-F", "--flush", "-X", "--delete-chain", "-Z", "--zero") {
			a.caution("iptables.flush", "风险操作：清空防火墙规则，需二次确认", name)
		}
	case "nft":
		if hasWord(args, "flush") {
			a.caution("nft.flush", "风险操作：清空 nftables 规则集，需二次确认", name)
		}
	case "ufw":
		if hasWord(args, "disable") || hasWord(args, "reset") {
			a.caution("ufw.disable", "风险操作：关闭或重置防火墙，需二次确认", name)
		}
	case "firewall-cmd":
		if hasWord(args, "--panic-on") || hasWord(args, "--complete-reload") {
			a.caution("firewall.reload", "风险操作：会中断防火墙规则，需二次确认", name)
		}
	case "systemctl", "service", "rc-service", "rcctl":
		// The action is the first argument for systemctl but the second for
		// `service <name> stop`, so match anywhere instead of positionally.
		if hasWord(args, "stop") || hasWord(args, "restart") || hasWord(args, "disable") ||
			hasWord(args, "mask") || hasWord(args, "kill") || hasWord(args, "poweroff") ||
			hasWord(args, "reboot") || hasWord(args, "halt") || hasWord(args, "isolate") ||
			hasWord(args, "rescue") {
			a.caution("service.change", "风险操作：会停止/重启/禁用系统服务，需二次确认", name)
		}
	case "shutdown", "reboot", "halt", "poweroff", "init", "telinit", "fastboot":
		a.caution("power", "风险操作：会关机或重启主机，需二次确认", name)
	case "kill":
		if hasFlag(args, "-9", "-KILL", "-s") && hasWord(args, "-1") {
			a.caution("kill.broadcast", "风险操作：向所有进程发送信号，需二次确认", name)
		}
	case "killall", "pkill":
		a.caution("kill.bulk", "风险操作：按名字批量结束进程，需二次确认", name)
	case "fuser":
		if hasFlag(args, "-k", "--kill") {
			a.caution("fuser.kill", "风险操作：结束占用文件的进程，需二次确认", name)
		}
	case "crontab":
		if hasFlag(args, "-r") {
			a.caution("crontab.remove", "风险操作：删除全部计划任务，需二次确认", name)
		}
	case "atrm":
		a.caution("at.remove", "风险操作：删除计划任务，需二次确认", name)
	case "userdel", "groupdel":
		a.caution("user.delete", "风险操作：删除用户或用户组，需二次确认", name)
	case "docker":
		a.ruleDocker(args)
	case "kubectl":
		if hasWord(args, "delete") || hasWord(args, "drain") || hasWord(args, "scale") {
			a.caution("kubectl.change", "风险操作：会变更集群资源，需二次确认", name)
		}
	case "truncate":
		a.caution("truncate", "风险操作：截断文件内容，需二次确认", name)
	case "shred":
		a.caution("shred", "风险操作：不可恢复地覆写文件，需二次确认", name)
	case "chmod", "chown", "chgrp", "chattr":
		if hasFlag(args, "-R", "--recursive") && anyRootish(args) {
			a.caution("perm.recursive", "风险操作：递归修改根目录或系统目录权限，需二次确认", name)
		}
	case "parted", "fdisk", "sfdisk", "sgdisk", "wipefs", "mkswap", "blkdiscard", "hdparm":
		if anyDevice(args) {
			a.caution("disk.partition", "风险操作：会修改磁盘分区或文件系统，需二次确认", name)
		}
	case "swapoff":
		if hasFlag(args, "-a", "--all") {
			a.caution("swapoff.all", "风险操作：关闭全部交换分区，需二次确认", name)
		}
	case "mount", "umount":
		if hasFlag(args, "-a", "--all") || anyRootish(args) {
			a.caution("mount.change", "风险操作：会卸载/挂载文件系统，需二次确认", name)
		}
	}
}

// ruleRm handles recursive removal. Ordinary `rm file` / `rm -f file` is left
// alone; anything recursive is at least worth a confirmation, and a recursive
// delete aimed at `/` or a system directory is refused outright.
func (a *analyzer) ruleRm(args []arg) {
	recursive, force, noPreserve := false, false, false
	var targets []arg
	flagsEnd := false
	for _, ar := range args {
		if flagsEnd || ar.dynamic {
			targets = append(targets, ar)
			continue
		}
		t := ar.text
		switch {
		case t == "--":
			flagsEnd = true
		case strings.HasPrefix(t, "--"):
			switch flagBase(strings.TrimPrefix(t, "--")) {
			case "recursive":
				recursive = true
			case "force":
				force = true
			case "no-preserve-root":
				noPreserve = true
			}
		case strings.HasPrefix(t, "-") && t != "-":
			// Short options may be clustered ("-rf", "-rfi"); only r/R and f
			// matter here, and they are recognised as flags — never as part of a
			// path, which is what a naive substring match would get wrong.
			for _, c := range t[1:] {
				switch c {
				case 'r', 'R':
					recursive = true
				case 'f':
					force = true
				}
			}
		default:
			targets = append(targets, ar)
		}
	}
	flags := "-r"
	if force {
		flags = "-rf"
	}
	if !recursive {
		return
	}
	if noPreserve {
		a.block("rm.no-preserve-root", "安全阻断：递归删除根目录且跳过保护（rm "+flags+" --no-preserve-root）", "rm")
		return
	}
	rootish, dynamic, glob := false, false, false
	for _, t := range targets {
		switch {
		case t.dynamic:
			dynamic = true
		case isRootish(t.text):
			rootish = true
		case hasGlob(t.text):
			glob = true
		}
	}
	switch {
	case rootish:
		a.block("rm.root", "安全阻断：递归删除根目录或系统目录（rm "+flags+" …）", "rm")
	case dynamic:
		a.caution("rm.dynamic", "风险操作：递归删除的目标无法静态确定，需二次确认", "rm")
	case glob:
		a.caution("rm.glob", "风险操作：递归删除通配符匹配的批量路径，需二次确认", "rm")
	default:
		a.caution("rm.recursive", "风险操作：递归删除目录（rm "+flags+"），需二次确认", "rm")
	}
}

// ruleDd refuses a raw write to a block device (the classic disk destructor).
func (a *analyzer) ruleDd(args []arg) {
	for _, ar := range args {
		if ar.dynamic || !strings.HasPrefix(ar.text, "of=") {
			continue
		}
		target := strings.TrimPrefix(ar.text, "of=")
		switch {
		case isBlockDevice(target):
			a.block("dd.device", "安全阻断：向块设备写入原始数据（dd of="+target+"）", "dd")
		case strings.HasPrefix(target, "/dev/"):
			a.caution("dd.device.other", "风险操作：向 /dev 下的设备写入数据，需二次确认", "dd")
		}
	}
}

// ruleMkfs refuses formatting a block device.
func (a *analyzer) ruleMkfs(name string, args []arg) {
	for _, ar := range args {
		if ar.dynamic {
			continue
		}
		if isBlockDevice(ar.text) || strings.HasPrefix(ar.text, "/dev/") {
			a.block("mkfs.device", "安全阻断：格式化块设备（"+name+" "+ar.text+"）", name)
			return
		}
	}
	a.caution("mkfs", "风险操作：格式化文件系统，需二次确认", name)
}

// ruleDocker flags the bulk-destructive docker subcommands.
func (a *analyzer) ruleDocker(args []arg) {
	bulk := [][]string{
		{"system", "prune"}, {"volume", "prune"}, {"container", "prune"},
		{"image", "prune"}, {"network", "prune"}, {"builder", "prune"},
	}
	for _, seq := range bulk {
		if hasSeq(args, seq) {
			a.caution("docker.prune", "风险操作：批量清理 Docker 资源，需二次确认", "docker")
			break
		}
	}
	if force := hasFlag(args, "-f", "--force"); force {
		if hasWord(args, "rm") {
			a.caution("docker.rm", "风险操作：强制删除容器，需二次确认", "docker")
		}
		if hasWord(args, "rmi") {
			a.caution("docker.rmi", "风险操作：强制删除镜像，需二次确认", "docker")
		}
		if hasWord(args, "volume") {
			a.caution("docker.volume.rm", "风险操作：强制删除数据卷，需二次确认", "docker")
		}
	}
	if hasWord(args, "kill") {
		a.caution("docker.kill", "风险操作：强制结束容器，需二次确认", "docker")
	}
}

// ---- helpers ----

// systemDirs are the top-level directories whose wholesale removal is treated as
// unrecoverable.
var systemDirs = []string{
	"/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib32", "/lib64",
	"/opt", "/proc", "/root", "/run", "/sbin", "/srv", "/sys", "/usr", "/var",
}

// isRootish reports whether a path is the filesystem root, a home shortcut or a
// system directory (and its direct glob) — targets whose recursive removal
// destroys the machine.
func isRootish(p string) bool {
	if p == "" {
		return false
	}
	t := strings.TrimRight(p, "/")
	if t == "" {
		return true // "/" or "//"
	}
	switch t {
	case ".", "..", "~", "~/*", "/*", "/.", "~/.":
		return true
	}
	for _, d := range systemDirs {
		if t == d || t == d+"/*" {
			return true
		}
	}
	return false
}

// isBlockDevice reports whether a path is a raw block device. Only the target
// path is inspected, so `dd of=/dev/null` stays harmless.
func isBlockDevice(p string) bool {
	for _, prefix := range []string{
		"/dev/sd", "/dev/hd", "/dev/vd", "/dev/xvd", "/dev/nvme", "/dev/mmcblk",
		"/dev/disk", "/dev/loop", "/dev/dm-", "/dev/ram", "/dev/md", "/dev/sr",
	} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// isSystemPath reports whether a path lives in a system directory, where an
// accidental `>` overwrite is worth a confirmation.
func isSystemPath(p string) bool {
	for _, prefix := range []string{
		"/etc/", "/boot/", "/bin/", "/sbin/", "/usr/bin/", "/usr/sbin/",
		"/usr/lib/", "/lib/", "/sys/", "/proc/",
	} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func hasGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func texts(args []arg) []string {
	out := make([]string, 0, len(args))
	for _, ar := range args {
		if !ar.dynamic {
			out = append(out, ar.text)
		}
	}
	return out
}

func hasWord(args []arg, w string) bool {
	for _, ar := range args {
		if !ar.dynamic && ar.text == w {
			return true
		}
	}
	return false
}

func hasFlag(args []arg, flags ...string) bool {
	for _, ar := range args {
		if ar.dynamic {
			continue
		}
		base := flagBase(ar.text)
		for _, f := range flags {
			if base == f {
				return true
			}
		}
	}
	return false
}

// hasSeq reports whether the given arguments appear consecutively.
func hasSeq(args []arg, seq []string) bool {
	words := texts(args)
	for i := 0; i+len(seq) <= len(words); i++ {
		match := true
		for j, want := range seq {
			if words[i+j] != want {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func anyRootish(args []arg) bool {
	for _, ar := range args {
		if !ar.dynamic && (isRootish(ar.text) || hasGlob(ar.text)) {
			return true
		}
	}
	return false
}

func anyDevice(args []arg) bool {
	for _, ar := range args {
		if !ar.dynamic && (strings.HasPrefix(ar.text, "/dev/") || isBlockDevice(ar.text)) {
			return true
		}
	}
	return false
}
