package nai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// 本文件是"QQ 框架适配层"：把易语言宿主导出表与宿主注入的能力，
// 映射到 OneBot v11 的事件与 API。
//
// ---- 易语言插件导出表（脱壳 DLL 的 Export 名）----
//
//	Init  Enable  Disable  _OnGroupNT  _OnPrivateNT  _OnEventNT
//
// 框架在收到消息时调用后三者；三者在 DLL 里都跳向同一分发
// （0x1039a484 起为宿主注入的函数表）。
//
// ---- 宿主注入的四个能力（VA 0x1039a484-0x1039a79c）----
//
//	发送群消息          -> SendGroupMsg
//	发送群json消息      -> SendGroupJSON（卡片）
//	消息合并转发至群    -> SendForward
//	上传图片            -> UploadImage（这里用 base64 段等价实现）
//
// 这四个函数在 DLL 里是外部导入，只有宿主（易语言 QQ 框架）才有实现，
// 这正是重制版必须自带适配层的原因。

// ---------- OneBot v11 事件 ----------

// Seg 是 OneBot v11 消息段。
type Seg struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// Sender 是上报事件里的发送者信息。
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Role     string `json:"role"`
}

// Event 是 OneBot v11 上报事件；只保留插件用得到的字段。
type Event struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	SubType     string `json:"sub_type"`
	MessageID   int64  `json:"message_id"`
	GroupID     int64  `json:"group_id"`
	UserID      int64  `json:"user_id"`
	SelfID      int64  `json:"self_id"`
	RawMessage  string `json:"raw_message"`
	Message     []Seg  `json:"message"`
	Sender      Sender `json:"sender"`
}

// Text 取出事件里的纯文本（丢弃图片等非文本段）。
func (e Event) Text() string {
	if len(e.Message) == 0 {
		return strings.TrimSpace(e.RawMessage)
	}
	var b strings.Builder
	for _, s := range e.Message {
		if s.Type != "text" {
			continue
		}
		if v, ok := s.Data["text"].(string); ok {
			b.WriteString(v)
		}
	}
	return strings.TrimSpace(b.String())
}

