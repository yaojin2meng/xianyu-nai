package nai

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestConfuseRoundTrip(t *testing.T) {
	w, h := 37, 23
	src := make([]byte, w*h*4)
	for i := range src {
		src[i] = byte(i*7 + 13)
	}
	enc, err := Confuse(src, w, h, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, src) {
		t.Fatal("混淆后与原图相同，重排没生效")
	}
	dec, err := Confuse(enc, w, h, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, src) {
		t.Fatal("混淆后再解混淆没有还原")
	}
}

// 曲线位移量 K 的实测口径：K = trunc(((sqrt(5)-1)/2)*n)。
func TestConfuseShift(t *testing.T) {
	cases := []struct{ n, k int }{
		{0, 0}, {1, 0}, {2, 1}, {3, 1}, {4, 2}, {8, 4}, {10, 6},
		{100, 61}, {851, 525}, {1048576, 648055}, {16777216, 10368889},
	}
	for _, c := range cases {
		if got := ConfuseShift(c.n); got != c.k {
			t.Errorf("ConfuseShift(%d)=%d want %d", c.n, got, c.k)
		}
	}
}

// 逐像素核对反汇编公式：
//
//	混淆  ：out[order[i+K]] = in[order[i]]
//	解混淆：out[order[i]]   = in[order[i+K]]
func TestConfuseFollowsCurveShift(t *testing.T) {
	w, h := 13, 9
	n := w * h
	order := CurveOrder(w, h)
	k := ConfuseShift(n)
	src := make([]byte, n*4)
	for i := 0; i < n; i++ {
		src[i*4] = byte(i)
		src[i*4+1] = byte(i >> 8)
		src[i*4+2] = byte(i * 3)
		src[i*4+3] = 0xff
	}
	enc, err := Confuse(src, w, h, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		j := (i + k) % n
		// 输出位置 order[j] 应等于输入位置 order[i]
		for b := 0; b < 4; b++ {
			if enc[order[j]*4+b] != src[order[i]*4+b] {
				t.Fatalf("混淆 i=%d: out[%d] != in[%d]", i, order[j], order[i])
			}
		}
	}
	dec, err := Confuse(enc, w, h, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		j := (i + k) % n
		// 输出位置 order[i] 应等于输入位置 order[j]
		for b := 0; b < 4; b++ {
			if dec[order[i]*4+b] != enc[order[j]*4+b] {
				t.Fatalf("解混淆 i=%d: out[%d] != in[%d]", i, order[i], order[j])
			}
		}
	}
}

func TestConfusePixelLimit(t *testing.T) {
	// 4096*4096 = 0x1000000，恰好等于上限，应当允许
	if _, err := Confuse(make([]byte, 0), 4096, 4096, true); err == nil {
		t.Fatal("像素不够应报无效图片")
	}
	// 4097*4096 > 上限
	if _, err := Confuse(make([]byte, 4097*4096*4), 4097, 4096, true); err != ErrTooLarge {
		t.Fatalf("超上限应返回 ErrTooLarge，实际 %v", err)
	}
	if ErrTooLarge.Error() != "处理失败，请确认图片格式受支持（最大1600万像素）" {
		t.Fatal(ErrTooLarge.Error())
	}
}

func TestConfuseIsPermutation(t *testing.T) {
	w, h := 11, 7
	order := CurveOrder(w, h)
	if len(order) != w*h {
		t.Fatalf("order 长度 %d，应为 %d", len(order), w*h)
	}
	seen := make(map[int]bool, len(order))
	for _, p := range order {
		if p < 0 || p >= w*h {
			t.Fatalf("下标越界: %d", p)
		}
		if seen[p] {
			t.Fatalf("下标重复: %d", p)
		}
		seen[p] = true
	}
}

func TestResolveMessageTexts(t *testing.T) {
	if ConfuseName(true) != "请提供需要混淆的图片" {
		t.Fatal(ConfuseName(true))
	}
	if ConfuseName(false) != "请提供需要解混淆的图片" {
		t.Fatal(ConfuseName(false))
	}
	if ConfuseProgress(true) != "正在进行图片混淆" {
		t.Fatal(ConfuseProgress(true))
	}
	p, d := ConfuseDone(false)
	if p != "图片已解混淆已完成" || d != "图片已解混淆完成" {
		t.Fatal(p, d)
	}
}

func TestParseOverrides(t *testing.T) {
	ov, rest := ParseOverrides("1girl #步骤35 #采样 Euler a #1024x1536 #指导7.5")
	if ov.Steps == nil || *ov.Steps != 35 {
		t.Fatalf("steps: %+v", ov.Steps)
	}
	if ov.Sampler == nil || *ov.Sampler != "Euler a" {
		t.Fatalf("sampler: %+v", ov.Sampler)
	}
	if ov.Width == nil || *ov.Width != 1024 || ov.Height == nil || *ov.Height != 1536 {
		t.Fatalf("size: %+v %+v", ov.Width, ov.Height)
	}
	if ov.Guidance == nil || *ov.Guidance != 7.5 {
		t.Fatalf("guidance: %+v", ov.Guidance)
	}
	if rest != "1girl" {
		t.Fatalf("rest=%q", rest)
	}
}

