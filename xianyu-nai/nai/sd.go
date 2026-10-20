package nai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 本文件复刻插件的 SD（Stable Diffusion WebUI）通道。
//
// ---- 配置（ini 节 sd设置 / sd参数，证据 0x1009A600-0x1009AE60）----
//
//	sd设置/链接        -> 全局 0x104EBE18（接口地址，也接受配置键 "sdapi"）
//	sd设置/账号        -> 0x104EBE94
//	sd设置/密码        -> 0x104EBE98
//	sd参数/采样        -> 结构体 +0x00，默认 "Euler a"
//	sd参数/识别率      -> +0x04，默认 "4"      （cfg_scale）
//	sd参数/差异        -> +0x08，默认 "0.5"    （denoising_strength）
//	sd参数/steps       -> +0x0c，默认 "30"
//	sd参数/负面词条    -> +0x10，默认长串 0x1039AD40
//	sd参数/宽度        -> +0x14，默认 "896"
//	sd参数/高度        -> +0x18，默认 "1472"
//	sd参数/高清修复算法-> +0x1c，默认 "Lanczos"
//	sd参数/默认种子    -> +0x20，默认 "-1"
//	sd参数/高清修复    -> 运行时读，默认 "0"，等于 "1" 才写高清修复块
//	sd参数/全局默认模型-> 0x104EBEA4
//	sd参数/完成后冷却  -> 0x1039AC9C
//	sd参数/完成冷却模式-> 0x1039ACB0
//
// ---- 接口（全部以 {sd设置/链接} 裸拼，0x102B8418 起）----
//
//	/sdapi/v1/txt2img      0x102B8418  文本消息走这条
//	/sdapi/v1/img2img      0x102B8560  图片消息走这条
//	/sdapi/v1/sd-models    0x102B7E30
//	/sdapi/v1/sd-vae       0x102B7E4C
//	/sdapi/v1/loras        0x102B96A0
//	/sdapi/v1/refresh-loras 0x102B9688
//
// ---- 鉴权（0x10024664-0x10024758）----
//
//	Authorization: Basic base64(账号 + ":" + 密码)
//	Content-Type: application/json                      (0x102B8594)
//
// ---- 请求体模板（逐字节取自 0x102B8264 / 0x102B843C）----
//
// 字段置值顺序（txt2img 0x10023743-0x100239FE、img2img 0x10023E90-0x10024181）：
// prompt、seed、steps、cfg_scale、width、height、negative_prompt、sampler_index、
// denoising_strength，之后是高清修复块与 override_settings。
//
// 类型也照抄：seed 是长整数(+0xf0)、steps/width/height 是整数(+0xf8)、
// cfg_scale/denoising_strength 是小数(+0xe8)、enable_hr 是逻辑(+0xe0)。

// SDTxt2ImgTemplate 是 txt2img 请求体模板（0x102B8264）。
const SDTxt2ImgTemplate = "{\r\n" +
	"    \"prompt\": \"关键词\",\r\n" +
	"    \"seed\": -1,\r\n" +
	"    \"steps\": 30,\r\n" +
	"    \"cfg_scale\": 7,\r\n" +
	"    \"width\": 512,\r\n" +
	"    \"height\": 768,\r\n" +
	"    \"negative_prompt\": \"负面\",\r\n" +
	"    \"sampler_index\": \"Euler\"\r\n" +
	"  }"

// SDImg2ImgTemplate 是 img2img 请求体模板（0x102B843C）。
const SDImg2ImgTemplate = "{\r\n" +
	"  \"init_images\": [\r\n" +
	"    \"string\"\r\n" +
	"  ],\r\n" +
	"    \"prompt\": \"girl,nsfw\",\r\n" +
	"    \"seed\": -1,\r\n" +
	"    \"steps\": 30,\r\n" +
	"    \"cfg_scale\": 7,\r\n" +
	"    \"width\": 512,\r\n" +
	"    \"height\": 768,\r\n" +
	"    \"negative_prompt\": \"\",\r\n" +
	"    \"denoising_strength\":0.75,\r\n" +
	"    \"sampler_index\": \"Euler\"\r\n" +
	"}"

