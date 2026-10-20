package nai

import (
	"strconv"
	"strings"
)

// 本文件把 咸鱼ai画图.ini 里的 NAI 配置读进 Config，
// 节名/键名全部来自对 unpacked.dll 的“读配置项”调用点实测：
//
//	0x100A3EE1  nai3设置 / key
//	0x100A3F96  nai3设置 / 官方key
//	0x100A4051  nai3设置 / 模式
//	0x100A410C  nai3参数 / scale
//	0x100A41C7  nai3参数 / steps
//	0x100A4282  nai3参数 / cfg
//	0x100A433D  nai3参数 / strength
//	0x100A43F8  nai3参数 / 默认种子
//	0x100A44B3  nai3参数 / 宽度
//	0x100A456E  nai3参数 / 高度
//	0x100A4629  nai3参数 / 采样
//	0x100A46E4  nai3参数 / 噪声
//	0x100A479F  nai3参数 / sm
//	0x100A485A  nai3参数 / sm_dyn
//	0x100A4915  nai3参数 / 全局默认模型
//	0x100A49D0  nai3参数 / 负面词条
//	0x100A4A8B  nai3参数 / 完成后冷却
//	0x100A4B46  nai3参数 / 完成冷却模式
//	0x100A4C01  nai3指令 / 画图
//	0x100A4CBC  nai3指令 / 修改种子
//	0x100A4D77  nai3指令 / 修改负面
//	0x100A4E32  nai3指令 / 查看负面
//	0x100A4EED  nai3指令 / 情绪修改
//	0x100A4FA8  nai3指令 / 转线稿
//	0x100A5063  nai3指令 / 转草稿
//	0x100A511E  nai3指令 / 图像清理
//	0x100A51D9  nai3指令 / 着色
//	0x100A5294  nai3指令 / 图像解析
//	0x100A534F  nai3指令 / 查看预设
//	0x100A540A  nai3指令 / 查看模型
//	0x100A54C5  nai3指令 / 查询额度
//	0x100A5580  nai3指令 / 上传标签
//	0x100A563B  nai3指令 / 上传预设
//	0x100A56F6  nai3指令 / 查看标签
//	0x100A57B1  nai3指令 / 删除标签
//	0x100A586C  nai3指令 / 删除预设
//
// [重制版] 节是新增的：原插件里 nai3api / naiofficialapi / nai3_plugin-api /
// 资源地址 / upload-image 由授权服务器下发（见 0x1005E4B7 取 nai3api），
// 重制版没有下发通道，必须由用户填写并持久化。
//
// 指令键在这里用于覆盖 commands.go 里的默认指令词：匹配时优先用 ini 值。

// NAI 配置节与键名。
const (
	IniSectionNAIAPI   = "nai3设置" // 0x102B5C14
	IniSectionNAIParam = "nai3参数" // 0x102B5B78
	IniSectionNAICmd   = "nai3指令" // 0x102B5D6C

	IniKeyNAIKey         = "key"
	IniKeyNAIOfficialKey = "官方key"
	IniKeyNAIMode        = "模式"

	IniKeyNAIScale     = "scale"
	IniKeyNAISteps     = "steps"
	IniKeyNAICfg       = "cfg"
	IniKeyNAIStrength  = "strength"
	IniKeyNAISeed      = "默认种子"
	IniKeyNAIWidth     = "宽度"
	IniKeyNAIHeight    = "高度"
	IniKeyNAISampler   = "采样"
	IniKeyNAINoise     = "噪声"
	IniKeyNAISmea      = "sm"
	IniKeyNAIDyn       = "sm_dyn"
	IniKeyNAIModel     = "全局默认模型"
	IniKeyNAINegative  = "负面词条"
	IniKeyNAICooldown  = "完成后冷却"
	IniKeyNAICoolMode  = "完成冷却模式"
	IniSectionRemake   = "重制版"
	IniKeyRemakeProxy  = "nai3api"
	IniKeyRemakeOffic  = "naiofficialapi"
	IniKeyRemakePlugin = "nai3_plugin-api"
	IniKeyRemakeRes    = "资源地址"
	IniKeyRemakeUpload = "upload-image"
)

