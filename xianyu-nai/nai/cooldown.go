package nai

import (
	"strconv"
	"strings"
	"sync"
)

// 本文件实现插件三条画图通道共用的"完成后冷却"。
//
// 证据：
//
//	0x102B7D58 = "画图冷却中，还需等待 "   —— 三条通道共用同一句回执
//	0x1039AC9C = "完成后冷却"             —— sd参数 / comfy参数 同名键
//	0x1039ACB0 = "完成冷却模式"           —— 取值 个人 / 群
//	0x1039ACA8 = "个人"                   —— 默认值
//
// 原插件把"上次出图时间"放在进程内的全局表里（不落盘），所以重制版也用内存表，
// 进程重启即清零。剩余秒数的具体拼接（"…还需等待 <n> 秒"）中 <n> 的写法
// 未在反汇编里逐字确认，这里用十进制整数秒。

// MsgCooldownWait 是冷却回执前缀（0x102B7D58）。
const MsgCooldownWait = "画图冷却中，还需等待 "

// MsgCooldownTail 是冷却回执后缀（措辞推定）。
const MsgCooldownTail = " 秒"

// CooldownUser 是"完成冷却模式"的两个取值。
const (
	CooldownPerUser  = "个人"
	CooldownPerGroup = "群"
)

// cooldownBook 记录每个 (通道, 用户/群) 的历史出图时间。
type cooldownBook struct {
	mu   sync.Mutex
	last map[string]int64
}

func newCooldownBook() *cooldownBook {
	return &cooldownBook{last: map[string]int64{}}
}

// Check 返回还需等待的秒数（0 表示可以出图）。
func (b *cooldownBook) Check(scope, key string, seconds int) int {
	if b == nil || seconds <= 0 || key == "" {
		return 0
	}
	now := NowUnix()
	b.mu.Lock()
	defer b.mu.Unlock()
	prev, ok := b.last[scope+"|"+key]
	if !ok {
		return 0
	}
	left := seconds - int(now-prev)
	if left < 0 {
		return 0
	}
	return left
}

// Touch 记一次出图时间。
func (b *cooldownBook) Touch(scope, key string) {
	if b == nil || key == "" {
		return
	}
	b.mu.Lock()
	b.last[scope+"|"+key] = NowUnix()
	b.mu.Unlock()
}

// CooldownKey 按"完成冷却模式"算冷却键：个人 -> QQ，群 -> 群号。
func CooldownKey(mode, qq, group string) string {
	if strings.TrimSpace(mode) == CooldownPerGroup {
		if strings.TrimSpace(group) != "" {
			return "g" + group
		}
	}
	if strings.TrimSpace(qq) == "" {
		return ""
	}
	return "u" + qq
}

// RenderCooldown 拼冷却回执。
func RenderCooldown(left int) string {
	if left < 0 {
		left = 0
	}
	return MsgCooldownWait + strconv.Itoa(left) + MsgCooldownTail
}
