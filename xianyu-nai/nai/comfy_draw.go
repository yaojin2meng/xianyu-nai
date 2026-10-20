package nai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 本文件是 ComfyUI 的 HTTP 客户端 + "comfy画图" 编排。
//
// ---- 提交（0x10079402-0x10079645）----
//
//	POST {链接}/prompt（0x102BBAxx "/prompt"）
//	body: {"prompt":<工作流对象>,"client_id":"xianyuai"}（先 prompt 后 client_id，
//	      client_id 值 0x102BB9F0，键 0x102BB9FC）
//	头 0x10079609: "Authorization: Bearer " + Token + "\r\n" + Content-Type
//	  —— Token 为空时该段为空，只剩 Content-Type: application/json
//
// ---- 轮询（0x10079B11-0x10079FF4）----
//
//	GET {链接}/history/{prompt_id}，最多 0x5A=90 次，每次 0x7D0=2000ms
//	判完成：路径 "<id>.status" 存在（0x10079E7D）
//	取状态：路径 "<id>.status.status_str"（0x1007A0BE / 0x1007A263）
//	  != "success" -> "comfy绘图失败：" + 状态
//
// ---- 取图（0x1007A4A2-0x1007B092）----
//
//	"<id>.outputs" -> 每个节点 "<id>.outputs.<node>.images"
//	"<id>.outputs.<node>.images[i].filename" / ".subfolder" / ".type"
//	GET {链接}/view?filename=<f>&subfolder=<s>&type=<t>
//
// ---- 参考图上传（0x1007C084-0x1007C30C）----
//
//	POST {链接}/upload/image?overwrite=1
//	Content-Type: multipart/form-data; boundary=WebKitFormBoundary
//	头同 Bearer；表单字段 name（0x102BBC94）

// ComfyImageRef 是 history 里的一张贴图。
type ComfyImageRef struct {
	Filename  string
	Subfolder string
	Type      string
	Node      string
}

// ComfyDrawOptions 是一次 comfy画图 的输入。
type ComfyDrawOptions struct {
	QQ      string
	GroupID string
	// 已经过预设替换、参数剥离的提示词
	Prompt string
	// 聊天里 #参数名:值 的覆盖项
	Params map[string]string
	// #模型N（1 基）
	ModelIdx *int
	// #CFG / #识别率
	CFG *float64
	// #步骤
	Steps *int
	// 消息里带的参考图地址
	ImageURLs []string
}

// ComfyDrawResult 是 comfy画图 的产出。
type ComfyDrawResult struct {
	Images   [][]byte
	ImageURL string
	Caption  string
	Card     string
	Meta     ResultMeta
	Rejected string
	// 命中的预设提示（"comfy匹配预设：" + 名，0x102BB874）
	PresetHit string
	// 使用的工作流绑定名
	Workflow string
}

// ComfyHTTPError 把 DLL 的错误文案带出来。
type ComfyHTTPError struct{ Msg string }

func (e *ComfyHTTPError) Error() string { return e.Msg }

func comfyErr(msg string) error { return &ComfyHTTPError{Msg: msg} }

// comfyHeaders 复刻 0x10079609 / 0x1007C1C2 的头拼装。
func comfyHeaders(token, contentType string) map[string]string {
	h := map[string]string{"Content-Type": contentType}
	if strings.TrimSpace(token) != "" {
		h["Authorization"] = "Bearer " + token
	}
	return h
}

// comfyDo 发一个请求并返回状态码与正文。
func (c *Client) comfyDo(ctx context.Context, method, u string, headers map[string]string, body []byte) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

// BuildComfyPromptBody 复刻提交体：{"prompt":<工作流>,"client_id":"xianyuai"}。
func BuildComfyPromptBody(workflowJSON string) string {
	return `{"prompt":` + strings.TrimSpace(workflowJSON) + `,"client_id":"` + ComfyClientID + `"}`
}

// ComfySubmit 提交工作流，返回 prompt_id。
//
// 响应里取 "prompt_id"；取不到时按 0x102BBA34 的文案把响应带出去。
func (c *Client) ComfySubmit(ctx context.Context, base, token, body string) (string, error) {
	status, raw, err := c.comfyDo(ctx, http.MethodPost, base+PathComfyPrompt,
		comfyHeaders(token, "application/json"), []byte(body))
	if err != nil {
		return "", comfyErr(MsgComfySubmitHint)
	}
	if status/100 != 2 {
		return "", comfyErr(MsgComfySubmitFail + string(raw))
	}
	var root struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(raw, &root); err != nil || strings.TrimSpace(root.PromptID) == "" {
		return "", comfyErr(MsgComfySubmitFail + string(raw))
	}
	return root.PromptID, nil
}

