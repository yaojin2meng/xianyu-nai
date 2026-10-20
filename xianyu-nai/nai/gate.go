package nai

import (
	"fmt"
	"strings"
	"sync"
)

// 本文件复刻机器人侧的门禁：违禁词、黑名单、积分。
//
// ---- 配置键（逐条来自 DLL）----
// 0x1039b89c sections.home.blockWords
// 0x1039b930 sections.home.pointsSwitch
// 0x1039b94c sections.home.basePoints
// 0x1039b968 sections.home.pointsType
// 0x1039b984 sections.home.pointsTip
// 0x1039b99c sections.home.rechargeCmd
// 0x1039b9b8 sections.home.queryPointsCmd
// 0x1039bad0 sections.home.addBlockCmd
// 0x1039baec sections.home.blacklistAddCmd
// 0x1039bb0c sections.home.blacklistDelCmd
// 0x1039bb2c sections.home.blSd
// 0x1039bb40 sections.home.blNai
// 0x1039bb54 sections.home.blUpscale
// 0x1039bb6c sections.home.blRemoveBg
// 0x1039bb88 sections.home.blUploadTag
// 0x1039bba4 sections.home.blUploadPreset
//
// ---- 文案（VA）----
// 0x102b7f1c "该指令包含屏蔽词条，已屏蔽该指令"
// 0x102b5b44 "该qq已添加黑名单"   0x102b5b7c "该qq不在黑名单"
// 0x102b5b70 "成功添加("          0x102b5b8c "成功移除("
// 0x102b5bac "已添加屏蔽词："
// 0x102b7fbc "当前画图点数不足"
// 0x102b7fd0 "点数不足提示"
// 0x102b7fe4 "，剩余点数："
// 0x102b7ff4 "(所需点数："
// 0x102b5b98 "屏蔽词条"
//
// ---- 数据文件（0x102b5b58 / 0x102b7d70 / 0x102b8000）----
// "黑名单.txt"      "user_cache.ini"
// "sd模型点数.json" "nai模型点数.json" "comfy模型点数.json"
// 0x102b8038 "点数消耗"
//
// 注意：0x102b7fac 的 "ò×z¾µãÊý²»×ã" 是 GBK 被按 UTF-8 误解码的乱码，
// 反转后是 "当前画图点数不足"，与 0x102b7fbc 是同一句（UTF-16/GBK 双份）。

// 门禁原文案。
const (
	MsgBlocked         = "该指令包含屏蔽词条，已屏蔽该指令" // 0x102b7f1c
	MsgBlacklisted     = "该qq已添加黑名单"        // 0x102b5b44
	MsgNotBlacklisted  = "该qq不在黑名单"         // 0x102b5b7c
	MsgAddBlacklistOK  = "成功添加("            // 0x102b5b70
	MsgDelBlacklistOK  = "成功移除("            // 0x102b5b8c
	MsgBlockWordAdded  = "已添加屏蔽词："          // 0x102b5bac
	MsgPointsNotEnough = "当前画图点数不足"         // 0x102b7fbc
	MsgPointsTipLabel  = "点数不足提示"           // 0x102b7fd0
)

// 数据文件名。
const (
	FileBlacklist  = "黑名单.txt"        // 0x102b5b58
	FileUserCache  = "user_cache.ini" // 0x102b7d70
	FileNaiPoints  = "nai模型点数.json"   // 0x102b8010
	FileSdPoints   = "sd模型点数.json"    // 0x102b8000
	FilePointsCost = "点数消耗"           // 0x102b8038
)

// Reject 表示一次请求被门禁拦下。
type Reject struct {
	Kind string // blockword / blacklist / points
	Text string
}

func (r *Reject) Error() string { return r.Text }

// ParseBlockWords 把配置里的屏蔽词串拆成列表。
// 插件在 0x102b5b98 处按"屏蔽词条"分组存储，
// 分隔符未见明确证据，这里兼容常见几种。
func ParseBlockWords(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	f := func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == '\n' || r == '\r' || r == '|'
	}
	var out []string
	for _, s := range strings.FieldsFunc(raw, f) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// HitBlockWord 返回命中的屏蔽词（空串表示未命中）。
// 子串匹配、忽略大小写；未确认插件是否区分大小写。
func HitBlockWord(prompt string, words []string) string {
	low := strings.ToLower(prompt)
	for _, w := range words {
		if w == "" {
			continue
		}
		if strings.Contains(low, strings.ToLower(w)) {
			return w
		}
	}
	return ""
}

// CheckPrompt 是绘图前的第一道门禁。
func (c *Client) CheckPrompt(prompt string) *Reject {
	if w := HitBlockWord(prompt, ParseBlockWords(c.cfg.BlockWords)); w != "" {
		return &Reject{Kind: "blockword", Text: MsgBlocked + "（命中：" + w + "）"}
	}
	return nil
}

// Blacklist 是按功能分开的黑名单（blSd/blNai/blUpscale/blRemoveBg/blUploadTag/blUploadPreset）。
type Blacklist map[string][]string

