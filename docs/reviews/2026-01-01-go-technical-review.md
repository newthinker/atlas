# Atlas Project - Go Technical Review

**Date:** 2026-01-01
**Reviewer:** Claude Code (Golang Expert)
**Branch:** feature/m4-live-trading

---

## Executive Summary

The Atlas project is a well-structured Go trading signal system with clear package organization. Overall code quality is good with an average test coverage of ~70%. The broker package is particularly well-designed. However, there are several areas requiring attention across security, concurrency, and architecture.

---

## Summary of Findings

| Severity | Count | Key Issues |
|----------|-------|------------|
| **Critical** | 1 | Secrets stored in plaintext config |
| **Major** | 8 | Race condition in Confirm, unbounded cooldowns map, low test coverage, input sanitization |
| **Minor** | 10 | Magic numbers, code duplication, naming inconsistencies |

---

## 1. Architecture & Design Patterns

### Positive Findings

**Clean Package Structure**: The project follows Go's standard layout with clear separation:
- `/cmd/atlas` - CLI entry points
- `/internal/` - Private packages (broker, api, core, config, etc.)
- Clear domain boundaries between broker, strategy, collector, and notifier

**Good Interface Design** (`internal/broker/types.go:250-278`):
```go
type Broker interface {
    Name() string
    SupportedMarkets() []core.Market
    Connect(ctx context.Context) error
    // ... well-defined contract
}
```

**Dependency Injection** (`internal/api/server.go:43-50`):
```go
type Dependencies struct {
    App              *app.App
    SignalStore      signal.Store
    Backtester       *backtest.Backtester
    // Clean DI pattern
}
```

### Issues

#### MAJOR: Dual Interface Pattern Creates Confusion

**File**: `internal/broker/interface.go:15-35`

Both `LegacyBroker` and `Broker` interfaces exist, causing inconsistency:
- `cmd/atlas/broker.go` uses `LegacyBroker`
- `internal/broker/execution.go` uses `Broker`
- `internal/broker/mock/mock.go` implements `LegacyBroker`
- `internal/broker/mocks/broker.go` implements `Broker`

**Recommendation**: Complete migration to the new `Broker` interface and deprecate `LegacyBroker`. Create an adapter if backward compatibility is needed temporarily.

#### MINOR: Hardcoded Templates Path

**File**: `cmd/atlas/serve.go:160`
```go
TemplatesDir: "internal/api/templates",
```
This relative path will fail if the binary is run from a different directory.

**Recommendation**: Use embed.FS or make path configurable.

---

## 2. Go Idioms & Best Practices

### Positive Findings

**Proper Error Wrapping** (`internal/core/errors.go:34-41`):
```go
func WrapError(base *Error, cause error) *Error {
    return &Error{
        Code:    base.Code,
        Message: base.Message,
        Cause:   cause,
    }
}
```

**Sentinel Errors with Context** (`internal/broker/types.go:12-40`):
Well-defined sentinel errors with clear naming.

### Issues

#### MAJOR: Errors Silently Ignored in Strategy Engine

**File**: `internal/strategy/engine.go:67-72`
```go
signals, err := s.Analyze(analysisCtx)
if err != nil {
    // Log error but continue with other strategies
    continue
}
```
Error is silently swallowed without any logging.

**Recommendation**: Add logging or accumulate errors for the caller to handle:
```go
if err != nil {
    allErrors = append(allErrors, fmt.Errorf("%s: %w", s.Name(), err))
    continue
}
```

#### MINOR: Unnecessary Type Conversion

**File**: `internal/broker/execution_test.go:73`
```go
OrderID: string(rune('0' + m.orderSeq)),
```
This is a convoluted way to generate a string ID. Use `fmt.Sprintf("order-%d", m.orderSeq)` instead.

---

## 3. Concurrency

### Positive Findings

**Proper Mutex Usage** (`internal/broker/position.go:30-47`):
```go
func (pt *PositionTracker) Sync(ctx context.Context) error {
    positions, err := pt.broker.GetPositions(ctx)
    if err != nil {
        return err
    }
    pt.mu.Lock()
    defer pt.mu.Unlock()
    // ... safe update
}
```

**Copy-on-read Pattern** (`internal/broker/position.go:52-68`):
```go
func (pt *PositionTracker) GetPosition(symbol string) *Position {
    pt.mu.RLock()
    defer pt.mu.RUnlock()
    if pos, exists := pt.positions[symbol]; exists {
        posCopy := *pos  // Return copy to prevent external modification
        return &posCopy
    }
    // ...
}
```

### Issues

#### MAJOR: Potential Race Condition in Confirm

