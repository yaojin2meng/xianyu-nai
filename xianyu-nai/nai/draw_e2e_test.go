package nai

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// 本文件是"官方通道出图 -> 补图片直链 -> 卡片投递"的端到端测试。
//
// 覆盖的三处还原：
//  1. 官方 /ai/generate-image 的请求体（字段类型逐条来自 DLL）；
//  2. 0x1005F0C2-0x1005F100 的分支：响应无 image_url 时把图片传到上传接口换直链；
//  3. 0x100267F2 / 0x10030E18 的卡片填充：info[1].button = ",button=" + base64(UTF-8(按钮串))，按钮串里带该直链。
//
// 注：data.button（0x102b8f84）在 nai 通道并未被写（只出现在别的卡片类型），此处不写。

// ---------- 假 PNG / 假 zip ----------

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 11), 0x80, 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码测试图失败：%v", err)
	}
	return buf.Bytes()
}

func testZip(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, err := zw.Create(name)
	if err != nil {
		t.Fatalf("建 zip 失败：%v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("写 zip 失败：%v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关 zip 失败：%v", err)
	}
	return buf.Bytes()
}

// ---------- 假服务端：NAi 官方 + 资源 + OneBot ----------

type drawServer struct {
	*httptest.Server

	mu sync.Mutex

	// 官方出图
	generateRaw  []byte
	generateHits int

	// 上传接口
	uploadHits    int
	uploadBody    string
	uploadCT      string
	uploadAuth    string
	uploadedPixel []byte // 解出来的上传内容（用来确认"上传的就是刚出的图"）

	// 资源端
	cardJSON string

	// OneBot
	onebotCalls []map[string]interface{}

	// 出图返回的图片（默认 8x8 的一格，打成一个 zip 的 base64）
	imageBase64 string

	// 反代通道响应里的 image_url（空串时插件会走上传换直链）
	proxyImageURL string
}

func newDrawServer(t *testing.T) *drawServer {
	t.Helper()
	s := &drawServer{
		cardJSON: `{"richtexts":[{"filename":"","info":[{"content":{"data":{"content":[{}]}}},{"content":{"data":{"content":[{}]},"button":""}}]}]}`,
	}
	s.imageBase64 = base64.StdEncoding.EncodeToString(testZip(t, "image.png", testPNG(t, 8, 8)))

	mux := http.NewServeMux()
	mux.HandleFunc("/ai/generate-image", func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAll(r)
		s.mu.Lock()
		s.generateRaw = body
		s.generateHits++
		b64 := s.imageBase64
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{
			"images": []map[string]interface{}{{"image": b64, "seed": 4242}},
		})
	})
	mux.HandleFunc("/api/generate_image", func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAll(r)
		s.mu.Lock()
		s.generateRaw = body
		s.generateHits++
		b64, url := s.imageBase64, s.proxyImageURL
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{
			"info":      map[string]interface{}{"seed": 4242, "positivePrompt": "1girl"},
			"images":    []string{b64},
			"image_url": url,
		})
	})
	mux.HandleFunc("/plug/AI/upload-image", func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAll(r)
		s.mu.Lock()
		s.uploadHits++
		s.uploadBody = string(body)
		s.uploadCT = r.Header.Get("Content-Type")
		s.uploadAuth = r.Header.Get("Authorization")
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{"imageUrl": "https://img.example/nai-4242.png"},
		})
	})
	mux.HandleFunc("/plug/AI/nai3json.json", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, _ = w.Write([]byte(s.cardJSON))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAll(r)
		var params map[string]interface{}
		_ = json.Unmarshal(body, &params)
		s.mu.Lock()
		s.onebotCalls = append(s.onebotCalls, map[string]interface{}{
			"action": strings.TrimPrefix(r.URL.Path, "/"),
			"params": params,
		})
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{"status": "ok", "retcode": 0})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Server.Close)
	return s
}

func readAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *drawServer) snapshot() (generate []byte, uploads int, uploadBody, uploadCT, uploadAuth string, calls []map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generateRaw, s.uploadHits, s.uploadBody, s.uploadCT, s.uploadAuth, append([]map[string]interface{}(nil), s.onebotCalls...)
}

// ---------- 端到端 ----------