func TestResolutionPreset(t *testing.T) {
	r, rest, ok := MatchResolution("1girl #纵向")
	if !ok || r.Width != 832 || r.Height != 1216 {
		t.Fatalf("%+v %v", r, ok)
	}
	if rest != "1girl" {
		t.Fatalf("rest=%q", rest)
	}
	r, _, ok = MatchResolution("风景 #横向")
	if !ok || r.Width != 1216 || r.Height != 832 {
		t.Fatalf("#横向 -> %+v", r)
	}
	r, _, ok = MatchResolution("#人像 头像")
	if !ok || r.Width != 832 || r.Height != 1216 {
		t.Fatalf("#人像 -> %+v", r)
	}
	r, _, ok = MatchResolution("#方形")
	if !ok || r.Width != 1024 || r.Height != 1024 {
		t.Fatalf("#方形 -> %+v", r)
	}
}

func TestMatchCommand(t *testing.T) {
	c, m, ok := MatchCommand("nai3指令 1girl")
	if !ok || c.Kind != CmdDraw || m != "nai3指令" {
		t.Fatalf("%+v %q %v", c, m, ok)
	}
	if got := StripCommand("nai3指令 1girl", m); got != "1girl" {
		t.Fatalf("arg=%q", got)
	}
	c, m, ok = MatchCommand("画图 1girl")
	if !ok || c.Kind != CmdDraw || m != "画图" {
		t.Fatalf("alias: %+v %q %v", c, m, ok)
	}
	c, _, ok = MatchCommand("nai查询额度")
	if !ok || c.Kind != CmdQuota {
		t.Fatalf("quota: %+v %v", c, ok)
	}
}

func TestBlockWord(t *testing.T) {
	if _, err := base64.StdEncoding.DecodeString(""); err != nil {
		t.Fatal(err)
	}
	words := ParseBlockWords("血腥, 暴力、政治\n色情")
	if len(words) != 4 {
		t.Fatalf("%v", words)
	}
	if w := HitBlockWord("1girl, 血腥场景", words); w != "血腥" {
		t.Fatalf("hit=%q", w)
	}
	if w := HitBlockWord("1girl", words); w != "" {
		t.Fatalf("miss=%q", w)
	}
	if MsgBlocked != "该指令包含屏蔽词条，已屏蔽该指令" {
		t.Fatal(MsgBlocked)
	}
}

func TestBlacklist(t *testing.T) {
	b := Blacklist{}
	if got := b.Add(ScopeNai, "12345"); got != "成功添加(12345)" {
		t.Fatalf("add=%q", got)
	}
	if !b.IsBlacklisted(ScopeNai, "12345") {
		t.Fatal("应命中黑名单")
	}
	if b.IsBlacklisted(ScopeSd, "12345") {
		t.Fatal("作用域串了")
	}
	if got := b.Del(ScopeNai, "12345"); got != "成功移除(12345)" {
		t.Fatalf("del=%q", got)
	}
	if got := b.Del(ScopeNai, "12345"); got != "该qq不在黑名单" {
		t.Fatalf("del2=%q", got)
	}
}

func TestPoints(t *testing.T) {
	p := NewPointsBook(10, map[string]int64{"nai-diffusion-3": 15})
	if _, err := p.Charge("u1", "nai-diffusion-3"); err == nil {
		t.Fatal("余额 0 应扣费失败")
	}
	p.Add("u1", 20)
	left, err := p.Charge("u1", "nai-diffusion-3")
	if err != nil {
		t.Fatal(err)
	}
	if left != 5 {
		t.Fatalf("left=%d", left)
	}
	txt := PointsShortText(15, 3)
	if !strings.Contains(txt, "当前画图点数不足") || !strings.Contains(txt, "所需点数") {
		t.Fatal(txt)
	}
}

func TestDecodeOfficialImageZip(t *testing.T) {
	// 裸 base64 PNG 风格（这里用任意字节验证不走 zip 分支）
	raw := []byte("\x89PNG\r\n\x1a\nfake")
	got, err := DecodeOfficialImage(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("裸 base64 应原样返回")
	}
	// data URI 前缀
	got, err = DecodeOfficialImage("data:image/png;base64," + base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("data URI 前缀未剥离")
	}
}

func TestPublishModeBranch(t *testing.T) {
	cases := map[string]SendMode{
		"json": SendJSON,
		"链接发送": SendLink,
		"卡片跳转": SendCard,
		"合并转发": SendForward,
		"":     SendImage,
		"随便":   SendImage,
	}
	for in, want := range cases {
		if got := PublishMode(in); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestPathsMatchBinary(t *testing.T) {
	// 这些路径全部逐字来自 DLL，改动即失配
	if PathNai3Aiurl != "/plug/AI/nai3/aiurl.php" ||
		PathNai3CardJSON != "/plug/AI/nai3json.json" ||
		PathNai3View != "/plug/AI/nai3?id=" ||
		PathSdAiurl != "/plug/AI/sd/aiurl.php" ||
		PathSdCardJSON != "/plug/AI/sdjson.json" ||
		PathSdView != "/plug/AI/sd?id=" {
		t.Fatal("路径常量被改动")
	}
	if UpstreamJSON != "http://xianyu.xianyuw.cn/json.php" {
		t.Fatal(UpstreamJSON)
	}
	if !strings.Contains(DefaultCardButton, "gchat.qpic.cn") {
		t.Fatal("按钮模板")
	}
}

func TestGalleryURL(t *testing.T) {
	u := GalleryViewURL("https://res.xianyuw.cn/", "abc123", true)
	if u != "https://res.xianyuw.cn/plug/AI/nai3?id=abc123" {
		t.Fatal(u)
	}
	if id := GalleryID(u); id != "abc123" {
		t.Fatal(id)
	}
}
