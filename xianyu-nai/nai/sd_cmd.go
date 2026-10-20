package nai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 本文件把 SD 通道接到指令表与插件实例上。
//
// ---- SD 指令（配置节 sd指令 0x1039ACC0；默认词来自 0x102B5CC8 起）----
//
//	sd指令/画图       = "sd画图"      0x102B5CC8（通用别名 "画图" 0x1039AA40）
//	sd指令/修改种子   = "sd种子"      0x1039ACC8
//	sd指令/修改负面   = "sd修改负面"  0x1039ACD0
//	sd指令/查看负面   = "sd查看负面"  0x1039ACDC
//	sd指令/查看lora   = "sd查看lora"  0x1039ACE8
//	sd指令/查看模型   = "sd查看模型"  0x1039ACF4
//	sd指令/查看vae    = "sd查看vae"   0x1039AD00
//	sd指令/查看预设   = "sd查看预设"  0x102B9834
//	sd指令/上传预设   = "sd上传预设"  0x102B6164
//	sd指令/删除预设   = "sd删除预设"  0x102B6180
//
// 固定词（帮助文本 0x102B6190 起）：
//
//	"加载lora" 0x102B61A8   "刷新lora" 0x102B61CC
//
// ---- 每用户状态 ----
//
// 种子：seed.ini   [sd] <QQ> = 种子      （0x1001428C 起）
// 负面：Negative.ini [sd] <QQ> = 负面词条（0x1001463D 起）
//
// 取不到时回退到 sd参数/默认种子、sd参数/负面词条。

// SD 指令默认词。
const (
	CmdSDDraw         = "sd画图"     // 0x102B5CC8
	CmdSDSeed         = "sd种子"     // 0x1039ACC8
	CmdSDSetNegative  = "sd修改负面"   // 0x1039ACD0
	CmdSDViewNegative = "sd查看负面"   // 0x1039ACDC
	CmdSDViewLora     = "sd查看lora" // 0x1039ACE8
	CmdSDViewModel    = "sd查看模型"   // 0x1039ACF4
	CmdSDViewVAE      = "sd查看vae"  // 0x1039AD00
	CmdSDViewPreset   = "sd查看预设"   // 0x102B9834
	CmdSDUploadPreset = "sd上传预设"   // 0x102B6164
	CmdSDDeletePreset = "sd删除预设"   // 0x102B6180
	CmdSDLoadLora     = "加载lora"   // 0x102B61A8
	CmdSDRefreshLora  = "刷新lora"   // 0x102B61CC
)

// 关联文案。
const (
	MsgSdBlacklisted = "黑名单禁止使用sd画图："   // 0x102B5CD0
	MsgSdSeedSet     = "已更换sd种子为"       // 0x102B7CC8
	MsgSdNegSet      = "已更换sd负面词条为："    // 0x102B7CE8
	MsgSdNegYours    = "你的sd负面词条如下："    // 0x102B7D02
	MsgSdNegDefault  = "默认sd负面词条如下："    // 0x102B7D1E
	MsgSdCooldown    = "画图冷却中，剩余冷却 "    // 0x102B7D58
	MsgSdBusy        = "上一张画图还在处理中，请稍后" // 0x102B7D38（措辞按常量推定）

	// sd 预设回执（0x102B9904 起）
	MsgSdPresetExists   = "[sd预设]已存在相同预设，请修改后上传" // 0x102B9904
	MsgSdPresetUploaded = "[sd预设]已上传预设内容"        // 0x102B9984
	MsgSdPresetDeleted  = "[sd预设]已删除该预设内容"       // 0x102B999C
	MsgSdPresetNotFound = "[sd预设]未找到该预设内容"       // 0x102B99B8

	// sd 预设上传正文（0x102B995C：'# [sd预设]已上传预设内容\r\n# 预设名：'）
	MsgSdPresetBody = "# [sd预设]已上传预设内容\r\n# 预设名："

	// 上传预设的参数示例（0x102B613C：' 我的预设=预设内容'）
	MsgSdPresetUsage = "请按「sd上传预设 我的预设=预设内容」的格式上传"
)