// Images 取出事件里的图片引用（优先 url，其次 file）。
func (e Event) Images() []string {
	var out []string
	for _, s := range e.Message {
		if s.Type != "image" {
			continue
		}
		if v, ok := s.Data["url"].(string); ok && v != "" {
			out = append(out, v)
			continue
		}
		if v, ok := s.Data["file"].(string); ok && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// AtList 取出被 @ 的 QQ 号（含 "all"）。
func (e Event) AtList() []string {
	var out []string
	for _, s := range e.Message {
		if s.Type != "at" {
			continue
		}
		if v, ok := s.Data["qq"].(string); ok {
			out = append(out, v)
		}
	}
	return out
}

// Nickname 取发送者显示名（群名片优先，其次昵称，最后 QQ 号）。
func (e Event) Nickname() string {
	if e.Sender.Card != "" {
		return e.Sender.Card
	}
	if e.Sender.Nickname != "" {
		return e.Sender.Nickname
	}
	return strconv.FormatInt(e.UserID, 10)
}

// Ats 判断是否 @ 了指定 QQ。
func (e Event) Ats(qq string) bool {
	for _, v := range e.AtList() {
		if v == qq || v == "all" {
			return true
		}
	}
	return false
}

// ---------- 宿主能力 ----------

// ForwardNode 是合并转发里的一条消息节点。
type ForwardNode struct {
	UserID  string
	Nick    string
	Text    string
	Image   []byte // 非空时作为图片段
	ImageOK bool
}

// Seg 把节点转成 OneBot 的 node 段。
func (n ForwardNode) Seg() Seg {
	var content []Seg
	if n.ImageOK && len(n.Image) > 0 {
		content = append(content, ImageSeg(n.Image))
	} else {
		content = append(content, TextSeg(n.Text))
	}
	return Seg{Type: "node", Data: map[string]interface{}{
		"uin":     n.UserID,
		"name":    n.Nick,
		"content": content,
	}}
}

// ImageSeg 用 base64:// 组装一个图片段（无需外网上传即可发送）。
func ImageSeg(data []byte) Seg {
	return Seg{Type: "image", Data: map[string]interface{}{
		"file": "base64://" + base64.StdEncoding.EncodeToString(data),
	}}
}

// TextSeg 组装一个文本段。
func TextSeg(s string) Seg {
	return Seg{Type: "text", Data: map[string]interface{}{"text": s}}
}

// JSONSeg 组装一个卡片段（对应宿主"发送群json消息"）。
func JSONSeg(card string) Seg {
	return Seg{Type: "json", Data: map[string]interface{}{"data": card}}
}

// Caller 是 OneBot v11 的 API 调用通道。
type Caller interface {
	Call(action string, params map[string]interface{}) (json.RawMessage, error)
}

// Host 复刻易语言宿主注入的四个能力。
type Host interface {
	SendGroupMsg(group, msg string) error
	SendPrivateMsg(user, msg string) error
	SendGroupSegs(group string, segs []Seg) error
	SendGroupJSON(group, card string) error
	SendForward(group string, nodes []ForwardNode) error
}

// OneBotHost 把 Host 实现在 OneBot v11 API 上。
type OneBotHost struct{ C Caller }

func idOrZero(s string) int64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// SendGroupMsg 实现 host "发送群消息"。
func (h OneBotHost) SendGroupMsg(group, msg string) error {
	_, err := h.C.Call("send_group_msg", map[string]interface{}{
		"group_id": idOrZero(group),
		"message":  []Seg{TextSeg(msg)},
	})
	return err
}

// SendPrivateMsg 实现私聊发送。
func (h OneBotHost) SendPrivateMsg(user, msg string) error {
	_, err := h.C.Call("send_private_msg", map[string]interface{}{
		"user_id": idOrZero(user),
		"message": []Seg{TextSeg(msg)},
	})
	return err
}

// SendGroupSegs 发复合消息段（文字+图片）。
func (h OneBotHost) SendGroupSegs(group string, segs []Seg) error {
	_, err := h.C.Call("send_group_msg", map[string]interface{}{
		"group_id": idOrZero(group),
		"message":  segs,
	})
	return err
}

// SendGroupJSON 实现 host "发送群json消息"。
func (h OneBotHost) SendGroupJSON(group, card string) error {
	return h.SendGroupSegs(group, []Seg{JSONSeg(card)})
}

// SendForward 实现 host "消息合并转发至群"。
func (h OneBotHost) SendForward(group string, nodes []ForwardNode) error {
	segs := make([]Seg, 0, len(nodes))
	for _, n := range nodes {
		segs = append(segs, n.Seg())
	}
	_, err := h.C.Call("send_group_forward_msg", map[string]interface{}{
		"group_id": idOrZero(group),
		"messages": segs,
	})
	return err
}

// HTTPCaller 通过 OneBot v11 的 HTTP API 调用（含 access_token）。
type HTTPCaller struct {
	Base   string // 例如 http://127.0.0.1:5700
	Token  string
	Client *http.Client
}

// Call 实现 Caller。
func (c *HTTPCaller) Call(action string, params map[string]interface{}) (json.RawMessage, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(c.Base, "/") + "/" + strings.TrimPrefix(action, "/")
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.Client
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// ---------- 插件（对应导出表） ----------

// Reply 是一条待发出的回复。
type Reply struct {
	Text  string
	Image []byte // 非空则按图片发送
	Card  string // 非空则按卡片发送
	IsNai bool
}

// Plugin 复刻插件实例：持配置、账本、黑名单、预设，并响应用户指令。
type Plugin struct {
	mu      sync.RWMutex
	inited  bool
	enabled bool

	cfg  Config
	host Host

	client *Client
	pipe   *PipelineConfig
	store  *Store

	cardTmpl []byte // 卡片模板（{资源地址}/plug/AI/nai3json.json）

	// sdCardTmpl 是 SD 通道的卡片模板（{资源地址}/plug/AI/sdjson.json，0x102b8b2c）
	sdCardTmpl []byte

	// ComfyUI 通道状态（见 comfy.go / comfy_cmd.go / comfy_draw.go）
	comfyCardTmpl   []byte          // 卡片模板
	comfyPresets    ComfyPresetFile // comfy预设列表.json
	comfyPresetPath string

	// cool 是三条画图通道共用的"完成后冷却"表（内存，进程内有效）
	cool *cooldownBook

	seed     int64
	negative string

	// SD 通道状态（见 sd.go / sd_cmd.go / sd_draw.go）
	sdState      *SDUserState  // seed.ini / Negative.ini 的每用户设置
	sdCache      *sdModelCache // /sdapi/v1/sd-models 缓存
	sdPresets    SDPresetFile  // sd预设列表.json
	sdPresetPath string

	// ini 是 咸鱼ai画图.ini 的只读快照，用于指令词覆盖等热配置。
	ini *IniFile
}

// SetCardTemplate 装载卡片模板（对应插件启动时从资源地址下载 nai3json.json）。
func (p *Plugin) SetCardTemplate(tmpl []byte) {
	p.mu.Lock()
	p.cardTmpl = tmpl
	p.mu.Unlock()
}

// SetComfyCardTemplate 装载 ComfyUI 卡片模板。
func (p *Plugin) SetComfyCardTemplate(tmpl []byte) {
	p.mu.Lock()
	p.comfyCardTmpl = tmpl
	p.mu.Unlock()
}

// SetSdCardTemplate 装载 SD 卡片模板（sdjson.json，0x102b8b2c）。
func (p *Plugin) SetSdCardTemplate(tmpl []byte) {
	p.mu.Lock()
	p.sdCardTmpl = tmpl
	p.mu.Unlock()
}

// NewPlugin 建实例（不启用）。
func NewPlugin(host Host, cfg Config) *Plugin {
	p := &Plugin{host: host}
	p.applyConfig(cfg)
	return p
}

// applyConfig 由 Init 与 NewPlugin 共用。
func (p *Plugin) applyConfig(cfg Config) {
	if cfg.Negative == "" {
		cfg.Negative = DefaultNegative
	}
	if cfg.Seed == 0 {
		cfg.Seed = -1
	}
	// SD 通道：先从 咸鱼ai画图.ini 的 [sd设置]/[sd参数] 取值覆盖调用方传入的值。
	if cfg.DataDir != "" {
		iniPath := joinDir(cfg.DataDir, FileMainINI)
		cfg = LoadNAIConfigFromINI(iniPath, cfg)
		cfg = LoadHomeConfigFromINI(iniPath, cfg)
		cfg.SD = LoadSDConfigFromINI(iniPath, cfg.SD)
		cfg.Comfy = LoadComfyConfigFromINI(iniPath, cfg.Comfy)
	}
	cfg.SD = cfg.SD.withDefaults()
	cfg.Comfy = cfg.Comfy.withDefaults()
	if cfg.DataDir != "" {
		p.ini = LoadIni(joinDir(cfg.DataDir, FileMainINI))
	} else {
		p.ini = LoadIni(FileMainINI)
	}
	p.cfg = cfg
	p.negative = cfg.Negative
	p.seed = cfg.Seed
	p.client = New(cfg)
	p.sdState = &SDUserState{Dir: cfg.DataDir}
	p.sdCache = &sdModelCache{}
	p.sdPresetPath = joinDir(cfg.DataDir, FileSdPresetJSON)
	p.sdPresets = LoadSDPresets(p.sdPresetPath)
	p.comfyPresetPath = joinDir(cfg.DataDir, FileComfyPresetJSON)
	p.comfyPresets = LoadComfyPresets(p.comfyPresetPath)
	p.cool = newCooldownBook()
	// 积分单价按通道写入：模型名 -> 单次消耗。
	// [点数消耗] 三个键分别对应三条通道（0x10097FE5/0x100980A4/0x10098163）。
	price := map[string]int64{}
	if cfg.CostNai > 0 {
		price[cfg.Model] = cfg.CostNai
	}
	if cfg.CostSD > 0 {
		price[cfg.SD.GlobalModel] = cfg.CostSD
	}
	if cfg.CostComfy > 0 {
		price[cfg.Comfy.GlobalModel] = cfg.CostComfy
	}
	auditAct := cfg.AuditAct
	if auditAct == "" {
		auditAct = string(ActionNoop)
	}
	auditTh := cfg.AuditTh
	if auditTh == "" {
		auditTh = DefaultAuditThreshold
	}
	auditPlat := PlatformAPI4AI
	if cfg.AuditPlat != "" {
		auditPlat = AuditPlatform(cfg.AuditPlat)
	}
	pt := PointsGlobal
	if cfg.PointsType == string(PointsPersonal) {
		pt = PointsPersonal
	}
	p.pipe = &PipelineConfig{
		Client: p.client,
		Gate: &PipelineGate{
			Blacklist:   Blacklist{},
			Points:      NewPointsBook(cfg.BasePoints, price),
			PointsOn:    cfg.PointsOn || cfg.BasePoints > 0,
			PointsType:  pt,
			AuditOn:     cfg.AuditOn,
			AuditPlat:   auditPlat,
			AuditThresh: auditTh,
			AuditAction: auditAct,
		},
	}
}

// rebuildLocked 在配置变更后重建 Client 与流水线（调用方须持写锁）。
func (p *Plugin) rebuildLocked() {
	p.client = New(p.cfg)
	gate := p.pipe.Gate
	p.pipe = &PipelineConfig{Client: p.client, Gate: gate}
	if p.store != nil {
		p.pipe.Gate.Presets = p.store.Presets
		p.pipe.Gate.Tags = p.store.Tags
	}
}

// Init 对应导出 Init：装载配置、账本、黑名单与预设。
func (p *Plugin) Init(cfg Config, store *Store) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.applyConfig(cfg)
	if store != nil {
		p.store = store
		p.pipe.Gate.Presets = store.Presets
		p.pipe.Gate.Tags = store.Tags
	}
	p.inited = true
}

// Inited 报告是否已初始化。
func (p *Plugin) Inited() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.inited
}

// Enable 对应导出 Enable。
func (p *Plugin) Enable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = true
}

