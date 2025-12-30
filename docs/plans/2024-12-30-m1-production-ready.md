# M1: Production Ready Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make ATLAS production-ready with signal persistence, standardized error handling, and config validation.

**Architecture:** Add unified error types in core/, signal storage layer with interface + memory implementation, integrate persistence into Router, add config validation at startup.

**Tech Stack:** Go 1.24, sync.RWMutex for thread-safety, interface-based design for storage backends.

---

## Task 1: Define Unified Error Types

**Files:**
- Create: `internal/core/errors.go`
- Test: `internal/core/errors_test.go`

**Step 1: Write the failing test**

```go
// internal/core/errors_test.go
package core

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	err := &Error{Code: "TEST_ERROR", Message: "test message"}
	if err.Error() != "[TEST_ERROR] test message" {
		t.Errorf("unexpected error string: %s", err.Error())
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &Error{Code: "WRAP", Message: "wrapped", Cause: cause}
	if !errors.Is(err, cause) {
		t.Error("Unwrap should return cause")
	}
}

func TestError_Is(t *testing.T) {
	if !errors.Is(ErrSymbolNotFound, ErrSymbolNotFound) {
		t.Error("same error should match")
	}
}

func TestWrapError(t *testing.T) {
	cause := errors.New("original")
	wrapped := WrapError(ErrCollectorFailed, cause)
	if wrapped.Cause != cause {
		t.Error("cause not set")
	}
	if wrapped.Code != ErrCollectorFailed.Code {
		t.Error("code not preserved")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/errors_test.go -v`
Expected: FAIL - file not found

**Step 3: Write minimal implementation**

```go
// internal/core/errors.go
package core

import "fmt"

// Error represents a structured error with code and optional cause.
type Error struct {
	Code    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause for errors.Is/As support.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is implements errors.Is matching by code.
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.Code == t.Code
	}
	return false
}

// WrapError creates a new error with the same code but with a cause.
func WrapError(base *Error, cause error) *Error {
	return &Error{
		Code:    base.Code,
		Message: base.Message,
		Cause:   cause,
	}
}

// Predefined errors
var (
	// Data errors
	ErrSymbolNotFound = &Error{Code: "SYMBOL_NOT_FOUND", Message: "symbol not found"}
	ErrNoData         = &Error{Code: "NO_DATA", Message: "no data available"}

	// Collector errors
	ErrCollectorFailed   = &Error{Code: "COLLECTOR_FAILED", Message: "collector failed"}
	ErrCollectorTimeout  = &Error{Code: "COLLECTOR_TIMEOUT", Message: "collector timeout"}

	// Strategy errors
	ErrStrategyFailed    = &Error{Code: "STRATEGY_FAILED", Message: "strategy analysis failed"}
	ErrInsufficientData  = &Error{Code: "INSUFFICIENT_DATA", Message: "insufficient data for analysis"}

	// Notifier errors
	ErrNotifierFailed    = &Error{Code: "NOTIFIER_FAILED", Message: "notifier failed"}

	// Broker errors
	ErrBrokerDisconnected = &Error{Code: "BROKER_DISCONNECTED", Message: "broker not connected"}
	ErrOrderFailed        = &Error{Code: "ORDER_FAILED", Message: "order failed"}

	// Config errors
	ErrConfigInvalid     = &Error{Code: "CONFIG_INVALID", Message: "configuration invalid"}
	ErrConfigMissing     = &Error{Code: "CONFIG_MISSING", Message: "required configuration missing"}

	// LLM errors
	ErrLLMFailed         = &Error{Code: "LLM_FAILED", Message: "LLM request failed"}
	ErrLLMTimeout        = &Error{Code: "LLM_TIMEOUT", Message: "LLM request timeout"}
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/errors.go internal/core/errors_test.go
git commit -m "feat(core): add unified error types with codes and wrap support"
```

---

## Task 2: Signal Storage Interface and Memory Implementation

**Files:**
- Create: `internal/storage/signal/interface.go`
- Create: `internal/storage/signal/memory.go`
- Test: `internal/storage/signal/memory_test.go`

**Step 1: Write the failing test**