// LoadNAIConfigFromINI 用 ini 覆盖 base 中的 NAI 字段。
// ini 里没有的键保留 base 的值（与原插件“读配置项带默认值”一致）。
func LoadNAIConfigFromINI(path string, base Config) Config {
	f := LoadIni(path)
	if f.Path == "" {
		return base
	}
	out := base

	out.Key = f.Get(IniSectionNAIAPI, IniKeyNAIKey, out.Key)
	out.OfficialKey = f.Get(IniSectionNAIAPI, IniKeyNAIOfficialKey, out.OfficialKey)
	out.Mode = f.Get(IniSectionNAIAPI, IniKeyNAIMode, out.Mode)

	out.Scale = iniFloat(f, IniSectionNAIParam, IniKeyNAIScale, out.Scale)
	out.Steps = iniInt(f, IniSectionNAIParam, IniKeyNAISteps, out.Steps)
	out.Cfg = iniFloat(f, IniSectionNAIParam, IniKeyNAICfg, out.Cfg)
	out.Strength = iniFloat(f, IniSectionNAIParam, IniKeyNAIStrength, out.Strength)
	out.Width = iniInt(f, IniSectionNAIParam, IniKeyNAIWidth, out.Width)
	out.Height = iniInt(f, IniSectionNAIParam, IniKeyNAIHeight, out.Height)
	out.Sampler = f.Get(IniSectionNAIParam, IniKeyNAISampler, out.Sampler)
	out.Noise = f.Get(IniSectionNAIParam, IniKeyNAINoise, out.Noise)
	out.Smea = strings.TrimSpace(f.Get(IniSectionNAIParam, IniKeyNAISmea, boolStr(out.Smea))) == "1"
	out.Dyn = strings.TrimSpace(f.Get(IniSectionNAIParam, IniKeyNAIDyn, boolStr(out.Dyn))) == "1"
	out.Model = f.Get(IniSectionNAIParam, IniKeyNAIModel, out.Model)
	out.Negative = f.Get(IniSectionNAIParam, IniKeyNAINegative, out.Negative)

	// 默认种子：ini 里是文本，"-1" 表示随机
	if s := strings.TrimSpace(f.Get(IniSectionNAIParam, IniKeyNAISeed, "")); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			out.Seed = v
		}
	}

	// [重制版]：地址类（原为服务端下发）
	out.ProxyURL = f.Get(IniSectionRemake, IniKeyRemakeProxy, out.ProxyURL)
	out.OfficialURL = f.Get(IniSectionRemake, IniKeyRemakeOffic, out.OfficialURL)
	out.PluginAPI = f.Get(IniSectionRemake, IniKeyRemakePlugin, out.PluginAPI)
	out.ResourceURL = f.Get(IniSectionRemake, IniKeyRemakeRes, out.ResourceURL)
	out.UploadImage = f.Get(IniSectionRemake, IniKeyRemakeUpload, out.UploadImage)
	return out
}

