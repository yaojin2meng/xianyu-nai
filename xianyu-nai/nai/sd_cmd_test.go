package nai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildSdCaptionExact 锁定 SD 结果正文的 16 段拼接结果。
//
// 证据：0x10026495-0x100264DD / 0x10026682-0x100266C5。
func TestBuildSdCaptionExact(t *testing.T) {
	m := ResultMeta{
		Width:   896,
		Height:  1472,
		Seed:    987654321,
		Sampler: "Euler a",
		Model:   "m1",
		RawURL:  "http://x/raw.png",
		Prompt:  "1girl, solo",
	}
	got := BuildSdCaption(m, "http://x/img.png")
	want := "![sd #896px #1472px](http://x/img.png)\r\n" +
		"[🔗下载图片](http://x/raw.png)\r\n" +
		"```\r\n1girl, solo\r\n```\r\n" +
		"# seed:987654321\r\n# 采样方式:Euler a\r\n# 模型:m1"
	if got != want {
		t.Fatalf("BuildSdCaption mismatch\n got=%q\nwant=%q", got, want)
	}
}

// TestBuildSdCaptionEmptyURLs 说明 urls[0] 为空时下载链接就是一对空括号。
func TestBuildSdCaptionEmptyURLs(t *testing.T) {
	m := ResultMeta{Width: 512, Height: 512, Seed: -1, Sampler: "Euler a", Model: "m"}
	got := BuildSdCaption(m, "http://x/img.png")
	if !strings.Contains(got, "[🔗下载图片]()") {
		t.Fatalf("expected empty download link, got %q", got)
	}
}

// TestMatchSDCommandNoNaiHijack 确认 SD 指令只保留 sd 前缀主名，
// 不会把 nai 的 "画图" / "查看预设" 抢走。
func TestMatchSDCommandNoNaiHijack(t *testing.T) {
	for _, s := range []string{"画图 1girl", "查看预设", "修改种子 1", "查看负面", "查看模型"} {
		if c, w, ok := MatchSDCommand(s); ok {
			t.Fatalf("%q 被 SD 指令 %q(%q) 抢走", s, c.Kind, w)
		}
	}
	cases := map[string]SDCommandKind{
		"sd画图 1girl":    SdCmdDraw,
		"sd种子 123":      SdCmdSeed,
		"sd修改负面 lowres": SdCmdSetNegative,
		"sd查看负面":        SdCmdViewNegative,
		"sd查看模型":        SdCmdViewModel,
		"sd查看vae":       SdCmdViewVAE,
		"sd查看lora":      SdCmdViewLora,
		"sd查看预设":        SdCmdViewPreset,
		"sd上传预设 a=bb":   SdCmdUploadPreset,
		"sd删除预设 a":      SdCmdDeletePreset,
		"加载lora":        SdCmdLoadLora,
		"刷新lora":        SdCmdRefreshLora,
	}
	for text, kind := range cases {
		c, _, ok := MatchSDCommand(text)
		if !ok {
			t.Fatalf("%q 未匹配到任何 SD 指令", text)
		}
		if c.Kind != kind {
			t.Fatalf("%q 匹配到 %q，期望 %q", text, c.Kind, kind)
		}
	}
}

// TestSDUserStateSeedNegative 验证 seed.ini / Negative.ini 的每用户读写。
func TestSDUserStateSeedNegative(t *testing.T) {
	dir := t.TempDir()
	st := &SDUserState{Dir: dir}

	if _, err := os.Stat(filepath.Join(dir, FileSeedINI)); err == nil {
		t.Fatal("seed.ini 不应在写入前存在")
	}
	if err := st.SetSeed("10001", "-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSeed("10002", "42"); err != nil {
		t.Fatal(err)
	}
	if got := st.Seed("10001", "7"); got != "-1" {
		t.Fatalf("seed 10001 = %q, want -1", got)
	}
	if got := st.Seed("10002", "7"); got != "42" {
		t.Fatalf("seed 10002 = %q, want 42", got)
	}
	if got := st.Seed("10003", "7"); got != "7" {
		t.Fatalf("未设置用户应回退默认值，got %q", got)
	}

	if err := st.SetNegative("10001", "lowres, bad hands"); err != nil {
		t.Fatal(err)
	}
	if !st.HasNegative("10001") {
		t.Fatal("10001 应已有负面词条")
	}
	if st.HasNegative("10002") {
		t.Fatal("10002 不应有负面词条")
	}
	if got := st.Negative("10001", "dflt"); got != "lowres, bad hands" {
		t.Fatalf("negative = %q", got)
	}
	if got := st.Negative("10002", "dflt"); got != "dflt" {
		t.Fatalf("未设置用户应回退默认值，got %q", got)
	}
}

