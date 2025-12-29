// internal/context/track_record.go
package context

import (
	"context"
	"sync"
)

// InMemoryTrackRecord implements TrackRecordProvider with in-memory storage.
type InMemoryTrackRecord struct {
	mu    sync.RWMutex
	stats map[string]*StrategyStats
}

// NewInMemoryTrackRecord creates a new in-memory track record provider.
func NewInMemoryTrackRecord() *InMemoryTrackRecord {
	return &InMemoryTrackRecord{
		stats: make(map[string]*StrategyStats),
	}
}

// GetStats returns stats for a specific strategy.
func (t *InMemoryTrackRecord) GetStats(ctx context.Context, strategy string) (*StrategyStats, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if stats, ok := t.stats[strategy]; ok {
		return stats, nil
	}
	return &StrategyStats{Strategy: strategy}, nil
}

// GetAllStats returns stats for all strategies.
func (t *InMemoryTrackRecord) GetAllStats(ctx context.Context) (map[string]*StrategyStats, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*StrategyStats)
	for k, v := range t.stats {
		result[k] = v
	}
	return result, nil
}

// UpdateStats updates the stats for a strategy.
func (t *InMemoryTrackRecord) UpdateStats(strategy string, stats *StrategyStats) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stats[strategy] = stats
}

// RecordOutcome records the outcome of a signal for a strategy.
func (t *InMemoryTrackRecord) RecordOutcome(strategy string, won bool, returnPct float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats, ok := t.stats[strategy]
	if !ok {
		stats = &StrategyStats{Strategy: strategy}
		t.stats[strategy] = stats
	}

	stats.TotalSignals++
	if won {
		// Update win rate incrementally
		wins := float64(stats.TotalSignals-1)*stats.WinRate + 1
		stats.WinRate = wins / float64(stats.TotalSignals)
	} else {
		wins := float64(stats.TotalSignals-1) * stats.WinRate
		stats.WinRate = wins / float64(stats.TotalSignals)
	}

	// Update average return incrementally
	stats.AvgReturn = (stats.AvgReturn*float64(stats.TotalSignals-1) + returnPct) / float64(stats.TotalSignals)
}
