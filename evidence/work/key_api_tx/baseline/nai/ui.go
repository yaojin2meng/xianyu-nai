package nai

// 本文件实现“浏览器配置面板”，对应原插件的设置界面。
//
// 原插件 UI 的技术形态（反汇编实测）：
//
//	0x10090E20  Setting()：释放 edgeview.dll（0x102BC8C0）、WebView2 安装器
//	            （0x102BC8D0 = webview2runtime_win7.exe），创建 WebView2 环境。
//	0x1009128C  拼 ui\index.html（0x10399FFC），读 咸鱼ai画图.ini 的
//	            [浏览器UI] ui_url（0x1039A00C）与 版本号（0x1039A014），
//	            版本不一致则从授权服务器下载 UI 包。
//	0x100092B0  另有一套菜单页 ui\menu.html（0x102B6698）：
//	            读 ui\menu.html，用 JSON 生成 menuConfig（0x102B66A8），
//	            再写到 图片缓存\菜单.png（0x102B66B4）作为图片菜单。
//	0x1039A1B0  http://xianyu.ui/index.html?r=      虚拟主机地址
//	0x10399F00  “edgeview.dll 释放失败，请手动从群文件下载复制到框架根目录”
//	0x10399FAC  “未检测到 WebView2 Runtime，安装器已下载到运行目录，请手动安装后重新打开设置”
//
// 也就是说：原插件的界面是**网页**，由 WebView2 承载，最终配置仍落在
// 咸鱼ai画图.ini。它的菜单页由 menuConfig 驱动，结构是
// groups[1..4] 每组若干条 {key,name,enabled,notice}（0x102B6030..0x102B65DC）。
//
// 重制版不依赖 WebView2、不依赖 edgeview.dll、不需要联网下载 UI，
// 直接由进程内置一个仅监听本机的 HTTP 服务托管同一套配置面板：
//
//	GET  /             面板页面（按 schema 动态渲染）
//	GET  /api/schema   面板结构（节 / 键 / 类型 / 默认值）
//	GET  /api/config   读出当前 ini 全部节键
//	POST /api/config   写入 ini 并热重载
//	GET  /api/status   运行状态
//	GET  /api/presets  预设与工作流清单
//
// 面板读写的节名/键名全部来自对 DLL 的实测（见 UISchema 的注释与 RVA），
// 因此手工编辑 ini 与面板编辑完全等价。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UIOptions 是面板服务参数。
type UIOptions struct {
	// Addr 监听地址，空则 127.0.0.1:0（随机端口）。
	Addr string
	// DataDir 数据目录，决定 ini 与工作流目录位置。
	DataDir string
}

// UIServer 是配置面板服务。
type UIServer struct {
	opt  UIOptions
	ln   net.Listener
	srv  *http.Server
	plug *Plugin
}

// NewUIServer 创建面板服务（尚未开始监听）。plug 可为 nil，此时只读写 ini。
func NewUIServer(opt UIOptions, plug *Plugin) *UIServer {
	return &UIServer{opt: opt, plug: plug}
}

// Start 启动监听并返回可打开的地址。
func (s *UIServer) Start() (string, error) {
	addr := s.opt.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("配置面板监听失败：%w", err)
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/schema", s.handleSchema)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/presets", s.handlePresets)
	s.srv = &http.Server{Handler: mux}
	go func() { _ = s.srv.Serve(ln) }()
	return s.UIURL(), nil
}

// Stop 关闭监听。
func (s *UIServer) Stop() error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

// UIAddr 返回实际监听地址（含随机端口）。
func (s *UIServer) UIAddr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// UIURL 返回可直接在浏览器打开的地址。
func (s *UIServer) UIURL() string {
	a := s.UIAddr()
	if a == "" {
		return ""
	}
	return "http://" + a + "/"
}

// MainINIPath 返回主配置路径。
func MainINIPath(dir string) string {
	if dir == "" {
		return FileMainINI
	}
	return filepath.Join(dir, FileMainINI)
}

// ---------------------------------------------------------------------------
// 面板结构（schema）
// ---------------------------------------------------------------------------

// UIField 是面板上的一个配置项，Key 与 ini 的键名逐字一致。
type UIField struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Type  string   `json:"type"` // text | number | bool | select | password | textarea
	Def   string   `json:"def"`
	Opts  []string `json:"opts,omitempty"`
	Hint  string   `json:"hint,omitempty"`
}