// BlacklistScope 是黑名单的作用域常量。
const (
	ScopeNai = "blNai"
	ScopeSd  = "blSd"
	// ScopeComfy 是 ComfyUI 通道的黑名单（DLL 里 comfy 画图的黑名单作用域与
	// nai/sd 并列，具体键名未在字符串表出现，按命名惯例取 blComfy）。
	ScopeComfy        = "blComfy"
	ScopeUpscale      = "blUpscale"
	ScopeRemoveBg     = "blRemoveBg"
	ScopeUploadTag    = "blUploadTag"
	ScopeUploadPreset = "blUploadPreset"
)

// IsBlacklisted 判定某 QQ 是否在指定作用域的黑名单里。
func (b Blacklist) IsBlacklisted(scope, qq string) bool {
	qq = strings.TrimSpace(qq)
	if qq == "" {
		return false
	}
	for _, v := range b[scope] {
		if strings.TrimSpace(v) == qq {
			return true
		}
	}
	return false
}

// Add 添加黑名单，返回给用户的回执。
func (b Blacklist) Add(scope, qq string) string {
	if b.IsBlacklisted(scope, qq) {
		return MsgBlacklisted
	}
	b[scope] = append(b[scope], qq)
	return MsgAddBlacklistOK + qq + ")" // 0x102b5b68 ")" + 0x102b5b70
}

// Del 移除黑名单。
func (b Blacklist) Del(scope, qq string) string {
	list := b[scope]
	for i, v := range list {
		if strings.TrimSpace(v) == qq {
			b[scope] = append(list[:i], list[i+1:]...)
			return MsgDelBlacklistOK + qq + ")"
		}
	}
	return MsgNotBlacklisted
}

// ---------- 积分 ----------

// PointsType 是计费归属方式（0x1039b968 sections.home.pointsType）。
//
// 插件把该项当成文本读入（0x100A18FB-0x100A1918，方法 +0xcc）。
// 二进制中与"点数"同族的取值文本只有：
//
//	0x1039aca8 "个人"   0x1039ac8c "全局"
//
// 而 0x1039b930 sections.home.pointsSwitch 是总开关。
// 注意：插件里不存在 "按次"/"按模型" 两个字面量（全库字节搜索无命中），
// 这两项是早期推测值，已删除。
type PointsType string

const (
	PointsGlobal   PointsType = "全局" // 0x1039ac8c
	PointsPersonal PointsType = "个人" // 0x1039aca8
)

// 积分相关配置键（VA 逐一确认；指令类键在 commands.go）。
const (
	CfgPointsSwitch = "sections.home.pointsSwitch" // 0x1039b930
	CfgBasePoints   = "sections.home.basePoints"   // 0x1039b94c
	CfgPointsType   = "sections.home.pointsType"   // 0x1039b968
	CfgPointsTip    = "sections.home.pointsTip"    // 0x1039b984
)

// PointsBook 是最小可用积分账本。
// 插件把余额记在数据键 "naiPoints"/"naiModelPoints"，
// 模型单价在 nai模型点数.json（0x102b8010）。
type PointsBook struct {
	mu      sync.Mutex
	balance map[string]int64
	price   map[string]int64
	base    int64
}

// NewPointsBook 建账本。price 为空时用 base 统一计价。
func NewPointsBook(base int64, price map[string]int64) *PointsBook {
	if price == nil {
		price = map[string]int64{}
	}
	return &PointsBook{balance: map[string]int64{}, price: price, base: base}
}

func (p *PointsBook) Balance(qq string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.balance[qq]
}

func (p *PointsBook) Add(qq string, delta int64) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.balance[qq] += delta
	if p.balance[qq] < 0 {
		p.balance[qq] = 0
	}
	return p.balance[qq]
}

// Cost 返回某模型单次价格。
func (p *PointsBook) Cost(model string) int64 {
	if v, ok := p.price[model]; ok && v > 0 {
		return v
	}
	return p.base
}

// Charge 扣费；余额不足返回 *Reject 且不动账。
func (p *PointsBook) Charge(qq, model string) (int64, error) {
	cost := p.Cost(model)
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.balance[qq]
	if cur < cost {
		return cur, &Reject{Kind: "points", Text: PointsShortText(cost, cur)}
	}
	p.balance[qq] = cur - cost
	return p.balance[qq], nil
}

// PointsShortText 复刻点数不足文案。
// 0x102b7fe4 "，剩余点数：" + 0x102b7ff4 "(所需点数："
func PointsShortText(cost, left int64) string {
	return fmt.Sprintf("%s，剩余点数：%d(所需点数：%d)", MsgPointsNotEnough, left, cost)
}

// PointsReceipt 复刻积分查询回执。
func PointsReceipt(qq string, bal int64, tip string) string {
	var b strings.Builder
	b.WriteString("===【积分信息】===\r\n")
	fmt.Fprintf(&b, "QQ：%s\r\n", qq)
	fmt.Fprintf(&b, "剩余积分：%d\r\n", bal)
	if tip != "" {
		fmt.Fprintf(&b, "提示：%s\r\n", tip)
	}
	return b.String()
}
