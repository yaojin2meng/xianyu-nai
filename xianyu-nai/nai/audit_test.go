package nai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditThresholdIsInclusive(t *testing.T) {
	// 0x1002e4cb: fsub threshold; fcomp 0.0; JAE 判违规 → >= 判违规
	cases := []struct {
		score string
		want  string
	}{
		{"0.64", "违规"},
		{"0.5", "违规"},    // 边界：等于阈值也违规
		{"0.4999", "合规"}, // 刚低于阈值
	}
	for _, c := range cases {
		got := judgeNSFW(mustFloat(t, c.score), 0.5)
		if got != c.want {
			t.Fatalf("score=%s got=%s want=%s", c.score, got, c.want)
		}
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	var f float64
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAuditRequestPerPlatform(t *testing.T) {
	// api4ai 走 image（本地路径），baidu / baiduai 走 imgUrl
	if r := buildAuditRequest(PlatformAPI4AI, "/tmp/a.png", "https://x/a.png"); r.Image != "/tmp/a.png" || r.Img != "" {
		t.Fatalf("api4ai body wrong: %+v", r)
	}
	if r := buildAuditRequest(PlatformBaidu, "/tmp/a.png", "https://x/a.png"); r.Img != "https://x/a.png" || r.Image != "" {
		t.Fatalf("baidu body wrong: %+v", r)
	}
	if r := buildAuditRequest(PlatformBaiduAI, "", "https://x/a.png"); r.Type != "baiduai" || r.Img != "https://x/a.png" {
		t.Fatalf("baiduai body wrong: %+v", r)
	}
}

func TestAuditDefaultThreshold(t *testing.T) {
	if DefaultAuditThreshold != "0.5" {
		t.Fatal("threshold default changed (VA 0x102b8e60)")
	}
}

func TestPublishMode(t *testing.T) {
	if PublishMode("json") != SendJSON {
		t.Fatal("json")
	}
	if PublishMode("卡片跳转") != SendCard {
		t.Fatal("card")
	}
	if PublishMode("合并转发") != SendForward {
		t.Fatal("forward")
	}
	if PublishMode("随便什么") != SendImage {
		t.Fatal("default must be image")
	}
}

func TestAiurlForm(t *testing.T) {
	ct, body := AiurlForm("nai3api", `{"a":1}`)
	if ct != "application/x-www-form-urlencoded" {
		t.Fatal(ct)
	}
	if !strings.HasPrefix(body, "name=nai3api&data=") {
		t.Fatal(body)
	}
}

func TestDenoiseLevels(t *testing.T) {
	if denoiseLevels["二重奏"] != "2重奏" || denoiseLevels["四重奏"] != "4重奏" {
		t.Fatal("denoise map wrong")
	}
	if denoiseLevels["低级"] != "denoise1x" {
		t.Fatal("level1")
	}
}

func TestCaption(t *testing.T) {
	s := BuildCaption(ResultMeta{Seed: 42, Model: "nai-diffusion-3", Sampler: "k_euler", Scale: 5, Steps: 28, Width: 1024, Height: 1024}, "x.png")
	if !strings.HasPrefix(s, "nai画图\r\n") || !strings.Contains(s, "seed：42") {
		t.Fatal(s)
	}
}
