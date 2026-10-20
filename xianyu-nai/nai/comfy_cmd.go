package nai

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// 本文件是 ComfyUI 通道的指令表、预设、模型列表与聊天参数解析。
//
// ---- 指令（配置节 comfy指令，0x1039B058）----
//
//	comfy指令/画图      = "comfy画图"      0x102B5D84（通用别名 "画图" 0x1039AA40）
//	comfy指令/查看模型  = "comfy查看模型"  0x1039B064
//	comfy指令/查看预设  = "comfy查看预设"  0x1039B074
//	comfy指令/上传预设  = "comfy上传预设"  0x102BB63C
//	comfy指令/删除预设  = "comfy删除预设"  0x1039B084
//
// ---- 预设文件（comfy预设列表.json，0x102BB600）----
//
// 结构是对象：{"<预设名>": {"tags": <内容>, "info": "上传者：QQ  上传群：群号"}}
// 证据：0x1006E661 / 0x1006E873 取 "<名>.tags"，0x1006E8B6 取 "<名>.info"，
// 0x1006E8D6 拼 "上传者：…  上传群：…"，0x1006E99B 拼 {数据目录}comfy预设列表.json。
//
// ---- 渲染文案（与 SD 预设同一套正文，comfy 自己的回执词）----
//
//	列表 0x1006CE17: "# 预设列表(指令+预设名查看提示词)\r\n```\r\n" + 各项 + "\r\n```"
//	各项 0x1006CDBB: 名 + "\r\n"
//	纯文 0x1006D916: "预设列表(指令+预设名查看提示词)\r\n==========\r\n" + 各项
//	详情 0x1006C03E: "预设名：" + 名 + "\r\n预设内容=\r\n" + 内容 + "\r\n\r\n"
//	未找到 0x1006CCB1: "未找到" + 名 + "预设"
//	上传正文 0x1006EAC1: "# [comfy预设]已上传预设内容\r\n# 预设名：" + 名 + "\r\n```\r\n" + 内容 + "\r\n```"

// Comfy 指令动作。
type ComfyCommandKind string

const (
	ComfyCmdDraw         ComfyCommandKind = "comfyDraw"
	ComfyCmdViewModel    ComfyCommandKind = "comfyViewModel"
	ComfyCmdViewPreset   ComfyCommandKind = "comfyViewPreset"
	ComfyCmdUploadPreset ComfyCommandKind = "comfyUploadPreset"
	ComfyCmdDeletePreset ComfyCommandKind = "comfyDeletePreset"
)

// ComfyCommand 是一条 ComfyUI 指令。
type ComfyCommand struct {
	Kind    ComfyCommandKind
	Default string
	Word    string
}

// ComfyCommands 由配置生成指令表（词面来自 [comfy指令]）。
func ComfyCommands(c ComfyConfig) []ComfyCommand {
	c = c.withDefaults()
	return []ComfyCommand{
		{ComfyCmdDraw, "comfy画图", c.CmdDraw},
		{ComfyCmdViewModel, "comfy查看模型", c.CmdViewModel},
		{ComfyCmdViewPreset, "comfy查看预设", c.CmdViewPreset},
		{ComfyCmdUploadPreset, "comfy上传预设", c.CmdUploadPreset},
		{ComfyCmdDeletePreset, "comfy删除预设", c.CmdDeletePreset},
	}
}

// MatchComfyCommand 做前缀匹配（长词优先）。
func MatchComfyCommand(text string, c ComfyConfig) (ComfyCommand, string, bool) {
	text = trimLeftSpace(text)
	var best ComfyCommand
	bestWord := ""
	for _, cmd := range ComfyCommands(c) {
		for _, w := range []string{cmd.Word, cmd.Default} {
			if w == "" || !hasPrefixFold(text, w) {
				continue
			}
			if len(w) > len(bestWord) {
				best, bestWord = cmd, w
			}
		}
	}
	if bestWord == "" {
		return ComfyCommand{}, "", false
	}
	return best, bestWord, true
}

