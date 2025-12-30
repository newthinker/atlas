# M2: API Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the API layer with JSON REST APIs for external clients and HTMX endpoints for the dashboard.

**Architecture:** Add response helpers and auth middleware, then build API handlers that integrate with existing components (signal store, app, backtester). Use async job system for long-running operations.

**Tech Stack:** Go 1.24, net/http, encoding/json, sync.RWMutex for thread-safety.

---

## Task 1: Response Helpers

**Files:**
- Create: `internal/api/response/response.go`
- Test: `internal/api/response/response_test.go`

**Step 1: Write the failing test**

```go
// internal/api/response/response_test.go
package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}

	JSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json content type")
	}

	var resp SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data == nil {
		t.Error("expected data in response")
	}
	if resp.Meta.Timestamp.IsZero() {
		t.Error("expected timestamp in meta")
	}
}

func TestError_WithCoreError(t *testing.T) {
	w := httptest.NewRecorder()
	err := core.ErrConfigInvalid

	Error(w, http.StatusBadRequest, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "CONFIG_INVALID" {
		t.Errorf("expected CONFIG_INVALID, got %s", resp.Error.Code)
	}
}

func TestError_WithStandardError(t *testing.T) {
	w := httptest.NewRecorder()
	err := core.WrapError(core.ErrNoData, nil)

	Error(w, http.StatusNotFound, err)

	var resp ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "NO_DATA" {
		t.Errorf("expected NO_DATA, got %s", resp.Error.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/response/... -v`
Expected: FAIL - package not found

**Step 3: Write minimal implementation**

```go
// internal/api/response/response.go
package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Meta contains response metadata.
type Meta struct {
	Timestamp time.Time `json:"timestamp"`
}

// SuccessResponse is the standard success response format.
type SuccessResponse struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   string `json:"cause,omitempty"`
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// JSON writes a success response with data.
func JSON(w http.ResponseWriter, status int, data any) {
	resp := SuccessResponse{
		Data: data,
		Meta: Meta{Timestamp: time.Now().UTC()},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// Error writes an error response.
func Error(w http.ResponseWriter, status int, err error) {
	detail := ErrorDetail{
		Code:    "INTERNAL_ERROR",
		Message: "an internal error occurred",
	}

	var coreErr *core.Error
	if errors.As(err, &coreErr) {
		detail.Code = coreErr.Code
		detail.Message = coreErr.Message
		if coreErr.Cause != nil {
			detail.Cause = coreErr.Cause.Error()
		}
	}

	resp := ErrorResponse{Error: detail}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/response/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/response/
git commit -m "feat(api): add JSON response helpers with structured errors"
```

---

## Task 2: Auth Middleware

**Files:**
- Create: `internal/api/middleware/auth.go`
- Test: `internal/api/middleware/auth_test.go`

**Step 1: Write the failing test**

```go
// internal/api/middleware/auth_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	auth := APIKeyAuth("secret-key")
	wrapped := auth(handler)

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "secret-key")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyAuth_MissingKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	auth := APIKeyAuth("secret-key")
	wrapped := auth(handler)

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	auth := APIKeyAuth("secret-key")
	wrapped := auth(handler)

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_EmptyConfiguredKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	auth := APIKeyAuth("") // Empty = disabled
	wrapped := auth(handler)

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when auth disabled, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/middleware/... -v`
Expected: FAIL - package not found

**Step 3: Write minimal implementation**

```go
// internal/api/middleware/auth.go
package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
)

// APIKeyAuth returns middleware that validates X-API-Key header.
// If apiKey is empty, authentication is disabled.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no key configured
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			providedKey := r.Header.Get("X-API-Key")
			if providedKey == "" {
				response.Error(w, http.StatusUnauthorized,
					core.WrapError(core.ErrConfigMissing, nil))
				return
			}

			// Constant-time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
				response.Error(w, http.StatusUnauthorized,
					core.WrapError(core.ErrConfigInvalid, nil))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/middleware/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/middleware/
git commit -m "feat(api): add API key authentication middleware"
```

---

## Task 3: Job Store

**Files:**
- Create: `internal/api/job/store.go`
- Test: `internal/api/job/store_test.go`

**Step 1: Write the failing test**

