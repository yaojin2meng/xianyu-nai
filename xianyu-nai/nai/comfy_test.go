package nai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComfyCaption18 校验 comfy 正文 19 参数拼接（0x1007CC66）的逐字节形态。
func TestComfyCaption18(t *testing.T) {
	m := ResultMeta{
		Width:   512,
		Height:  768,
		Seed:    12345,
		Sampler: "euler/normal",
		Model:   "wf1",
		RawURL:  "http://raw/img.png",
		Prompt:  "1girl, solo",
	}
	got := BuildComfyCaption(m, "http://img/x.png")
	want := "![comfy #512px #768px](http://img/x.png)\r\n" +
		"[🔗下载图片](http://raw/img.png)\r\n" +
		"```\r\n1girl, solo\r\n```\r\n" +
		"# seed:12345\r\n" +
		"# 采样器:euler/normal\r\n" +
		"# 模型:wf1"
	if got != want {
		t.Fatalf("正文不符：\n got=%q\nwant=%q", got, want)
	}
	// 空提示词也要保留 ``` 段（原插件无条件拼接）
	m.Prompt = ""
	got = BuildComfyCaption(m, "u")
	if !strings.Contains(got, "```\r\n\r\n```\r\n") {
		t.Fatalf("空提示词段缺失：%q", got)
	}
}

// TestComfyPlaceholders 校验 《名》/《名|默认值》 与未提供参数的收集顺序。
func TestComfyPlaceholders(t *testing.T) {
	vals := ComfyPlaceholderValues{
		PhComfyWidth:  "512",
		PhComfyHeight: "768",
		PhComfySteps:  "20",
	}
	tmpl := `{"a":"《宽度》","b":"《高度》","c":"《步骤》","d":"《画风|anime》","e":"《缺失》"}`
	got, missing := SubstituteComfyPlaceholders(tmpl, vals)
	want := `{"a":"512","b":"768","c":"20","d":"anime","e":"《缺失》"}`
	if got != want {
		t.Fatalf("替换结果=%q，期望=%q", got, want)
	}
	if len(missing) != 1 || missing[0] != "缺失" {
		t.Fatalf("缺失参数=%v", missing)
	}
	if txt := RenderComfyMissingParams(missing); txt !=
		"工作流存在未提供的参数：《缺失》 请在聊天中用 #参数名:值 提供，或在占位符中写默认值（《参数名|默认值》）" {
		t.Fatalf("缺失提示=%q", txt)
	}
	// 同名占位符只报一次
	_, missing = SubstituteComfyPlaceholders("《x》《x》《y》", nil)
	if len(missing) != 2 || missing[0] != "x" || missing[1] != "y" {
		t.Fatalf("去重失败：%v", missing)
	}
}

// TestParseComfyArgs 校验 #模型N / #步骤N / #CFG / #参数名:值 的剥离。
func TestParseComfyArgs(t *testing.T) {
	got := ParseComfyArgs("1girl #模型2 #步骤30 #CFG7.5 #画风:anime #负面提示词：blurry")
	if got.Prompt != "1girl" {
		t.Fatalf("提示词=%q", got.Prompt)
	}
	if got.ModelIdx == nil || *got.ModelIdx != 2 {
		t.Fatalf("模型序号=%v", got.ModelIdx)
	}
	if got.Steps == nil || *got.Steps != 30 {
		t.Fatalf("步骤=%v", got.Steps)
	}
	if got.CFG == nil || *got.CFG != 7.5 {
		t.Fatalf("CFG=%v", got.CFG)
	}
	if got.Params["画风"] != "anime" {
		t.Fatalf("参数表=%v", got.Params)
	}
	if got.Params["负面提示词"] != "blurry" {
		t.Fatalf("中文冒号参数=%v", got.Params)
	}
	// 名以数字开头不当作参数（正则 "#([^#:：\d]…"）
	g2 := ParseComfyArgs("x #1:2")
	if _, ok := g2.Params["1"]; ok {
		t.Fatalf("数字开头的名不应被当成参数：%v", g2.Params)
	}
}

