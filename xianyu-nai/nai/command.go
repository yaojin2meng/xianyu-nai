package nai

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 本文件复刻插件"把用户指令翻译成绘图参数"的一段。
//
// 证据（VA 0x102b7d90 起的一段正则常量表，全部为 VBScript.RegExp 使用）：
//
//	0x102b7d90 "sd_model_name"
//	0x102b7d9c "#模型\d+"          0x102b7da8 "#vae\d+"
//	0x102b7db0 "#步骤(\d+)"
//	0x102b7dbc "#(采样方式|采样)([^#]+)"
//	0x102b7dd4 "#(\d+x\d+)"
//	0x102b7de0 "#(识别率|指导)([\d.]+)"
//	0x102b7df8 "#识别率([\d.]+)"
//
// 分辨率预设（0x102b7e74 起）：
//
//	"#方形" 1024        0x102b7e74 / 0x102b7e7c
//	"#纵向" "#人像" 832 1216   0x102b7e84 / 0x102b7e8c / 0x102b7e94 / 0x102b7e98
//	"#横向" "#风景" 1216 832   0x102b7ea0 / 0x102b7ea8
//	"|#|#|"                    0x102b7ebc
//
// 预设/标签匹配回执：
//
//	0x102b7ef8 ".tags"
//	0x102b7f00 "匹配预设："   0x102b7f0c "匹配标签："
//	0x102b7f1c "该指令包含屏蔽词条，已屏蔽该指令"
//	0x102b7f40 "翻译无效"      0x102b7f4c "翻译无效，请检查翻译接口"
//
// 文件（0x1039a92c / 0x1039a940 / 0x1039a96c / 0x1039a97c）：
//
//	nai3预设列表.json  nai3标签列表.json
//	nai预设列表.ini    nai标签列表.ini

// 分辨率预设，逐条对应 0x102b7e74-0x102b7eac。
var (
	// #方形 -> 1024x1024
	presetSquare = []string{"#方形"}
	// #纵向 / #人像 -> 832x1216
	presetPortrait = []string{"#纵向", "#人像"}
	// #横向 / #风景 -> 1216x832
	presetLandscape = []string{"#横向", "#风景"}
)

// ResolutionPreset 是解析出的分辨率。
type ResolutionPreset struct {
	Width, Height int
	Name          string
}

// MatchResolution 复刻 #方形/#纵向/#横向 的替换。
// 命中时返回预设与"去掉该标记后"的提示词。
func MatchResolution(prompt string) (ResolutionPreset, string, bool) {
	for _, k := range presetSquare {
		if i := strings.Index(prompt, k); i >= 0 {
			return ResolutionPreset{1024, 1024, k}, cut(prompt, i, len(k)), true
		}
	}
	for _, k := range presetPortrait {
		if i := strings.Index(prompt, k); i >= 0 {
			return ResolutionPreset{832, 1216, k}, cut(prompt, i, len(k)), true
		}
	}
	for _, k := range presetLandscape {
		if i := strings.Index(prompt, k); i >= 0 {
			return ResolutionPreset{1216, 832, k}, cut(prompt, i, len(k)), true
		}
	}
	return ResolutionPreset{}, prompt, false
}

func cut(s string, i, n int) string {
	return strings.TrimSpace(s[:i] + " " + s[i+n:])
}

// 指令内联参数正则，逐条对应 0x102b7d9c-0x102b7df8。
var (
	reModel    = regexp.MustCompile(`#模型(\d+)`)
	reVAE      = regexp.MustCompile(`#vae(\d+)`)
	reSteps    = regexp.MustCompile(`#步骤(\d+)`)
	reSampler  = regexp.MustCompile(`#(采样方式|采样)([^#]+)`)
	reSize     = regexp.MustCompile(`#(\d+x\d+)`)
	reGuidance = regexp.MustCompile(`#(识别率|指导)([\d.]+)`)
	reIdentity = regexp.MustCompile(`#识别率([\d.]+)`)
)

// Overrides 是用户指令里覆盖掉的参数（未出现的字段保持零值）。
type Overrides struct {
	ModelIdx   *int
	VAEIdx     *int
	Steps      *int
	Sampler    *string
	Width      *int
	Height     *int
	Guidance   *float64
	Resolution *ResolutionPreset
}

