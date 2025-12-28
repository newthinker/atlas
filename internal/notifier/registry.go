package notifier

import (
	"fmt"
	"sync"

	"github.com/newthinker/atlas/internal/core"
)

// Registry manages notifier instances
type Registry struct {
	mu        sync.RWMutex
	notifiers map[string]Notifier
}

// NewRegistry creates a new notifier registry
func NewRegistry() *Registry {
	return &Registry{
		notifiers: make(map[string]Notifier),
	}
}

// Register adds a notifier to the registry
func (r *Registry) Register(n Notifier) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := n.Name()
	if _, exists := r.notifiers[name]; exists {
		return fmt.Errorf("notifier %s already registered", name)
	}

	r.notifiers[name] = n
	return nil
}

// Get retrieves a notifier by name
func (r *Registry) Get(name string) (Notifier, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, exists := r.notifiers[name]
	if !exists {
		return nil, fmt.Errorf("notifier %s not found", name)
	}
	return n, nil
}

// GetAll returns all registered notifiers
func (r *Registry) GetAll() []Notifier {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Notifier, 0, len(r.notifiers))
	for _, n := range r.notifiers {
		result = append(result, n)
	}
	return result
}

// NotifyAll sends a signal to all registered notifiers
func (r *Registry) NotifyAll(signal core.Signal) map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	errors := make(map[string]error)
	for name, n := range r.notifiers {
		if err := n.Send(signal); err != nil {
			errors[name] = err
		}
	}
	return errors
}

// NotifyAllBatch sends multiple signals to all registered notifiers
func (r *Registry) NotifyAllBatch(signals []core.Signal) map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	errors := make(map[string]error)
	for name, n := range r.notifiers {
		if err := n.SendBatch(signals); err != nil {
			errors[name] = err
		}
	}
	return errors
}
