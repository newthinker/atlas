# ATLAS Phase 2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add fundamental data support via Lixinger API, fundamental trading strategies (PE band, dividend yield), and additional notifiers (email, webhook).

**Architecture:** Extend core types with Fundamental data struct. Add FundamentalCollector interface for Lixinger. Implement PE band and dividend yield strategies that use fundamental data. Add email (SMTP) and webhook notifiers.

**Tech Stack:** Go 1.21+, net/smtp for email, net/http for webhook, Lixinger REST API

---

## Task 1: Add Fundamental Data Type

**Files:**
- Modify: `internal/core/types.go`
- Modify: `internal/core/types_test.go`

**Step 1: Add Fundamental struct to types.go**

Add after the OHLCV struct:

```go
// Fundamental represents fundamental data for a stock
type Fundamental struct {
	Symbol       string
	Market       Market
	Date         time.Time // Report date
	PE           float64   // Price to Earnings ratio
	PB           float64   // Price to Book ratio
	PS           float64   // Price to Sales ratio
	ROE          float64   // Return on Equity (percentage)
	ROA          float64   // Return on Assets (percentage)
	DividendYield float64  // Dividend yield (percentage)
	MarketCap    float64   // Market capitalization
	Revenue      float64   // Total revenue
	NetIncome    float64   // Net income
	EPS          float64   // Earnings per share
	Source       string    // Data source
}

// IsValid checks if fundamental data has required fields
func (f Fundamental) IsValid() bool {
	return f.Symbol != "" && !f.Date.IsZero()
}
```

**Step 2: Add test for Fundamental validation**

```go
func TestFundamental_IsValid(t *testing.T) {
	tests := []struct {
		name string
		f    Fundamental
		want bool
	}{
		{"valid", Fundamental{Symbol: "600519", Date: time.Now()}, true},
		{"empty symbol", Fundamental{Date: time.Now()}, false},
		{"zero date", Fundamental{Symbol: "600519"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/core/... -v
```

**Step 4: Commit**

```bash
git add internal/core/
git commit -m "feat: add Fundamental data type for fundamental analysis"
```

---

## Task 2: Add FundamentalCollector Interface

**Files:**
- Create: `internal/collector/fundamental.go`
- Create: `internal/collector/fundamental_test.go`

**Step 1: Create fundamental collector interface**

```go
// internal/collector/fundamental.go
package collector

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// FundamentalCollector defines interface for fundamental data collectors
type FundamentalCollector interface {
	// Metadata
	Name() string
	SupportedMarkets() []core.Market

	// Lifecycle
	Init(cfg Config) error
	Start(ctx context.Context) error
	Stop() error

	// Data fetching
	FetchFundamental(symbol string) (*core.Fundamental, error)
	FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error)
}

// FundamentalRegistry manages fundamental collector instances
type FundamentalRegistry struct {
	collectors map[string]FundamentalCollector
}

// NewFundamentalRegistry creates a new fundamental collector registry
func NewFundamentalRegistry() *FundamentalRegistry {
	return &FundamentalRegistry{
		collectors: make(map[string]FundamentalCollector),
	}
}

func (r *FundamentalRegistry) Register(c FundamentalCollector) {
	r.collectors[c.Name()] = c
}

func (r *FundamentalRegistry) Get(name string) (FundamentalCollector, bool) {
	c, ok := r.collectors[name]
	return c, ok
}

func (r *FundamentalRegistry) GetAll() []FundamentalCollector {
	result := make([]FundamentalCollector, 0, len(r.collectors))
	for _, c := range r.collectors {
		result = append(result, c)
	}
	return result
}
```

**Step 2: Add tests**

