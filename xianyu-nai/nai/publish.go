package nai

import (
	"fmt"
	"net/url"
	"strings"
)

// 本文件复刻"出图后怎么发出去"的分支与文案。
//
// ---- 发送模式 ----
// ini 键 "发送模式"（fcn.1002d7f0 里四次比较），取值与分支：
//
//	"json"     0x102b8abc -> POST {资源地址}/plug/AI/nai3/aiurl.php
//	                          表单 name=...&data=...（0x102b8af4 / 0x102b8aec）
//	                          Content-Type: application/x-www-form-urlencoded（0x102b8afc）
//	"链接发送"  0x102b8fdc
//	"卡片跳转"  0x102b8ffc
//	"合并转发"  0x102b904c
//	其余        -> 直接发图片
//
// 配置键 "sections.home.sendMode"（0x1039b7c0）。
//
// ---- 资源地址下的路径 ----
//
//	/plug/AI/nai3/aiurl.php   0x102b9008   json 模式落盘
//	/plug/AI/nai3json.json    0x102b9020   卡片模板
//	/plug/AI/nai3?id=         0x102b9038   图库查看
//	/plug/AI/sdjson.json      0x102b8b2c
//	/plug/AI/sd/aiurl.php     0x102b8ad4
//	/plug/AI/sd?id=           0x102b8b60
//
// ---- 卡片/合并转发的 JSON 路径 ----
// 这些是模板占位，逐条来自 DLL：
//   0x102b9058 richtexts[0].info[1].card
//   0x102b9074 richtexts[0].info[1].receivetime
//   0x102b8868 richtexts[0].info[1].content
//   0x102b87f0 richtexts[0].info[1].button
//   0x102b8650 richtexts[0].filename
//   0x102b8668 richtexts[0].info[0].card
//   0x102b8684 richtexts[0].info[0].receivetime
//   0x102b86a8 richtexts[0].info[0].fromqq
//   0x102b86c4 richtexts[0].info[0].fromgroup
//   0x102b86e4 richtexts[0].info[0].content
//   0x102b8704 richtexts[0].info[1].fromqq
//   0x102b8720 richtexts[0].info[1].fromgroup
//   0x102b88c4 richtexts[0].info[0].content.type
//   0x102b88e8 richtexts[0].info[0].content.data.user_id
//   0x102b8914 richtexts[0].info[0].content.data.nickname
//   0x102b8940 markdown
//   0x102b894c richtexts[0].info[0].content.data.content[0].type
//   0x102b8980 richtexts[0].info[0].content.data.content[0].data.content
//   0x102b89bc richtexts[0].info[1].content.type
//   0x102b89e0 richtexts[0].info[1].content.data.user_id
//   0x102b8a0c richtexts[0].info[1].content.data.nickname
//   0x102b8a38 richtexts[0].info[1].content.data.content[0].type
//   0x102b8a6c richtexts[0].info[1].content.data.content[0].data.content
//   0x102b8f84 data.button
//   0x102b87d4 [0].[0].data
//
// 默认按钮串（0x102b8740，逐字保留）：
//   [[{"type":0,"name":"下载图片","data":"https://gchat.qpic.cn/gchatpic_new/0/0-0-95EDECA936FD9CE2A03ECCCE73A4EABB/0","reply":0,"enter":1,"blue":1}]]
//
// ---- 正文拼装（逐段来自 DLL）----
//   0x102b8ec8 "nai3"          0x102b8ee0 "\n### 模型:"
//   0x102b8ed0 "\n### 噪声调度:" 0x102b8eec "**\n### 采样方式:"
//   0x102b8f00 "\n```\n\n### seed:**"
//   0x102b8f18 ")\n\n```markdown\n"
//   0x102b8f2c "](mqqapi://aio/inlinecmd?command=咸鱼ai画图&reply=false&enter=false)"
//   0x102b880e "# 模型:"       0x102b881a "# 采样方式:"
//   0x102b882f "# seed:"       0x102b8847 "[🔗下载图片]("
//   0x102b8860 "![sd #"       0x102b8c38 "nai画图结果"
//   0x102b8c2a "模型:"         0x102b8c32 "seed:"
//   0x102b858b "![nai #"       0x102b8574 "px]("
//   0x102b857c "px #"

// SendMode 对应 ini "发送模式"。
type SendMode string

const (
	SendJSON    SendMode = "json"
	SendLink    SendMode = "链接发送"
	SendCard    SendMode = "卡片跳转"
	SendForward SendMode = "合并转发"
	SendImage   SendMode = "图片" // 默认分支
)