// TestOfficialDrawToCardEndToEnd 走一遍官方通道出图并投递卡片。
func TestOfficialDrawToCardEndToEnd(t *testing.T) {
	srv := newDrawServer(t)

	cfg := DefaultConfig()
	cfg.Mode = "官方"
	cfg.OfficialURL = srv.URL
	cfg.OfficialKey = "test-key"
	cfg.ResourceURL = srv.URL
	cfg.SendMode = string(SendCard)
	cfg.Seed = -1

	fc := &fakeCaller{}
	p := NewPlugin(OneBotHost{C: fc}, cfg)

	tmpl, err := FetchCardTemplate(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("拉卡片模板失败：%v", err)
	}
	p.SetCardTemplate(tmpl)
	p.Init(cfg, nil)
	p.Enable()

	ev := Event{
		MessageType: "group",
		SelfID:      10001,
		UserID:      66666,
		GroupID:     88888,
		Message:     []Seg{TextSeg("nai3指令 1girl, solo")},
	}
	replies := p.OnEventNT(ev)
	if len(replies) == 0 {
		t.Fatal("官方通道出图未产生回复")
	}
	p.deliver(ev, replies)

	generateRaw, uploads, uploadBody, uploadCT, _, _ := srv.snapshot()

	// 1) 官方请求体
	var req map[string]interface{}
	if err := json.Unmarshal(generateRaw, &req); err != nil {
		t.Fatalf("官方请求体不是 JSON：%v（%s）", err, generateRaw)
	}
	for k, want := range map[string]string{
		"action": "generate",
	} {
		if got, _ := req[k].(string); got != want {
			t.Fatalf("官方请求体 %s=%#v，想要 %q", k, req[k], want)
		}
	}
	params, ok := req["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("官方请求体没有 parameters：%s", generateRaw)
	}
	// width / height / steps 在原生实现里都是字符串
	for k, want := range map[string]string{"width": "1024", "height": "1024", "steps": "28"} {
		if got, _ := params[k].(string); got != want {
			t.Fatalf("parameters.%s=%#v（应是字符串 %q）", k, params[k], want)
		}
	}
	if v, _ := params["n_samples"].(float64); v != 1 {
		t.Fatalf("parameters.n_samples=%#v", params["n_samples"])
	}
	if v, _ := req["model"].(string); v != "nai-diffusion-3" {
		t.Fatalf("请求体 model=%#v", req["model"])
	}
	if v, _ := params["params_version"].(float64); v != 3 {
		t.Fatalf("parameters.params_version=%#v", params["params_version"])
	}

	// 2) 官方响应没有 image_url -> 必须走上传接口换直链
	if uploads != 1 {
		t.Fatalf("上传接口调用次数=%d，想要 1（官方通道没有 image_url）", uploads)
	}
	if uploadCT != "multipart/form-data; boundary=WebKitFormBoundary" {
		t.Fatalf("上传 Content-Type=%q（原生是写死的 boundary）", uploadCT)
	}
	if !strings.HasPrefix(uploadBody, "--WebKitFormBoundary") {
		t.Fatalf("上传体不是以固定界开头：%.60q", uploadBody)
	}
	if n := strings.Count(uploadBody, "--WebKitFormBoundary"); n != 2 {
		t.Fatalf("界出现 %d 次，想要 2 次（只有 file 一个字段）：%s", n, uploadBody)
	}
	if !strings.Contains(uploadBody, `name="file"`) {
		t.Fatalf("上传体里没有 file 字段：%s", uploadBody)
	}
	if !bytes.Contains([]byte(uploadBody), []byte("\x89PNG")) {
		t.Fatal("上传体里没有刚出的 PNG 字节")
	}

	// 3) 卡片投递：info[1].button = ",button=" + base64(UTF-8(按钮串))，按钮串里带直链
	if len(fc.calls) == 0 {
		t.Fatal("OneBot 侧没有收到任何调用")
	}
	call := fc.calls[0]
	if call.Action != "send_group_msg" {
		t.Fatalf("动作=%s，想要 send_group_msg", call.Action)
	}
	segs, ok := call.Params["message"].([]Seg)
	if !ok || len(segs) == 0 || segs[0].Type != "json" {
		t.Fatalf("投递消息段=%#v，想要 json 卡片段", call.Params["message"])
	}
	card, _ := segs[0].Data["data"].(string)
	if card == "" {
		t.Fatal("卡片内容为空")
	}
	var root interface{}
	dec := json.NewDecoder(strings.NewReader(card))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		t.Fatalf("卡片不是 JSON：%v（%s）", err, card)
	}
	for _, path := range CardFieldPath {
		if _, ok := getPath(root, path); !ok {
			t.Fatalf("卡片缺字段 %s：%s", path, card)
		}
	}
	btn2, _ := getPath(root, "richtexts[0].info[1].button")
	s2, _ := btn2.(string)
	if !strings.HasPrefix(s2, CardButtonPrefix) {
		t.Fatalf("info[1].button 少了 %q 前缀：%#v", CardButtonPrefix, btn2)
	}
	rawBtn, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s2, CardButtonPrefix))
	if err != nil {
		t.Fatalf("info[1].button 不是 base64：%v（%q）", err, s2)
	}
	if !strings.Contains(string(rawBtn), "https://img.example/nai-4242.png") {
		t.Fatalf("info[1].button 解出的按钮串没带上直链：%q", rawBtn)
	}
	if !strings.HasPrefix(string(rawBtn), `[[{"type":0,"name":"下载图片"`) {
		t.Fatalf("按钮串开头不对：%q", rawBtn)
	}
	if v, _ := getPath(root, "richtexts[0].info[0].content.data.content[0].data.content"); v != "1girl, solo" {
		t.Fatalf("用户节点正文=%#v", v)
	}
	if v, _ := getPath(root, "richtexts[0].filename"); v != CardFileName {
		t.Fatalf("filename=%#v", v)
	}
}