```go
// internal/collector/fundamental_test.go
package collector

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

type mockFundamentalCollector struct {
	name string
}

func (m *mockFundamentalCollector) Name() string                        { return m.name }
func (m *mockFundamentalCollector) SupportedMarkets() []core.Market     { return []core.Market{core.MarketCNA} }
func (m *mockFundamentalCollector) Init(cfg Config) error               { return nil }
func (m *mockFundamentalCollector) Start(ctx context.Context) error     { return nil }
func (m *mockFundamentalCollector) Stop() error                         { return nil }
func (m *mockFundamentalCollector) FetchFundamental(symbol string) (*core.Fundamental, error) {
	return &core.Fundamental{Symbol: symbol, PE: 15.5}, nil
}
func (m *mockFundamentalCollector) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	return []core.Fundamental{{Symbol: symbol}}, nil
}

func TestFundamentalRegistry(t *testing.T) {
	r := NewFundamentalRegistry()
	mock := &mockFundamentalCollector{name: "test"}

	r.Register(mock)

	c, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find collector")
	}
	if c.Name() != "test" {
		t.Errorf("expected name 'test', got %s", c.Name())
	}

	all := r.GetAll()
	if len(all) != 1 {
		t.Errorf("expected 1 collector, got %d", len(all))
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/collector/... -v
```

**Step 4: Commit**

```bash
git add internal/collector/fundamental*.go
git commit -m "feat: add FundamentalCollector interface and registry"
```

---

## Task 3: Implement Lixinger Collector

**Files:**
- Create: `internal/collector/lixinger/lixinger.go`
- Create: `internal/collector/lixinger/lixinger_test.go`

**Step 1: Create Lixinger collector**

```go
// internal/collector/lixinger/lixinger.go
package lixinger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

const baseURL = "https://open.lixinger.com/api"

// Lixinger implements FundamentalCollector for Lixinger API
type Lixinger struct {
	apiKey string
	client *http.Client
}

// New creates a new Lixinger collector
func New(apiKey string) *Lixinger {
	return &Lixinger{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *Lixinger) Name() string { return "lixinger" }

func (l *Lixinger) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketCNA}
}

func (l *Lixinger) Init(cfg collector.Config) error {
	if cfg.APIKey != "" {
		l.apiKey = cfg.APIKey
	}
	if l.apiKey == "" {
		return fmt.Errorf("lixinger: api_key is required")
	}
	return nil
}

func (l *Lixinger) Start(ctx context.Context) error { return nil }
func (l *Lixinger) Stop() error                     { return nil }

// FetchFundamental fetches latest fundamental data for a stock
func (l *Lixinger) FetchFundamental(symbol string) (*core.Fundamental, error) {
	url := fmt.Sprintf("%s/cn/company/fundamental/non_financial", baseURL)

	payload := map[string]any{
		"token":      l.apiKey,
		"stockCodes": []string{symbol},
		"metrics":    []string{"pe_ttm", "pb", "ps_ttm", "roe_ttm", "dividend_yield_ratio", "market_value"},
	}

	resp, err := l.postJSON(url, payload)
	if err != nil {
		return nil, fmt.Errorf("lixinger: fetch failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("lixinger: no data for symbol %s", symbol)
	}

	item := resp.Data[0]
	return &core.Fundamental{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Date:          time.Now(),
		PE:            item.PETTM,
		PB:            item.PB,
		PS:            item.PSTTM,
		ROE:           item.ROETTM,
		DividendYield: item.DividendYieldRatio,
		MarketCap:     item.MarketValue,
		Source:        "lixinger",
	}, nil
}

// FetchFundamentalHistory fetches historical fundamental data
func (l *Lixinger) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	// Lixinger returns latest values; for history we'd need to call repeatedly
	// For now, just return latest as single item
	f, err := l.FetchFundamental(symbol)
	if err != nil {
		return nil, err
	}
	return []core.Fundamental{*f}, nil
}

type lixingerResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    []lixingerMetric `json:"data"`
}

type lixingerMetric struct {
	StockCode          string  `json:"stockCode"`
	PETTM              float64 `json:"pe_ttm"`
	PB                 float64 `json:"pb"`
	PSTTM              float64 `json:"ps_ttm"`
	ROETTM             float64 `json:"roe_ttm"`
	DividendYieldRatio float64 `json:"dividend_yield_ratio"`
	MarketValue        float64 `json:"market_value"`
}

func (l *Lixinger) postJSON(url string, payload any) (*lixingerResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result lixingerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API error: %s", result.Message)
	}

	return &result, nil
}
```