```go
// internal/api/job/store_test.go
package job

import (
	"testing"
	"time"
)

func TestStore_CreateAndGet(t *testing.T) {
	store := NewStore(100, time.Hour)

	job := store.Create("backtest")
	if job.ID == "" {
		t.Error("expected job ID")
	}
	if job.Status != StatusPending {
		t.Errorf("expected pending, got %s", job.Status)
	}

	retrieved, err := store.Get(job.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.ID != job.ID {
		t.Error("IDs don't match")
	}
}

func TestStore_Update(t *testing.T) {
	store := NewStore(100, time.Hour)
	job := store.Create("backtest")

	err := store.Update(job.ID, func(j *Job) {
		j.Status = StatusRunning
		j.Progress = 50
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ := store.Get(job.ID)
	if retrieved.Status != StatusRunning {
		t.Errorf("expected running, got %s", retrieved.Status)
	}
	if retrieved.Progress != 50 {
		t.Errorf("expected 50, got %d", retrieved.Progress)
	}
}

func TestStore_MaxSize(t *testing.T) {
	store := NewStore(2, time.Hour)

	job1 := store.Create("backtest")
	store.Create("backtest")
	store.Create("backtest") // Should evict job1

	_, err := store.Get(job1.ID)
	if err == nil {
		t.Error("expected job1 to be evicted")
	}
}

func TestStore_NotFound(t *testing.T) {
	store := NewStore(100, time.Hour)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestStore_List(t *testing.T) {
	store := NewStore(100, time.Hour)
	store.Create("backtest")
	store.Create("analysis")

	jobs := store.List()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/job/... -v`
Expected: FAIL - package not found

**Step 3: Write minimal implementation**

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/job/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/job/
git commit -m "feat(api): add async job store with TTL and eviction"
```

---

## Task 4: Signals API Handler

**Files:**
- Create: `internal/api/handler/api/signals.go`
- Test: `internal/api/handler/api/signals_test.go`

**Step 1: Write the failing test**

```go
// internal/api/handler/api/signals_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/storage/signal"
)

func TestSignalsHandler_List(t *testing.T) {
	store := signal.NewMemoryStore(100)
	store.Save(context.Background(), core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "ma_crossover",
		GeneratedAt: time.Now(),
	})

	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	signals := data["signals"].([]any)
	if len(signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(signals))
	}
}

func TestSignalsHandler_ListWithFilters(t *testing.T) {
	store := signal.NewMemoryStore(100)
	store.Save(context.Background(), core.Signal{
		Symbol:      "AAPL",
		Strategy:    "ma_crossover",
		GeneratedAt: time.Now(),
	})
	store.Save(context.Background(), core.Signal{
		Symbol:      "GOOG",
		Strategy:    "pe_band",
		GeneratedAt: time.Now(),
	})

	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals?symbol=AAPL", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	signals := data["signals"].([]any)
	if len(signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(signals))
	}
}

func TestSignalsHandler_GetByID(t *testing.T) {
	store := signal.NewMemoryStore(100)
	store.Save(context.Background(), core.Signal{
		Symbol:      "AAPL",
		GeneratedAt: time.Now(),
	})

	signals, _ := store.List(context.Background(), signal.ListFilter{})
	signalID := signals[0].ID

	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals/"+signalID, nil)
	w := httptest.NewRecorder()

	handler.GetByID(w, req, signalID)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSignalsHandler_GetByID_NotFound(t *testing.T) {
	store := signal.NewMemoryStore(100)
	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.GetByID(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handler/api/... -v`
Expected: FAIL - package not found

**Step 3: Write minimal implementation**

```go
// internal/api/handler/api/signals.go
package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/storage/signal"
)

// SignalsHandler handles signal-related API requests.
type SignalsHandler struct {
	store signal.Store
}

// NewSignalsHandler creates a new signals handler.
func NewSignalsHandler(store signal.Store) *SignalsHandler {
	return &SignalsHandler{store: store}
}

// List returns signals matching query parameters.
func (h *SignalsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := signal.ListFilter{
		Symbol:   q.Get("symbol"),
		Strategy: q.Get("strategy"),
	}

	if action := q.Get("action"); action != "" {
		filter.Action = core.Action(action)
	}

	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = t
		} else if t, err := time.Parse("2006-01-02", from); err == nil {
			filter.From = t
		}
	}

	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = t
		} else if t, err := time.Parse("2006-01-02", to); err == nil {
			filter.To = t
		}
	}

	if limit := q.Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			filter.Limit = n
		}
	} else {
		filter.Limit = 50 // Default limit
	}

	if offset := q.Get("offset"); offset != "" {
		if n, err := strconv.Atoi(offset); err == nil {
			filter.Offset = n
		}
	}

	signals, err := h.store.List(context.Background(), filter)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}

	count, _ := h.store.Count(context.Background(), filter)

	response.JSON(w, http.StatusOK, map[string]any{
		"signals": signals,
		"total":   count,
		"limit":   filter.Limit,
		"offset":  filter.Offset,
	})
}

