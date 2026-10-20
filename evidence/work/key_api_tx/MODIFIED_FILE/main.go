package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"xianyu-nai/nai"
)

func main() {
	var (
		mode      = flag.String("mode", "", "官方|反代，留空=反代")
		modeLite  = flag.Bool("official", false, "等价于 -mode 官方")
		proxy     = flag.String("proxy", "", "第三方反代地址 nai3api")
		official  = flag.String("official-url", "", "官方地址 naiofficialapi，如 https://image.novelai.net")
		pluginAPI = flag.String("plugin-api", "", "nai3_plugin-api（鉴黄/超分/去背景/反推的 base）")
		resURL    = flag.String("resource-url", "", "资源地址（图库上传/卡片模板的 base）")
		key       = flag.String("key", "", "nai3key（反代 & 插件接口）")
		okey      = flag.String("official-key", "", "naiOfficialKey（官方）")
		prompt    = flag.String("prompt", "", "正向提示词")
		negative  = flag.String("negative", "", "反向提示词，留空用插件默认负面词")
		model     = flag.String("model", "nai-diffusion-3", "模型 title")
		w         = flag.Int("width", 1024, "宽")
		h         = flag.Int("height", 1024, "高")
		steps     = flag.Int("steps", 28, "步数")
		scale     = flag.Float64("scale", 5, "CFG scale")
		sampler   = flag.String("sampler", "k_euler", "采样器")
		noise     = flag.String("noise", "native", "噪声调度")
		seed      = flag.Int64("seed", -1, "种子，-1=随机")
		strength  = flag.Float64("strength", 0.52, "img2img 重绘幅度")
		imgPath   = flag.String("image", "", "img2img 底图路径")
		augment   = flag.String("augment", "", "导演工具：emotion|declutter|colorize|sketch|lineart")
		augImg    = flag.String("augment-image", "", "导演工具原图路径")
		dump      = flag.Bool("dump", false, "只打印请求体，不发送")
		sub       = flag.Bool("subscription", false, "查询额度")
		listMdl   = flag.Bool("models", false, "列出模型")
		nsfw      = flag.Bool("nsfw", false, "鉴黄：用 -image 指定的本地图")
		platform  = flag.String("audit-platform", "api4ai", "审核平台：api4ai|baidu|baiduai")
		threshold = flag.String("audit-threshold", nai.DefaultAuditThreshold, "审核阈值，默认 0.5，>= 判违规")
		imgURL    = flag.String("img-url", "", "远程图片 URL（超分/去背景/反推/图库上限用）")
		superres  = flag.String("superres", "", "清晰术档位：低级|中级|高级|二重奏|三重奏|四重奏")
		rembg     = flag.Bool("remove-bg", false, "去背景")
		tagger    = flag.Bool("tagger", false, "图片反推（识别角色/作品）")
		gallery   = flag.Bool("gallery", false, "上传图库（需 -img-url/-user-qq/-bot-qq）")
		userQQ    = flag.String("user-qq", "", "图库上传：用户 QQ")
		botQQ     = flag.String("bot-qq", "", "图库上传：机器人 QQ")
		publish   = flag.String("publish", "", "预览发送模式分支：json|链接发送|卡片跳转|合并转发")
		confuse   = flag.String("confuse", "", "图片混淆：encrypt|decrypt，需 -image")
		confOut   = flag.String("confuse-out", "confused.png", "混淆输出路径")
		parseOV   = flag.Bool("parse", false, "只解析提示词里的 # 参数，不发送")
		cmdText   = flag.String("cmd", "", "按插件指令表解析一条消息，如 \"nai3指令 1girl\"")
		timeout   = flag.Int("timeout", 300000, "超时毫秒")
		onebot    = flag.Bool("onebot", false, "以 QQ 机器人方式运行（OneBot v11 适配层）")
		obAPI     = flag.String("ob-api", "http://127.0.0.1:5700", "OneBot v11 HTTP API 基址")
		obToken   = flag.String("ob-token", "", "OneBot access_token")
		obListen  = flag.String("ob-listen", ":5701", "OneBot 事件上报监听地址")
		obPath    = flag.String("ob-path", "/onebot", "OneBot 事件上报路径")
		obWS      = flag.Bool("ob-ws", false, "在 -ob-listen 上同时提供正向 WebSocket（框架连进来）")
		obWSRev   = flag.String("ob-ws-reverse", "", "反向 WebSocket 地址（插件连框架，如 ws://127.0.0.1:3001/onebot/v11/ws）")
		dataDir   = flag.String("data", ".", "数据目录（预设/标签 JSON）")
		sendMode  = flag.String("send-mode", "", "发送模式：json|链接发送|卡片跳转|合并转发|图片（留空用配置默认）")
		uiOn      = flag.Bool("ui", false, "启动浏览器配置面板（仅监听本机）")
		uiAddr    = flag.String("ui-addr", "127.0.0.1:0", "配置面板监听地址；0 表示随机端口")

		sdURL    = flag.String("sd-url", "", "SD 接口地址（sd设置/链接）")
		sdUser   = flag.String("sd-user", "", "SD 账号（sd设置/账号）")
		sdPass   = flag.String("sd-pass", "", "SD 密码（sd设置/密码）")
		sdModel  = flag.String("sd-model", "", "SD 全局默认模型（sd参数/全局默认模型）")
		sdSeed   = flag.String("sd-seed", "", "SD 默认种子（sd参数/默认种子）")
		sdHires  = flag.Bool("sd-hires", false, "开启高清修复（sd参数/高清修复=1）")
		sdDump   = flag.Bool("sd-dump", false, "只打印 SD 请求体，不发送")
		sdRun    = flag.Bool("sd-draw", false, "走 SD 通道出图（txt2img/img2img）")
		sdImg2   = flag.String("sd-init", "", "SD img2img 底图路径")
		sdModels = flag.Bool("sd-models", false, "列出 SD 模型（/sdapi/v1/sd-models）")
		sdVAEs   = flag.Bool("sd-vaes", false, "列出 SD VAE（/sdapi/v1/sd-vae）")
		sdLoras  = flag.Bool("sd-loras", false, "列出 SD LoRA（/sdapi/v1/loras）")
		sdINI    = flag.String("ini", "", "咸鱼ai画图.ini 路径（读 [sd设置]/[sd参数]）")

		comfyURL   = flag.String("comfy-url", "", "ComfyUI 链接（comfy设置/链接）")
		comfyToken = flag.String("comfy-token", "", "ComfyUI Token（comfy设置/Token）")
		comfyModel = flag.String("comfy-model", "", "ComfyUI 全局默认工作流名（comfy参数/全局默认模型）")
		comfySeed  = flag.String("comfy-seed", "", "ComfyUI 默认种子（comfy参数/默认种子）")
		comfySteps = flag.Int("comfy-steps", 0, "ComfyUI 默认步数（comfy参数/steps）")
		comfyCfg   = flag.Float64("comfy-cfg", 0, "ComfyUI 默认 CFG（comfy参数/cfg）")
		comfyW     = flag.Int("comfy-width", 0, "ComfyUI 默认宽度（comfy参数/宽度）")
		comfyH     = flag.Int("comfy-height", 0, "ComfyUI 默认高度（comfy参数/高度）")
		comfyINI   = flag.String("comfy-ini", "", "咸鱼ai画图.ini 路径（读 [comfy设置]/[comfy参数]/[comfy指令]）")
		comfyList  = flag.Bool("comfy-models", false, "列出 ComfyUI 工作流绑定")
		comfyDump  = flag.Bool("comfy-dump", false, "只打印 comfy 提交体，不发送")
		comfyRun   = flag.Bool("comfy-draw", false, "走 ComfyUI 通道出图")
		comfyPrm   = flag.String("comfy-prompt", "", "ComfyUI 提示词")
	)
	flag.Parse()

	if *listMdl {
		fmt.Print(nai.ModelsMarkdown())
		return
	}
	if *publish != "" {
		fmt.Printf("发送模式=%s -> %s\n", *publish, nai.PublishMode(*publish))
		return
	}
	if *cmdText != "" {
		dumpCommand(*cmdText)
		return
	}
	if *parseOV {
		dumpOverrides(*prompt)
		return
	}
	if *confuse != "" {
		runConfuse(*imgPath, *confOut, *confuse == "encrypt")
		return
	}

	cfg := nai.DefaultConfig()
	cfg.ProxyURL = *proxy
	cfg.OfficialURL = *official
	cfg.PluginAPI = *pluginAPI
	cfg.ResourceURL = *resURL
	cfg.Key = *key
	cfg.OfficialKey = *okey
	cfg.Model = *model
	cfg.Width = *w
	cfg.Height = *h
	cfg.Steps = *steps
	cfg.Scale = *scale
	cfg.Sampler = *sampler
	cfg.Noise = *noise
	cfg.Seed = *seed
	cfg.Strength = *strength
	cfg.TimeoutMS = *timeout
	if *modeLite {
		cfg.Mode = "官方"
	} else {
		cfg.Mode = *mode
	}
	if *negative != "" {
		cfg.Negative = *negative
	}
	if *sendMode != "" {
		cfg.SendMode = *sendMode
	}
	if *resURL != "" {
		cfg.ResourceURL = *resURL
	}
	cfg.DataDir = *dataDir

	// SD 通道参数（命令行优先，其次 咸鱼ai画图.ini 的 [sd设置]/[sd参数]）
	if *sdINI != "" {
		cfg.SD = nai.LoadSDConfigFromINI(*sdINI, cfg.SD)
	}
	if *sdURL != "" {
		cfg.SD.URL = *sdURL
	}
	if *sdUser != "" {
		cfg.SD.User = *sdUser
	}
	if *sdPass != "" {
		cfg.SD.Pass = *sdPass
	}
	if *sdModel != "" {
		cfg.SD.GlobalModel = *sdModel
	}
	if *sdSeed != "" {
		cfg.SD.Seed = *sdSeed
	}
	if *sdHires {
		cfg.SD.Hires = true
	}
	cfg.SD = nai.SDWithDefaults(cfg.SD)

	// ComfyUI 通道参数（命令行优先，其次 咸鱼ai画图.ini 的 [comfy设置]/[comfy参数]/[comfy指令]）
	if *comfyINI != "" {
		cfg.Comfy = nai.LoadComfyConfigFromINI(*comfyINI, cfg.Comfy)
	}
	if *comfyURL != "" {
		cfg.Comfy.URL = *comfyURL
	}
	if *comfyToken != "" {
		cfg.Comfy.Token = *comfyToken
	}
	if *comfyModel != "" {
		cfg.Comfy.GlobalModel = *comfyModel
	}
	if *comfySeed != "" {
		cfg.Comfy.Seed = *comfySeed
	}
	if *comfySteps > 0 {
		cfg.Comfy.Steps = *comfySteps
	}
	if *comfyCfg > 0 {
		cfg.Comfy.Cfg = *comfyCfg
	}
	if *comfyW > 0 {
		cfg.Comfy.Width = *comfyW
	}
	if *comfyH > 0 {
		cfg.Comfy.Height = *comfyH
	}
	cfg.Comfy = nai.ComfyWithDefaults(cfg.Comfy)

	if *onebot {
		runOneBot(cfg, *obAPI, *obToken, *obListen, *obPath, *obWS, *obWSRev, *dataDir, *uiOn, *uiAddr)
		return
	}

	// 只开配置面板、不接 QQ：用于离线改 ini
	if *uiOn {
		runUIOnly(cfg, *dataDir, *uiAddr)
		return
	}

	c := nai.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutMS)*time.Millisecond)
	defer cancel()

	// SD 通道的独立子命令
	if *sdModels || *sdVAEs || *sdLoras {
		if !c.SDAvailable() {
			fatal(fmt.Errorf("sd接口未配置，请配置后再来绘图"))
		}
		switch {
		case *sdModels:
			items, err := c.SDModels(ctx)
			if err != nil {
				fatal(err)
			}
			fmt.Print(nai.RenderSDModels(items))
		case *sdVAEs:
			items, err := c.SDVAEs(ctx)
			if err != nil {
				fatal(err)
			}
			fmt.Print(nai.RenderSDVAEs(items))
		case *sdLoras:
			items, err := c.SDLoRAs(ctx)
			if err != nil {
				fatal(err)
			}
			fmt.Print(nai.RenderSDLoRAs(items))
		}
		return
	}
	if *sdDump || *sdRun {
		opts := nai.SDOptions{
			Prompt:   *prompt,
			Negative: cfg.SD.Negative,
			Seed:     cfg.SD.Seed,
			Steps:    cfg.SD.Steps,
			CFG:      cfg.SD.Identity,
			Width:    cfg.SD.Width,
			Height:   cfg.SD.Height,
			Sampler:  cfg.SD.Sampler,
		}
		if *sdImg2 != "" {
			b, err := os.ReadFile(*sdImg2)
			if err != nil {
				fatal(err)
			}
			opts.InitImage = base64.StdEncoding.EncodeToString(b)
		}
		var body string
		var err error
		if opts.InitImage != "" {
			body, err = nai.BuildSDImg2Img(cfg.SD, opts)
		} else {
			body, err = nai.BuildSDTxt2Img(cfg.SD, opts)
		}
		if err != nil {
			fatal(err)
		}
		if *sdDump {
			fmt.Println(body)
			return
		}
		res, err := c.SDGenerate(ctx, opts)
		if err != nil {
			fatal(err)
		}
		printSDResult(res)
		return
	}

	// ComfyUI 通道的独立子命令
	if *comfyList || *comfyDump || *comfyRun {
		if !cfg.Comfy.ComfyAvailable() {
			fatal(fmt.Errorf("ComfyUI链接未配置，请配置后再来绘图"))
		}
		ws := nai.LoadComfyWorkflows(cfg.DataDir)
		if *comfyList {
			fmt.Print(nai.RenderComfyModels(ws, cfg.Comfy.CmdDraw))
			return
		}
		prm := *comfyPrm
		if prm == "" {
			prm = *prompt
		}
		if *comfyDump {
			if len(ws) == 0 {
				fatal(fmt.Errorf("未配置 ComfyUI 工作流绑定，请管理员到设置界面添加"))
			}
			vals := nai.ComfyDefaultValues(cfg.Comfy, prm)
			content, missing := nai.SubstituteComfyPlaceholders(ws[0].Content, vals)
			if len(missing) > 0 {
				fatal(fmt.Errorf("%s", nai.RenderComfyMissingParams(missing)))
			}
			fmt.Println(nai.BuildComfyPromptBody(content))
			return
		}
		res, err := p2DrawComfy(cfg, prm)
		if err != nil {
			fatal(err)
		}
		if len(res.Images) == 0 {
			fatal(fmt.Errorf("comfy未返回图片"))
		}
		_ = os.WriteFile("comfy-output.png", res.Images[0], 0o644)
		fmt.Println(res.Caption)
		fmt.Println("已写出 comfy-output.png")
		return
	}

	switch {
	case *sub:
		s, err := c.QuerySubscription(ctx)
		if err != nil {
			fatal(err)
		}
		fmt.Print(s.Render())
		return

	case *nsfw:
		if *imgPath == "" {
			fatal(fmt.Errorf("鉴黄需要 -image <本地图片>"))
		}
		abs, err := absPath(*imgPath)
		if err != nil {
			fatal(err)
		}
		r := c.CheckNSFW(ctx, nai.AuditPlatform(*platform), abs, *imgURL, *threshold)
		if r.Failed {
			fmt.Printf("结论=%s（%s）\n", r.Verdict, r.FailText)
		} else {
			fmt.Printf("结论=%s nsfw=%.4f 阈值=%s\n", r.Verdict, r.NSFW, *threshold)
		}
		return

	case *superres != "" || *rembg || *tagger || *gallery:
		if *imgURL == "" {
			fatal(fmt.Errorf("该功能需要 -img-url <图片URL>"))
		}
		switch {
		case *superres != "":
			raw, err := c.SuperResolution(ctx, *imgURL, *superres)
			if err != nil {
				fatal(err)
			}
			printRaw(raw)
		case *rembg:
			raw, err := c.RemoveBackground(ctx, *imgURL)
			if err != nil {
				fatal(err)
			}
			printRaw(raw)
		case *tagger:
			r, err := c.Interrogate(ctx, *imgURL)
			if err != nil {
				fatal(err)
			}
			fmt.Println("识别角色:", r.Tags.Character)
			fmt.Println("作品来源:", r.Tags.Copyright)
		case *gallery:
			raw, err := c.UploadToGallery(ctx, nai.GalleryUpload{
				ImageURL:  *imgURL,
				UserQQ:    *userQQ,
				BotQQ:     *botQQ,
				Sampler:   cfg.Sampler,
				ModelName: cfg.Model,
			})
			if err != nil {
				fatal(err)
			}
			printRaw(raw)
		}
		return
	}

	if *augment != "" {
		b64, err := readImage(*augImg)
		if err != nil {
			fatal(err)
		}
		req := c.BuildAugmentRequest(nai.AugmentType(*augment), b64, *prompt)
		if *dump {
			printJSON(req)
			return
		}
		res, err := c.Augment(ctx, nai.AugmentType(*augment), b64, *prompt)
		if err != nil {
			fatal(err)
		}
		printResult(res)
		return
	}

	b64, err := readImage(*imgPath)
	if err != nil {
		fatal(err)
	}
	neg := cfg.Negative
	if c.IsOfficial() {
		req := c.BuildOfficialRequest(*prompt, neg, b64)
		if *dump {
			printJSON(req)
			return
		}
	} else if *dump {
		printJSON(c.BuildProxyRequest(*prompt, neg))
		return
	}

	res, err := c.Generate(ctx, *prompt, neg, b64)
	if err != nil {
		fatal(err)
	}
	printResult(res)
}

func absPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	a, err := os.Getwd()
	if err != nil {
		return p, nil
	}
	if len(p) > 1 && (p[1] == ':' || p[0] == '/' || p[0] == '\\') {
		return p, nil
	}
	return a + string(os.PathSeparator) + p, nil
}

func readImage(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal(err)
	}
}

func printRaw(raw []byte) {
	var v interface{}
	if json.Unmarshal(raw, &v) == nil {
		printJSON(v)
		return
	}
	fmt.Println(string(raw))
}

func printResult(r *nai.Result) {
	fmt.Printf("seed=%d images=%d\n", r.Seed, len(r.Images))
	if r.ImageURL != "" {
		fmt.Println("image_url:", r.ImageURL)
	}
	if len(r.Images) > 0 {
		raw, err := base64.StdEncoding.DecodeString(r.Images[0])
		if err == nil && len(raw) > 0 {
			_ = os.WriteFile("output.png", raw, 0o644)
			fmt.Println("已写入 output.png")
		}
	}
}

func printSDResult(r *nai.SDResult) {
	fmt.Printf("seed=%d images=%d urls=%d\n", r.Seed, len(r.Images), len(r.URLs))
	if r.Model != "" {
		fmt.Println("model:", r.Model)
	}
	if r.Sampler != "" {
		fmt.Println("sampler:", r.Sampler)
	}
	if r.ImageURL != "" {
		fmt.Println("image_url:", r.ImageURL)
	}
	if len(r.Images) > 0 {
		raw, err := nai.DecodeOfficialImage(r.Images[0])
		if err == nil && len(raw) > 0 {
			_ = os.WriteFile("sd-output.png", raw, 0o644)
			fmt.Println("已写出 sd-output.png")
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}

// dumpCommand 演示指令表解析结果。
func dumpCommand(msg string) {
	c, matched, ok := nai.MatchCommand(msg)
	if !ok {
		fmt.Println("未命中任何指令")
		return
	}
	fmt.Printf("指令=%s  匹配词=%s  配置键=%s\n", c.Kind, matched, c.Config)
	fmt.Printf("参数=%q\n", nai.StripCommand(msg, matched))
}

// dumpOverrides 演示 # 参数与预设解析。
func dumpOverrides(prompt string) {
	ov, rest := nai.ParseOverrides(prompt)
	fmt.Printf("清理后提示词=%q\n", rest)
	if ov.Steps != nil {
		fmt.Printf("#步骤 -> %d\n", *ov.Steps)
	}
	if ov.Sampler != nil {
		fmt.Printf("#采样 -> %q\n", *ov.Sampler)
	}
	if ov.Width != nil && ov.Height != nil {
		fmt.Printf("#尺寸 -> %dx%d\n", *ov.Width, *ov.Height)
	}
	if ov.Guidance != nil {
		fmt.Printf("#指导 -> %g\n", *ov.Guidance)
	}
	if ov.Resolution != nil {
		fmt.Printf("分辨率预设 %s -> %dx%d\n", ov.Resolution.Name, ov.Resolution.Width, ov.Resolution.Height)
	}
}

// runOneBot 以 QQ 机器人方式运行：OneBot v11 事件上报 + HTTP API 回发。
func runOneBot(cfg nai.Config, apiBase, token, listen, path string, withWS bool, wsReverse, dataDir string, uiOn bool, uiAddr string) {
	store, err := nai.LoadStore(
		filepath.Join(dataDir, nai.FileNai3PresetJSON),
		filepath.Join(dataDir, nai.FileNai3TagJSON),
	)
	if err != nil {
		fatal(err)
	}
	host := nai.OneBotHost{C: &nai.HTTPCaller{Base: apiBase, Token: token}}
	p := nai.NewPlugin(host, cfg)
	p.Init(cfg, store)
	p.Enable()

	if cfg.SendMode == string(nai.SendCard) || cfg.SendMode == string(nai.SendForward) {
		tmpl, err := nai.FetchCardTemplate(context.Background(), cfg.ResourceURL, nil)
		if err != nil {
			fmt.Printf("卡片模板未装载（%v），该模式将回退为直接发图\n", err)
		} else {
			p.SetCardTemplate(tmpl)
			fmt.Printf("卡片模板已装载：%d 字节\n", len(tmpl))
		}
	}

	// SD 通道的卡片模板是独立文件（sdjson.json，0x102b8b2c）。
	if cfg.SD.SDBaseURL() != "" {
		tmpl, err := nai.FetchSdCardTemplate(context.Background(), cfg.ResourceURL, nil)
		if err != nil {
			fmt.Printf("SD 卡片模板未装载（%v），卡片模式将回退为直接发图\n", err)
		} else {
			p.SetSdCardTemplate(tmpl)
			fmt.Printf("SD 卡片模板已装载：%d 字节\n", len(tmpl))
		}
	}

	// ComfyUI 通道的卡片模板（aijson.json，0x102b9644）
	if cfg.Comfy.ComfyAvailable() {
		tmpl, err := nai.FetchComfyCardTemplate(context.Background(), cfg.ResourceURL, nil)
		if err != nil {
			fmt.Printf("ComfyUI 卡片模板未装载（%v），卡片模式将回退为直接发图\n", err)
		} else {
			p.SetComfyCardTemplate(tmpl)
			fmt.Printf("ComfyUI 卡片模板已装载：%d 字节\n", len(tmpl))
		}
	}

	mux := http.NewServeMux()
	mux.Handle(path, p.ReportHandler())
	if withWS {
		// 正向 WebSocket：框架作为客户端连到同一个路径即可（与 HTTP 上报共存）
		mux.Handle(path+"/ws", p.WSHandler())
		mux.Handle(path+"/ws/", p.WSHandler())
	}
	fmt.Printf("OneBot 适配层已启动\n  上报地址: http://0.0.0.0%s%s\n  回调 API: %s\n", listen, path, apiBase)
	if withWS {
		fmt.Printf("  正向 WebSocket: ws://0.0.0.0%s%s/ws\n", listen, path)
	}

	if wsReverse != "" {
		go runWSReverse(p, wsReverse)
	}

	// 配置面板：与原插件的 WebView2 设置页等价，只是换成浏览器打开
	if uiOn {
		srv := nai.NewUIServer(nai.UIOptions{Addr: uiAddr, DataDir: dataDir}, p)
		url, err := srv.Start()
		if err != nil {
			fmt.Printf("配置面板启动失败：%v\n", err)
		} else {
			fmt.Printf("  配置面板: %s\n", url)
		}
	}

	if err := http.ListenAndServe(listen, mux); err != nil {
		fatal(err)
	}
}

// runUIOnly 只启动配置面板（不接 QQ），用于离线改 ini 后退出。
func runUIOnly(cfg nai.Config, dataDir, uiAddr string) {
	store, err := nai.LoadStore(
		filepath.Join(dataDir, nai.FileNai3PresetJSON),
		filepath.Join(dataDir, nai.FileNai3TagJSON),
	)
	if err != nil {
		fatal(err)
	}
	host := nai.OneBotHost{C: &nai.HTTPCaller{Base: "http://127.0.0.1:5700"}}
	p := nai.NewPlugin(host, cfg)
	p.Init(cfg, store)
	p.Enable()

	srv := nai.NewUIServer(nai.UIOptions{Addr: uiAddr, DataDir: dataDir}, p)
	url, err := srv.Start()
	if err != nil {
		fatal(err)
	}
	fmt.Printf("配置面板已启动：%s\n", url)
	fmt.Printf("数据目录：%s\n", dataDir)
	fmt.Println("按 Ctrl+C 退出")
	select {}
}

// runWSReverse 连框架的反向 WebSocket 地址，断线自动重连。
func runWSReverse(p *nai.Plugin, raw string) {
	for {
		ws, err := nai.DialWS(raw)
		if err != nil {
			fmt.Printf("反向 WebSocket 连接失败（%v），5 秒后重试\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		fmt.Printf("反向 WebSocket 已连上：%s\n", raw)
		p.ServeWS(ws)
		fmt.Printf("反向 WebSocket 已断开，5 秒后重连\n")
		time.Sleep(5 * time.Second)
	}
}

// p2DrawComfy 用最小插件实例跑一次 ComfyUI 出图（CLI 子命令用）。
func p2DrawComfy(cfg nai.Config, prompt string) (*nai.ComfyDrawResult, error) {
	p := nai.NewPlugin(nil, cfg)
	p.Init(cfg, nil)
	return p.DrawComfy(context.Background(), nai.ComfyDrawOptions{Prompt: prompt})
}

// runConfuse 执行图片混淆/解混淆并落盘。
func runConfuse(in, out string, encrypt bool) {
	if in == "" {
		fatal(fmt.Errorf("混淆需要 -image <本地图片>"))
	}
	b, err := os.ReadFile(in)
	if err != nil {
		fatal(err)
	}
	img, err := nai.DecodeImage(b)
	if err != nil {
		fatal(err)
	}
	rgba := nai.ToRGBA(img)
	bd := img.Bounds()
	w, h := bd.Dx(), bd.Dy()
	res, err := nai.Confuse(rgba, w, h, encrypt)
	if err != nil {
		fatal(err)
	}
	imgOut := nai.RGBABytesToImage(res, w, h)
	f, err := os.Create(out)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := nai.EncodePNG(f, imgOut); err != nil {
		fatal(err)
	}
	progress, done := nai.ConfuseDone(encrypt)
	fmt.Printf("%s -> %s\n%s\n", progress, out, done)
}