// SD 接口路径（0x102B8418 起）。
const (
	PathSDTxt2Img     = "/sdapi/v1/txt2img"       // 0x102B8418
	PathSDImg2Img     = "/sdapi/v1/img2img"       // 0x102B8560
	PathSDModels      = "/sdapi/v1/sd-models"     // 0x102B7E30
	PathSDVAE         = "/sdapi/v1/sd-vae"        // 0x102B7E4C
	PathSDLoRAs       = "/sdapi/v1/loras"         // 0x102B96A0
	PathSDRefreshLora = "/sdapi/v1/refresh-loras" // 0x102B9688
)

// SD 请求体里用到的键（0x102B8324-0x102B8418）。
const (
	SDKeyPrompt       = "prompt"                                // 0x102B8324
	SDKeySeed         = "seed"                                  // 0x102B832C
	SDKeySteps        = "steps"                                 // 0x102B8334
	SDKeyCfgScale     = "cfg_scale"                             // 0x102B833C
	SDKeyWidth        = "width"                                 // 0x102B8348
	SDKeyHeight       = "height"                                // 0x102B8350
	SDKeyNegative     = "negative_prompt"                       // 0x102B8358
	SDKeySamplerIndex = "sampler_index"                         // 0x102B8368
	SDKeyDenoise      = "denoising_strength"                    // 0x102B8378
	SDKeyEnableHR     = "enable_hr"                             // 0x102B8398
	SDKeyHRScale      = "hr_scale"                              // 0x102B83A4
	SDKeyHRUpscaler   = "hr_upscaler"                           // 0x102B83B0
	SDKeyHRSteps      = "hr_second_pass_steps"                  // 0x102B83BC
	SDKeyModelCP      = "override_settings.sd_model_checkpoint" // 0x102B83D4
	SDKeyVAE          = "override_settings.sd_vae"              // 0x102B83FC
	SDKeyInitImages   = "init_images[0]"                        // 0x102B8540
)

// SD 通道文案。
const (
	MsgSDNotConfigured = "sd接口未配置，请配置后再来绘图" // 0x102B5D20
	MsgSDEmptyResponse = "sd接口返回为空"         // 0x102B85B8
	MsgSDLinkFailed    = "链接失败"             // 0x102B85C8
	MsgSDErrorPrefix   = "sd错误响应："          // 0x102B85E8
	MsgSDDrawFailed    = "sd绘图失败，请检查后重试"    // 0x102B85F8
)

// 高清修复块的固定值（0x10023A34-0x10023BFD）。
const (
	SDHRScale       = 2
	SDHRSecondSteps = 10
	SDHRFallbackUp  = "4x-UltraSharp" // 0x102B8550，img2img 分支的空值回退
)

// SDDefaultNegative 是 sd参数/负面词条的默认值（0x1039AD40，逐字节保留）。
const SDDefaultNegative = "lowres, bad anatomy, bad hands, text, error, missing fingers, extra digit, fewer digits, cropped, worst quality, low quality, normal quality, jpeg artifacts, signature, watermark, username, blurry,hairband,, lowres,(bad),text,error,fewer,extra,missing,worst quality,jpeg artifacts,low quality,watermark,unfinished,displeasing,oldest,early,chromatic aberration,signature,extra digits,artistic error,username,scan,[abstract],(artist name:1.2),(watermark:1.2),lowres, bad hands, text, multiple fingers, few fingers,wrong structure, wrong proportion,wrong perspective, (deformed hand:1.2), (deformed finger:1.2), deformed body, deformed facial features,wrong human body proportions, low quality, 2014, 2013, 2012, 2011, 2010,"

