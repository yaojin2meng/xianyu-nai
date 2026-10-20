package nai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NAI ini 装载的回归测试。
// 节名/键名取自对 unpacked.dll 的“读配置项”调用点实测：
// 0x100A3EE1 key / 0x100A410C scale / 0x100A4A8B 完成后冷却 / 0x100A4C01 画图。

// ini 里的值必须覆盖默认值。
func TestLoadNAIConfigFromINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)
	body := strings.Join([]string{
		"[nai3设置]",
		"key=proxy-key",
		"官方key=official-key",
		"模式=官方",
		"",
		"[nai3参数]",
		"scale=7.5",
		"steps=40",
		"cfg=6.5",
		"strength=0.65",
		"默认种子=123456789",
		"宽度=832",
		"高度=1216",
		"采样=k_dpmpp_2m",
		"噪声=karras",
		"sm=1",
		"sm_dyn=1",
		"全局默认模型=nai-diffusion-4-full",
		"负面词条=低质量, 模糊",
		"完成后冷却=90",
		"完成冷却模式=群",
		"",
		"[重制版]",
		"nai3api=https://proxy.example.com",
		"naiofficialapi=https://image.novelai.net",
		"nai3_plugin-api=https://plugin.example.com",
		"资源地址=https://res.example.com",
		"upload-image=https://res.example.com/up",
	}, "\r\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadNAIConfigFromINI(path, DefaultConfig())

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"key", got.Key, "proxy-key"},
		{"官方key", got.OfficialKey, "official-key"},
		{"模式", got.Mode, "官方"},
		{"采样", got.Sampler, "k_dpmpp_2m"},
		{"噪声", got.Noise, "karras"},
		{"全局默认模型", got.Model, "nai-diffusion-4-full"},
		{"负面词条", got.Negative, "低质量, 模糊"},
		{"nai3api", got.ProxyURL, "https://proxy.example.com"},
		{"naiofficialapi", got.OfficialURL, "https://image.novelai.net"},
		{"nai3_plugin-api", got.PluginAPI, "https://plugin.example.com"},
		{"资源地址", got.ResourceURL, "https://res.example.com"},
		{"upload-image", got.UploadImage, "https://res.example.com/up"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q，应为 %q", c.name, c.got, c.want)
		}
	}
	if got.Steps != 40 {
		t.Errorf("steps = %d，应为 40", got.Steps)
	}
	if got.Width != 832 || got.Height != 1216 {
		t.Errorf("尺寸 = %dx%d，应为 832x1216", got.Width, got.Height)
	}
	if got.Scale != 7.5 || got.Cfg != 6.5 || got.Strength != 0.65 {
		t.Errorf("scale/cfg/strength = %v/%v/%v", got.Scale, got.Cfg, got.Strength)
	}
	if got.Seed != 123456789 {
		t.Errorf("默认种子 = %d，应为 123456789", got.Seed)
	}
	if !got.Smea || !got.Dyn {
		t.Errorf("sm/sm_dyn 应为 true：%v/%v", got.Smea, got.Dyn)
	}
}

// ini 不存在时不得改动传入配置（CLI 参数优先）。
func TestLoadNAIConfigMissingINI(t *testing.T) {
	base := DefaultConfig()
	base.Key = "from-cli"
	base.Steps = 55
	base.ProxyURL = "https://cli.example.com"

	got := LoadNAIConfigFromINI(filepath.Join(t.TempDir(), "不存在.ini"), base)
	if got.Key != "from-cli" || got.Steps != 55 || got.ProxyURL != "https://cli.example.com" {
		t.Errorf("ini 缺失时应保留 base：key=%q steps=%d proxy=%q", got.Key, got.Steps, got.ProxyURL)
	}
}

