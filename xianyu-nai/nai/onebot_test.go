package nai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeCaller 记录 OneBot API 调用。
type fakeCaller struct {
	mu    sync.Mutex
	calls []struct {
		Action string
		Params map[string]interface{}
	}
}

func (f *fakeCaller) Call(action string, params map[string]interface{}) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		Action string
		Params map[string]interface{}
	}{action, params})
	return json.RawMessage(`{"status":"ok","retcode":0}`), nil
}

func (f *fakeCaller) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.Action)
	}
	return out
}

func TestEventTextAndAt(t *testing.T) {
	ev := Event{
		MessageType: "group",
		SelfID:      10001,
		Message: []Seg{
			TextSeg("nai查看模型"),
			{Type: "at", Data: map[string]interface{}{"qq": "10001"}},
			{Type: "image", Data: map[string]interface{}{"url": "http://x/a.png"}},
		},
	}
	if got := ev.Text(); got != "nai查看模型" {
		t.Fatalf("Text()=%q", got)
	}
	if !ev.Ats("10001") {
		t.Fatal("@ 判定失败")
	}
	if imgs := ev.Images(); len(imgs) != 1 || imgs[0] != "http://x/a.png" {
		t.Fatalf("Images()=%v", imgs)
	}
}

func TestOneBotHostActionShapes(t *testing.T) {
	fc := &fakeCaller{}
	h := OneBotHost{C: fc}

	if err := h.SendGroupMsg("123", "hi"); err != nil {
		t.Fatal(err)
	}
	if err := h.SendPrivateMsg("456", "yo"); err != nil {
		t.Fatal(err)
	}
	if err := h.SendGroupJSON("123", DefaultCardButton); err != nil {
		t.Fatal(err)
	}
	if err := h.SendForward("123", []ForwardNode{{UserID: "1", Nick: "n", Text: "t"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(fc.actions(), ","),
		"send_group_msg,send_private_msg,send_group_msg,send_group_forward_msg"; got != want {
		t.Fatalf("actions=%s want %s", got, want)
	}

	// 群 id / 用户 id 必须是数字
	if v, ok := fc.calls[0].Params["group_id"].(int64); !ok || v != 123 {
		t.Fatalf("group_id=%#v", fc.calls[0].Params["group_id"])
	}
	// 卡片必须落在 json 段里
	segs := fc.calls[2].Params["message"].([]Seg)
	if segs[0].Type != "json" || segs[0].Data["data"] != DefaultCardButton {
		t.Fatalf("卡片段=%#v", segs)
	}
	// 合并转发必须是 node 段
	msgs := fc.calls[3].Params["messages"].([]Seg)
	if msgs[0].Type != "node" {
		t.Fatalf("转发段=%#v", msgs)
	}
}

func newTestPlugin(t *testing.T) (*Plugin, *fakeCaller) {
	t.Helper()
	fc := &fakeCaller{}
	cfg := DefaultConfig()
	cfg.BlockWords = "暴力"
	cfg.BasePoints = 0
	p := NewPlugin(OneBotHost{C: fc}, cfg)
	p.Init(cfg, &Store{Path: t.TempDir() + "/p.json"})
	p.Enable()
	return p, fc
}

func TestPluginLifecycleGatesMessages(t *testing.T) {
	p, _ := newTestPlugin(t)
	ev := Event{MessageType: "private", UserID: 7, Message: []Seg{TextSeg("nai查看模型")}}

	if got := p.OnEventNT(ev); len(got) == 0 {
		t.Fatal("启用后应响应")
	}
	p.Disable()
	if got := p.OnEventNT(ev); got != nil {
		t.Fatalf("停用后不应响应，得到 %v", got)
	}
}

func TestPluginBlockWordAndGroupGate(t *testing.T) {
	p, _ := newTestPlugin(t)

	// 屏蔽词优先于指令
	got := p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai3指令 暴力内容")}})
	if len(got) != 1 || got[0].Text != MsgBlocked {
		t.Fatalf("屏蔽词未拦截：%#v", got)
	}

	// 群里非指令、未 @ 机器人 -> 忽略
	if r := p.OnGroupNT(Event{MessageType: "group", SelfID: 1, UserID: 2,
		Message: []Seg{TextSeg("随便聊聊")}}); r != nil {
		t.Fatalf("群里闲聊不应响应：%#v", r)
	}

	// 群里 @ 机器人但非指令 -> 也忽略（插件按指令前缀分派）
	ev := Event{MessageType: "group", SelfID: 1, UserID: 2, Message: []Seg{
		{Type: "at", Data: map[string]interface{}{"qq": "1"}},
		TextSeg("随便聊聊"),
	}}
	if r := p.OnGroupNT(ev); r != nil {
		t.Fatalf("群里 @ 机器人但非指令不应响应：%#v", r)
	}

	// 预设/标签管理词属于指令，群里应放行
	r := p.OnGroupNT(Event{MessageType: "group", SelfID: 1, UserID: 2,
		Message: []Seg{TextSeg("上传预设 群里的预设 一只狗")}})
	if len(r) != 1 || r[0].Text != MsgPresetUploaded {
		t.Fatalf("群里上传预设未放行：%#v", r)
	}
}

func TestPluginViewModelAndPresetCommands(t *testing.T) {
	p, _ := newTestPlugin(t)

	got := p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai查看模型")}})
	if len(got) != 1 || !strings.Contains(got[0].Text, Models[0].Name) {
		t.Fatalf("查看模型：%#v", got)
	}

	// 上传预设 -> 查看预设
	p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("上传预设 我的预设 一只猫")}})
	got = p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai查看预设")}})
	if len(got) != 1 || !strings.Contains(got[0].Text, "我的预设") {
		t.Fatalf("查看预设：%#v", got)
	}

	// 删除预设
	got = p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai删除预设 我的预设")}})
	if len(got) != 1 || got[0].Text != MsgPresetDeleted {
		t.Fatalf("删除预设：%#v", got)
	}
}

