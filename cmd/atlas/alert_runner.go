package main

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/alert"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/metrics"
	"github.com/newthinker/atlas/internal/notifier"
	signalstore "github.com/newthinker/atlas/internal/storage/signal"
	"go.uber.org/zap"
)

// defaultAlertInterval is used when cfg.Alerts.CheckInterval is unset or zero.
const defaultAlertInterval = 60 * time.Second

// signalCounter is the slice of the signal store the alert loop needs to derive
// signals_24h. *signalstore.MemoryStore satisfies it.
type signalCounter interface {
	Count(ctx context.Context, filter signalstore.ListFilter) (int, error)
}

// derivedMetrics carries the previous snapshot needed to compute interval-based
// (delta) metrics; a cumulative counter ratio is meaningless for alerting.
type derivedMetrics struct {
	prev5xx   float64
	prevTotal float64
	hasPrev   bool
}

// httpErrorRate returns the interval 5xx-over-total error rate from the delta
// between this snapshot and the previous one. produced is false on the first
// snapshot (no baseline). Counter resets (negative deltas) clamp to 0, and a
// zero total-request delta yields 0 rather than dividing by zero.
func (d *derivedMetrics) httpErrorRate(snap map[string]float64) (rate float64, produced bool) {
	cur5xx := snap["http_requests_total_5xx"]
	curTotal := snap["http_requests_total"]

	if !d.hasPrev {
		d.prev5xx, d.prevTotal, d.hasPrev = cur5xx, curTotal, true
		return 0, false
	}

	d5xx := cur5xx - d.prev5xx
	dTotal := curTotal - d.prevTotal
	d.prev5xx, d.prevTotal = cur5xx, curTotal

	if d5xx < 0 {
		d5xx = 0 // counter reset
	}
	if dTotal <= 0 {
		return 0, true // no requests in interval: avoid divide-by-zero
	}
	return d5xx / dTotal, true
}

// mapRules converts config alert rules to evaluator rules one-to-one. For is
// already a time.Duration (viper's string-duration decode hook handles "5m").
func mapRules(in []config.AlertRule) []alert.Rule {
	out := make([]alert.Rule, len(in))
	for i, r := range in {
		out[i] = alert.Rule{
			Name:     r.Name,
			Expr:     r.Expr,
			For:      r.For,
			Severity: r.Severity,
			Message:  r.Message,
		}
	}
	return out
}

// alertRunner periodically snapshots metrics, augments them with derived values
// (http_error_rate, signals_24h), and evaluates the configured alert rules.
type alertRunner struct {
	interval  time.Duration
	rules     []alert.Rule
	evaluator *alert.Evaluator
	snapshot  func() map[string]float64
	count     func(since time.Time) (int, error)
	now       func() time.Time
	log       *zap.Logger
	derived   derivedMetrics
}

// evaluateOnce runs a single snapshot → derive → SetMetrics → EvaluateAll cycle.
// snapshot() returns a fresh map each call, so the derived keys added here never
// leak into a shared map.
func (r *alertRunner) evaluateOnce() {
	m := r.snapshot()
	if m == nil {
		m = map[string]float64{}
	}

	if rate, ok := r.derived.httpErrorRate(m); ok {
		m["http_error_rate"] = rate
	}

	if cnt, err := r.count(r.now().Add(-24 * time.Hour)); err != nil {
		r.log.Warn("signals_24h count failed; skipping metric", zap.Error(err))
	} else {
		m["signals_24h"] = float64(cnt)
	}

	r.evaluator.SetMetrics(m)
	r.evaluator.EvaluateAll(r.rules)
}

// run loops on the check interval until ctx is cancelled, then returns.
func (r *alertRunner) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.log.Info("alert evaluation loop started",
		zap.Duration("check_interval", r.interval),
		zap.Int("rules", len(r.rules)),
	)

	for {
		select {
		case <-ctx.Done():
			r.log.Info("alert evaluation loop stopped")
			return
		case <-ticker.C:
			r.evaluateOnce()
		}
	}
}

// maybeStartAlertRunner builds the alert evaluation loop and starts it in its
// own goroutine when alerts are enabled, returning the running runner. When
// alerts are disabled it returns nil without constructing an evaluator or
// starting a goroutine (zero behaviour change).
func maybeStartAlertRunner(
	ctx context.Context,
	cfg *config.Config,
	notifiers []notifier.Notifier,
	reg *metrics.Registry,
	store signalCounter,
	log *zap.Logger,
) *alertRunner {
	if !cfg.Alerts.Enabled {
		return nil
	}

	adapters := make([]alert.Notifier, 0, len(notifiers))
	for _, n := range notifiers {
		adapters = append(adapters, notifier.NewAlertAdapter(n))
	}

	interval := cfg.Alerts.CheckInterval
	if interval <= 0 {
		interval = defaultAlertInterval
	}

	r := &alertRunner{
		interval:  interval,
		rules:     mapRules(cfg.Alerts.Rules),
		evaluator: alert.NewEvaluator(adapters),
		snapshot: func() map[string]float64 {
			if reg == nil {
				return map[string]float64{}
			}
			return reg.Snapshot()
		},
		count: func(since time.Time) (int, error) {
			return store.Count(ctx, signalstore.ListFilter{From: since})
		},
		now: time.Now,
		log: log,
	}

	go r.run(ctx)
	return r
}
