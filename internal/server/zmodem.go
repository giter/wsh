package server

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// zmSessionMode tells whether we send (upload, remote rz) or receive
// (download, remote sz) during an active transfer.
type zmSessionMode int

const (
	zmModeUnknown zmSessionMode = iota
	zmModeSend                  // we are the sender: browser file -> remote
	zmModeRecv                  // we are the receiver: remote -> browser save
)

// zmFile is a file handed over by the browser for an upload.
type zmFile struct {
	name string
	size int
	data []byte
}

// Timeout before an idle transfer is aborted (waiting for the remote side or
// for the browser to pick a file).
const zmIdleTimeout = 60 * time.Second

// zmodemMsg pushes transfer events to the browser.
type zmodemMsg struct {
	Type      string `json:"type"` // zmodem.send-file | zmodem.receive
	SessionID string `json:"sessionId"`
	Name      string `json:"name,omitempty"`
	Size      int    `json:"size,omitempty"`
	Data      string `json:"data,omitempty"` // base64 payload (receive)
}

// zmSession runs one ZMODEM transfer on a dedicated goroutine. The terminal
// read loop feeds it raw stdout bytes (zmSession.feed); the browser delivers
// the upload file through the fileCh channel; all protocol I/O happens here.
type zmSession struct {
	ws *WebSession

	done     chan struct{}
	doneOnce sync.Once

	stateMu  sync.Mutex // guards mode/upSent/upData for the RPC side
	mode     zmSessionMode
	upSent   bool
	upData   bool
	zfinSent bool

	parser   *zmParser
	inCh     chan []byte
	fileCh   chan zmFile
	cancelCh chan struct{}
	lastAct  time.Time

	dataPhase int // 0 none, 1 file name, 2 file content
	downName  string
	downData  []byte

	upFile zmFile
}

// startZmSession activates a transfer session.
func startZmSession(ws *WebSession) *zmSession {
	z := &zmSession{
		ws:       ws,
		done:     make(chan struct{}),
		inCh:     make(chan []byte, 8),
		fileCh:   make(chan zmFile, 1),
		cancelCh: make(chan struct{}),
		lastAct:  time.Now(),
	}
	z.parser = newZmParser(z.onHeader, z.onData, z.onError)
	log.Printf("zmodem: transfer started (session %s)", ws.id)
	go z.run()
	return z
}

func (z *zmSession) active() bool {
	select {
	case <-z.done:
		return false
	default:
		return true
	}
}

// feed delivers a stdout chunk to the session goroutine.
func (z *zmSession) feed(chunk []byte) {
	select {
	case z.inCh <- chunk:
	default:
		select {
		case z.inCh <- chunk:
		case <-z.done:
		}
	}
}

func (z *zmSession) cancel() {
	select {
	case <-z.cancelCh:
	default:
		close(z.cancelCh)
	}
}

// sendable reports whether the browser may deliver an upload file right now.
func (z *zmSession) sendable() bool {
	z.stateMu.Lock()
	defer z.stateMu.Unlock()
	return z.mode == zmModeSend && !z.upSent
}

func (z *zmSession) run() {
	defer z.finish()
	for {
		select {
		case chunk := <-z.inCh:
			for _, b := range chunk {
				z.parser.Feed(b)
			}
			z.lastAct = time.Now()
		case f := <-z.fileCh:
			z.onFile(f)
			z.lastAct = time.Now()
		case <-z.cancelCh:
			z.sendAbort()
			return
		case <-time.After(zmIdleTimeout):
			if time.Since(z.lastAct) > zmIdleTimeout {
				z.sendAbort()
				return
			}
		}
	}
}

func (z *zmSession) finish() {
	z.doneOnce.Do(func() { close(z.done) })
	log.Printf("zmodem: transfer finished (session %s)", z.ws.id)
	z.ws.zmodemDone(z)
}

func (z *zmSession) push(v interface{}) { z.ws.pushMsg(v) }

