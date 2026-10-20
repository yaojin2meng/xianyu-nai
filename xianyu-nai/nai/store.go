package nai

import (
	"encoding/json"
	"os"
	"strings"
)

// 本文件复刻预设/标签的增删查，含真实文案。
//
// 文案（VA 0x102ba050 起）：
//
//	0x102ba050 "[nai预设]已存在相同预设，请修改后上传"
//	0x102ba08c "# [nai预设]已上传预设内容"
//	0x102ba0a7 "# 预设名："
//	0x102ba0b4 "nai上传预设"
//	0x102ba0c0 "[nai预设]已上传预设内容"
//	0x102ba0d8 "已存在相同标签，请修改后上传"
//	0x102ba10c "# 已上传标签内容"
//	0x102ba11e "# 标签："
//	0x102ba128 "nai上传标签"
//	0x102ba134 "已上传标签内容"
//	0x102baf90 "[nai预设]已删除该预设内容"
//	0x102bafac "[nai预设]未找到该预设内容"
//	0x102bafc8 "[nai标签]已删除该标签内容"
//	0x102bafe4 "[nai标签]未找到该标签内容"
//	0x102ba014 "nai标签列表"
//	0x102ba020 "标签列表(指令+标签名查看提示词)"
//	0x102ba041 "=========="
//
// 文件：nai预设列表.json / nai标签列表.json（0x102ba078 / 0x102ba0f8）。

// 预设/标签操作文案。
const (
	MsgPresetExists   = "[nai预设]已存在相同预设，请修改后上传" // 0x102ba050
	MsgPresetUploaded = "[nai预设]已上传预设内容"        // 0x102ba0c0
	MsgPresetDeleted  = "[nai预设]已删除该预设内容"       // 0x102baf90
	MsgPresetNotFound = "[nai预设]未找到该预设内容"       // 0x102bafac
	MsgTagExists      = "已存在相同标签，请修改后上传"        // 0x102ba0d8
	MsgTagUploaded    = "已上传标签内容"               // 0x102ba134
	MsgTagDeleted     = "[nai标签]已删除该标签内容"       // 0x102bafc8
	MsgTagNotFound    = "[nai标签]未找到该标签内容"       // 0x102bafe4
	MsgTagListTitle   = "标签列表(指令+标签名查看提示词)"     // 0x102ba020
)

// 文件常量（VA）。
const (
	FileNaiPresetJSON  = "nai预设列表.json"  // 0x102ba078
	FileNaiTagJSON     = "nai标签列表.json"  // 0x102ba0f8
	FileNaiPresetINI   = "nai预设列表.ini"   // 0x1039a96c
	FileNaiTagINI      = "nai标签列表.ini"   // 0x1039a97c
	FileNai3PresetJSON = "nai3预设列表.json" // 0x1039a92c
	FileNai3TagJSON    = "nai3标签列表.json" // 0x1039a940
)

// Store 是预设/标签的落盘容器。
type Store struct {
	Path    string
	Presets PresetFile
	Tags    PresetFile
}

// LoadStore 从两个 JSON 文件读入。文件不存在时按空处理（插件首次运行也是空）。
func LoadStore(presetPath, tagPath string) (*Store, error) {
	s := &Store{Path: presetPath}
	if b, err := os.ReadFile(presetPath); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &s.Presets); err != nil {
			return nil, err
		}
	}
	if b, err := os.ReadFile(tagPath); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &s.Tags); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// SavePresets 写回预设文件。
func (s *Store) SavePresets(path string) error {
	b, err := json.MarshalIndent(s.Presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// SaveTags 写回标签文件。
func (s *Store) SaveTags(path string) error {
	b, err := json.MarshalIndent(s.Tags, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// UploadPreset 添加预设，重名返回 MsgPresetExists。
func (s *Store) UploadPreset(name, content string) string {
	if _, ok := s.Presets.Find(name); ok {
		return MsgPresetExists
	}
	s.Presets = append(s.Presets, Preset{Name: name, Content: content})
	return MsgPresetUploaded
}

// UploadTag 添加标签。
func (s *Store) UploadTag(name, content string) string {
	if _, ok := s.Tags.Find(name); ok {
		return MsgTagExists
	}
	s.Tags = append(s.Tags, Preset{Name: name, Content: content})
	return MsgTagUploaded
}

// DeletePreset 删除预设。
func (s *Store) DeletePreset(name string) string {
	if s.Presets.Delete(name) {
		return MsgPresetDeleted
	}
	return MsgPresetNotFound
}

// DeleteTag 删除标签。
func (s *Store) DeleteTag(name string) string {
	if s.Tags.Delete(name) {
		return MsgTagDeleted
	}
	return MsgTagNotFound
}

// RenderPresetList 复刻"nai查看预设"的回执。
// 用 0x102ba041 的 "==========" 做分隔线。
func (s *Store) RenderPresetList() string {
	var b strings.Builder
	b.WriteString("==========\r\n")
	for i, p := range s.Presets {
		b.WriteString(p.Name)
		b.WriteString("\r\n")
		if p.Content != "" {
			b.WriteString(p.Content)
			b.WriteString("\r\n")
		}
		if i != len(s.Presets)-1 {
			b.WriteString("==========\r\n")
		}
	}
	return b.String()
}

// RenderTagList 复刻"nai查看标签"的回执。
func (s *Store) RenderTagList() string {
	var b strings.Builder
	b.WriteString("==========\r\n")
	b.WriteString(MsgTagListTitle)
	b.WriteString("\r\n==========\r\n")
	for _, t := range s.Tags {
		b.WriteString(t.Name)
		b.WriteString("\r\n")
		if t.Content != "" {
			b.WriteString(t.Content)
			b.WriteString("\r\n")
		}
	}
	return b.String()
}
