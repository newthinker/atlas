// internal/storage/signal/memory.go
package signal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// MemoryStore is an in-memory signal store.
type MemoryStore struct {
	signals []core.Signal
	maxSize int
	mu      sync.RWMutex
	counter int64
}

// NewMemoryStore creates a new in-memory store with max capacity.
func NewMemoryStore(maxSize int) *MemoryStore {
	return &MemoryStore{
		signals: make([]core.Signal, 0, maxSize),
		maxSize: maxSize,
	}
}

// Save adds a signal to the store.
func (m *MemoryStore) Save(ctx context.Context, signal core.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter++
	signal.ID = fmt.Sprintf("sig_%d_%d", time.Now().UnixNano(), m.counter)

	m.signals = append(m.signals, signal)

	// Trim if over capacity (remove oldest)
	if len(m.signals) > m.maxSize {
		m.signals = m.signals[len(m.signals)-m.maxSize:]
	}

	return nil
}

// GetByID retrieves a signal by ID.
func (m *MemoryStore) GetByID(ctx context.Context, id string) (*core.Signal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.signals {
		if m.signals[i].ID == id {
			sig := m.signals[i]
			return &sig, nil
		}
	}
	return nil, core.ErrSymbolNotFound
}

// List returns signals matching the filter.
func (m *MemoryStore) List(ctx context.Context, filter ListFilter) ([]core.Signal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []core.Signal
	for _, sig := range m.signals {
		if m.matches(sig, filter) {
			result = append(result, sig)
		}
	}

	// Apply offset and limit
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return []core.Signal{}, nil
	}

	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}

	return result, nil
}

// Count returns the count of matching signals.
func (m *MemoryStore) Count(ctx context.Context, filter ListFilter) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, sig := range m.signals {
		if m.matches(sig, filter) {
			count++
		}
	}
	return count, nil
}

func (m *MemoryStore) matches(sig core.Signal, filter ListFilter) bool {
	if filter.Symbol != "" && sig.Symbol != filter.Symbol {
		return false
	}
	if filter.Strategy != "" && sig.Strategy != filter.Strategy {
		return false
	}
	if filter.Action != "" && sig.Action != filter.Action {
		return false
	}
	if !filter.From.IsZero() && sig.GeneratedAt.Before(filter.From) {
		return false
	}
	if !filter.To.IsZero() && sig.GeneratedAt.After(filter.To) {
		return false
	}
	return true
}
