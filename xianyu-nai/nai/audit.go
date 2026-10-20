package nai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// 本文件为"插件其余接口"：鉴黄、图库上传、超分（清晰术）、去背景、图片反推、图片元数据。
// 除图库上传走 {资源地址} 外，其余全部走配置项 nai3_plugin-api 这个 base。
// 证据：全部函数的 base 都取自 0x104ebe78 根对象上的 "nai3_plugin-api"（0x102b8dec），
// 取值方式 call [eax+0x114]。

// AuditPlatform 对应 sections.home.auditPlatform 的取值（fcn.1002d7f0 三次字符串比较）。
type AuditPlatform string

const (
	PlatformAPI4AI  AuditPlatform = "api4ai"  // 0x1002e009, 返回路径 results[0].entities[0].classes.nsfw
	PlatformBaidu   AuditPlatform = "baidu"   // 0x1002e4fd, 返回路径 result.conclusion
	PlatformBaiduAI AuditPlatform = "baiduai" // 0x1002e969, 返回路径 result.auditResult.conclusion
)

// AuditAction 对应 sections.home.auditAction 的取值。
// 证据：fcn.1002d7f0 内的 ini 读取 "图像审核"/"处理" + 四次比较。
type AuditAction string

const (
	ActionProcess AuditAction = "处理"   // 0x102b8f9c
	ActionNoop    AuditAction = "不处理"  // 0x102b8fa4
	ActionBlur    AuditAction = "模糊处理" // 0x102b8fac
	ActionLink    AuditAction = "链接发送" // 0x102b8fdc
	ActionFilter  AuditAction = "过滤发送" // 0x102b8fb8
	ActionConfuse AuditAction = "图片混淆" // 0x102b8f90
)

// DefaultAuditThreshold 是"阈值"读不到时的字面量默认值 "0.5"（VA 0x102b8e60）。
const DefaultAuditThreshold = "0.5"

// AuditRequest 是 POST {nai3_plugin-api}/api/nsfw_check 的请求体。
// 字段证据：fcn.1002d7f0 0x1002dff3 写 "type"（VA 0x102b7654），
// 0x1002e092 写 "image"（VA 0x102b7f80）。
// api4ai 分支的 image 传的是"图片本地路径"（0x1002e031 调 fcn.1001d81c 取路径后再做文本型数值拼接），
// baidu / baiduai 分支传的是 "imgUrl"（VA 0x102b8e74，0x1002e535）。
type AuditRequest struct {
	Type  string `json:"type"`
	Image string `json:"image,omitempty"`
	Img   string `json:"imgUrl,omitempty"`
}

// AuditResult 返回审核结论。合规时 Verdict=="合规"（VA 0x102b8e90）。
type AuditResult struct {
	Verdict  string  // "合规" / "违规"
	NSFW     float64 // api4ai 的 nsfw 分数
	Raw      []byte
	Failed   bool   // 审核请求/解析失败 → 强制判违规
	FailText string // 审核失败，为了安全，强制判定为违规图（VA 0x102b8e38）
}

// 返回路径与结论常量
const (
	pathNSFWAPI4AI = "results[0].entities[0].classes.nsfw"
	pathBaidu      = "result.conclusion"
	pathBaiduAI    = "result.auditResult.conclusion"
	verdictOK      = "合规"
	verdictBad     = "违规"
	auditFailText  = "审核失败，为了安全，强制判定为违规图"
)

// buildAuditRequest 复刻三平台分支的请求体构造。
func buildAuditRequest(platform AuditPlatform, localPath, imgURL string) AuditRequest {
	switch platform {
	case PlatformBaidu, PlatformBaiduAI:
		return AuditRequest{Type: string(platform), Img: imgURL}
	default: // api4ai 及未知值都走 api4ai 分支
		return AuditRequest{Type: "api4ai", Image: localPath}
	}
}