// UISection 是面板上的一个分组，Name 与 ini 的节名逐字一致。
type UISection struct {
	Name   string    `json:"name"`
	Desc   string    `json:"desc"`
	RVA    string    `json:"rva,omitempty"` // 该节名在 DLL 中的常量地址
	Ext    bool      `json:"ext"`           // true = 重制版新增节，原插件没有
	Fields []UIField `json:"fields"`
}

// UISchema 返回面板结构。
//
// 全部节名/键名来自对 unpacked.dll 的“读配置项”调用点实测：
//
//	基本          0x10093xxx / 0x100A0Bxx
//	类型          0x10020DD9 翻译接口 / 0x100A29C2 sd接口
//	指令          0x100976F2 起
//	点数限制      0x10010529 起
//	点数消耗      0x10097FE5 / 0x100980A4 / 0x10098163
//	图像审核      0x10025978 起
//	黑名单功能    0x100A2490 起
//	nai3设置      0x100A3EE1 key / 0x100A3F96 官方key / 0x100A4051 模式
//	nai3参数      0x100A410C scale … 0x100A4B46 完成冷却模式
//	nai3指令      0x100A4C01 画图 … 0x100A586C 删除预设
//	sd设置        0x1009AC74 账号 / 0x1009AD03 密码
//	sd参数        0x1009A6E9 steps … 0x1009AEE0 完成冷却模式
//	sd指令        0x10099EBB 画图 … 0x1009A649 删除预设
//	comfy设置     0x1009B028 链接 / 0x1009B0B7 Token
//	comfy参数     0x1009B472 steps … 0x1009BAA8 完成冷却模式
//	comfy指令     0x1009B155 画图 … 0x1009B3D5 删除预设
//	浏览器UI      0x100913D8 版本号
//
// [重制版] 是新增节，用来持久化原插件由授权服务器下发、或由宿主框架
// 提供的字段（反代地址、插件接口、资源地址、OneBot 连接等）。原插件没有
// 这一节，因此标 Ext=true，在面板上单独提示。
func UISchema() []UISection {
	num := func(k, label, def string) UIField {
		return UIField{Key: k, Label: label, Type: "number", Def: def}
	}
	txt := func(k, label, def string) UIField {
		return UIField{Key: k, Label: label, Type: "text", Def: def}
	}
	sec := func(k, label, def string) UIField {
		return UIField{Key: k, Label: label, Type: "password", Def: def}
	}
	ta := func(k, label, def string) UIField {
		return UIField{Key: k, Label: label, Type: "textarea", Def: def}
	}
	onoff := func(k, label, def string) UIField {
		return UIField{Key: k, Label: label, Type: "bool", Def: def}
	}
	sel := func(k, label, def string, opts ...string) UIField {
		return UIField{Key: k, Label: label, Type: "select", Def: def, Opts: opts}
	}

	return []UISection{
		{
			Name: "基本", Desc: "宿主层通用设置（发送方式、翻译、屏蔽词）",
			Fields: []UIField{
				txt("主人", "主人 QQ", ""),
				txt("机器人QQ", "机器人 QQ", ""),
				sel("发送模式", "发送模式", "", "图片", "json", "链接发送", "卡片跳转", "合并转发"),
				onoff("是否引用", "回复时引用原消息", "0"),
				onoff("官机互动", "官方机器人互动", "0"),
				onoff("上传图像", "上传生成的图到图床", "0"),
				txt("AI翻译令牌", "AI 翻译令牌", ""),
				ta("屏蔽词条", "屏蔽词条（一行一个）", ""),
				sec("psw", "面板/接口密码", ""),
			},
		},
		{
			Name: "类型", Desc: "各功能使用的接口类型",
			Fields: []UIField{
				sel("翻译接口", "翻译接口", "", "", "tag", "youdao", "ai"),
				sel("今日少女接口", "今日少女接口", "", "", "1", "2"),
				sel("sd接口", "SD 接口类型", "", "", "sd", "sdcn"),
			},
		},
		{
			Name: "指令", Desc: "通用功能指令词（对应 menuConfig 的 groups[1]/groups[4]）",
			Fields: []UIField{
				txt("菜单", "菜单", "菜单"),
				txt("开启群", "开启群", ""),
				txt("关闭群", "关闭群", ""),
				txt("图片鉴赏", "图片鉴赏", ""),
				txt("清晰术", "清晰术（超分）", ""),
				txt("去背景", "去背景", ""),
				txt("ai鉴别", "AI 鉴别", ""),
				txt("图片混淆", "图片混淆", ""),
				txt("图片解混淆", "图片解混淆", ""),
				txt("添加屏蔽", "添加屏蔽词", ""),
				txt("添加黑名单", "添加黑名单", ""),
				txt("取消黑名单", "取消黑名单", ""),
				txt("查询点数", "查询点数", ""),
			},
		},
		{
			Name: "点数限制", Desc: "积分开关与基础点数",
			Fields: []UIField{
				onoff("开关", "积分开关", "0"),
				num("基础点数", "基础点数", "0"),
				sel("点数处理类型", "点数处理类型", "", "", "增加", "扣除"),
				ta("点数不足提示", "点数不足提示", ""),
				txt("充值点数指令", "充值点数指令", ""),
			},
		},
		{
			Name: "点数消耗", Desc: "各通道每次画图消耗的点数",
			Fields: []UIField{
				num("sd画图", "SD 画图点数", "0"),
				num("nai画图", "NAI 画图点数", "0"),
				num("comfy画图", "ComfyUI 画图点数", "0"),
			},
		},
		{
			Name: "图像审核", Desc: "生成图的鉴黄 / 审核",
			Fields: []UIField{
				sel("平台", "审核平台", "", "", "baidu", "baiduai"),
				onoff("开关", "审核开关", "0"),
				num("阈值", "审核阈值", "0"),
				sel("处理", "命中后的处理", "", "", "处理", "不处理", "模糊处理", "链接发送", "过滤发送", "图片混淆"),
			},
		},
		{
			Name: "黑名单功能", Desc: "被拉黑用户仍可使用的功能（取值为该功能指令名）",
			Fields: []UIField{
				txt("sd画图", "SD 画图", ""),
				txt("nai画图", "NAI 画图", ""),
				txt("清晰术", "清晰术", ""),
				txt("去背景", "去背景", ""),
				txt("上传标签", "上传标签", ""),
				txt("上传预设", "上传预设", ""),
			},
		},
		{
			Name: "nai3设置", Desc: "NovelAI 接入参数",
			RVA: "0x102B5C14",
			Fields: []UIField{
				sel("模式", "模式（官方 / 反代）", "", "", "官方", "反代"),
				sec("key", "反代 Key", ""),
				sec("官方key", "官方 Key", ""),
			},
		},
		{
			Name: "nai3参数", Desc: "NovelAI 默认出图参数",
			RVA: "0x102B5B78",
			Fields: []UIField{
				num("scale", "CFG Scale", "5"),
				num("steps", "步数", "28"),
				num("cfg", "CFG", "5"),
				num("strength", "重绘强度", "0.28"),
				txt("默认种子", "默认种子（-1 随机）", "-1"),
				num("宽度", "宽度", "1024"),
				num("高度", "高度", "1024"),
				txt("采样", "采样器", "k_euler"),
				txt("噪声", "噪声调度", "native"),
				onoff("sm", "SMEA", "0"),
				onoff("sm_dyn", "动态阈值", "0"),
				txt("全局默认模型", "全局默认模型", "nai-diffusion-3"),
				ta("负面词条", "负面词条", DefaultNegative),
				num("完成后冷却", "完成后冷却（秒）", "0"),
				sel("完成冷却模式", "冷却模式", "", "个人", "群"),
			},
		},
		{
			Name: "nai3指令", Desc: "NovelAI 指令词",
			RVA: "0x102B5D6C",
			Fields: []UIField{
				txt("画图", "画图", "画图"),
				txt("修改种子", "修改种子", ""),
				txt("修改负面", "修改负面", ""),
				txt("查看负面", "查看负面", ""),
				txt("情绪修改", "情绪修改", ""),
				txt("转线稿", "转线稿", ""),
				txt("转草稿", "转草稿", ""),
				txt("图像清理", "图像清理", ""),
				txt("着色", "着色", ""),
				txt("图像解析", "图像解析", ""),
				txt("查看预设", "查看预设", ""),
				txt("查看模型", "查看模型", ""),
				txt("查询额度", "查询额度", ""),
				txt("上传标签", "上传标签", ""),
				txt("上传预设", "上传预设", ""),
				txt("查看标签", "查看标签", ""),
				txt("删除标签", "删除标签", ""),
				txt("删除预设", "删除预设", ""),
			},
		},
		{
			Name: "sd设置", Desc: "Stable Diffusion WebUI 接入",
			RVA: "0x1039B034",
			Fields: []UIField{
				txt("链接", "链接（http://ip:port）", ""),
				txt("账号", "账号", ""),
				sec("密码", "密码", ""),
			},
		},
		{
			Name: "sd参数", Desc: "SD 默认出图参数",
			RVA: "0x102B5BCC",
			Fields: []UIField{
				txt("全局默认模型", "全局默认模型", ""),
				txt("默认种子", "默认种子（-1 随机）", "-1"),
				num("steps", "步数", "30"),
				num("识别率", "识别率", "4"),
				num("差异", "重绘差异", "0.5"),
				num("宽度", "宽度", "896"),
				num("高度", "高度", "1472"),
				txt("采样", "采样器", "Euler a"),
				onoff("高清修复", "高清修复", "0"),
				txt("高清修复算法", "高清修复算法", "Lanczos"),
				ta("负面词条", "负面词条", ""),
				num("完成后冷却", "完成后冷却（秒）", "0"),
				sel("完成冷却模式", "冷却模式", "", "个人", "群"),
			},
		},
		{
			Name: "sd指令", Desc: "SD 指令词",
			RVA: "0x102B5D10",
			Fields: []UIField{
				txt("画图", "画图", "画图"),
				txt("修改种子", "修改种子", ""),
				txt("修改负面", "修改负面", ""),
				txt("查看负面", "查看负面", ""),
				txt("查看lora", "查看 LoRA", ""),
				txt("查看模型", "查看模型", ""),
				txt("查看预设", "查看预设", ""),
				txt("查看vae", "查看 VAE", ""),
				txt("上传预设", "上传预设", ""),
				txt("删除预设", "删除预设", ""),
			},
		},
		{
			Name: "comfy设置", Desc: "ComfyUI 接入",
			RVA: "0x1039B044",
			Fields: []UIField{
				txt("链接", "链接（http://ip:port）", ""),
				sec("Token", "Token", ""),
			},
		},
		{
			Name: "comfy参数", Desc: "ComfyUI 默认出图参数与占位符默认值",
			RVA: "0x102B5BF8",
			Fields: []UIField{
				txt("全局默认模型", "全局默认模型 / 工作流", ""),
				txt("默认种子", "默认种子（-1 随机）", "-1"),
				num("steps", "步数", "20"),
				num("cfg", "CFG", "7"),
				num("宽度", "宽度", "512"),
				num("高度", "高度", "512"),
				txt("采样", "采样器", "euler"),
				txt("调度", "调度器", "normal"),
				ta("负面词条", "负面词条", ""),
				num("完成后冷却", "完成后冷却（秒）", "0"),
				sel("完成冷却模式", "冷却模式", "", "个人", "群"),
			},
		},
		{
			Name: "comfy指令", Desc: "ComfyUI 指令词",
			RVA: "0x1039B058",
			Fields: []UIField{
				txt("画图", "画图", "comfy画图"),
				txt("查看模型", "查看模型", "comfy查看模型"),
				txt("查看预设", "查看预设", "comfy查看预设"),
				txt("上传预设", "上传预设", "comfy上传预设"),
				txt("删除预设", "删除预设", "comfy删除预设"),
			},
		},
		{
			Name: "浏览器UI", Desc: "原插件用 WebView2 加载的界面版本号（重制版不用，仅保留兼容）",
			RVA: "0x1039A01C",
			Fields: []UIField{
				txt("版本号", "版本号", ""),
			},
		},
		{
			Name: "重制版", Desc: "重制版新增：持久化原插件由授权服务器下发或宿主提供的参数",
			Ext: true,
			Fields: []UIField{
				txt("nai3api", "NAI 反代地址（原为服务端下发）", ""),
				txt("naiofficialapi", "NAI 官方地址", "https://image.novelai.net"),
				txt("nai3_plugin-api", "NAI 插件接口 base（鉴黄/超分/去背景）", ""),
				txt("资源地址", "资源地址（图床上传 / 卡片模板 base）", ""),
				txt("upload-image", "图片直传接口（空则用 资源地址+/plug/AI/upload-image）", ""),
				txt("onebot_api", "OneBot HTTP API 地址", "http://127.0.0.1:3000"),
				sec("onebot_token", "OneBot access_token", ""),
				txt("onebot_listen", "OneBot 上报监听地址", "0.0.0.0:8080"),
				txt("onebot_path", "OneBot 上报路径", "/onebot"),
				onoff("onebot_ws", "启用正向 WebSocket（同路径 /ws）", "0"),
				txt("onebot_ws_reverse", "反向 WebSocket 地址", ""),
			},
		},
	}
}

