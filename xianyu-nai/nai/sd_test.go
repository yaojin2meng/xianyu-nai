package nai

import (
	"strings"
	"testing"
)

// SD 通道的单元测试：请求体键序/取值类型、鉴权头、高清修复闸门、
// 模型/VAE 覆盖闸门、响应解析与模型列表格式。

func TestSDAuthHeader(t *testing.T) {
	c := SDConfig{User: "u", Pass: "p"}
	if got, want := c.SDAuthHeader(), "Basic dTpw"; got != want {
		t.Fatalf("鉴权头=%q，期望 %q", got, want)
	}
	// 空账号密码也照写（DLL 无条件拼 "账号:密码"）。
	c2 := SDConfig{}
	if got, want := c2.SDAuthHeader(), "Basic Og=="; got != want {
		t.Fatalf("空鉴权头=%q，期望 %q", got, want)
	}
}

func TestBuildSDTxt2ImgDefaultOrder(t *testing.T) {
	c := DefaultSDConfig()
	body, err := BuildSDTxt2Img(c, SDOptions{
		Prompt:   "1girl",
		Negative: "lowres",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"prompt":"1girl","seed":-1,"steps":30,"cfg_scale":4,"width":896,` +
		`"height":1472,"negative_prompt":"lowres","sampler_index":"Euler a",` +
		`"denoising_strength":0.5}`
	if body != want {
		t.Fatalf("请求体不符\n实际: %s\n期望: %s", body, want)
	}
}

func TestBuildSDTxt2ImgHiresAndOverride(t *testing.T) {
	c := DefaultSDConfig()
	c.Hires = true
	c.HiresUp = "Lanczos"
	body, err := BuildSDTxt2Img(c, SDOptions{
		Prompt:   "1girl",
		Negative: "low",
		Model:    "m1",
		VAE:      "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"prompt":"1girl","seed":-1,"steps":30,"cfg_scale":4,"width":896,` +
		`"height":1472,"negative_prompt":"low","sampler_index":"Euler a",` +
		`"denoising_strength":0.5,"enable_hr":true,"hr_scale":2,` +
		`"hr_upscaler":"Lanczos","hr_second_pass_steps":10,` +
		`"override_settings":{"sd_model_checkpoint":"m1","sd_vae":"v1"}}`
	if body != want {
		t.Fatalf("请求体不符\n实际: %s\n期望: %s", body, want)
	}
}

func TestBuildSDOverrideZeroSkipped(t *testing.T) {
	c := DefaultSDConfig()
	body, err := BuildSDTxt2Img(c, SDOptions{Prompt: "x", Negative: "y", Model: "0", VAE: ""})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "override_settings") {
		t.Fatalf("模型/VAE 为 0 或空时不应写 override_settings：%s", body)
	}
}

func TestBuildSDImg2ImgOrder(t *testing.T) {
	c := DefaultSDConfig()
	body, err := BuildSDImg2Img(c, SDOptions{
		Prompt:    "1girl",
		Negative:  "low",
		InitImage: "QUJD",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 注意：img2img 模板里 denoising_strength 本来就在 sampler_index 之前，
	// DLL 用"按键置值"更新原位置，因此输出保持模板顺序。
	want := `{"init_images":["QUJD"],"prompt":"1girl","seed":-1,"steps":30,` +
		`"cfg_scale":4,"width":896,"height":1472,"negative_prompt":"low",` +
		`"denoising_strength":0.5,"sampler_index":"Euler a"}`
	if body != want {
		t.Fatalf("请求体不符\n实际: %s\n期望: %s", body, want)
	}
}

func TestBuildSDImg2ImgHiresFallback(t *testing.T) {
	c := DefaultSDConfig()
	c.Hires = true
	c.HiresUp = "" // 空值 -> img2img 回退 4x-UltraSharp
	body, err := BuildSDImg2Img(c, SDOptions{Prompt: "p", Negative: "n", InitImage: "QQ=="})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"hr_upscaler":"4x-UltraSharp"`) {
		t.Fatalf("img2img 高清修复算法空值应回退 4x-UltraSharp：%s", body)
	}
}

func TestParseSDResponse(t *testing.T) {
	raw := []byte(`{"images":["QUJD"],"info":"{\"prompt\":\"1girl\",\"seed\":123,\"sd_model_name\":\"m\",\"sampler_name\":\"Euler a\"}","parameters":{}}`)
	res, err := ParseSDResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || res.Images[0] != "QUJD" {
		t.Fatalf("images=%v", res.Images)
	}
	if res.Seed != 123 || res.Model != "m" || res.Sampler != "Euler a" || res.Prompt != "1girl" {
		t.Fatalf("info 解析错误: %+v", res)
	}
}

func TestParseSDResponseURLOnly(t *testing.T) {
	res, err := ParseSDResponse([]byte(`{"images":[],"urls":["https://x/1.png"],"info":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ImageURL != "https://x/1.png" {
		t.Fatalf("urls[0] 未取到: %+v", res)
	}
}

func TestParseSDResponseErrors(t *testing.T) {
	if _, err := ParseSDResponse(nil); err == nil || err.Error() != MsgSDEmptyResponse {
		t.Fatalf("空响应错误=%v", err)
	}
	if _, err := ParseSDResponse([]byte("not json")); err == nil ||
		!strings.HasPrefix(err.Error(), MsgSDErrorPrefix) {
		t.Fatalf("非 JSON 错误=%v", err)
	}
	if _, err := ParseSDResponse([]byte(`{"images":[],"urls":[]}`)); err == nil ||
		err.Error() != MsgSDDrawFailed {
		t.Fatalf("无图错误=%v", err)
	}
}

func TestRenderSDModels(t *testing.T) {
	got := RenderSDModels([]SDModel{
		{ModelName: "a", Hash: "h1"},
		{ModelName: "b", Hash: "h2"},
	})
	want := "===【sd模型列表】===\r\n" +
		"| 序号 | 模型名 | hash |\r\n" +
		"| ----- | ----- | ----- |\r\n" +
		"| 1 | a | h1 |\r\n" +
		"| 2 | b | h2 |\r\n"
	if got != want {
		t.Fatalf("模型列表不符\n实际: %q\n期望: %q", got, want)
	}
}

func TestSDConfigDefaultsKeepSeedZero(t *testing.T) {
	c := SDConfig{Seed: "0"}.withDefaults()
	if c.Seed != "0" {
		t.Fatalf("显式种子 0 被改写为 %q", c.Seed)
	}
	c2 := SDConfig{}.withDefaults()
	if c2.Seed != "-1" || c2.Steps != 30 || c2.Width != 896 || c2.Height != 1472 ||
		c2.Sampler != "Euler a" || c2.HiresUp != "Lanczos" {
		t.Fatalf("默认值不全: %+v", c2)
	}
}
