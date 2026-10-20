package nai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCardFieldPathAllPresent 用空模板填充，逐条核对字段表里的路径全部落地。
func TestCardFieldPathAllPresent(t *testing.T) {
	f := CardFields{
		UserQQ:      "10001",
		UserName:    "用户甲",
		BotQQ:       "20002",
		BotName:     "咸鱼",
		GroupID:     "999",
		Prompt:      "1girl, cat",
		Body:        "### 模型:V5",
		ButtonData:  "https://img/x.png",
		Receivetime: "1700000000",
	}
	card, err := FillCard(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	var root interface{}
	if err := json.Unmarshal([]byte(card), &root); err != nil {
		t.Fatalf("卡片不是合法 JSON：%v", err)
	}
	for _, p := range CardFieldPath {
		if _, ok := getPath(root, p); !ok {
			t.Fatalf("字段缺失：%s", p)
		}
	}
	// 关键字面量
	want := map[string]interface{}{
		"richtexts[0].filename":                                     "MultiMsg",
		"richtexts[0].info[0].content.type":                         "node",
		"richtexts[0].info[0].content.data.content[0].type":         "markdown",
		"richtexts[0].info[0].content.data.user_id":                 "10001",
		"richtexts[0].info[0].content.data.nickname":                "用户甲",
		"richtexts[0].info[0].fromqq":                               "10001",
		"richtexts[0].info[0].fromgroup":                            "999",
		"richtexts[0].info[0].receivetime":                          "1700000000",
		"richtexts[0].info[0].content.data.content[0].data.content": "1girl, cat",
		"richtexts[0].info[1].card":                                 "咸鱼",
		"richtexts[0].info[1].fromqq":                               "20002",
		"richtexts[0].info[1].content.data.user_id":                 "20002",
		"richtexts[0].info[1].content.data.content[0].data.content": "### 模型:V5",
	}
	for p, v := range want {
		got, ok := getPath(root, p)
		if !ok || got != v {
			t.Fatalf("%s = %#v，期望 %#v", p, got, v)
		}
	}
	// 按钮字段 = ",button=" + base64(UTF-8(按钮串))，见 CardButtonField
	btn, _ := getPath(root, "richtexts[0].info[1].button")
	s, _ := btn.(string)
	if !strings.HasPrefix(s, CardButtonPrefix) {
		t.Fatalf("按钮字段=%q", s)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, CardButtonPrefix))
	if err != nil {
		t.Fatalf("按钮字段不是 base64：%v（%q）", err, s)
	}
	if !strings.HasPrefix(string(raw), `[[{"type":0,"name":"下载图片"`) {
		t.Fatalf("按钮串内容不对：%q", raw)
	}
	if !strings.Contains(string(raw), "https://img/x.png") {
		t.Fatalf("按钮串没带直链：%q", raw)
	}
}

// TestFillCardKeepsTemplateFields 模板里的其它字段必须保留。
func TestFillCardKeepsTemplateFields(t *testing.T) {
	tmpl := []byte(`{"app":"com.tencent.multimsg","meta":{"detail":{"news":[]}},"richtexts":[{"info":[{},{}]}]}`)
	card, err := FillCard(tmpl, CardFields{UserQQ: "1", BotQQ: "2"})
	if err != nil {
		t.Fatal(err)
	}
	var root interface{}
	if err := json.Unmarshal([]byte(card), &root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"app", "meta.detail.news", "richtexts"} {
		if _, ok := getPath(root, p); !ok {
			t.Fatalf("模板字段丢失：%s", p)
		}
	}
	if v, _ := getPath(root, "app"); v != "com.tencent.multimsg" {
		t.Fatalf("app=%#v", v)
	}
	// 模板里的 info[0] 被就地补全，不能变成新数组
	if v, _ := getPath(root, "richtexts[0].info[0].content.type"); v != "node" {
		t.Fatalf("info[0] 未就地补全：%#v", v)
	}
}

// TestButtonJSONReplacesData 按钮串只替换 [0].[0].data，其余字段逐字保留。
func TestButtonJSONReplacesData(t *testing.T) {
	got, err := ButtonJSON("https://img/x.png")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "95EDECA936FD9CE2A03ECCCE73A4EABB") {
		t.Fatalf("默认地址没被替换：%s", got)
	}
	for _, want := range []string{`"name":"下载图片"`, `"reply":0`, `"enter":1`, `"blue":1`, `"type":0`} {
		if !strings.Contains(got, want) {
			t.Fatalf("按钮串丢字段 %s：%s", want, got)
		}
	}
	var root interface{}
	if err := json.Unmarshal([]byte(got), &root); err != nil {
		t.Fatalf("按钮串不是合法 JSON：%v", err)
	}
	if v, _ := getPath(root, CardButtonDataKey); v != "https://img/x.png" {
		t.Fatalf("[0].[0].data=%#v", v)
	}
}

func TestFillCardGateKeepsButtonPrefix(t *testing.T) {
	card, err := FillCard(nil, CardFields{ButtonData: "http://a/b.png"})
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(card, CardButtonPrefix)
	if i < 0 {
		t.Fatalf("按钮前缀不对：%s", card)
	}
	rest := card[i+len(CardButtonPrefix):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatalf("按钮字段没有结束引号：%s", card)
	}
	raw, err := base64.StdEncoding.DecodeString(rest[:j])
	if err != nil {
		t.Fatalf("按钮串不是 base64：%v", err)
	}
	if !strings.HasPrefix(string(raw), "[[{") {
		t.Fatalf("按钮串开头不对：%q", raw)
	}
}

func TestCardTitleAndURL(t *testing.T) {
	if CardTitle(true) != CardTitleNai || CardTitle(false) != CardTitleSd {
		t.Fatal("标题字面量不符")
	}
	if got := CardTemplateURL("https://res.xianyuww.cn"); got != "https://res.xianyuww.cn/plug/AI/nai3json.json" {
		t.Fatalf("模板地址=%s", got)
	}
	if got := CardTemplateURL("https://res.xianyuww.cn/"); got != "https://res.xianyuww.cn/plug/AI/nai3json.json" {
		t.Fatalf("末尾斜杠未处理：%s", got)
	}
}

func TestFetchCardTemplate(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"app":"com.tencent.multimsg"}`))
	}))
	defer srv.Close()

	tmpl, err := FetchCardTemplate(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != PathNai3CardJSON {
		t.Fatalf("请求路径=%s", gotPath)
	}
	card, err := FillCard(tmpl, CardFields{UserQQ: "1", BotQQ: "2", Receivetime: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(card, "com.tencent.multimsg") || !strings.Contains(card, "MultiMsg") {
		t.Fatalf("卡片=%s", card)
	}
	// 未配置资源地址时必须报错，而不是静默返回空模板
	if _, err := FetchCardTemplate(context.Background(), "", nil); err == nil {
		t.Fatal("空资源地址应报错")
	}
}

func TestParsePathForms(t *testing.T) {
	cases := map[string]int{
		"a.b.c":                 3,
		"richtexts[0].filename": 3,
		"[0].[0].data":          3,
		"a[1][2]":               3, // a, [1], [2]
	}
	for p, n := range cases {
		if got := len(parsePath(p)); got != n {
			t.Fatalf("parsePath(%q) 段数=%d，期望 %d", p, got, n)
		}
	}
}