// SDConfig 汇总 sd设置 / sd参数 两节。
type SDConfig struct {
	URL  string // sd设置/链接（配置键 sdapi）
	User string // sd设置/账号
	Pass string // sd设置/密码

	Sampler  string  // sd参数/采样，默认 Euler a
	Identity float64 // sd参数/识别率，默认 4  -> cfg_scale
	Diff     float64 // sd参数/差异，默认 0.5 -> denoising_strength
	Steps    int     // sd参数/steps，默认 30
	Negative string  // sd参数/负面词条
	Width    int     // sd参数/宽度，默认 896
	Height   int     // sd参数/高度，默认 1472
	HiresUp  string  // sd参数/高清修复算法，默认 Lanczos
	Seed     string  // sd参数/默认种子，默认 -1（存文本，保持 "0" 有意义）

	Hires        bool   // sd参数/高清修复 == "1"
	GlobalModel  string // sd参数/全局默认模型
	Cooldown     string // sd参数/完成后冷却
	CooldownMode string // sd参数/完成冷却模式

	TimeoutMS int
}

// DefaultSDConfig 返回 sd设置 / sd参数 的默认值（0x1009A600-0x1009AE60）。
func DefaultSDConfig() SDConfig {
	return SDConfig{
		Sampler:   "Euler a", // 0x1039AD38
		Identity:  4,         // 0x1039AD1C 默认 "4"
		Diff:      0.5,       // 0x102B8xxx 默认 "0.5"
		Steps:     30,        // 0x1039AD18 默认 "30"
		Negative:  SDDefaultNegative,
		Width:     896,       // 0x1039AD34
		Height:    1472,      // 0x1039AD2C
		HiresUp:   "Lanczos", // 0x1039B014
		Seed:      "-1",      // 0x102B8DD0
		TimeoutMS: 300000,
	}
}

// withDefaults 给零值字段补默认值。注意 Seed 用文本保存：
// "" 才补 "-1"，"0" 是合法种子，不会被改写。
func (c SDConfig) withDefaults() SDConfig {
	d := DefaultSDConfig()
	if c.Sampler == "" {
		c.Sampler = d.Sampler
	}
	if c.Identity == 0 {
		c.Identity = d.Identity
	}
	if c.Diff == 0 {
		c.Diff = d.Diff
	}
	if c.Steps == 0 {
		c.Steps = d.Steps
	}
	if c.Negative == "" {
		c.Negative = d.Negative
	}
	if c.Width == 0 {
		c.Width = d.Width
	}
	if c.Height == 0 {
		c.Height = d.Height
	}
	if c.HiresUp == "" {
		c.HiresUp = d.HiresUp
	}
	if c.Seed == "" {
		c.Seed = d.Seed
	}
	if c.TimeoutMS <= 0 {
		c.TimeoutMS = d.TimeoutMS
	}
	return c
}

// SDWithDefaults 给零值字段补默认值（供包外调用）。
func SDWithDefaults(c SDConfig) SDConfig { return c.withDefaults() }

// SDBaseURL 返回可用的接口基址；空串表示未配置。
func (c SDConfig) SDBaseURL() string {
	return strings.TrimSpace(c.URL)
}

// SDAuthHeader 复刻 "Authorization: Basic " + base64(账号 + ":" + 密码)。
//
// 0x10024664-0x10024758：先拼 账号 + ":" + 密码（最后压栈的在前），
// 再调 0x101C8200 做 Base64，最后与 0x102B7E0C 拼接。
func (c SDConfig) SDAuthHeader() string {
	raw := c.User + ":" + c.Pass
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// SDOptions 是一次 SD 出图的参数（已从配置解析或经 # 参数覆盖）。
type SDOptions struct {
	Prompt    string
	Negative  string
	Seed      string // "" 用配置默认种子
	Steps     int
	CFG       float64
	Width     int
	Height    int
	Sampler   string
	Model     string // override_settings.sd_model_checkpoint
	VAE       string // override_settings.sd_vae
	InitImage string // img2img 的 init_images[0]（base64）
}

// fill 用配置补足未指定的字段。
func (o SDOptions) fill(c SDConfig) SDOptions {
	if o.Seed == "" {
		o.Seed = c.Seed
	}
	if o.Steps <= 0 {
		o.Steps = c.Steps
	}
	if o.CFG == 0 {
		o.CFG = c.Identity
	}
	if o.Width <= 0 {
		o.Width = c.Width
	}
	if o.Height <= 0 {
		o.Height = c.Height
	}
	if o.Sampler == "" {
		o.Sampler = c.Sampler
	}
	if o.Negative == "" {
		o.Negative = c.Negative
	}
	if o.Model == "" {
		o.Model = c.GlobalModel
	}
	return o
}

// sdNum 把浮点格式化成 JSON 数字文本（4 -> "4"，0.5 -> "0.5"）。
func sdNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// SDSeedValue 把种子文本转成数字文本，非法值按 -1 处理。
func SDSeedValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-1"
	}
	if _, err := strconv.ParseInt(s, 10, 64); err != nil {
		return "-1"
	}
	return s
}

