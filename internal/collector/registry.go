package collector

import "sync"

// Registry manages collector plugins
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
	// order 记录首次注册顺序。GetAll 必须确定性地按它返回：app.orderedCollectors
	// 把「首选之外」的 collector 当作有序兜底链逐个尝试，map 迭代顺序是随机的。
	order []string
}

// NewRegistry creates a new collector registry
func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
	}
}

// Register adds a collector to the registry. 同名重注册覆盖实例但保持原位置。
func (r *Registry) Register(c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := c.Name()
	if _, ok := r.collectors[name]; !ok {
		r.order = append(r.order, name)
	}
	r.collectors[name] = c
}

// Get retrieves a collector by name
func (r *Registry) Get(name string) (Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.collectors[name]
	return c, ok
}

// GetAll returns all registered collectors in registration order.
func (r *Registry) GetAll() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Collector, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.collectors[name])
	}
	return result
}