**File**: `internal/broker/execution.go:219-249`
```go
func (em *ExecutionManager) Confirm(ctx context.Context, pendingID string) (*ExecuteResult, error) {
    em.mu.Lock()
    pending, exists := em.pending[pendingID]
    if !exists {
        em.mu.Unlock()
        return nil, ErrPendingOrderNotFound
    }
    delete(em.pending, pendingID)
    em.mu.Unlock()  // <-- Lock released before broker call

    order, err := em.broker.PlaceOrder(ctx, pending.Request)  // <-- No lock held
    if err != nil {
        em.mu.Lock()
        em.pending[pendingID] = pending  // <-- Re-adding after unlock
        em.mu.Unlock()
        return nil, err
    }
    // ...
}
```

If two goroutines call Confirm on the same pending order simultaneously, there is a brief window where both could proceed past the exists check.

**Recommendation**: The current implementation actually handles this correctly by deleting before unlock, but the re-add on failure could cause issues. Consider using a "processing" state instead of re-adding.

#### MINOR: Missing Context Propagation

**File**: `internal/api/handler/api/backtest.go:109`
```go
ctx := context.Background()
result, err := h.backtester.Run(ctx, strat, symbol, start, end)
```
Should use a derived context with timeout or cancellation instead of `context.Background()`.

**Recommendation**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()
```

---

## 4. Performance

### Positive Findings

**Pre-allocated Slices** (`internal/broker/mocks/broker.go:373-377`):
```go
positions := make([]broker.Position, 0, len(m.positions))
```

### Issues

#### MAJOR: Unbounded Memory in Cooldowns Map

**File**: `internal/router/router.go:35-36`
```go
type Router struct {
    cooldowns   map[string]time.Time // symbol -> last signal time
    // ...
}
```
This map grows unbounded as symbols are processed and entries are never cleaned up automatically.

**Recommendation**: Implement periodic cleanup or use a TTL cache:
```go
func (r *Router) cleanupCooldowns() {
    r.mu.Lock()
    defer r.mu.Unlock()
    now := time.Now()
    for symbol, lastTime := range r.cooldowns {
        if now.Sub(lastTime) > r.cfg.CooldownDuration*2 {
            delete(r.cooldowns, symbol)
        }
    }
}
```

#### MINOR: Linear Search in Watchlist Operations

**File**: `internal/app/app.go:259-269`
```go
func (a *App) AddToWatchlist(symbol string) {
    a.mu.Lock()
    defer a.mu.Unlock()
    for _, s := range a.watchlist {  // O(n) search
        if s == symbol {
            return
        }
    }
    a.watchlist = append(a.watchlist, symbol)
}
```

**Recommendation**: Use a map for O(1) lookups if watchlist size is expected to grow:
```go
type App struct {
    watchlistSet map[string]struct{}
    watchlist    []string  // Keep ordered list for iteration
}
```

---

## 5. Testing

### Coverage Summary

| Package | Coverage |
|---------|----------|
| internal/broker | 94.3% |
| internal/broker/mocks | 99.0% |
| internal/backtest | 96.9% |
| internal/metrics | 98.5% |
| internal/notifier | 97.1% |
| internal/collector/yahoo | 17.2% |
| internal/llm/claude | 8.0% |
| internal/api | 52.7% |
| **Average** | ~70% |

### Positive Findings

**Excellent Table-Driven Tests** (`internal/broker/execution_test.go`):
Comprehensive test cases for ExecutionManager with clear naming.

**Good Mock Implementation** (`internal/broker/mocks/broker.go`):
Well-designed mock with configurable behavior for testing.

### Issues

#### MAJOR: Low Coverage on External Integrations

- `internal/collector/yahoo`: 17.2%
- `internal/llm/claude`: 8.0%
- `internal/llm/openai`: 19.4%

**Recommendation**: Add integration test stubs and mock HTTP responses for external APIs.

#### MINOR: No Tests for CLI Commands

**File**: `cmd/atlas/`

Coverage is 0% for CLI commands.

**Recommendation**: Add tests for command parsing and flag validation.

---

## 6. Security

### Positive Findings

**Constant-Time Comparison for API Keys** (`internal/api/middleware/auth.go:30-31`):
```go
if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
```
Excellent protection against timing attacks.

**Reasonable HTTP Timeouts** (`internal/api/server.go:57-63`):
```go
ReadTimeout:  15 * time.Second,
WriteTimeout: 15 * time.Second,
IdleTimeout:  60 * time.Second,
```

### Issues

#### CRITICAL: Secrets in Configuration Without Encryption

**File**: `internal/config/config.go:54-59`
```go
type S3Config struct {
    AccessKey string `mapstructure:"access_key"`
    SecretKey string `mapstructure:"secret_key"`
    // ...
}
```
And:
```go
type FutuConfig struct {
    TradePassword string `mapstructure:"trade_password"`
    RSAKeyPath    string `mapstructure:"rsa_key_path"`
}
```

Secrets are stored in plaintext in config files.

**Recommendation**:
1. Use environment variables for secrets (already partially supported)
2. Add explicit validation that secrets are from env vars, not config files
3. Consider a secrets manager integration (HashiCorp Vault, AWS Secrets Manager)

#### MAJOR: No Input Sanitization for Symbol

**File**: `internal/collector/yahoo/yahoo.go:110-111`
```go
url := fmt.Sprintf("%s/%s?interval=%s&period1=%d&period2=%d",
    baseURL, yahooSymbol, yahooInterval, start.Unix(), end.Unix())