// 资源地址下的接口路径（逐字来自 DLL）。
const (
	PathNai3Aiurl    = "/plug/AI/nai3/aiurl.php" // 0x102b9008
	PathNai3CardJSON = "/plug/AI/nai3json.json"  // 0x102b9020
	PathNai3View     = "/plug/AI/nai3?id="       // 0x102b9038
	PathSdAiurl      = "/plug/AI/sd/aiurl.php"   // 0x102b8ad4
	PathSdCardJSON   = "/plug/AI/sdjson.json"    // 0x102b8b2c
	PathSdView       = "/plug/AI/sd?id="         // 0x102b8b60
	PathUploadImage  = "/plug/AI/upload-image"   // 0x102b8d22 附近 "@upload-image"
	// PathComfyCardJSON 是 ComfyUI 结果的卡片模板（0x102b9644）。
	// 说明：0x102b9644 的字面值是 /plug/AI/aijson.json；出图后 0x1007CC77 写的是
	// richtexts[0].info[1].content，标题 0x102bbcB4。绑定到 aijson.json 属
	// "同一资源目录下唯一的通用卡片模板" 推定（ComfyUI 自己的模板名未在常量表出现）。
	PathComfyCardJSON = "/plug/AI/aijson.json"
)

// UploadImageFileName 是直传文件的名字。
// 说明：DLL 里 multipart 由表单类拼装，文件名不是一条独立常量（推定用 "nai画图.png"）。
const UploadImageFileName = "nai画图.png"

// FixedMultipartBoundary 是原生实现写死的 multipart 界（0x103D0EBC "--WebKitFormBoundary"）。
const FixedMultipartBoundary = "WebKitFormBoundary"

// CfgUploadImage 是图片上传接口的配置键。
//
// 取证：上传函数 0x1002CDC8 起手就用配置对象（全局 0x104EBE78）的 +0x114
// 方法按名取值，键串 0x102B8D24 = "upload-image"；取到的串被直接当作
// HTTP 请求地址交给 0x10012D18，成功后再读响应里的 data.imageUrl
// （0x102B8D7C）作为图片直链。缺省值在本地 INI 里是 "0"（视为未配置），
// 此时用 {资源地址} + PathUploadImage 兜底。
const CfgUploadImage = "upload-image"

// 上游配置下发地址（唯一硬编码的外网地址之一）。
const (
	UpstreamJSON = "http://xianyu.xianyuw.cn/json.php" // 0x1039b1ac
	ResBaseHost  = "https://res.xianyuw.cn"
)

// 默认按钮串（0x102b8740），逐字保留。
const DefaultCardButton = `[[{"type":0,"name":"下载图片","data":"https://gchat.qpic.cn/gchatpic_new/0/0-0-95EDECA936FD9CE2A03ECCCE73A4EABB/0","reply":0,"enter":1,"blue":1}]]`

// CfgSendMode 是"发送模式"的配置键（0x1039b7c0）。
const CfgSendMode = "sections.home.sendMode"

// 违规相关文案。
const (
	MsgViolationFiltered = "图像内容违规，已过滤"         // 0x102b8fc4
	MsgAuditStart        = "开始审核图像"             // 0x102b8ddc
	MsgAuditForceFail    = "审核失败，为了安全，强制判定为违规图" // 0x102b8e38
)

// 图片处理动作文案（0x102b8f9c 起）。
const (
	MsgImgProcess  = "图片混淆"           // 0x102b8f90
	MsgDoProcess   = "处理"             // 0x102b8f9c
	MsgNoProcess   = "不处理"            // 0x102b8fa4
	MsgBlurProcess = "模糊处理"           // 0x102b8fac
	MsgFilterSend  = "过滤发送"           // 0x102b8fb8
	MsgDrawError   = "画图出错，请重试或联系管理员" // 0x102b8c44
)

// ResultMeta 是回帖正文用到的元信息。
type ResultMeta struct {
	Seed    int64
	Model   string
	Sampler string
	Noise   string
	Scale   float64
	Steps   int
	Width   int
	Height  int
	UserQQ  string
	BotQQ   string
	BotName string

	// 以下字段只给 sd 正文用（0x10026495-0x100264DD 的 16 段拼接）：
	//   - RawURL  是"下载图片"那一行的链接：SD 响应有 urls[0] 就用它，
	//     否则回落到上传换来的直链（0x100252A0 与 0x10025415 两条赋值路径）
	//   - Prompt  是 SD 响应 info.prompt，非空时用 ``` 代码块包一段
	RawURL string
	Prompt string
}

// PublishMode 判断命中哪个发送分支。
func PublishMode(v string) SendMode {
	switch SendMode(trimSpace(v)) {
	case SendJSON:
		return SendJSON
	case SendLink:
		return SendLink
	case SendCard:
		return SendCard
	case SendForward:
		return SendForward
	default:
		return SendImage
	}
}

// AiurlForm 复刻 json 模式的上传表单。
// 原实现用 WinHttp 的 "POST" + application/x-www-form-urlencoded，
// 拼法是字面 "name=" + name + "&data=" + data（0x102b8af4 / 0x102b8aec），
// 没有做 URL 编码，这里保持一致。
func AiurlForm(name, data string) (contentType, body string) {
	return "application/x-www-form-urlencoded", "name=" + name + "&data=" + data
}

