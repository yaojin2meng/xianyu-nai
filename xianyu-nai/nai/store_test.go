package nai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nai预设列表.json")
	tg := filepath.Join(dir, "nai标签列表.json")
	s, err := LoadStore(p, tg)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.UploadPreset("二次元", "1girl, anime"); got != MsgPresetUploaded {
		t.Fatal(got)
	}
	if got := s.UploadPreset("二次元", "x"); got != MsgPresetExists {
		t.Fatal(got)
	}
	if got := s.UploadTag("风景", "scenery"); got != MsgTagUploaded {
		t.Fatal(got)
	}
	if got := s.UploadTag("风景", "x"); got != MsgTagExists {
		t.Fatal(got)
	}
	if err := s.SavePresets(p); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTags(tg); err != nil {
		t.Fatal(err)
	}
	s2, err := LoadStore(p, tg)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Presets) != 1 || s2.Presets[0].Name != "二次元" {
		t.Fatalf("%+v", s2.Presets)
	}
	if len(s2.Tags) != 1 || s2.Tags[0].Content != "scenery" {
		t.Fatalf("%+v", s2.Tags)
	}
	if got := s2.DeletePreset("不存在"); got != MsgPresetNotFound {
		t.Fatal(got)
	}
	if got := s2.DeletePreset("二次元"); got != MsgPresetDeleted {
		t.Fatal(got)
	}
	if got := s2.DeleteTag("风景"); got != MsgTagDeleted {
		t.Fatal(got)
	}
}

func TestPresetApply(t *testing.T) {
	presets := PresetFile{{Name: "二次元", Content: "anime, masterpiece"}}
	tags := PresetFile{{Name: "风景", Content: "scenery"}}
	ms := MatchPresets("画个二次元少女 风景", presets, tags)
	if len(ms) != 2 {
		t.Fatalf("%+v", ms)
	}
	out := ApplyPresets("画个二次元少女 风景", ms)
	if !strings.Contains(out, "anime, masterpiece") || !strings.Contains(out, "scenery") {
		t.Fatal(out)
	}
	if strings.Contains(out, "二次元") {
		t.Fatal("触发词没被去掉: " + out)
	}
	if r := PresetMatchReceipt(ms); !strings.Contains(r, "匹配预设：二次元") {
		t.Fatal(r)
	}
}

func TestStoreFilesReadable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	_ = os.WriteFile(p, []byte(`[{"名":"x","内容":"y"}]`), 0o644)
	s, err := LoadStore(p, filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Presets) != 1 || s.Presets[0].Content != "y" {
		t.Fatalf("%+v", s.Presets)
	}
}