// BuildSDTxt2Img 按 DLL 的字段顺序生成 txt2img 请求体。
func BuildSDTxt2Img(c SDConfig, o SDOptions) (string, error) {
	o = o.fill(c)
	root, err := decodeOJSON([]byte(SDTxt2ImgTemplate))
	if err != nil {
		return "", err
	}
	root = root.set(parsePath(SDKeyPrompt), o.Prompt)
	root = root.setNum(parsePath(SDKeySeed), SDSeedValue(o.Seed))
	root = root.setNum(parsePath(SDKeySteps), strconv.Itoa(o.Steps))
	root = root.setNum(parsePath(SDKeyCfgScale), sdNum(o.CFG))
	root = root.setNum(parsePath(SDKeyWidth), strconv.Itoa(o.Width))
	root = root.setNum(parsePath(SDKeyHeight), strconv.Itoa(o.Height))
	root = root.set(parsePath(SDKeyNegative), o.Negative)
	root = root.set(parsePath(SDKeySamplerIndex), o.Sampler)
	root = root.setNum(parsePath(SDKeyDenoise), sdNum(c.Diff))
	root = applySDHires(root, c, o, "")
	root = applySDOverrides(root, o)
	return root.Text(), nil
}

// BuildSDImg2Img 按 DLL 的字段顺序生成 img2img 请求体。
func BuildSDImg2Img(c SDConfig, o SDOptions) (string, error) {
	o = o.fill(c)
	root, err := decodeOJSON([]byte(SDImg2ImgTemplate))
	if err != nil {
		return "", err
	}
	root = root.set(parsePath(SDKeyInitImages), o.InitImage)
	root = root.set(parsePath(SDKeyPrompt), o.Prompt)
	root = root.setNum(parsePath(SDKeySeed), SDSeedValue(o.Seed))
	root = root.setNum(parsePath(SDKeySteps), strconv.Itoa(o.Steps))
	root = root.setNum(parsePath(SDKeyCfgScale), sdNum(o.CFG))
	root = root.setNum(parsePath(SDKeyWidth), strconv.Itoa(o.Width))
	root = root.setNum(parsePath(SDKeyHeight), strconv.Itoa(o.Height))
	root = root.set(parsePath(SDKeyNegative), o.Negative)
	root = root.set(parsePath(SDKeySamplerIndex), o.Sampler)
	root = root.setNum(parsePath(SDKeyDenoise), sdNum(c.Diff))
	root = applySDHires(root, c, o, SDHRFallbackUp)
	root = applySDOverrides(root, o)
	return root.Text(), nil
}

// applySDHires 复刻高清修复块（0x10023A34-0x10023BFD）。
//
// 闸门：读配置项(ini, "sd参数", "高清修复", "0") == "1"。
// 写入：enable_hr=1（逻辑）、hr_scale=2、hr_upscaler=配置的高清修复算法、
// hr_second_pass_steps=10。img2img 分支的空值回退是 "4x-UltraSharp"。
func applySDHires(root *ojson, c SDConfig, o SDOptions, fallback string) *ojson {
	if !c.Hires {
		return root
	}
	up := c.HiresUp
	if up == "" && fallback != "" {
		up = fallback
	}
	root = root.setBool(parsePath(SDKeyEnableHR), true)
	root = root.setNum(parsePath(SDKeyHRScale), strconv.Itoa(SDHRScale))
	root = root.set(parsePath(SDKeyHRUpscaler), up)
	root = root.setNum(parsePath(SDKeyHRSteps), strconv.Itoa(SDHRSecondSteps))
	return root
}