func TestPluginSetSeedAndNegative(t *testing.T) {
	p, _ := newTestPlugin(t)

	if got := p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai种子 12345")}}); len(got) != 1 ||
		!strings.Contains(got[0].Text, "12345") {
		t.Fatalf("设置种子：%#v", got)
	}
	p.mu.RLock()
	seed := p.cfg.Seed
	p.mu.RUnlock()
	if seed != 12345 {
		t.Fatalf("种子未落到配置：%d", seed)
	}

	p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai修改负面 不要文字")}})
	if got := p.OnPrivateNT(Event{MessageType: "private", UserID: 7,
		Message: []Seg{TextSeg("nai查看负面")}}); len(got) != 1 || got[0].Text != "不要文字" {
		t.Fatalf("负面词：%#v", got)
	}
}

func TestReportHandlerDelivers(t *testing.T) {
	p, fc := newTestPlugin(t)

	ev := Event{
		PostType:    "message",
		MessageType: "group",
		GroupID:     999,
		UserID:      7,
		SelfID:      1,
		Message:     []Seg{TextSeg("nai查看模型")},
	}
	body, _ := json.Marshal(ev)
	req := httptest.NewRequest(http.MethodPost, "/onebot", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	p.ReportHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if acts := fc.actions(); len(acts) != 1 || acts[0] != "send_group_msg" {
		t.Fatalf("actions=%v", acts)
	}
	if v, ok := fc.calls[0].Params["group_id"].(int64); !ok || v != 999 {
		t.Fatalf("group_id=%#v", fc.calls[0].Params["group_id"])
	}
}

// TestUploadPresetIsNotACommand 记录预设/标签管理词不属于 Commands 表，
// 必须由 presetCommand 单独处理。
func TestUploadPresetIsNotACommand(t *testing.T) {
	for _, s := range []string{
		CmdUploadPreset + " 我的预设 一只猫",
		CmdUploadTag + " 我的标签 一只猫",
		CmdDeletePreset + " 我的预设",
		CmdDeleteTag + " 我的标签",
	} {
		if c, m, ok := MatchCommand(s); ok {
			t.Fatalf("%q 被当成指令 %v/%q 匹配了", s, c.Kind, m)
		}
	}
}

func TestImageSegIsBase64(t *testing.T) {
	s := ImageSeg([]byte{1, 2, 3})
	if s.Type != "image" {
		t.Fatalf("type=%s", s.Type)
	}
	f, _ := s.Data["file"].(string)
	if !strings.HasPrefix(f, "base64://") {
		t.Fatalf("file=%s", f)
	}
}

// TestEndToEndOneBotHTTP 走完整链路：
//
//	HTTP 上报 -> ReportHandler -> OnEventNT -> Host -> HTTPCaller -> OneBot API
//
// 与 xianyu-nai.exe -onebot 的运行路径一致。
func TestEndToEndOneBotHTTP(t *testing.T) {
	// 假的 OneBot API，记录收到的动作与正文
	var mu sync.Mutex
	var got []struct {
		Action string
		Body   string
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, struct {
			Action string
			Body   string
		}{strings.TrimPrefix(r.URL.Path, "/"), string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","retcode":0}`))
	}))
	defer api.Close()

	cfg := DefaultConfig()
	cfg.BlockWords = "暴力"
	host := OneBotHost{C: &HTTPCaller{Base: api.URL}}
	p := NewPlugin(host, cfg)
	p.Init(cfg, &Store{Path: filepath.Join(t.TempDir(), "nai3预设列表.json")})
	p.Enable()

	bot := httptest.NewServer(p.ReportHandler())
	defer bot.Close()

	post := func(ev string) {
		t.Helper()
		resp, err := http.Post(bot.URL+"/onebot", "application/json", strings.NewReader(ev))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("code=%d", resp.StatusCode)
		}
	}

	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, 0, len(got))
		for _, g := range got {
			out = append(out, g.Action)
		}
		return out
	}

	// 1) 群：查看模型 -> send_group_msg
	post(`{"message_type":"group","group_id":999,"user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"nai查看模型"}}]}`)
	// 2) 私聊：查看模型 -> send_private_msg
	post(`{"message_type":"private","user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"nai查看模型"}}]}`)
	// 3) 群：闲聊 -> 不回应
	post(`{"message_type":"group","group_id":999,"user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"随便聊聊"}}]}`)
	// 4) 群：上传预设
	post(`{"message_type":"group","group_id":999,"user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"上传预设 我的预设 一只猫"}}]}`)
	// 5) 群：查看预设（应能看到刚上传的）
	post(`{"message_type":"group","group_id":999,"user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"nai查看预设"}}]}`)
	// 6) 群：屏蔽词 -> 拦截提示
	post(`{"message_type":"group","group_id":999,"user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"nai3指令 暴力内容"}}]}`)
	// 7) 群：改种子
	post(`{"message_type":"group","group_id":999,"user_id":7,"self_id":10001,
		"message":[{"type":"text","data":{"text":"nai种子 4242"}}]}`)

	mu.Lock()
	texts := make([]string, 0, len(got))
	for _, g := range got {
		texts = append(texts, g.Action+" "+g.Body)
	}
	all := strings.Join(texts, "\n")
	mu.Unlock()

	if acts := snapshot(); len(acts) != 6 {
		t.Fatalf("动作数=%d %v\n%s", len(acts), acts, all)
	}

	if !strings.Contains(all, Models[0].Name) {
		t.Fatalf("模型列表未出现: %s", all)
	}
	if !strings.Contains(all, "我的预设") {
		t.Fatalf("预设上传后未出现在查看预设里: %s", all)
	}
	if !strings.Contains(all, MsgBlocked) {
		t.Fatalf("屏蔽词未拦截: %s", all)
	}
	if !strings.Contains(all, "4242") {
		t.Fatalf("种子未生效: %s", all)
	}
	// 群 id 必须是数字而不是字符串
	if !strings.Contains(all, `"group_id":999`) {
		t.Fatalf("group_id 不是数字: %s", all)
	}
}