// comfyGetJSON 取一个 JSON 对象。
func (c *Client) comfyGetJSON(ctx context.Context, u, token string) (map[string]interface{}, error) {
	status, raw, err := c.comfyDo(ctx, http.MethodGet, u, comfyHeaders(token, "application/json"), nil)
	if err != nil {
		return nil, err
	}
	if status/100 != 2 {
		return map[string]interface{}{}, nil
	}
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return map[string]interface{}{}, nil
	}
	return root, nil
}

// ComfyHistoryResult 是一次 /history 轮询的结果。
type ComfyHistoryResult struct {
	Done   bool
	Status string
	Images []ComfyImageRef
	Raw    string
	HasOut bool
}

// ComfyParseHistory 解析 /history 响应（0x10079E7D 起）。
func ComfyParseHistory(raw []byte, promptID string) ComfyHistoryResult {
	out := ComfyHistoryResult{Raw: string(raw)}
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return out
	}
	ent, ok := root[promptID].(map[string]interface{})
	if !ok {
		return out
	}
	statusObj, ok := ent["status"].(map[string]interface{})
	if !ok {
		return out // 还没有 status，继续等
	}
	out.Done = true
	if s, ok := statusObj["status_str"].(string); ok {
		out.Status = s
	}
	outputs, ok := ent["outputs"].(map[string]interface{})
	if !ok {
		return out
	}
	out.HasOut = true
	// 节点顺序按 /history 里的键序不可知，这里排序保证稳定（Node 名升序）。
	nodes := make([]string, 0, len(outputs))
	for k := range outputs {
		nodes = append(nodes, k)
	}
	sortStrings(nodes)
	for _, node := range nodes {
		obj, ok := outputs[node].(map[string]interface{})
		if !ok {
			continue
		}
		arr, ok := obj["images"].([]interface{})
		if !ok {
			continue
		}
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			ref := ComfyImageRef{Node: node}
			if s, ok := m["filename"].(string); ok {
				ref.Filename = s
			}
			if s, ok := m["subfolder"].(string); ok {
				ref.Subfolder = s
			}
			if s, ok := m["type"].(string); ok {
				ref.Type = s
			}
			if ref.Filename != "" {
				out.Images = append(out.Images, ref)
			}
		}
	}
	return out
}

// ComfyWait 轮询 /history 直到出现 status 或超时。
func (c *Client) ComfyWait(ctx context.Context, base, token, promptID string) (*ComfyHistoryResult, error) {
	var last []byte
	for i := 0; i < ComfyPollMax; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, comfyErr(MsgComfyTimeout)
			case <-time.After(ComfyPollInterval * time.Millisecond):
			}
		}
		status, raw, err := c.comfyDo(ctx, http.MethodGet, base+PathComfyHistory+promptID,
			comfyHeaders(token, "application/json"), nil)
		if err != nil {
			return nil, comfyErr(MsgComfyTimeout)
		}
		if status/100 != 2 {
			last = raw
			continue
		}
		last = raw
		res := ComfyParseHistory(raw, promptID)
		if res.Done {
			return &res, nil
		}
	}
	return nil, comfyErr(MsgComfyTimeout + "（最后一次响应：" + string(last) + "）")
}

// ComfyViewURL 拼 /view 的地址（0x1007B092）。
func ComfyViewURL(base, filename, subfolder, typ string) string {
	return base + PathComfyView + url.QueryEscape(filename) +
		"&subfolder=" + url.QueryEscape(subfolder) + "&type=" + url.QueryEscape(typ)
}

// ComfyFetchImage 取一张图。
func (c *Client) ComfyFetchImage(ctx context.Context, u, token string) ([]byte, error) {
	status, raw, err := c.comfyDo(ctx, http.MethodGet, u, comfyHeaders(token, "application/json"), nil)
	if err != nil {
		return nil, comfyErr(MsgComfyDrawFail + err.Error())
	}
	if status/100 != 2 || len(raw) == 0 {
		return nil, comfyErr(MsgComfyDrawFail + string(raw))
	}
	return raw, nil
}

// ComfyUploadImage 把一张参考图塞进 ComfyUI（0x1007C084 起）。
//
// 返回 ComfyUI 回填的文件名（响应里的 name 字段）。
func (c *Client) ComfyUploadImage(ctx context.Context, base, token, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.SetBoundary(FixedMultipartBoundary); err != nil {
		return "", comfyErr(MsgComfySubmitHint)
	}
	fw, err := mw.CreateFormFile("image", filename)
	if err != nil {
		return "", comfyErr(MsgComfySubmitHint)
	}
	if _, err := fw.Write(data); err != nil {
		return "", comfyErr(MsgComfySubmitHint)
	}
	_ = mw.WriteField(ComfyFieldName, filename)
	_ = mw.Close()

	status, raw, err := c.comfyDo(ctx, http.MethodPost, base+PathComfyUpload,
		comfyHeaders(token, "multipart/form-data; boundary="+FixedMultipartBoundary), buf.Bytes())
	if err != nil {
		return "", comfyErr(MsgComfySubmitHint)
	}
	if status/100 != 2 {
		return "", comfyErr(MsgComfySubmitFail + string(raw))
	}
	var root struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &root); err != nil || strings.TrimSpace(root.Name) == "" {
		return filename, nil
	}
	return root.Name, nil
}