// ini 里缺少某个键时，该键保留 base 值。
func TestLoadNAIConfigPartialINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)
	if err := os.WriteFile(path, []byte("[nai3参数]\r\nsteps=12\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := DefaultConfig()
	base.Sampler = "dpmpp_2m"
	got := LoadNAIConfigFromINI(path, base)
	if got.Steps != 12 {
		t.Errorf("steps = %d，应为 12", got.Steps)
	}
	if got.Sampler != "dpmpp_2m" {
		t.Errorf("采样应保留 base 的 dpmpp_2m，实际 %q", got.Sampler)
	}
}

// 写回后必须落在 [nai3设置]/[nai3参数]/[重制版] 对应节下。
func TestSaveNAIConfigToINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)

	cfg := DefaultConfig()
	cfg.Key = "k1"
	cfg.OfficialKey = "k2"
	cfg.Mode = "官方"
	cfg.Steps = 44
	cfg.Sampler = "euler_a"
	cfg.ProxyURL = "https://proxy.example.com"
	cfg.ResourceURL = "https://res.example.com"

	if err := SaveNAIConfigToINI(path, cfg); err != nil {
		t.Fatalf("SaveNAIConfigToINI: %v", err)
	}

	f := LoadIni(path)
	checks := []struct{ sec, key, want string }{
		{"nai3设置", "key", "k1"},
		{"nai3设置", "官方key", "k2"},
		{"nai3设置", "模式", "官方"},
		{"nai3参数", "steps", "44"},
		{"nai3参数", "采样", "euler_a"},
		{"重制版", "nai3api", "https://proxy.example.com"},
		{"重制版", "资源地址", "https://res.example.com"},
	}
	for _, c := range checks {
		if got := f.Get(c.sec, c.key, ""); got != c.want {
			t.Errorf("%s/%s = %q，应为 %q", c.sec, c.key, got, c.want)
		}
	}
	// 默认值不应被写进 ini
	if f.Has("nai3参数", "负面词条") {
		t.Error("默认负面词条不应被写入 ini")
	}
}

// 插件初始化时必须从 ini 装载 NAI 配置。
func TestPluginApplyConfigLoadsNAIINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)
	body := "[nai3设置]\r\nkey=ini-key\r\n模式=官方\r\n" +
		"[nai3参数]\r\nsteps=37\r\n采样=k_euler_a\r\n" +
		"[重制版]\r\nnai3api=https://ini.example.com\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dir
	// CLI 没用任何参数，ini 应生效
	host := OneBotHost{C: &HTTPCaller{Base: "http://127.0.0.1:5700"}}
	p := NewPlugin(host, cfg)
	p.Init(cfg, nil)

	got := p.Config()
	if got.Key != "ini-key" {
		t.Errorf("key = %q，应为 ini-key", got.Key)
	}
	if got.Mode != "官方" {
		t.Errorf("模式 = %q，应为 官方", got.Mode)
	}
	if got.Steps != 37 {
		t.Errorf("steps = %d，应为 37", got.Steps)
	}
	if got.Sampler != "k_euler_a" {
		t.Errorf("采样 = %q，应为 k_euler_a", got.Sampler)
	}
	if got.ProxyURL != "https://ini.example.com" {
		t.Errorf("nai3api = %q", got.ProxyURL)
	}
}

