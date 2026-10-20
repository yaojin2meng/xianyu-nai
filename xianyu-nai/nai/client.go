package nai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config 对应插件 ini 里的 sections.nai.* 配置项。
// 默认值取自 DLL 初始化段（RVA 0x100A3DE3-0x100A5900）。
type Config struct {
	Mode        string // naiMode：值为 "官方" 时走官方通道，否则走反代
	Key         string // naiKey：反代通道的 Bearer Token
	OfficialKey string // naiOfficialKey：官方通道的 Bearer Token
	OfficialURL string // naiofficialapi：官方地址，插件不内置默认值
	ProxyURL    string // nai3api：第三方反代地址，插件不内置默认值
	PluginAPI   string // nai3_plugin-api：鉴黄/超分/去背景/反推的 base（服务端下发）
	ResourceURL string // 资源地址：图库上传与卡片模板的 base（服务端下发）
	UploadImage string // upload-image（0x102b8d24）：图片直传接口；空或 "0" 时用 {资源地址}+/plug/AI/upload-image

	Model     string  // naiModel
	Scale     float64 // naiScale
	Cfg       float64 // naiCfg
	Steps     int     // naiSteps
	Sampler   string  // naiSampler
	Noise     string  // naiNoise
	Smea      bool    // naiSmea
	Dyn       bool    // naiDyn
	Seed      int64   // naiSeed
	Width     int     // naiWidth
	Height    int     // naiHeight
	Strength  float64 // naiStrength
	Negative  string  // naiNegative
	TimeoutMS int

	TranslateAPI     string // translation_api：tag / youdao / ai
	AITranslateToken string // AI翻译令牌

	BlockWords string // sections.home.blockWords（[基本] 屏蔽词条）
	BasePoints int64  // sections.home.basePoints（[点数限制] 基础点数）
	PointsTip  string // sections.home.pointsTip（[点数限制] 点数不足提示）

	SendMode string // sections.home.sendMode（0x1039b7c0）：json/链接发送/卡片跳转/合并转发/图片

	// 门禁：取自 [点数限制] / [图像审核] / [黑名单功能] / [点数消耗]。
	// 原插件把这几节读进 sections.home.*，重制版直接从 ini 读。
	PointsOn   bool   // [点数限制] 开关（0 = 关）
	PointsType string // [点数限制] 点数处理类型：全局 / 个人
	AuditOn    bool   // [图像审核] 开关
	AuditPlat  string // [图像审核] 平台：api4ai / baidu / baiduai
	AuditTh    string // [图像审核] 阈值
	AuditAct   string // [图像审核] 处理：处理/不处理/模糊处理/链接发送/过滤发送/图片混淆

	// 各通道单次消耗（[点数消耗]）。
	CostSD    int64 // sd画图
	CostNai   int64 // nai画图
	CostComfy int64 // comfy画图

	// 黑名单豁免功能（[黑名单功能] 的取值是该功能的指令名）。
	BLExemptSD     string // sd画图
	BLExemptNai    string // nai画图
	BLExemptUpscal string // 清晰术
	BLExemptRmBg   string // 去背景
	BLExemptTag    string // 上传标签
	BLExemptPreset string // 上传预设

	SD SDConfig // sd设置 / sd参数（SD 通道，见 sd.go）
	// Comfy 是 comfy设置 / comfy参数 / comfy指令（ComfyUI 通道，见 comfy.go）
	Comfy ComfyConfig

	// DataDir 是插件数据目录（咸鱼ai画图.ini / seed.ini / Negative.ini /
	// sd预设列表.json 所在处）。空表示当前目录。
	DataDir string
}

// DefaultNegative 是 DLL 里 naiNegative 的默认串（VA 0x1039AB60）。
const DefaultNegative = "lowres, {bad}, error, fewer, extra, missing, worst quality, " +
	"jpeg artifacts, bad quality, watermark, unfinished, displeasing, " +
	"chromatic aberration, signature, extra digits, artistic error, " +
	"username, scan, [abstract]"

// DefaultConfig 返回插件初始化时写入的默认配置。
func DefaultConfig() Config {
	return Config{
		Mode:      "中转",              // 0x100A4051 读 [nai3设置] 模式，默认串 0x102B5C04 = "中转"
		Model:     "nai-diffusion-3", // 反代模板默认模型
		Scale:     5,                 // 模板 "scale": 5
		Cfg:       0.28,              // nai3参数/cfg 默认 "0.28"（0x100A4282 → 0x1039AC60）
		Steps:     28,
		Sampler:   "k_euler",
		Noise:     "native",
		Smea:      false,
		Dyn:       false,
		Seed:      -1,
		Width:     1024,
		Height:    1024,
		Strength:  0.52, // nai3参数/strength 默认 "0.52"（0x100A433D → 0x1039AB54）
		Negative:  DefaultNegative,
		TimeoutMS: 300000,
		SD:        DefaultSDConfig(),
		Comfy:     DefaultComfyConfig(),
	}
}