Note: Add `"bytes"` to imports.

**Step 2: Add tests**

```go
// internal/collector/lixinger/lixinger_test.go
package lixinger

import (
	"testing"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

func TestLixinger_ImplementsFundamentalCollector(t *testing.T) {
	var _ collector.FundamentalCollector = (*Lixinger)(nil)
}

func TestLixinger_Name(t *testing.T) {
	l := New("test-key")
	if l.Name() != "lixinger" {
		t.Errorf("expected 'lixinger', got %s", l.Name())
	}
}

func TestLixinger_SupportedMarkets(t *testing.T) {
	l := New("test-key")
	markets := l.SupportedMarkets()
	if len(markets) != 1 || markets[0] != core.MarketCNA {
		t.Errorf("expected [CN_A], got %v", markets)
	}
}

func TestLixinger_Init_RequiresAPIKey(t *testing.T) {
	l := &Lixinger{}
	err := l.Init(collector.Config{})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestLixinger_Init_WithAPIKey(t *testing.T) {
	l := &Lixinger{}
	err := l.Init(collector.Config{APIKey: "test-key"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/collector/lixinger/... -v
```

**Step 4: Commit**

```bash
git add internal/collector/lixinger/
git commit -m "feat: add Lixinger collector for CN fundamental data"
```

---

## Task 4: Update Strategy Interface for Fundamentals

**Files:**
- Modify: `internal/strategy/interface.go`

**Step 1: Add FundamentalData to AnalysisContext**

```go
// Add to AnalysisContext struct
type AnalysisContext struct {
	Symbol      string
	OHLCV       []core.OHLCV
	Fundamental *core.Fundamental  // Add this field
	Now         time.Time
}

// Add to DataRequirements struct
type DataRequirements struct {
	PriceHistory int      // Days of OHLCV data needed
	Indicators   []string // Indicator names needed
	Fundamentals bool     // Whether fundamental data is needed
}
```

**Step 2: Run tests**

```bash
go test ./internal/strategy/... -v
```

**Step 3: Commit**

```bash
git add internal/strategy/interface.go
git commit -m "feat: add Fundamental support to strategy interface"
```

---

## Task 5: Implement PE Band Strategy

**Files:**
- Create: `internal/strategy/pe_band/strategy.go`
- Create: `internal/strategy/pe_band/strategy_test.go`

**Step 1: Create PE band strategy**