func (z *zmSession) sendAbort() {
	_ = zmWriteHexHeader(z.ws.stdin, zABORT, 0, 0, 0, 0, false)
	for i := 0; i < 8; i++ {
		_, _ = z.ws.stdin.Write([]byte{zDLE})
	}
}

// ---- Protocol handling (all on the session goroutine) ----

func (z *zmSession) onHeader(typ byte, params [4]byte) {
	switch z.mode {
	case zmModeUnknown:
		switch typ {
		case zRQINIT:
			// Remote sz: we become the receiver and announce readiness.
			// ZRQINIT is only ever sent by the sender (sz); rz announces
			// itself with ZRINIT instead.
			z.stateMu.Lock()
			z.mode = zmModeRecv
			z.stateMu.Unlock()
			log.Printf("zmodem: detected remote sz, receiving (session %s)", z.ws.id)
			_ = zmWriteHexHeader(z.ws.stdin, zRINIT, zmCANFDX|zmCANOVIO, 0, 0, 0, true)
			// Direction is now known: dismiss any pending upload banner.
			z.push(zmodemMsg{Type: "zmodem.download-start", SessionID: z.ws.id})
		case zRINIT:
			// Remote rz: we become the sender; ask the browser for a file.
			// lrzsz's rz prints its banner and immediately sends a ZRINIT
			// hex frame ("**B0100000023be50"), which announces it is ready
			// to receive our upload.
			z.stateMu.Lock()
			z.mode = zmModeSend
			z.stateMu.Unlock()
			log.Printf("zmodem: detected remote rz, sending (session %s)", z.ws.id)
			z.push(zmodemMsg{Type: "zmodem.send-file", SessionID: z.ws.id})
		}
	case zmModeSend:
		switch typ {
		case zRPOS, zACK:
			z.stateMu.Lock()
			ok := z.upSent && !z.upData
			z.stateMu.Unlock()
			if ok {
				z.sendFileData()
			}
		case zRINIT:
			// rz answered our ZRQINIT (or announced itself): send the file
			// header when we have one; otherwise, if everything was already
			// sent, this re-announcement means "ready for the next file"
			// (none) -> finish.
			z.stateMu.Lock()
			ready := !z.upSent && (len(z.upFile.data) > 0 || z.upFile.name != "")
			done := z.upSent && z.upData && !z.zfinSent
			z.stateMu.Unlock()
			if ready {
				z.sendFileHeader()
			} else if done {
				z.sendZFIN()
			}
		case zCOMPL:
			z.stateMu.Lock()
			done := z.zfinSent
			z.stateMu.Unlock()
			if !done {
				z.sendZFIN()
			}
		case zFIN:
			// Remote finished; answer with the closing "OO".
			_, _ = z.ws.stdin.Write([]byte{'O', 'O'})
			z.doneOnce.Do(func() { close(z.done) })
		}
	case zmModeRecv:
		switch typ {
		case zFILE:
			z.dataPhase = 1 // file name subpacket follows
		case zDATA:
			z.dataPhase = 2 // content subpackets follow
		case zEOF:
			z.finishReceive()
		case zFIN:
			// Acknowledge the remote ZFIN and end the session.
			_ = zmWriteHexHeader(z.ws.stdin, zFIN, 0, 0, 0, 0, false)
			z.doneOnce.Do(func() { close(z.done) })
		}
	}
}

func (z *zmSession) onData(data []byte, end byte) {
	if z.mode == zmModeUnknown {
		return
	}
	switch z.mode {
	case zmModeRecv:
		switch z.dataPhase {
		case 1: // ZFILE payload: file name etc.
			name := string(data)
			if i := strings.IndexByte(name, 0); i >= 0 {
				name = name[:i]
			}
			z.downName = name
			z.dataPhase = 2
			_ = zmWriteHexHeader(z.ws.stdin, zRPOS, 0, 0, 0, 0, false)
		case 2: // ZDATA content
			z.downData = append(z.downData, data...)
			if end == zCRCQ || end == zCRCW {
				_ = zmWriteHexHeader(z.ws.stdin, zACK, uint32(len(z.downData)), 0, 0, 0, false)
			}
		}
	case zmModeSend:
		// Not expected while sending; ignore.
	}
}

