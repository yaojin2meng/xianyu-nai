package nai

import (
	"os"
	"sort"
	"strings"
)

// 本文件是极简 ini 读写，对应插件的 读配置项(0x101CA360) / 写配置项。
//
// 插件用到的 ini 文件（VA）：
//
//	咸鱼ai画图.ini   0x102B5AEC   主配置
//	Negative.ini     0x102B7CD8   每用户负面词条（节 "sd" / "nai"）
//	seed.ini         0x102B7CBC   每用户种子（节 "sd" / "nai"）
//
// 注意：易语言的 写配置项 在中文 Windows 上落盘是 ANSI（GBK）。
// 重制版统一按 UTF-8 读写；文件内容语义一致，编码不同已在文档标注。

// IniFile 是一份内存里的 ini。
type IniFile struct {
	Path string
	Sec  map[string]map[string]string
	// Ordered 用来稳定写回顺序：节名 -> 键名序列。
	Order map[string][]string
}

// LoadIni 读取 ini；文件不存在或读失败时返回空表（与插件首次运行一致）。
func LoadIni(path string) *IniFile {
	f := &IniFile{Path: path, Sec: map[string]map[string]string{}, Order: map[string][]string{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	f.parse(string(b))
	return f
}

func (f *IniFile) parse(s string) {
	cur := ""
	// 去 BOM
	s = strings.TrimPrefix(s, "\ufeff")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			cur = strings.TrimSpace(t[1 : len(t)-1])
			f.ensure(cur)
			continue
		}
		if i := strings.IndexByte(line, '='); i >= 0 {
			if cur == "" {
				continue
			}
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			f.ensure(cur)
			if _, ok := f.Sec[cur][k]; !ok {
				f.Order[cur] = append(f.Order[cur], k)
			}
			f.Sec[cur][k] = v
		}
	}
}

func (f *IniFile) ensure(sec string) {
	if f.Sec[sec] == nil {
		f.Sec[sec] = map[string]string{}
	}
}

// Get 读配置项：节/键不存在时返回 def。
func (f *IniFile) Get(sec, key, def string) string {
	if v, ok := f.Sec[sec][key]; ok && v != "" {
		return v
	}
	return def
}

// Has 报告节/键是否已存在且非空。
func (f *IniFile) Has(sec, key string) bool {
	if f.Sec == nil {
		return false
	}
	v, ok := f.Sec[sec][key]
	return ok && v != ""
}

// Set 写配置项（只在内存里）。
func (f *IniFile) Set(sec, key, val string) {
	f.ensure(sec)
	if _, ok := f.Sec[sec][key]; !ok {
		f.Order[sec] = append(f.Order[sec], key)
	}
	f.Sec[sec][key] = val
}

// Save 写回文件。
func (f *IniFile) Save() error {
	if f.Path == "" {
		return nil
	}
	var b strings.Builder
	secs := make([]string, 0, len(f.Sec))
	for s := range f.Sec {
		secs = append(secs, s)
	}
	sort.Strings(secs)
	for _, s := range secs {
		b.WriteString("[" + s + "]\r\n")
		keys := f.Order[s]
		seen := map[string]bool{}
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			b.WriteString(k + "=" + f.Sec[s][k] + "\r\n")
		}
		// 兜底：不在 Order 里的键
		var extra []string
		for k := range f.Sec[s] {
			if !seen[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, k := range extra {
			b.WriteString(k + "=" + f.Sec[s][k] + "\r\n")
		}
	}
	return os.WriteFile(f.Path, []byte(b.String()), 0o644)
}