```go
// internal/strategy/pe_band/strategy.go
package pe_band

import (
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// PEBand implements a PE band strategy
// Generates buy signals when PE is below historical low threshold
// Generates sell signals when PE is above historical high threshold
type PEBand struct {
	lowThreshold  float64 // PE below this = buy signal
	highThreshold float64 // PE above this = sell signal
}

// New creates a new PE Band strategy
func New(lowThreshold, highThreshold float64) *PEBand {
	return &PEBand{
		lowThreshold:  lowThreshold,
		highThreshold: highThreshold,
	}
}

func (p *PEBand) Name() string { return "pe_band" }

func (p *PEBand) Description() string {
	return fmt.Sprintf("PE Band Strategy (low: %.1f, high: %.1f)", p.lowThreshold, p.highThreshold)
}

func (p *PEBand) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: 0,
		Fundamentals: true,
	}
}

func (p *PEBand) Init(cfg strategy.Config) error {
	if low, ok := cfg.Params["low_threshold"].(float64); ok {
		p.lowThreshold = low
	}
	if high, ok := cfg.Params["high_threshold"].(float64); ok {
		p.highThreshold = high
	}
	return nil
}

func (p *PEBand) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Fundamental == nil {
		return nil, nil // No fundamental data available
	}

	pe := ctx.Fundamental.PE
	if pe <= 0 {
		return nil, nil // Invalid PE
	}

	var signals []core.Signal

	// Buy signal: PE below low threshold
	if pe < p.lowThreshold {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionBuy,
			Confidence:  p.calculateConfidence(pe, p.lowThreshold, true),
			Reason:      fmt.Sprintf("PE (%.2f) below threshold (%.1f)", pe, p.lowThreshold),
			GeneratedAt: time.Now(),
			Metadata: map[string]any{
				"pe":            pe,
				"low_threshold": p.lowThreshold,
				"type":          "pe_undervalued",
			},
		})
	}

	// Sell signal: PE above high threshold
	if pe > p.highThreshold {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionSell,
			Confidence:  p.calculateConfidence(pe, p.highThreshold, false),
			Reason:      fmt.Sprintf("PE (%.2f) above threshold (%.1f)", pe, p.highThreshold),
			GeneratedAt: time.Now(),
			Metadata: map[string]any{
				"pe":             pe,
				"high_threshold": p.highThreshold,
				"type":           "pe_overvalued",
			},
		})
	}

	return signals, nil
}

func (p *PEBand) calculateConfidence(pe, threshold float64, isBuy bool) float64 {
	var diff float64
	if isBuy {
		diff = (threshold - pe) / threshold
	} else {
		diff = (pe - threshold) / threshold
	}

	confidence := 0.5 + (diff * 2)
	if confidence > 0.9 {
		confidence = 0.9
	}
	if confidence < 0.5 {
		confidence = 0.5
	}
	return confidence
}
```

**Step 2: Add tests**

```go
// internal/strategy/pe_band/strategy_test.go
package pe_band

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

func TestPEBand_ImplementsStrategy(t *testing.T) {
	var _ strategy.Strategy = (*PEBand)(nil)
}

func TestPEBand_Name(t *testing.T) {
	p := New(10, 30)
	if p.Name() != "pe_band" {
		t.Errorf("expected 'pe_band', got %s", p.Name())
	}
}

func TestPEBand_RequiresFundamentals(t *testing.T) {
	p := New(10, 30)
	req := p.RequiredData()
	if !req.Fundamentals {
		t.Error("PE band should require fundamentals")
	}
}

func TestPEBand_BuySignal(t *testing.T) {
	p := New(15, 30) // Buy when PE < 15

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol: "600519",
			PE:     10, // Below 15, should trigger buy
			Date:   time.Now(),
		},
	}

	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	if signals[0].Action != core.ActionBuy {
		t.Errorf("expected Buy action, got %s", signals[0].Action)
	}
}

func TestPEBand_SellSignal(t *testing.T) {
	p := New(15, 30) // Sell when PE > 30

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol: "600519",
			PE:     40, // Above 30, should trigger sell
			Date:   time.Now(),
		},
	}

	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	if signals[0].Action != core.ActionSell {
		t.Errorf("expected Sell action, got %s", signals[0].Action)
	}
}

func TestPEBand_NoSignal(t *testing.T) {
	p := New(15, 30)

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol: "600519",
			PE:     20, // Between 15 and 30, no signal
			Date:   time.Now(),
		},
	}

	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals, got %d", len(signals))
	}
}

func TestPEBand_NoFundamental(t *testing.T) {
	p := New(15, 30)

	ctx := strategy.AnalysisContext{
		Symbol:      "600519",
		Fundamental: nil,
	}

	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals without fundamentals, got %d", len(signals))
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/strategy/pe_band/... -v
```

**Step 4: Commit**

```bash
git add internal/strategy/pe_band/
git commit -m "feat: add PE Band fundamental strategy"
```

---

## Task 6: Implement Dividend Yield Strategy

**Files:**
- Create: `internal/strategy/dividend_yield/strategy.go`
- Create: `internal/strategy/dividend_yield/strategy_test.go`

**Step 1: Create dividend yield strategy**