// Disable 对应导出 Disable。
func (p *Plugin) Disable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = false
}

// Enabled 报告当前是否启用。
func (p *Plugin) Enabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled
}

// Config 返回当前生效配置的副本（配置面板读取用）。
func (p *Plugin) Config() Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

// iniSnapshot 返回当前 ini 快照（可能为 nil）。
func (p *Plugin) iniSnapshot() *IniFile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ini
}

// ReloadFromINI 重新从数据目录的 ini 装载 SD / ComfyUI 配置并重建客户端。
// 对应原插件“改完设置后点确定，立即生效”的行为；面板保存后调用。
func (p *Plugin) ReloadFromINI() {
	p.mu.Lock()
	defer p.mu.Unlock()
	cfg := p.cfg
	if cfg.DataDir != "" {
		iniPath := joinDir(cfg.DataDir, FileMainINI)
		cfg = LoadNAIConfigFromINI(iniPath, cfg)
		cfg = LoadHomeConfigFromINI(iniPath, cfg)
		cfg.SD = LoadSDConfigFromINI(iniPath, cfg.SD)
		cfg.Comfy = LoadComfyConfigFromINI(iniPath, cfg.Comfy)
	}
	cfg.SD = cfg.SD.withDefaults()
	cfg.Comfy = cfg.Comfy.withDefaults()
	if cfg.DataDir != "" {
		p.ini = LoadIni(joinDir(cfg.DataDir, FileMainINI))
	} else {
		p.ini = LoadIni(FileMainINI)
	}
	p.cfg = cfg
	p.client = New(cfg)
	gate := p.pipe.Gate
	gate.PointsOn = cfg.PointsOn || cfg.BasePoints > 0
	if cfg.PointsType == string(PointsPersonal) {
		gate.PointsType = PointsPersonal
	} else {
		gate.PointsType = PointsGlobal
	}
	gate.AuditOn = cfg.AuditOn
	if cfg.AuditPlat != "" {
		gate.AuditPlat = AuditPlatform(cfg.AuditPlat)
	}
	if cfg.AuditTh != "" {
		gate.AuditThresh = cfg.AuditTh
	}
	if cfg.AuditAct != "" {
		gate.AuditAction = cfg.AuditAct
	}
	p.pipe = &PipelineConfig{Client: p.client, Gate: gate}
	if p.store != nil {
		p.pipe.Gate.Presets = p.store.Presets
		p.pipe.Gate.Tags = p.store.Tags
	}
	p.sdPresets = LoadSDPresets(p.sdPresetPath)
	p.comfyPresets = LoadComfyPresets(p.comfyPresetPath)
}

