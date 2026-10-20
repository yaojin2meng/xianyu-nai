package nai

// 本文件是 OneBot v11 的 WebSocket 通道（RFC 6455 自实现，不引第三方库）。
//
// 原插件是易语言写的 QQ 框架插件 DLL，本身不含网络适配层；重制版用 OneBot v11
// 对接宿主。HTTP 上报 + HTTP API 回调在 onebot.go 里，本文件补上同协议的
// WebSocket 形态，两种接法行为完全一致：
//
//	反向 WS（框架当服务端，插件连出去）  ws://host:port/path
//	正向 WS（插件当服务端，框架连进来）  http://0.0.0.0:port/onebot  Upgrade
//
// 帧层只实现 OneBot 需要的部分：文本帧、分片、ping/pong、close。
// 二进制帧、扩展协商、压缩都不支持。

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// randRead 是 crypto/rand.Read 的别名，便于测试替换。
var randRead = rand.Read

// WebSocket 帧类型（RFC 6455 §5.2）。
const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xA
)

// wsGUID 是握手用的固定串（RFC 6455 §1.3）。
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ErrWSClosed 表示对端已关闭连接。
var ErrWSClosed = errors.New("websocket 已关闭")

// WSConn 是一条已握手的 WebSocket 连接。
type WSConn struct {
	c       net.Conn
	br      *bufio.Reader
	wmu     sync.Mutex
	closing bool
	// mask 为 true 时发出的帧带掩码（RFC 6455 §5.3 要求客户端必须掩码）。
	mask bool
}

func newWSConn(c net.Conn, br *bufio.Reader) *WSConn {
	if br == nil {
		br = bufio.NewReader(c)
	}
	return &WSConn{c: c, br: br}
}

// readFrame 读一帧，返回 op 与负载（已合并分片）。
func (w *WSConn) readFrame() (byte, []byte, error) {
	var payload []byte
	var firstOp byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.br, h[:]); err != nil {
			return 0, nil, err
		}
		fin := h[0]&0x80 != 0
		op := h[0] & 0x0F
		masked := h[1]&0x80 != 0
		n := int(h[1] & 0x7F)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return 0, nil, err
			}
			n = int(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return 0, nil, err
			}
			v := binary.BigEndian.Uint64(b[:])
			if v > 1<<26 {
				return 0, nil, fmt.Errorf("websocket 帧过大: %d", v)
			}
			n = int(v)
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.br, mask[:]); err != nil {
				return 0, nil, err
			}
		}
		buf := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(w.br, buf); err != nil {
				return 0, nil, err
			}
		}
		if masked {
			for i := range buf {
				buf[i] ^= mask[i%4]
			}
		}
		// 控制帧不能分片，直接返回
		if op >= wsOpClose {
			return op, buf, nil
		}
		if op != wsOpContinuation {
			firstOp = op
		}
		payload = append(payload, buf...)
		if fin {
			return firstOp, payload, nil
		}
	}
}

// writeFrame 写一帧。服务端发出的帧不加掩码，客户端加掩码（RFC 6455 §5.1/§5.3）。
func (w *WSConn) writeFrame(op byte, data []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	if w.closing {
		return ErrWSClosed
	}
	return w.writeFrameLocked(op, data)
}

// writeFrameLocked 已持锁的写帧实现。
func (w *WSConn) writeFrameLocked(op byte, data []byte) error {
	var hdr []byte
	hdr = append(hdr, 0x80|op)
	n := len(data)
	maskBit := byte(0)
	if w.mask {
		maskBit = 0x80
	}
	switch {
	case n < 126:
		hdr = append(hdr, maskBit|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, maskBit|126, byte(n>>8), byte(n))
	default:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, maskBit|127)
		hdr = append(hdr, b[:]...)
	}
	payload := data
	if w.mask {
		var k [4]byte
		if _, err := randRead(k[:]); err != nil {
			return err
		}
		hdr = append(hdr, k[:]...)
		payload = make([]byte, n)
		for i := 0; i < n; i++ {
			payload[i] = data[i] ^ k[i%4]
		}
	}
	if _, err := w.c.Write(hdr); err != nil {
		return err
	}
	if n > 0 {
		if _, err := w.c.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// WriteText 发一条文本消息。
func (w *WSConn) WriteText(s string) error { return w.writeFrame(wsOpText, []byte(s)) }

// Ping 发心跳。
func (w *WSConn) Ping() error { return w.writeFrame(wsOpPing, nil) }

// Close 发 close 帧并关闭底层连接。
func (w *WSConn) Close() error {
	w.wmu.Lock()
	if w.closing {
		w.wmu.Unlock()
		return nil
	}
	w.closing = true
	w.wmu.Unlock()
	_ = w.writeFrameRaw(wsOpClose, nil)
	return w.c.Close()
}

// writeFrameRaw 允许在 closing 已置位后再写一次 close 帧。
func (w *WSConn) writeFrameRaw(op byte, data []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	return w.writeFrameLocked(op, data)
}

// ---------- 握手 ----------

// wsAcceptKey 算 Sec-WebSocket-Accept。
func wsAcceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// WSHandler 返回把 HTTP 请求升级成 WebSocket 的处理器。
//
// 框架（NapCat / LLOneBot 等）以"WebSocket 客户端"方式连进来后，
// 事件经同一条连接进来、API 调用也经同一条连接出去。
func (p *Plugin) WSHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "需要 WebSocket 升级", http.StatusBadRequest)
			return
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		if key == "" {
			http.Error(w, "缺少 Sec-WebSocket-Key", http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "当前服务器不支持连接劫持", http.StatusInternalServerError)
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			return
		}
		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n\r\n"
		if _, err := conn.Write([]byte(resp)); err != nil {
			_ = conn.Close()
			return
		}
		ws := newWSConn(conn, rw.Reader)
		go p.ServeWS(ws)
	})
}

