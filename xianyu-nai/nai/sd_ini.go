package nai

import (
	"strconv"
	"strings"
)

// 本文件把 咸鱼ai画图.ini 的 [sd设置] / [sd参数] 两节读成 SDConfig。
//
// 证据（默认值写入段 0x1009AC6A-0x1009AF00，全部是 读配置项(ini, 节, 键, 默认)）：
//
//	[sd设置] 链接 / 账号 / 密码
//	[sd参数] steps=30 识别率=4 差异=0.5 高度=1472 宽度=896 采样="Euler a"
//	         负面词条=<长默认串 0x1039AD40> 高清修复算法=Lanczos 默认种子=-1
//	         全局默认模型 完成后冷却=0 完成冷却模式
//
// 另外 高清修复 不在默认写入列表里，但出图时按 读配置项(ini,"sd参数","高清修复","0")
// 读取（0x100239F0），因此这里也读，默认 "0"。

// IniKeysSD 是 SD 两节用到的键名（逐字来自 DLL）。
const (
	FileMainINI = "咸鱼ai画图.ini" // 0x102B5AEC

	IniSectionSDAPI   = "sd设置" // 0x1039B034
	IniKeySDURL       = "链接"
	IniKeySDUser      = "账号"   // 0x1039B02C
	IniKeySDPass      = "密码"   // 0x1039B03C
	IniSectionSDParam = "sd参数" // 0x102B5BCC

	IniKeySDSteps    = "steps"
	IniKeySDIdentity = "识别率"    // 0x1039AD1C
	IniKeySDDiff     = "差异"     // 0x1039AD24
	IniKeySDHeight   = "高度"     // 0x1039AD2C
	IniKeySDWidth    = "宽度"     // 0x1039AD34
	IniKeySDSampler  = "采样"     // 0x1039AD38
	IniKeySDNegative = "负面词条"   // 0x1039AC38
	IniKeySDHiresUp  = "高清修复算法" // 0x1039B01C
	IniKeySDSeed     = "默认种子"   // 0x1039AC44
	IniKeySDHires    = "高清修复"
	IniKeySDModel    = "全局默认模型" // 0x1039AC8C
	IniKeySDCooldown = "完成后冷却"  // 0x1039AC9C
	IniKeySDCoolMode = "完成冷却模式" // 0x1039ACB0
)

// LoadSDConfigFromINI 读 [sd设置]/[sd参数]；文件缺失时原样返回 base。
func LoadSDConfigFromINI(path string, base SDConfig) SDConfig {
	out := base
	f := LoadIni(path)
	if f.Path == "" {
		return out
	}

	out.URL = f.Get(IniSectionSDAPI, IniKeySDURL, out.URL)
	out.User = f.Get(IniSectionSDAPI, IniKeySDUser, out.User)
	out.Pass = f.Get(IniSectionSDAPI, IniKeySDPass, out.Pass)

	out.Steps = iniInt(f, IniSectionSDParam, IniKeySDSteps, out.Steps)
	out.Identity = iniFloat(f, IniSectionSDParam, IniKeySDIdentity, out.Identity)
	out.Diff = iniFloat(f, IniSectionSDParam, IniKeySDDiff, out.Diff)
	out.Height = iniInt(f, IniSectionSDParam, IniKeySDHeight, out.Height)
	out.Width = iniInt(f, IniSectionSDParam, IniKeySDWidth, out.Width)
	out.Sampler = f.Get(IniSectionSDParam, IniKeySDSampler, out.Sampler)
	out.Negative = f.Get(IniSectionSDParam, IniKeySDNegative, out.Negative)
	out.HiresUp = f.Get(IniSectionSDParam, IniKeySDHiresUp, out.HiresUp)
	out.Seed = f.Get(IniSectionSDParam, IniKeySDSeed, out.Seed)
	out.GlobalModel = f.Get(IniSectionSDParam, IniKeySDModel, out.GlobalModel)
	out.Cooldown = f.Get(IniSectionSDParam, IniKeySDCooldown, out.Cooldown)
	out.CooldownMode = f.Get(IniSectionSDParam, IniKeySDCoolMode, out.CooldownMode)
	out.Hires = strings.TrimSpace(f.Get(IniSectionSDParam, IniKeySDHires, "0")) == "1"
	return out
}

// SaveSDConfigToINI 把 SD 设置写回 ini（供首次运行补默认值）。
// 只写显式给出的项；空串不覆盖。
func SaveSDConfigToINI(path string, c SDConfig) error {
	f := LoadIni(path)
	if c.URL != "" {
		f.Set(IniSectionSDAPI, IniKeySDURL, c.URL)
	}
	if c.User != "" {
		f.Set(IniSectionSDAPI, IniKeySDUser, c.User)
	}
	if c.Pass != "" {
		f.Set(IniSectionSDAPI, IniKeySDPass, c.Pass)
	}
	if c.Steps > 0 {
		f.Set(IniSectionSDParam, IniKeySDSteps, strconv.Itoa(c.Steps))
	}
	if c.Identity > 0 {
		f.Set(IniSectionSDParam, IniKeySDIdentity, sdNum(c.Identity))
	}
	if c.Diff > 0 {
		f.Set(IniSectionSDParam, IniKeySDDiff, sdNum(c.Diff))
	}
	if c.Width > 0 {
		f.Set(IniSectionSDParam, IniKeySDWidth, strconv.Itoa(c.Width))
	}
	if c.Height > 0 {
		f.Set(IniSectionSDParam, IniKeySDHeight, strconv.Itoa(c.Height))
	}
	if c.Sampler != "" {
		f.Set(IniSectionSDParam, IniKeySDSampler, c.Sampler)
	}
	if c.Negative != "" {
		f.Set(IniSectionSDParam, IniKeySDNegative, c.Negative)
	}
	if c.HiresUp != "" {
		f.Set(IniSectionSDParam, IniKeySDHiresUp, c.HiresUp)
	}
	if c.Seed != "" {
		f.Set(IniSectionSDParam, IniKeySDSeed, c.Seed)
	}
	if c.GlobalModel != "" {
		f.Set(IniSectionSDParam, IniKeySDModel, c.GlobalModel)
	}
	if c.Cooldown != "" {
		f.Set(IniSectionSDParam, IniKeySDCooldown, c.Cooldown)
	}
	if c.CooldownMode != "" {
		f.Set(IniSectionSDParam, IniKeySDCoolMode, c.CooldownMode)
	}
	if c.Hires {
		f.Set(IniSectionSDParam, IniKeySDHires, "1")
	}
	return f.Save()
}

func iniInt(f *IniFile, sec, key string, def int) int {
	v := strings.TrimSpace(f.Get(sec, key, ""))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func iniFloat(f *IniFile, sec, key string, def float64) float64 {
	v := strings.TrimSpace(f.Get(sec, key, ""))
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
