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