// ---------- 预设 ----------

// Comfy 预设回执。
const (
	MsgComfyPresetExists   = "[comfy预设]已存在相同预设，请修改后上传"      // 0x102BB5D8
	MsgComfyPresetUploaded = "[comfy预设]已上传预设内容"             // 0x102BB64C
	MsgComfyPresetDeleted  = "[comfy预设]已删除该预设内容"            // 0x102BB668
	MsgComfyPresetNotFound = "[comfy预设]未找到该预设内容"            // 0x102BB684
	MsgComfyPresetBody     = "# [comfy预设]已上传预设内容\r\n# 预设名：" // 0x102BB614
)

// ComfyPreset 是 comfy预设列表.json 的一条：名 + 内容(tags) + 来源(info)。
type ComfyPreset struct {
	Name    string
	Content string
	Info    string
}

// ComfyPresetFile 是 comfy预设列表.json。
type ComfyPresetFile []ComfyPreset

// LoadComfyPresets 读 comfy预设列表.json（对象形式：名 -> {tags, info}）。
func LoadComfyPresets(path string) ComfyPresetFile {
	var out ComfyPresetFile
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	root, err := decodeOJSON(raw)
	if err != nil || root == nil || root.kind != 'o' {
		return out
	}
	for i, k := range root.keys {
		if i >= len(root.vals) {
			break
		}
		v := root.vals[i]
		it := ComfyPreset{Name: k}
		if v != nil && v.kind == 'o' {
			if t, ok := v.get("tags"); ok && t != nil {
				it.Content = t.textValue()
			}
			if inf, ok := v.get("info"); ok && inf != nil {
				it.Info = inf.textValue()
			}
		}
		out = append(out, it)
	}
	return out
}