// SaveNAIConfigToINI 把 NAI 配置写回 ini（只写非空值）。
func SaveNAIConfigToINI(path string, c Config) error {
	f := LoadIni(path)

	// 只有当内存默认值不是 DLL 默认值时才写，避免把默认值刷进 ini。
	d := DefaultConfig()

	if c.Key != "" {
		f.Set(IniSectionNAIAPI, IniKeyNAIKey, c.Key)
	}
	if c.OfficialKey != "" {
		f.Set(IniSectionNAIAPI, IniKeyNAIOfficialKey, c.OfficialKey)
	}
	if c.Mode != "" {
		f.Set(IniSectionNAIAPI, IniKeyNAIMode, c.Mode)
	}
	if c.Scale != 0 {
		f.Set(IniSectionNAIParam, IniKeyNAIScale, sdNum(c.Scale))
	}
	if c.Steps != 0 {
		f.Set(IniSectionNAIParam, IniKeyNAISteps, strconv.Itoa(c.Steps))
	}
	if c.Cfg != 0 {
		f.Set(IniSectionNAIParam, IniKeyNAICfg, sdNum(c.Cfg))
	}
	if c.Strength != 0 {
		f.Set(IniSectionNAIParam, IniKeyNAIStrength, sdNum(c.Strength))
	}
	f.Set(IniSectionNAIParam, IniKeyNAISeed, strconv.FormatInt(c.Seed, 10))
	if c.Width != 0 {
		f.Set(IniSectionNAIParam, IniKeyNAIWidth, strconv.Itoa(c.Width))
	}
	if c.Height != 0 {
		f.Set(IniSectionNAIParam, IniKeyNAIHeight, strconv.Itoa(c.Height))
	}
	if c.Sampler != "" {
		f.Set(IniSectionNAIParam, IniKeyNAISampler, c.Sampler)
	}
	if c.Noise != "" {
		f.Set(IniSectionNAIParam, IniKeyNAINoise, c.Noise)
	}
	f.Set(IniSectionNAIParam, IniKeyNAISmea, boolStr(c.Smea))
	f.Set(IniSectionNAIParam, IniKeyNAIDyn, boolStr(c.Dyn))
	if c.Model != "" && c.Model != d.Model {
		f.Set(IniSectionNAIParam, IniKeyNAIModel, c.Model)
	}
	if c.Negative != "" && c.Negative != d.Negative {
		f.Set(IniSectionNAIParam, IniKeyNAINegative, c.Negative)
	}

	if c.ProxyURL != "" {
		f.Set(IniSectionRemake, IniKeyRemakeProxy, c.ProxyURL)
	}
	if c.OfficialURL != "" {
		f.Set(IniSectionRemake, IniKeyRemakeOffic, c.OfficialURL)
	}
	if c.PluginAPI != "" {
		f.Set(IniSectionRemake, IniKeyRemakePlugin, c.PluginAPI)
	}
	if c.ResourceURL != "" {
		f.Set(IniSectionRemake, IniKeyRemakeRes, c.ResourceURL)
	}
	if c.UploadImage != "" {
		f.Set(IniSectionRemake, IniKeyRemakeUpload, c.UploadImage)
	}
	return f.Save()
}

// NAICmdOverride 返回 ini 里配置的 NAI 指令词；
// 未配置（空）时返回 fallback。
func NAICmdOverride(f *IniFile, key, fallback string) string {
	if f == nil {
		return fallback
	}
	v := strings.TrimSpace(f.Get(IniSectionNAICmd, key, ""))
	if v == "" {
		return fallback
	}
	return v
}

// ---------------------------------------------------------------------------
// [基本] / [点数限制] / [点数消耗] / [图像审核] / [黑名单功能] / [类型]
// ---------------------------------------------------------------------------

// 通用配置节与键名（逐条来自“读配置项”调用点实测）。
const (
	IniSectionBasic  = "基本"    // 0x102B5BA4
	IniSectionTypes  = "类型"    // 0x10020DD9
	IniSectionAudit  = "图像审核"  // 0x10025978
	IniSectionPoints = "点数限制"  // 0x10010529
	IniSectionCost   = "点数消耗"  // 0x10097FE5
	IniSectionBLFunc = "黑名单功能" // 0x100A2490

	IniKeyBasicOwner    = "主人"
	IniKeyBasicBotQQ    = "机器人QQ"
	IniKeyBasicSendMode = "发送模式"
	IniKeyBasicQuote    = "是否引用"
	IniKeyBasicBotTalk  = "官机互动"
	IniKeyBasicUpload   = "上传图像"
	IniKeyBasicAIToken  = "AI翻译令牌"
	IniKeyBasicBlock    = "屏蔽词条"

	IniKeyTypeTranslate = "翻译接口"
	IniKeyTypeGirl      = "今日少女接口"
	IniKeyTypeSD        = "sd接口"

	IniKeyPointsSwitch   = "开关"
	IniKeyPointsBase     = "基础点数"
	IniKeyPointsType     = "点数处理类型"
	IniKeyPointsTip      = "点数不足提示"
	IniKeyPointsRecharge = "充值点数指令"

	IniKeyCostSD    = "sd画图"
	IniKeyCostNai   = "nai画图"
	IniKeyCostComfy = "comfy画图"

	IniKeyAuditPlatform  = "平台"
	IniKeyAuditSwitch    = "开关"
	IniKeyAuditThreshold = "阈值"
	IniKeyAuditAction    = "处理"

	IniKeyBLSd     = "sd画图"
	IniKeyBLNai    = "nai画图"
	IniKeyBLUpscal = "清晰术"
	IniKeyBLRmBg   = "去背景"
	IniKeyBLTag    = "上传标签"
	IniKeyBLPreset = "上传预设"
)

