package pricewatch

import (
	"fmt"
	"math"
	"time"
)

// NotifyPolicy 控制同一档内的提醒频率（spec §5）。
type NotifyPolicy struct {
	RepeatInterval time.Duration `mapstructure:"repeat_interval"`
	MaxPerTier     int           `mapstructure:"max_per_tier"`
}

// RuleCfg 是配置文件中一条规则的原始形态，经 ParseRule 校验后得到 Rule。
type RuleCfg struct {
	ID, Type, Window string
	ThresholdPct     float64 `mapstructure:"threshold_pct"`
	StepPct          float64 `mapstructure:"step_pct"`
	Notify           *NotifyPolicy
}

// RuleType 是规则类型（spec §4）。
type RuleType string

const (
	DropFromHigh RuleType = "drop_from_high"
	RiseFromLow  RuleType = "rise_from_low"
)

// Window 是滚动窗口长度。
type Window string

const (
	Window24h Window = "24h"
	Window7d  Window = "7d"
	Window30d Window = "30d"
	Window4y  Window = "4y"
	WindowAll Window = "all"
)

// Span 返回窗口长度；all 返回 0，表示不设起点。
func (w Window) Span() time.Duration {
	switch w {
	case Window24h:
		return 24 * time.Hour
	case Window7d:
		return 7 * 24 * time.Hour
	case Window30d:
		return 30 * 24 * time.Hour
	case Window4y:
		return 1461 * 24 * time.Hour
	}
	return 0
}

// Bar 返回计算该窗口极值所用的 K 线周期：24h、7d 用 5 分钟线，其余用日线。
func (w Window) Bar() string {
	if w == Window24h || w == Window7d {
		return "5m"
	}
	return "1d"
}

// Rule 是校验后可直接使用的规则。
type Rule struct {
	ID                    string
	Type                  RuleType
	Window                Window
	ThresholdPct, StepPct float64
	Notify                NotifyPolicy
}

// ParseRule 校验规则取值；c.Notify 为 nil 时使用 def。
func ParseRule(c RuleCfg, def NotifyPolicy) (Rule, error) {
	fail := func(format string, args ...any) (Rule, error) {
		return Rule{}, fmt.Errorf("rule %q: "+format, append([]any{c.ID}, args...)...)
	}
	t := RuleType(c.Type)
	if t != DropFromHigh && t != RiseFromLow {
		return fail("invalid type %q", c.Type)
	}
	w := Window(c.Window)
	switch w {
	case Window24h, Window7d, Window30d, Window4y, WindowAll:
	default:
		return fail("invalid window %q", c.Window)
	}
	// 写成 !(x > 0) 而非 x <= 0：NaN 的任何比较都为假，后者会放行 NaN
	if !(c.ThresholdPct > 0) || math.IsInf(c.ThresholdPct, 1) {
		return fail("threshold_pct must be a finite number > 0, got %v", c.ThresholdPct)
	}
	if !(c.StepPct >= 0) || math.IsInf(c.StepPct, 1) {
		return fail("step_pct must be a finite number >= 0, got %v", c.StepPct)
	}
	n := def
	if c.Notify != nil {
		n = *c.Notify
	}
	if n.MaxPerTier < 1 {
		return fail("max_per_tier must be >= 1, got %d", n.MaxPerTier)
	}
	if n.RepeatInterval <= 0 {
		return fail("repeat_interval must be > 0, got %v", n.RepeatInterval)
	}
	return Rule{ID: c.ID, Type: t, Window: w, ThresholdPct: c.ThresholdPct, StepPct: c.StepPct, Notify: n}, nil
}

// Measure 按 spec §4 计算 x（百分数）；high/low 已包含现价。
func Measure(t RuleType, price, high, low float64) float64 {
	if t == DropFromHigh {
		return (high - price) / high * 100
	}
	return (price - low) / low * 100
}

// tierEps 吸收浮点误差：x 恰在阈值或档界上时算作达到。
const tierEps = 1e-9

// Tier 返回 x 所在档位；未达阈值或 x 非有限值时返回 0。
func Tier(x, thresholdPct, stepPct float64) int {
	if math.IsNaN(x) || math.IsInf(x, 0) || x+tierEps < thresholdPct {
		return 0
	}
	if stepPct <= 0 {
		return 1
	}
	return 1 + int(math.Floor((x-thresholdPct)/stepPct+tierEps))
}
