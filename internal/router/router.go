package router

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/storage/signal"
	"go.uber.org/zap"
)

// Config holds router configuration
type Config struct {
	MinConfidence    float64       `mapstructure:"min_confidence"`
	CooldownDuration time.Duration `mapstructure:"cooldown_duration"`
	EnabledActions   []core.Action `mapstructure:"enabled_actions"`
	// PercentileStep is the global fallback step for percentile re-alert gating
	// (|current percentile - last notified| >= step routes again). 0 = disabled,
	// in which case percentile signals fall back to the cooldown path. A
	// per-signal Metadata["percentile_step"] (strategy-level config) overrides
	// this value when present and > 0.
	PercentileStep float64 `mapstructure:"percentile_step"`
	// BatchNotify defers notification: Route buffers passed signals instead of
	// notifying immediately; FlushNotifications sends them as one batch. Routing
	// decision/cooldown/execution stay per-signal. Default wired true by config.
	BatchNotify bool `mapstructure:"batch_notify"`
}

// DefaultConfig returns default router configuration
func DefaultConfig() Config {
	return Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour,
		EnabledActions:   []core.Action{core.ActionBuy, core.ActionSell, core.ActionStrongBuy, core.ActionStrongSell},
	}
}

// Router routes signals to notifiers with filtering
type Router struct {
	cfg         Config
	registry    *notifier.Registry
	logger      *zap.Logger
	cooldowns   map[string]time.Time // symbol -> last signal time
	pctGates    map[string]float64   // symbol|strategy|side -> last notified percentile
	signalStore signal.Store
	pending     []core.Signal // batch-notify buffer; guarded by mu
	mu          sync.RWMutex
}

// SetSignalStore sets the signal persistence store
func (r *Router) SetSignalStore(store signal.Store) {
	r.signalStore = store
}

// New creates a new signal router
func New(cfg Config, registry *notifier.Registry, logger *zap.Logger) *Router {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Router{
		cfg:       cfg,
		registry:  registry,
		logger:    logger,
		cooldowns: make(map[string]time.Time),
		pctGates:  make(map[string]float64),
	}
}

// Route processes a signal through filters and sends to notifiers. It reports
// whether the signal was actually routed (true) or suppressed by a filter such
// as the per-symbol cooldown (false), so callers can avoid acting on a
// suppressed signal (e.g. submitting it for execution).
func (r *Router) Route(signal core.Signal) (routed bool, err error) {
	// Static filters (confidence + action) are a common precondition for both
	// the percentile-step gate and the cooldown path.
	if !r.passesStaticFilters(signal) {
		r.logger.Debug("signal filtered out",
			zap.String("symbol", signal.Symbol),
			zap.String("action", string(signal.Action)),
			zap.Float64("confidence", signal.Confidence),
		)
		return false, nil
	}

	if !r.passesDispatchGate(signal) {
		return false, nil
	}

	// Persist signal if store is configured
	if r.signalStore != nil {
		if err := r.signalStore.Save(context.Background(), signal); err != nil {
			r.logger.Error("failed to persist signal", zap.Error(err))
		}
	}

	// nil registry: nothing to notify (parity with original).
	if r.registry == nil {
		return true, nil
	}
	// Batch mode: buffer and defer; FlushNotifications sends one batch per cycle.
	if r.cfg.BatchNotify {
		r.mu.Lock()
		r.pending = append(r.pending, signal)
		r.mu.Unlock()
		return true, nil
	}
	errors := r.registry.NotifyAll(signal)
	if len(errors) > 0 {
		for name, err := range errors {
			r.logger.Error("notifier failed", zap.String("notifier", name), zap.Error(err))
		}
	}
	r.logger.Info("signal routed",
		zap.String("symbol", signal.Symbol),
		zap.String("action", string(signal.Action)),
		zap.Float64("confidence", signal.Confidence),
		zap.Int("notifiers", len(r.registry.GetAll())),
		zap.Int("errors", len(errors)),
	)
	return true, nil
}