```go
// internal/strategy/dividend_yield/strategy.go
package dividend_yield

import (
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// DividendYield implements a dividend yield strategy
// Generates buy signals when dividend yield is above threshold
type DividendYield struct {
	minYield float64 // Minimum yield percentage for buy signal
}

// New creates a new Dividend Yield strategy
func New(minYield float64) *DividendYield {
	return &DividendYield{
		minYield: minYield,
	}
}

func (d *DividendYield) Name() string { return "dividend_yield" }

func (d *DividendYield) Description() string {
	return fmt.Sprintf("Dividend Yield Strategy (min: %.1f%%)", d.minYield)
}

func (d *DividendYield) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: 0,
		Fundamentals: true,
	}
}

func (d *DividendYield) Init(cfg strategy.Config) error {
	if yield, ok := cfg.Params["min_yield"].(float64); ok {
		d.minYield = yield
	}
	return nil
}

func (d *DividendYield) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Fundamental == nil {
		return nil, nil
	}

	yield := ctx.Fundamental.DividendYield
	if yield <= 0 {
		return nil, nil // No dividend
	}

	var signals []core.Signal

	if yield >= d.minYield {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionBuy,
			Confidence:  d.calculateConfidence(yield),
			Reason:      fmt.Sprintf("Dividend yield (%.2f%%) above threshold (%.1f%%)", yield, d.minYield),
			GeneratedAt: time.Now(),
			Metadata: map[string]any{
				"dividend_yield": yield,
				"min_yield":      d.minYield,
				"type":           "high_dividend",
			},
		})
	}

	return signals, nil
}

func (d *DividendYield) calculateConfidence(yield float64) float64 {
	// Higher yield = higher confidence, capped at 0.9
	diff := (yield - d.minYield) / d.minYield
	confidence := 0.5 + (diff * 0.5)
	if confidence > 0.9 {
		confidence = 0.9
	}
	return confidence
}
```

**Step 2: Add tests**

```go
// internal/strategy/dividend_yield/strategy_test.go
package dividend_yield

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

func TestDividendYield_ImplementsStrategy(t *testing.T) {
	var _ strategy.Strategy = (*DividendYield)(nil)
}

func TestDividendYield_Name(t *testing.T) {
	d := New(3.0)
	if d.Name() != "dividend_yield" {
		t.Errorf("expected 'dividend_yield', got %s", d.Name())
	}
}

func TestDividendYield_BuySignal(t *testing.T) {
	d := New(3.0) // Buy when yield >= 3%

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol:        "600519",
			DividendYield: 4.5, // 4.5% > 3%, should trigger buy
			Date:          time.Now(),
		},
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	if signals[0].Action != core.ActionBuy {
		t.Errorf("expected Buy action, got %s", signals[0].Action)
	}
}

func TestDividendYield_NoSignal_LowYield(t *testing.T) {
	d := New(3.0)

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol:        "600519",
			DividendYield: 2.0, // 2% < 3%, no signal
			Date:          time.Now(),
		},
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals for low yield, got %d", len(signals))
	}
}

func TestDividendYield_NoSignal_ZeroYield(t *testing.T) {
	d := New(3.0)

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol:        "600519",
			DividendYield: 0,
			Date:          time.Now(),
		},
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals for zero yield, got %d", len(signals))
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/strategy/dividend_yield/... -v
```

**Step 4: Commit**

```bash
git add internal/strategy/dividend_yield/
git commit -m "feat: add Dividend Yield fundamental strategy"
```

---

## Task 7: Implement Email Notifier

**Files:**
- Create: `internal/notifier/email/email.go`
- Create: `internal/notifier/email/email_test.go`

**Step 1: Create email notifier**

