package nai

// 本文件是插件的"指令表"，逐条来自 DLL 的配置键与别名常量，
// 用于把 QQ 消息映射到具体动作。
//
// 指令名证据（VA 0x1039aa40 起，主名 + 别名成对出现）：
//
//	0x1039aa48 "nai3指令"   0x1039aa40 "画图"
//	0x1039aa54 "nai查看预设" 0x1039aa60 "查看预设"
//	0x1039aa6c "nai查看标签" 0x1039aa78 "查看标签"
//	0x1039aa84 "nai种子"     0x1039aa8c "修改种子"
//	0x1039aa98 "nai查看负面" 0x1039aaa4 "查看负面"
//	0x1039aab0 "nai修改负面" 0x1039aabc "修改负面"
//	0x1039aac8 "ai解析"      0x1039aad0 "图像解析"
//	0x1039aadc "上传预设"    0x1039aae8 "上传标签"
//	0x1039aaf4 "nai删除预设" 0x1039ab00 "删除预设"
//	0x1039ab0c "nai删除标签"
//	0x1039ab18 "nai查看模型" 0x1039ab24 "查看模型"
//	0x1039ab30 "nai查询额度" 0x1039ab3c "查询额度"
//
//	maybe 0x1039a48c 附近的"nai3参数/画图参数"
//
// 配置键证据（0x1039c078 起）：naiCmdDraw / naiCmdSeed / naiCmdSetNegative /
// naiCmdViewNegative / naiCmdEmotion / naiCmdToLineart / naiCmdToSketch /
// naiCmdClean / naiCmdColorize / naiCmdParse / naiCmdViewPreset /
// naiCmdViewModel / naiCmdQuota。
//
// 指令别名可在 ini 里改，所以这里保留"默认值"与"配置键"两套。

// CommandKind 是动作类型。
type CommandKind string

const (
	CmdDraw         CommandKind = "draw"         // naiCmdDraw
	CmdSeed         CommandKind = "seed"         // naiCmdSeed
	CmdSetNegative  CommandKind = "setNegative"  // naiCmdSetNegative
	CmdViewNegative CommandKind = "viewNegative" // naiCmdViewNegative
	CmdEmotion      CommandKind = "emotion"      // naiCmdEmotion
	CmdLineart      CommandKind = "lineart"      // naiCmdToLineart
	CmdSketch       CommandKind = "sketch"       // naiCmdToSketch
	CmdClean        CommandKind = "clean"        // naiCmdClean
	CmdColorize     CommandKind = "colorize"     // naiCmdColorize
	CmdParse        CommandKind = "parse"        // naiCmdParse
	CmdViewPreset   CommandKind = "viewPreset"   // naiCmdViewPreset
	CmdViewModel    CommandKind = "viewModel"    // naiCmdViewModel
	CmdQuota        CommandKind = "quota"        // naiCmdQuota
)

// Command 是一条指令定义。
type Command struct {
	Kind    CommandKind
	Config  string   // 原插件的 JSON 配置路径（sections.nai.*）
	Default string   // 默认主指令
	Aliases []string // 默认别名
	// IniKey 是 [nai3指令] 节下的键名；非空时该键的值会覆盖默认指令词。
	IniKey string
}

// Commands 是插件内置的默认指令表。
// IniKey 对应 [nai3指令] 节下的键名（0x100A4C01 起实测），
// 该键有值时覆盖 Default。
var Commands = []Command{
	{CmdDraw, "sections.nai.naiCmdDraw", "nai3指令", []string{"画图"}, "画图"},
	{CmdViewPreset, "sections.nai.naiCmdViewPreset", "nai查看预设", []string{"查看预设"}, "查看预设"},
	{CmdViewModel, "sections.nai.naiCmdViewModel", "nai查看模型", []string{"查看模型"}, "查看模型"},
	{CmdSeed, "sections.nai.naiCmdSeed", "nai种子", []string{"修改种子"}, "修改种子"},
	{CmdViewNegative, "sections.nai.naiCmdViewNegative", "nai查看负面", []string{"查看负面"}, "查看负面"},
	{CmdSetNegative, "sections.nai.naiCmdSetNegative", "nai修改负面", []string{"修改负面"}, "修改负面"},
	{CmdParse, "sections.nai.naiCmdParse", "ai解析", []string{"图像解析"}, "图像解析"},
	{CmdQuota, "sections.nai.naiCmdQuota", "nai查询额度", []string{"查询额度"}, "查询额度"},
	{CmdEmotion, "sections.nai.naiCmdEmotion", "nai情绪", nil, "情绪修改"},
	{CmdLineart, "sections.nai.naiCmdToLineart", "nai转线稿", nil, "转线稿"},
	{CmdSketch, "sections.nai.naiCmdToSketch", "nai转草稿", nil, "转草稿"},
	{CmdClean, "sections.nai.naiCmdClean", "nai图像清理", nil, "图像清理"},
	{CmdColorize, "sections.nai.naiCmdColorize", "nai着色", nil, "着色"},
}