// AugmentType 对应 augment-image 的 req_type。
type AugmentType string

const (
	AugmentEmotion   AugmentType = "emotion"
	AugmentDeclutter AugmentType = "declutter"
	AugmentColorize  AugmentType = "colorize"
	AugmentSketch    AugmentType = "sketch"
	AugmentLineart   AugmentType = "lineart"
)

// Client 复刻插件的两条 NAI 通道。
type Client struct {
	cfg  Config
	HTTP *http.Client
}

func New(cfg Config) *Client {
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 300000
	}
	return &Client{
		cfg:  cfg,
		HTTP: &http.Client{Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond},
	}
}

// isOfficial 复刻 0x1005E3F3 的判断：naiMode 与 "官方" 比较。
func (c *Client) isOfficial() bool {
	return c.cfg.Mode == "官方"
}

// IsOfficial 对外暴露通道判定，供 CLI 选择请求体构造函数。
func (c *Client) IsOfficial() bool { return c.isOfficial() }

// ---------- 官方通道请求体（与 DLL 逐字段对齐） ----------

type V4Caption struct {
	BaseCaption  string        `json:"base_caption"`
	CharCaptions []interface{} `json:"char_captions"`
}

type V4Prompt struct {
	Caption   V4Caption `json:"caption"`
	UseCoords bool      `json:"use_coords"`
	UseOrder  bool      `json:"use_order"`
}

type V4NegativePrompt struct {
	Caption V4Caption `json:"caption"`
}

// OfficialParameters 字段顺序与 DLL 写入顺序一致。
// 注意 width/height/steps 在原生插件里是字符串（走 vtable+0xcc 文本写入），
// 这里用 string 保持逐字节一致；官方接口本身也接受字符串。
type OfficialParameters struct {
	Width           string           `json:"width"`
	Height          string           `json:"height"`
	Scale           float64          `json:"scale"`
	Sampler         string           `json:"sampler"`
	Steps           string           `json:"steps"`
	Seed            int64            `json:"seed"`
	N               int              `json:"n_samples"`
	UCPreset        int              `json:"ucPreset"`
	QualityToggle   bool             `json:"qualityToggle"`
	NegativePrompt  string           `json:"negative_prompt"`
	CfgRescale      float64          `json:"cfg_rescale"`
	NoiseSchedule   string           `json:"noise_schedule"`
	DynThreshold    bool             `json:"dynamic_thresholding"`
	ControlStrength int              `json:"controlnet_strength"`
	Legacy          bool             `json:"legacy"`
	LegacyV3Extend  bool             `json:"legacy_v3_extend"`
	AddOriginal     bool             `json:"add_original_image"`
	ParamsVersion   int              `json:"params_version"`
	UseCoords       bool             `json:"use_coords"`
	CharacterPrompt []interface{}    `json:"characterPrompts"`
	V4Prompt        V4Prompt         `json:"v4_prompt"`
	V4NegPrompt     V4NegativePrompt `json:"v4_negative_prompt"`
	DeliberateBug   bool             `json:"deliberate_euler_ancestral_bug"`
	PreferBrownian  bool             `json:"prefer_brownian"`
	Sm              bool             `json:"sm"`
	SmDyn           bool             `json:"sm_dyn"`
	RefImages       []interface{}    `json:"reference_image_multiple"`
	RefInfo         []interface{}    `json:"reference_information_extracted_multiple"`
	RefStrength     []interface{}    `json:"reference_strength_multiple"`
	Image           string           `json:"image,omitempty"`
	Strength        float64          `json:"strength"`
	Noise           float64          `json:"noise"`
	ExtraNoiseSeed  int64            `json:"extra_noise_seed"`
}

type OfficialRequest struct {
	Input      string             `json:"input"`
	Model      string             `json:"model"`
	Action     string             `json:"action"`
	Parameters OfficialParameters `json:"parameters"`
}

