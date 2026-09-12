package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// ---- ZMODEM 协议方向识别回归测试 ----
// 用内存 SSH server 模拟远端 lrzsz，验证 routeOutput / zmSession 对
// rz（上传）与 sz（下载）握手帧的方向判定与前端推送。
//
// 关键回归点：rz 的 banner 和 ZRINIT 帧到达时，前端必须收到
// zmodem.send-file（显示“选择文件”横幅）且**不得**被 download-start
// 移除——download-start 只能由 sz 的 ZRQINIT 触发。

type fakeRemote struct {
	rcvCh chan []byte // 本端写给远端的字节
	outCh chan []byte // 远端要推给本端的字节
}

func (f *fakeRemote) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	copy(b, p)
	f.rcvCh <- b
	return len(p), nil
}

func (f *fakeRemote) Close() error { return nil }

// hexHeader 构造一个 ZMODEM hex 头帧（同 lrzsz 发出的形态，尾部 CR LF XON）。
func hexHeader(typ byte, p0, p1, p2, p3 byte) []byte {
	body := []byte{typ, p0, p1, p2, p3}
	out := []byte("**\x18B")
	for _, b := range body {
		out = append(out, zmHexDigits[b>>4], zmHexDigits[b&0xf])
	}
	out = append(out, fmt.Sprintf("%04x", zmCRC16(body))...)
	out = append(out, '\r', '\n', 0x8a, 0x11)
	return out
}

// rzBurst 构造 lrzsz rz 的真实输出形态：banner + ZRINIT(flags=0x23) + 8x ZDLE。
func rzBurst() []byte {
	return []byte("rz waiting to receive." +
		string(hexHeader(1, 0x00, 0x00, 0x00, 0x23)) +
		strings.Repeat("\x18", 8))
}

// szBurst 构造 lrzsz sz 的输出形态：banner + ZRQINIT(全零 flags) + 8x ZDLE。
func szBurst() []byte {
	return []byte("sz waiting to receive." +
		string(hexHeader(0, 0x00, 0x00, 0x00, 0x00)) +
		strings.Repeat("\x18", 8))
}

// startFakeSSH 起一个内存 SSH server：PTY 会话输出由 fake 驱动，
// stdin 字节投递到 rcvCh。
func startFakeSSH(t *testing.T) (addr string, fr *fakeRemote) {
	fr = &fakeRemote{rcvCh: make(chan []byte, 64), outCh: make(chan []byte, 64)}

	config := &ssh.ServerConfig{NoClientAuth: true}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sconn, chans, reqs, err := ssh.NewServerConn(c, config)
				if err != nil {
					return
				}
				defer sconn.Close()
				go ssh.DiscardRequests(reqs)
				for newCh := range chans {
					if newCh.ChannelType() != "session" {
						newCh.Reject(ssh.UnknownChannelType, "no")
						continue
					}
					ch, chReqs, _ := newCh.Accept()
					go func() {
						for req := range chReqs {
							switch req.Type {
							case "pty-req":
								req.Reply(true, nil)
							case "shell":
								req.Reply(true, nil)
								go func() {
									for out := range fr.outCh {
										ch.Write(out)
									}
								}()
								go func() {
									buf := make([]byte, 4096)
									for {
										n, err := ch.Read(buf)
										if n > 0 {
											b := make([]byte, n)
											copy(b, buf[:n])
											fr.rcvCh <- b
										}
										if err != nil {
											return
										}
									}
								}()
							default:
								if req.WantReply {
									req.Reply(false, nil)
								}
							}
						}
					}()
				}
			}(c)
		}
	}()

	return ln.Addr().String(), fr
}

// newTestWs 构造一个 WebSession，把 zmodem push 消息投递到返回的 channel。
func newTestWs(t *testing.T, id string) (*WebSession, *fakeRemote, chan string) {
	addr, fr := startFakeSSH(t)
	cfg := &ssh.ClientConfig{
		User:            "u",
		Auth:            []ssh.AuthMethod{ssh.Password("p")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	cli, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })

	pushed := make(chan string, 16)
	ws := &WebSession{
		id:     id,
		client: cli,
		stdin:  fr,
		done:   make(chan struct{}),
	}
	ws.pushOutput = func(d []byte) {}
	ws.pushMsg = func(v interface{}) {
		if m, ok := v.(zmodemMsg); ok {
			pushed <- m.Type
		}
	}
	return ws, fr, pushed
}