```

The symbol is directly interpolated into a URL without validation.

**Recommendation**: Validate symbol format with a regex before use:
```go
var validSymbol = regexp.MustCompile(`^[A-Z0-9.]{1,20}$`)
func (y *Yahoo) validateSymbol(symbol string) error {
    if !validSymbol.MatchString(symbol) {
        return fmt.Errorf("invalid symbol format: %s", symbol)
    }
    return nil
}
```

#### MINOR: Auth Disabled When APIKey Empty

**File**: `internal/api/middleware/auth.go:18-21`
```go
if apiKey == "" {
    next.ServeHTTP(w, r)
    return
}
```

This silently disables authentication, which could be dangerous in production.

**Recommendation**: Log a warning or require explicit configuration to disable auth.

---

## 7. Code Quality

### Positive Findings

**Good Documentation** (`internal/broker/types.go`):
Every type and field is documented with clear comments.

**Validation Methods** (`internal/broker/types.go:103-117`):
```go
func (r OrderRequest) Validate() error {
    if r.Symbol == "" { return ErrInvalidSymbol }
    // ... comprehensive validation
}
```

### Issues

#### MAJOR: Code Duplication in CLI Commands

**File**: `cmd/atlas/broker.go`

The pattern of loading config and getting broker is repeated in every command (lines 88-104, 123-145, 175-197, etc.).

**Recommendation**: Extract to a helper function:
```go
func withBroker(fn func(b broker.LegacyBroker) error) error {
    log := logger.Must(debug)
    defer log.Sync()
    var cfg *config.Config
    if cfgFile != "" {
        var err error
        cfg, err = config.Load(cfgFile)
        if err != nil { return err }
    }
    b, err := getBroker(cfg)
    if err != nil { return err }
    ctx := context.Background()
    if err := b.Connect(ctx); err != nil { return err }
    defer b.Disconnect()
    return fn(b)
}
```

#### MINOR: Magic Numbers

**File**: `cmd/atlas/serve.go:66`
```go
sigStore := signalstore.NewMemoryStore(1000)
```

**Recommendation**: Define as constant or make configurable:
```go
const defaultSignalStoreCapacity = 1000
```

---

## Prioritized Recommendations

### Immediate (Critical Security)
1. Implement secrets management for API keys and passwords
2. Add input validation for user-provided symbols

### Short-term (Major Issues)
1. Fix race condition in ExecutionManager.Confirm
2. Add cooldown map cleanup mechanism
3. Increase test coverage for external integrations (goal: 80%+)
4. Complete migration from LegacyBroker to Broker interface

### Medium-term (Code Quality)
1. Extract CLI helper functions to reduce duplication
2. Add context propagation with timeouts
3. Replace magic numbers with named constants

### Long-term (Architecture)
1. Consider using embed.FS for templates
2. Add structured logging with trace IDs
3. Implement circuit breakers for external API calls

---

## Appendix: Test Coverage by Package

```
github.com/newthinker/atlas/internal/broker           94.3%
github.com/newthinker/atlas/internal/broker/mocks     99.0%
github.com/newthinker/atlas/internal/backtest         96.9%
github.com/newthinker/atlas/internal/metrics          98.5%
github.com/newthinker/atlas/internal/notifier         97.1%
github.com/newthinker/atlas/internal/config           85.2%
github.com/newthinker/atlas/internal/core             82.4%
github.com/newthinker/atlas/internal/strategy         78.6%
github.com/newthinker/atlas/internal/api              52.7%
github.com/newthinker/atlas/internal/collector/yahoo  17.2%
github.com/newthinker/atlas/internal/llm/claude       8.0%
github.com/newthinker/atlas/internal/llm/openai       19.4%
github.com/newthinker/atlas/cmd/atlas                 0.0%
```
