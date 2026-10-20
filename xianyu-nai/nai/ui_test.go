package nai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 配置面板的回归测试：schema 覆盖、ini 读写、HTTP 接口、热重载。

func newUITestServer(t *testing.T) (*UIServer, string) {
	t.Helper()
	dir := t.TempDir()
	srv := NewUIServer(UIOptions{Addr: "127.0.0.1:0", DataDir: dir}, nil)
	url, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv, url
}

// schema 必须覆盖 DLL 里实测到的全部 ini 节。
func TestUISchemaSections(t *testing.T) {
	want := []string{
		"基本", "类型", "指令", "点数限制", "点数消耗", "图像审核", "黑名单功能",
		"nai3设置", "nai3参数", "nai3指令",
		"sd设置", "sd参数", "sd指令",
		"comfy设置", "comfy参数", "comfy指令",
		"浏览器UI", "重制版",
	}
	got := map[string]UISection{}
	for _, s := range UISchema() {
		got[s.Name] = s
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("schema 缺少节 %q", w)
		}
	}
	// 每个节必须有字段，且键名不重复
	for name, s := range got {
		if len(s.Fields) == 0 {
			t.Errorf("节 %q 没有字段", name)
		}
		seen := map[string]bool{}
		for _, f := range s.Fields {
			if seen[f.Key] {
				t.Errorf("节 %q 键 %q 重复", name, f.Key)
			}
			seen[f.Key] = true
			switch f.Type {
			case "text", "number", "bool", "select", "password", "textarea":
			default:
				t.Errorf("节 %q 键 %q 类型非法：%q", name, f.Key, f.Type)
			}
		}
	}
}

// NAI 的三个节名/键名必须与 DLL 实测一致。
func TestUISchemaNAIKeys(t *testing.T) {
	secs := map[string]map[string]bool{}
	for _, s := range UISchema() {
		secs[s.Name] = map[string]bool{}
		for _, f := range s.Fields {
			secs[s.Name][f.Key] = true
		}
	}
	cases := []struct{ sec, key string }{
		{"nai3设置", "key"},
		{"nai3设置", "官方key"},
		{"nai3设置", "模式"},
		{"nai3参数", "scale"},
		{"nai3参数", "steps"},
		{"nai3参数", "cfg"},
		{"nai3参数", "strength"},
		{"nai3参数", "默认种子"},
		{"nai3参数", "采样"},
		{"nai3参数", "噪声"},
		{"nai3参数", "sm"},
		{"nai3参数", "sm_dyn"},
		{"nai3参数", "负面词条"},
		{"nai3参数", "完成后冷却"},
		{"nai3参数", "完成冷却模式"},
		{"nai3指令", "画图"},
		{"sd设置", "链接"},
		{"sd设置", "账号"},
		{"sd设置", "密码"},
		{"sd参数", "steps"},
		{"sd参数", "识别率"},
		{"sd参数", "差异"},
		{"sd参数", "高清修复算法"},
		{"comfy设置", "链接"},
		{"comfy设置", "Token"},
		{"comfy参数", "调度"},
		{"comfy指令", "画图"},
	}
	for _, c := range cases {
		if !secs[c.sec][c.key] {
			t.Errorf("schema 缺少 %s/%s", c.sec, c.key)
		}
	}
}

// 读取时，ini 里没有的项应回落到 schema 默认值。
func TestUIReadConfigDefaults(t *testing.T) {
	srv, _ := newUITestServer(t)
	doc, err := srv.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if got := doc["nai3参数"]["steps"]; got != "28" {
		t.Errorf("nai3参数/steps 默认应为 28，实际 %q", got)
	}
	if got := doc["nai3参数"]["采样"]; got != "k_euler" {
		t.Errorf("nai3参数/采样 默认应为 k_euler，实际 %q", got)
	}
	if got := doc["sd参数"]["steps"]; got != "30" {
		t.Errorf("sd参数/steps 默认应为 30，实际 %q", got)
	}
	if got := doc["comfy参数"]["steps"]; got != "20" {
		t.Errorf("comfy参数/steps 默认应为 20，实际 %q", got)
	}
	if got := doc["nai3指令"]["画图"]; got != "画图" {
		t.Errorf("nai3指令/画图 默认应为 画图，实际 %q", got)
	}
}