// DialWS 作为客户端连上框架的 WebSocket 服务端（反向 WS）。
func DialWS(rawURL string) (*WSConn, error) {
	host, path, tls := splitWSURL(rawURL)
	if host == "" {
		return nil, fmt.Errorf("WebSocket 地址无效：%s", rawURL)
	}
	if path == "" {
		path = "/"
	}
	network := "tcp"
	if tls {
		return nil, fmt.Errorf("暂不支持 wss（请用 ws:// 或前置反向代理）")
	}
	c, err := net.DialTimeout(network, host, 15*time.Second)
	if err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString([]byte("xianyu-nai-ws-key"))
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		_ = c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = c.Close()
		return nil, fmt.Errorf("WebSocket 握手失败：HTTP %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != wsAcceptKey(key) {
		_ = c.Close()
		return nil, fmt.Errorf("WebSocket 握手校验失败")
	}
	w := newWSConn(c, br)
	w.mask = true // 客户端出帧必须加掩码
	return w, nil
}

// splitWSURL 拆 ws://host:port/path。
func splitWSURL(raw string) (host, path string, tls bool) {
	s := raw
	switch {
	case strings.HasPrefix(s, "ws://"):
		s = s[len("ws://"):]
	case strings.HasPrefix(s, "wss://"):
		s = s[len("wss://"):]
		tls = true
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i:], tls
	}
	return s, "/", tls
}

// ---------- 连接上的 OneBot 会话 ----------

// wscaller 把 API 调用发到这条 WebSocket 上，按 echo 等回包。
type wscaller struct {
	ws *WSConn

	mu      sync.Mutex
	pending map[string]chan json.RawMessage
	seq     int
}

func newWSCaller(ws *WSConn) *wscaller {
	return &wscaller{ws: ws, pending: map[string]chan json.RawMessage{}}
}

// Call 发 {"action","params","echo"} 并等同 echo 的回包。
func (c *wscaller) Call(action string, params map[string]interface{}) (json.RawMessage, error) {
	if params == nil {
		params = map[string]interface{}{}
	}
	c.mu.Lock()
	c.seq++
	echo := fmt.Sprintf("xianyu-%d", c.seq)
	ch := make(chan json.RawMessage, 1)
	c.pending[echo] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, echo)
		c.mu.Unlock()
	}()

	body, err := json.Marshal(map[string]interface{}{
		"action": action,
		"params": params,
		"echo":   echo,
	})
	if err != nil {
		return nil, err
	}
	if err := c.ws.WriteText(string(body)); err != nil {
		return nil, err
	}
	select {
	case out := <-ch:
		return out, nil
	case <-time.After(180 * time.Second):
		return nil, fmt.Errorf("OneBot API %s 超时", action)
	}
}

// resolve 把回包交给等待方（按 echo）。
func (c *wscaller) resolve(echo string, raw json.RawMessage) bool {
	c.mu.Lock()
	ch, ok := c.pending[echo]
	c.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- raw:
	default:
	}
	return true
}

// ServeWS 在一条已握手的连接上跑 OneBot 会话：
// 事件进来 -> 交给插件处理 -> 回复仍走这条连接。
func (p *Plugin) ServeWS(ws *WSConn) {
	defer ws.Close()
	caller := newWSCaller(ws)
	host := &OneBotHost{C: caller}
	// 心跳：OneBot 实现通常在 30s 空闲后需要保活。
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if err := ws.Ping(); err != nil {
					return
				}
			}
		}
	}()

	for {
		op, data, err := ws.readFrame()
		if err != nil {
			return
		}
		switch op {
		case wsOpPing:
			_ = ws.writeFrame(wsOpPong, data)
		case wsOpPong, wsOpContinuation, wsOpBinary:
			// 忽略
		case wsOpClose:
			return
		case wsOpText:
			var head struct {
				Echo     *string         `json:"echo"`
				PostType string          `json:"post_type"`
				Raw      json.RawMessage `json:"-"`
			}
			if err := json.Unmarshal(data, &head); err != nil {
				continue
			}
			if head.Echo != nil {
				caller.resolve(*head.Echo, json.RawMessage(data))
				continue
			}
			if head.PostType == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal(data, &ev); err != nil {
				continue
			}
			// 事件处理可能很慢（出图），放后台，保持连接可读
			go func(ev Event) {
				replies := p.OnEventNT(ev)
				deliverVia(host, ev, replies)
			}(ev)
		}
	}
}

// SetHost 在运行期替换宿主（HTTP / WebSocket 两种接法切换用）。
func (p *Plugin) SetHost(h Host) {
	p.mu.Lock()
	p.host = h
	p.mu.Unlock()
}
