package nai

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 本文件复刻插件的 ComfyUI 通道（配置、工作流绑定、占位符）。
//
// ---- 配置（ini）----
//
//	[comfy设置] 链接 -> 0x104EBE14      Token -> 0x104EBF18
//	[comfy参数] steps=20    cfg=7        宽度=512      高度=512
//	            默认种子=-1 负面词条=<sd 默认串>  采样=euler  调度=normal
//	            全局默认模型 -> 0x104EBF10      完成后冷却=0 -> 0x104EBF04
//	            完成冷却模式=个人 -> 0x104EBF08
//	[comfy指令] 画图=comfy画图 查看模型=comfy查看模型 查看预设=comfy查看预设
//	            上传预设=comfy上传预设 删除预设=comfy删除预设
//
// 证据：默认值写入段 0x1009B440-0x1009BAE0（全部是 读配置项(ini,节,键,默认)），
// 结构体基址 0x104EBF0C：+0x00 steps +0x04 cfg +0x08 宽度 +0x0c 高度
// +0x10 默认种子 +0x14 负面词条 +0x18 采样 +0x1c 调度。
//
// ---- 工作流绑定 ----
//
//   - 运行期聚合文本放在 0x104EBEFC（0x1009BB00 赋值）。
//   - 装载函数 0x1009BBB4：读 comfy预设列表.json -> 0x104EBEF8；
//     目录 {数据目录}comfy工作流\（0x1039B09C）不存在就创建（0x101C91D0）；
//     旧版聚合文件 comfy工作流.json（0x1039B0AC）按成员拆成一个个
//     {数据目录}comfy工作流\<名>.json，随后改名成 comfy工作流.json.bak
//     （0x1039B0C8，0x101C9210 文件更名）；最后枚举目录里的 *.json 组装绑定表。
//   - 绑定名就是文件名去扩展名；绑定内容是该文件的 JSON 文本。
//   - 每用户选中的绑定名：user_cache.ini（0x102B7D70）节=<QQ>
//     键=comfy_model_name（0x102BB768）。
//
// ---- 占位符 ----
//
//	《种子》0x102BB8B8 《高度》0x102BB8C4 《宽度》0x102BB8D0
//	《负面提示词》0x102BB8DC 《提示词》0x102BB8EC 《调度器》0x102BB8F8
//	《采样器》0x102BB904 《CFG》0x102BB910 《步骤》0x102BB918
//	另支持 《参数名|默认值》：「|」0x102B81C4，默认值在未提供该参数时使用。
//	未提供的参数会被收集，拼成
//	  "工作流存在未提供的参数："(0x102BB99C) + 《a》、《b》(0x102BB944/0x102BB94C/0x102BB948)
//	  + " 请在聊天中用 #参数名:值 提供，或在占位符中写默认值（《参数名|默认值》）"(0x102BB950)

const (
	// 小节名
	IniSectionComfyAPI   = "comfy设置" // 0x1039B044
	IniSectionComfyParam = "comfy参数" // 0x102B5BF8
	IniSectionComfyCmd   = "comfy指令" // 0x1039B058

	// comfy设置
	IniKeyComfyURL   = "链接"    // 0x102B8C18
	IniKeyComfyToken = "Token" // 0x1039B050

	// comfy参数
	IniKeyComfySteps     = "steps"
	IniKeyComfyCfg       = "cfg"    // 0x1039AC68
	IniKeyComfyWidth     = "宽度"     // 0x1039AC6C
	IniKeyComfyHeight    = "高度"     // 0x1039AC74
	IniKeyComfySeed      = "默认种子"   // 0x1039AC44
	IniKeyComfyNegative  = "负面词条"   // 0x1039AC38
	IniKeyComfySampler   = "采样"     // 0x1039AC58
	IniKeyComfyScheduler = "调度"     // 0x1039B094
	IniKeyComfyModel     = "全局默认模型" // 0x1039AC8C
	IniKeyComfyCooldown  = "完成后冷却"  // 0x1039AC9C
	IniKeyComfyCoolMode  = "完成冷却模式" // 0x1039ACB0

	// comfy指令
	IniKeyComfyCmdDraw         = "画图"   // 0x1039AA40
	IniKeyComfyCmdViewModel    = "查看模型" // 0x1039B064
	IniKeyComfyCmdViewPreset   = "查看预设" // 0x1039B074
	IniKeyComfyCmdUploadPreset = "上传预设" // 0x1039B063 起
	IniKeyComfyCmdDeletePreset = "删除预设" // 0x1039B084
)

