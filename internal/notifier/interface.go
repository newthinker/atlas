package notifier

import (
	"github.com/newthinker/atlas/internal/core"
)

// Config holds notifier configuration
type Config struct {
	Type   string         `mapstructure:"type"`
	Params map[string]any `mapstructure:"params"`
}

// Notifier defines the interface for signal notification
type Notifier interface {
	// Name returns the unique identifier for this notifier
	Name() string

	// Init initializes the notifier with configuration
	Init(cfg Config) error

	// Send sends a single signal notification
	Send(signal core.Signal) error

	// SendBatch sends multiple signal notifications
	SendBatch(signals []core.Signal) error
}