// FlushNotifications sends all buffered (batch-notify) signals as one batch and
// clears the buffer. No-op when the buffer is empty or no registry is set.
// Called at the end of an analysis cycle.
func (r *Router) FlushNotifications() {
	r.mu.Lock()
	batch := r.pending
	r.pending = nil
	r.mu.Unlock()

	if len(batch) == 0 || r.registry == nil {
		return
	}
	errors := r.registry.NotifyAllBatch(batch)
	for name, err := range errors {
		r.logger.Error("notifier failed on digest", zap.String("notifier", name), zap.Error(err))
	}
	r.logger.Info("signal digest sent", zap.Int("count", len(batch)), zap.Int("errors", len(errors)))
}

// RouteBatch processes multiple signals
func (r *Router) RouteBatch(signals []core.Signal) error {
	var filtered []core.Signal

	for _, signal := range signals {
		if !r.passesStaticFilters(signal) {
			continue
		}
		// Same dispatch as Route. Evaluated in order so a later signal sees the
		// earlier one's gate/cooldown state.
		if !r.passesDispatchGate(signal) {
			continue
		}
		filtered = append(filtered, signal)
	}

	if len(filtered) == 0 {
		return nil
	}

	// nil registry is allowed (parity with Route): nothing to notify.
	if r.registry == nil {
		return nil
	}

	errors := r.registry.NotifyAllBatch(filtered)

	if len(errors) > 0 {
		for name, err := range errors {
			r.logger.Error("notifier failed on batch",
				zap.String("notifier", name),
				zap.Error(err),
			)
		}
	}

	r.logger.Info("batch routed",
		zap.Int("total", len(signals)),
		zap.Int("filtered", len(filtered)),
		zap.Int("errors", len(errors)),
	)

	return nil
}

// passesStaticFilters checks the confidence threshold and action whitelist —
// the common precondition that applies regardless of percentile/cooldown path.
func (r *Router) passesStaticFilters(signal core.Signal) bool {
	// Check confidence threshold
	if signal.Confidence < r.cfg.MinConfidence {
		return false
	}

	// Check action whitelist
	if len(r.cfg.EnabledActions) > 0 && !slices.Contains(r.cfg.EnabledActions, signal.Action) {
		return false
	}

	return true
}

// passesDispatchGate applies the percentile-step gate or the per-symbol cooldown
// and reports whether the signal may proceed. Percentile signals (percentile
// metadata present AND an effective step > 0) go through the step gate, which
// fully replaces the cooldown — it neither reads nor stamps the cooldown. All
// other signals take the cooldown path and, on pass, stamp the cooldown; the
// percentile branch must never touch it (otherwise it would suppress other
// strategies' signals for the same symbol).
func (r *Router) passesDispatchGate(signal core.Signal) bool {
	if pct, ok := r.percentileOf(signal); ok {
		if step := r.effectiveStep(signal); step > 0 {
			return r.passPercentileGate(signal, pct, step)
		}
	}

	if !r.passesCooldown(signal) {
		return false
	}
	r.mu.Lock()
	r.cooldowns[signal.Symbol] = time.Now()
	r.mu.Unlock()
	return true
}

// passesCooldown reports whether the per-symbol cooldown allows this signal.
// CooldownDuration == 0 makes time.Since(last) < 0 always false → always passes
// (cooldown disabled).
func (r *Router) passesCooldown(signal core.Signal) bool {
	r.mu.RLock()
	lastSignal, exists := r.cooldowns[signal.Symbol]
	r.mu.RUnlock()

	if exists && time.Since(lastSignal) < r.cfg.CooldownDuration {
		return false
	}

	return true
}