// sd 数据文件。
const (
	FileSdPresetJSON = "sd预设列表.json"  // 0x102B994C
	FileSeedINI      = "seed.ini"     // 0x102B7CBC
	FileNegativeINI  = "Negative.ini" // 0x102B7CD8
)

// SDCommandKind 是 SD 指令动作。
type SDCommandKind string

const (
	SdCmdDraw         SDCommandKind = "sdDraw"
	SdCmdSeed         SDCommandKind = "sdSeed"
	SdCmdSetNegative  SDCommandKind = "sdSetNegative"
	SdCmdViewNegative SDCommandKind = "sdViewNegative"
	SdCmdViewLora     SDCommandKind = "sdViewLora"
	SdCmdViewModel    SDCommandKind = "sdViewModel"
	SdCmdViewVAE      SDCommandKind = "sdViewVAE"
	SdCmdViewPreset   SDCommandKind = "sdViewPreset"
	SdCmdUploadPreset SDCommandKind = "sdUploadPreset"
	SdCmdDeletePreset SDCommandKind = "sdDeletePreset"
	SdCmdLoadLora     SDCommandKind = "sdLoadLora"
	SdCmdRefreshLora  SDCommandKind = "sdRefreshLora"
)

// SDCommand 是 SD 指令定义。
type SDCommand struct {
	Kind    SDCommandKind
	Config  string
	Default string
	Aliases []string
}

// SDCommands 是 SD 指令表（配置节 sd指令）。
//
// 说明：DLL 里这些指令按"渠道分组"分派（groups[0]/[1]/[2]），所以 SD 与 NAI
// 可以用同一个别名（如 "画图"、"查看预设"）。重制版是单一分派口，为避免与
// nai 指令表抢前缀，SD 表只用带 sd 前缀的主名；别名仍记录在注释里，
// 需要时可通过 ini 把 sd指令/画图 改成想要的值。
var SDCommands = []SDCommand{
	{SdCmdDraw, "sd指令/画图", CmdSDDraw, nil},                   // 别名 "画图" 0x1039AA40
	{SdCmdSeed, "sd指令/修改种子", CmdSDSeed, nil},                 // 别名 "修改种子" 0x1039AA8C
	{SdCmdSetNegative, "sd指令/修改负面", CmdSDSetNegative, nil},   // 别名 "修改负面" 0x1039AABC
	{SdCmdViewNegative, "sd指令/查看负面", CmdSDViewNegative, nil}, // 别名 "查看负面" 0x1039AAA4
	{SdCmdViewLora, "sd指令/查看lora", CmdSDViewLora, nil},
	{SdCmdViewModel, "sd指令/查看模型", CmdSDViewModel, nil},
	{SdCmdViewVAE, "sd指令/查看vae", CmdSDViewVAE, nil}, // 别名 "查看vae" 0x102B9680
	{SdCmdViewPreset, "sd指令/查看预设", CmdSDViewPreset, nil},
	{SdCmdUploadPreset, "sd指令/上传预设", CmdSDUploadPreset, nil},
	{SdCmdDeletePreset, "sd指令/删除预设", CmdSDDeletePreset, nil},
	{SdCmdLoadLora, "sd指令/加载lora", CmdSDLoadLora, nil},
	{SdCmdRefreshLora, "sd指令/刷新lora", CmdSDRefreshLora, nil},
}

// MatchSDCommand 在 SD 指令表里做前缀匹配。
// 注意顺序：更长的词必须先于更短的词（"sd查看负面" 先于 "查看负面"）。
func MatchSDCommand(text string) (SDCommand, string, bool) {
	text = trimLeftSpace(text)
	var best SDCommand
	var bestWord string
	for _, c := range SDCommands {
		words := append([]string{c.Default}, c.Aliases...)
		for _, w := range words {
			if w == "" || !hasPrefixFold(text, w) {
				continue
			}
			if len(w) > len(bestWord) {
				best, bestWord = c, w
			}
		}
	}
	if bestWord == "" {
		return SDCommand{}, "", false
	}
	return best, bestWord, true
}

