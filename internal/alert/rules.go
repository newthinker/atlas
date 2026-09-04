package alert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Rule defines an alert rule.
type Rule struct {
	Name     string        `mapstructure:"name"`
	Expr     string        `mapstructure:"expr"`
	For      time.Duration `mapstructure:"for"`
	Severity string        `mapstructure:"severity"`
	Message  string        `mapstructure:"message"`
	// Cooldown 是本规则两次触发之间的最小间隔（M1.5 的 TASK-005）。0 = 用评估器的
	// 全局值（5 分钟）。持续几天的状态（如 hestia 停跑）设 24h，否则每 5 分钟一条。
	// 负值按 `> 0` 判定视同未写，不加校验（与 For 同形，For 也不校验负值）。
	Cooldown time.Duration `mapstructure:"cooldown"`
}

// Evaluate evaluates the rule expression against metrics.
func (r *Rule) Evaluate(metrics map[string]float64) bool {
	// Simple expression parser: "metric op value"
	// Supports: >, <, >=, <=, ==, !=
	pattern := regexp.MustCompile(`^(\w+)\s*(>|<|>=|<=|==|!=)\s*([\d.]+)$`)
	matches := pattern.FindStringSubmatch(strings.TrimSpace(r.Expr))

	if len(matches) != 4 {
		return false
	}

	metricName := matches[1]
	op := matches[2]
	threshold, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return false
	}

	value, exists := metrics[metricName]
	if !exists {
		return false
	}

	switch op {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

// FormatMessage formats the alert message with metric values.
func (r *Rule) FormatMessage(metrics map[string]float64) string {
	msg := fmt.Sprintf("[%s] %s: %s", strings.ToUpper(r.Severity), r.Name, r.Message)
	return msg
}
