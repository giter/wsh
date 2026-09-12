package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// TestZmSendFileToRemote 端到端验证后端上传链路：rz 建立会话后，
// 把浏览器选择的文件投递到 fileCh，远端应收到 ZFILE 头 + 文件名 payload。
func TestZmSendFileToRemote(t *testing.T) {
	ws, fr, _ := newTestWs(t, "send")
	ws.routeOutput(rzBurst())
	time.Sleep(200 * time.Millisecond) // 等会话建立 + mode=Send

	ws.zmMu.Lock()
	zm := ws.zm
	ws.zmMu.Unlock()
	if zm == nil || !zm.active() {
		t.Fatal("no active zm session after rz burst")
	}
	if !zm.sendable() {
		t.Fatal("session not sendable")
	}

	// 模拟浏览器选择文件（与 sendZmFile 的 fileCh 投递一致）。
	zm.fileCh <- zmFile{name: "KEP.zip", size: 10, data: []byte("0123456789")}

	// 收集远端收到的字节，直到出现文件名（sendFileHeader 的 payload）。
	deadline := time.After(3 * time.Second)
	var all []byte
	for {
		select {
		case b := <-fr.rcvCh:
			all = append(all, b...)
			if bytes.Contains(all, []byte("KEP.zip")) {
				t.Logf("remote received ZFILE header + name (total %d bytes)", len(all))
				return
			}
		case <-deadline:
			t.Fatalf("remote never received ZFILE, got %d bytes: %x", len(all), all)
		}
	}
}

// TestZmChunkedUpload 走完整分块上传 RPC 链路：sendBegin → sendChunk×N →
// sendEnd，远端最终收到 ZFILE 头 + 文件名。
func TestZmChunkedUpload(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)
	ws, fr, _ := newTestWs(t, "chunk")
	srv.sessionsMu.Lock()
	srv.sessions[ws.id] = ws
	srv.sessionsMu.Unlock()

	ws.routeOutput(rzBurst())
	time.Sleep(200 * time.Millisecond) // 等会话建立 + mode=Send

	// sendBegin
	if _, err := srv.handleZmodemSendBegin(nil, json.RawMessage(
		`{"sessionId":"chunk","name":"KEP.zip","size":3000000}`)); err != nil {
		t.Fatalf("sendBegin: %v", err)
	}
	// sendChunk × 3（每块 1 MiB）
	chunk := make([]byte, 1<<20)
	for i := 0; i < 3; i++ {
		payload := map[string]interface{}{
			"sessionId": "chunk",
			"index":     i,
			"data":      base64.StdEncoding.EncodeToString(chunk),
		}
		raw, _ := json.Marshal(payload)
		if _, err := srv.handleZmodemSendChunk(nil, raw); err != nil {
			t.Fatalf("sendChunk %d: %v", i, err)
		}
	}
	// sendEnd
	if _, err := srv.handleZmodemSendEnd(nil, json.RawMessage(`{"sessionId":"chunk"}`)); err != nil {
		t.Fatalf("sendEnd: %v", err)
	}

	// 远端应收到 ZFILE + 文件名
	deadline := time.After(3 * time.Second)
	var all []byte
	for {
		select {
		case b := <-fr.rcvCh:
			all = append(all, b...)
			if bytes.Contains(all, []byte("KEP.zip")) {
				t.Logf("chunked upload: remote received ZFILE (total %d bytes)", len(all))
				return
			}
		case <-deadline:
			t.Fatalf("chunked upload: remote never received ZFILE, got %d bytes", len(all))
		}
	}
}