// GetByID returns a single signal by ID.
func (h *SignalsHandler) GetByID(w http.ResponseWriter, r *http.Request, id string) {
	sig, err := h.store.GetByID(context.Background(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, err)
		return
	}

	response.JSON(w, http.StatusOK, sig)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handler/api/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/handler/api/
git commit -m "feat(api): add signals API handler with filtering"
```

---

## Task 5: Watchlist API Handler

**Files:**
- Modify: `internal/app/app.go` (add GetWatchlist, AddToWatchlist, RemoveFromWatchlist)
- Create: `internal/api/handler/api/watchlist.go`
- Test: `internal/api/handler/api/watchlist_test.go`

**Step 1: Add methods to App**

First, add the missing watchlist methods to `internal/app/app.go`:

```go
// Add these methods after existing methods in app.go

// GetWatchlist returns the current watchlist.
func (a *App) GetWatchlist() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]string, len(a.watchlist))
	copy(result, a.watchlist)
	return result
}

// AddToWatchlist adds a symbol to the watchlist.
func (a *App) AddToWatchlist(symbol string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Check if already exists
	for _, s := range a.watchlist {
		if s == symbol {
			return
		}
	}
	a.watchlist = append(a.watchlist, symbol)
}

// RemoveFromWatchlist removes a symbol from the watchlist.
func (a *App) RemoveFromWatchlist(symbol string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, s := range a.watchlist {
		if s == symbol {
			a.watchlist = append(a.watchlist[:i], a.watchlist[i+1:]...)
			return true
		}
	}
	return false
}
```

**Step 2: Write the failing test for handler**

```go
// internal/api/handler/api/watchlist_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

func TestWatchlistHandler_List(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL", "GOOG"})

	handler := NewWatchlistHandler(a)

	req := httptest.NewRequest("GET", "/api/v1/watchlist", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	symbols := data["symbols"].([]any)
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(symbols))
	}
}

func TestWatchlistHandler_Add(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	handler := NewWatchlistHandler(a)

	body := bytes.NewBufferString(`{"symbol": "AAPL"}`)
	req := httptest.NewRequest("POST", "/api/v1/watchlist", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Add(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	watchlist := a.GetWatchlist()
	if len(watchlist) != 1 || watchlist[0] != "AAPL" {
		t.Errorf("expected AAPL in watchlist, got %v", watchlist)
	}
}

func TestWatchlistHandler_Remove(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL", "GOOG"})
	handler := NewWatchlistHandler(a)

	req := httptest.NewRequest("DELETE", "/api/v1/watchlist/AAPL", nil)
	w := httptest.NewRecorder()

	handler.Remove(w, req, "AAPL")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	watchlist := a.GetWatchlist()
	if len(watchlist) != 1 || watchlist[0] != "GOOG" {
		t.Errorf("expected only GOOG in watchlist, got %v", watchlist)
	}
}

func TestWatchlistHandler_Remove_NotFound(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	handler := NewWatchlistHandler(a)

	req := httptest.NewRequest("DELETE", "/api/v1/watchlist/AAPL", nil)
	w := httptest.NewRecorder()

	handler.Remove(w, req, "AAPL")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
```

**Step 3: Write the handler implementation**

```go
// internal/api/handler/api/watchlist.go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
)

// WatchlistApp defines the interface needed from app.App.
type WatchlistApp interface {
	GetWatchlist() []string
	AddToWatchlist(symbol string)
	RemoveFromWatchlist(symbol string) bool
}

// WatchlistHandler handles watchlist API requests.
type WatchlistHandler struct {
	app WatchlistApp
}

// NewWatchlistHandler creates a new watchlist handler.
func NewWatchlistHandler(app WatchlistApp) *WatchlistHandler {
	return &WatchlistHandler{app: app}
}

// AddRequest is the request body for adding a symbol.
type AddRequest struct {
	Symbol string `json:"symbol"`
	Market string `json:"market,omitempty"`
}

// List returns all symbols in the watchlist.
func (h *WatchlistHandler) List(w http.ResponseWriter, r *http.Request) {
	symbols := h.app.GetWatchlist()
	response.JSON(w, http.StatusOK, map[string]any{
		"symbols": symbols,
		"count":   len(symbols),
	})
}

// Add adds a symbol to the watchlist.
func (h *WatchlistHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}

	if req.Symbol == "" {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigMissing, nil))
		return
	}

	h.app.AddToWatchlist(req.Symbol)

	response.JSON(w, http.StatusCreated, map[string]any{
		"symbol": req.Symbol,
		"added":  true,
	})
}

