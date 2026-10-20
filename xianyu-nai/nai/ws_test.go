package nai

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// 起一个假的 OneBot WS 服务端（框架侧）：收 action、回 echo。
func TestWSServerHandshakeAndCall(t *testing.T) {
	var mu sync.Mutex
	var got []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("no hijacker")
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		if key == "" || r.Header.Get("Upgrade") != "websocket" {
			t.Errorf("握手头不全: %v", r.Header)
		}
		_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n\r\n"))
		ws := newWSConn(conn, rw.Reader)
		defer ws.Close()

		for {
			op, data, err := ws.readFrame()
			if err != nil {
				return
			}
			if op != wsOpText {
				continue
			}
			mu.Lock()
			got = append(got, string(data))
			mu.Unlock()
			var msg struct {
				Action string `json:"action"`
				Echo   string `json:"echo"`
			}
			_ = json.Unmarshal(data, &msg)
			_ = ws.WriteText(`{"status":"ok","retcode":0,"data":{"message_id":1},"echo":"` +
				msg.Echo + `"}`)
		}
	}))
	defer srv.Close()

	url := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/"
	ws, err := DialWS(url)
	if err != nil {
		t.Fatalf("DialWS: %v", err)
	}
	defer ws.Close()
	if !ws.mask {
		t.Fatal("客户端连接未置掩码位")
	}

	// 客户端读循环：把 echo 回包交给 caller
	caller := newWSCaller(ws)
	// 服务端不会主动推事件，这里只需处理回包
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			op, data, err := ws.readFrame()
			if err != nil {
				return
			}
			if op != wsOpText {
				continue
			}
			var head struct {
				Echo *string `json:"echo"`
			}
			if json.Unmarshal(data, &head) == nil && head.Echo != nil {
				caller.resolve(*head.Echo, json.RawMessage(data))
			}
		}
	}()

	raw, err := caller.Call("send_group_msg", map[string]interface{}{
		"group_id": 88888,
		"message":  []Seg{TextSeg("hi")},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(string(raw), `"retcode":0`) {
		t.Fatalf("回包不对: %s", raw)
	}
	ws.Close()
	<-readDone

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("服务端没收到 action")
	}
	var req struct {
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}
	_ = json.Unmarshal([]byte(got[0]), &req)
	if req.Action != "send_group_msg" {
		t.Fatalf("action=%q", req.Action)
	}
	if req.Params["group_id"].(float64) != 88888 {
		t.Fatalf("params=%v", req.Params)
	}
}

// 分片文本帧要能拼回完整消息。
func TestWSFragmentReassembly(t *testing.T) {
	cli, srv := pipePair()
	defer cli.Close()
	defer srv.Close()

	go func() {
		// 手写两片：text(FIN=0) + continuation(FIN=1)
		_, _ = srv.c.Write([]byte{0x01, 0x03, 'a', 'b', 'c'})
		_, _ = srv.c.Write([]byte{0x80, 0x02, 'd', 'e'})
	}()
	op, data, err := cli.readFrame()
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if op != wsOpText || string(data) != "abcde" {
		t.Fatalf("op=%d data=%q", op, data)
	}
}

// 掩码帧要能正确解出。
func TestWSMaskedFrame(t *testing.T) {
	cli, srv := pipePair()
	cli.mask = true
	defer cli.Close()
	defer srv.Close()

	go func() {
		if err := cli.WriteText("你好，masked"); err != nil {
			t.Error(err)
		}
	}()
	op, data, err := srv.readFrame()
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if op != wsOpText || string(data) != "你好，masked" {
		t.Fatalf("op=%d data=%q", op, data)
	}
}

// ping 应当回 pong（单向验证帧类型）。
func TestWSPing(t *testing.T) {
	cli, srv := pipePair()
	defer cli.Close()
	defer srv.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		op, _, err := srv.readFrame()
		if err != nil {
			t.Errorf("读 ping 失败: %v", err)
			return
		}
		if op != wsOpPing {
			t.Errorf("期望 ping，得到 op=%d", op)
		}
	}()
	if err := cli.Ping(); err != nil {
		t.Fatal(err)
	}
	<-done
}

// wsAcceptKey 与 RFC 6455 §1.3 的示例值一致。
func TestWSAcceptKey(t *testing.T) {
	// 例子来自 RFC 6455：key = "dGhlIHNhbXBsZSBub25jZQ=="
	// 期望 accept = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := wsAcceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept=%s", got)
	}
}

// pipePair 造一对已握手的连接（用本机 TCP，避免 net.Pipe 的同步阻塞语义）。
func pipePair() (*WSConn, *WSConn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		panic(err)
	}
	srv, ok := <-accepted
	if !ok {
		panic("accept 失败")
	}
	return newWSConn(client, nil), newWSConn(srv, nil)
}