// TestComfyCommandsDoNotSteal 校验 comfy 指令不抢 nai / sd 前缀。
func TestComfyCommandsDoNotSteal(t *testing.T) {
	for _, s := range []string{"nai画图 x", "sd画图 x", "查看模型", "画图 x"} {
		if _, _, ok := MatchComfyCommand(s, ComfyConfig{}); ok {
			t.Fatalf("%q 被 comfy 抢走", s)
		}
	}
	for _, s := range []string{"comfy画图 x", "comfy查看模型", "comfy查看预设", "comfy上传预设 a=b", "comfy删除预设 a"} {
		if _, _, ok := MatchComfyCommand(s, ComfyConfig{}); !ok {
			t.Fatalf("%q 未命中 comfy", s)
		}
	}
	// 自定义指令词
	cfg := ComfyConfig{CmdDraw: "c画"}
	if _, w, ok := MatchComfyCommand("c画 1girl", cfg); !ok || w != "c画" {
		t.Fatalf("自定义指令词未生效：%v %q", ok, w)
	}
}

// TestComfyWorkflowLoadAndMigrate 校验 {数据目录}comfy工作流\*.json 装载与
// 旧版 comfy工作流.json 的拆分 + 改名 .bak（0x1009BBB4）。
func TestComfyWorkflowLoadAndMigrate(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, FileComfyWfDir)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "甲.json"), []byte(`{"1":{"class_type":"KSampler"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// 旧版聚合文件
	agg := `{"乙":{"1":{"class_type":"SaveImage"}}}`
	if err := os.WriteFile(filepath.Join(dir, FileComfyWfJSON), []byte(agg), 0o644); err != nil {
		t.Fatal(err)
	}
	list := LoadComfyWorkflows(dir)
	names := ComfyWorkflowNames(list)
	if len(names) != 2 || names[0] != "乙" || names[1] != "甲" {
		t.Fatalf("绑定列表=%v", names)
	}
	// 聚合文件应被改名成 .bak，且拆出的文件存在
	if _, err := os.Stat(filepath.Join(ws, "乙.json")); err != nil {
		t.Fatalf("拆分文件缺失：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileComfyWfJSONBak)); err != nil {
		t.Fatalf(".bak 缺失：%v", err)
	}
	if w, ok := FindComfyWorkflow(list, "乙"); !ok || !strings.Contains(w.Content, "SaveImage") {
		t.Fatalf("按名取绑定失败：%v %q", ok, w.Content)
	}
}

// TestComfyPerUserSelection 校验 user_cache.ini 的 comfy_model_name。
func TestComfyPerUserSelection(t *testing.T) {
	dir := t.TempDir()
	if got := ComfySelectedWorkflow(dir, "10001"); got != "" {
		t.Fatalf("未写入时应为空：%q", got)
	}
	if err := SetComfySelectedWorkflow(dir, "10001", "甲"); err != nil {
		t.Fatal(err)
	}
	if got := ComfySelectedWorkflow(dir, "10001"); got != "甲" {
		t.Fatalf("选中绑定=%q", got)
	}
	if got := ComfySelectedWorkflow(dir, "10002"); got != "" {
		t.Fatalf("不同用户不应串：%q", got)
	}
}

// TestComfyPresets 校验预设的渲染/增删/文件往返（comfy预设列表.json 是对象）。
func TestComfyPresets(t *testing.T) {
	var p ComfyPresetFile
	if msg := p.Upload("画风A", "oil painting", "上传者：1  上传群：2"); msg != MsgComfyPresetUploaded {
		t.Fatalf("上传回执=%q", msg)
	}
	if msg := p.Upload("画风A", "x", ""); msg != MsgComfyPresetExists {
		t.Fatalf("重名回执=%q", msg)
	}
	list := p.Render()
	if list != "# 预设列表(指令+预设名查看提示词)\r\n```\r\n画风A\r\n\r\n```" {
		t.Fatalf("列表=%q", list)
	}
	detail, ok := p.RenderDetail("画风A")
	if !ok || detail != "预设名：画风A\r\n预设内容=\r\n\r\n\r\n" && detail != "预设名：画风A\r\n预设内容=\r\noil painting\r\n\r\n" {
		t.Fatalf("详情=%q", detail)
	}
	if txt, ok := p.RenderDetail("没有"); ok || txt != "未找到没有预设" {
		t.Fatalf("未找到文案=%q", txt)
	}
	if body := ComfyPresetUploadBody("画风A", "oil painting"); body !=
		"# [comfy预设]已上传预设内容\r\n# 预设名：画风A\r\n```\r\noil painting\r\n```" {
		t.Fatalf("上传正文=%q", body)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, FileComfyPresetJSON)
	if err := SaveComfyPresets(path, p); err != nil {
		t.Fatal(err)
	}
	back := LoadComfyPresets(path)
	if len(back) != 1 || back[0].Name != "画风A" || back[0].Content != "oil painting" ||
		back[0].Info != "上传者：1  上传群：2" {
		t.Fatalf("文件往返=%+v", back)
	}
	if msg := back.Delete("画风A"); msg != MsgComfyPresetDeleted {
		t.Fatalf("删除回执=%q", msg)
	}
	if msg := back.Delete("画风A"); msg != MsgComfyPresetNotFound {
		t.Fatalf("重复删除回执=%q", msg)
	}
}

// TestComfyModelList 校验 comfy查看模型 的文本与卡片形态。
func TestComfyModelList(t *testing.T) {
	ws := []ComfyWorkflow{{Name: "甲"}, {Name: "乙"}}
	txt := RenderComfyModels(ws, "comfy画图")
	want := "\r\nComfyUI 工作流绑定如下，使用 comfy画图 提示词 #模型序号 切换：\r\n" +
		"| 序号 | 模型名 |\r\n| --- | --- |\r\n| 1 | 甲 |\r\n| 2 | 乙 |"
	if txt != want {
		t.Fatalf("模型列表=\n%q\n期望=\n%q", txt, want)
	}
	if RenderComfyModels(nil, "comfy画图") != "\r\n"+MsgComfyNoWorkflow {
		t.Fatal("空绑定文案不符")
	}
	if got := RenderComfyModelOutOfRange(3); got != "模型序号超出范围，当前共 3 个工作流绑定" {
		t.Fatalf("越界文案=%q", got)
	}
	card := RenderComfyModelsCard(ws, "comfy画图", "10001", "小明")
	if !strings.Contains(card, "| 2 | 乙 |") || !strings.Contains(card, "> 使用 comfy画图 提示词 #模型序号 切换工作流") {
		t.Fatalf("卡片形态=%q", card)
	}
}

// TestComfyHistoryParse 校验 /history 解析（.status / status_str / outputs.*.images[]）。
func TestComfyHistoryParse(t *testing.T) {
	raw := []byte(`{"pid":{"status":{"status_str":"success"},"outputs":{
		"9":{"images":[{"filename":"a.png","subfolder":"sub","type":"output"}]}}}}`)
	res := ComfyParseHistory(raw, "pid")
	if !res.Done || res.Status != "success" {
		t.Fatalf("解析=%+v", res)
	}
	if len(res.Images) != 1 || res.Images[0].Filename != "a.png" ||
		res.Images[0].Subfolder != "sub" || res.Images[0].Type != "output" {
		t.Fatalf("图片=%+v", res.Images)
	}
	if res := ComfyParseHistory([]byte(`{"pid":{}}`), "pid"); res.Done {
		t.Fatal("无 status 不应算完成")
	}
	if res := ComfyParseHistory([]byte(`{}`), "pid"); res.Done {
		t.Fatal("无条目不应算完成")
	}
}

// TestComfySubmitBody 校验 {"prompt":…,"client_id":"xianyuai"} 的键序。
func TestComfySubmitBody(t *testing.T) {
	got := BuildComfyPromptBody(`{"1":{"class_type":"X"}}`)
	want := `{"prompt":{"1":{"class_type":"X"}},"client_id":"xianyuai"}`
	if got != want {
		t.Fatalf("提交体=%q", got)
	}
	if i, j := strings.Index(got, "prompt"), strings.Index(got, "client_id"); i < 0 || j < 0 || i > j {
		t.Fatal("键序应为 prompt -> client_id")
	}
	// /view 地址
	u := ComfyViewURL("http://c:8188", "a b.png", "s", "output")
	if u != "http://c:8188/view?filename=a+b.png&subfolder=s&type=output" {
		t.Fatalf("/view=%q", u)
	}
}

// TestComfyINIDefaults 校验 [comfy设置]/[comfy参数]/[comfy指令] 的默认值与读取。
func TestComfyINIDefaults(t *testing.T) {
	d := DefaultComfyConfig()
	if d.Steps != 20 || d.Cfg != 7 || d.Width != 512 || d.Height != 512 ||
		d.Seed != "-1" || d.Sampler != "euler" || d.Scheduler != "normal" ||
		d.Cooldown != "0" || d.CooldownMode != "个人" ||
		d.Negative != SDDefaultNegative {
		t.Fatalf("默认值=%+v", d)
	}
	if d.CmdDraw != "comfy画图" || d.CmdViewModel != "comfy查看模型" ||
		d.CmdViewPreset != "comfy查看预设" || d.CmdUploadPreset != "comfy上传预设" ||
		d.CmdDeletePreset != "comfy删除预设" {
		t.Fatalf("指令默认值=%+v", d)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, FileMainINI)
	body := "[comfy设置]\r\n链接=http://127.0.0.1:8188\r\nToken=tk\r\n" +
		"[comfy参数]\r\nsteps=33\r\ncfg=5.5\r\n宽度=640\r\n高度=960\r\n" +
		"默认种子=42\r\n采样=dpmpp_2m\r\n调度=karras\r\n全局默认模型=甲\r\n" +
		"完成后冷却=9\r\n完成冷却模式=群\r\n负面词条=bad\r\n" +
		"[comfy指令]\r\n画图=mycomfy\r\n查看模型=myModel\r\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c := LoadComfyConfigFromINI(p, ComfyConfig{})
	if c.URL != "http://127.0.0.1:8188" || c.Token != "tk" {
		t.Fatalf("链接/Token=%+v", c)
	}
	if c.Steps != 33 || c.Cfg != 5.5 || c.Width != 640 || c.Height != 960 ||
		c.Seed != "42" || c.Sampler != "dpmpp_2m" || c.Scheduler != "karras" ||
		c.GlobalModel != "甲" || c.Cooldown != "9" || c.CooldownMode != "群" ||
		c.Negative != "bad" {
		t.Fatalf("参数=%+v", c)
	}
	if c.CmdDraw != "mycomfy" || c.CmdViewModel != "myModel" ||
		c.CmdViewPreset != "comfy查看预设" {
		t.Fatalf("指令=%+v", c)
	}
	if c.Cooldown != "9" || ComfyCooldownSeconds("9") != 9 {
		t.Fatal("冷却解析失败")
	}
	if got := ComfyEffectiveSeed("-1", 1700000000, nil); got != "1700000000" {
		t.Fatalf("默认种子=%q", got)
	}
	if got := ComfyEffectiveSeed("777", 1700000000, nil); got != "777" {
		t.Fatalf("指定种子=%q", got)
	}
}

// TestComfyComfyBaseURL 校验链接末尾斜杠处理与可用性判断。
func TestComfyComfyBaseURL(t *testing.T) {
	c := ComfyConfig{URL: " http://x:8188/ "}
	if c.ComfyBaseURL() != "http://x:8188" {
		t.Fatalf("base=%q", c.ComfyBaseURL())
	}
	if !c.ComfyAvailable() {
		t.Fatal("应视为已配置")
	}
	if (ComfyConfig{}).ComfyAvailable() {
		t.Fatal("空链接不应视为已配置")
	}
}
