package nai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProxyBodyMatchesDLLTemplate(t *testing.T) {
	cfg := DefaultConfig()
	c := New(cfg)
	body := c.BuildProxyRequest("1girl", "bad")
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"nai-diffusion-3","positivePrompt":"1girl","negativePrompt":"bad","scale":5,"steps":28,"width":1024,"height":1024,"promptGuidanceRescale":0.28,"noise_schedule":"native","seed":"-1","sampler":"k_euler","sm":false,"sm_dyn":false}`
	if string(raw) != want {
		t.Fatalf("proxy body mismatch\n got: %s\nwant: %s", raw, want)
	}
}

func TestOfficialTxt2Img(t *testing.T) {
	c := New(DefaultConfig())
	r := c.BuildOfficialRequest("p", "n", "")
	if r.Action != "generate" {
		t.Fatalf("action=%s", r.Action)
	}
	p := r.Parameters
	if p.Width != "1024" || p.Height != "1024" || p.Steps != "28" {
		t.Fatalf("dims must be strings: %s %s %s", p.Width, p.Height, p.Steps)
	}
	if p.ParamsVersion != 3 || !p.QualityToggle || p.UCPreset != 0 || p.N != 1 {
		t.Fatalf("scalar defaults wrong: %+v", p)
	}
	if !p.AddOriginal || !p.V4Prompt.UseOrder || p.V4Prompt.UseCoords {
		t.Fatalf("v4 defaults wrong: %+v", p)
	}
	if p.V4Prompt.Caption.BaseCaption != "p" {
		t.Fatalf("base_caption=%s", p.V4Prompt.Caption.BaseCaption)
	}
}

func TestOfficialImg2Img(t *testing.T) {
	c := New(DefaultConfig())
	r := c.BuildOfficialRequest("p", "n", "BASE64")
	if r.Action != "img2img" {
		t.Fatalf("action=%s", r.Action)
	}
	if r.Parameters.AddOriginal {
		t.Fatal("img2img must reset add_original_image to false (0x10064A03)")
	}
	if r.Parameters.Image != "BASE64" {
		t.Fatalf("image=%q", r.Parameters.Image)
	}
}

func TestAugmentBody(t *testing.T) {
	c := New(DefaultConfig())
	r := c.BuildAugmentRequest(AugmentColorize, "IMG", "")
	raw, _ := json.Marshal(r)
	want := `{"req_type":"colorize","image":"IMG","width":1024,"height":1024,"prompt":"","defry":0}`
	if string(raw) != want {
		t.Fatalf("augment mismatch\n got: %s\nwant: %s", raw, want)
	}
}

func TestJoinURL(t *testing.T) {
	if got := joinURL("https://image.novelai.net/", "/ai/generate-image", true); got != "https://image.novelai.net/ai/generate-image" {
		t.Fatal(got)
	}
	if got := joinURL("https://image.novelai.net", "/ai/generate-image", true); got != "https://image.novelai.net/ai/generate-image" {
		t.Fatal(got)
	}
	// 反代通道不做末尾斜杠处理
	if got := joinURL("https://x/api", "/api/generate_image", false); got != "https://x/api/api/generate_image" {
		t.Fatal(got)
	}
}

func TestPresetFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "nai预设列表.json")
	if err := os.WriteFile(f, []byte(`[{"名":"分辨率","内容":"masterpiece"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(f)
	p, err := ParsePresetFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := p.Find("分辨率"); !ok || v.Content != "masterpiece" {
		t.Fatal("preset lookup failed")
	}
	_ = p.Upsert(Preset{Name: "分辨率", Content: "best"})
	if v, _ := p.Find("分辨率"); v.Content != "best" {
		t.Fatal("upsert failed")
	}
	if !p.Delete("分辨率") {
		t.Fatal("delete failed")
	}
}

func TestEmotionTable(t *testing.T) {
	if v, ok := EmotionTag("开心"); !ok || v != "happy" {
		t.Fatal(v, ok)
	}
	if v, ok := EmotionTag("恼火"); !ok || v != "lrritated" {
		t.Fatal(v, ok)
	}
}
