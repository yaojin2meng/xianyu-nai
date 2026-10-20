package nai

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 本文件是 ComfyUI 指令的派发与回执，以及结果正文（18 段）。
//
// ---- 正文（0x1007CC66，19 参数拼接；最后压栈的在最前）----
//
//	"![comfy #" + 宽 + "px #" + 高 + "px](" + 图链 + ")\r\n[🔗下载图片](" + 原始链接 +
//	")\r\n```\r\n" + 提示词 + "\r\n```\r\n# seed:" + 种子 +
//	"\r\n# 采样器:" + 采样器 + "/" + 调度器 + "\r\n# 模型:" + 模型
//
// 卡片标题 0x102BBCB4 = "comfy画图结果"，卡片模板 /plug/AI/aijson.json（0x102B9644）。

// CardTitleComfy 是 ComfyUI 结果的卡片标题（0x102BBCB4）。
const CardTitleComfy = "comfy画图结果"

// MsgComfyPresetHit 是预设命中提示前缀（0x102BB874）。
const MsgComfyPresetHit = "comfy匹配预设："

// BuildComfyCaption 复刻 comfy 结果正文。
func BuildComfyCaption(m ResultMeta, imageURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "![comfy #%dpx #%dpx](%s)\r\n", m.Width, m.Height, imageURL)
	fmt.Fprintf(&b, "[🔗下载图片](%s)\r\n", m.RawURL)
	fmt.Fprintf(&b, "```\r\n%s\r\n```\r\n", m.Prompt)
	fmt.Fprintf(&b, "# seed:%d\r\n# 采样器:%s\r\n# 模型:%s", m.Seed, m.Sampler, m.Model)
	return b.String()
}

// FetchImageBytes 下载一张图（参考图上传用）。
func (c *Client) FetchImageBytes(ctx context.Context, u string) ([]byte, error) {
	u = strings.TrimSpace(u)
	if u == "" {
		return nil, fmt.Errorf("图片地址为空")
	}
	if strings.HasPrefix(u, "base64://") {
		return base64.StdEncoding.DecodeString(strings.TrimPrefix(u, "base64://"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", MsgSDLinkFailed)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s", MsgSDLinkFailed)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("%s", MsgSDLinkFailed)
	}
	return b, nil
}

// comfyAtoi 文本转整数，失败用默认值。
func comfyAtoi(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}

// comfyAtof 文本转小数，失败用默认值。
func comfyAtof(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil || f <= 0 {
		return def
	}
	return f
}

// comfySeedInt 种子文本转 int64（超长/非数字返回 0）。
func comfySeedInt(s string) int64 {
	var n int64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil {
		return 0
	}
	return n
}

// comfyCommand 处理一条 ComfyUI 指令。
func (p *Plugin) comfyCommand(ev Event, c ComfyCommand, arg string) []Reply {
	qq := numToStr(ev.UserID)
	group := numToStr(ev.GroupID)

	switch c.Kind {
	case ComfyCmdDraw:
		return p.comfyDraw(ev, arg)

	case ComfyCmdViewModel:
		p.mu.RLock()
		cfg := p.cfg
		p.mu.RUnlock()
		cc := cfg.Comfy.withDefaults()
		ws := LoadComfyWorkflows(cfg.DataDir)
		// 卡片/合并转发用卡片态（0x100708BE），其余用文本态（0x100709C5）
		if mode := PublishMode(cfg.SendMode); mode == SendCard || mode == SendForward {
			card := RenderComfyModelsCard(ws, cc.CmdDraw, qq, ev.Nickname())
			return []Reply{{Card: card, Text: card}}
		}
		return []Reply{{Text: RenderComfyModels(ws, cc.CmdDraw)}}

	case ComfyCmdViewPreset:
		p.mu.RLock()
		presets := p.comfyPresets
		p.mu.RUnlock()
		name := strings.TrimSpace(arg)
		if name == "" {
			return []Reply{{Text: presets.Render()}}
		}
		txt, _ := presets.RenderDetail(name)
		return []Reply{{Text: txt}}

	case ComfyCmdUploadPreset:
		name, content := splitPresetArg(arg)
		if name == "" || content == "" {
			return []Reply{{Text: MsgSdPresetUsage}}
		}
		p.mu.Lock()
		msg := p.comfyPresets.Upload(name, content, ComfyPresetInfo(qq, group))
		path := p.comfyPresetPath
		presets := p.comfyPresets
		p.mu.Unlock()
		_ = SaveComfyPresets(path, presets)
		if msg != MsgComfyPresetUploaded {
			return []Reply{{Text: msg}}
		}
		// 0x1006EAC1 是 5 段拼接，成功时只回正文（不含单独的"M已上传"行）
		return []Reply{{Text: ComfyPresetUploadBody(name, content)}}

	case ComfyCmdDeletePreset:
		name, _ := splitNameContent(arg)
		if name == "" {
			return []Reply{{Text: MsgComfyPresetNotFound}}
		}
		p.mu.Lock()
		msg := p.comfyPresets.Delete(name)
		path := p.comfyPresetPath
		presets := p.comfyPresets
		p.mu.Unlock()
		_ = SaveComfyPresets(path, presets)
		return []Reply{{Text: msg}}
	}
	return nil
}

// comfyDraw 是 comfy画图 的指令壳：解析参数 -> 出图 -> 按发送模式回帖。
func (p *Plugin) comfyDraw(ev Event, arg string) []Reply {
	p.mu.RLock()
	cfg := p.cfg
	cardTmpl := p.comfyCardTmpl
	p.mu.RUnlock()

	args := ParseComfyArgs(arg)
	if args.Prompt == "" {
		return []Reply{{Text: MsgComfyNeedPrompt}}
	}
	res, err := p.DrawComfy(context.Background(), ComfyDrawOptions{
		QQ:        numToStr(ev.UserID),
		GroupID:   numToStr(ev.GroupID),
		Prompt:    args.Prompt,
		Params:    args.Params,
		ModelIdx:  args.ModelIdx,
		CFG:       args.CFG,
		Steps:     args.Steps,
		ImageURLs: ev.Images(),
	})
	if err != nil {
		return []Reply{{Text: err.Error()}}
	}
	if res.Rejected != "" {
		return []Reply{{Text: res.Rejected}}
	}
	if len(res.Images) == 0 {
		return []Reply{{Text: MsgComfyNoImage}}
	}

	meta := res.Meta
	meta.UserQQ = numToStr(ev.UserID)
	meta.BotQQ = numToStr(ev.SelfID)
	if meta.BotName == "" {
		meta.BotName = meta.BotQQ
	}

	prefix := ""
	if res.PresetHit != "" {
		prefix = res.PresetHit + "\r\n"
	}

	if mode := PublishMode(cfg.SendMode); mode == SendCard || mode == SendForward {
		fields := CardFieldsFromMeta(meta, numToStr(ev.GroupID), meta.Prompt,
			res.Caption, res.ImageURL)
		fields.Title = CardTitleComfy
		if card, err := FillCard(cardTmpl, fields); err == nil {
			return []Reply{{Card: card, Text: prefix + res.Caption}}
		}
	}

	out := make([]Reply, 0, len(res.Images))
	for i, b := range res.Images {
		text := ""
		if i == 0 {
			text = prefix + res.Caption
		}
		out = append(out, Reply{Image: b, Text: text})
	}
	return out
}