// SaveComfyPresets 以对象形式写回（保持顺序）。
func SaveComfyPresets(path string, p ComfyPresetFile) error {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, it := range p {
		if i > 0 {
			b.WriteByte(',')
		}
		appendJSONString(&b, it.Name)
		b.WriteString(":{\"tags\":")
		appendJSONString(&b, it.Content)
		b.WriteString(",\"info\":")
		appendJSONString(&b, it.Info)
		b.WriteByte('}')
	}
	b.WriteByte('}')
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// textValue 取节点的文本形态。
func (o *ojson) textValue() string {
	if o == nil {
		return ""
	}
	if o.kind == 's' {
		return o.str
	}
	return o.Text()
}

// get 取对象的直接成员。
func (o *ojson) get(key string) (*ojson, bool) {
	if o == nil || o.kind != 'o' {
		return nil, false
	}
	for i, k := range o.keys {
		if k == key && i < len(o.vals) {
			return o.vals[i], true
		}
	}
	return nil, false
}

// Render 复刻 comfy 预设列表正文（0x1006CE17 / 0x1006CDBB）。
func (p ComfyPresetFile) Render() string {
	var items strings.Builder
	for _, it := range p {
		items.WriteString(it.Name)
		items.WriteString("\r\n")
	}
	var b strings.Builder
	b.WriteString("# 预设列表(指令+预设名查看提示词)\r\n```\r\n")
	b.WriteString(items.String())
	b.WriteString("\r\n```")
	return b.String()
}

// RenderPlain 复刻纯文本/合并转发形态（0x1006D916 / 0x1006C03E）。
func (p ComfyPresetFile) RenderPlain() string {
	var b strings.Builder
	b.WriteString("预设列表(指令+预设名查看提示词)\r\n==========\r\n")
	for _, it := range p {
		b.WriteString("预设名：")
		b.WriteString(it.Name)
		b.WriteString("\r\n预设内容=\r\n")
		b.WriteString(it.Content)
		b.WriteString("\r\n\r\n")
	}
	return b.String()
}

// RenderDetail 单条详情（0x1006C03E）。
func (p ComfyPresetFile) RenderDetail(name string) (string, bool) {
	it, ok := p.Find(name)
	if !ok {
		return "未找到" + name + "预设", false // 0x1006CCB1
	}
	var b strings.Builder
	b.WriteString("预设名：")
	b.WriteString(it.Name)
	b.WriteString("\r\n预设内容=\r\n")
	b.WriteString(it.Content)
	b.WriteString("\r\n\r\n")
	return b.String(), true
}

// Find 按名取预设。
func (p ComfyPresetFile) Find(name string) (ComfyPreset, bool) {
	name = strings.TrimSpace(name)
	for _, it := range p {
		if it.Name == name {
			return it, true
		}
	}
	return ComfyPreset{}, false
}

// Upload 新增预设，重名返回 MsgComfyPresetExists。
func (p *ComfyPresetFile) Upload(name, content, info string) string {
	for _, it := range *p {
		if it.Name == name {
			return MsgComfyPresetExists
		}
	}
	*p = append(*p, ComfyPreset{Name: name, Content: content, Info: info})
	return MsgComfyPresetUploaded
}

// Delete 删除预设。
func (p *ComfyPresetFile) Delete(name string) string {
	for i, it := range *p {
		if it.Name == name {
			*p = append((*p)[:i], (*p)[i+1:]...)
			return MsgComfyPresetDeleted
		}
	}
	return MsgComfyPresetNotFound
}

// ComfyPresetInfo 复刻 0x1006E8D6："上传者：" + QQ + "  上传群：" + 群号。
func ComfyPresetInfo(qq, group string) string {
	return "上传者：" + qq + "  上传群：" + group
}

// ComfyPresetUploadBody 复刻上传成功正文（0x1006EAC1）。
func ComfyPresetUploadBody(name, content string) string {
	return MsgComfyPresetBody + name + "\r\n```\r\n" + content + "\r\n```"
}

// MatchComfyPreset 在提示词里匹配预设名（命中就替换成内容，0x10075252-0x10075460）。
// 返回替换后的提示词与命中的第一个预设名（未命中返回 ""）。
func (p ComfyPresetFile) MatchComfyPreset(prompt string) (string, string) {
	hit := ""
	for _, it := range p {
		if it.Name == "" || !strings.Contains(prompt, it.Name) {
			continue
		}
		prompt = strings.ReplaceAll(prompt, it.Name, " ")
		if it.Content != "" {
			prompt = it.Content + ", " + prompt
		}
		if hit == "" {
			hit = it.Name
		}
	}
	return prompt, hit
}

// ---------- 模型（工作流绑定）列表 ----------

// ComfyModelTable 复刻模型表格（0x102BB6D4 表头 + 0x100706E6 行）。
func ComfyModelTable(ws []ComfyWorkflow) string {
	var b strings.Builder
	b.WriteString("| 序号 | 模型名 |\r\n| --- | --- |") // 0x102BB6D4
	for i, w := range ws {
		b.WriteString("\r\n| ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(" | ")
		b.WriteString(w.Name)
		b.WriteString(" |")
	}
	return b.String()
}

// RenderComfyModels 复刻 comfy查看模型 的文本形态（0x100709C5）。
//
//	<前缀> + "\r\nComfyUI 工作流绑定如下，使用 " + 指令 + " 提示词 #模型序号 切换：\r\n" + 表格
func RenderComfyModels(ws []ComfyWorkflow, cmd string) string {
	if len(ws) == 0 {
		return "\r\n" + MsgComfyNoWorkflow // 0x102BB6A0
	}
	return "\r\nComfyUI 工作流绑定如下，使用 " + cmd + " 提示词 #模型序号 切换：\r\n" + ComfyModelTable(ws)
}

// RenderComfyModelsCard 复刻卡片形态（0x100708BE）。
func RenderComfyModelsCard(ws []ComfyWorkflow, cmd, qq, name string) string {
	if len(ws) == 0 {
		return MsgComfyNoWorkflow
	}
	if name == "" {
		name = qq
	}
	return "![img #28px #28px](https://q1.qlogo.cn/g?b=qq&nk=" + qq + "&s=100)  |  [@" + name +
		"](mqqapi://aio/inlinecmd?command=咸鱼ai画图&reply=false&enter=false)\n\n---\n\n" +
		ComfyModelTable(ws) + "\n\n> 使用 " + cmd + " 提示词 #模型序号 切换工作流"
}

// RenderComfyModelOutOfRange 复刻"模型序号超出范围，当前共 N 个工作流绑定"。
func RenderComfyModelOutOfRange(n int) string {
	return MsgComfyModelOutOfRange + strconv.Itoa(n) + MsgComfyModelCount
}

// ---------- 聊天参数 ----------

// 聊天参数正则（逐字来自 DLL 常量）。
var (
	reComfyParamPair = regexp.MustCompile(`#([^#:：\d][^#:：]*?)[:：]([^#]*)`) // 0x102BB7FC
	reComfyCfgWord   = regexp.MustCompile(`#(识别率|CFG)([\d.]+)`)             // 0x102BB77C
	reComfyModelIdx  = regexp.MustCompile(`#模型(\d+)`)
	reComfyStepsWord = regexp.MustCompile(`#步骤(\d+)`) // 0x102BB7F4 "#步骤"
)

// ComfyChatArgs 是一条 comfy画图 解析出来的东西。
type ComfyChatArgs struct {
	Prompt   string
	Params   map[string]string
	ModelIdx *int
	CFG      *float64
	Steps    *int
}

// ParseComfyArgs 复刻聊天参数解析。
//
// 顺序（0x10072xxx-0x10073B70）：
//  1. #模型N   -> 选用第 N 个工作流绑定（1 基），并从提示词里抹掉；
//  2. #(识别率|CFG)<数字> -> CFG；
//  3. #步骤<数字> -> 步骤；
//  4. #参数名:值 / #参数名：值（名不能以数字开头，值到下一个 # 为止）
//     -> 占位符覆盖，并从提示词里抹掉。
//
// 剩下的文本就是提示词。
func ParseComfyArgs(text string) ComfyChatArgs {
	out := ComfyChatArgs{Params: map[string]string{}}
	s := text

	if loc := reComfyModelIdx.FindStringSubmatchIndex(s); loc != nil {
		if n, err := strconv.Atoi(s[loc[2]:loc[3]]); err == nil && n > 0 {
			out.ModelIdx = &n
		}
		s = s[:loc[0]] + " " + s[loc[1]:]
	}
	if loc := reComfyCfgWord.FindStringSubmatchIndex(s); loc != nil {
		if f, err := strconv.ParseFloat(s[loc[4]:loc[5]], 64); err == nil {
			out.CFG = &f
		}
		s = s[:loc[0]] + " " + s[loc[1]:]
	}
	if loc := reComfyStepsWord.FindStringSubmatchIndex(s); loc != nil {
		if n, err := strconv.Atoi(s[loc[2]:loc[3]]); err == nil {
			out.Steps = &n
		}
		s = s[:loc[0]] + " " + s[loc[1]:]
	}
	for {
		loc := reComfyParamPair.FindStringSubmatchIndex(s)
		if loc == nil {
			break
		}
		name := strings.TrimSpace(s[loc[2]:loc[3]])
		val := strings.TrimSpace(s[loc[4]:loc[5]])
		if name != "" {
			out.Params[name] = val
		}
		s = s[:loc[0]] + " " + s[loc[1]:]
	}
	out.Prompt = strings.TrimSpace(ComfyCollapseSpace(s))
	return out
}

// ComfyCollapseSpace 把连续空白压成一个空格（参数被抹掉后会留下空洞）。
func ComfyCollapseSpace(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}