```go
// internal/notifier/email/email.go
package email

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

// Email implements the Notifier interface for SMTP email
type Email struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
}

// New creates a new Email notifier
func New(host string, port int, username, password, from string, to []string) *Email {
	return &Email{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		to:       to,
	}
}

func (e *Email) Name() string { return "email" }

func (e *Email) Init(cfg notifier.Config) error {
	if host, ok := cfg.Params["host"].(string); ok {
		e.host = host
	}
	if port, ok := cfg.Params["port"].(int); ok {
		e.port = port
	}
	if username, ok := cfg.Params["username"].(string); ok {
		e.username = username
	}
	if password, ok := cfg.Params["password"].(string); ok {
		e.password = password
	}
	if from, ok := cfg.Params["from"].(string); ok {
		e.from = from
	}
	if to, ok := cfg.Params["to"].([]string); ok {
		e.to = to
	}

	if e.host == "" || e.from == "" || len(e.to) == 0 {
		return fmt.Errorf("email: host, from, and to are required")
	}
	return nil
}

func (e *Email) Send(signal core.Signal) error {
	subject := fmt.Sprintf("ATLAS Signal: %s %s", signal.Symbol, signal.Action)
	body := e.formatSignal(signal)
	return e.sendEmail(subject, body)
}

func (e *Email) SendBatch(signals []core.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	subject := fmt.Sprintf("ATLAS Digest: %d Trading Signals", len(signals))

	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString("<h2>ATLAS Trading Signals</h2>")
	sb.WriteString(fmt.Sprintf("<p>Generated at: %s</p>", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("<hr>")

	for _, signal := range signals {
		sb.WriteString(e.formatSignalHTML(signal))
		sb.WriteString("<hr>")
	}

	sb.WriteString("</body></html>")

	return e.sendEmail(subject, sb.String())
}

func (e *Email) formatSignal(signal core.Signal) string {
	return fmt.Sprintf(`
ATLAS Trading Signal

Symbol: %s
Action: %s
Confidence: %.1f%%
Strategy: %s
Reason: %s
Time: %s
`,
		signal.Symbol,
		signal.Action,
		signal.Confidence*100,
		signal.Strategy,
		signal.Reason,
		signal.GeneratedAt.Format("2006-01-02 15:04:05"),
	)
}

func (e *Email) formatSignalHTML(signal core.Signal) string {
	actionColor := "#28a745" // green for buy
	if signal.Action == core.ActionSell || signal.Action == core.ActionStrongSell {
		actionColor = "#dc3545" // red for sell
	}

	return fmt.Sprintf(`
<div style="margin: 10px 0;">
  <h3 style="color: %s;">%s - %s</h3>
  <p><strong>Confidence:</strong> %.1f%%</p>
  <p><strong>Strategy:</strong> %s</p>
  <p><strong>Reason:</strong> %s</p>
  <p><small>%s</small></p>
</div>
`,
		actionColor,
		signal.Symbol,
		signal.Action,
		signal.Confidence*100,
		signal.Strategy,
		signal.Reason,
		signal.GeneratedAt.Format("2006-01-02 15:04:05"),
	)
}

func (e *Email) sendEmail(subject, body string) error {
	addr := fmt.Sprintf("%s:%d", e.host, e.port)

	var auth smtp.Auth
	if e.username != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.host)
	}

	contentType := "text/plain"
	if strings.Contains(body, "<html>") {
		contentType = "text/html"
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: %s; charset=UTF-8\r\n"+
		"\r\n"+
		"%s",
		e.from,
		strings.Join(e.to, ","),
		subject,
		contentType,
		body,
	)

	return smtp.SendMail(addr, auth, e.from, e.to, []byte(msg))
}
```

**Step 2: Add tests**