// 预设/标签单独管理指令（0x1039aadc / 0x1039aae8 / 0x1039aaf4 / 0x1039ab00 / 0x1039ab0c）
const (
	CmdUploadPreset = "上传预设"
	CmdUploadTag    = "上传标签"
	CmdDeletePreset = "nai删除预设"
	CmdDeleteTag    = "nai删除标签"
)

// 其它功能指令（配置键 0x1039b9d8 起）。
const (
	CfgMenuCmd        = "sections.home.menuCmd"
	CfgGroupOnCmd     = "sections.home.groupOnCmd"
	CfgGroupOffCmd    = "sections.home.groupOffCmd"
	CfgViewPicCmd     = "sections.home.viewPicCmd"
	CfgUpscaleCmd     = "sections.home.upscaleCmd"
	CfgRemoveBgCmd    = "sections.home.removeBgCmd"
	CfgAIDetectCmd    = "sections.home.aiDetectCmd"
	CfgConfuseCmd     = "sections.home.confuseCmd"
	CfgDeConfuseCmd   = "sections.home.deConfuseCmd"
	CfgAddBlockCmd    = "sections.home.addBlockCmd"
	CfgBlacklistAdd   = "sections.home.blacklistAddCmd"
	CfgBlacklistDel   = "sections.home.blacklistDelCmd"
	CfgRechargeCmd    = "sections.home.rechargeCmd"
	CfgQueryPointsCmd = "sections.home.queryPointsCmd"
)

// MatchCommand 在默认指令表里查找（主名或别名，前缀匹配）。
// 返回命中的指令与实际匹配的字符串，便于后续截取参数。
func MatchCommand(text string) (Command, string, bool) {
	return MatchCommandWithIni(text, nil)
}

// MatchCommandWithIni 与 MatchCommand 相同，但用 ini 的 [nai3指令] 值覆盖指令词。
//
// 原插件的指令词来自宿主配置 sections.nai.naiCmd*；重制版没有宿主，
// 改为在 咸鱼ai画图.ini 的 [nai3指令] 节里持久化（键名与 DLL 实测一致）。
// 同名键优先，其次别名，最后默认主名。
func MatchCommandWithIni(text string, f *IniFile) (Command, string, bool) {
	text = trimLeftSpace(text)
	for _, c := range Commands {
		main := c.Default
		if f != nil && c.IniKey != "" {
			main = NAICmdOverride(f, c.IniKey, c.Default)
		}
		if hasPrefixFold(text, main) {
			return c, main, true
		}
		for _, a := range c.Aliases {
			if hasPrefixFold(text, a) {
				return c, a, true
			}
		}
	}
	return Command{}, "", false
}

// StripCommand 去掉命中的指令前缀，返回参数部分。
func StripCommand(text, cmd string) string {
	if len(text) >= len(cmd) {
		return trimSpace(text[len(cmd):])
	}
	return ""
}

func trimLeftSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return s[i:]
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r' || s[j-1] == '\n') {
		j--
	}
	return s[i:j]
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return equalFoldASCII(s[:len(prefix)], prefix)
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// EmotionCommandArgs 解析 "nai情绪 角色 开心" 这类参数。
// 情绪表见 data.go 的 Emotions。
func EmotionCommandArgs(arg string) (name, emotion string, ok bool) {
	fields := splitFields(arg)
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], fields[1], true
}

func splitFields(s string) []string {
	var out []string
	cur := make([]byte, 0, 16)
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n' {
			flush()
			continue
		}
		cur = append(cur, s[i])
	}
	flush()
	return out
}