// TestProxyDrawSkipsUploadWhenImageURLPresent 反代通道给了 image_url 时不再上传
// （复刻 0x1005F0C2 的比较：非空就沿用，跳到 0x1005F2FB）。
func TestProxyDrawSkipsUploadWhenImageURLPresent(t *testing.T) {
	srv := newDrawServer(t)
	srv.mu.Lock()
	srv.cardJSON = `{}`
	srv.proxyImageURL = "https://img.example/proxy.png"
	srv.mu.Unlock()

	cfg := DefaultConfig()
	cfg.ProxyURL = srv.URL
	cfg.Key = "proxy-key"
	cfg.ResourceURL = srv.URL
	cfg.SendMode = string(SendLink)

	fc := &fakeCaller{}
	p := NewPlugin(OneBotHost{C: fc}, cfg)
	p.Init(cfg, nil)
	p.Enable()

	got := p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai3指令 1girl")}})
	if len(got) == 0 {
		t.Fatal("反代通道出图未产生回复")
	}
	if _, uploads, _, _, _, _ := srv.snapshot(); uploads != 0 {
		t.Fatalf("有 image_url 时不该上传，却调用了 %d 次", uploads)
	}
	if !strings.Contains(got[0].Text, "https://img.example/proxy.png") {
		t.Fatalf("正文没用响应里的 image_url：%q", got[0].Text)
	}
}

// TestProxyDrawUploadsWhenImageURLEmpty 反代通道没给 image_url 时同样要上传换直链。
func TestProxyDrawUploadsWhenImageURLEmpty(t *testing.T) {
	srv := newDrawServer(t)
	srv.mu.Lock()
	srv.cardJSON = `{}`
	srv.proxyImageURL = ""
	srv.mu.Unlock()

	cfg := DefaultConfig()
	cfg.ProxyURL = srv.URL
	cfg.Key = "proxy-key"
	cfg.ResourceURL = srv.URL
	cfg.SendMode = string(SendLink)

	fc := &fakeCaller{}
	p := NewPlugin(OneBotHost{C: fc}, cfg)
	p.Init(cfg, nil)
	p.Enable()

	got := p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai3指令 1girl")}})
	if len(got) == 0 {
		t.Fatal("反代通道出图未产生回复")
	}
	if _, uploads, _, _, _, _ := srv.snapshot(); uploads != 1 {
		t.Fatalf("没有 image_url 时应上传 1 次，实际 %d 次", uploads)
	}
	if !strings.Contains(got[0].Text, "https://img.example/nai-4242.png") {
		t.Fatalf("正文没用上传得到的直链：%q", got[0].Text)
	}
}

// ---------- 上传接口本身 ----------

func TestUploadImageEndpointResolution(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"资源地址兜底", Config{ResourceURL: "https://res.xianyuww.cn/"}, "https://res.xianyuww.cn/plug/AI/upload-image"},
		{"配置项直用", Config{ResourceURL: "https://res.example", UploadImage: "https://api.example/up"}, "https://api.example/up"},
		{"INI 默认 0 视为未配置", Config{ResourceURL: "https://res.example", UploadImage: "0"}, "https://res.example/plug/AI/upload-image"},
		{"都没配", Config{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(tc.cfg)
			if got := c.UploadImageEndpoint(); got != tc.want {
				t.Fatalf("UploadImageEndpoint()=%q，想要 %q", got, tc.want)
			}
		})
	}
}

func TestUploadImagePostsMultipartAndReadsImageURL(t *testing.T) {
	var gotField, gotCT string
	var gotPNG bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAll(r)
		gotField = string(body)
		gotCT = r.Header.Get("Content-Type")
		gotPNG = bytes.Contains(body, []byte("\x89PNG"))
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"imageUrl": "https://img.example/a.png"}})
	}))
	defer srv.Close()

	c := New(Config{ResourceURL: srv.URL, Key: "k"})
	u, err := c.EnsureImageURL(context.Background(), testPNG(t, 4, 4), "")
	if err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	if u != "https://img.example/a.png" {
		t.Fatalf("返回直链=%q", u)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data; boundary=WebKitFormBoundary") {
		t.Fatalf("Content-Type=%q", gotCT)
	}
	if !strings.Contains(gotField, `name="file"`) || !gotPNG {
		t.Fatalf("multipart 体不对：%s", gotField)
	}
}