// applySDOverrides 复刻模型/VAE 覆盖：值非空且不等于 "0" 才写。
func applySDOverrides(root *ojson, o SDOptions) *ojson {
	if v := strings.TrimSpace(o.Model); v != "" && v != "0" {
		root = root.set(parsePath(SDKeyModelCP), v)
	}
	if v := strings.TrimSpace(o.VAE); v != "" && v != "0" {
		root = root.set(parsePath(SDKeyVAE), v)
	}
	return root
}

// SDResult 是 SD 出图的解析结果。
type SDResult struct {
	Images   []string // images[]，base64
	URLs     []string // urls[]，部分实现直接给直链
	Seed     int64
	Model    string // info.sd_model_name
	Sampler  string // info.sampler_name
	Prompt   string // info.prompt
	Info     string // 原始 info 串
	ImageURL string // urls[0]（部分实现直接给直链）
	Raw      []byte
}

// sdInfo 是 SD 响应里 info 的常用字段（SD WebUI 把 info 当字符串给）。
type sdInfo struct {
	Prompt      string `json:"prompt"`
	Seed        int64  `json:"seed"`
	ModelName   string `json:"sd_model_name"`
	SamplerName string `json:"sampler_name"`
}

// SDGenerate 发起一次 SD 出图。initImage 非空走 img2img，否则走 txt2img。
func (c *Client) SDGenerate(ctx context.Context, o SDOptions) (*SDResult, error) {
	cfg := c.cfg.SD.withDefaults()
	if cfg.SDBaseURL() == "" {
		return nil, fmt.Errorf("%s", MsgSDNotConfigured)
	}
	var (
		path = PathSDTxt2Img
		body string
		err  error
	)
	if strings.TrimSpace(o.InitImage) != "" {
		path = PathSDImg2Img
		body, err = BuildSDImg2Img(cfg, o)
	} else {
		body, err = BuildSDTxt2Img(cfg, o)
	}
	if err != nil {
		return nil, err
	}
	raw, err := c.sdDo(ctx, http.MethodPost, cfg.SDBaseURL()+path, body)
	if err != nil {
		return nil, err
	}
	return ParseSDResponse(raw)
}

// sdDo 发一次带 Basic 鉴权的请求。
//
// 传输层沿用插件的 HTTP 封装（超时 4x300000ms、关证书校验），
// 这里直接用 Client.HTTP。
func (c *Client) sdDo(ctx context.Context, method, url, body string) ([]byte, error) {
	var rdr io.Reader
	if method == http.MethodPost {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	cfg := c.cfg.SD.withDefaults()
	req.Header.Set("Content-Type", "application/json") // 0x102B8594
	req.Header.Set("Authorization", cfg.SDAuthHeader())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", MsgSDLinkFailed) // 0x102B85C8
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s", MsgSDLinkFailed)
	}
	return out, nil
}