// ComfyUI 数据文件与目录。
const (
	FileComfyPresetJSON  = "comfy预设列表.json"    // 0x102BB600
	FileComfyWfJSON      = "comfy工作流.json"     // 0x1039B0AC
	FileComfyWfJSONBak   = "comfy工作流.json.bak" // 0x1039B0C8
	FileComfyWfDir       = "comfy工作流"          // 0x1039B09C
	FileUserCacheINI     = "user_cache.ini"    // 0x102B7D70
	IniKeyComfyModelName = "comfy_model_name"  // 0x102BB768
	ComfyClientID        = "xianyuai"          // 0x102BB9F0
)

// 占位符名（逐字来自 0x102BB8B8-0x102BB918）。
const (
	PhComfySeed      = "种子"
	PhComfyHeight    = "高度"
	PhComfyWidth     = "宽度"
	PhComfyNegative  = "负面提示词"
	PhComfyPrompt    = "提示词"
	PhComfyScheduler = "调度器"
	PhComfySampler   = "采样器"
	PhComfyCFG       = "CFG"
	PhComfySteps     = "步骤"
)

// ComfyUI 回执/错误文案（0x102B5D84 起、0x102BB794 起）。
const (
	MsgComfyBlacklisted     = "黑名单禁止使用comfy画图："                         // 0x102B5D90
	MsgComfyNotConfigured   = "ComfyUI链接未配置，请配置后再来绘图"                   // 0x102B5DAC
	MsgComfyNoWorkflow      = "未配置 ComfyUI 工作流绑定，请管理员到设置界面添加"           // 0x102BB794
	MsgComfyWfNotExist      = "所选 ComfyUI 工作流绑定不存在，请管理员在设置界面重新配置"       // 0x102BB820
	MsgComfyWfEmpty         = "ComfyUI 工作流绑定内容为空"                       // 0x102BB884
	MsgComfyNeedPrompt      = "请在指令后输入提示词"                              // 0x102BB85C
	MsgComfyModelOutOfRange = "模型序号超出范围，当前共 "                           // 0x102BB7D8
	MsgComfyModelCount      = " 个工作流绑定"                                 // 0x102BB7C8
	MsgComfyParamsMissing   = "工作流存在未提供的参数："                            // 0x102BB99C
	MsgComfyParamsHint      = " 请在聊天中用 #参数名:值 提供，或在占位符中写默认值（《参数名|默认值》）" // 0x102BB950
	MsgComfyJSONFail        = "ComfyUI 工作流 JSON 解析失败，请检查绑定内容与占位符"       // 0x102BB9B8
	MsgComfySubmitFail      = "comfy提交失败，接口响应："                         // 0x102BBA34
	MsgComfySubmitHint      = "comfy提交失败，请检查 ComfyUI 链接与工作流配置"          // 0x102BBA50
	MsgComfyTimeout         = "comfy绘图超时，请检查 ComfyUI 服务状态"              // 0x102BBAE0
	MsgComfyExecFail        = "comfy执行失败，history响应："                    // 0x102BBB1C
	MsgComfyDrawFail        = "comfy绘图失败："                              // 0x102BBB3C
	MsgComfyNoImage         = "comfy绘图完成但未返回图片，请检查工作流的 SaveImage 节点"    // 0x102BBBB8
	MsgComfyAuditRejected   = "comfy出图全部被审核拦截"                          // 0x102BBC44
	MsgComfyRefUploadFail   = " 张参考图上传 ComfyUI 失败"                      // 0x102BB924
)

// ComfyUI 接口路径（0x102BBAxx / 0x102BBCxx）。
const (
	PathComfyPrompt   = "/prompt"                   // 0x102BBAxx
	PathComfyHistory  = "/history/"                 // 0x102BBBxx
	PathComfyView     = "/view?filename="           // 0x102BBC08
	PathComfyUpload   = "/upload/image?overwrite=1" // 0x102BBC5C
	ComfyFieldName    = "name"                      // 0x102BBC94
	ComfyPollMax      = 0x5a                        // 90 次
	ComfyPollInterval = 2000                        // 毫秒
)

