package nai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Subscription 对应 GET {naiofficialapi}/user/subscription 的返回（fcn.10068C0C）。
type Subscription struct {
	Tier      int    `json:"tier"`
	Active    bool   `json:"active"`
	ExpiresAt string `json:"expiresAt"`
	Grace     bool   `json:"isGracePeriod"`
	Training  struct {
		Fixed     int `json:"fixedTrainingStepsLeft"`
		Purchased int `json:"purchasedTrainingSteps"`
	} `json:"trainingStepsLeft"`
	Usage struct {
		Percent     float64 `json:"percent"`
		NextPercent float64 `json:"timeUntilNextPercent"`
		IsNegative  bool    `json:"isNegative"`
	} `json:"usage"`
}

// TierName 复刻 0x1006935A-0x1006943D 的数值→等级映射。
func (s Subscription) TierName() string {
	switch s.Tier {
	case 0:
		return "Paper"
	case 1:
		return "Tablet"
	case 2:
		return "Scroll"
	case 3:
		return "Opus"
	}
	return "未知"
}

// QuerySubscription 查询账号额度。
func (c *Client) QuerySubscription(ctx context.Context) (*Subscription, error) {
	if c.cfg.OfficialURL == "" {
		return nil, fmt.Errorf("当前未配置官方接口")
	}
	url := joinURL(c.cfg.OfficialURL, "/user/subscription", true)
	raw, err := c.getJSON(ctx, url, c.cfg.OfficialKey)
	if err != nil {
		return nil, fmt.Errorf("NAI 额度查询失败：%w", err)
	}
	var sub Subscription
	if err := json.Unmarshal(raw, &sub); err != nil {
		return nil, fmt.Errorf("NAI 额度查询失败：%v", err)
	}
	return &sub, nil
}

// Render 复刻插件额度回执的拼装（0x1006A5E3 起）。
func (s Subscription) Render() string {
	var b strings.Builder
	b.WriteString("===【NAI 额度信息】===\r\n")
	fmt.Fprintf(&b, "账号等级：%s\r\n", s.TierName())
	if s.ExpiresAt != "" {
		fmt.Fprintf(&b, "到期时间：%s\r\n", s.ExpiresAt)
	}
	if !s.Active {
		b.WriteString("账号状态：未激活\r\n")
	}
	if s.Grace {
		b.WriteString("账号状态：宽限期\r\n")
	}
	fmt.Fprintf(&b, "模块训练步数剩余：%d\r\n", s.Training.Fixed)
	if s.Training.Purchased > 0 {
		fmt.Fprintf(&b, "已购买 Anlas：%d\r\n", s.Training.Purchased)
	}
	if s.Usage.Percent <= 0 || s.Usage.IsNegative {
		b.WriteString("V5 额度：已耗尽\r\n")
		return b.String()
	}
	// 原生实现用 percent 与 86400 做换算（0x102BB208/0x102BB210 都是 86400.0）
	total := s.Usage.Percent / 100 * 86400
	fmt.Fprintf(&b, "V5 额度：剩余约 %d 张（%.2f%%）· 每日恢复约 %d 张\r\n",
		int(total), s.Usage.Percent, int(s.Usage.NextPercent/100*86400))
	return b.String()
}

func (c *Client) getJSON(ctx context.Context, url, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrLinkFailed
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
