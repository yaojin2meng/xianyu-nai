package nai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Model 对应 DLL 里 naiModelsList 的条目（VA 0x102B9A34 起，单条常量字符串）。
type Model struct {
	Name    string `json:"model_name"`
	Title   string `json:"title"`
	Docs    string `json:"docs"`
	Version string `json:"version"`
}

// Models 是从 DLL 里原样提取的模型表。
var Models = []Model{
	{"NAI Diffusion V5", "nai-diffusion-5-full", "最新v5高品质模型全量版，限制级", "v5"},
	{"NAI Diffusion V5 Curated", "nai-diffusion-5-curated", "最新v5高品质模型预览版，全年龄", "v5"},
	{"NAI Diffusion V4.5", "nai-diffusion-4-5-full", "最新v4.5高品质模型全量版，限制级", "v4.5"},
	{"NAI Diffusion V4.5 Curated", "nai-diffusion-4-5-curated", "最新v4.5高品质模型，全年龄", "v4.5"},
	{"NAI Diffusion V4", "nai-diffusion-4-full", "最新v4高品质模型，限制级", "v4"},
	{"NAI Diffusion V4 Curated", "nai-diffusion-4-curated-preview", "最新v4测试版模型，全年龄", "v4"},
	{"NAI Diffusion V3", "nai-diffusion-3", "v3通用品质模型，限制级", "v3"},
	{"NAI Diffusion Furry 3", "nai-diffusion-furry-3", "福瑞风格特化模型，限制级", "v3"},
}

// Emotions 是 emotion 参数的中文→英文映射（VA 0x102BA144，24 条）。
// 注意 "恼火" 的值在二进制里就是 "lrritated"（小写 L），不是拼写错误，原样保留。
var Emotions = map[string]string{
	"中立": "neutral", "开心": "happy", "悲伤": "sad", "愤怒": "angry",
	"惊讶": "surprised", "厌恶": "disgusted", "害怕": "scared", "困惑": "confused",
	"疲倦": "tired", "兴奋": "excited", "尴尬": "embarrassed", "害羞": "shy",
	"得意": "smug", "坚定": "determined", "无聊": "bored", "思考": "thinking",
	"紧张": "nervous", "大笑": "laughing", "恼火": "lrritated", "激动": "aroused",
	"担忧": "worried", "恋爱": "love", "痛苦": "hurt", "调皮": "playful",
}

// EmotionTag 把中文情绪转成官方 emotion 枚举值。
func EmotionTag(cn string) (string, bool) {
	v, ok := Emotions[strings.TrimSpace(cn)]
	return v, ok
}

// ModelsMarkdown 复刻插件 "nai查看模型" 的输出（表头 VA 0x102B9F50）。
func ModelsMarkdown() string {
	var b strings.Builder
	b.WriteString("===【novelai模型列表】===\r\n")
	b.WriteString("| 序号 | 模型名 | 说明 |\r\n")
	b.WriteString("| ----- | ----- | ----- |\r\n")
	for i, m := range Models {
		fmt.Fprintf(&b, "| %d | %s | %s |\r\n", i+1, m.Title, m.Docs)
	}
	return b.String()
}

// ModelsByVersion 按 version 过滤（v3/v4/v4.5/v5），供命令里区分 v3 与 v4+ 的两套参数。
func ModelsByVersion(version string) []Model {
	var out []Model
	for _, m := range Models {
		if m.Version == version {
			out = append(out, m)
		}
	}
	return out
}

// IsV3 用于判断是否需要写 sm/sm_dyn/v4_prompt 等 v3 专有字段。
func IsV3(model string) bool {
	return model == "nai-diffusion-3" || model == "nai-diffusion-furry-3"
}

// Preset 对应 nai预设列表.json 的元素。
type Preset struct {
	Name    string `json:"名"`
	Content string `json:"内容"`
}

// Tag 对应 nai标签列表.json 的元素。
type Tag struct {
	Name    string `json:"名"`
	Content string `json:"内容"`
}

// PresetFile 是 nai预设列表.json / nai标签列表.json 的外层数组。
type PresetFile []Preset

func (p PresetFile) Find(name string) (Preset, bool) {
	for _, item := range p {
		if item.Name == name {
			return item, true
		}
	}
	return Preset{}, false
}

func (p *PresetFile) Upsert(item Preset) error {
	for i, old := range *p {
		if old.Name == item.Name {
			(*p)[i] = item
			return nil
		}
	}
	*p = append(*p, item)
	return nil
}

func (p *PresetFile) Delete(name string) bool {
	for i, old := range *p {
		if old.Name == name {
			*p = append((*p)[:i], (*p)[i+1:]...)
			return true
		}
	}
	return false
}

// SeedText 复刻反代模板里 seed 用字符串的写法。
func SeedText(seed int64) string {
	if seed < 0 {
		return "-1"
	}
	return strconv.FormatInt(seed, 10)
}

// ParseSeed 复刻插件里 "种子" 解析：-1 表示随机。
func ParseSeed(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// ParsePresetFile 读取预设/标签文件内容。
func ParsePresetFile(raw []byte) (PresetFile, error) {
	var out PresetFile
	if len(strings.TrimSpace(string(raw))) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