// OnEventNT 对应导出 _OnEventNT：任意消息进来后统一分发。
func (p *Plugin) OnEventNT(ev Event) []Reply {
	if !p.Enabled() {
		return nil
	}
	switch ev.MessageType {
	case "group":
		return p.OnGroupNT(ev)
	case "private":
		return p.OnPrivateNT(ev)
	}
	return nil
}

// presetPrefixes 是不在 Commands 表里、单独管理的预设/标签指令词
// （0x1039aadc / 0x1039aae8 / 0x1039aaf4 / 0x1039ab00 / 0x1039ab0c）。
var presetPrefixes = []string{CmdUploadPreset, CmdUploadTag, CmdDeletePreset, CmdDeleteTag}

// matchesAnyCommand 判断文本是否以任一条指令开头。
// 群里只有这种文本会被响应（插件按"指令前缀"分派）。
func (p *Plugin) matchesAnyCommand(text string) bool {
	if _, _, ok := MatchCommandWithIni(text, p.iniSnapshot()); ok {
		return true
	}
	if _, _, ok := MatchSDCommand(text); ok {
		return true
	}
	if _, _, ok := MatchComfyCommand(text, ComfyConfig{}); ok {
		return true
	}
	_, _, kind := splitPresetCommand(text)
	return kind != ""
}

