package router

import (
	"context"
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
	signalStore signal.Store
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
	}
}

// Route processes a signal through filters and sends to notifiers
func (r *Router) Route(signal core.Signal) error {
	// Apply filters
	if !r.passesFilters(signal) {
		r.logger.Debug("signal filtered out",
			zap.String("symbol", signal.Symbol),
			zap.String("action", string(signal.Action)),
			zap.Float64("confidence", signal.Confidence),
		)
		return nil
	}

	// Persist signal if store is configured
	if r.signalStore != nil {
		if err := r.signalStore.Save(context.Background(), signal); err != nil {
			r.logger.Error("failed to persist signal", zap.Error(err))
		}
	}

	// Update cooldown
	r.mu.Lock()
	r.cooldowns[signal.Symbol] = time.Now()
	r.mu.Unlock()

	// Send to all notifiers (nil registry is allowed)
	if r.registry == nil {
		return nil
	}
	errors := r.registry.NotifyAll(signal)

	if len(errors) > 0 {
		for name, err := range errors {
			r.logger.Error("notifier failed",
				zap.String("notifier", name),
				zap.Error(err),
			)
		}
	}

	r.logger.Info("signal routed",
		zap.String("symbol", signal.Symbol),
		zap.String("action", string(signal.Action)),
		zap.Float64("confidence", signal.Confidence),
		zap.Int("notifiers", len(r.registry.GetAll())),
		zap.Int("errors", len(errors)),
	)

	return nil
}

// RouteBatch processes multiple signals
func (r *Router) RouteBatch(signals []core.Signal) error {
	var filtered []core.Signal

	for _, signal := range signals {
		if r.passesFilters(signal) {
			filtered = append(filtered, signal)

			// Update cooldown
			r.mu.Lock()
			r.cooldowns[signal.Symbol] = time.Now()
			r.mu.Unlock()
		}
	}

	if len(filtered) == 0 {
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

// passesFilters checks if a signal passes all configured filters
func (r *Router) passesFilters(signal core.Signal) bool {
	// Check confidence threshold
	if signal.Confidence < r.cfg.MinConfidence {
		return false
	}

	// Check action whitelist
	if len(r.cfg.EnabledActions) > 0 {
		actionAllowed := false
		for _, a := range r.cfg.EnabledActions {
			if signal.Action == a {
				actionAllowed = true
				break
			}
		}
		if !actionAllowed {
			return false
		}
	}

	// Check cooldown
	r.mu.RLock()
	lastSignal, exists := r.cooldowns[signal.Symbol]
	r.mu.RUnlock()

	if exists && time.Since(lastSignal) < r.cfg.CooldownDuration {
		return false
	}

	return true
}

// ClearCooldown removes cooldown for a specific symbol
func (r *Router) ClearCooldown(symbol string) {
	r.mu.Lock()
	delete(r.cooldowns, symbol)
	r.mu.Unlock()
}

// ClearAllCooldowns removes all cooldowns
func (r *Router) ClearAllCooldowns() {
	r.mu.Lock()
	r.cooldowns = make(map[string]time.Time)
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
		"cooldowns_active": len(r.cooldowns),
		"min_confidence":   r.cfg.MinConfidence,
		"cooldown_seconds": r.cfg.CooldownDuration.Seconds(),
		"enabled_actions":  r.cfg.EnabledActions,
	}
}