// ComfyConfig 是 [comfy设置] + [comfy参数] 的联合视图。
type ComfyConfig struct {
	URL   string // comfy设置/链接
	Token string // comfy设置/Token

	Steps     int     // comfy参数/steps，默认 20
	Cfg       float64 // comfy参数/cfg，默认 7
	Width     int     // comfy参数/宽度，默认 512
	Height    int     // comfy参数/高度，默认 512
	Seed      string  // comfy参数/默认种子，默认 -1
	Negative  string  // comfy参数/负面词条
	Sampler   string  // comfy参数/采样，默认 euler
	Scheduler string  // comfy参数/调度，默认 normal

	GlobalModel  string // comfy参数/全局默认模型
	Cooldown     string // comfy参数/完成后冷却
	CooldownMode string // comfy参数/完成冷却模式，默认 个人

	// 指令词（[comfy指令]）
	CmdDraw         string
	CmdViewModel    string
	CmdViewPreset   string
	CmdUploadPreset string
	CmdDeletePreset string

	TimeoutMS int
}

// DefaultComfyConfig 返回 DLL 写入 ini 的默认值（0x1009B440-0x1009BAE0）。
func DefaultComfyConfig() ComfyConfig {
	return ComfyConfig{
		Steps:           20,   // 0x102BB8A0
		Cfg:             7,    // 0x102B5AA8
		Width:           512,  // 0x102BB8A4
		Height:          512,  // 0x102BB8A4
		Seed:            "-1", // 0x102B8DD0
		Negative:        SDDefaultNegative,
		Sampler:         "euler",  // 0x102BB8A8
		Scheduler:       "normal", // 0x102BB8B0
		Cooldown:        "0",      // 0x102B5A8C
		CooldownMode:    "个人",     // 0x1039ACA8
		CmdDraw:         "comfy画图",
		CmdViewModel:    "comfy查看模型",
		CmdViewPreset:   "comfy查看预设",
		CmdUploadPreset: "comfy上传预设",
		CmdDeletePreset: "comfy删除预设",
		TimeoutMS:       300000,
	}
}

// withDefaults 给零值字段补默认值（Seed 用文本，"0" 合法，不覆盖）。
func (c ComfyConfig) withDefaults() ComfyConfig {
	d := DefaultComfyConfig()
	if c.Steps <= 0 {
		c.Steps = d.Steps
	}
	if c.Cfg <= 0 {
		c.Cfg = d.Cfg
	}
	if c.Width <= 0 {
		c.Width = d.Width
	}
	if c.Height <= 0 {
		c.Height = d.Height
	}
	if c.Seed == "" {
		c.Seed = d.Seed
	}
	if c.Negative == "" {
		c.Negative = d.Negative
	}
	if c.Sampler == "" {
		c.Sampler = d.Sampler
	}
	if c.Scheduler == "" {
		c.Scheduler = d.Scheduler
	}
	if c.CooldownMode == "" {
		c.CooldownMode = d.CooldownMode
	}
	if c.CmdDraw == "" {
		c.CmdDraw = d.CmdDraw
	}
	if c.CmdViewModel == "" {
		c.CmdViewModel = d.CmdViewModel
	}
	if c.CmdViewPreset == "" {
		c.CmdViewPreset = d.CmdViewPreset
	}
	if c.CmdUploadPreset == "" {
		c.CmdUploadPreset = d.CmdUploadPreset
	}
	if c.CmdDeletePreset == "" {
		c.CmdDeletePreset = d.CmdDeletePreset
	}
	if c.TimeoutMS <= 0 {
		c.TimeoutMS = d.TimeoutMS
	}
	return c
}

// ComfyWithDefaults 对外暴露 withDefaults。
func ComfyWithDefaults(c ComfyConfig) ComfyConfig { return c.withDefaults() }

// ComfyBaseURL 返回去掉末尾斜杠的链接（DLL 是裸拼，重制版按同样的裸拼，
// 只在配置里带尾巴斜杠时替用户收一下）。
func (c ComfyConfig) ComfyBaseURL() string {
	u := strings.TrimSpace(c.URL)
	return strings.TrimRight(u, "/")
}

