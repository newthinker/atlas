package collector

import "sync"

// Registry manages collector plugins
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
}

// NewRegistry creates a new collector registry
func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
	}
}

// Register adds a collector to the registry
func (r *Registry) Register(c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors[c.Name()] = c
}

// Get retrieves a collector by name
func (r *Registry) Get(name string) (Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.collectors[name]
	return c, ok
}

// GetAll returns all registered collectors
func (r *Registry) GetAll() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Collector, 0, len(r.collectors))
	for _, c := range r.collectors {
		result = append(result, c)
	}
	return result
}