// ---------------------------------------------------------------------------
// 读写 ini
// ---------------------------------------------------------------------------

// ReadConfig 读出 ini 里所有节的键值。
//
// 返回值是 map[节]map[键]值；只包含 schema 里声明过的键与 ini 里已存在的键，
// 未出现在 ini 里的键用 schema 的默认值补齐，保证面板显示完整。
func (s *UIServer) ReadConfig() (map[string]map[string]string, error) {
	ini := LoadIni(MainINIPath(s.opt.DataDir))
	out := map[string]map[string]string{}
	for _, sec := range UISchema() {
		m := map[string]string{}
		for _, f := range sec.Fields {
			m[f.Key] = ini.Get(sec.Name, f.Key, f.Def)
		}
		out[sec.Name] = m
	}
	return out, nil
}

// WriteConfig 把面板提交的键值写回 ini，并热重载插件。
//
// 只写 schema 里声明过的节与键，避免面板误写其它内容；空值不写（保持
// ini 精简，与原插件“有值才落盘”的行为一致）。
func (s *UIServer) WriteConfig(doc map[string]map[string]string) error {
	path := MainINIPath(s.opt.DataDir)
	ini := LoadIni(path)

	known := map[string]map[string]bool{}
	for _, sec := range UISchema() {
		known[sec.Name] = map[string]bool{}
		for _, f := range sec.Fields {
			known[sec.Name][f.Key] = true
		}
	}
	for secName, kv := range doc {
		keys, ok := known[secName]
		if !ok {
			return fmt.Errorf("未知配置节：%s", secName)
		}
		for k, v := range kv {
			if !keys[k] {
				return fmt.Errorf("未知配置项：%s/%s", secName, k)
			}
			if strings.TrimSpace(v) == "" {
				continue
			}
			ini.Set(secName, k, v)
		}
	}
	if err := ini.Save(); err != nil {
		return err
	}
	if s.plug != nil {
		s.plug.ReloadFromINI()
	}
	return nil
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (s *UIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(uiPageHTML))
}