// BuildOfficialRequest 复刻 fcn.10062cd8 的 /ai/generate-image body 构造（RVA 0x63DE3-0x64A30）。
// inputImage 为 base64 原图；非空时 action=img2img 且 add_original_image 被改回 false（0x10064A03）。
func (c *Client) BuildOfficialRequest(prompt, negative, inputImage string) OfficialRequest {
	action := "generate"
	addOriginal := true
	image := ""
	if inputImage != "" {
		action = "img2img"
		addOriginal = false
		image = inputImage
	}
	return OfficialRequest{
		Input:  "",
		Model:  c.cfg.Model,
		Action: action,
		Parameters: OfficialParameters{
			Width:           fmt.Sprint(c.cfg.Width),  // 原生默认 "1024"
			Height:          fmt.Sprint(c.cfg.Height), // 原生默认 "1024"
			Scale:           c.cfg.Scale,              // 5
			Sampler:         c.cfg.Sampler,            // k_euler
			Steps:           fmt.Sprint(c.cfg.Steps),  // 原生默认 "28"
			Seed:            c.cfg.Seed,
			N:               1,
			UCPreset:        0,
			QualityToggle:   true,
			NegativePrompt:  negative,
			CfgRescale:      0,
			NoiseSchedule:   c.cfg.Noise, // native
			DynThreshold:    false,
			ControlStrength: 1,
			Legacy:          false,
			LegacyV3Extend:  false,
			AddOriginal:     addOriginal,
			ParamsVersion:   3,
			UseCoords:       false,
			CharacterPrompt: []interface{}{},
			V4Prompt: V4Prompt{
				Caption:   V4Caption{BaseCaption: prompt, CharCaptions: []interface{}{}},
				UseCoords: false,
				UseOrder:  true,
			},
			V4NegPrompt: V4NegativePrompt{
				Caption: V4Caption{BaseCaption: negative, CharCaptions: []interface{}{}},
			},
			DeliberateBug:  false,
			PreferBrownian: false,
			Sm:             c.cfg.Smea,
			SmDyn:          c.cfg.Dyn,
			RefImages:      []interface{}{},
			RefInfo:        []interface{}{},
			RefStrength:    []interface{}{},
			Image:          image,
			Strength:       c.cfg.Strength,
			Noise:          0,
			ExtraNoiseSeed: 0,
		},
	}
}

// AugmentRequest 复刻 /ai/augment-image 的 body（RVA 0x63941-0x63B49）。
// 只有 req_type / image / width / height / prompt / defry 六个字段，与生成接口完全不同构。
type AugmentRequest struct {
	ReqType AugmentType `json:"req_type"`
	Image   string      `json:"image"`
	Width   int         `json:"width"`
	Height  int         `json:"height"`
	Prompt  string      `json:"prompt"`
	Defry   int         `json:"defry"`
}

// BuildAugmentRequest 构造导演工具请求。defry 在原生实现里是常量 0（0x10063AD9）。
func (c *Client) BuildAugmentRequest(t AugmentType, image, prompt string) AugmentRequest {
	return AugmentRequest{
		ReqType: t,
		Image:   image,
		Width:   c.cfg.Width,
		Height:  c.cfg.Height,
		Prompt:  prompt,
		Defry:   0,
	}
}

// ---------- 反代通道请求体 ----------

// ProxyRequest 对应 VA 0x102BA5F8 的模板，字段名与顺序与模板一致。
// 模板中 width/height/steps 是数字，"seed" 是字符串 —— 这是作者原样保留的怪异之处，
// 复刻时保持原样以确保与反代服务端兼容。
type ProxyRequest struct {
	Model                 string  `json:"model"`
	PositivePrompt        string  `json:"positivePrompt"`
	NegativePrompt        string  `json:"negativePrompt"`
	Scale                 float64 `json:"scale"`
	Steps                 int     `json:"steps"`
	Width                 int     `json:"width"`
	Height                int     `json:"height"`
	PromptGuidanceRescale float64 `json:"promptGuidanceRescale"`
	NoiseSchedule         string  `json:"noise_schedule"`
	Seed                  string  `json:"seed"`
	Sampler               string  `json:"sampler"`
	Sm                    bool    `json:"sm"`
	SmDyn                 bool    `json:"sm_dyn"`
}

// BuildProxyRequest 复刻 fcn.1005B4E8 中模板解析后逐字段改写的结果。
func (c *Client) BuildProxyRequest(prompt, negative string) ProxyRequest {
	return ProxyRequest{
		Model:                 c.cfg.Model,
		PositivePrompt:        prompt,
		NegativePrompt:        negative,
		Scale:                 c.cfg.Scale,
		Steps:                 c.cfg.Steps,
		Width:                 c.cfg.Width,
		Height:                c.cfg.Height,
		PromptGuidanceRescale: c.cfg.Cfg,
		NoiseSchedule:         c.cfg.Noise,
		Seed:                  fmt.Sprint(c.cfg.Seed),
		Sampler:               c.cfg.Sampler,
		Sm:                    c.cfg.Smea,
		SmDyn:                 c.cfg.Dyn,
	}
}

