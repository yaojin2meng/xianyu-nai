package nai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"strings"
)

// 本文件是"整条绘图流水线"的编排，把前面各文件串起来。
//
// 顺序（依据 DLL 里 fcn.1002d7f0 与 fcn.1005b4e8 的控制流）：
//
//	1. 屏蔽词检查        -> gate.CheckPrompt      (0x102b7f1c)
//	2. 黑名单检查        -> gate.Blacklist        (0x1039bb40 blNai)
//	3. 积分扣费          -> gate.PointsBook       (0x102b7fc)
//	4. 预设/标签替换     -> command.MatchPresets  (0x102b7f00/0x102b7f0c)
//	5. # 参数解析        -> command.ParseOverrides(0x102b7d9c)
//	6. lora 抽取 + 翻译  -> prompt.PreparePrompt  (0x102b8078/0x102b8134)
//	7. 发请求            -> client.Generate
//	8. 官方 base64+zip 解包 -> decode.DecodeOfficialImage
//	9. 出图审核          -> audit.CheckNSFW       (0x102b8ddc/0x102b8e38)
//	10. 图片处理(混淆/模糊/过滤) -> 0x102b8f90
//	11. 按发送模式投递    -> publish

// PipelineConfig 是一条流水线需要的全部输入。
type PipelineConfig struct {
	Client *Client
	Gate   *PipelineGate
}

// PipelineGate 汇总门禁与数据。
type PipelineGate struct {
	Blacklist   Blacklist
	Points      *PointsBook
	Presets     PresetFile
	Tags        PresetFile
	PointsOn    bool
	PointsType  PointsType
	AuditOn     bool
	AuditPlat   AuditPlatform
	AuditThresh string
	AuditAction string
}

// PipelineRequest 是一次用户请求。
type PipelineRequest struct {
	QQ       string
	BotQQ    string
	GroupID  string
	Prompt   string
	Negative string
	ImageURL string // img2img/导演工具的原图
}

// PipelineResult 是流水线产出。
type PipelineResult struct {
	Seed     int64
	Images   [][]byte // 已解包成原始图片字节
	ImageURL string
	Caption  string
	Rejected *Reject
	Audit    *AuditResult
	Loras    []Lora
	Meta     ResultMeta
}

// RunPipeline 执行一次完整绘图。
//
// 返回的 error 只表示技术失败；门禁拦截走 PipelineResult.Rejected。
func (p *PipelineConfig) RunPipeline(ctx context.Context, req PipelineRequest) (*PipelineResult, error) {
	c := p.Client
	g := p.Gate
	out := &PipelineResult{}

	// 1. 屏蔽词
	if r := c.CheckPrompt(req.Prompt); r != nil {
		out.Rejected = r
		return out, nil
	}
	// 2. 黑名单
	if g != nil && g.Blacklist.IsBlacklisted(ScopeNai, req.QQ) {
		out.Rejected = &Reject{Kind: "blacklist", Text: MsgBlacklisted}
		return out, nil
	}
	// 3. 积分
	if g != nil && g.PointsOn && g.Points != nil {
		if _, err := g.Points.Charge(req.QQ, c.cfg.Model); err != nil {
			if r, ok := err.(*Reject); ok {
				out.Rejected = r
				return out, nil
			}
			return nil, err
		}
	}
	// 4. 预设/标签
	prompt := req.Prompt
	if g != nil {
		if ms := MatchPresets(prompt, g.Presets, g.Tags); len(ms) > 0 {
			prompt = ApplyPresets(prompt, ms)
		}
	}
	// 5. # 参数
	ov, prompt := ParseOverrides(prompt)
	cfg := c.cfg
	ov.Apply(&cfg)
	c2 := New(cfg)

	// 6. lora + 翻译
	prompt, negative, loras := c2.PreparePrompt(ctx, prompt, req.Negative)
	out.Loras = loras

	// 7. 请求
	res, err := c2.Generate(ctx, prompt, negative, req.ImageURL)
	if err != nil {
		return nil, err
	}
	out.Seed = res.Seed
	out.ImageURL = res.ImageURL

	// 8. 解包
	for _, s := range res.Images {
		b, err := DecodeOfficialImage(s)
		if err != nil {
			return nil, err
		}
		out.Images = append(out.Images, b)
	}
	// 8.1 反代只给 image_url、不给 base64 时（如 nai.xianyuww.cn / nai.xianyuw.cn），
	// 需要把直链下载回来，否则下游按"没出图"处理。
	if len(out.Images) == 0 && strings.TrimSpace(out.ImageURL) != "" {
		if b, err := c2.FetchImageBytes(ctx, out.ImageURL); err == nil && len(b) > 0 {
			out.Images = append(out.Images, b)
		}
	}

	// 8.5 图片直链：反代通道若没给 image_url，就把图传到上传接口换直链。
	// 对应 0x1005F0C2-0x1005F100：成员("image_url") 为空 -> 分支到 0x1005F1D4 上传。
	if out.ImageURL == "" && len(out.Images) > 0 {
		if u, err := c2.EnsureImageURL(ctx, out.Images[0], ""); err == nil {
			out.ImageURL = u
		}
	}

	// 9. 审核
	if g != nil && g.AuditOn && len(out.Images) > 0 {
		r := c2.CheckNSFW(ctx, g.AuditPlat, "", out.ImageURL, g.AuditThresh)
		out.Audit = r
		if r.Verdict == "违规" {
			out.Rejected = &Reject{Kind: "audit", Text: MsgViolationFiltered}
			return out, nil
		}
	}

	// 10/11. 正文
	out.Meta = ResultMeta{
		Seed:    out.Seed,
		Model:   cfg.Model,
		Sampler: cfg.Sampler,
		Noise:   cfg.Noise,
		Scale:   cfg.Scale,
		Steps:   cfg.Steps,
		Width:   cfg.Width,
		Height:  cfg.Height,
		UserQQ:  req.QQ,
		BotQQ:   req.BotQQ,
	}
	out.Caption = BuildNaiCaption(out.Meta, out.ImageURL)
	return out, nil
}

