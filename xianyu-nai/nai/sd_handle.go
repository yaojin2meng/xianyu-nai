package nai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// numToStr 把 QQ / 群号转成文本。
func numToStr(v int64) string { return strconv.FormatInt(v, 10) }

// 本文件是 SD 指令的分派与回执（指令表见 sd_cmd.go）。

// sdCommand 处理一条 SD 指令。
func (p *Plugin) sdCommand(ev Event, c SDCommand, arg string) []Reply {
	qq := numToStr(ev.UserID)
	group := numToStr(ev.GroupID)

	switch c.Kind {
	case SdCmdDraw:
		return p.sdDraw(ev, arg)

	case SdCmdSeed:
		v, ok := SdSeedArg(arg)
		if !ok {
			return []Reply{{Text: "请输入数字种子"}}
		}
		p.mu.Lock()
		st := p.sdState
		p.mu.Unlock()
		if st != nil {
			_ = st.SetSeed(qq, v)
		}
		return []Reply{{Text: SdSeedReceipt(v)}}

	case SdCmdViewNegative:
		p.mu.RLock()
		st, sc := p.sdState, p.cfg.SD.withDefaults()
		p.mu.RUnlock()
		if st == nil {
			return []Reply{{Text: SdViewNegativeText("", sc.Negative, false)}}
		}
		has := st.HasNegative(qq)
		return []Reply{{Text: SdViewNegativeText(st.Negative(qq, ""), sc.Negative, has)}}

	case SdCmdSetNegative:
		p.mu.Lock()
		st := p.sdState
		p.mu.Unlock()
		if st == nil {
			return nil
		}
		_ = st.SetNegative(qq, arg)
		return []Reply{{Text: SdNegativeReceipt(arg)}}

	case SdCmdViewModel:
		items, err := p.client.SDModels(context.Background())
		if err != nil {
			return []Reply{{Text: err.Error()}}
		}
		return []Reply{{Text: RenderSDModels(items)}}

	case SdCmdViewVAE:
		items, err := p.client.SDVAEs(context.Background())
		if err != nil {
			return []Reply{{Text: err.Error()}}
		}
		return []Reply{{Text: RenderSDVAEs(items)}}

	case SdCmdViewLora:
		items, err := p.client.SDLoRAs(context.Background())
		if err != nil {
			return []Reply{{Text: err.Error()}}
		}
		return []Reply{{Text: RenderSDLoRAs(items)}}

	case SdCmdRefreshLora:
		if err := p.client.SDRefreshLoRAs(context.Background()); err != nil {
			return []Reply{{Text: err.Error()}}
		}
		items, err := p.client.SDLoRAs(context.Background())
		if err != nil {
			return []Reply{{Text: err.Error()}}
		}
		return []Reply{{Text: RenderSDLoRAs(items)}}

	case SdCmdLoadLora:
		return []Reply{{Text: p.loadLocalLoras()}}

	case SdCmdViewPreset:
		p.mu.RLock()
		list := p.sdPresets
		p.mu.RUnlock()
		// "指令+预设名查看提示词"：带名字看单条，不带名字看列表。
		if name := strings.TrimSpace(arg); name != "" {
			it, ok := list.Find(name)
			if !ok {
				return []Reply{{Text: MsgSdPresetNotFound}}
			}
			return []Reply{{Text: RenderSDPresetDetail(it)}}
		}
		return []Reply{{Text: list.Render()}}

	case SdCmdUploadPreset:
		name, content := splitPresetArg(arg)
		if name == "" || content == "" {
			return []Reply{{Text: MsgSdPresetUsage}}
		}
		p.mu.Lock()
		msg := p.sdPresets.Upload(name, content)
		path := p.sdPresetPath
		_ = saveSDPresets(path, p.sdPresets)
		p.mu.Unlock()
		if msg != MsgSdPresetUploaded {
			return []Reply{{Text: msg}}
		}
		// 0x10043A0F 的 5 段拼接：
		//   "# [sd预设]已上传预设内容\r\n# 预设名：" + 名 + "\r\n```\r\n" + 内容 + "\r\n```"
		return []Reply{{Text: MsgSdPresetBody + name + "\r\n```\r\n" + content + "\r\n```"}}

	case SdCmdDeletePreset:
		name, _ := splitNameContent(arg)
		p.mu.Lock()
		msg := p.sdPresets.Delete(name)
		path := p.sdPresetPath
		_ = saveSDPresets(path, p.sdPresets)
		p.mu.Unlock()
		return []Reply{{Text: msg}}
	}
	_ = group
	return nil
}