// ---------- 发送 ----------

// Result 统一了两条通道的返回。
type Result struct {
	Images   []string `json:"images"` // base64 图片（官方为 zip 包 base64）
	Seed     int64    `json:"seed"`
	Prompt   string   `json:"prompt"`
	ImageURL string   `json:"image_url,omitempty"` // 反代通道可能直接给 URL
	Raw      []byte   `json:"-"`
}

// Generate 按 naiMode 选择通道并发送文生图/图生图请求。
func (c *Client) Generate(ctx context.Context, prompt, negative, inputImage string) (*Result, error) {
	if c.isOfficial() {
		return c.official(ctx, "/ai/generate-image", c.BuildOfficialRequest(prompt, negative, inputImage))
	}
	return c.proxy(ctx, prompt, negative)
}

// Augment 走官方 /ai/augment-image（导演工具）。
func (c *Client) Augment(ctx context.Context, t AugmentType, image, prompt string) (*Result, error) {
	if c.cfg.OfficialKey == "" {
		return nil, ErrOfficialKeyMissing
	}
	return c.official(ctx, "/ai/augment-image", c.BuildAugmentRequest(t, image, prompt))
}

var (
	ErrKeyMissing         = fmt.Errorf("nai3key未配置，请配置后再来绘图")
	ErrOfficialKey        = fmt.Errorf("官方key未配置，请配置后再来绘图")
	ErrOfficialKeyMissing = fmt.Errorf("官方接口未配置(naiofficialapi)")
	ErrEmptyResponse      = fmt.Errorf("nai接口返回为空")
	ErrLinkFailed         = fmt.Errorf("nai链接失败")
)

// joinURL 复刻插件的拼接方式：官方通道会判断 base 是否已以 "/" 结尾（0x10063D4B / 0x100637F7），
// 反代通道直接裸拼（0x1005E4F3），不处理末尾斜杠。
func joinURL(base, path string, checkSlash bool) string {
	if checkSlash {
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		return base + strings.TrimPrefix(path, "/")
	}
	return base + path
}

func (c *Client) official(ctx context.Context, path string, body interface{}) (*Result, error) {
	if c.cfg.OfficialKey == "" {
		return nil, ErrOfficialKey
	}
	if c.cfg.OfficialURL == "" {
		return nil, ErrOfficialKeyMissing
	}
	url := joinURL(c.cfg.OfficialURL, path, true)
	raw, err := c.postJSON(ctx, url, c.cfg.OfficialKey, body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		StatusCode int    `json:"statusCode"`
		Message    string `json:"message"`
		Images     []struct {
			Image string `json:"image"`
			Seed  int64  `json:"seed"`
		} `json:"images"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("官方接口返回异常: %w", err)
	}
	if resp.StatusCode != 0 || resp.Message != "" {
		// 复刻 0x64D2C：只要 statusCode 或 message 出现就按错误处理
		return nil, fmt.Errorf("官方接口错误：%s", resp.Message)
	}
	if len(resp.Images) == 0 {
		return nil, fmt.Errorf("官方接口返回异常")
	}
	return &Result{Images: []string{resp.Images[0].Image}, Seed: resp.Images[0].Seed, Raw: raw}, nil
}

func (c *Client) proxy(ctx context.Context, prompt, negative string) (*Result, error) {
	if c.cfg.Key == "" {
		return nil, ErrKeyMissing
	}
	url := joinURL(c.cfg.ProxyURL, "/api/generate_image", false)
	raw, err := c.postJSON(ctx, url, c.cfg.Key, c.BuildProxyRequest(prompt, negative))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrEmptyResponse
	}
	var resp struct {
		Info struct {
			Seed           int64  `json:"seed"`
			PositivePrompt string `json:"positivePrompt"`
		} `json:"info"`
		Images   []string `json:"images"`
		ImageURL string   `json:"image_url"`
		Message  string   `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("nai错误响应：%s", string(raw))
	}
	if resp.Message != "" {
		return nil, fmt.Errorf("nai错误响应：%s", resp.Message)
	}
	if resp.ImageURL == "" && len(resp.Images) == 0 {
		return nil, fmt.Errorf("nai错误响应：%s", string(raw))
	}
	return &Result{
		Images:   resp.Images,
		Seed:     resp.Info.Seed,
		Prompt:   resp.Info.PositivePrompt,
		ImageURL: resp.ImageURL,
		Raw:      raw,
	}, nil
}

func (c *Client) postJSON(ctx context.Context, url, key string, body interface{}) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrLinkFailed
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrLinkFailed
	}
	return out, nil
}