// ---------- 每用户状态 ----------

// SDUserState 管理 seed.ini / Negative.ini 里的每用户 SD 设置。
type SDUserState struct {
	Dir string // 数据目录
	mu  sync.Mutex
}

func (s *SDUserState) seedPath() string {
	return joinDir(s.Dir, FileSeedINI)
}

func (s *SDUserState) negPath() string {
	return joinDir(s.Dir, FileNegativeINI)
}

func joinDir(dir, name string) string {
	if dir == "" {
		return name
	}
	d := strings.TrimRight(dir, "/\\")
	return d + "/" + name
}

// Seed 读某用户的种子，缺省返回 def。
func (s *SDUserState) Seed(qq, def string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := LoadIni(s.seedPath())
	return f.Get("sd", qq, def)
}

// SetSeed 写某用户的种子。
func (s *SDUserState) SetSeed(qq, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := LoadIni(s.seedPath())
	f.Set("sd", qq, v)
	return f.Save()
}

// Negative 读某用户的负面词条，缺省返回 def。
func (s *SDUserState) Negative(qq, def string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := LoadIni(s.negPath())
	return f.Get("sd", qq, def)
}

// HasNegative 报告该用户是否自定义过负面词条。
func (s *SDUserState) HasNegative(qq string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := LoadIni(s.negPath())
	return f.Has("sd", qq)
}

// SetNegative 写某用户的负面词条。
func (s *SDUserState) SetNegative(qq, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := LoadIni(s.negPath())
	f.Set("sd", qq, v)
	return f.Save()
}

// ---------- 插件侧状态 ----------

// sdModelCache 缓存 /sdapi/v1/sd-models 的结果，供 #模型N 解析。
type sdModelCache struct {
	mu    sync.Mutex
	items []SDModel
	at    time.Time
}

// SDModelsCached 取模型列表（带 60 秒缓存）。
func (c *Client) SDModelsCached(ctx context.Context, cache *sdModelCache) ([]SDModel, error) {
	if cache != nil {
		cache.mu.Lock()
		if len(cache.items) > 0 && time.Since(cache.at) < time.Minute {
			out := cache.items
			cache.mu.Unlock()
			return out, nil
		}
		cache.mu.Unlock()
	}
	items, err := c.SDModels(ctx)
	if err != nil {
		return nil, err
	}
	if cache != nil {
		cache.mu.Lock()
		cache.items, cache.at = items, time.Now()
		cache.mu.Unlock()
	}
	return items, nil
}

// SDVAEsCached 取 VAE 列表（不带缓存，VAE 数量少、变更是用户主动行为）。
func (c *Client) SDVAEsCached(ctx context.Context) ([]SDVAE, error) {
	return c.SDVAEs(ctx)
}

// ResolveSDModel 把 #模型N（1 基）解析成 sd_model_checkpoint 的值。
// 取 title（SD WebUI 的 title 形如 "name [hash]"），没有 title 则用 model_name。
func ResolveSDModel(items []SDModel, idx int) (string, bool) {
	i := idx - 1
	if i < 0 || i >= len(items) {
		return "", false
	}
	if items[i].Title != "" {
		return items[i].Title, true
	}
	if items[i].ModelName != "" {
		return items[i].ModelName, true
	}
	return "", false
}

// ResolveSDVAE 把 #vaeN（1 基）解析成 override_settings.sd_vae 的值。
func ResolveSDVAE(items []SDVAE, idx int) (string, bool) {
	i := idx - 1
	if i < 0 || i >= len(items) {
		return "", false
	}
	if items[i].ModelName != "" {
		return items[i].ModelName, true
	}
	if items[i].FileName != "" {
		return items[i].FileName, true
	}
	return "", false
}