// AiurlFormEscaped 是同一表单的转义版本，供服务端严格要求时使用。
func AiurlFormEscaped(name, data string) (contentType, body string) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("data", data)
	return "application/x-www-form-urlencoded", q.Encode()
}

// BuildNaiCaption 复刻 nai 结果的 markdown 正文。
//
// 段落顺序按 DLL 的拼接顺序：
//
//	"![nai #" + 图 + "px](" + ... "px #" ...
//	"\n\n---\n\n### " ... "\n### 模型:" ... "\n### 噪声调度:" ...
//	"**\n### 采样方式:" ... "\n```\n\n### seed:**" ... ")\n\n```markdown\n"
//	"](mqqapi://aio/inlinecmd?command=咸鱼ai画图&reply=false&enter=false)"
func BuildNaiCaption(m ResultMeta, imageURL string) string {
	var b strings.Builder
	if imageURL != "" {
		fmt.Fprintf(&b, "![nai #%dpx](%s)\n\n", m.Width, imageURL)
	}
	b.WriteString("\n\n---\n\n### 模型:\n")
	fmt.Fprintf(&b, "%s\n### 噪声调度:\n%s**\n### 采样方式:\n%s\n```\n\n### seed:**\n%d\n\n```markdown\n",
		m.Model, m.Noise, m.Sampler, m.Seed)
	b.WriteString("](mqqapi://aio/inlinecmd?command=咸鱼ai画图&reply=false&enter=false)")
	return b.String()
}

// BuildCaption 是 nai 结果的回帖正文（旧接口保留）。
// 等价于 BuildPlainCaption 的带尺寸版本。
func BuildCaption(m ResultMeta, imageURL string) string {
	var b strings.Builder
	b.WriteString("nai画图\r\n")
	fmt.Fprintf(&b, "\r\n模型：%s\r\n采样方式：%s\r\nseed：%d\r\n", m.Model, m.Sampler, m.Seed)
	fmt.Fprintf(&b, "尺寸：%d×%d\r\n步数：%d\r\n缩放：%.2f\r\n", m.Width, m.Height, m.Steps, m.Scale)
	if imageURL != "" {
		fmt.Fprintf(&b, "图片：%s\r\n", imageURL)
	}
	return b.String()
}

// BuildPlainCaption 复刻简版正文（0x102b880e / 0x102b8c2a 两套 "模型:"/"seed:"）。
func BuildPlainCaption(m ResultMeta, imageURL string) string {
	var b strings.Builder
	b.WriteString("nai画图结果\r\n")
	if imageURL != "" {
		fmt.Fprintf(&b, "[🔗下载图片](%s)\r\n", imageURL)
	}
	fmt.Fprintf(&b, "模型:%s\r\nseed:%d\r\n", m.Model, m.Seed)
	return b.String()
}

// BuildSdCaption 复刻 sd 结果正文。
//
// 证据：0x10026495-0x100264DD（知识卡正文）与 0x10026682-0x100266C5（无 info 分支）
// 是同一段 16 参数拼接（0x1000106f，最后压栈的在最前）。按压栈逆序还原后是：
//
//	"![sd #" + 宽 + "px #" + 高 + "px](" + 图链 + ")\r\n[🔗下载图片](" + urls[0] +
//	")\r\n```\r\n" + 提示词 + "\r\n```\r\n# seed:" + 种子 + "\r\n# 采样方式:" + 采样 +
//	"\r\n# 模型:" + 模型
//
// 两个链接都无条件拼接（0x100264B8 / 0x100266A5 没有判空分支），所以 urls[0] 为空时
// 会出现一对空括号——这是原插件的行为，这里照抄。
func BuildSdCaption(m ResultMeta, imageURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "![sd #%dpx #%dpx](%s)\r\n", m.Width, m.Height, imageURL)
	fmt.Fprintf(&b, "[🔗下载图片](%s)\r\n", m.RawURL)
	if m.Prompt != "" {
		fmt.Fprintf(&b, "```\r\n%s\r\n```\r\n", m.Prompt)
	}
	fmt.Fprintf(&b, "# seed:%d\r\n# 采样方式:%s\r\n# 模型:%s", m.Seed, m.Sampler, m.Model)
	return b.String()
}

// AvatarURL 复刻头像拼装（0x102b8888 附近的 q1.qlogo.cn 模板）。
func AvatarURL(qq string) string {
	return "https://q1.qlogo.cn/g?b=qq&nk=" + qq + "&s=640"
}

// GalleryViewURL 复刻"查看图库"的链接（0x102b9038 / 0x102b8b60）。
func GalleryViewURL(resBase, id string, isNai bool) string {
	path := PathSdView
	if isNai {
		path = PathNai3View
	}
	return strings.TrimSuffix(resBase, "/") + path + id
}

// GalleryViewID 从图库链接里抠出 id。
func GalleryViewID(u string) string {
	if i := strings.Index(u, "?id="); i >= 0 {
		return u[i+4:]
	}
	if i := strings.Index(u, "id="); i >= 0 {
		return u[i+3:]
	}
	return ""
}
