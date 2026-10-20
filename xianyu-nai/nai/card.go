package nai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 本文件复刻"卡片跳转 / 合并转发"两种发送模式下对卡片模板的填充。
//
// ---- 模板来源 ----
// 插件从 {资源地址} 下载 /plug/AI/nai3json.json（0x102b9020）作为模板，
// 再按固定字段表往里写值。模板本体不在 DLL 里，只在资源服务器上。
//
// ---- 赋值点（两处同构，sd 通道与 nai 通道）----
//
//	0x100267F2-0x10027140   sd 通道（文件名用 "sd画图结果" 0x102b88a4）
//	0x10030E18-0x10030F80   nai 通道
//	0x1005FE4A-0x10060000   nai 通道的按钮段
//
// ---- 字段表（逐条来自上面的赋值点；路径串 VA 见括号）----
//
//	richtexts[0].filename                                        (0x102b8650) = "MultiMsg" (0x102b8644)
//	richtexts[0].info[0].card                                    (0x102b8668) = 用户名片
//	richtexts[0].info[0].receivetime                             (0x102b8684) = 当前时间戳（call 0x1000b65c(1,1)）
//	richtexts[0].info[0].fromqq                                  (0x102b86a8) = 用户QQ
//	richtexts[0].info[0].fromgroup                               (0x102b86c4) = 群号
//	richtexts[0].info[0].content                                 (0x102b86e4) = 子对象
//	richtexts[0].info[0].content.type                            (0x102b88c4) = "node" (0x102b88bc)
//	richtexts[0].info[0].content.data.user_id                    (0x102b88e8) = 用户QQ
//	richtexts[0].info[0].content.data.nickname                   (0x102b8914) = 用户昵称
//	richtexts[0].info[0].content.data.content[0].type            (0x102b894c) = "markdown" (0x102b8940)
//	richtexts[0].info[0].content.data.content[0].data.content    (0x102b8980) = 用户输入（提示词）
//	richtexts[0].info[1].card                                    (0x102b9058) = 机器人名片
//	richtexts[0].info[1].receivetime                             (0x102b9074) = 当前时间戳
//	richtexts[0].info[1].fromqq                                  (0x102b8704) = 机器人QQ
//	richtexts[0].info[1].fromgroup                               (0x102b8720) = 群号
//	richtexts[0].info[1].content                                 (0x102b8868) = 子对象
//	richtexts[0].info[1].content.type                            (0x102b89bc) = "node"
//	richtexts[0].info[1].content.data.user_id                    (0x102b89e0) = 机器人QQ
//	richtexts[0].info[1].content.data.nickname                   (0x102b8a0c) = 机器人昵称
//	richtexts[0].info[1].content.data.content[0].type            (0x102b8a38) = "markdown"
//	richtexts[0].info[1].content.data.content[0].data.content     (0x102b8a6c) = 拼装正文
//	richtexts[0].info[1].button                                  (0x102b87f0) = ",button=" + base64(UTF-8(按钮串))
//
// ---- 按钮 ----
// 默认按钮串（0x102b8740，逐字保留，见 DefaultCardButton）先被当成 JSON 解析，
// 再把 [0].[0].data（0x102b87d4）改成图片地址，序列化后送到
// richtexts[0].info[1].button，前缀是字面量 ",button="（0x102b87e4）。
// 中间两次字符串加工已实测确认（work/harness 直调）：
//
//	0x10039a3c = 文本 -> UTF-8 字节集（输入 GBK "下载图片"，输出 12 字节 e4 b8 8b ...）
//	0x10033e34 = Base64 编码（"abc"->"YWJj"，"a"->"YQ=="，"下载图片"->"5LiL6L295Zu+54mH"）
//
// 也就是：",button=" + base64(UTF-8(按钮串))。
// 注：data.button（0x102b8f84）在 nai 通道里并未被写，它只出现在别的卡片类型
// （0x1002f282 / 0x1007fa37 等，取值自子程序 0x10038ed8）。
//
// ---- 时间戳 ----
// 两处 receivetime 都来自 call 0x1000b65c(1,1)，实测返回秒级 Unix 时间戳文本
// （如 "1792400107"；(0,0) 则返回毫秒 13 位）。

// 卡片里的固定字面量（逐字来自 DLL）。
const (
	CardFileName      = "MultiMsg" // 0x102b8644 -> richtexts[0].filename
	CardContentType   = "node"     // 0x102b88bc -> content.type
	CardMarkdownType  = "markdown" // 0x102b8940 -> content.data.content[0].type
	CardButtonPrefix  = ",button=" // 0x102b87e4
	CardTitleNai      = "nai画图结果"  // 0x102b8c38
	CardTitleSd       = "sd画图结果"   // 0x102b88a4
	CardButtonDataKey = "[0].[0].data"
)