```go
// internal/notifier/email/email_test.go
package email

import (
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

func TestEmail_ImplementsNotifier(t *testing.T) {
	var _ notifier.Notifier = (*Email)(nil)
}

func TestEmail_Name(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})
	if e.Name() != "email" {
		t.Errorf("expected 'email', got %s", e.Name())
	}
}

func TestEmail_Init_RequiredFields(t *testing.T) {
	e := &Email{}
	err := e.Init(notifier.Config{Params: map[string]any{}})
	if err == nil {
		t.Error("expected error for missing required fields")
	}
}

func TestEmail_Init_WithConfig(t *testing.T) {
	e := &Email{}
	err := e.Init(notifier.Config{
		Params: map[string]any{
			"host": "smtp.example.com",
			"port": 587,
			"from": "atlas@example.com",
			"to":   []string{"user@example.com"},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if e.host != "smtp.example.com" {
		t.Errorf("expected host smtp.example.com, got %s", e.host)
	}
}

func TestEmail_FormatSignal(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})

	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "pe_band",
		Reason:      "PE below threshold",
		GeneratedAt: time.Now(),
	}

	formatted := e.formatSignal(signal)

	if !strings.Contains(formatted, "AAPL") {
		t.Error("formatted message should contain symbol")
	}
	if !strings.Contains(formatted, "buy") {
		t.Error("formatted message should contain action")
	}
	if !strings.Contains(formatted, "85.0%") {
		t.Error("formatted message should contain confidence")
	}
}

func TestEmail_SendBatch_Empty(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})

	err := e.SendBatch([]core.Signal{})
	if err != nil {
		t.Errorf("empty batch should not error: %v", err)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/notifier/email/... -v
```

**Step 4: Commit**

```bash
git add internal/notifier/email/
git commit -m "feat: add Email notifier with SMTP support"
```

---

## Task 8: Implement Webhook Notifier

**Files:**
- Create: `internal/notifier/webhook/webhook.go`
- Create: `internal/notifier/webhook/webhook_test.go`

**Step 1: Create webhook notifier**

```go
// internal/notifier/webhook/webhook.go
package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

// Webhook implements the Notifier interface for HTTP webhooks
type Webhook struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// New creates a new Webhook notifier
func New(url string, headers map[string]string) *Webhook {
	return &Webhook{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Init(cfg notifier.Config) error {
	if url, ok := cfg.Params["url"].(string); ok {
		w.url = url
	}
	if headers, ok := cfg.Params["headers"].(map[string]string); ok {
		w.headers = headers
	}

	if w.url == "" {
		return fmt.Errorf("webhook: url is required")
	}
	return nil
}

func (w *Webhook) Send(signal core.Signal) error {
	payload := w.signalToPayload(signal)
	return w.post(payload)
}

func (w *Webhook) SendBatch(signals []core.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	payloads := make([]map[string]any, len(signals))
	for i, sig := range signals {
		payloads[i] = w.signalToPayload(sig)
	}

	batchPayload := map[string]any{
		"type":    "batch",
		"count":   len(signals),
		"signals": payloads,
	}

	return w.post(batchPayload)
}

func (w *Webhook) signalToPayload(signal core.Signal) map[string]any {
	return map[string]any{
		"type":        "signal",
		"symbol":      signal.Symbol,
		"action":      signal.Action,
		"confidence":  signal.Confidence,
		"price":       signal.Price,
		"reason":      signal.Reason,
		"strategy":    signal.Strategy,
		"metadata":    signal.Metadata,
		"generated_at": signal.GeneratedAt.Format(time.RFC3339),
	}
}

func (w *Webhook) post(payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: server returned %d", resp.StatusCode)
	}

	return nil
}
```

**Step 2: Add tests**