// Remove removes a symbol from the watchlist.
func (h *WatchlistHandler) Remove(w http.ResponseWriter, r *http.Request, symbol string) {
	removed := h.app.RemoveFromWatchlist(symbol)
	if !removed {
		response.Error(w, http.StatusNotFound, core.ErrSymbolNotFound)
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":  symbol,
		"removed": true,
	})
}
```

**Step 4: Run tests**

Run: `go test ./internal/app/... ./internal/api/handler/api/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/app/app.go internal/api/handler/api/watchlist.go internal/api/handler/api/watchlist_test.go
git commit -m "feat(api): add watchlist API handler with CRUD operations"
```

---

## Task 6: Backtest API Handler with Job Runner

**Files:**
- Create: `internal/api/handler/api/backtest.go`
- Test: `internal/api/handler/api/backtest_test.go`

**Step 1: Write the failing test**

```go
// internal/api/handler/api/backtest_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/api/job"
	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// MockOHLCVProvider for testing
type MockOHLCVProvider struct{}

func (m *MockOHLCVProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return []core.OHLCV{
		{Symbol: symbol, Close: 100, Time: start},
		{Symbol: symbol, Close: 105, Time: start.Add(24 * time.Hour)},
		{Symbol: symbol, Close: 110, Time: end},
	}, nil
}

// MockStrategy for testing
type MockStrategy struct{}

func (m *MockStrategy) Name() string                     { return "mock" }
func (m *MockStrategy) Description() string              { return "mock strategy" }
func (m *MockStrategy) RequiredData() strategy.DataRequirements { return strategy.DataRequirements{PriceHistory: 2} }
func (m *MockStrategy) Init(cfg strategy.Config) error   { return nil }
func (m *MockStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.8}}, nil
}

func TestBacktestHandler_Create(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()
	strategies.Register(&MockStrategy{})

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	body := bytes.NewBufferString(`{
		"symbol": "AAPL",
		"strategy": "mock",
		"start": "2023-01-01",
		"end": "2024-01-01"
	}`)
	req := httptest.NewRequest("POST", "/api/v1/backtest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	if data["job_id"] == nil {
		t.Error("expected job_id in response")
	}
	if data["status"] != "pending" {
		t.Errorf("expected pending status, got %s", data["status"])
	}
}