// OnGroupNT 对应导出 _OnGroupNT。
func (p *Plugin) OnGroupNT(ev Event) []Reply {
	text := ev.Text()
	if text == "" {
		return nil
	}
	qq := strconv.FormatInt(ev.UserID, 10)
	p.mu.RLock()
	black := p.pipe.Gate.Blacklist
	p.mu.RUnlock()
	if black.IsBlacklisted(ScopeNai, qq) {
		return nil
	}
	// 群里要求命中指令（或 @ 机器人），私聊不限。
	if !p.matchesAnyCommand(text) && !ev.Ats(strconv.FormatInt(ev.SelfID, 10)) {
		return nil
	}
	return p.handle(ev, text, "group")
}

// OnPrivateNT 对应导出 _OnPrivateNT。
func (p *Plugin) OnPrivateNT(ev Event) []Reply {
	text := ev.Text()
	if text == "" {
		return nil
	}
	qq := strconv.FormatInt(ev.UserID, 10)
	p.mu.RLock()
	black := p.pipe.Gate.Blacklist
	p.mu.RUnlock()
	if black.IsBlacklisted(ScopeNai, qq) {
		return nil
	}
	return p.handle(ev, text, "private")
}

// handle 是指令派发主体。
func (p *Plugin) handle(ev Event, text, scope string) []Reply {
	p.mu.RLock()
	cfg := p.cfg
	neg := p.negative
	store := p.store
	p.mu.RUnlock()
	_ = scope

	// 屏蔽词（0x102b7f1c）
	if w := HitBlockWord(text, ParseBlockWords(cfg.BlockWords)); w != "" {
		return []Reply{{Text: MsgBlocked}}
	}

	cmd, matched, ok := MatchCommandWithIni(text, p.iniSnapshot())
	if !ok {
		// SD 通道指令（sd画图 / sd种子 / sd查看模型 …，配置节 sd指令）
		if sc, sw, sok := MatchSDCommand(text); sok {
			return p.sdCommand(ev, sc, strings.TrimSpace(strings.TrimPrefix(text, sw)))
		}
		// ComfyUI 通道指令（comfy画图 / comfy查看模型 / comfy查看预设 …）
		if r, ok := p.comfyDispatch(ev, text); ok {
			return r
		}
		if r := p.presetCommand(text, store); r != nil {
			return []Reply{{Text: *r}}
		}
		return nil
	}
	arg := strings.TrimSpace(strings.TrimPrefix(text, matched))

	switch cmd.Kind {
	case CmdSeed:
		if v, err := strconv.ParseInt(strings.TrimSpace(arg), 10, 64); err == nil {
			p.mu.Lock()
			p.cfg.Seed = v
			p.rebuildLocked()
			p.mu.Unlock()
			return []Reply{{Text: "种子已修改为 " + strconv.FormatInt(v, 10)}}
		}
		return []Reply{{Text: "请输入数字种子"}}

	case CmdViewNegative:
		return []Reply{{Text: neg}}

	case CmdSetNegative:
		if arg != "" {
			p.mu.Lock()
			p.cfg.Negative = arg
			p.negative = arg
			p.rebuildLocked()
			p.mu.Unlock()
		}
		return []Reply{{Text: neg}}

	case CmdViewPreset:
		if store == nil {
			return nil
		}
		return []Reply{{Text: store.RenderPresetList()}}

	case CmdViewModel:
		var b strings.Builder
		for i, m := range Models {
			b.WriteString("#模型" + strconv.Itoa(i+1) + " " + m.Name + "\r\n")
		}
		return []Reply{{Text: b.String()}}

	case CmdQuota:
		sub, err := p.client.QuerySubscription(context.Background())
		if err != nil {
			return []Reply{{Text: err.Error()}}
		}
		return []Reply{{Text: sub.Render()}}

	case CmdDraw:
		return p.draw(ev, arg, neg)

	case CmdEmotion, CmdLineart, CmdSketch, CmdClean, CmdColorize:
		return p.augment(ev, cmd.Kind, arg)
	}
	return nil
}