// ParseSDResponse 复刻 SD 响应解析（0x10024A09-0x100251B8）。
func ParseSDResponse(raw []byte) (*SDResult, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("%s", MsgSDEmptyResponse) // 0x102B85B8
	}
	var root struct {
		Images []string        `json:"images"`
		URLs   []string        `json:"urls"`
		Info   json.RawMessage `json:"info"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%s%s", MsgSDErrorPrefix, string(raw)) // 0x102B85E8
	}
	if root.Error != "" {
		return nil, fmt.Errorf("%s%s", MsgSDErrorPrefix, root.Error)
	}
	hasImg := false
	for _, s := range root.Images {
		if strings.TrimSpace(s) != "" {
			hasImg = true
			break
		}
	}
	hasURL := false
	for _, s := range root.URLs {
		if strings.TrimSpace(s) != "" {
			hasURL = true
			break
		}
	}
	if !hasImg && !hasURL {
		return nil, fmt.Errorf("%s", MsgSDDrawFailed) // 0x102B85F8
	}

	out := &SDResult{Images: root.Images, URLs: root.URLs, Raw: raw}
	info := decodeSDInfo(root.Info)
	out.Info = info.raw
	out.Seed = info.v.Seed
	out.Model = info.v.ModelName
	out.Sampler = info.v.SamplerName
	out.Prompt = info.v.Prompt
	if len(out.URLs) > 0 {
		out.ImageURL = out.URLs[0]
	}
	return out, nil
}

type sdInfoBox struct {
	v   sdInfo
	raw string
}

// decodeSDInfo 兼容 info 为 JSON 字符串或 JSON 对象两种写法。
func decodeSDInfo(raw json.RawMessage) sdInfoBox {
	var box sdInfoBox
	if len(raw) == 0 {
		return box
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		box.raw = s
		_ = json.Unmarshal([]byte(s), &box.v)
		return box
	}
	box.raw = string(raw)
	_ = json.Unmarshal(raw, &box.v)
	return box
}

// SDModel 是 /sdapi/v1/sd-models 的条目。
type SDModel struct {
	Title     string `json:"title"`
	ModelName string `json:"model_name"`
	Hash      string `json:"hash"`
	SHA256    string `json:"sha256"`
	FileName  string `json:"filename"`
}

// SDModels 拉取模型列表（0x102B7E30）。
func (c *Client) SDModels(ctx context.Context) ([]SDModel, error) {
	cfg := c.cfg.SD.withDefaults()
	if cfg.SDBaseURL() == "" {
		return nil, fmt.Errorf("%s", MsgSDNotConfigured)
	}
	raw, err := c.sdDo(ctx, http.MethodGet, cfg.SDBaseURL()+PathSDModels, "")
	if err != nil {
		return nil, err
	}
	var out []SDModel
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s%s", MsgSDErrorPrefix, string(raw))
	}
	return out, nil
}

// SDVAE 拉取 VAE 列表（0x102B7E4C）。SD WebUI 返回 [{model_name,filename,hash},…]。
type SDVAE struct {
	ModelName string `json:"model_name"`
	FileName  string `json:"filename"`
	Hash      string `json:"hash"`
}

// SDVAEs 拉取 VAE 列表。
func (c *Client) SDVAEs(ctx context.Context) ([]SDVAE, error) {
	cfg := c.cfg.SD.withDefaults()
	if cfg.SDBaseURL() == "" {
		return nil, fmt.Errorf("%s", MsgSDNotConfigured)
	}
	raw, err := c.sdDo(ctx, http.MethodGet, cfg.SDBaseURL()+PathSDVAE, "")
	if err != nil {
		return nil, err
	}
	var out []SDVAE
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s%s", MsgSDErrorPrefix, string(raw))
	}
	return out, nil
}

// SDLora 是 /sdapi/v1/loras 的条目（只取用到的字段）。
type SDLora struct {
	Name   string `json:"name"`
	Alias  string `json:"alias"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// SDLoRAs 拉取 LoRA 列表（0x102B96A0）。
func (c *Client) SDLoRAs(ctx context.Context) ([]SDLora, error) {
	cfg := c.cfg.SD.withDefaults()
	if cfg.SDBaseURL() == "" {
		return nil, fmt.Errorf("%s", MsgSDNotConfigured)
	}
	raw, err := c.sdDo(ctx, http.MethodGet, cfg.SDBaseURL()+PathSDLoRAs, "")
	if err != nil {
		return nil, err
	}
	var out []SDLora
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s%s", MsgSDErrorPrefix, string(raw))
	}
	return out, nil
}

// SDRefreshLoRAs 触发接口重扫 LoRA 目录（0x102B9688）。
func (c *Client) SDRefreshLoRAs(ctx context.Context) error {
	cfg := c.cfg.SD.withDefaults()
	if cfg.SDBaseURL() == "" {
		return fmt.Errorf("%s", MsgSDNotConfigured)
	}
	_, err := c.sdDo(ctx, http.MethodPost, cfg.SDBaseURL()+PathSDRefreshLora, "{}")
	return err
}