// ComfyAvailable 报告链接是否已配置。
func (c ComfyConfig) ComfyAvailable() bool { return strings.TrimSpace(c.URL) != "" }

// LoadComfyConfigFromINI 读 [comfy设置] / [comfy参数] / [comfy指令]。
func LoadComfyConfigFromINI(path string, base ComfyConfig) ComfyConfig {
	out := base.withDefaults()
	f := LoadIni(path)
	if f.Path == "" {
		return out
	}
	out.URL = f.Get(IniSectionComfyAPI, IniKeyComfyURL, out.URL)
	out.Token = f.Get(IniSectionComfyAPI, IniKeyComfyToken, out.Token)

	out.Steps = iniInt(f, IniSectionComfyParam, IniKeyComfySteps, out.Steps)
	out.Cfg = iniFloat(f, IniSectionComfyParam, IniKeyComfyCfg, out.Cfg)
	out.Width = iniInt(f, IniSectionComfyParam, IniKeyComfyWidth, out.Width)
	out.Height = iniInt(f, IniSectionComfyParam, IniKeyComfyHeight, out.Height)
	out.Seed = f.Get(IniSectionComfyParam, IniKeyComfySeed, out.Seed)
	out.Negative = f.Get(IniSectionComfyParam, IniKeyComfyNegative, out.Negative)
	out.Sampler = f.Get(IniSectionComfyParam, IniKeyComfySampler, out.Sampler)
	out.Scheduler = f.Get(IniSectionComfyParam, IniKeyComfyScheduler, out.Scheduler)
	out.GlobalModel = f.Get(IniSectionComfyParam, IniKeyComfyModel, out.GlobalModel)
	out.Cooldown = f.Get(IniSectionComfyParam, IniKeyComfyCooldown, out.Cooldown)
	out.CooldownMode = f.Get(IniSectionComfyParam, IniKeyComfyCoolMode, out.CooldownMode)

	out.CmdDraw = f.Get(IniSectionComfyCmd, IniKeyComfyCmdDraw, out.CmdDraw)
	out.CmdViewModel = f.Get(IniSectionComfyCmd, IniKeyComfyCmdViewModel, out.CmdViewModel)
	out.CmdViewPreset = f.Get(IniSectionComfyCmd, IniKeyComfyCmdViewPreset, out.CmdViewPreset)
	out.CmdUploadPreset = f.Get(IniSectionComfyCmd, IniKeyComfyCmdUploadPreset, out.CmdUploadPreset)
	out.CmdDeletePreset = f.Get(IniSectionComfyCmd, IniKeyComfyCmdDeletePreset, out.CmdDeletePreset)
	return out
}

// SaveComfyConfigToINI 把非零项写回 ini。
func SaveComfyConfigToINI(path string, c ComfyConfig) error {
	f := LoadIni(path)
	if c.URL != "" {
		f.Set(IniSectionComfyAPI, IniKeyComfyURL, c.URL)
	}
	if c.Token != "" {
		f.Set(IniSectionComfyAPI, IniKeyComfyToken, c.Token)
	}
	if c.Steps > 0 {
		f.Set(IniSectionComfyParam, IniKeyComfySteps, strconv.Itoa(c.Steps))
	}
	if c.Cfg > 0 {
		f.Set(IniSectionComfyParam, IniKeyComfyCfg, sdNum(c.Cfg))
	}
	if c.Width > 0 {
		f.Set(IniSectionComfyParam, IniKeyComfyWidth, strconv.Itoa(c.Width))
	}
	if c.Height > 0 {
		f.Set(IniSectionComfyParam, IniKeyComfyHeight, strconv.Itoa(c.Height))
	}
	if c.Seed != "" {
		f.Set(IniSectionComfyParam, IniKeyComfySeed, c.Seed)
	}
	if c.Negative != "" {
		f.Set(IniSectionComfyParam, IniKeyComfyNegative, c.Negative)
	}
	if c.Sampler != "" {
		f.Set(IniSectionComfyParam, IniKeyComfySampler, c.Sampler)
	}
	if c.Scheduler != "" {
		f.Set(IniSectionComfyParam, IniKeyComfyScheduler, c.Scheduler)
	}
	if c.GlobalModel != "" {
		f.Set(IniSectionComfyParam, IniKeyComfyModel, c.GlobalModel)
	}
	if c.Cooldown != "" {
		f.Set(IniSectionComfyParam, IniKeyComfyCooldown, c.Cooldown)
	}
	if c.CooldownMode != "" {
		f.Set(IniSectionComfyParam, IniKeyComfyCoolMode, c.CooldownMode)
	}
	return f.Save()
}

