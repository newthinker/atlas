// internal/api/job/store.go
package job

import (
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Status represents job status.
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
)

// Job represents an async job.
type Job struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Status    Status      `json:"status"`
	Progress  int         `json:"progress"`
	Result    any         `json:"result,omitempty"`
	Error     *core.Error `json:"error,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// Store manages async jobs.
type Store struct {
	jobs    map[string]*Job
	order   []string // Track insertion order for eviction
	maxSize int
	ttl     time.Duration
	mu      sync.RWMutex
	counter int64
}

// NewStore creates a new job store.
func NewStore(maxSize int, ttl time.Duration) *Store {
	return &Store{
		jobs:    make(map[string]*Job),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Create creates a new job and returns it.
func (s *Store) Create(jobType string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	now := time.Now()
	job := &Job{
		ID:        fmt.Sprintf("job_%d_%d", now.UnixNano(), s.counter),
		Type:      jobType,
		Status:    StatusPending,
		Progress:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Evict oldest if at capacity
	if len(s.jobs) >= s.maxSize && len(s.order) > 0 {
		oldest := s.order[0]
		delete(s.jobs, oldest)
		s.order = s.order[1:]
	}

	s.jobs[job.ID] = job
	s.order = append(s.order, job.ID)

	return job
}

// Get retrieves a job by ID.
func (s *Store) Get(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[id]
	if !ok {
		return nil, core.ErrSymbolNotFound // Reuse existing error
	}

	// Return copy to prevent race conditions
	jobCopy := *job
	return &jobCopy, nil
}

// Update modifies a job using an update function.
func (s *Store) Update(id string, fn func(*Job)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return core.ErrSymbolNotFound
	}

	fn(job)
	job.UpdatedAt = time.Now()
	return nil
}

// List returns all jobs.
func (s *Store) List() []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, *job)
	}
	return result
}