// TestSplitPresetArg 按 "=" 切 " 我的预设=预设内容"（0x102B613C）。
func TestSplitPresetArg(t *testing.T) {
	n, c := splitPresetArg(" 我的预设=预设内容 ")
	if n != "我的预设" || c != "预设内容" {
		t.Fatalf("got (%q,%q)", n, c)
	}
	if n, c := splitPresetArg("只有名字"); n != "只有名字" || c != "" {
		t.Fatalf("无等号时 got (%q,%q)", n, c)
	}
}

// TestSDPresetRender 锁定预设列表 / 单条 / 纯文本三种拼接。
func TestSDPresetRender(t *testing.T) {
	list := SDPresetFile{
		{Name: "a", Content: "aaa"},
		{Name: "b", Content: "bbb"},
	}
	if got, want := list.Render(),
		"# 预设列表(指令+预设名查看提示词)\r\n```\r\na\r\nb\r\n\r\n```"; got != want {
		t.Fatalf("Render\n got=%q\nwant=%q", got, want)
	}
	if got, want := RenderSDPresetDetail(list[1]),
		"# b\r\n\r\n```\r\nbbb\r\n```\r\n\r\n"; got != want {
		t.Fatalf("RenderSDPresetDetail\n got=%q\nwant=%q", got, want)
	}
	if got, want := list.RenderPlain(),
		"预设列表(指令+预设名查看提示词)\r\n==========\r\n"+
			"预设名a\r\n预设内容=\r\naaa\r\n\r\n"+
			"预设名b\r\n预设内容=\r\nbbb\r\n\r\n"; got != want {
		t.Fatalf("RenderPlain\n got=%q\nwant=%q", got, want)
	}
}

// TestSDPresetUploadDelete 重名与删除的回执。
func TestSDPresetUploadDelete(t *testing.T) {
	var list SDPresetFile
	if got := list.Upload("a", "aaa"); got != MsgSdPresetUploaded {
		t.Fatalf("首次上传 got %q", got)
	}
	if got := list.Upload("a", "zzz"); got != MsgSdPresetExists {
		t.Fatalf("重名上传 got %q", got)
	}
	if it, ok := list.Find("a"); !ok || it.Content != "aaa" {
		t.Fatalf("Find 得到 %v %v", it, ok)
	}
	if got := list.Delete("nope"); got != MsgSdPresetNotFound {
		t.Fatalf("删不存在 got %q", got)
	}
	if got := list.Delete("a"); got != MsgSdPresetDeleted {
		t.Fatalf("删除 got %q", got)
	}
	if len(list) != 0 {
		t.Fatalf("删除后剩余 %d 条", len(list))
	}
}

// TestSdCooldownSeconds 解析 "完成后冷却"。
func TestSdCooldownSeconds(t *testing.T) {
	cases := map[string]int{"": 0, "0": 0, "60": 60, " 30 ": 30, "-5": 0, "abc": 0}
	for raw, want := range cases {
		if got := SdCooldownSeconds(raw); got != want {
			t.Fatalf("SdCooldownSeconds(%q) = %d, want %d", raw, got, want)
		}
	}
}

// TestSDPresetFileRoundTrip 预设落盘后再读回。
func TestSDPresetFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileSdPresetJSON)
	list := SDPresetFile{{Name: "风格", Content: "masterpiece"}}
	if err := saveSDPresets(path, list); err != nil {
		t.Fatal(err)
	}
	back := LoadSDPresets(path)
	if len(back) != 1 || back[0].Name != "风格" || back[0].Content != "masterpiece" {
		t.Fatalf("读回 %+v", back)
	}
}

// TestLoadSDConfigFromINI 确认 [sd设置]/[sd参数] 的键名与默认值。
func TestLoadSDConfigFromINI(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileMainINI)
	body := "[sd设置]\r\n链接=http://127.0.0.1:7860\r\n账号=u\r\n密码=p\r\n" +
		"[sd参数]\r\nsteps=12\r\n采样=DPM++ 2M Karras\r\n宽度=640\r\n高度=960\r\n" +
		"负面词条=x\r\n高清修复=1\r\n默认种子=99\r\n全局默认模型=mm\r\n完成后冷却=7\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := LoadSDConfigFromINI(path, DefaultSDConfig())
	if cfg.URL != "http://127.0.0.1:7860" || cfg.User != "u" || cfg.Pass != "p" {
		t.Fatalf("sd设置 未读到: %+v", cfg)
	}
	if cfg.Steps != 12 || cfg.Sampler != "DPM++ 2M Karras" || cfg.Width != 640 ||
		cfg.Height != 960 || cfg.Negative != "x" || !cfg.Hires || cfg.Seed != "99" ||
		cfg.GlobalModel != "mm" || cfg.Cooldown != "7" {
		t.Fatalf("sd参数 未读到: %+v", cfg)
	}
}