// CardFields 是一次卡片填充的全部输入。
type CardFields struct {
	UserQQ   string
	UserName string
	BotQQ    string
	BotName  string
	GroupID  string

	Prompt string // info[0] 的 markdown 正文（用户输入）
	Body   string // info[1] 的 markdown 正文（出图参数）

	ButtonJSON string // 按钮串；留空则由 ButtonJSON(buttonImage) 生成
	ButtonData string // [0].[0].data，通常是图片地址

	Receivetime string // 留空则用当前秒级时间戳
	Title       string // 0x102b8c38 / 0x102b88a4，仅作记录用
}

// pathSeg 是模板路径的一段：要么是对象键，要么是数组下标。
type pathSeg struct {
	key   string
	index int
	isIdx bool
}

// parsePath 解析 "a.b[0].c" / "[0].[0].data" 这类模板路径。
func parsePath(p string) []pathSeg {
	var out []pathSeg
	for _, part := range strings.Split(p, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// 前导键名（可缺省，如 "[0]"）
		name := part
		if i := strings.IndexByte(part, '['); i >= 0 {
			name = part[:i]
		}
		if name != "" {
			out = append(out, pathSeg{key: name})
		}
		// 其后的连续下标 [n][m]...
		rest := part[len(name):]
		for len(rest) > 0 && rest[0] == '[' {
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				break
			}
			n, err := strconv.Atoi(rest[1:end])
			if err != nil {
				break
			}
			out = append(out, pathSeg{index: n, isIdx: true})
			rest = rest[end+1:]
		}
	}
	return out
}

// setPath 按路径写入值，缺失的中间节点自动创建，返回（可能被替换的）根。
func setPath(root interface{}, segs []pathSeg, val interface{}) interface{} {
	if len(segs) == 0 {
		return val
	}
	s := segs[0]
	if s.isIdx {
		arr, ok := root.([]interface{})
		if !ok {
			arr = []interface{}{}
		}
		for len(arr) <= s.index {
			arr = append(arr, nil)
		}
		arr[s.index] = setPath(arr[s.index], segs[1:], val)
		return arr
	}
	obj, ok := root.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	obj[s.key] = setPath(obj[s.key], segs[1:], val)
	return obj
}