// percentileOf extracts the historical percentile from signal metadata, trying
// the price then PE keys in order. Asserting float64 is safe: signals travel
// in-memory only (strategy → app → router); signalStore is write-only. Revisit
// if a replay-from-store path is ever added. A present-but-wrong-typed value is
// logged at debug and treated as absent (signal then takes the cooldown path).
func (r *Router) percentileOf(sig core.Signal) (float64, bool) {
	for _, key := range []string{"percentile", "pe_percentile"} {
		v, ok := sig.Metadata[key]
		if !ok {
			continue
		}
		if f, ok := v.(float64); ok {
			return f, true
		}
		r.logger.Debug("percentile metadata is not float64; falling back to cooldown path",
			zap.String("symbol", sig.Symbol),
			zap.String("strategy", sig.Strategy),
			zap.String("key", key),
			zap.Any("value", v),
		)
	}
	return 0, false
}

// effectiveStep returns the gate step for a signal: the strategy-carried
// Metadata["percentile_step"] wins when present and positive; otherwise the
// global router config value. <= 0 means the gate is disabled for this signal.
func (r *Router) effectiveStep(sig core.Signal) float64 {
	if v, ok := sig.Metadata["percentile_step"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			return f
		}
	}
	return r.cfg.PercentileStep
}

// sideOf maps an action onto the gate side: buy/strong_buy share the buy side,
// sell/strong_sell share the sell side.
func sideOf(action core.Action) string {
	if action == core.ActionBuy || action == core.ActionStrongBuy {
		return "buy"
	}
	return "sell"
}

// passPercentileGate reports whether the signal clears the step gate and records
// its percentile when it does. Check and update happen in one critical section
// (no check-then-act race). First sighting of a key always passes.
func (r *Router) passPercentileGate(sig core.Signal, pct, step float64) bool {
	key := sig.Symbol + "|" + sig.Strategy + "|" + sideOf(sig.Action)
	r.mu.Lock()
	defer r.mu.Unlock()

	last, exists := r.pctGates[key]
	if exists && math.Abs(pct-last) < step {
		return false
	}
	r.pctGates[key] = pct
	return true
}

// ClearCooldown removes cooldown for a specific symbol, and also clears that
// symbol's percentile gate state. Gate keys are "symbol|strategy|side"; we
// assume symbol contains no '|' (true for all current watchlist symbol forms),
// so the "symbol|" prefix uniquely identifies the symbol's gate entries.
func (r *Router) ClearCooldown(symbol string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.cooldowns, symbol)

	prefix := symbol + "|"
	for key := range r.pctGates {
		if strings.HasPrefix(key, prefix) {
			delete(r.pctGates, key)
		}
	}
}

// ClearAllCooldowns removes all cooldowns and all percentile gate state.
func (r *Router) ClearAllCooldowns() {
	r.mu.Lock()
	r.cooldowns = make(map[string]time.Time)
	r.pctGates = make(map[string]float64)
	r.mu.Unlock()
}

// CleanupExpiredCooldowns removes cooldown entries older than 2x the cooldown duration.
func (r *Router) CleanupExpiredCooldowns() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	expiry := r.cfg.CooldownDuration * 2
	removed := 0

	for symbol, lastTime := range r.cooldowns {
		if now.Sub(lastTime) > expiry {
			delete(r.cooldowns, symbol)
			removed++
		}
	}

	return removed
}

// StartCleanupRoutine starts a background goroutine that periodically cleans up expired cooldowns.
func (r *Router) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := r.CleanupExpiredCooldowns()
				if removed > 0 {
					r.logger.Debug("cleaned up expired cooldowns", zap.Int("removed", removed))
				}
			}
		}
	}()
}

// GetStats returns router statistics
func (r *Router) GetStats() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]any{
		"cooldowns_active":        len(r.cooldowns),
		"min_confidence":          r.cfg.MinConfidence,
		"cooldown_seconds":        r.cfg.CooldownDuration.Seconds(),
		"enabled_actions":         r.cfg.EnabledActions,
		"percentile_gates_active": len(r.pctGates),
		"percentile_step":         r.cfg.PercentileStep, // global fallback only
	}
}