// 指令词覆盖：ini 有值用 ini，无值用默认。
func TestNAICmdOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)
	if err := os.WriteFile(path, []byte("[nai3指令]\r\n画图=来张图\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := LoadIni(path)
	if got := NAICmdOverride(f, "画图", "画图"); got != "来张图" {
		t.Errorf("画图 = %q，应为 来张图", got)
	}
	if got := NAICmdOverride(f, "查看模型", "查看模型"); got != "查看模型" {
		t.Errorf("未配置的键应返回 fallback，实际 %q", got)
	}
	if got := NAICmdOverride(nil, "画图", "画图"); got != "画图" {
		t.Errorf("nil ini 应返回 fallback，实际 %q", got)
	}
}

// 面板改指令词后，插件必须用新词分派。
func TestUICmdOverrideTakesEffect(t *testing.T) {
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
	doc["nai3指令"]["画图"] = "来张图"
	if err := srv.WriteConfig(doc); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	// 新指令词应命中
	if c, matched, ok := MatchCommandWithIni("来张图 1girl", p.iniSnapshot()); !ok {
		t.Error("新指令词 来张图 未命中")
	} else if c.Kind != CmdDraw {
		t.Errorf("命中的 Kind = %q，应为 draw", c.Kind)
	} else if matched != "来张图" {
		t.Errorf("matched = %q，应为 来张图", matched)
	}

	// 旧主指令词仍可作为别名命中（画图是 Draw 的别名）
	if _, _, ok := MatchCommandWithIni("画图 1girl", p.iniSnapshot()); !ok {
		t.Error("别名 画图 应仍命中")
	}
}

// [基本]/[点数限制]/[点数消耗]/[图像审核]/[黑名单功能] 的装载测试。
func TestLoadHomeConfigFromINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)
	body := strings.Join([]string{
		"[基本]",
		"发送模式=卡片跳转",
		"AI翻译令牌=tok-1",
		"屏蔽词条=动物,血腥",
		"",
		"[类型]",
		"翻译接口=ai",
		"",
		"[点数限制]",
		"开关=1",
		"基础点数=120",
		"点数处理类型=个人",
		"点数不足提示=请充值",
		"",
		"[点数消耗]",
		"sd画图=3",
		"nai画图=5",
		"comfy画图=7",
		"",
		"[图像审核]",
		"平台=baidu",
		"开关=1",
		"阈值=0.72",
		"处理=模糊处理",
		"",
		"[黑名单功能]",
		"sd画图=sd画图",
		"nai画图=nai画图",
		"清晰术=清晰术",
		"去背景=去背景",
		"上传标签=上传标签",
		"上传预设=上传预设",
	}, "\r\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadHomeConfigFromINI(path, DefaultConfig())
	if got.SendMode != "卡片跳转" || got.AITranslateToken != "tok-1" || got.TranslateAPI != "ai" {
		t.Errorf("基本/类型装载错误：%q %q %q", got.SendMode, got.AITranslateToken, got.TranslateAPI)
	}
	if got.BlockWords != "动物,血腥" {
		t.Errorf("屏蔽词条 = %q", got.BlockWords)
	}
	if !got.PointsOn || got.BasePoints != 120 || got.PointsType != "个人" || got.PointsTip != "请充值" {
		t.Errorf("点数限制装载错误：on=%v base=%d type=%q tip=%q",
			got.PointsOn, got.BasePoints, got.PointsType, got.PointsTip)
	}
	if got.CostSD != 3 || got.CostNai != 5 || got.CostComfy != 7 {
		t.Errorf("点数消耗 = %d/%d/%d", got.CostSD, got.CostNai, got.CostComfy)
	}
	if !got.AuditOn || got.AuditPlat != "baidu" || got.AuditTh != "0.72" || got.AuditAct != "模糊处理" {
		t.Errorf("图像审核装载错误：on=%v plat=%q th=%q act=%q",
			got.AuditOn, got.AuditPlat, got.AuditTh, got.AuditAct)
	}
	if got.BLExemptNai != "nai画图" || got.BLExemptPreset != "上传预设" {
		t.Errorf("黑名单功能装载错误：%q %q", got.BLExemptNai, got.BLExemptPreset)
	}
}

// 门禁开关必须真正传到 pipeline。
func TestPluginAppliesGateFromINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileMainINI)
	body := "[点数限制]\r\n开关=1\r\n基础点数=50\r\n点数处理类型=个人\r\n" +
		"[点数消耗]\r\nnai画图=9\r\n" +
		"[图像审核]\r\n开关=1\r\n平台=baidu\r\n阈值=0.5\r\n处理=过滤发送\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dir
	host := OneBotHost{C: &HTTPCaller{Base: "http://127.0.0.1:5700"}}
	p := NewPlugin(host, cfg)
	p.Init(cfg, nil)

	g := p.pipe.Gate
	if !g.PointsOn {
		t.Error("积分开关应为开")
	}
	if g.PointsType != PointsPersonal {
		t.Errorf("PointsType = %q，应为 个人", g.PointsType)
	}
	if got := g.Points.Cost(cfg.Model); got != 9 {
		t.Errorf("单价 = %d，应为 9", got)
	}
	if !g.AuditOn || g.AuditPlat != PlatformBaidu || g.AuditThresh != "0.5" || g.AuditAction != string(ActionFilter) {
		t.Errorf("审核 = %v/%q/%q/%q", g.AuditOn, g.AuditPlat, g.AuditThresh, g.AuditAction)
	}
}
