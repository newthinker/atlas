// internal/storage/signal/interface.go
package signal

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Store defines the interface for signal persistence.
type Store interface {
	// Save persists a signal and assigns an ID.
	Save(ctx context.Context, signal core.Signal) error

	// GetByID retrieves a signal by its ID.
	GetByID(ctx context.Context, id string) (*core.Signal, error)

	// List retrieves signals matching the filter.
	List(ctx context.Context, filter ListFilter) ([]core.Signal, error)

	// Count returns the number of signals matching the filter.
	Count(ctx context.Context, filter ListFilter) (int, error)
}

// ListFilter defines criteria for listing signals.
type ListFilter struct {
	Symbol   string
	Strategy string
	Action   core.Action
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
}