```go
// internal/storage/signal/memory_test.go
package signal

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

func TestMemoryStore_SaveAndList(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	sig := core.Signal{
		Symbol:     "AAPL",
		Action:     core.ActionBuy,
		Confidence: 0.85,
		Strategy:   "ma_crossover",
		GeneratedAt: time.Now(),
	}

	err := store.Save(ctx, sig)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	signals, err := store.List(ctx, ListFilter{Symbol: "AAPL"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(signals))
	}
}

func TestMemoryStore_ListByStrategy(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	store.Save(ctx, core.Signal{Symbol: "AAPL", Strategy: "ma_crossover", GeneratedAt: time.Now()})
	store.Save(ctx, core.Signal{Symbol: "GOOG", Strategy: "pe_band", GeneratedAt: time.Now()})

	signals, _ := store.List(ctx, ListFilter{Strategy: "ma_crossover"})
	if len(signals) != 1 {
		t.Errorf("expected 1, got %d", len(signals))
	}
}

func TestMemoryStore_ListByTimeRange(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	now := time.Now()
	store.Save(ctx, core.Signal{Symbol: "AAPL", GeneratedAt: now.Add(-2 * time.Hour)})
	store.Save(ctx, core.Signal{Symbol: "GOOG", GeneratedAt: now})

	signals, _ := store.List(ctx, ListFilter{From: now.Add(-1 * time.Hour)})
	if len(signals) != 1 {
		t.Errorf("expected 1, got %d", len(signals))
	}
}

func TestMemoryStore_MaxSize(t *testing.T) {
	store := NewMemoryStore(2)
	ctx := context.Background()

	store.Save(ctx, core.Signal{Symbol: "A", GeneratedAt: time.Now()})
	store.Save(ctx, core.Signal{Symbol: "B", GeneratedAt: time.Now()})
	store.Save(ctx, core.Signal{Symbol: "C", GeneratedAt: time.Now()})

	signals, _ := store.List(ctx, ListFilter{})
	if len(signals) != 2 {
		t.Errorf("expected 2 (max size), got %d", len(signals))
	}
}

func TestMemoryStore_GetByID(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	sig := core.Signal{Symbol: "AAPL", GeneratedAt: time.Now()}
	store.Save(ctx, sig)

	signals, _ := store.List(ctx, ListFilter{})
	if len(signals) == 0 {
		t.Fatal("no signals saved")
	}

	retrieved, err := store.GetByID(ctx, signals[0].ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Symbol != "AAPL" {
		t.Errorf("wrong symbol: %s", retrieved.Symbol)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/signal/... -v`
Expected: FAIL - package not found

**Step 3: Write interface**

```go
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
```

**Step 4: Write memory implementation**

```go
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
```

**Step 5: Add ID field to core.Signal**

```go
// Modify internal/core/types.go - add ID field to Signal struct
// Add this field at the beginning of the Signal struct:
// ID string `json:"id"`
```

**Step 6: Run test to verify it passes**

Run: `go test ./internal/storage/signal/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/storage/signal/ internal/core/types.go
git commit -m "feat(storage): add signal persistence with memory store"
```

---