func (s *UIServer) handleSchema(w http.ResponseWriter, r *http.Request) {
	uiWriteJSON(w, http.StatusOK, UISchema())
}

func (s *UIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc, err := s.ReadConfig()
		if err != nil {
			uiWriteJSONErr(w, http.StatusInternalServerError, err)
			return
		}
		uiWriteJSON(w, http.StatusOK, doc)
	case http.MethodPost:
		var doc map[string]map[string]string
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			uiWriteJSONErr(w, http.StatusBadRequest, fmt.Errorf("配置格式错误：%w", err))
			return
		}
		if err := s.WriteConfig(doc); err != nil {
			uiWriteJSONErr(w, http.StatusInternalServerError, err)
			return
		}
		back, err := s.ReadConfig()
		if err != nil {
			uiWriteJSONErr(w, http.StatusInternalServerError, err)
			return
		}
		uiWriteJSON(w, http.StatusOK, map[string]any{"ok": true, "config": back})
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *UIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := map[string]any{
		"data_dir": s.opt.DataDir,
		"ini_path": MainINIPath(s.opt.DataDir),
	}
	if s.plug != nil {
		c := s.plug.Config()
		st["inited"] = s.plug.Inited()
		st["enabled"] = s.plug.Enabled()
		st["sd_url"] = c.SD.URL
		st["comfy_url"] = c.Comfy.URL
		st["nai_mode"] = c.Mode
	}
	uiWriteJSON(w, http.StatusOK, st)
}