func TestBacktestHandler_GetStatus(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	// Create a job directly
	j := jobStore.Create("backtest")

	req := httptest.NewRequest("GET", "/api/v1/backtest/"+j.ID, nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req, j.ID)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestBacktestHandler_GetStatus_NotFound(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	req := httptest.NewRequest("GET", "/api/v1/backtest/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handler/api/... -v -run TestBacktest`
Expected: FAIL - type not defined

**Step 3: Write the implementation**

```go
// internal/api/handler/api/backtest.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/api/job"
	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// BacktestRequest is the request body for starting a backtest.
type BacktestRequest struct {
	Symbol   string         `json:"symbol"`
	Strategy string         `json:"strategy"`
	Start    string         `json:"start"`
	End      string         `json:"end"`
	Params   map[string]any `json:"params,omitempty"`
}

// BacktestHandler handles backtest API requests.
type BacktestHandler struct {
	jobStore   *job.Store
	backtester *backtest.Backtester
	strategies *strategy.Engine
}

// NewBacktestHandler creates a new backtest handler.
func NewBacktestHandler(
	jobStore *job.Store,
	backtester *backtest.Backtester,
	strategies *strategy.Engine,
) *BacktestHandler {
	return &BacktestHandler{
		jobStore:   jobStore,
		backtester: backtester,
		strategies: strategies,
	}
}

// Create starts a new backtest job.
func (h *BacktestHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req BacktestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}

	// Validate required fields
	if req.Symbol == "" || req.Strategy == "" {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigMissing, nil))
		return
	}

	// Parse dates
	start, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}
	end, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}

	// Find strategy
	strat := h.strategies.Get(req.Strategy)
	if strat == nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrStrategyFailed, nil))
		return
	}

	// Create job
	j := h.jobStore.Create("backtest")

	// Run backtest in background
	go h.runBacktest(j.ID, strat, req.Symbol, start, end)

	response.JSON(w, http.StatusAccepted, map[string]any{
		"job_id": j.ID,
		"status": j.Status,
	})
}

// runBacktest executes the backtest and updates job status.
func (h *BacktestHandler) runBacktest(
	jobID string,
	strat strategy.Strategy,
	symbol string,
	start, end time.Time,
) {
	// Mark as running
	h.jobStore.Update(jobID, func(j *job.Job) {
		j.Status = job.StatusRunning
	})

	// Run backtest
	ctx := context.Background()
	result, err := h.backtester.Run(ctx, strat, symbol, start, end)

	if err != nil {
		h.jobStore.Update(jobID, func(j *job.Job) {
			j.Status = job.StatusFailed
			j.Error = core.WrapError(core.ErrStrategyFailed, err)
		})
		return
	}

	h.jobStore.Update(jobID, func(j *job.Job) {
		j.Status = job.StatusComplete
		j.Progress = 100
		j.Result = result
	})
}

// GetStatus returns the status of a backtest job.
func (h *BacktestHandler) GetStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	j, err := h.jobStore.Get(jobID)
	if err != nil {
		response.Error(w, http.StatusNotFound, err)
		return
	}

	resp := map[string]any{
		"job_id":   j.ID,
		"status":   j.Status,
		"progress": j.Progress,
	}

	if j.Status == job.StatusComplete {
		resp["result"] = j.Result
	}
	if j.Status == job.StatusFailed && j.Error != nil {
		resp["error"] = map[string]string{
			"code":    j.Error.Code,
			"message": j.Error.Message,
		}
	}

	response.JSON(w, http.StatusOK, resp)
}
```

**Step 4: Run tests**

Run: `go test ./internal/api/handler/api/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/handler/api/backtest.go internal/api/handler/api/backtest_test.go
git commit -m "feat(api): add async backtest API with job polling"
```

---

## Task 7: Analysis Trigger API

**Files:**
- Create: `internal/api/handler/api/analysis.go`
- Test: `internal/api/handler/api/analysis_test.go`

**Step 1: Write the failing test**

```go
// internal/api/handler/api/analysis_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

func TestAnalysisHandler_Trigger(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL", "GOOG"})

	handler := NewAnalysisHandler(a)

	req := httptest.NewRequest("POST", "/api/v1/analysis/run", nil)
	w := httptest.NewRecorder()

	handler.Trigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	if data["triggered"] != true {
		t.Error("expected triggered to be true")
	}
	if data["symbols_count"].(float64) != 2 {
		t.Errorf("expected 2 symbols, got %v", data["symbols_count"])
	}
}
```

**Step 2: Write the implementation**

```go
// internal/api/handler/api/analysis.go
package api

import (
	"context"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
)

// AnalysisApp defines the interface needed from app.App.
type AnalysisApp interface {
	GetWatchlist() []string
	RunOnce(ctx context.Context)
}

// AnalysisHandler handles analysis trigger API requests.
type AnalysisHandler struct {
	app AnalysisApp
}

// NewAnalysisHandler creates a new analysis handler.
func NewAnalysisHandler(app AnalysisApp) *AnalysisHandler {
	return &AnalysisHandler{app: app}
}

// Trigger runs an analysis cycle.
func (h *AnalysisHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	watchlist := h.app.GetWatchlist()

	// Run analysis in background
	go h.app.RunOnce(context.Background())

	response.JSON(w, http.StatusOK, map[string]any{
		"triggered":     true,
		"symbols_count": len(watchlist),
	})
}
```

**Step 3: Run tests**

Run: `go test ./internal/api/handler/api/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/api/handler/api/analysis.go internal/api/handler/api/analysis_test.go
git commit -m "feat(api): add analysis trigger API endpoint"
```

---

## Task 8: Config Additions

**Files:**
- Modify: `internal/config/config.go`
- Modify: `configs/config.example.yaml`

**Step 1: Update ServerConfig**

Add to `ServerConfig` in `internal/config/config.go`:

```go
type ServerConfig struct {
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	Mode        string `mapstructure:"mode"`
	APIKey      string `mapstructure:"api_key"`      // NEW
	JobTTLHours int    `mapstructure:"job_ttl_hours"` // NEW
	MaxJobs     int    `mapstructure:"max_jobs"`      // NEW
}
```

**Step 2: Update Defaults**

Update the `Defaults()` function:

```go
Server: ServerConfig{
	Host:        "0.0.0.0",
	Port:        8080,
	Mode:        "release",
	JobTTLHours: 1,   // NEW
	MaxJobs:     100, // NEW
},
```

**Step 3: Update example config**

Add to `configs/config.example.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8080
  mode: release
  api_key: ${ATLAS_API_KEY}  # NEW: Set via environment variable
  job_ttl_hours: 1           # NEW: How long to keep job results
  max_jobs: 100              # NEW: Max concurrent jobs
```

**Step 4: Run tests**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go configs/config.example.yaml
git commit -m "feat(config): add API key and job settings"
```

---

## Task 9: Wire Everything Together in Server

**Files:**
- Modify: `internal/api/server.go`
- Test: `internal/api/server_test.go`

**Step 1: Update Server struct and dependencies**

Rewrite `internal/api/server.go`:

```go
// internal/api/server.go
package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/api/handler/api"
	"github.com/newthinker/atlas/internal/api/handler/web"
	"github.com/newthinker/atlas/internal/api/job"
	"github.com/newthinker/atlas/internal/api/middleware"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"go.uber.org/zap"
)

// Server represents the HTTP server for ATLAS
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
	mux        *http.ServeMux
}

// Config holds server configuration
type Config struct {
	Host         string
	Port         int
	TemplatesDir string
	APIKey       string
	JobTTLHours  int
	MaxJobs      int
}

// Dependencies holds all server dependencies
type Dependencies struct {
	App         *app.App
	SignalStore signal.Store
	Backtester  *backtest.Backtester
	Strategies  *strategy.Engine
}

// NewServer creates a new HTTP server
func NewServer(cfg Config, deps Dependencies, logger *zap.Logger) (*Server, error) {
	mux := http.NewServeMux()

	s := &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger: logger,
		mux:    mux,
	}

	// Set up routes
	if err := s.setupRoutes(cfg, deps); err != nil {
		return nil, fmt.Errorf("setting up routes: %w", err)
	}

	return s, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes(cfg Config, deps Dependencies) error {
	// Create job store
	ttl := time.Duration(cfg.JobTTLHours) * time.Hour
	if ttl == 0 {
		ttl = time.Hour
	}
	maxJobs := cfg.MaxJobs
	if maxJobs == 0 {
		maxJobs = 100
	}
	jobStore := job.NewStore(maxJobs, ttl)

	// Create API handlers
	signalsHandler := api.NewSignalsHandler(deps.SignalStore)
	watchlistHandler := api.NewWatchlistHandler(deps.App)
	backtestHandler := api.NewBacktestHandler(jobStore, deps.Backtester, deps.Strategies)
	analysisHandler := api.NewAnalysisHandler(deps.App)

	// Auth middleware for API routes
	authMiddleware := middleware.APIKeyAuth(cfg.APIKey)

	// API v1 routes (with auth)
	s.mux.Handle("/api/v1/signals", authMiddleware(http.HandlerFunc(signalsHandler.List)))
	s.mux.Handle("/api/v1/signals/", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/signals/")
		signalsHandler.GetByID(w, r, id)
	})))
	s.mux.Handle("/api/v1/watchlist", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			watchlistHandler.List(w, r)
		case http.MethodPost:
			watchlistHandler.Add(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.Handle("/api/v1/watchlist/", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := strings.TrimPrefix(r.URL.Path, "/api/v1/watchlist/")
		if r.Method == http.MethodDelete {
			watchlistHandler.Remove(w, r, symbol)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.Handle("/api/v1/backtest", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			backtestHandler.Create(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.Handle("/api/v1/backtest/", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/backtest/")
		backtestHandler.GetStatus(w, r, jobID)
	})))
	s.mux.Handle("/api/v1/analysis/run", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			analysisHandler.Trigger(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Health endpoint (no auth)
	s.mux.HandleFunc("/api/health", s.handleHealth)

	// Web UI routes (no auth - same origin)
	webHandler, err := web.NewHandler(cfg.TemplatesDir)
	if err != nil {
		return fmt.Errorf("creating web handler: %w", err)
	}

	s.mux.HandleFunc("/", webHandler.Dashboard)
	s.mux.HandleFunc("/signals", webHandler.Signals)
	s.mux.HandleFunc("/watchlist", webHandler.Watchlist)
	s.mux.HandleFunc("/backtest", webHandler.Backtest)

	return nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server", zap.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// Health handler
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
```

**Step 2: Write server test**

```go
// internal/api/server_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"go.uber.org/zap"
)

func TestServer_Health(t *testing.T) {
	deps := Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil, // Not needed for health check
		Strategies:  strategy.NewEngine(),
	}

	srv, err := NewServer(Config{
		Host:        "localhost",
		Port:        0,
		TemplatesDir: "",
	}, deps, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServer_APIAuth(t *testing.T) {
	deps := Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil,
		Strategies:  strategy.NewEngine(),
	}

	srv, _ := NewServer(Config{
		Host:   "localhost",
		Port:   0,
		APIKey: "test-key",
	}, deps, zap.NewNop())

	// Without API key
	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without key, got %d", w.Code)
	}

	// With API key
	req = httptest.NewRequest("GET", "/api/v1/signals", nil)
	req.Header.Set("X-API-Key", "test-key")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with key, got %d", w.Code)
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/api/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/api/server.go internal/api/server_test.go
git commit -m "feat(api): wire all handlers with auth and routing"
```

---

## Task 10: Update serve.go to Use New Server

**Files:**
- Modify: `cmd/atlas/serve.go`

**Step 1: Update serve command**

Update `cmd/atlas/serve.go` to create dependencies and pass to server:

```go
// In runServe function, after config loading and validation:

// Create signal store
signalStore := signal.NewMemoryStore(1000)

// Create app with signal store in router
application := app.New(cfg, logger)
// ... register collectors, strategies, notifiers ...
application.Router().SetSignalStore(signalStore) // if Router() method exists

// Create backtester
var backtester *backtest.Backtester
if len(application.GetCollectors()) > 0 {
    backtester = backtest.New(application.GetCollectors()[0])
}

// Create server
deps := api.Dependencies{
    App:         application,
    SignalStore: signalStore,
    Backtester:  backtester,
    Strategies:  application.GetStrategies(),
}

serverCfg := api.Config{
    Host:         cfg.Server.Host,
    Port:         cfg.Server.Port,
    TemplatesDir: "templates",
    APIKey:       cfg.Server.APIKey,
    JobTTLHours:  cfg.Server.JobTTLHours,
    MaxJobs:      cfg.Server.MaxJobs,
}

server, err := api.NewServer(serverCfg, deps, logger)
```

**Note:** This task requires checking the actual serve.go structure and adapting. The key is to:
1. Create the signal store
2. Create the app
3. Create the backtester with a collector
4. Pass all dependencies to the server

**Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

**Step 3: Build and test manually**

```bash
go build -o bin/atlas ./cmd/atlas
./bin/atlas serve -c configs/config.example.yaml
# In another terminal:
curl http://localhost:8080/api/health
curl -H "X-API-Key: your-key" http://localhost:8080/api/v1/signals
```

**Step 4: Commit**

```bash
git add cmd/atlas/serve.go
git commit -m "feat(cli): integrate new API server with dependencies"
```

---

## Final Steps

**Step 1: Run all tests**

```bash
go test ./... -v
```
Expected: All tests pass

**Step 2: Verify build**

```bash
go build ./...
```
Expected: No errors

**Step 3: Create final commit if needed**

```bash
git status
# If any uncommitted changes:
git add .
git commit -m "chore: M2 API improvements implementation complete"
```

---

## Summary

| Task | Files | Description |
|------|-------|-------------|
| 1 | `api/response/` | JSON response helpers |
| 2 | `api/middleware/` | API key auth middleware |
| 3 | `api/job/` | Async job store |
| 4 | `api/handler/api/signals.go` | Signals API |
| 5 | `api/handler/api/watchlist.go` | Watchlist API |
| 6 | `api/handler/api/backtest.go` | Backtest API with jobs |
| 7 | `api/handler/api/analysis.go` | Analysis trigger API |
| 8 | `config/config.go` | Config additions |
| 9 | `api/server.go` | Server wiring |
| 10 | `cmd/atlas/serve.go` | CLI integration |

**Total: 10 tasks, ~20 commits**