## Task 3: Integrate Signal Storage into Router

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`

**Step 1: Write the failing test**

```go
// Add to internal/router/router_test.go
func TestRouter_PersistsSignals(t *testing.T) {
	store := signal.NewMemoryStore(100)
	r := New(Config{MinConfidence: 0.5, CooldownDuration: time.Hour}, nil, nil)
	r.SetSignalStore(store)

	sig := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	r.Route(sig)

	signals, _ := store.List(context.Background(), signal.ListFilter{})
	if len(signals) != 1 {
		t.Errorf("expected 1 persisted signal, got %d", len(signals))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/router/... -v -run TestRouter_PersistsSignals`
Expected: FAIL - SetSignalStore not defined

**Step 3: Modify Router to support signal store**

```go
// Add to internal/router/router.go

import (
	// add this import
	"github.com/newthinker/atlas/internal/storage/signal"
)

// Add field to Router struct:
// signalStore signal.Store

// Add method:
func (r *Router) SetSignalStore(store signal.Store) {
	r.signalStore = store
}

// Modify Route method - after passing filters, before notifying:
// if r.signalStore != nil {
//     if err := r.signalStore.Save(context.Background(), sig); err != nil {
//         r.logger.Error("failed to persist signal", zap.Error(err))
//     }
// }
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/router/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/router/
git commit -m "feat(router): integrate signal persistence"
```

---

## Task 4: Config Validation

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
// Add to internal/config/config_test.go
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
			},
			wantErr: false,
		},
		{
			name: "invalid port - zero",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 0},
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 70000},
			},
			wantErr: true,
		},
		{
			name: "invalid router confidence",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
				Router: RouterConfig{MinConfidence: 1.5},
			},
			wantErr: true,
		},
		{
			name: "negative cooldown",
			cfg: Config{
				Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
				Router: RouterConfig{CooldownHours: -1},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -v -run TestConfig_Validate`
Expected: FAIL - Validate not defined

**Step 3: Add Validate method**

```go
// Add to internal/config/config.go

import (
	"github.com/newthinker/atlas/internal/core"
)

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("port must be between 1 and 65535, got %d", c.Server.Port))
	}

	// Router validation
	if c.Router.MinConfidence < 0 || c.Router.MinConfidence > 1 {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("min_confidence must be between 0 and 1, got %f", c.Router.MinConfidence))
	}
	if c.Router.CooldownHours < 0 {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("cooldown_hours cannot be negative, got %d", c.Router.CooldownHours))
	}

	// LLM validation - if provider set, check config exists
	if c.LLM.Provider != "" {
		switch c.LLM.Provider {
		case "claude":
			if c.LLM.Claude.APIKey == "" {
				return core.WrapError(core.ErrConfigMissing,
					fmt.Errorf("claude api_key required when provider is claude"))
			}
		case "openai":
			if c.LLM.OpenAI.APIKey == "" {
				return core.WrapError(core.ErrConfigMissing,
					fmt.Errorf("openai api_key required when provider is openai"))
			}
		case "ollama":
			if c.LLM.Ollama.Endpoint == "" {
				return core.WrapError(core.ErrConfigMissing,
					fmt.Errorf("ollama endpoint required when provider is ollama"))
			}
		}
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add validation with structured errors"
```

---

## Task 5: Startup Config Validation

**Files:**
- Modify: `cmd/atlas/serve.go`

**Step 1: Locate current config loading**

The serve command loads config but doesn't validate it. Find the `runServe` function.

**Step 2: Add validation after loading**

```go
// In cmd/atlas/serve.go, after config.Load():

cfg, err := config.Load(cfgFile)
if err != nil {
    return fmt.Errorf("loading config: %w", err)
}

// Add validation
if err := cfg.Validate(); err != nil {
    return fmt.Errorf("config validation failed: %w", err)
}
```

**Step 3: Run manually to verify**

Run: `go build -o bin/atlas ./cmd/atlas && ./bin/atlas serve -c config.example.yaml`
Expected: Starts successfully (config is valid)

Run with invalid config:
```bash
echo "server:\n  port: 0" > /tmp/bad.yaml
./bin/atlas serve -c /tmp/bad.yaml
```
Expected: Error message about invalid port

**Step 4: Commit**

```bash
git add cmd/atlas/serve.go
git commit -m "feat(cli): validate config at startup"
```

---

## Task 6: Standardize Error Handling in Meta Package

**Files:**
- Modify: `internal/meta/arbitrator.go`
- Modify: `internal/meta/synthesizer.go`

**Step 1: Review current error handling**

Current code silently ignores errors:
```go
marketCtx, _ := a.marketContext.GetContext(ctx, req.Market)
news, _ := a.newsProvider.GetNews(ctx, req.Symbol, a.contextDays)
```

**Step 2: Add proper error handling with logging**

```go
// In internal/meta/arbitrator.go, update the Arbitrate method:

// Replace silent ignores with logged fallbacks:
marketCtx, err := a.marketContext.GetContext(ctx, req.Market)
if err != nil {
    a.logger.Warn("failed to get market context, using defaults",
        zap.String("market", string(req.Market)),
        zap.Error(err))
    marketCtx = &context.MarketContext{Regime: "unknown", Volatility: 0}
}

news, err := a.newsProvider.GetNews(ctx, req.Symbol, a.contextDays)
if err != nil {
    a.logger.Warn("failed to get news, proceeding without",
        zap.String("symbol", req.Symbol),
        zap.Error(err))
    news = []context.NewsItem{}
}

allStats, err := a.trackRecord.GetAllStats(ctx)
if err != nil {
    a.logger.Warn("failed to get track records, proceeding without",
        zap.Error(err))
    allStats = make(map[string]*context.StrategyStats)
}
```

**Step 3: Update synthesizer similarly**

Apply same pattern to `internal/meta/synthesizer.go`.

**Step 4: Run tests**

Run: `go test ./internal/meta/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/meta/
git commit -m "fix(meta): standardize error handling with graceful degradation"
```

---

## Final Steps

**Step 1: Run all tests**

```bash
go test ./... -v
```
Expected: All tests pass

**Step 2: Create final commit for any remaining changes**

```bash
git status
# If any uncommitted changes:
git add .
git commit -m "chore: M1 production ready implementation complete"
```

**Step 3: Push branch**

```bash
git push -u origin feature/m1-production-ready
```

---

## Summary

| Task | Files | Description |
|------|-------|-------------|
| 1 | `core/errors.go` | Unified error types |
| 2 | `storage/signal/*` | Signal persistence layer |
| 3 | `router/router.go` | Router integration |
| 4 | `config/config.go` | Config validation |
| 5 | `cmd/atlas/serve.go` | Startup validation |
| 6 | `meta/*.go` | Error handling standardization |

**Total: 6 tasks, ~15 commits**
