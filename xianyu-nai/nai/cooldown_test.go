package nai

import "testing"

func TestCooldownBook(t *testing.T) {
	b := newCooldownBook()
	if got := b.Check("sd", "u10001", 30); got != 0 {
		t.Fatalf("首次应为 0，得到 %d", got)
	}
	b.Touch("sd", "u10001")
	if got := b.Check("sd", "u10001", 30); got <= 0 || got > 30 {
		t.Fatalf("冷却中应 >0，得到 %d", got)
	}
	// 不同通道互不影响
	if got := b.Check("comfy", "u10001", 30); got != 0 {
		t.Fatalf("通道隔离失败，得到 %d", got)
	}
	// 秒数为 0 视为不冷却
	if got := b.Check("sd", "u10001", 0); got != 0 {
		t.Fatalf("0 秒应不冷却，得到 %d", got)
	}
	// 另一个人不受影响
	if got := b.Check("sd", "u20002", 30); got != 0 {
		t.Fatalf("用户隔离失败，得到 %d", got)
	}
}

func TestCooldownKey(t *testing.T) {
	if got := CooldownKey(CooldownPerUser, "10001", "88888"); got != "u10001" {
		t.Fatalf("个人模式键错: %q", got)
	}
	if got := CooldownKey(CooldownPerGroup, "10001", "88888"); got != "g88888" {
		t.Fatalf("群模式键错: %q", got)
	}
	// 群模式但没群号 -> 退回个人
	if got := CooldownKey(CooldownPerGroup, "10001", ""); got != "u10001" {
		t.Fatalf("群模式无群号应退回个人: %q", got)
	}
	if got := CooldownKey(CooldownPerUser, "", ""); got != "" {
		t.Fatalf("无键应为空: %q", got)
	}
}

func TestRenderCooldown(t *testing.T) {
	want := "画图冷却中，还需等待 12 秒"
	if got := RenderCooldown(12); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := RenderCooldown(-3); got != "画图冷却中，还需等待 0 秒" {
		t.Fatalf("负值应夹到 0: %q", got)
	}
}

// 冷却值解析与 SD 一致（comfy参数/完成后冷却 与 sd参数/完成后冷却 同名同义）。
func TestComfyCooldownSeconds(t *testing.T) {
	cases := map[string]int{
		"":    0,
		"0":   0,
		"-5":  0,
		"30":  30,
		" 45": 45,
		"abc": 0,
	}
	for in, want := range cases {
		if got := ComfyCooldownSeconds(in); got != want {
			t.Fatalf("%q -> %d want %d", in, got, want)
		}
	}
}
