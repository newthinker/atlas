package alert

import (
	"sync"
	"time"
)

// Notifier interface for sending alerts.
type Notifier interface {
	Name() string
	Notify(msg string) error
}

// Evaluator evaluates alert rules and sends notifications.
type Evaluator struct {
	notifiers []Notifier
	metrics   map[string]float64
	cooldown  time.Duration

	// Track pending alerts (waiting for "for" duration)
	pending map[string]time.Time
	// Track last fired time for cooldown
	lastFired map[string]time.Time

	// For testing: allow time advancement
	now func() time.Time

	mu sync.RWMutex
}

// NewEvaluator creates a new alert evaluator.
func NewEvaluator(notifiers []Notifier) *Evaluator {
	return &Evaluator{
		notifiers: notifiers,
		metrics:   make(map[string]float64),
		cooldown:  5 * time.Minute,
		pending:   make(map[string]time.Time),
		lastFired: make(map[string]time.Time),
		now:       time.Now,
	}
}

// SetMetrics updates the current metrics.
func (e *Evaluator) SetMetrics(metrics map[string]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics = metrics
}

// SetCooldown sets the cooldown duration between alerts.
func (e *Evaluator) SetCooldown(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cooldown = d
}

// Evaluate evaluates a single rule and fires notification if triggered.
func (e *Evaluator) Evaluate(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()

	// Check if rule condition is met
	if !rule.Evaluate(e.metrics) {
		// Rule not triggered, clear pending state
		delete(e.pending, rule.Name)
		return
	}

	// Rule is triggered
	if rule.For > 0 {
		// Check if we're already pending
		pendingSince, isPending := e.pending[rule.Name]
		if !isPending {
			// Start pending
			e.pending[rule.Name] = now
			return
		}

		// Check if pending duration exceeded
		if now.Sub(pendingSince) < rule.For {
			return // Still waiting
		}
	}

	// Check cooldown
	lastFired, hasFired := e.lastFired[rule.Name]
	if hasFired && now.Sub(lastFired) < e.cooldown {
		return // In cooldown
	}

	// Fire alert
	msg := rule.FormatMessage(e.metrics)
	for _, n := range e.notifiers {
		n.Notify(msg)
	}

	e.lastFired[rule.Name] = now
	delete(e.pending, rule.Name)
}

// EvaluateAll evaluates all rules.
func (e *Evaluator) EvaluateAll(rules []Rule) {
	for _, rule := range rules {
		e.Evaluate(rule)
	}
}

// advanceTime is for testing - advances the internal clock.
func (e *Evaluator) advanceTime(d time.Duration) {
	oldNow := e.now
	e.now = func() time.Time {
		return oldNow().Add(d)
	}
}