// sortStrings 简单插入排序（避免为小列表引入 sort 依赖差异）。
func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// comfyRand 给默认种子用的随机后缀（0x100755xx 取 100..999）。
func comfyRand() int { return 100 + rand.Intn(900) }

var _ = comfyRand

// DrawComfy 执行一次 ComfyUI 出图。
func (p *Plugin) DrawComfy(ctx context.Context, o ComfyDrawOptions) (*ComfyDrawResult, error) {
	p.mu.RLock()
	cfg := p.cfg
	presets := p.comfyPresets
	dataDir := cfg.DataDir
	cardTmpl := p.comfyCardTmpl
	p.mu.RUnlock()

	cc := cfg.Comfy.withDefaults()
	if !cc.ComfyAvailable() {
		return nil, comfyErr(MsgComfyNotConfigured)
	}

	// 黑名单（blComfy）
	if p.pipe != nil && p.pipe.Gate != nil && p.pipe.Gate.Blacklist.IsBlacklisted(ScopeComfy, o.QQ) {
		return &ComfyDrawResult{Rejected: MsgComfyBlacklisted + o.QQ}, nil
	}

	// 完成后冷却（0x102B7D58；comfy参数/完成后冷却 + 完成冷却模式）
	if p.cool != nil {
		if left := p.cool.Check("comfy", CooldownKey(cc.CooldownMode, o.QQ, o.GroupID),
			ComfyCooldownSeconds(cc.Cooldown)); left > 0 {
			return &ComfyDrawResult{Rejected: RenderCooldown(left)}, nil
		}
	}

	ws := LoadComfyWorkflows(dataDir)
	if len(ws) == 0 {
		return &ComfyDrawResult{Rejected: MsgComfyNoWorkflow}, nil
	}

	// ---- 选绑定 ----
	name := ""
	if o.ModelIdx != nil {
		i := *o.ModelIdx - 1
		if i < 0 || i >= len(ws) {
			return &ComfyDrawResult{Rejected: RenderComfyModelOutOfRange(len(ws))}, nil
		}
		name = ws[i].Name
		_ = SetComfySelectedWorkflow(dataDir, o.QQ, name)
	} else {
		name = ComfySelectedWorkflow(dataDir, o.QQ)
		if name == "" {
			name = strings.TrimSpace(cc.GlobalModel)
		}
		if name == "" {
			name = ws[0].Name
		}
	}
	wf, ok := FindComfyWorkflow(ws, name)
	if !ok {
		return &ComfyDrawResult{Rejected: MsgComfyWfNotExist}, nil
	}
	if strings.TrimSpace(wf.Content) == "" {
		return &ComfyDrawResult{Rejected: MsgComfyWfEmpty}, nil
	}

	prompt := strings.TrimSpace(o.Prompt)
	if prompt == "" {
		return &ComfyDrawResult{Rejected: MsgComfyNeedPrompt}, nil
	}

	out := &ComfyDrawResult{Workflow: wf.Name}

	// ---- 预设替换 ----
	if len(presets) > 0 {
		if np, hit := presets.MatchComfyPreset(prompt); np != prompt {
			prompt = np
			out.PresetHit = MsgComfyPresetHit + hit // 0x102BB874
		}
	}

	// ---- 占位符取值 ----
	seed := ComfyEffectiveSeed(cc.Seed, time.Now().Unix(), func(n int) int {
		if n <= 0 {
			return 0
		}
		return rand.Intn(n)
	})
	vals := ComfyDefaultValues(cc, prompt)
	vals[PhComfySeed] = seed
	if o.Steps != nil {
		vals[PhComfySteps] = fmt.Sprint(*o.Steps)
	}
	if o.CFG != nil {
		vals[PhComfyCFG] = sdNum(*o.CFG)
	}
	for k, v := range o.Params {
		if k == "识别率" { // #识别率:x 等价 CFG
			k = PhComfyCFG
		}
		vals[k] = v
	}

	content, missing := SubstituteComfyPlaceholders(wf.Content, vals)
	if len(missing) > 0 {
		return &ComfyDrawResult{Rejected: RenderComfyMissingParams(missing)}, nil
	}
	if _, err := decodeOJSON([]byte(content)); err != nil {
		return &ComfyDrawResult{Rejected: MsgComfyJSONFail}, nil
	}

	c := New(cfg)
	base := cc.ComfyBaseURL()

	// ---- 参考图上传 ----
	if len(o.ImageURLs) > 0 {
		var uploaded []string
		fail := 0
		for i, u := range o.ImageURLs {
			b, err := c.FetchImageBytes(ctx, u)
			if err != nil {
				fail++
				continue
			}
			n, err := c.ComfyUploadImage(ctx, base, cc.Token,
				fmt.Sprintf("comfy_ref_%d.png", i+1), b)
			if err != nil {
				fail++
				continue
			}
			uploaded = append(uploaded, n)
		}
		if fail > 0 {
			return &ComfyDrawResult{Rejected: fmt.Sprint(fail) + MsgComfyRefUploadFail}, nil
		}
		for i, n := range uploaded {
			vals[fmt.Sprintf("参考图%d", i+1)] = n
			vals["参考图"] = n
		}
	}

	// ---- 提交 ----
	body := BuildComfyPromptBody(content)
	promptID, err := c.ComfySubmit(ctx, base, cc.Token, body)
	if err != nil {
		return nil, err
	}

	// ---- 轮询 ----
	hist, err := c.ComfyWait(ctx, base, cc.Token, promptID)
	if err != nil {
		return nil, err
	}
	if hist.Status != "" && hist.Status != "success" {
		return nil, comfyErr(MsgComfyDrawFail + hist.Status)
	}
	if len(hist.Images) == 0 {
		return nil, comfyErr(MsgComfyNoImage)
	}

	// ---- 取图 ----
	var refs [][]byte
	var firstURL string
	for _, ref := range hist.Images {
		u := ComfyViewURL(base, ref.Filename, ref.Subfolder, ref.Type)
		b, err := c.ComfyFetchImage(ctx, u, cc.Token)
		if err != nil {
			continue
		}
		refs = append(refs, b)
		if firstURL == "" {
			firstURL = u
		}
	}
	if len(refs) == 0 {
		return nil, comfyErr(MsgComfyNoImage)
	}

	// ---- 审核 ----
	if g := p.pipe; g != nil && g.Gate != nil && g.Gate.AuditOn {
		kept := refs[:0]
		for _, b := range refs {
			u := ""
			if out.ImageURL == "" {
				if up, err := c.EnsureImageURL(ctx, b, ""); err == nil {
					u = up
				}
			} else {
				u = out.ImageURL
			}
			r := c.CheckNSFW(ctx, g.Gate.AuditPlat, "", u, g.Gate.AuditThresh)
			if r == nil || r.Verdict != "违规" {
				kept = append(kept, b)
			}
		}
		refs = kept
		if len(refs) == 0 {
			return &ComfyDrawResult{Rejected: MsgComfyAuditRejected}, nil
		}
	}

	out.Images = refs
	out.ImageURL = firstURL

	// 没有可直连的图床就上传换直链（与 nai/sd 同一上传函数）
	if u, err := c.EnsureImageURL(ctx, refs[0], ""); err == nil && u != "" {
		out.ImageURL = u
	}

	sampler := vals[PhComfySampler]
	sched := vals[PhComfyScheduler]
	out.Meta = ResultMeta{
		Seed:    comfySeedInt(vals[PhComfySeed]),
		Model:   wf.Name,
		Sampler: sampler + "/" + sched,
		Steps:   comfyAtoi(vals[PhComfySteps], cc.Steps),
		Scale:   comfyAtof(vals[PhComfyCFG], cc.Cfg),
		Width:   comfyAtoi(vals[PhComfyWidth], cc.Width),
		Height:  comfyAtoi(vals[PhComfyHeight], cc.Height),
		UserQQ:  o.QQ,
		RawURL:  firstURL,
		Prompt:  vals[PhComfyPrompt],
	}
	out.Caption = BuildComfyCaption(out.Meta, out.ImageURL)

	// 卡片/合并转发用 comfy 自己的模板
	if mode := PublishMode(cfg.SendMode); mode == SendCard || mode == SendForward {
		fields := CardFieldsFromMeta(out.Meta, o.GroupID, vals[PhComfyPrompt],
			out.Caption, out.ImageURL)
		fields.Title = CardTitleComfy // 0x102BBCB4
		if card, err := FillCard(cardTmpl, fields); err == nil {
			out.Card = card
		}
	}
	// 出图成功才记冷却起点
	if p.cool != nil {
		p.cool.Touch("comfy", CooldownKey(cc.CooldownMode, o.QQ, o.GroupID))
	}
	return out, nil
}