// ParseOverrides 从提示词里抽出所有 # 参数并返回清理后的提示词。
// 顺序与插件一致：#模型 -> #vae -> #步骤 -> #采样 -> #尺寸 -> #识别率。
func ParseOverrides(prompt string) (Overrides, string) {
	var o Overrides
	p := prompt

	if m := reModel.FindStringSubmatch(p); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			o.ModelIdx = &v
		}
		p = reModel.ReplaceAllString(p, " ")
	}
	if m := reVAE.FindStringSubmatch(p); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			o.VAEIdx = &v
		}
		p = reVAE.ReplaceAllString(p, " ")
	}
	if m := reSteps.FindStringSubmatch(p); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			o.Steps = &v
		}
		p = reSteps.ReplaceAllString(p, " ")
	}
	if m := reSampler.FindStringSubmatch(p); m != nil {
		v := strings.TrimSpace(m[2])
		if v != "" {
			o.Sampler = &v
		}
		p = reSampler.ReplaceAllString(p, " ")
	}
	if m := reSize.FindStringSubmatch(p); m != nil {
		parts := strings.SplitN(m[1], "x", 2)
		if len(parts) == 2 {
			w, e1 := strconv.Atoi(parts[0])
			h, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil {
				o.Width, o.Height = &w, &h
			}
		}
		p = reSize.ReplaceAllString(p, " ")
	}
	if m := reGuidance.FindStringSubmatch(p); m != nil {
		if v, err := strconv.ParseFloat(m[2], 64); err == nil {
			o.Guidance = &v
		}
		p = reGuidance.ReplaceAllString(p, " ")
	} else if m := reIdentity.FindStringSubmatch(p); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			o.Guidance = &v
		}
		p = reIdentity.ReplaceAllString(p, " ")
	}
	if r, rest, ok := MatchResolution(p); ok {
		o.Resolution = &r
		p = rest
	}
	return o, strings.TrimSpace(p)
}

// Apply 把 Overrides 落到 Config 上（直接改副本）。
func (o Overrides) Apply(c *Config) {
	if o.Steps != nil && *o.Steps > 0 {
		c.Steps = *o.Steps
	}
	if o.Sampler != nil {
		c.Sampler = *o.Sampler
	}
	if o.Width != nil && *o.Width > 0 {
		c.Width = *o.Width
	}
	if o.Height != nil && *o.Height > 0 {
		c.Height = *o.Height
	}
	if o.Guidance != nil {
		c.Cfg = *o.Guidance
	}
	if o.Resolution != nil {
		c.Width = o.Resolution.Width
		c.Height = o.Resolution.Height
	}
	if o.ModelIdx != nil {
		if i := *o.ModelIdx - 1; i >= 0 && i < len(Models) {
			c.Model = Models[i].Name
		}
	}
}

// PresetMatch 复刻"匹配预设："/"匹配标签："（0x102b7f00 / 0x102b7f0c）。
//
// 插件把提示词按预设名/标签名做子串匹配，命中就把对应内容拼到提示词前面。
// 匹配规则未确认（是否区分大小写、是否整词），这里按子串、忽略大小写处理。
type PresetMatch struct {
	Kind    string // "预设" / "标签"
	Name    string
	Content string
}

// MatchPresets 在提示词里找命中的预设与标签。
func MatchPresets(prompt string, presets PresetFile, tags PresetFile) []PresetMatch {
	low := strings.ToLower(prompt)
	var out []PresetMatch
	for _, p := range presets {
		if p.Name == "" {
			continue
		}
		if strings.Contains(low, strings.ToLower(p.Name)) {
			out = append(out, PresetMatch{"预设", p.Name, p.Content})
		}
	}
	for _, t := range tags {
		if t.Name == "" {
			continue
		}
		if strings.Contains(low, strings.ToLower(t.Name)) {
			out = append(out, PresetMatch{"标签", t.Name, t.Content})
		}
	}
	return out
}

// ApplyPresets 把命中的预设/标签内容拼进提示词，并去掉触发词本身。
// 插件里预设是整段替换（".tags" 0x102b7ef8 表示标签文件后缀）。
func ApplyPresets(prompt string, matches []PresetMatch) string {
	out := prompt
	var head []string
	for _, m := range matches {
		if m.Content != "" {
			head = append(head, m.Content)
		}
		out = strings.ReplaceAll(out, m.Name, " ")
	}
	if len(head) == 0 {
		return strings.TrimSpace(out)
	}
	return strings.TrimSpace(strings.Join(head, ", ") + ", " + out)
}

// PresetMatchReceipt 复刻命中回执。
func PresetMatchReceipt(matches []PresetMatch) string {
	if len(matches) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range matches {
		fmt.Fprintf(&b, "匹配%s：%s\r\n", m.Kind, m.Name)
	}
	return b.String()
}