// ---------- 图片工具 ----------

// DecodeImage 把字节解成 RGBA。
func DecodeImage(b []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// ToRGBA 把任意图片转成紧密 RGBA 字节。
func ToRGBA(img image.Image) []byte {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst.Pix
}

// ToRGB 转成 RGB（丢弃 alpha），供混淆使用。
func ToRGB(img image.Image) ([]byte, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := ToRGBA(img)
	out := make([]byte, w*h*3)
	for i := 0; i < w*h; i++ {
		out[i*3] = rgba[i*4]
		out[i*3+1] = rgba[i*4+1]
		out[i*3+2] = rgba[i*4+2]
	}
	return out, w, h
}

// ---------- 官方响应解包 ----------

// ErrNotImageBytes 表示解出来的字节既不是图片也不是 zip。
var ErrNotImageBytes = &ImageError{Msg: "返回数据不是有效图片"}

// DecodeOfficialImage 把官方通道的 images[0].image 解成原始图片字节。
//
// 官方 /ai/generate-image 返回的是 base64(zip(png))；
// 反代通道通常直接给裸 base64。两者用同一函数兼容：
//  1. 去掉 "data:image/png;base64," 前缀（0x102bc274）
//  2. base64 解码
//  3. 若以 "PK\x03\x04" 开头，当 zip 解，取第一个非目录条目
//  4. 否则原样返回
func DecodeOfficialImage(s string) ([]byte, error) {
	raw, err := decodeBase64Loose(s)
	if err != nil {
		return nil, err
	}
	return unwrapZipIfAny(raw)
}

func decodeBase64Loose(s string) ([]byte, error) {
	s = trimSpaceAll(s)
	if strings.HasPrefix(s, "data:") {
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[i+1:]
		}
	}
	s = stripWhitespace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func unwrapZipIfAny(raw []byte) ([]byte, error) {
	if !isZip(raw) {
		return raw, nil
	}
	return unzipFirstFile(raw)
}

func trimSpaceAll(s string) string {
	i, j := 0, len(s)
	for i < j && isSpaceByte(s[i]) {
		i++
	}
	for j > i && isSpaceByte(s[j-1]) {
		j--
	}
	return s[i:j]
}

func stripWhitespace(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if !isSpaceByte(s[i]) {
			b = append(b, s[i])
		}
	}
	return string(b)
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// ---------- 图库查看 ----------

// GalleryViewURL 已挪到 publish.go；这里补一个解析函数。

// GalleryID 从图库链接里抠出 id 参数。
func GalleryID(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("id")
}

// ---------- 上传图片 ----------

// UploadImageEndpoint 返回图片直传接口地址。
//
// 取值顺序（复刻 0x1002CE5D 的 配置["upload-image"] + 其它通道的 {资源地址} 拼装）：
//  1. 配置项 upload-image（0x102b8d24，上传函数 0x1002CDC8 直接拿它当请求地址）；
//  2. 兜底 {资源地址} + PathUploadImage（0x102b8d22 的 "@upload-image"）。
//
// 值为空或 "0"（框架 INI 里该项的默认值）时视为未配置，调用方跳过上传。
func (c *Client) UploadImageEndpoint() string {
	if v := strings.TrimSpace(c.cfg.UploadImage); v != "" && v != "0" {
		return v
	}
	base := strings.TrimSuffix(strings.TrimSpace(c.cfg.ResourceURL), "/")
	if base == "" {
		return ""
	}
	return base + PathUploadImage
}

// UploadImage 复刻图片直传接口的 multipart 上传。
// 表单字段名 "file"，boundary 固定为 "WebKitFormBoundary"（0x102b8d3c）。
// 成功文案 0x102b8d94 "上传图片成功："，失败 0x102b8da4 "上传图片失败："。
// 返回体里的图片 URL 取 data.imageUrl（0x102b8d7c）。
func (c *Client) UploadImage(ctx context.Context, filename string, data []byte) (string, error) {
	endpoint := c.UploadImageEndpoint()
	if endpoint == "" {
		return "", &ImageError{Msg: "上传图片失败：未配置图片上传接口（upload-image / 资源地址）"}
	}
	body, ct := buildMultipart("file", filename, data)
	raw, err := c.postRaw(ctx, endpoint, ct, body)
	if err != nil {
		return "", err
	}
	var r struct {
		Data struct {
			ImageURL string `json:"imageUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", err
	}
	if r.Data.ImageURL == "" {
		return "", &ImageError{Msg: "上传图片失败：" + string(raw)}
	}
	return r.Data.ImageURL, nil
}

// EnsureImageURL 复刻 0x1005F0C2-0x1005F100 的分支：
// 响应里 image_url（0x102b94c4）为空串时，把图片字节传到上传接口换直链，
// 再把直链回填到同一个槽位（0x1005F2F5 -> [ebp-0x94]），后续正文/卡片都用它。
// 失败不改变已有 URL（返回空串与错误，调用方按未配置处理）。
func (c *Client) EnsureImageURL(ctx context.Context, data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	if filename == "" {
		filename = UploadImageFileName
	}
	return c.UploadImage(ctx, filename, data)
}