// sdDraw 是 sd画图 的指令入口：出图后按发送模式投递。
func (p *Plugin) sdDraw(ev Event, arg string) []Reply {
	imgURL := ""
	if imgs := ev.Images(); len(imgs) > 0 {
		imgURL = imgs[0]
	}
	res, err := p.DrawSD(context.Background(), SdDrawOptions{
		QQ:       numToStr(ev.UserID),
		GroupID:  numToStr(ev.GroupID),
		Prompt:   arg,
		ImageURL: imgURL,
	})
	if err != nil {
		return []Reply{{Text: err.Error()}}
	}
	if res.Rejected != "" {
		return []Reply{{Text: res.Rejected}}
	}
	if len(res.Images) == 0 && res.ImageURL == "" {
		return []Reply{{Text: MsgSDDrawFailed}}
	}
	if res.Card != "" {
		return []Reply{{Card: res.Card, Text: res.Caption}}
	}
	out := make([]Reply, 0, len(res.Images))
	for i, b := range res.Images {
		text := ""
		if i == 0 {
			text = res.Caption
		}
		out = append(out, Reply{Image: b, Text: text})
	}
	if len(out) == 0 {
		out = append(out, Reply{Text: res.Caption})
	}
	return out
}

// splitPresetArg 解析 " 我的预设=预设内容"（0x102B613C 的用法示例）。
//
// 证据：0x10043090 与 0x100430E3 两次拿 "=" (0x102B9420) 去切同一段文本，
// 结果分别写进 [ebp-0x34] 与 [ebp-0x30]（前者是预设名、后者是内容）。
func splitPresetArg(s string) (string, string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "="); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// loadLocalLoras 复刻 "加载lora"。
//
// 证据（0x1003E0FC-0x1003E17C）：
//   - 分支条件是 文本 == "加载lora"（0x102B61A8）
//   - 读 {数据目录}\loras.json（0x102B8054）
//   - 解析后整个对象写进全局 0x104EBECC，随后跳到公共收尾 0x1003EF07
//
// 也就是说原插件只把本地 lora 表读进内存（供 lora 名字映射），不回复文本。
// 重制版没有那个全局槽位，改为把条目回显给用户，回执措辞属推定。
func (p *Plugin) loadLocalLoras() string {
	p.mu.RLock()
	dir := p.cfg.DataDir
	p.mu.RUnlock()
	path := joinDir(dir, FileLocalLoras)
	b, err := os.ReadFile(path)
	if err != nil {
		return MsgSdLocalLoraMissing
	}
	var list []SDLora
	if err := json.Unmarshal(b, &list); err != nil {
		return MsgSdLocalLoraMissing
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s%d\r\n", MsgSdLocalLoraLoaded, len(list))
	for _, it := range list {
		n := it.Name
		if it.Alias != "" {
			n = it.Alias
		}
		sb.WriteString(n + "\r\n")
	}
	return sb.String()
}

// FileLocalLoras 是本地 lora 映射表（0x102B8054）。
const FileLocalLoras = "loras.json"

// sd 本地 lora 回执。
const (
	MsgSdLocalLoraLoaded  = "已加载本地lora：" // 0x102B96E8 附近的提示语（措辞按常量推定）
	MsgSdLocalLoraMissing = "未找到loras.json，请先把lora文件放进插件目录"
)

// saveSDPresets 写回 sd预设列表.json。
func saveSDPresets(path string, list SDPresetFile) error {
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