// handlePresets 列出数据目录里的预设与工作流，供面板展示。
func (s *UIServer) handlePresets(w http.ResponseWriter, r *http.Request) {
	dir := s.opt.DataDir
	out := map[string]any{}

	sd := LoadSDPresets(filepath.Join(dir, FileSdPresetJSON))
	sdNames := make([]string, 0, len(sd))
	for _, p := range sd {
		sdNames = append(sdNames, p.Name)
	}
	out["sd_presets"] = sdNames

	cm := LoadComfyPresets(filepath.Join(dir, FileComfyPresetJSON))
	cNames := make([]string, 0, len(cm))
	for _, p := range cm {
		cNames = append(cNames, p.Name)
	}
	out["comfy_presets"] = cNames

	var wfs []string
	if ents, err := os.ReadDir(filepath.Join(dir, FileComfyWfDir)); err == nil {
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
				wfs = append(wfs, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
			}
		}
	}
	sort.Strings(wfs)
	out["comfy_workflows"] = wfs

	var naiPresets, naiTags []string
	if st, err := LoadStore(
		filepath.Join(dir, FileNai3PresetJSON),
		filepath.Join(dir, FileNai3TagJSON),
	); err == nil {
		for _, p := range st.Presets {
			naiPresets = append(naiPresets, p.Name)
		}
		for _, p := range st.Tags {
			naiTags = append(naiTags, p.Name)
		}
	}
	out["nai_presets"] = naiPresets
	out["nai_tags"] = naiTags

	uiWriteJSON(w, http.StatusOK, out)
}

func uiWriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func uiWriteJSONErr(w http.ResponseWriter, code int, err error) {
	uiWriteJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
}

// ErrUINotStarted 表示面板尚未 Start。
var ErrUINotStarted = errors.New("配置面板未启动")

// ---------------------------------------------------------------------------
// 页面
// ---------------------------------------------------------------------------

const uiPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>咸鱼ai画图 · 配置面板</title>
<style>
*{box-sizing:border-box}
body{margin:0;font:14px/1.6 -apple-system,"Segoe UI","Microsoft YaHei",sans-serif;background:#f4f5f7;color:#1c2026}
header{background:#171a1f;color:#fff;padding:18px 26px;display:flex;align-items:baseline;gap:14px}
header h1{margin:0;font-size:17px;font-weight:600;letter-spacing:.02em}
header .sub{font-size:12px;color:#8d97a5}
.layout{display:flex;min-height:calc(100vh - 62px)}
aside{width:186px;flex:0 0 186px;background:#fff;border-right:1px solid #e2e5ea;padding:10px 0;overflow:auto}
aside button{display:block;width:100%;text-align:left;background:none;border:0;border-left:3px solid transparent;
  padding:9px 16px;font:inherit;color:#4a5462;cursor:pointer}
aside button:hover{background:#f4f5f7}
aside button.on{color:#111;border-left-color:#111;font-weight:600;background:#f7f8fa}
aside button .ext{font-size:10px;color:#b06a00;border:1px solid #e8cfa6;background:#fdf6ea;
  border-radius:3px;padding:0 4px;margin-left:6px;vertical-align:1px}
main{flex:1;padding:22px 26px 90px;max-width:940px}
h2{margin:0 0 2px;font-size:15px}
.desc{margin:0 0 16px;font-size:12px;color:#727d8c}
.card{background:#fff;border:1px solid #e2e5ea;border-radius:8px;padding:20px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:14px 22px}
label{display:flex;flex-direction:column;gap:5px;font-size:12px;color:#5a6470}
label.wide{grid-column:1/-1}
label.chk{flex-direction:row;align-items:center;gap:8px;font-size:13px;color:#1c2026;padding-top:20px}
input,select,textarea{font:inherit;font-size:13px;padding:7px 9px;border:1px solid #d2d7df;border-radius:5px;
  background:#fff;color:#1c2026;width:100%}
textarea{min-height:70px;resize:vertical;font-family:ui-monospace,Consolas,monospace;font-size:12px}
input:focus,select:focus,textarea:focus{outline:2px solid #cddfff;border-color:#7aa4e8}
input[type=checkbox]{width:auto}
.key{font:11px/1.4 ui-monospace,Consolas,monospace;color:#9aa4b2}
footer{position:fixed;left:186px;right:0;bottom:0;background:#fff;border-top:1px solid #e2e5ea;
  padding:12px 26px;display:flex;align-items:center;gap:10px}
footer .path{margin-right:auto;font:11px/1.4 ui-monospace,Consolas,monospace;color:#8d97a5;
  white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
button.primary{background:#171a1f;color:#fff;border:0;padding:9px 22px;border-radius:5px;font:inherit;cursor:pointer}
button.ghost{background:#fff;color:#1c2026;border:1px solid #d2d7df;padding:9px 18px;border-radius:5px;
  font:inherit;cursor:pointer}
button.primary:disabled{opacity:.5;cursor:default}
#msg{position:fixed;right:26px;bottom:76px;background:#171a1f;color:#fff;padding:10px 16px;border-radius:6px;
  font-size:13px;opacity:0;transition:opacity .18s;pointer-events:none;max-width:420px}
#msg.on{opacity:1}
#msg.err{background:#a3231a}
.tag{display:inline-block;background:#eef1f5;border-radius:4px;padding:2px 8px;margin:2px 4px 2px 0;font-size:12px;color:#3b4553}
.empty{font-size:12px;color:#9aa4b2}
</style>
</head>
<body>
<header>
  <h1>咸鱼ai画图 · 配置面板</h1>
  <span class="sub">读写 咸鱼ai画图.ini，与手工编辑完全等价</span>
</header>
<div class="layout">
  <aside id="nav"></aside>
  <main>
    <h2 id="sec-title"></h2>
    <p class="desc" id="sec-desc"></p>
    <div class="card"><div class="grid" id="fields"></div></div>
    <div class="card" id="presets-card" style="display:none;margin-top:16px">
      <h2 style="font-size:13px;margin-bottom:12px">已装载内容</h2>
      <p><strong>NAI 预设</strong><br><span id="pl-nai-preset" class="empty">—</span></p>
      <p><strong>NAI 标签</strong><br><span id="pl-nai-tag" class="empty">—</span></p>
      <p><strong>SD 预设</strong><br><span id="pl-sd" class="empty">—</span></p>
      <p style="margin-bottom:0"><strong>ComfyUI 预设 / 工作流</strong><br>
         <span id="pl-comfy" class="empty">—</span><br><span id="pl-wf" class="empty">—</span></p>
    </div>
  </main>
</div>
<footer>
  <span class="path" id="ini-path"></span>
  <button class="ghost" id="reload">重新读取</button>
  <button class="primary" id="save">保存并生效</button>
</footer>
<div id="msg"></div>
<script>
(function(){
  var schema = [], cfg = {}, cur = null, dirty = false;
  var $ = function(s){ return document.querySelector(s); };

  function toast(t, err){
    var m = $('#msg'); m.textContent = t; m.className = 'on' + (err ? ' err' : '');
    clearTimeout(toast._t); toast._t = setTimeout(function(){ m.className = ''; }, 2600);
  }

  function el(tag, cls, text){
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function renderNav(){
    var nav = $('#nav'); nav.innerHTML = '';
    schema.forEach(function(s){
      var b = el('button', '', s.name);
      if (s.ext){
        var t = el('span', 'ext', '新增');
        b.appendChild(t);
      }
      b.dataset.sec = s.name;
      if (cur === s.name) b.classList.add('on');
      b.addEventListener('click', function(){ select(s.name); });
      nav.appendChild(b);
    });
  }

  function select(name){
    cur = name;
    renderNav();
    var s = schema.filter(function(x){ return x.name === name; })[0];
    if (!s) return;
    $('#sec-title').textContent = s.name;
    $('#sec-desc').textContent = s.desc + (s.rva ? '　节名常量 ' + s.rva : '');
    var box = $('#fields'); box.innerHTML = '';
    s.fields.forEach(function(f){
      var wrap = el('label');
      if (f.type === 'textarea') wrap.className = 'wide';
      if (f.type === 'bool') wrap.className = 'chk';

      var input;
      if (f.type === 'textarea'){
        input = el('textarea');
      } else if (f.type === 'select'){
        input = el('select');
        var opts = (f.opts || ['']).slice();
        if (opts.indexOf(f.def) < 0 && f.def) opts.unshift(f.def);
        opts.forEach(function(o){
          var op = el('option', '', o === '' ? '（默认）' : o);
          op.value = o; input.appendChild(op);
        });
      } else if (f.type === 'bool'){
        input = el('input'); input.type = 'checkbox';
      } else {
        input = el('input');
        input.type = f.type === 'number' ? 'number' : (f.type === 'password' ? 'password' : 'text');
        if (f.type === 'number') { input.step = 'any'; }
      }

      var v = (cfg[s.name] || {})[f.key];
      if (v === undefined || v === null) v = f.def || '';
      if (f.type === 'bool'){
        input.checked = (String(v) === '1' || v === 'true');
      } else {
        input.value = v;
      }
      input.dataset.sec = s.name; input.dataset.key = f.key;
      input.addEventListener('input', function(){ dirty = true; });
      input.addEventListener('change', function(){ dirty = true; });

      if (f.type === 'bool'){
        wrap.appendChild(input);
        wrap.appendChild(document.createTextNode(f.label));
      } else {
        wrap.appendChild(el('span', '', f.label));
        wrap.appendChild(input);
      }
      var k = el('span', 'key', f.key);
      wrap.appendChild(k);
      box.appendChild(wrap);
    });
    $('#presets-card').style.display = (name === '指令') ? 'block' : 'none';
    if (name === '指令') loadPresets();
  }

  function collect(){
    var out = {};
    schema.forEach(function(s){ out[s.name] = {}; });
    document.querySelectorAll('[data-sec][data-key]').forEach(function(inp){
      var s = inp.dataset.sec, k = inp.dataset.key;
      if (!out[s]) out[s] = {};
      if (inp.type === 'checkbox') out[s][k] = inp.checked ? '1' : '0';
      else out[s][k] = inp.value;
    });
    // 未在当前页显示的历史值原样带回，避免保存时丢失
    schema.forEach(function(s){
      var have = out[s.name] || {};
      var old = cfg[s.name] || {};
      Object.keys(old).forEach(function(k){
        if (have[k] === undefined) have[k] = old[k];
      });
      out[s.name] = have;
    });
    return out;
  }

  function load(){
    Promise.all([
      fetch('/api/schema').then(function(r){ return r.json(); }),
      fetch('/api/config').then(function(r){ return r.json(); }),
      fetch('/api/status').then(function(r){ return r.json(); }).catch(function(){ return {}; })
    ]).then(function(res){
      schema = res[0]; cfg = res[1]; var st = res[2] || {};
      $('#ini-path').textContent = st.ini_path || '';
      if (!cur && schema.length) cur = schema[0].name;
      select(cur);
      dirty = false;
      toast('已读取配置');
    }).catch(function(e){ toast('读取失败：' + e, 1); });
  }

  function tags(id, arr){
    var box = document.getElementById(id);
    box.className = '';
    box.innerHTML = '';
    if (!arr || !arr.length){ box.className = 'empty'; box.textContent = '（空）'; return; }
    arr.forEach(function(x){ box.appendChild(el('span', 'tag', x)); });
  }

  function loadPresets(){
    fetch('/api/presets').then(function(r){ return r.json(); }).then(function(d){
      tags('pl-nai-preset', d.nai_presets);
      tags('pl-nai-tag', d.nai_tags);
      tags('pl-sd', d.sd_presets);
      tags('pl-comfy', d.comfy_presets);
      tags('pl-wf', d.comfy_workflows);
    }).catch(function(){});
  }

  $('#reload').addEventListener('click', function(){
    if (dirty && !confirm('有未保存的修改，确定重新读取？')) return;
    load();
  });

  $('#save').addEventListener('click', function(){
    var btn = this; btn.disabled = true;
    fetch('/api/config', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(collect())
    }).then(function(r){ return r.json(); }).then(function(d){
      btn.disabled = false;
      if (d.ok){ cfg = d.config; dirty = false; select(cur); toast('已保存并热重载'); }
      else { toast('保存失败：' + (d.error || '未知错误'), 1); }
    }).catch(function(e){ btn.disabled = false; toast('保存失败：' + e, 1); });
  });

  window.addEventListener('beforeunload', function(e){
    if (dirty){ e.preventDefault(); e.returnValue = ''; }
  });

  load();
})();
</script>
</body>
</html>`