// getPath 按路径取值，供校验/测试使用；路径不存在时 ok=false。
func getPath(root interface{}, p string) (interface{}, bool) {
	cur := root
	for _, s := range parsePath(p) {
		if s.isIdx {
			arr, ok := cur.([]interface{})
			if !ok || s.index < 0 || s.index >= len(arr) {
				return nil, false
			}
			cur = arr[s.index]
			continue
		}
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		v, ok := obj[s.key]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// CardFieldPath 是卡片字段表的完整路径清单（顺序与 DLL 赋值顺序一致）。
// 供自检与测试逐条比对。
var CardFieldPath = []string{
	"richtexts[0].filename",
	"richtexts[0].info[0].card",
	"richtexts[0].info[0].receivetime",
	"richtexts[0].info[0].fromqq",
	"richtexts[0].info[0].fromgroup",
	"richtexts[0].info[0].content.type",
	"richtexts[0].info[0].content.data.user_id",
	"richtexts[0].info[0].content.data.nickname",
	"richtexts[0].info[0].content.data.content[0].type",
	"richtexts[0].info[0].content.data.content[0].data.content",
	"richtexts[0].info[1].card",
	"richtexts[0].info[1].receivetime",
	"richtexts[0].info[1].fromqq",
	"richtexts[0].info[1].fromgroup",
	"richtexts[0].info[1].content.type",
	"richtexts[0].info[1].content.data.user_id",
	"richtexts[0].info[1].content.data.nickname",
	"richtexts[0].info[1].content.data.content[0].type",
	"richtexts[0].info[1].content.data.content[0].data.content",
	"richtexts[0].info[1].button",
}

// ButtonJSON 复刻按钮串处理：把默认按钮里的 [0].[0].data 换成图片地址。
//
// 默认按钮串见 DefaultCardButton（0x102b8740，146 字节单串）。
// 键序按原串保留（E 语言 JSON 类按路径换值后取文本不改键序），之后由
// 0x10033e34 做 base64（见 CardButtonField）。
func ButtonJSON(imageURL string) (string, error) {
	root, err := decodeOJSON([]byte(DefaultCardButton))
	if err != nil {
		return "", err
	}
	root = root.set(parsePath(CardButtonDataKey), imageURL)
	return root.Text(), nil
}

// CardButtonField 复刻写进 richtexts[0].info[1].button 的值。
//
// 由实测（work/harness 直调 DLL 子程序）确认：
//
//	call 0x10039a3c  = 文本 -> UTF-8 字节集
//	call 0x10033e34  = Base64 编码
//
// 故该字段 = ",button=" + base64(UTF-8(按钮串))。
func CardButtonField(imageURL string) (string, error) {
	b, err := ButtonJSON(imageURL)
	if err != nil {
		return "", err
	}
	return CardButtonPrefix + base64.StdEncoding.EncodeToString([]byte(b)), nil
}

// FillCard 把 CardFields 写进卡片模板，返回填充后的 JSON 文本。
//
// 模板为空时按空对象处理（等价于只写字段表里的路径）。
// 键序按模板原样保留（E 语言 JSON 类按路径换值后取文本不改键序）。
func FillCard(tmpl []byte, f CardFields) (string, error) {
	var root *ojson
	if t := bytes.TrimSpace(tmpl); len(t) > 0 {
		var err error
		root, err = decodeOJSON(t)
		if err != nil {
			return "", errCardTemplate(err)
		}
	}
	if root == nil {
		root = &ojson{kind: 'o'}
	}

	btn := f.ButtonJSON
	if btn == "" {
		var err error
		btn, err = CardButtonField(f.ButtonData)
		if err != nil {
			return "", err
		}
	}
	rt := f.Receivetime
	if rt == "" {
		rt = strconv.FormatInt(NowUnix(), 10)
	}

	set := func(path, v string) {
		root = root.set(parsePath(path), v)
	}

	set("richtexts[0].filename", CardFileName)
	// info[0]：用户节点
	set("richtexts[0].info[0].card", f.UserName)
	set("richtexts[0].info[0].receivetime", rt)
	set("richtexts[0].info[0].fromqq", f.UserQQ)
	set("richtexts[0].info[0].fromgroup", f.GroupID)
	set("richtexts[0].info[0].content.type", CardContentType)
	set("richtexts[0].info[0].content.data.user_id", f.UserQQ)
	set("richtexts[0].info[0].content.data.nickname", f.UserName)
	set("richtexts[0].info[0].content.data.content[0].type", CardMarkdownType)
	set("richtexts[0].info[0].content.data.content[0].data.content", f.Prompt)
	// info[1]：机器人节点
	set("richtexts[0].info[1].card", f.BotName)
	set("richtexts[0].info[1].receivetime", rt)
	set("richtexts[0].info[1].fromqq", f.BotQQ)
	set("richtexts[0].info[1].fromgroup", f.GroupID)
	set("richtexts[0].info[1].content.type", CardContentType)
	set("richtexts[0].info[1].content.data.user_id", f.BotQQ)
	set("richtexts[0].info[1].content.data.nickname", f.BotName)
	set("richtexts[0].info[1].content.data.content[0].type", CardMarkdownType)
	set("richtexts[0].info[1].content.data.content[0].data.content", f.Body)
	set("richtexts[0].info[1].button", btn)

	return root.Text(), nil
}

// MarshalCompact 序列化 JSON 且不转义 < > &（E 语言置文本不转义）。
func MarshalCompact(v interface{}) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// CardTitle 返回该通道的结果标题（0x102b8c38 / 0x102b88a4）。
func CardTitle(isNai bool) string {
	if isNai {
		return CardTitleNai
	}
	return CardTitleSd
}

// CardFieldsFromMeta 由一次出图的元信息生成卡片字段。
// prompt 是写进 info[0]（用户节点）的正文，body 是 info[1]（机器人节点）的正文。
func CardFieldsFromMeta(m ResultMeta, groupID, prompt, body, imageURL string) CardFields {
	return CardFields{
		UserQQ:     m.UserQQ,
		UserName:   m.UserQQ,
		BotQQ:      m.BotQQ,
		BotName:    m.BotName,
		GroupID:    groupID,
		Prompt:     prompt,
		Body:       body,
		ButtonData: imageURL,
		Title:      CardTitle(true),
	}
}

// CardTemplateURL 拼出卡片模板地址（与其它资源路径同源）。
func CardTemplateURL(resBase string) string {
	return strings.TrimSuffix(resBase, "/") + PathNai3CardJSON
}

// FetchCardTemplate 从资源地址下载卡片模板。
// 插件在启动段做同样的事（0x102b9020），下载失败时卡片模式不可用。
func FetchCardTemplate(ctx context.Context, resBase string, hc *http.Client) ([]byte, error) {
	return fetchCardTemplate(ctx, resBase, PathNai3CardJSON, hc)
}

// FetchSdCardTemplate 下载 SD 通道的卡片模板（sdjson.json，0x102b8b2c）。
func FetchSdCardTemplate(ctx context.Context, resBase string, hc *http.Client) ([]byte, error) {
	return fetchCardTemplate(ctx, resBase, PathSdCardJSON, hc)
}

// FetchComfyCardTemplate 装载 ComfyUI 通道的卡片模板（aijson.json，0x102b9644）。
func FetchComfyCardTemplate(ctx context.Context, resBase string, hc *http.Client) ([]byte, error) {
	return fetchCardTemplate(ctx, resBase, PathComfyCardJSON, hc)
}

func fetchCardTemplate(ctx context.Context, resBase, path string, hc *http.Client) ([]byte, error) {
	if strings.TrimSpace(resBase) == "" {
		return nil, errors.New("资源地址未配置，无法下载卡片模板")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(resBase, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("卡片模板下载失败：HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// NowUnix 是两处 receivetime 用的时间源（call 0x1000b65c(1,1) 返回秒级时间戳文本）。
func NowUnix() int64 { return time.Now().Unix() }

// errCardTemplate 是模板解析失败的统一错误包装。
func errCardTemplate(err error) error {
	return fmt.Errorf("卡片模板解析失败: %w", err)
}