// comfyDispatch 在 NAI 指令表不命中时尝试 comfy 指令（配置词见 [comfy指令]）。
func (p *Plugin) comfyDispatch(ev Event, text string) ([]Reply, bool) {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()
	c, w, ok := MatchComfyCommand(text, cfg.Comfy)
	if !ok {
		return nil, false
	}
	return p.comfyCommand(ev, c, strings.TrimSpace(strings.TrimPrefix(text, w))), true
}

// splitPresetCommand 识别预设/标签管理词并切出参数。
func splitPresetCommand(text string) (name, content, kind string) {
	for _, k := range presetPrefixes {
		if strings.HasPrefix(text, k) {
			n, c := splitNameContent(strings.TrimPrefix(text, k))
			return n, c, k
		}
	}
	return "", "", ""
}

// presetCommand 处理单独管理的预设/标签指令。
func (p *Plugin) presetCommand(text string, store *Store) *string {
	if store == nil {
		return nil
	}
	name, content, kind := splitPresetCommand(text)
	if kind == "" {
		return nil
	}
	var m string
	switch kind {
	case CmdUploadPreset:
		m = store.UploadPreset(name, content)
		_ = store.SavePresets(store.Path)
	case CmdUploadTag:
		m = store.UploadTag(name, content)
	case CmdDeletePreset:
		m = store.DeletePreset(name)
		_ = store.SavePresets(store.Path)
	case CmdDeleteTag:
		m = store.DeleteTag(name)
	}
	return &m
}