// ---------- SD 预设 ----------

// SDPreset 对应 sd预设列表.json 的元素（字段与 nai 预设同构）。
type SDPreset struct {
	Name    string `json:"名"`
	Content string `json:"内容"`
}

// SDPresetFile 是 sd预设列表.json。
type SDPresetFile []SDPreset

// LoadSDPresets 读预设文件；不存在按空处理。
func LoadSDPresets(path string) SDPresetFile {
	var out SDPresetFile
	b, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return out
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return SDPresetFile{}
	}
	return out
}

// Render 复刻 sd查看预设 的列表正文。
//
// 证据（0x10041D01 的 3 段循环拼接 + 0x10041D5D 的 3 段收尾，均"最后压栈的在最前"）：
//
//	逐项：累加串 + 预设名 + "\r\n"
//	收尾："# 预设列表(指令+预设名查看提示词)\r\n```\r\n" + 累加串 + "\r\n```"
func (p SDPresetFile) Render() string {
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

// RenderDetail 复刻"预设名查看提示词"的单条正文（0x10040E38 的 6 段拼接）：
//
//	"# " + 预设名 + "\r\n\r\n```\r\n" + 预设内容 + "\r\n```\r\n\r\n"
func RenderSDPresetDetail(it SDPreset) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(it.Name)
	b.WriteString("\r\n\r\n```\r\n")
	b.WriteString(it.Content)
	b.WriteString("\r\n```\r\n\r\n")
	return b.String()
}

// RenderPlain 复刻纯文本/合并转发形态（0x10040F77 的 6 段 + 0x10042865 的 2 段）：
//
//	"预设列表(指令+预设名查看提示词)\r\n==========\r\n" + 逐项
//	逐项 = "预设名" + 预设名 + "\r\n预设内容=\r\n" + 预设内容 + "\r\n\r\n"
func (p SDPresetFile) RenderPlain() string {
	var b strings.Builder
	b.WriteString("预设列表(指令+预设名查看提示词)\r\n==========\r\n")
	for _, it := range p {
		b.WriteString("预设名")
		b.WriteString(it.Name)
		b.WriteString("\r\n预设内容=\r\n")
		b.WriteString(it.Content)
		b.WriteString("\r\n\r\n")
	}
	return b.String()
}

// Find 按名字取预设内容（"指令+预设名查看提示词"）。
func (p SDPresetFile) Find(name string) (SDPreset, bool) {
	name = strings.TrimSpace(name)
	for _, it := range p {
		if it.Name == name {
			return it, true
		}
	}
	return SDPreset{}, false
}

// Upload 新增预设，重名返回 MsgSdPresetExists。
func (p *SDPresetFile) Upload(name, content string) string {
	for _, it := range *p {
		if it.Name == name {
			return MsgSdPresetExists
		}
	}
	*p = append(*p, SDPreset{Name: name, Content: content})
	return MsgSdPresetUploaded
}

// Delete 删除预设。
func (p *SDPresetFile) Delete(name string) string {
	for i, it := range *p {
		if it.Name == name {
			*p = append((*p)[:i], (*p)[i+1:]...)
			return MsgSdPresetDeleted
		}
	}
	return MsgSdPresetNotFound
}

// ---------- 图片下载（img2img 底图） ----------

// FetchImageBase64 下载一张图并转成 base64，供 img2img 的 init_images[0]。
func (c *Client) FetchImageBase64(ctx context.Context, u string) (string, error) {
	b, err := c.FetchImageBytes(ctx, u)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SDPointsModelKey 返回计费用到的模型键（全局默认模型或用户指定模型）。
func SDPointsModelKey(model, global string) string {
	if strings.TrimSpace(model) != "" && model != "0" {
		return model
	}
	return global
}

// SdCooldownSeconds 解析"完成后冷却"（秒）。
func SdCooldownSeconds(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