func (z *zmSession) onError() {
	if z.mode != zmModeUnknown {
		_ = zmWriteHexHeader(z.ws.stdin, zNAK, 0, 0, 0, 0, false)
	}
}

// onFile delivers the browser-selected file and starts the upload. Used when
// the transfer session was already running (remote rz announced ZRINIT).
func (z *zmSession) onFile(f zmFile) {
	z.stateMu.Lock()
	if z.mode != zmModeSend || z.upSent {
		z.stateMu.Unlock()
		return
	}
	z.upFile = f
	z.stateMu.Unlock()
	z.sendFileHeader()
}

// presetSend arms a freshly created session as the sender with the given file
// and prods the remote rz with ZRQINIT (used when rz only printed its banner
// and is waiting for our request).
func (z *zmSession) presetSend(f zmFile) {
	z.stateMu.Lock()
	z.mode = zmModeSend
	z.upFile = f
	z.upSent = false
	z.upData = false
	z.stateMu.Unlock()
	log.Printf("zmodem: sending upload %s (session %s)", f.name, z.ws.id)
	_ = zmWriteHexHeader(z.ws.stdin, zRQINIT, 0, 0, 0, 0, false)
}

// sendFileHeader emits the ZFILE header and the file-name payload.
func (z *zmSession) sendFileHeader() {
	z.stateMu.Lock()
	if z.upSent {
		z.stateMu.Unlock()
		return
	}
	z.upSent = true
	f := z.upFile
	z.stateMu.Unlock()

	// ZFILE header: binary transfer, no more files follow.
	_ = zmWriteHexHeader(z.ws.stdin, zFILE, zmZCBIN, 0, 0, 0, true)
	// Name payload: fname\0 mtime\0 mode\0 uid\0 gid\0 size\0
	payload := fmt.Sprintf("%s\x000\x000\x000\x000\x00%d\x00", f.name, f.size)
	_ = zmWriteData(z.ws.stdin, []byte(payload), zCRCW)
}

// sendFileData streams the upload file after the remote accepted ZFILE.
func (z *zmSession) sendFileData() {
	z.stateMu.Lock()
	if z.upData {
		z.stateMu.Unlock()
		return
	}
	z.upData = true
	data := z.upFile.data
	z.stateMu.Unlock()

	_ = zmWriteHexHeader(z.ws.stdin, zDATA, 0, 0, 0, 0, false)
	for len(data) > 0 {
		n := len(data)
		if n > zmMaxBlock {
			n = zmMaxBlock
		}
		end := byte(zCRCG)
		if n == len(data) {
			end = zCRCW // last block: expect ZACK
		}
		_ = zmWriteData(z.ws.stdin, data[:n], end)
		data = data[n:]
	}
	_ = zmWriteHexHeader(z.ws.stdin, zEOF, uint32(len(z.upFile.data)), 0, 0, 0, false)
}

func (z *zmSession) sendZFIN() {
	z.stateMu.Lock()
	z.zfinSent = true
	z.stateMu.Unlock()
	_ = zmWriteHexHeader(z.ws.stdin, zFIN, 0, 0, 0, 0, false)
}

// finishReceive hands the downloaded file to the browser and acknowledges.
func (z *zmSession) finishReceive() {
	z.push(zmodemMsg{
		Type:      "zmodem.receive",
		SessionID: z.ws.id,
		Name:      z.downName,
		Size:      len(z.downData),
		Data:      base64.StdEncoding.EncodeToString(z.downData),
	})
	z.downData = nil
	// Ready for the next file; the remote sz then sends ZFIN to finish.
	_ = zmWriteHexHeader(z.ws.stdin, zRINIT, zmCANFDX|zmCANOVIO, 0, 0, 0, true)
}