// ---------- 工作流绑定 ----------

// ComfyWorkflow 是一条工作流绑定：名字就是文件名（去 .json），内容是工作流 JSON 文本。
type ComfyWorkflow struct {
	Name    string
	Content string
	Path    string
}

// ComfyWorkflowDir 返回 {数据目录}/comfy工作流。
func ComfyWorkflowDir(dataDir string) string {
	return joinDir(dataDir, FileComfyWfDir)
}

// LoadComfyWorkflows 装载工作流绑定。
//
// 顺序与 DLL 一致（0x1009BBB4）：
//  1. 目录里每个 *.json 一个绑定（文件名去扩展名 = 绑定名）；
//  2. 若目录里有旧版聚合文件 comfy工作流.json，把里面还没落盘的成员拆成文件，
//     然后把它改名成 comfy工作流.json.bak（若 .bak 已存在则直接删除原名）。
func LoadComfyWorkflows(dataDir string) []ComfyWorkflow {
	dir := ComfyWorkflowDir(dataDir)
	out := listComfyWorkflowFiles(dir)

	aggPath := joinDir(dataDir, FileComfyWfJSON)
	if raw, err := os.ReadFile(aggPath); err == nil {
		added := splitComfyWorkflowAggregate(dir, raw, out)
		if len(added) > 0 {
			out = append(out, added...)
		}
		bak := joinDir(dataDir, FileComfyWfJSONBak)
		if err := os.Rename(aggPath, bak); err != nil {
			_ = os.Remove(aggPath)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func listComfyWorkflowFiles(dir string) []ComfyWorkflow {
	var out []ComfyWorkflow
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out = append(out, ComfyWorkflow{
			Name:    strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
			Content: string(b),
			Path:    p,
		})
	}
	return out
}

// splitComfyWorkflowAggregate 把旧版聚合文件拆成单文件。返回新增的绑定。
// 聚合结构是 { "<名>": <工作流 JSON> }（DLL 按成员逐个 写到文件）。
func splitComfyWorkflowAggregate(dir string, raw []byte, existing []ComfyWorkflow) []ComfyWorkflow {
	root, err := decodeOJSON(raw)
	if err != nil || root == nil || root.kind != 'o' || len(root.keys) == 0 {
		return nil
	}
	have := make(map[string]bool, len(existing))
	for _, w := range existing {
		have[w.Name] = true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	var out []ComfyWorkflow
	for i, key := range root.keys {
		name := strings.TrimSpace(key)
		if name == "" || have[name] {
			continue
		}
		var node *ojson
		if i < len(root.vals) {
			node = root.vals[i]
		}
		content := ""
		if node != nil {
			if node.kind == 's' {
				content = node.str
			} else {
				content = node.Text()
			}
		}
		p := filepath.Join(dir, name+".json")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			continue
		}
		out = append(out, ComfyWorkflow{Name: name, Content: content, Path: p})
	}
	return out
}

// Names 返回绑定名列表（顺序即 #模型N 的序号）。
func ComfyWorkflowNames(ws []ComfyWorkflow) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Name)
	}
	return out
}

// FindComfyWorkflow 按名字取绑定。
func FindComfyWorkflow(ws []ComfyWorkflow, name string) (ComfyWorkflow, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ComfyWorkflow{}, false
	}
	for _, w := range ws {
		if w.Name == name {
			return w, true
		}
	}
	return ComfyWorkflow{}, false
}

// ComfySelectedWorkflow 返回某用户选中的绑定名（user_cache.ini，节=QQ，键 comfy_model_name）。
func ComfySelectedWorkflow(dataDir, qq string) string {
	f := LoadIni(joinDir(dataDir, FileUserCacheINI))
	if f.Path == "" {
		return ""
	}
	return strings.TrimSpace(f.Get(qq, IniKeyComfyModelName, ""))
}