// judgeNSFW 是阈值判定的纯函数版本，供测试与 CheckNSFW 共用。
func judgeNSFW(score, threshold float64) string {
	if score-threshold >= 0 {
		return verdictBad
	}
	return verdictOK
}

// CheckNSFW 复刻 fcn.1002d7f0 的鉴黄调用与阈值判定。
//
// 阈值逻辑（0x1002e4cb-0x1002e4ec）：
//
//	fld qword [nsfw]; fsub qword [threshold]; fcomp 0.0; test ah,1
//	→ nsfw - threshold >= 0 时判违规（即 >= 判违规，不是 >）
//
// 阈值默认 "0.5"（0x102b8e63/0x102b8e60），读不到时 fallback。
func (c *Client) CheckNSFW(ctx context.Context, platform AuditPlatform, localPath, imgURL, threshold string) *AuditResult {
	if threshold == "" {
		threshold = DefaultAuditThreshold
	}
	base := c.cfg.PluginAPI
	if base == "" {
		return &AuditResult{Verdict: verdictBad, Failed: true, FailText: auditFailText}
	}
	url := base + "/api/nsfw_check"
	raw, err := c.postJSON(ctx, url, c.cfg.Key, buildAuditRequest(platform, localPath, imgURL))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return &AuditResult{Verdict: verdictBad, Failed: true, FailText: auditFailText}
	}

	var score float64
	switch platform {
	case PlatformBaidu:
		var r struct {
			Result struct {
				Conclusion string `json:"conclusion"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &r) != nil {
			return &AuditResult{Verdict: verdictBad, Failed: true, FailText: auditFailText, Raw: raw}
		}
		res := &AuditResult{Raw: raw, Verdict: verdictOK}
		if strings.TrimSpace(r.Result.Conclusion) == "不合规" {
			res.Verdict = verdictBad
		}
		return res
	case PlatformBaiduAI:
		var r struct {
			Result struct {
				AuditResult struct {
					Conclusion string `json:"conclusion"`
				} `json:"auditResult"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &r) != nil {
			return &AuditResult{Verdict: verdictBad, Failed: true, FailText: auditFailText, Raw: raw}
		}
		res := &AuditResult{Raw: raw, Verdict: verdictOK}
		if strings.TrimSpace(r.Result.AuditResult.Conclusion) == "不合规" {
			res.Verdict = verdictBad
		}
		return res
	default:
		var r struct {
			Results []struct {
				Entities []struct {
					Classes struct {
						NSFW float64 `json:"nsfw"`
					} `json:"classes"`
				} `json:"entities"`
			} `json:"results"`
		}
		if json.Unmarshal(raw, &r) != nil || len(r.Results) == 0 || len(r.Results[0].Entities) == 0 {
			return &AuditResult{Verdict: verdictBad, Failed: true, FailText: auditFailText, Raw: raw}
		}
		score = r.Results[0].Entities[0].Classes.NSFW
	}

	th, err := strconv.ParseFloat(threshold, 64)
	if err != nil {
		return &AuditResult{Verdict: verdictBad, Failed: true, FailText: auditFailText, Raw: raw, NSFW: score}
	}
	return &AuditResult{Raw: raw, NSFW: score, Verdict: judgeNSFW(score, th)}
}

// ---------- 图库上传 ----------

// GalleryUpload 是 POST {资源地址}/plug/AI/v1/upload_image/upload.php 的请求体。
// 字段证据：fcn.1003b750 依次写 image_url(0x102b94c4) / user_qq(0x102b94d0) /
// bot_qq(0x102b94d8) / sampler(0x102b94e0) / model_name(0x102b94e8)。
// 注意：这里是 JSON（Content-Type:application/json，0x102b9518），不是 multipart。
type GalleryUpload struct {
	ImageURL  string `json:"image_url"`
	UserQQ    string `json:"user_qq"`
	BotQQ     string `json:"bot_qq"`
	Sampler   string `json:"sampler"`
	ModelName string `json:"model_name"`
}