```go
// internal/notifier/webhook/webhook_test.go
package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

func TestWebhook_ImplementsNotifier(t *testing.T) {
	var _ notifier.Notifier = (*Webhook)(nil)
}

func TestWebhook_Name(t *testing.T) {
	w := New("http://example.com/hook", nil)
	if w.Name() != "webhook" {
		t.Errorf("expected 'webhook', got %s", w.Name())
	}
}

func TestWebhook_Init_RequiresURL(t *testing.T) {
	w := &Webhook{}
	err := w.Init(notifier.Config{Params: map[string]any{}})
	if err == nil {
		t.Error("expected error for missing URL")
	}
}

func TestWebhook_Send(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := New(server.URL, nil)

	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "pe_band",
		GeneratedAt: time.Now(),
	}

	err := w.Send(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["symbol"] != "AAPL" {
		t.Errorf("expected symbol AAPL, got %v", receivedPayload["symbol"])
	}
	if receivedPayload["action"] != "buy" {
		t.Errorf("expected action buy, got %v", receivedPayload["action"])
	}
}

func TestWebhook_SendBatch(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := New(server.URL, nil)

	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, GeneratedAt: time.Now()},
		{Symbol: "GOOG", Action: core.ActionSell, GeneratedAt: time.Now()},
	}

	err := w.SendBatch(signals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["type"] != "batch" {
		t.Errorf("expected type batch, got %v", receivedPayload["type"])
	}
	if receivedPayload["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", receivedPayload["count"])
	}
}

func TestWebhook_SendBatch_Empty(t *testing.T) {
	w := New("http://example.com/hook", nil)
	err := w.SendBatch([]core.Signal{})
	if err != nil {
		t.Errorf("empty batch should not error: %v", err)
	}
}

func TestWebhook_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	w := New(server.URL, nil)

	err := w.Send(core.Signal{Symbol: "TEST", GeneratedAt: time.Now()})
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestWebhook_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	headers := map[string]string{
		"Authorization": "Bearer test-token",
		"X-Custom":      "value",
	}
	w := New(server.URL, headers)

	w.Send(core.Signal{Symbol: "TEST", GeneratedAt: time.Now()})

	if receivedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Error("expected Authorization header")
	}
	if receivedHeaders.Get("X-Custom") != "value" {
		t.Error("expected X-Custom header")
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/notifier/webhook/... -v
```

**Step 4: Commit**

```bash
git add internal/notifier/webhook/
git commit -m "feat: add Webhook notifier for HTTP integrations"
```

---

## Task 9: Update Config for Phase 2 Components

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`

**Step 1: Add Phase 2 config sections**

Add to config structs:

```go
// Add to Config struct
type Config struct {
	// ... existing fields ...
	Lixinger LixingerConfig `mapstructure:"lixinger"`
	Email    EmailConfig    `mapstructure:"email"`
	Webhook  WebhookConfig  `mapstructure:"webhook"`
}

// Add new config types
type LixingerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"api_key"`
}

type EmailConfig struct {
	Enabled  bool     `mapstructure:"enabled"`
	Host     string   `mapstructure:"host"`
	Port     int      `mapstructure:"port"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
	From     string   `mapstructure:"from"`
	To       []string `mapstructure:"to"`
}

type WebhookConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	URL     string            `mapstructure:"url"`
	Headers map[string]string `mapstructure:"headers"`
}
```

**Step 2: Update config.example.yaml**

Add:

```yaml
# Lixinger API for CN fundamental data
lixinger:
  enabled: false
  api_key: "YOUR_LIXINGER_API_KEY"

# Fundamental strategies
strategies:
  # ... existing strategies ...
  pe_band:
    enabled: true
    low_threshold: 15
    high_threshold: 30
  dividend_yield:
    enabled: true
    min_yield: 3.0

# Email notifier
email:
  enabled: false
  host: "smtp.example.com"
  port: 587
  username: ""
  password: ""
  from: "atlas@example.com"
  to:
    - "user@example.com"

# Webhook notifier
webhook:
  enabled: false
  url: "https://example.com/webhook"
  headers:
    Authorization: "Bearer YOUR_TOKEN"
```

**Step 3: Run tests**

```bash
go test ./internal/config/... -v
```

**Step 4: Commit**

```bash
git add internal/config/config.go config.example.yaml
git commit -m "feat: add Phase 2 configuration (Lixinger, email, webhook)"
```

---

## Task 10: Final Integration Test

**Step 1: Run full test suite**

```bash
go test ./... -cover
```

**Step 2: Build and verify**

```bash
go build ./...
go vet ./...
./bin/atlas version
./bin/atlas --help
```

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "chore: Phase 2 final cleanup and integration"
```

---

## Summary

Phase 2 adds:
- **Fundamental data type** in core
- **FundamentalCollector interface** for data providers
- **Lixinger collector** for CN stock fundamentals
- **PE Band strategy** for valuation-based signals
- **Dividend Yield strategy** for income investing
- **Email notifier** with SMTP support
- **Webhook notifier** for integrations
- **Updated configuration** for all new components