// RenderSDModels 复刻 "sd查看模型" 的表格（0x1003C94B-0x1003CB6D）。
//
// 表头：===【sd模型列表】===
//
//	| 序号 | 模型名 | hash |
//	| ----- | ----- | ----- |
//
// 每行：| 序号 | [序号-1].model_name | [序号-1].hash |
//
// 注意：序号是 1 基，取值下标是 0 基（0x1003CAB3 的 fsub 1.0）。
func RenderSDModels(models []SDModel) string {
	var b strings.Builder
	b.WriteString("===【sd模型列表】===\r\n")            // 0x102B9568
	b.WriteString("| 序号 | 模型名 | hash |\r\n")       // 0x102B9580
	b.WriteString("| ----- | ----- | ----- |\r\n") // 0x102B9580 后半
	for i, m := range models {
		name := m.ModelName
		if name == "" {
			name = m.Title
		}
		fmt.Fprintf(&b, "| %d | %s | %s |\r\n", i+1, name, m.Hash)
	}
	return b.String()
}

// RenderSDVAEs 是 sd查看vae 的纯文本兜底（DLL 里是上传 JSON 换图库链接，
// 见 SDUploadList；此处的逐行格式未在二进制里找到常量，标记未确认）。
func RenderSDVAEs(list []SDVAE) string {
	var b strings.Builder
	for i, v := range list {
		name := v.ModelName
		if name == "" {
			name = v.FileName
		}
		fmt.Fprintf(&b, "%d. %s\r\n", i+1, name)
	}
	return b.String()
}

// RenderSDLoRAs 是 sd查看lora 的纯文本兜底（同上，标记未确认）。
func RenderSDLoRAs(list []SDLora) string {
	var b strings.Builder
	for i, v := range list {
		name := v.Name
		if name == "" {
			name = v.Alias
		}
		fmt.Fprintf(&b, "%d. %s\r\n", i+1, name)
	}
	return b.String()
}

// SD 列表上传地址（0x102B9624 / 0x102B96D0 / 0x102B965C / 0x102B97CC）。
const (
	PathSDVaeUp    = "/plug/AI/v1/vae/up.php"              // 0x102B9624
	PathSDLoraUp   = "/plug/AI/v1/lora/up.php"             // 0x102B96D0
	PathSDVaeView  = "/plug/AI?id=/plug/AI/v1/vae/?name="  // 0x102B965C
	PathSDLoraView = "/plug/AI?id=/plug/AI/v1/lora/?name=" // 0x102B97CC
)

// SDUploadList 复刻 "把列表 JSON 丢给资源站换查看链接" 的链路：
// GET {资源地址}<up.php>?name=<name>&json=<urlencoded json>，
// 返回 {资源地址}<view>?name=<name>。
func (c *Client) SDUploadList(ctx context.Context, upPath, viewPath, name, listJSON string) (string, error) {
	base := strings.TrimSpace(c.cfg.ResourceURL)
	if base == "" {
		return "", fmt.Errorf("资源地址未配置")
	}
	u := base + upPath + "?name=" + urlQueryEscape(name) + "&json=" + urlQueryEscape(listJSON)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s", MsgSDLinkFailed)
	}
	resp.Body.Close()
	return base + viewPath + urlQueryEscape(name), nil
}

// urlQueryEscape 只做最小转义（与插件 urlencode 行为一致：空格转 %20）。
func urlQueryEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// SDAvailable 报告 SD 通道是否可用。
func (c *Client) SDAvailable() bool {
	return c.cfg.SD.SDBaseURL() != ""
}

// sdTimeout 让调用方可以按需延长超时（SD 出图经常 > 5 分钟）。
func sdTimeout(cfg SDConfig) time.Duration {
	t := cfg.TimeoutMS
	if t <= 0 {
		t = 300000
	}
	return time.Duration(t) * time.Millisecond
}
