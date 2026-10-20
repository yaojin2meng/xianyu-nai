package nai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// 本文件复刻"发请求之前的提示词加工链"，证据全部来自 fcn.1005b4e8。
//
// 关键字符串（VA）：
//   "<lora:"                0x102b8078
//   "检测到lora："           0x102b8088
//   "已替换"                 0x102b8080
//   "<lora:(.*?)>"          0x102b80dc   lora 提取正则
//   ":0.8>"                 0x102b8070   替换模板
//   "VBScript.RegExp"       0x102b8098   易语言用 VBScript 正则引擎
//   "^[\\u4e00-\\u9fa5]{1,}$" 0x102b80a8  纯中文判定
//   "[\\u4e00-\\u9fa5]"      0x102b80d0  含中文判定
//   "提示词无中文，跳过翻译"   0x102b80ec
//   "翻译接口"               0x102b8104
//   "调用ai翻译:"            0x102b8110
//   "translation_api"       0x102b8124
//   "/api/v1/tag-translate"  0x102b8134
//   "/api/v1/youdao-translate" 0x102b8204

// LoraPattern 是插件用于从提示词里抠 lora 的正则（VA 0x102b80dc）。
var LoraPattern = regexp.MustCompile(`<lora:(.*?)>`)

// hanPattern 对应 "^[\u4e00-\u9fa5]{1,}$"（VA 0x102b80a8）。
var hanPattern = regexp.MustCompile(`^[\p{Han}]{1,}$`)

// hanAnyPattern 对应 "[\u4e00-\u9fa5]"（VA 0x102b80d0）。
var hanAnyPattern = regexp.MustCompile(`[\p{Han}]`)

// IsPureChinese 复刻 0x102b80a8 的判定：整串都是汉字才算。
func IsPureChinese(s string) bool { return hanPattern.MatchString(s) }

// HasChinese 复刻 0x102b80d0 的判定：含任意汉字即可。
func HasChinese(s string) bool { return hanAnyPattern.MatchString(s) }

// Lora 是提示词里解析出的一条 lora 指令。
type Lora struct {
	Raw   string  // 原始片段，如 <lora:xxx:0.8>
	Name  string  // lora 名
	Scale float64 // 触发权重，缺省 0.8
}

// ParseLoras 复刻 "<lora:(.*?)>" 的提取逻辑（0x102b80dc）。
// 插件把每段替换成 ":0.8>"（0x102b8070）后再拼回，这里给出结构化结果 + 替换后的提示词。
func ParseLoras(prompt string) ([]Lora, string) {
	var out []Lora
	replaced := LoraPattern.ReplaceAllStringFunc(prompt, func(m string) string {
		inner := m[len("<lora:") : len(m)-1]
		parts := strings.Split(inner, ":")
		l := Lora{Raw: m, Name: parts[0], Scale: 0.8}
		if len(parts) > 1 {
			if v, err := parseFloatSafe(parts[len(parts)-1]); err == nil {
				l.Scale = v
			}
		}
		out = append(out, l)
		// 原实现把这个片段替换成 ":0.8>" 形态
		return fmt.Sprintf("%s:0.8>", parts[0])
	})
	return out, replaced
}

func parseFloatSafe(s string) (float64, error) {
	var f float64
	err := json.Unmarshal([]byte(strings.TrimSpace(s)), &f)
	return f, err
}

// TranslateMode 对应配置项 translation_api 的取值（VA 0x102b8124）。
type TranslateMode string

const (
	TranslateNone   TranslateMode = ""
	TranslateTag    TranslateMode = "tag"    // /api/v1/tag-translate
	TranslateYoudao TranslateMode = "youdao" // /api/v1/youdao-translate
	TranslateAI     TranslateMode = "ai"     // 走 aiTranslateToken
)

// TranslatePrompt 复刻发请求前的中文翻译分支。
//
// 逻辑（fcn.1005b4e8）：
//  1. 先跑 hanAnyPattern；不含中文 -> 直接返回原文，打 "提示词无中文，跳过翻译"（0x102b80ec）
//  2. 含中文且 translation_api 非空 -> 调对应翻译端点
//  3. 未配置翻译接口 -> 原文返回
func (c *Client) TranslatePrompt(ctx context.Context, prompt string) (string, error) {
	if !HasChinese(prompt) {
		return prompt, nil
	}
	mode := TranslateMode(c.cfg.TranslateAPI)
	switch mode {
	case TranslateTag:
		return c.translateVia(ctx, EPTagTranslate, prompt)
	case TranslateYoudao:
		return c.translateVia(ctx, EPYoudaoTrans, prompt)
	case TranslateAI:
		// AI 翻译用 aiTranslateToken，且不挂在 nai3_plugin-api 下（走服务端下发的翻译网关）
		if c.cfg.AITranslateToken == "" {
			return prompt, nil
		}
		return c.translateVia(ctx, EPTagTranslate, prompt)
	default:
		return prompt, nil
	}
}

// translateVia 调 {nai3_plugin-api} 的翻译端点。
// 请求体字段 "text"（提示词）+ "to":"en"（VA 0x102b811c / 0x102b8120）。
func (c *Client) translateVia(ctx context.Context, path, text string) (string, error) {
	raw, err := c.postPlugin(ctx, path, map[string]interface{}{
		"text": text,
		"to":   "en",
	})
	if err != nil {
		return text, err
	}
	var r struct {
		Text        string `json:"text"`
		Result      string `json:"result"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return text, err
	}
	for _, s := range []string{r.Text, r.Result, r.Translation} {
		if strings.TrimSpace(s) != "" {
			return s, nil
		}
	}
	return text, nil
}

// PreparePrompt 是发请求前的完整加工链：lora 抽取 → 中文翻译 → 负向词兜底。
// 返回加工后的正向提示词与 lora 列表。
func (c *Client) PreparePrompt(ctx context.Context, prompt, negative string) (string, string, []Lora) {
	loras, cleaned := ParseLoras(prompt)
	if t, err := c.TranslatePrompt(ctx, cleaned); err == nil {
		cleaned = t
	}
	if strings.TrimSpace(negative) == "" {
		negative = c.cfg.Negative
	}
	if strings.TrimSpace(negative) == "" {
		negative = DefaultNegative
	}
	return cleaned, negative, loras
}

// BuildLoraPrompt 是插件"检测到lora："回执的拼装（VA 0x102b8088）。
func BuildLoraPrompt(loras []Lora) string {
	if len(loras) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("检测到lora：")
	for i, l := range loras {
		if i > 0 {
			b.WriteString("、")
		}
		fmt.Fprintf(&b, "%s(%.1f)", l.Name, l.Scale)
	}
	return b.String()
}