// drain 在 quiet 时间内收集所有已到达的 push，然后返回。
func drain(t *testing.T, pushed chan string, quiet time.Duration) []string {
	t.Helper()
	var out []string
	deadline := time.After(quiet)
	for {
		select {
		case m := <-pushed:
			out = append(out, m)
		case <-deadline:
			return out
		}
	}
}

// TestParserDiag 隔离验证：hexHeader 生成的帧能否被 zmParser 解析出 onHeader。
func TestParserDiag(t *testing.T) {
	calls := 0
	var lastTyp byte
	p := newZmParser(
		func(typ byte, params [4]byte) {
			calls++
			lastTyp = typ
			t.Logf("onHeader typ=0x%02x params=%v", typ, params)
		},
		nil,
		func() { t.Log("onError (CRC mismatch)") },
	)
	hdr := hexHeader(1, 0x00, 0x00, 0x00, 0x23)
	t.Logf("frame: %x", hdr)
	for _, b := range hdr {
		p.Feed(b)
	}
	if calls != 1 || lastTyp != 1 {
		t.Fatalf("expect 1 onHeader with typ=1, got calls=%d typ=%d", calls, lastTyp)
	}
}

// TestRzUploadBanner 关键回归：远端 rz 一次 burst（banner + ZRINIT + 8x
// ZDLE，lrzsz 真实形态）到达时，前端收到 zmodem.send-file（弹选择文件栏），
// 且不得收到 download-start（否则横幅会被移除）。
func TestRzUploadBanner(t *testing.T) {
	ws, _, pushed := newTestWs(t, "test")
	ws.routeOutput(rzBurst())

	got := drain(t, pushed, 400*time.Millisecond)
	if len(got) == 0 {
		t.Fatal("no zmodem push at all (file picker never shown)")
	}
	hasSend := false
	for _, m := range got {
		if m == "zmodem.send-file" {
			hasSend = true
		}
		if m == "zmodem.download-start" {
			t.Fatalf("rz 场景收到 download-start（横幅会被移除）: %v", got)
		}
	}
	if !hasSend {
		t.Fatalf("rz 场景缺少 zmodem.send-file: %v", got)
	}
}

// TestRzBannerOnly 远端 rz 只打印 banner（未带 ZRINIT 帧的变体）时，
// banner 兜底仍推送 send-file。
func TestRzBannerOnly(t *testing.T) {
	ws, _, pushed := newTestWs(t, "test-banner")
	ws.routeOutput([]byte("rz waiting to receive.\r\n"))

	got := drain(t, pushed, 300*time.Millisecond)
	if len(got) != 1 || got[0] != "zmodem.send-file" {
		t.Fatalf("expect [zmodem.send-file], got %v", got)
	}
}

// TestRzSplitFrame 远端 rz 的 burst 被切成两块到达（真实网络常见）：
// 即使 ZRINIT 帧解析失败，banner 兜底也已推送 send-file，且全程无
// download-start 移除横幅。
func TestRzSplitFrame(t *testing.T) {
	ws, _, pushed := newTestWs(t, "test-split")
	burst := rzBurst()
	mid := len(burst) / 2
	ws.routeOutput(burst[:mid])
	ws.routeOutput(burst[mid:])

	got := drain(t, pushed, 400*time.Millisecond)
	hasSend := false
	for _, m := range got {
		if m == "zmodem.send-file" {
			hasSend = true
		}
		if m == "zmodem.download-start" {
			t.Fatalf("rz 分块场景收到 download-start: %v", got)
		}
	}
	if !hasSend {
		t.Fatalf("rz 分块场景缺少 zmodem.send-file: %v", got)
	}
}

// TestSzDownloadStart 反向回归：远端 sz 的 ZRQINIT 帧经 onHeader 解析后
// 推送 download-start（下载路径），不误弹上传栏。
func TestSzDownloadStart(t *testing.T) {
	ws, _, pushed := newTestWs(t, "test-sz")
	ws.routeOutput(szBurst())

	got := drain(t, pushed, 400*time.Millisecond)
	hasDown := false
	for _, m := range got {
		if m == "zmodem.download-start" {
			hasDown = true
		}
		if m == "zmodem.send-file" {
			t.Fatalf("sz 场景误推 send-file: %v", got)
		}
	}
	if !hasDown {
		t.Fatalf("sz 场景缺少 zmodem.download-start: %v", got)
	}
}