// 写入后必须真正落到 ini 的对应节下，且能读回。
func TestUIWriteConfigRoundTrip(t *testing.T) {
	srv, _ := newUITestServer(t)

	doc, err := srv.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	doc["nai3设置"]["模式"] = "官方"
	doc["nai3设置"]["key"] = "test-key-123"
	doc["nai3参数"]["steps"] = "33"
	doc["nai3参数"]["cfg"] = "6.5"
	doc["nai3参数"]["宽度"] = "832"
	doc["nai3参数"]["高度"] = "1216"
	doc["nai3参数"]["sm"] = "1"
	doc["nai3参数"]["负面词条"] = "lowres, blurry"
	doc["sd设置"]["链接"] = "http://127.0.0.1:7860"
	doc["sd设置"]["账号"] = "sduser"
	doc["sd参数"]["steps"] = "27"
	doc["sd参数"]["完成冷却模式"] = "群"
	doc["comfy设置"]["链接"] = "http://127.0.0.1:8188"
	doc["comfy设置"]["Token"] = "comfy-token"
	doc["comfy参数"]["steps"] = "24"
	doc["comfy参数"]["完成后冷却"] = "45"
	doc["基本"]["发送模式"] = "卡片跳转"

	if err := srv.WriteConfig(doc); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	back, err := srv.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig(2): %v", err)
	}
	checks := []struct{ sec, key, want string }{
		{"nai3设置", "模式", "官方"},
		{"nai3设置", "key", "test-key-123"},
		{"nai3参数", "steps", "33"},
		{"nai3参数", "cfg", "6.5"},
		{"nai3参数", "宽度", "832"},
		{"nai3参数", "高度", "1216"},
		{"nai3参数", "sm", "1"},
		{"sd设置", "链接", "http://127.0.0.1:7860"},
		{"sd参数", "steps", "27"},
		{"sd参数", "完成冷却模式", "群"},
		{"comfy设置", "Token", "comfy-token"},
		{"comfy参数", "steps", "24"},
		{"comfy参数", "完成后冷却", "45"},
		{"基本", "发送模式", "卡片跳转"},
	}
	for _, c := range checks {
		if got := back[c.sec][c.key]; got != c.want {
			t.Errorf("%s/%s 回读 %q，应为 %q", c.sec, c.key, got, c.want)
		}
	}

	// ini 文件里必须出现原插件的节名与键名
	raw, err := os.ReadFile(filepath.Join(srv.opt.DataDir, FileMainINI))
	if err != nil {
		t.Fatalf("读 ini: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"[nai3设置]",
		"key=test-key-123",
		"[nai3参数]",
		"steps=33",
		"[sd设置]",
		"链接=http://127.0.0.1:7860",
		"[sd参数]",
		"[comfy设置]",
		"[comfy参数]",
		"完成冷却模式=群",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ini 缺少 %q\n--- ini ---\n%s", want, text)
		}
	}
	// NAI 的键必须落在 [nai3设置] 节下，不能落到根节
	ini := LoadIni(filepath.Join(srv.opt.DataDir, FileMainINI))
	if got := ini.Get("nai3设置", "key", ""); got != "test-key-123" {
		t.Errorf("[nai3设置]/key 应为 test-key-123，实际 %q", got)
	}
	if got := ini.Get("", "key", ""); got != "" {
		t.Errorf("根节不应有 key，实际 %q", got)
	}
}

// 未知节/键必须被拒绝，防止面板误写。
func TestUIWriteRejectsUnknown(t *testing.T) {
	srv, _ := newUITestServer(t)

	if err := srv.WriteConfig(map[string]map[string]string{
		"不存在的节": {"a": "1"},
	}); err == nil {
		t.Error("未知节应报错")
	}
	if err := srv.WriteConfig(map[string]map[string]string{
		"sd设置": {"不存在的键": "1"},
	}); err == nil {
		t.Error("未知键应报错")
	}
}

