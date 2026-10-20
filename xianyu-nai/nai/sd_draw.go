package nai

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// 本文件是 "sd画图" 的编排：指令参数 -> SD 请求 -> 图片/卡片投递。
//
// 出图入口按消息段类型分流（0x100236C1 的 [ebp-0x34] 与 0x102B7F70 "text" 比较）：
// text -> /sdapi/v1/txt2img，其它（image）-> /sdapi/v1/img2img。
//
// 结果正文用 sd 专用拼接（0x102B8860 "![sd #" + 0x102B8574 "px](" ...）：
//
//	![sd #<宽>px #<高>px](<图链>)
//	[🔗下载图片](<urls[0]>)
//	```<info.prompt>```
//	# seed:<种子>
//	# 采样方式:<采样>
//	# 模型:<模型>

// SdDrawOptions 是一次 sd画图 的输入。
type SdDrawOptions struct {
	QQ      string
	GroupID string
	Prompt  string
	// 用户消息里带的图片地址（非空即 img2img）
	ImageURL string
}

// SdDrawResult 是 sd画图 的产出。
type SdDrawResult struct {
	Images   [][]byte
	ImageURL string
	Caption  string
	Card     string
	Meta     ResultMeta
	Rejected string
}

// DrawSD 执行一次 SD 出图。
func (p *Plugin) DrawSD(ctx context.Context, o SdDrawOptions) (*SdDrawResult, error) {
	p.mu.RLock()
	cfg := p.cfg
	state := p.sdState
	cache := p.sdCache
	presets := p.sdPresets
	cardTmpl := p.sdCardTmpl
	p.mu.RUnlock()

	sc := cfg.SD.withDefaults()
	if sc.SDBaseURL() == "" {
		return nil, fmt.Errorf("%s", MsgSDNotConfigured) // 0x102B5D20
	}

	// 黑名单（blSd）
	p.mu.RLock()
	black := p.pipe != nil && p.pipe.Gate != nil && p.pipe.Gate.Blacklist.IsBlacklisted(ScopeSd, o.QQ)
	p.mu.RUnlock()
	if black {
		return &SdDrawResult{Rejected: MsgSdBlacklisted + o.QQ}, nil
	}

	out := &SdDrawResult{}

	// 完成后冷却（0x102B7D58；键按 完成冷却模式 取个人 QQ 或群号）
	if p.cool != nil {
		if left := p.cool.Check("sd", CooldownKey(sc.CooldownMode, o.QQ, o.GroupID),
			SdCooldownSeconds(sc.Cooldown)); left > 0 {
			return &SdDrawResult{Rejected: RenderCooldown(left)}, nil
		}
	}

	// 预设/标签替换
	prompt := o.Prompt
	if len(presets) > 0 {
		for _, it := range presets {
			if it.Name != "" && strings.Contains(prompt, it.Name) {
				prompt = strings.ReplaceAll(prompt, it.Name, " ")
				if it.Content != "" {
					prompt = it.Content + ", " + prompt
				}
			}
		}
	}

	// # 参数（0x102B7D9C 起同一套正则）
	ov, prompt := ParseOverrides(prompt)
	c := New(cfg)
	opts := SDOptions{
		Prompt:  prompt,
		Seed:    sc.Seed,
		Steps:   sc.Steps,
		CFG:     sc.Identity,
		Width:   sc.Width,
		Height:  sc.Height,
		Sampler: sc.Sampler,
	}
	if state != nil {
		opts.Seed = state.Seed(o.QQ, sc.Seed)
	}
	// 用户负面词条优先，其次 sd参数/负面词条
	if state != nil {
		opts.Negative = state.Negative(o.QQ, sc.Negative)
	} else {
		opts.Negative = sc.Negative
	}

	// SD 通道只做中文翻译，保留 <lora:name:weight> 原样（SD 需要这个标签本身）；
	// nai 通道的 ParseLoras 会把标签改写成 "name:0.8>"，不能用在 SD 上。
	if t, err := c.TranslatePrompt(ctx, opts.Prompt); err == nil && strings.TrimSpace(t) != "" {
		opts.Prompt = t
	}
	if strings.TrimSpace(opts.Negative) == "" {
		opts.Negative = sc.Negative
	}

	if ov.Steps != nil && *ov.Steps > 0 {
		opts.Steps = *ov.Steps
	}
	if ov.Sampler != nil {
		opts.Sampler = *ov.Sampler
	}
	if ov.Guidance != nil {
		opts.CFG = *ov.Guidance
	}
	if ov.Width != nil && *ov.Width > 0 {
		opts.Width = *ov.Width
	}
	if ov.Height != nil && *ov.Height > 0 {
		opts.Height = *ov.Height
	}
	if ov.Resolution != nil {
		opts.Width = ov.Resolution.Width
		opts.Height = ov.Resolution.Height
	}
	// #模型N / #vaeN 先查列表再取名字
	if ov.ModelIdx != nil && c.SDAvailable() {
		if items, err := c.SDModelsCached(ctx, cache); err == nil {
			if v, ok := ResolveSDModel(items, *ov.ModelIdx); ok {
				opts.Model = v
			}
		}
	}
	if ov.VAEIdx != nil && c.SDAvailable() {
		if items, err := c.SDVAEsCached(ctx); err == nil {
			if v, ok := ResolveSDVAE(items, *ov.VAEIdx); ok {
				opts.VAE = v
			}
		}
	}

	// 图生图底图
	if strings.TrimSpace(o.ImageURL) != "" {
		b64, err := c.FetchImageBase64(ctx, o.ImageURL)
		if err != nil {
			return nil, err
		}
		opts.InitImage = b64
	}

	res, err := c.SDGenerate(ctx, opts)
	if err != nil {
		return nil, err
	}
	out.ImageURL = res.ImageURL

	for _, s := range res.Images {
		if strings.TrimSpace(s) == "" {
			continue
		}
		b, err := DecodeOfficialImage(s)
		if err != nil {
			return nil, err
		}
		out.Images = append(out.Images, b)
	}
	if len(out.Images) == 0 && res.ImageURL == "" {
		return nil, fmt.Errorf("%s", MsgSDDrawFailed)
	}

	// 没有直链就把图传上去换直链（与 nai 通道同一上传函数）
	if out.ImageURL == "" && len(out.Images) > 0 {
		if u, err := c.EnsureImageURL(ctx, out.Images[0], ""); err == nil {
			out.ImageURL = u
		}
	}

	model := res.Model
	if model == "" {
		model = opts.Model
	}
	sampler := res.Sampler
	if sampler == "" {
		sampler = opts.Sampler
	}
	out.Meta = ResultMeta{
		Seed:    res.Seed,
		Model:   model,
		Sampler: sampler,
		Steps:   opts.Steps,
		Scale:   opts.CFG,
		Width:   opts.Width,
		Height:  opts.Height,
		UserQQ:  o.QQ,
		RawURL:  res.ImageURL, // urls[0]，正文第二行用它
		Prompt:  res.Prompt,   // info.prompt，非空时正文用 ``` 包一段
	}
	out.Caption = BuildSdCaption(out.Meta, out.ImageURL)

	// 发送模式：json/卡片跳转 走 sdjson.json 模板，其余直接发图
	mode := PublishMode(cfg.SendMode)
	if mode == SendCard || mode == SendForward {
		fields := CardFieldsFromMeta(out.Meta, o.GroupID, o.Prompt,
			out.Caption, out.ImageURL)
		fields.Title = CardTitleSd // 0x102B88A4
		card, err := FillCard(cardTmpl, fields)
		if err == nil {
			out.Card = card
		}
	}
	// 出图成功才记冷却起点（与"完成后冷却"的字面语义一致）
	if p.cool != nil {
		p.cool.Touch("sd", CooldownKey(sc.CooldownMode, o.QQ, o.GroupID))
	}
	return out, nil
}

// SdSeedReceipt 复刻 "已更换sd种子为" + 数值（0x102B7CC8）。
func SdSeedReceipt(v string) string {
	return MsgSdSeedSet + v
}

// SdNegativeReceipt 复刻 "已更换sd负面词条为：" + 内容（0x102B7CE8）。
func SdNegativeReceipt(v string) string {
	return MsgSdNegSet + v
}

// SdViewNegativeText 复刻查看负面：有自定义用 "你的…"，否则 "默认…"。
func SdViewNegativeText(yours, def string, has bool) string {
	if has {
		return MsgSdNegYours + yours // 0x102B7D02
	}
	return MsgSdNegDefault + def // 0x102B7D1E
}

// SdSeedArg 解析 sd种子 的参数，非法返回 -1。
func SdSeedArg(arg string) (string, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", false
	}
	if _, err := strconv.ParseInt(arg, 10, 64); err != nil {
		return "", false
	}
	return arg, true
}