// UploadToGallery 上传一张图到图库，返回原始响应（成功判定看 "success"，0x102b9538）。
func (c *Client) UploadToGallery(ctx context.Context, up GalleryUpload) ([]byte, error) {
	if c.cfg.ResourceURL == "" {
		return nil, fmt.Errorf("资源地址未配置")
	}
	url := c.cfg.ResourceURL + "/plug/AI/v1/upload_image/upload.php"
	return c.postJSON(ctx, url, c.cfg.Key, up)
}

// ---------- 超分 / 去背景 / 反推 ----------

// {nai3_plugin-api} 下的其余 AI 工具端点。
const (
	EPSuperRes     = "/api/superResolution_image" // 0x1039cb8c
	EPRemoveBG     = "/api/remove_background"     // 0x1039cba8
	EPTagger       = "/api/tagger"                // 0x102bbe34
	EPImageMeta    = "/api/image-metadata"        // 0x102bbcf8
	EPNSFWCheck    = "/api/nsfw_check"            // 0x102b8dfc
	EPTagTranslate = "/api/v1/tag-translate"      // 0x102b8134
	EPYoudaoTrans  = "/api/v1/youdao-translate"   // 0x102b8204
)

// 清晰术档位 → denoise 参数（0x102bbfe4 起的一串常量）。
var denoiseLevels = map[string]string{
	"低级":    "denoise1x",
	"中级":    "denoise2x",
	"高级":    "denoise3x",
	"二重奏":   "2重奏",
	"三重奏":   "3重奏",
	"四重奏":   "4重奏",
	"中级二重奏": "denoise2x",
}

// SuperResolution 调用清晰术。imgURL 走 "imgUrl"，档位决定 denoise。
func (c *Client) SuperResolution(ctx context.Context, imgURL, level string) ([]byte, error) {
	body := map[string]interface{}{"imgUrl": imgURL}
	if d, ok := denoiseLevels[strings.TrimSpace(level)]; ok {
		body["denoise"] = d
	}
	return c.postPlugin(ctx, EPSuperRes, body)
}

// RemoveBackground 调用去背景。
func (c *Client) RemoveBackground(ctx context.Context, imgURL string) ([]byte, error) {
	return c.postPlugin(ctx, EPRemoveBG, map[string]interface{}{"imgUrl": imgURL})
}

// TaggerResult 是 /api/tagger 的返回结构。
type TaggerResult struct {
	Tags struct {
		Character []string `json:"character"`
		Copyright []string `json:"copyright"`
	} `json:"tags"`
	Raw []byte `json:"-"`
}

// Interrogate 图片反推（识别角色/作品来源）。
func (c *Client) Interrogate(ctx context.Context, imgURL string) (*TaggerResult, error) {
	raw, err := c.postPlugin(ctx, EPTagger, map[string]interface{}{"imgUrl": imgURL})
	if err != nil {
		return nil, fmt.Errorf("图片鉴赏失败")
	}
	var r TaggerResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("图片鉴赏失败")
	}
	r.Raw = raw
	return &r, nil
}

// ImageMetadata 读取图片元数据（返回任意 JSON）。
func (c *Client) ImageMetadata(ctx context.Context, imgURL string) (map[string]interface{}, error) {
	raw, err := c.postPlugin(ctx, EPImageMeta, map[string]interface{}{"imgUrl": imgURL})
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// postPlugin 向 {nai3_plugin-api} 发一个 POST。
func (c *Client) postPlugin(ctx context.Context, path string, body interface{}) ([]byte, error) {
	if c.cfg.PluginAPI == "" {
		return nil, fmt.Errorf("nai3_plugin-api 未配置")
	}
	return c.postJSON(ctx, c.cfg.PluginAPI+path, c.cfg.Key, body)
}

// doRaw 用于需要自定义头（不一定是 Bearer）的调用。
func (c *Client) doRaw(ctx context.Context, method, url string, headers map[string]string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrLinkFailed
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