// HTTP：GET schema、GET config、POST config、GET status、GET presets。
func TestUIHTTPEndpoints(t *testing.T) {
	srv, url := newUITestServer(t)

	// schema
	resp, err := http.Get(url + "api/schema")
	if err != nil {
		t.Fatalf("GET schema: %v", err)
	}
	var schema []UISection
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		t.Fatalf("schema 解码: %v", err)
	}
	resp.Body.Close()
	if len(schema) < 15 {
		t.Errorf("schema 节数 %d，应 >= 15", len(schema))
	}

	// config GET
	resp, err = http.Get(url + "api/config")
	if err != nil {
		t.Fatalf("GET config: %v", err)
	}
	var doc map[string]map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("config 解码: %v", err)
	}
	resp.Body.Close()

	// config POST
	doc["sd参数"]["steps"] = "41"
	body, _ := json.Marshal(doc)
	resp, err = http.Post(url+"api/config", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST config: %v", err)
	}
	var res struct {
		OK     bool                         `json:"ok"`
		Config map[string]map[string]string `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("POST 解码: %v", err)
	}
	resp.Body.Close()
	if !res.OK {
		t.Fatal("POST 应返回 ok=true")
	}
	if got := res.Config["sd参数"]["steps"]; got != "41" {
		t.Errorf("POST 回读 steps=%q，应为 41", got)
	}

	// status
	resp, err = http.Get(url + "api/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	var st map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("status 解码: %v", err)
	}
	resp.Body.Close()
	if st["ini_path"] == nil || st["ini_path"] == "" {
		t.Error("status 应返回 ini_path")
	}

	// presets
	resp, err = http.Get(url + "api/presets")
	if err != nil {
		t.Fatalf("GET presets: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("presets 状态 %d", resp.StatusCode)
	}
	resp.Body.Close()

	_ = srv
}

// 页面必须能取到，且包含导航与保存逻辑。
func TestUIIndexPage(t *testing.T) {
	_, url := newUITestServer(t)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	page := string(b)
	for _, want := range []string{
		"咸鱼ai画图", "/api/schema", "/api/config", "/api/presets",
		"保存并生效", "配置面板",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("页面缺少 %q", want)
		}
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type=%q", ct)
	}
}

// 预设接口应能列出数据目录里的文件。
func TestUIPresetsEndpoint(t *testing.T) {
	srv, url := newUITestServer(t)
	dir := srv.opt.DataDir

	if err := os.WriteFile(filepath.Join(dir, FileSdPresetJSON),
		[]byte(`[{"名":"预设A","内容":"1girl"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	wfDir := filepath.Join(dir, FileComfyWfDir)
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "测试流.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(url + "api/presets")
	if err != nil {
		t.Fatalf("GET presets: %v", err)
	}
	defer resp.Body.Close()
	var out map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解码: %v", err)
	}
	if len(out["sd_presets"]) != 1 || out["sd_presets"][0] != "预设A" {
		t.Errorf("sd_presets=%v", out["sd_presets"])
	}
	if len(out["comfy_workflows"]) != 1 || out["comfy_workflows"][0] != "测试流" {
		t.Errorf("comfy_workflows=%v", out["comfy_workflows"])
	}
}

// 保存后插件应重新装载（SD / Comfy 地址变化）。
func TestUIWriteTriggersReload(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	host := OneBotHost{C: &HTTPCaller{Base: "http://127.0.0.1:5700"}}
	p := NewPlugin(host, cfg)
	p.Init(cfg, nil)
	p.Enable()

	srv := NewUIServer(UIOptions{Addr: "127.0.0.1:0", DataDir: dir}, p)
	if _, err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	doc, err := srv.ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	doc["sd设置"]["链接"] = "http://127.0.0.1:9999"
	doc["comfy设置"]["链接"] = "http://127.0.0.1:8888"
	if err := srv.WriteConfig(doc); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	got := p.Config()
	if got.SD.URL != "http://127.0.0.1:9999" {
		t.Errorf("热重载后 sd.url=%q", got.SD.URL)
	}
	if got.Comfy.URL != "http://127.0.0.1:8888" {
		t.Errorf("热重载后 comfy.url=%q", got.Comfy.URL)
	}
}