// iniBool 读取开关型键：“1”/“开启”/“是” 视为真。
func iniBool(f *IniFile, sec, key string, def bool) bool {
	raw := strings.TrimSpace(f.Get(sec, key, ""))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "开启", "是", "真", "true", "True", "on", "ON":
		return true
	}
	return false
}

// LoadHomeConfigFromINI 从 ini 读入通用配置与门禁参数。
func LoadHomeConfigFromINI(path string, base Config) Config {
	f := LoadIni(path)
	if f.Path == "" {
		return base
	}
	out := base

	out.SendMode = f.Get(IniSectionBasic, IniKeyBasicSendMode, out.SendMode)
	out.AITranslateToken = f.Get(IniSectionBasic, IniKeyBasicAIToken, out.AITranslateToken)
	out.BlockWords = f.Get(IniSectionBasic, IniKeyBasicBlock, out.BlockWords)
	out.TranslateAPI = f.Get(IniSectionTypes, IniKeyTypeTranslate, out.TranslateAPI)

	if s := strings.TrimSpace(f.Get(IniSectionPoints, IniKeyPointsBase, "")); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			out.BasePoints = v
		}
	}
	out.PointsOn = iniBool(f, IniSectionPoints, IniKeyPointsSwitch, out.PointsOn)
	out.PointsType = f.Get(IniSectionPoints, IniKeyPointsType, out.PointsType)
	out.PointsTip = f.Get(IniSectionPoints, IniKeyPointsTip, out.PointsTip)

	out.CostSD = iniInt64(f, IniSectionCost, IniKeyCostSD, out.CostSD)
	out.CostNai = iniInt64(f, IniSectionCost, IniKeyCostNai, out.CostNai)
	out.CostComfy = iniInt64(f, IniSectionCost, IniKeyCostComfy, out.CostComfy)

	out.AuditOn = iniBool(f, IniSectionAudit, IniKeyAuditSwitch, out.AuditOn)
	out.AuditPlat = f.Get(IniSectionAudit, IniKeyAuditPlatform, out.AuditPlat)
	out.AuditTh = f.Get(IniSectionAudit, IniKeyAuditThreshold, out.AuditTh)
	out.AuditAct = f.Get(IniSectionAudit, IniKeyAuditAction, out.AuditAct)

	out.BLExemptSD = f.Get(IniSectionBLFunc, IniKeyBLSd, out.BLExemptSD)
	out.BLExemptNai = f.Get(IniSectionBLFunc, IniKeyBLNai, out.BLExemptNai)
	out.BLExemptUpscal = f.Get(IniSectionBLFunc, IniKeyBLUpscal, out.BLExemptUpscal)
	out.BLExemptRmBg = f.Get(IniSectionBLFunc, IniKeyBLRmBg, out.BLExemptRmBg)
	out.BLExemptTag = f.Get(IniSectionBLFunc, IniKeyBLTag, out.BLExemptTag)
	out.BLExemptPreset = f.Get(IniSectionBLFunc, IniKeyBLPreset, out.BLExemptPreset)

	return out
}

// iniInt64 是 iniInt 的 int64 版本。
func iniInt64(f *IniFile, sec, key string, def int64) int64 {
	s := strings.TrimSpace(f.Get(sec, key, ""))
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// boolStr 把布尔值写成 ini 里的 "1"/"0"。
func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
