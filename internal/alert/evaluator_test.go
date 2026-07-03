package alert

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type mockNotifier struct {
	sent []string
}

func (m *mockNotifier) Name() string { return "mock" }
func (m *mockNotifier) Notify(msg string) error {
	m.sent = append(m.sent, msg)
	return nil
}

// errNotifier always fails and counts how many times it was called, so tests
// can assert on retry behaviour and warn logging.
type errNotifier struct {
	name  string
	calls int
	err   error
}

func (m *errNotifier) Name() string { return m.name }
func (m *errNotifier) Notify(string) error {
	m.calls++
	return m.err
}

func TestEvaluator_EvaluateRule(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})

	rule := Rule{
		Name:     "test_alert",
		Expr:     "error_rate > 0.05",
		For:      time.Minute,
		Severity: "warning",
		Message:  "Error rate is high",
	}

	// Provide metrics that trigger the rule
	metrics := map[string]float64{
		"error_rate": 0.10, // 10% > 5%
	}

	eval.SetMetrics(metrics)
	eval.Evaluate(rule)

	// First evaluation starts the pending timer, doesn't fire
	if len(notifier.sent) != 0 {
		t.Errorf("expected no notification on first eval, got %d", len(notifier.sent))
	}

	// Simulate time passing and re-evaluate
	eval.advanceTime(2 * time.Minute)
	eval.Evaluate(rule)

	if len(notifier.sent) != 1 {
		t.Errorf("expected 1 notification after duration, got %d", len(notifier.sent))
	}
}

func TestEvaluator_Cooldown(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})
	eval.SetCooldown(5 * time.Minute)

	rule := Rule{
		Name:     "test_alert",
		Expr:     "up == 0",
		For:      0, // Immediate
		Severity: "critical",
		Message:  "Service is down",
	}

	metrics := map[string]float64{"up": 0}
	eval.SetMetrics(metrics)

	eval.Evaluate(rule)
	eval.Evaluate(rule)
	eval.Evaluate(rule)

	// Should only notify once due to cooldown
	if len(notifier.sent) != 1 {
		t.Errorf("expected 1 notification due to cooldown, got %d", len(notifier.sent))
	}
}

func TestEvaluator_RuleNotTriggered(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})

	rule := Rule{
		Name:     "test_alert",
		Expr:     "error_rate > 0.05",
		For:      0,
		Severity: "warning",
		Message:  "Error rate is high",
	}

	metrics := map[string]float64{"error_rate": 0.01} // 1% < 5%
	eval.SetMetrics(metrics)

	eval.Evaluate(rule)

	if len(notifier.sent) != 0 {
		t.Errorf("expected no notification, got %d", len(notifier.sent))
	}
}

func TestEvaluator_EvaluateAll(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})

	rules := []Rule{
		{Name: "rule1", Expr: "up == 0", For: 0, Severity: "critical", Message: "Down"},
		{Name: "rule2", Expr: "error_rate > 0.5", For: 0, Severity: "warning", Message: "Errors"},
	}

	// Only rule1 triggers
	metrics := map[string]float64{"up": 0, "error_rate": 0.1}
	eval.SetMetrics(metrics)

	eval.EvaluateAll(rules)

	if len(notifier.sent) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.sent))
	}
}

func TestRule_Evaluate(t *testing.T) {
	tests := []struct {
		expr     string
		metrics  map[string]float64
		expected bool
	}{
		{"error_rate > 0.05", map[string]float64{"error_rate": 0.10}, true},
		{"error_rate > 0.05", map[string]float64{"error_rate": 0.01}, false},
		{"up == 0", map[string]float64{"up": 0}, true},
		{"up == 0", map[string]float64{"up": 1}, false},
		{"count >= 10", map[string]float64{"count": 10}, true},
		{"count >= 10", map[string]float64{"count": 9}, false},
		{"latency <= 100", map[string]float64{"latency": 50}, true},
		{"latency <= 100", map[string]float64{"latency": 150}, false},
		{"status != 200", map[string]float64{"status": 500}, true},
		{"status != 200", map[string]float64{"status": 200}, false},
		{"missing > 0", map[string]float64{}, false}, // missing metric
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			rule := Rule{Expr: tt.expr}
			result := rule.Evaluate(tt.metrics)
			if result != tt.expected {
				t.Errorf("expr %q with metrics %v: expected %v, got %v",
					tt.expr, tt.metrics, tt.expected, result)
			}
		})
	}
}

func TestRule_FormatMessage(t *testing.T) {
	rule := Rule{
		Name:     "high_error_rate",
		Severity: "warning",
		Message:  "API error rate above 5%",
	}

	msg := rule.FormatMessage(map[string]float64{})

	if msg != "[WARNING] high_error_rate: API error rate above 5%" {
		t.Errorf("unexpected message: %s", msg)
	}
}