// SetComfySelectedWorkflow 写某用户选中的绑定名。
func SetComfySelectedWorkflow(dataDir, qq, name string) error {
	f := LoadIni(joinDir(dataDir, FileUserCacheINI))
	f.Set(qq, IniKeyComfyModelName, name)
	return f.Save()
}

// ---------- 占位符 ----------

// ComfyPlaceholderValues 是一轮替换用的取值表。
type ComfyPlaceholderValues map[string]string

// SubstituteComfyPlaceholders 把工作流文本里的 《名》/《名|默认值》 换成取值。
//
// 返回值 unresolved 是既没有取值又没有默认值的参数名（去重、保持出现顺序）。
// 有默认值时用默认值；取值非空时优先用取值。
func SubstituteComfyPlaceholders(tmpl string, vals ComfyPlaceholderValues) (string, []string) {
	var b strings.Builder
	var missing []string
	seen := map[string]bool{}
	for i := 0; i < len(tmpl); {
		// 《 的 GBK 是 0xA1 0xB6，UTF-8 是 E3 80 8A；这里按 UTF-8 处理。
		if strings.HasPrefix(tmpl[i:], "《") {
			j := strings.Index(tmpl[i+len("《"):], "》")
			if j < 0 {
				b.WriteString(tmpl[i:])
				break
			}
			inner := tmpl[i+len("《") : i+len("《")+j]
			end := i + len("《") + j + len("》")
			name, def, hasDef := inner, "", false
			if k := strings.Index(inner, "|"); k >= 0 {
				name = strings.TrimSpace(inner[:k])
				def = inner[k+1:]
				hasDef = true
			} else {
				name = strings.TrimSpace(inner)
			}
			if v := vals[name]; strings.TrimSpace(v) != "" {
				b.WriteString(v)
			} else if hasDef {
				b.WriteString(def)
			} else {
				if !seen[name] {
					seen[name] = true
					missing = append(missing, name)
				}
				b.WriteString(tmpl[i:end])
			}
			i = end
			continue
		}
		b.WriteByte(tmpl[i])
		i++
	}
	return b.String(), missing
}

// RenderComfyMissingParams 复刻"工作流存在未提供的参数"提示（0x10078CC5 / 0x10079029 / 0x1007915B）。
//
//	"工作流存在未提供的参数：" + "《a》、《b》" + " 请在聊天中用 #参数名:值 提供，或在占位符中写默认值（《参数名|默认值》）"
func RenderComfyMissingParams(names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, "《"+n+"》")
	}
	return MsgComfyParamsMissing + strings.Join(parts, "、") + MsgComfyParamsHint
}

// ComfyDefaultValues 由配置生成内置占位符取值（可能在聊天里被 #参数名:值 覆盖）。
func ComfyDefaultValues(c ComfyConfig, prompt string) ComfyPlaceholderValues {
	return ComfyPlaceholderValues{
		PhComfySeed:      c.Seed,
		PhComfyHeight:    strconv.Itoa(c.Height),
		PhComfyWidth:     strconv.Itoa(c.Width),
		PhComfyNegative:  c.Negative,
		PhComfyPrompt:    prompt,
		PhComfyScheduler: c.Scheduler,
		PhComfySampler:   c.Sampler,
		PhComfyCFG:       sdNum(c.Cfg),
		PhComfySteps:     strconv.Itoa(c.Steps),
	}
}

// ComfyEffectiveSeed 解析种子文本：数字原样，"-1"/空 时用时间戳+随机后缀（0x100755F7 分支）。
func ComfyEffectiveSeed(raw string, now int64, rand3 func(int) int) string {
	v := strings.TrimSpace(raw)
	if v != "" && v != "-1" {
		return v
	}
	if rand3 == nil {
		return strconv.FormatInt(now, 10)
	}
	return strconv.FormatInt(now, 10) + strconv.Itoa(100+rand3(900))
}

// ComfyCooldownSeconds 解析"完成后冷却"（秒）。
func ComfyCooldownSeconds(raw string) int { return SdCooldownSeconds(raw) }