// draw 复刻画图链路：预设/标签 -> 门禁 -> 出图 -> 按发送模式回帖。
func (p *Plugin) draw(ev Event, arg string, neg string) []Reply {
	p.mu.RLock()
	pipe := p.pipe
	store := p.store
	p.mu.RUnlock()

	prompt := arg
	if store != nil {
		if m := MatchPresets(prompt, store.Presets, store.Tags); len(m) > 0 {
			prompt = ApplyPresets(prompt, m)
		}
	}
	imgURL := ""
	if imgs := ev.Images(); len(imgs) > 0 {
		imgURL = imgs[0]
	}

	res, err := pipe.RunPipeline(context.Background(), PipelineRequest{
		QQ:       strconv.FormatInt(ev.UserID, 10),
		BotQQ:    strconv.FormatInt(ev.SelfID, 10),
		GroupID:  strconv.FormatInt(ev.GroupID, 10),
		Prompt:   prompt,
		Negative: neg,
		ImageURL: imgURL,
	})
	if err != nil {
		return []Reply{{Text: MsgDrawError}}
	}
	if res.Rejected != nil {
		return []Reply{{Text: res.Rejected.Text}}
	}
	if len(res.Images) == 0 {
		return []Reply{{Text: MsgDrawError}}
	}
	meta := res.Meta
	meta.UserQQ = strconv.FormatInt(ev.UserID, 10)
	meta.BotQQ = strconv.FormatInt(ev.SelfID, 10)
	if meta.BotName == "" {
		meta.BotName = meta.BotQQ
	}

	p.mu.RLock()
	cardTmpl := p.cardTmpl
	p.mu.RUnlock()

	mode := PublishMode(p.cfg.SendMode)
	switch mode {
	case SendCard, SendForward:
		fields := CardFieldsFromMeta(meta, strconv.FormatInt(ev.GroupID, 10), prompt,
			BuildPlainCaption(meta, res.ImageURL), res.ImageURL)
		card, err := FillCard(cardTmpl, fields)
		if err == nil {
			return []Reply{{Card: card, Text: BuildPlainCaption(meta, res.ImageURL), IsNai: true}}
		}
	}

	out := make([]Reply, 0, len(res.Images))
	for i, b := range res.Images {
		text := ""
		if i == 0 {
			text = BuildNaiCaption(meta, res.ImageURL)
		}
		out = append(out, Reply{Image: b, Text: text, IsNai: true})
	}
	return out
}

// augment 复刻 /ai/augment-image 的五个动作。
func (p *Plugin) augment(ev Event, kind CommandKind, arg string) []Reply {
	imgs := ev.Images()
	if len(imgs) == 0 {
		return []Reply{{Text: ConfuseName(false)}}
	}
	t := AugmentEmotion
	switch kind {
	case CmdLineart:
		t = AugmentLineart
	case CmdSketch:
		t = AugmentSketch
	case CmdClean:
		t = AugmentDeclutter
	case CmdColorize:
		t = AugmentColorize
	}
	res, err := p.client.Augment(context.Background(), t, imgs[0], arg)
	if err != nil {
		return []Reply{{Text: MsgDrawError}}
	}
	var out []Reply
	for _, s := range res.Images {
		b, err := DecodeOfficialImage(s)
		if err != nil {
			continue
		}
		out = append(out, Reply{Image: b, Text: MsgDoProcess})
	}
	if len(out) == 0 {
		return []Reply{{Text: MsgDrawError}}
	}
	return out
}

// splitNameContent 把"名字 内容"切成两段（内容为空时用名字当名字）。
func splitNameContent(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.IndexAny(s, " \r\n\t"); i > 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// ---------- HTTP 上报入口 ----------

// ReportHandler 是 OneBot v11 的"HTTP 上报"入口：
// 收到事件 -> OnEventNT -> 按 Host 把回复发出去。
func (p *Plugin) ReportHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var ev Event
		if err := json.Unmarshal(body, &ev); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		replies := p.OnEventNT(ev)
		p.deliver(ev, replies)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","retcode":0}`))
	})
}

// deliver 按宿主能力把回复发出去。
func (p *Plugin) deliver(ev Event, replies []Reply) {
	p.mu.RLock()
	host := p.host
	p.mu.RUnlock()
	deliverVia(host, ev, replies)
}

// deliverVia 用指定宿主发回复（WebSocket 会话走自己的连接，不共用全局宿主）。
func deliverVia(host Host, ev Event, replies []Reply) {
	if host == nil {
		return
	}
	group := strconv.FormatInt(ev.GroupID, 10)
	user := strconv.FormatInt(ev.UserID, 10)
	for _, r := range replies {
		switch {
		case r.Card != "" && ev.MessageType == "group":
			_ = host.SendGroupJSON(group, r.Card)
		case len(r.Image) > 0 && ev.MessageType == "group":
			var segs []Seg
			if r.Text != "" {
				segs = append(segs, TextSeg(r.Text))
			}
			segs = append(segs, ImageSeg(r.Image))
			_ = host.SendGroupSegs(group, segs)
		case ev.MessageType == "group":
			if r.Text != "" {
				_ = host.SendGroupMsg(group, r.Text)
			}
		default:
			if r.Text != "" {
				_ = host.SendPrivateMsg(user, r.Text)
			}
		}
	}
}