// W2 functional[0]+[1]: a failing notifier is logged (with rule + notifier
// name) and must NOT enter cooldown, so the next evaluation retries.
func TestEvaluator_NotifyFailure_LogsWarnAndRetriesNextRound(t *testing.T) {
	failing := &errNotifier{name: "boom", err: errors.New("send failed")}
	eval := NewEvaluator([]Notifier{failing})
	obs, logs := observer.New(zapcore.WarnLevel)
	eval.logger = zap.New(obs)

	rule := Rule{Name: "down", Expr: "up == 0", For: 0, Severity: "critical", Message: "Service is down"}
	eval.SetMetrics(map[string]float64{"up": 0})

	eval.Evaluate(rule)
	if failing.calls != 1 {
		t.Fatalf("expected 1 notify attempt, got %d", failing.calls)
	}
	warns := logs.FilterLevelExact(zapcore.WarnLevel)
	if warns.Len() != 1 {
		t.Fatalf("expected 1 warn on notify failure, got %d", warns.Len())
	}
	fields := warns.All()[0].ContextMap()
	if fields["rule"] != "down" || fields["notifier"] != "boom" {
		t.Errorf("warn must name rule and notifier, got fields %v", fields)
	}

	// Total failure must not have entered cooldown → next round retries.
	eval.Evaluate(rule)
	if failing.calls != 2 {
		t.Errorf("failure must not enter cooldown; expected retry (2 attempts), got %d", failing.calls)
	}
}

// non_functional[1]: SetLogger provides a logger injection path; a nil logger
// is ignored (keeps the default no-op) so it never panics.
func TestEvaluator_SetLogger_InjectsAndNilSafe(t *testing.T) {
	failing := &errNotifier{name: "boom", err: errors.New("send failed")}
	eval := NewEvaluator([]Notifier{failing})
	obs, logs := observer.New(zapcore.WarnLevel)
	eval.SetLogger(zap.New(obs))

	rule := Rule{Name: "down", Expr: "up == 0", For: 0, Severity: "critical", Message: "down"}
	eval.SetMetrics(map[string]float64{"up": 0})
	eval.Evaluate(rule)

	if logs.FilterLevelExact(zapcore.WarnLevel).Len() != 1 {
		t.Errorf("injected logger must capture the notify-failure warn, got %d", logs.Len())
	}

	// A nil logger must be ignored (no panic, keeps prior logger).
	eval.SetLogger(nil)
	eval.Evaluate(rule) // must not panic
}

// W2 functional[0]+[2]: when at least one notifier succeeds, the rule enters
// cooldown (no re-dispatch next round), while the failing one is only logged
// and does not interrupt the successful delivery.
func TestEvaluator_PartialFailure_EntersCooldownAndWarns(t *testing.T) {
	failing := &errNotifier{name: "boom", err: errors.New("x")}
	ok := &mockNotifier{}
	// failing first proves a failure does not abort the remaining notifiers.
	eval := NewEvaluator([]Notifier{failing, ok})
	obs, logs := observer.New(zapcore.WarnLevel)
	eval.logger = zap.New(obs)
	eval.SetCooldown(5 * time.Minute)

	rule := Rule{Name: "down", Expr: "up == 0", For: 0, Severity: "critical", Message: "Service is down"}
	eval.SetMetrics(map[string]float64{"up": 0})

	eval.Evaluate(rule)
	if failing.calls != 1 || len(ok.sent) != 1 {
		t.Fatalf("both notifiers must be attempted: failing.calls=%d ok.sent=%d", failing.calls, len(ok.sent))
	}
	if logs.FilterLevelExact(zapcore.WarnLevel).Len() != 1 {
		t.Errorf("expected 1 warn for the failing notifier, got %d", logs.FilterLevelExact(zapcore.WarnLevel).Len())
	}

	// One success → cooldown entered → no re-dispatch on the next evaluation.
	eval.Evaluate(rule)
	if failing.calls != 1 || len(ok.sent) != 1 {
		t.Errorf("partial success must enter cooldown; got failing.calls=%d ok.sent=%d", failing.calls, len(ok.sent))
	}
}

func TestEvaluator_PendingClearsWhenRuleNoLongerTriggers(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})

	rule := Rule{
		Name:     "test_alert",
		Expr:     "error_rate > 0.05",
		For:      time.Minute,
		Severity: "warning",
		Message:  "Error rate is high",
	}

	// First: trigger rule to start pending
	eval.SetMetrics(map[string]float64{"error_rate": 0.10})
	eval.Evaluate(rule)

	// Second: rule no longer triggers - should clear pending
	eval.SetMetrics(map[string]float64{"error_rate": 0.01})
	eval.Evaluate(rule)

	// Third: advance time and re-trigger - should start new pending
	eval.advanceTime(2 * time.Minute)
	eval.SetMetrics(map[string]float64{"error_rate": 0.10})
	eval.Evaluate(rule)

	// Should not fire yet because pending was cleared
	if len(notifier.sent) != 0 {
		t.Errorf("expected no notification (pending cleared), got %d", len(notifier.sent))
	}
}
