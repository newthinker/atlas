# ATLAS Phase 3 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add backtesting framework, web UI dashboard, and S3 cold storage to ATLAS.

**Architecture:** S3 storage implements existing ArchiveStorage interface. Backtesting runs strategies on historical OHLCV and calculates performance stats. Web UI uses HTMX + Go templates for a single-binary dashboard with live signal updates.

**Tech Stack:** Go 1.21+, AWS SDK v2, html/template, HTMX (CDN), Tailwind CSS (CDN)

---

## Task 1: Add S3 Storage Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`

**Step 1: Add S3Config struct to config.go**

Add after ColdStorageConfig:

```go
type S3Config struct {
	Bucket    string `mapstructure:"bucket"`
	Endpoint  string `mapstructure:"endpoint"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Prefix    string `mapstructure:"prefix"`
}
```

Update ColdStorageConfig:

```go
type ColdStorageConfig struct {
	Type string   `mapstructure:"type"` // "localfs" or "s3"
	Path string   `mapstructure:"path"` // For localfs
	S3   S3Config `mapstructure:"s3"`   // For S3
}
```

**Step 2: Update config.example.yaml**

Add S3 section under storage.cold:

```yaml
storage:
  cold:
    type: localfs  # or "s3"
    path: "/mnt/nas/atlas/archive"
    s3:
      bucket: "atlas-archive"
      endpoint: "https://s3.amazonaws.com"
      region: "us-east-1"
      access_key: "${AWS_ACCESS_KEY_ID}"
      secret_key: "${AWS_SECRET_ACCESS_KEY}"
      prefix: "archive/"
```

**Step 3: Run tests**

```bash
go test ./internal/config/... -v
```

**Step 4: Commit**

```bash
git add internal/config/config.go config.example.yaml
git commit -m "feat: add S3 storage configuration"
```

---

## Task 2: Add AWS SDK Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add AWS SDK dependencies**

```bash
go get github.com/aws/aws-sdk-go-v2
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/s3
```

**Step 2: Verify dependencies**

```bash
go mod tidy
go build ./...
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add AWS SDK v2 dependencies"
```

---

## Task 3: Create Archive Storage Interface

**Files:**
- Create: `internal/storage/archive/interface.go`
- Create: `internal/storage/archive/interface_test.go`

**Step 1: Create interface file**

```go
// internal/storage/archive/interface.go
package archive

import "context"

// Storage defines the interface for cold/archive storage backends
type Storage interface {
	// Write stores data at the given path
	Write(ctx context.Context, path string, data []byte) error

	// Read retrieves data from the given path
	Read(ctx context.Context, path string) ([]byte, error)

	// List returns all paths matching the prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Delete removes the data at the given path
	Delete(ctx context.Context, path string) error

	// Exists checks if data exists at the given path
	Exists(ctx context.Context, path string) (bool, error)
}
```

**Step 2: Create test file with interface compliance check**

```go
// internal/storage/archive/interface_test.go
package archive

import "testing"

// Compile-time interface compliance checks will be added
// as implementations are created

func TestInterfaceDefined(t *testing.T) {
	// Placeholder to ensure package compiles
	var _ Storage = nil
}
```

**Step 3: Run tests**

```bash
go test ./internal/storage/archive/... -v
```

**Step 4: Commit**

```bash
git add internal/storage/archive/
git commit -m "feat: add archive Storage interface"
```

---

## Task 4: Implement LocalFS Storage

**Files:**
- Create: `internal/storage/archive/localfs.go`
- Create: `internal/storage/archive/localfs_test.go`

**Step 1: Create LocalFS implementation**

```go
// internal/storage/archive/localfs.go
package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalFS implements Storage for local filesystem
type LocalFS struct {
	basePath string
}

// NewLocalFS creates a new LocalFS storage
func NewLocalFS(basePath string) (*LocalFS, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("creating base path: %w", err)
	}
	return &LocalFS{basePath: basePath}, nil
}

func (l *LocalFS) fullPath(path string) string {
	return filepath.Join(l.basePath, path)
}

func (l *LocalFS) Write(ctx context.Context, path string, data []byte) error {
	fullPath := l.fullPath(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	return os.WriteFile(fullPath, data, 0644)
}

func (l *LocalFS) Read(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(l.fullPath(path))
}

func (l *LocalFS) List(ctx context.Context, prefix string) ([]string, error) {
	var paths []string
	searchPath := l.fullPath(prefix)

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(l.basePath, path)
			paths = append(paths, relPath)
		}
		return nil
	})

	if os.IsNotExist(err) {
		return []string{}, nil
	}
	return paths, err
}

func (l *LocalFS) Delete(ctx context.Context, path string) error {
	return os.Remove(l.fullPath(path))
}

func (l *LocalFS) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(l.fullPath(path))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
```

**Step 2: Create tests**

```go
// internal/storage/archive/localfs_test.go
package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFS_ImplementsStorage(t *testing.T) {
	var _ Storage = (*LocalFS)(nil)
}

func TestLocalFS_WriteRead(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewLocalFS(dir)
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}

	ctx := context.Background()
	data := []byte("test data")

	if err := fs.Write(ctx, "test/file.txt", data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fs.Read(ctx, "test/file.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestLocalFS_Exists(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewLocalFS(dir)
	ctx := context.Background()

	exists, _ := fs.Exists(ctx, "nonexistent.txt")
	if exists {
		t.Error("expected false for nonexistent file")
	}

	fs.Write(ctx, "exists.txt", []byte("data"))
	exists, _ = fs.Exists(ctx, "exists.txt")
	if !exists {
		t.Error("expected true for existing file")
	}
}

func TestLocalFS_List(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewLocalFS(dir)
	ctx := context.Background()

	fs.Write(ctx, "data/2024/01/a.txt", []byte("a"))
	fs.Write(ctx, "data/2024/01/b.txt", []byte("b"))
	fs.Write(ctx, "data/2024/02/c.txt", []byte("c"))

	paths, err := fs.List(ctx, "data/2024/01")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

func TestLocalFS_Delete(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewLocalFS(dir)
	ctx := context.Background()

	fs.Write(ctx, "delete.txt", []byte("data"))
	fs.Delete(ctx, "delete.txt")

	exists, _ := fs.Exists(ctx, "delete.txt")
	if exists {
		t.Error("file should be deleted")
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/storage/archive/... -v
```

**Step 4: Commit**

```bash
git add internal/storage/archive/localfs*.go
git commit -m "feat: add LocalFS archive storage implementation"
```

---

## Task 5: Implement S3 Storage

**Files:**
- Create: `internal/storage/archive/s3.go`
- Create: `internal/storage/archive/s3_test.go`

**Step 1: Create S3 implementation**

```go
// internal/storage/archive/s3.go
package archive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds S3 connection configuration
type S3Config struct {
	Bucket    string
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	Prefix    string
}

// S3Storage implements Storage for S3-compatible backends
type S3Storage struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3 creates a new S3 storage client
func NewS3(cfg S3Config) (*S3Storage, error) {
	opts := s3.Options{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}

	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
		opts.UsePathStyle = true // Required for MinIO and most S3-compatible services
	}

	client := s3.New(opts)

	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.TrimSuffix(cfg.Prefix, "/"),
	}, nil
}

func (s *S3Storage) key(path string) string {
	if s.prefix == "" {
		return path
	}
	return s.prefix + "/" + path
}

func (s *S3Storage) Write(ctx context.Context, path string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (s *S3Storage) Read(ctx context.Context, path string) ([]byte, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	if err != nil {
		return nil, err
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	var paths []string
	fullPrefix := s.key(prefix)

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			// Remove prefix to return relative paths
			relPath := strings.TrimPrefix(*obj.Key, s.prefix+"/")
			paths = append(paths, relPath)
		}
	}

	return paths, nil
}

func (s *S3Storage) Delete(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	return err
}

func (s *S3Storage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
```

**Step 2: Create tests (unit tests with interface check)**

```go
// internal/storage/archive/s3_test.go
package archive

import (
	"testing"
)

func TestS3Storage_ImplementsStorage(t *testing.T) {
	var _ Storage = (*S3Storage)(nil)
}

func TestS3Config_Key(t *testing.T) {
	tests := []struct {
		prefix string
		path   string
		want   string
	}{
		{"", "file.txt", "file.txt"},
		{"archive", "file.txt", "archive/file.txt"},
		{"archive/", "file.txt", "archive/file.txt"},
	}

	for _, tt := range tests {
		s := &S3Storage{prefix: strings.TrimSuffix(tt.prefix, "/")}
		got := s.key(tt.path)
		if got != tt.want {
			t.Errorf("key(%q) with prefix %q = %q, want %q", tt.path, tt.prefix, got, tt.want)
		}
	}
}
```

Note: Add `"strings"` to imports in test file.

**Step 3: Run tests**

```bash
go test ./internal/storage/archive/... -v
```

**Step 4: Commit**

```bash
git add internal/storage/archive/s3*.go
git commit -m "feat: add S3 archive storage implementation"
```

---

## Task 6: Create Backtest Types

**Files:**
- Create: `internal/backtest/types.go`
- Create: `internal/backtest/types_test.go`

**Step 1: Create backtest types**

```go
// internal/backtest/types.go
package backtest

import (
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Result holds the complete backtest output
type Result struct {
	Strategy  string
	Symbol    string
	StartDate time.Time
	EndDate   time.Time
	Signals   []core.Signal
	Trades    []Trade
	Stats     Stats
}

// Trade represents a simulated trade from entry to exit
type Trade struct {
	EntrySignal core.Signal
	ExitSignal  *core.Signal // nil if position still open
	EntryPrice  float64
	ExitPrice   float64
	Return      float64 // Percentage return
}

// Stats holds performance statistics
type Stats struct {
	TotalTrades  int
	WinningTrades int
	LosingTrades  int
	WinRate      float64 // Percentage of profitable trades
	TotalReturn  float64 // Net return percentage
	MaxDrawdown  float64 // Largest peak-to-trough decline
	SharpeRatio  float64 // Risk-adjusted return (annualized)
}

// IsWin returns true if the trade was profitable
func (t Trade) IsWin() bool {
	return t.Return > 0
}

// IsClosed returns true if the trade has an exit
func (t Trade) IsClosed() bool {
	return t.ExitSignal != nil
}
```

**Step 2: Create tests**

```go
// internal/backtest/types_test.go
package backtest

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

func TestTrade_IsWin(t *testing.T) {
	tests := []struct {
		name   string
		trade  Trade
		want   bool
	}{
		{"positive return", Trade{Return: 0.05}, true},
		{"negative return", Trade{Return: -0.02}, false},
		{"zero return", Trade{Return: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.trade.IsWin(); got != tt.want {
				t.Errorf("IsWin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrade_IsClosed(t *testing.T) {
	openTrade := Trade{EntrySignal: core.Signal{Symbol: "TEST"}}
	closedTrade := Trade{
		EntrySignal: core.Signal{Symbol: "TEST"},
		ExitSignal:  &core.Signal{Symbol: "TEST"},
	}

	if openTrade.IsClosed() {
		t.Error("open trade should not be closed")
	}
	if !closedTrade.IsClosed() {
		t.Error("closed trade should be closed")
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/backtest/... -v
```

**Step 4: Commit**

```bash
git add internal/backtest/
git commit -m "feat: add backtest types (Result, Trade, Stats)"
```

---

## Task 7: Implement Stats Calculator

**Files:**
- Create: `internal/backtest/stats.go`
- Create: `internal/backtest/stats_test.go`

**Step 1: Create stats calculator**

```go
// internal/backtest/stats.go
package backtest

import (
	"math"
)

// CalculateStats computes performance statistics from trades
func CalculateStats(trades []Trade) Stats {
	if len(trades) == 0 {
		return Stats{}
	}

	var winning, losing int
	var totalReturn float64
	var returns []float64

	for _, t := range trades {
		if !t.IsClosed() {
			continue
		}
		returns = append(returns, t.Return)
		totalReturn += t.Return
		if t.IsWin() {
			winning++
		} else {
			losing++
		}
	}

	closedTrades := winning + losing
	var winRate float64
	if closedTrades > 0 {
		winRate = float64(winning) / float64(closedTrades) * 100
	}

	return Stats{
		TotalTrades:   len(trades),
		WinningTrades: winning,
		LosingTrades:  losing,
		WinRate:       winRate,
		TotalReturn:   totalReturn * 100, // Convert to percentage
		MaxDrawdown:   calculateMaxDrawdown(returns) * 100,
		SharpeRatio:   calculateSharpeRatio(returns),
	}
}

// calculateMaxDrawdown finds the largest peak-to-trough decline
func calculateMaxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	var maxDD float64
	var peak float64
	cumulative := 1.0

	for _, r := range returns {
		cumulative *= (1 + r)
		if cumulative > peak {
			peak = cumulative
		}
		if peak > 0 {
			dd := (peak - cumulative) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
	}

	return maxDD
}

// calculateSharpeRatio computes risk-adjusted return
// Assumes risk-free rate of 0 for simplicity
func calculateSharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	// Calculate mean return
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Calculate standard deviation
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	stdDev := math.Sqrt(variance / float64(len(returns)-1))

	if stdDev == 0 {
		return 0
	}

	// Annualize (assuming ~252 trading days)
	annualizedReturn := mean * 252
	annualizedStdDev := stdDev * math.Sqrt(252)

	return annualizedReturn / annualizedStdDev
}
```

**Step 2: Create tests**

```go
// internal/backtest/stats_test.go
package backtest

import (
	"math"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestCalculateStats_Empty(t *testing.T) {
	stats := CalculateStats([]Trade{})
	if stats.TotalTrades != 0 {
		t.Error("expected 0 trades for empty input")
	}
}

func TestCalculateStats_WinRate(t *testing.T) {
	exit := &core.Signal{Symbol: "TEST"}
	trades := []Trade{
		{Return: 0.10, ExitSignal: exit}, // win
		{Return: 0.05, ExitSignal: exit}, // win
		{Return: -0.03, ExitSignal: exit}, // loss
		{Return: 0.02, ExitSignal: exit}, // win
	}

	stats := CalculateStats(trades)

	if stats.TotalTrades != 4 {
		t.Errorf("TotalTrades = %d, want 4", stats.TotalTrades)
	}
	if stats.WinningTrades != 3 {
		t.Errorf("WinningTrades = %d, want 3", stats.WinningTrades)
	}
	if stats.WinRate != 75 {
		t.Errorf("WinRate = %f, want 75", stats.WinRate)
	}
}

func TestCalculateStats_TotalReturn(t *testing.T) {
	exit := &core.Signal{Symbol: "TEST"}
	trades := []Trade{
		{Return: 0.10, ExitSignal: exit},
		{Return: -0.05, ExitSignal: exit},
	}

	stats := CalculateStats(trades)

	expected := 5.0 // (0.10 + -0.05) * 100
	if math.Abs(stats.TotalReturn-expected) > 0.001 {
		t.Errorf("TotalReturn = %f, want %f", stats.TotalReturn, expected)
	}
}

func TestCalculateMaxDrawdown(t *testing.T) {
	// Simulate: +10%, +5%, -20%, +10%
	// Peak at 1.155, trough at 0.924, DD = 20%
	returns := []float64{0.10, 0.05, -0.20, 0.10}
	dd := calculateMaxDrawdown(returns)

	if dd < 0.19 || dd > 0.21 {
		t.Errorf("MaxDrawdown = %f, expected ~0.20", dd)
	}
}

func TestCalculateStats_IgnoresOpenTrades(t *testing.T) {
	exit := &core.Signal{Symbol: "TEST"}
	trades := []Trade{
		{Return: 0.10, ExitSignal: exit},  // closed
		{Return: 0.05, ExitSignal: nil},   // open - should be ignored
	}

	stats := CalculateStats(trades)

	if stats.WinningTrades != 1 {
		t.Errorf("should only count closed trades, got %d", stats.WinningTrades)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/backtest/... -v
```

**Step 4: Commit**

```bash
git add internal/backtest/stats*.go
git commit -m "feat: add backtest stats calculator"
```

---

## Task 8: Implement Backtester Engine

**Files:**
- Create: `internal/backtest/backtester.go`
- Create: `internal/backtest/backtester_test.go`

**Step 1: Create backtester engine**

```go
// internal/backtest/backtester.go
package backtest

import (
	"context"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// OHLCVProvider interface for fetching historical data
type OHLCVProvider interface {
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}

// Backtester runs strategies against historical data
type Backtester struct {
	provider OHLCVProvider
}

// New creates a new Backtester
func New(provider OHLCVProvider) *Backtester {
	return &Backtester{provider: provider}
}

// Run executes a backtest for the given strategy and symbol
func (b *Backtester) Run(ctx context.Context, strat strategy.Strategy, symbol string, start, end time.Time) (*Result, error) {
	// Fetch historical data
	ohlcv, err := b.provider.FetchHistory(symbol, start, end, "1d")
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}

	if len(ohlcv) == 0 {
		return nil, fmt.Errorf("no data for symbol %s in range", symbol)
	}

	// Get data requirements
	req := strat.RequiredData()

	// Run strategy on each bar
	var signals []core.Signal
	for i := req.PriceHistory; i < len(ohlcv); i++ {
		// Build context with historical data up to this point
		analysisCtx := strategy.AnalysisContext{
			Symbol: symbol,
			OHLCV:  ohlcv[max(0, i-req.PriceHistory):i+1],
			Now:    ohlcv[i].Time,
		}

		sigs, err := strat.Analyze(analysisCtx)
		if err != nil {
			continue // Skip errors, log in production
		}

		// Add price to signals
		for j := range sigs {
			sigs[j].Price = ohlcv[i].Close
		}

		signals = append(signals, sigs...)
	}

	// Convert signals to trades
	trades := signalsToTrades(signals, ohlcv)

	return &Result{
		Strategy:  strat.Name(),
		Symbol:    symbol,
		StartDate: start,
		EndDate:   end,
		Signals:   signals,
		Trades:    trades,
		Stats:     CalculateStats(trades),
	}, nil
}

// signalsToTrades converts buy/sell signals into trades
func signalsToTrades(signals []core.Signal, ohlcv []core.OHLCV) []Trade {
	var trades []Trade
	var openTrade *Trade

	for i, sig := range signals {
		switch sig.Action {
		case core.ActionBuy, core.ActionStrongBuy:
			if openTrade == nil {
				openTrade = &Trade{
					EntrySignal: sig,
					EntryPrice:  sig.Price,
				}
			}
		case core.ActionSell, core.ActionStrongSell:
			if openTrade != nil {
				openTrade.ExitSignal = &signals[i]
				openTrade.ExitPrice = sig.Price
				openTrade.Return = (openTrade.ExitPrice - openTrade.EntryPrice) / openTrade.EntryPrice
				trades = append(trades, *openTrade)
				openTrade = nil
			}
		}
	}

	// Add open trade if any
	if openTrade != nil {
		trades = append(trades, *openTrade)
	}

	return trades
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

**Step 2: Create tests**

```go
// internal/backtest/backtester_test.go
package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// mockProvider implements OHLCVProvider for testing
type mockProvider struct {
	data []core.OHLCV
}

func (m *mockProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return m.data, nil
}

// mockStrategy implements strategy.Strategy for testing
type mockStrategy struct {
	signals []core.Signal
	idx     int
}

func (m *mockStrategy) Name() string                          { return "mock" }
func (m *mockStrategy) Description() string                   { return "mock strategy" }
func (m *mockStrategy) RequiredData() strategy.DataRequirements { return strategy.DataRequirements{PriceHistory: 1} }
func (m *mockStrategy) Init(cfg strategy.Config) error        { return nil }
func (m *mockStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if m.idx < len(m.signals) {
		sig := m.signals[m.idx]
		m.idx++
		return []core.Signal{sig}, nil
	}
	return nil, nil
}

func TestBacktester_Run(t *testing.T) {
	now := time.Now()
	ohlcv := []core.OHLCV{
		{Time: now.AddDate(0, 0, -4), Close: 100},
		{Time: now.AddDate(0, 0, -3), Close: 105},
		{Time: now.AddDate(0, 0, -2), Close: 110},
		{Time: now.AddDate(0, 0, -1), Close: 108},
		{Time: now, Close: 115},
	}

	signals := []core.Signal{
		{Action: core.ActionBuy, GeneratedAt: now.AddDate(0, 0, -3)},
		{Action: core.ActionSell, GeneratedAt: now.AddDate(0, 0, -1)},
	}

	provider := &mockProvider{data: ohlcv}
	strat := &mockStrategy{signals: signals}
	bt := New(provider)

	result, err := bt.Run(context.Background(), strat, "TEST", now.AddDate(0, 0, -5), now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Strategy != "mock" {
		t.Errorf("Strategy = %s, want mock", result.Strategy)
	}

	if len(result.Trades) == 0 {
		t.Error("expected at least one trade")
	}
}

func TestSignalsToTrades(t *testing.T) {
	signals := []core.Signal{
		{Action: core.ActionBuy, Price: 100},
		{Action: core.ActionSell, Price: 110},
		{Action: core.ActionBuy, Price: 105},
		{Action: core.ActionSell, Price: 115},
	}

	trades := signalsToTrades(signals, nil)

	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}

	// First trade: 100 -> 110 = 10% return
	if trades[0].Return < 0.09 || trades[0].Return > 0.11 {
		t.Errorf("first trade return = %f, expected ~0.10", trades[0].Return)
	}

	// Second trade: 105 -> 115 = 9.5% return
	if trades[1].Return < 0.09 || trades[1].Return > 0.10 {
		t.Errorf("second trade return = %f, expected ~0.095", trades[1].Return)
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/backtest/... -v
```

**Step 4: Commit**

```bash
git add internal/backtest/backtester*.go
git commit -m "feat: add backtester engine"
```

---

## Task 9: Add Backtest CLI Command

**Files:**
- Modify: `cmd/atlas/main.go` (or create `cmd/atlas/backtest.go`)

**Step 1: Create backtest command file**

```go
// cmd/atlas/backtest.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var backtestCmd = &cobra.Command{
	Use:   "backtest [strategy]",
	Short: "Run backtest on a strategy",
	Long:  "Run a strategy against historical data and show performance statistics",
	Args:  cobra.ExactArgs(1),
	Run:   runBacktest,
}

var (
	backtestSymbol string
	backtestFrom   string
	backtestTo     string
)

func init() {
	backtestCmd.Flags().StringVar(&backtestSymbol, "symbol", "", "Symbol to backtest (required)")
	backtestCmd.Flags().StringVar(&backtestFrom, "from", "", "Start date YYYY-MM-DD (required)")
	backtestCmd.Flags().StringVar(&backtestTo, "to", "", "End date YYYY-MM-DD (required)")
	backtestCmd.MarkFlagRequired("symbol")
	backtestCmd.MarkFlagRequired("from")
	backtestCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(backtestCmd)
}

func runBacktest(cmd *cobra.Command, args []string) {
	strategyName := args[0]

	from, err := time.Parse("2006-01-02", backtestFrom)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid from date: %v\n", err)
		os.Exit(1)
	}

	to, err := time.Parse("2006-01-02", backtestTo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid to date: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Running backtest...\n")
	fmt.Printf("Strategy: %s\n", strategyName)
	fmt.Printf("Symbol:   %s\n", backtestSymbol)
	fmt.Printf("Period:   %s to %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
	fmt.Println()

	// TODO: Wire up actual backtest engine with collectors
	// For now, just show the command structure works
	fmt.Println("Backtest engine integration pending (requires collector wiring)")
}
```

**Step 2: Verify command registers**

```bash
go build -o bin/atlas ./cmd/atlas && ./bin/atlas backtest --help
```

Expected output shows backtest command with flags.

**Step 3: Commit**

```bash
git add cmd/atlas/backtest.go
git commit -m "feat: add backtest CLI command"
```

---

## Task 10: Create Web UI Base Layout

**Files:**
- Create: `internal/api/templates/layout.html`
- Create: `internal/api/templates/dashboard.html`
- Create: `internal/api/handler/web/dashboard.go`

**Step 1: Create base layout template**

```html
<!-- internal/api/templates/layout.html -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - ATLAS</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
</head>
<body class="bg-gray-100 min-h-screen">
    <nav class="bg-indigo-600 text-white shadow-lg">
        <div class="max-w-7xl mx-auto px-4">
            <div class="flex justify-between h-16">
                <div class="flex items-center">
                    <span class="text-xl font-bold">ATLAS</span>
                </div>
                <div class="flex items-center space-x-4">
                    <a href="/" class="hover:bg-indigo-500 px-3 py-2 rounded">Dashboard</a>
                    <a href="/signals" class="hover:bg-indigo-500 px-3 py-2 rounded">Signals</a>
                    <a href="/watchlist" class="hover:bg-indigo-500 px-3 py-2 rounded">Watchlist</a>
                    <a href="/backtest" class="hover:bg-indigo-500 px-3 py-2 rounded">Backtest</a>
                    <a href="/settings" class="hover:bg-indigo-500 px-3 py-2 rounded">Settings</a>
                </div>
            </div>
        </div>
    </nav>

    <main class="max-w-7xl mx-auto py-6 px-4">
        {{template "content" .}}
    </main>

    <footer class="bg-gray-800 text-gray-400 py-4 mt-8">
        <div class="max-w-7xl mx-auto px-4 text-center text-sm">
            ATLAS - Asset Tracking & Leadership Analysis System
        </div>
    </footer>
</body>
</html>
```

**Step 2: Create dashboard template**

```html
<!-- internal/api/templates/dashboard.html -->
{{define "content"}}
<div class="space-y-6">
    <h1 class="text-3xl font-bold text-gray-900">Dashboard</h1>

    <!-- Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div class="bg-white rounded-lg shadow p-6">
            <div class="text-sm text-gray-500">Total Signals Today</div>
            <div class="text-2xl font-bold text-indigo-600">{{.SignalsToday}}</div>
        </div>
        <div class="bg-white rounded-lg shadow p-6">
            <div class="text-sm text-gray-500">Buy Signals</div>
            <div class="text-2xl font-bold text-green-600">{{.BuySignals}}</div>
        </div>
        <div class="bg-white rounded-lg shadow p-6">
            <div class="text-sm text-gray-500">Sell Signals</div>
            <div class="text-2xl font-bold text-red-600">{{.SellSignals}}</div>
        </div>
        <div class="bg-white rounded-lg shadow p-6">
            <div class="text-sm text-gray-500">Watchlist Items</div>
            <div class="text-2xl font-bold text-gray-600">{{.WatchlistCount}}</div>
        </div>
    </div>

    <!-- Recent Signals -->
    <div class="bg-white rounded-lg shadow">
        <div class="px-6 py-4 border-b">
            <h2 class="text-lg font-semibold">Recent Signals</h2>
        </div>
        <div id="signals-list" hx-get="/api/signals/recent" hx-trigger="load, every 30s" hx-swap="innerHTML">
            <div class="p-6 text-gray-500">Loading signals...</div>
        </div>
    </div>
</div>
{{end}}
```

**Step 3: Create dashboard handler**

```go
// internal/api/handler/web/dashboard.go
package web

import (
	"html/template"
	"net/http"
	"path/filepath"
)

type DashboardData struct {
	Title          string
	SignalsToday   int
	BuySignals     int
	SellSignals    int
	WatchlistCount int
}

type Handler struct {
	templates *template.Template
}

func NewHandler(templatesDir string) (*Handler, error) {
	tmpl, err := template.ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, err
	}
	return &Handler{templates: tmpl}, nil
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data := DashboardData{
		Title:          "Dashboard",
		SignalsToday:   0, // TODO: Wire to actual data
		BuySignals:     0,
		SellSignals:    0,
		WatchlistCount: 0,
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
```

**Step 4: Run build to verify**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add internal/api/templates/ internal/api/handler/web/
git commit -m "feat: add web UI base layout and dashboard"
```

---

## Task 11: Add Signals Page

**Files:**
- Create: `internal/api/templates/signals.html`
- Create: `internal/api/handler/web/signals.go`

**Step 1: Create signals template**

```html
<!-- internal/api/templates/signals.html -->
{{define "content"}}
<div class="space-y-6">
    <div class="flex justify-between items-center">
        <h1 class="text-3xl font-bold text-gray-900">Signals</h1>
        <div class="flex space-x-2">
            <select id="filter-action" class="border rounded px-3 py-2"
                    hx-get="/signals" hx-target="#signals-table" hx-include="[name='filter']">
                <option value="">All Actions</option>
                <option value="buy">Buy</option>
                <option value="sell">Sell</option>
            </select>
            <select id="filter-strategy" class="border rounded px-3 py-2"
                    hx-get="/signals" hx-target="#signals-table" hx-include="[name='filter']">
                <option value="">All Strategies</option>
                <option value="ma_crossover">MA Crossover</option>
                <option value="pe_band">PE Band</option>
                <option value="dividend_yield">Dividend Yield</option>
            </select>
        </div>
    </div>

    <div class="bg-white rounded-lg shadow overflow-hidden">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Time</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Symbol</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Action</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Strategy</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Confidence</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Reason</th>
                </tr>
            </thead>
            <tbody id="signals-table" class="bg-white divide-y divide-gray-200">
                {{range .Signals}}
                <tr>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">{{.Time}}</td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">{{.Symbol}}</td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        {{if eq .Action "buy"}}
                        <span class="px-2 py-1 text-xs font-semibold rounded-full bg-green-100 text-green-800">BUY</span>
                        {{else}}
                        <span class="px-2 py-1 text-xs font-semibold rounded-full bg-red-100 text-red-800">SELL</span>
                        {{end}}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">{{.Strategy}}</td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">{{.Confidence}}%</td>
                    <td class="px-6 py-4 text-sm text-gray-500">{{.Reason}}</td>
                </tr>
                {{else}}
                <tr>
                    <td colspan="6" class="px-6 py-4 text-center text-gray-500">No signals found</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</div>
{{end}}
```

**Step 2: Create signals handler**

```go
// internal/api/handler/web/signals.go
package web

import (
	"net/http"
)

type SignalView struct {
	Time       string
	Symbol     string
	Action     string
	Strategy   string
	Confidence int
	Reason     string
}

type SignalsData struct {
	Title   string
	Signals []SignalView
}

func (h *Handler) Signals(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch actual signals from storage
	data := SignalsData{
		Title:   "Signals",
		Signals: []SignalView{}, // Empty for now
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
```

**Step 3: Run build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/api/templates/signals.html internal/api/handler/web/signals.go
git commit -m "feat: add signals page with filtering"
```

---

## Task 12: Add Watchlist Page

**Files:**
- Create: `internal/api/templates/watchlist.html`
- Create: `internal/api/handler/web/watchlist.go`

**Step 1: Create watchlist template**

```html
<!-- internal/api/templates/watchlist.html -->
{{define "content"}}
<div class="space-y-6">
    <div class="flex justify-between items-center">
        <h1 class="text-3xl font-bold text-gray-900">Watchlist</h1>
        <button onclick="document.getElementById('add-modal').classList.remove('hidden')"
                class="bg-indigo-600 text-white px-4 py-2 rounded hover:bg-indigo-700">
            Add Symbol
        </button>
    </div>

    <div class="bg-white rounded-lg shadow overflow-hidden">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Symbol</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Strategies</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
            </thead>
            <tbody id="watchlist-table" class="bg-white divide-y divide-gray-200">
                {{range .Items}}
                <tr>
                    <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">{{.Symbol}}</td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">{{.Name}}</td>
                    <td class="px-6 py-4 text-sm text-gray-500">
                        {{range .Strategies}}
                        <span class="inline-block bg-gray-100 rounded px-2 py-1 text-xs mr-1">{{.}}</span>
                        {{end}}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm">
                        <button hx-delete="/api/watchlist/{{.Symbol}}" hx-target="closest tr" hx-swap="outerHTML"
                                class="text-red-600 hover:text-red-800">Remove</button>
                    </td>
                </tr>
                {{else}}
                <tr>
                    <td colspan="4" class="px-6 py-4 text-center text-gray-500">No items in watchlist</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</div>

<!-- Add Modal -->
<div id="add-modal" class="hidden fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center">
    <div class="bg-white rounded-lg p-6 w-96">
        <h3 class="text-lg font-semibold mb-4">Add to Watchlist</h3>
        <form hx-post="/api/watchlist" hx-target="#watchlist-table" hx-swap="beforeend">
            <div class="space-y-4">
                <div>
                    <label class="block text-sm font-medium text-gray-700">Symbol</label>
                    <input type="text" name="symbol" required class="mt-1 block w-full border rounded px-3 py-2">
                </div>
                <div>
                    <label class="block text-sm font-medium text-gray-700">Name</label>
                    <input type="text" name="name" class="mt-1 block w-full border rounded px-3 py-2">
                </div>
                <div class="flex justify-end space-x-2">
                    <button type="button" onclick="document.getElementById('add-modal').classList.add('hidden')"
                            class="px-4 py-2 border rounded">Cancel</button>
                    <button type="submit" class="px-4 py-2 bg-indigo-600 text-white rounded">Add</button>
                </div>
            </div>
        </form>
    </div>
</div>
{{end}}
```

**Step 2: Create watchlist handler**

```go
// internal/api/handler/web/watchlist.go
package web

import (
	"net/http"
)

type WatchlistItem struct {
	Symbol     string
	Name       string
	Strategies []string
}

type WatchlistData struct {
	Title string
	Items []WatchlistItem
}

func (h *Handler) Watchlist(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from config/storage
	data := WatchlistData{
		Title: "Watchlist",
		Items: []WatchlistItem{},
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
```

**Step 3: Run build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/api/templates/watchlist.html internal/api/handler/web/watchlist.go
git commit -m "feat: add watchlist page with add/remove"
```

---

## Task 13: Add Backtest Page

**Files:**
- Create: `internal/api/templates/backtest.html`
- Create: `internal/api/handler/web/backtest.go`

**Step 1: Create backtest template**

```html
<!-- internal/api/templates/backtest.html -->
{{define "content"}}
<div class="space-y-6">
    <h1 class="text-3xl font-bold text-gray-900">Backtest</h1>

    <div class="bg-white rounded-lg shadow p-6">
        <form hx-post="/api/backtest" hx-target="#results" hx-swap="innerHTML">
            <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
                <div>
                    <label class="block text-sm font-medium text-gray-700">Strategy</label>
                    <select name="strategy" required class="mt-1 block w-full border rounded px-3 py-2">
                        <option value="ma_crossover">MA Crossover</option>
                        <option value="pe_band">PE Band</option>
                        <option value="dividend_yield">Dividend Yield</option>
                    </select>
                </div>
                <div>
                    <label class="block text-sm font-medium text-gray-700">Symbol</label>
                    <input type="text" name="symbol" required placeholder="AAPL"
                           class="mt-1 block w-full border rounded px-3 py-2">
                </div>
                <div>
                    <label class="block text-sm font-medium text-gray-700">From</label>
                    <input type="date" name="from" required class="mt-1 block w-full border rounded px-3 py-2">
                </div>
                <div>
                    <label class="block text-sm font-medium text-gray-700">To</label>
                    <input type="date" name="to" required class="mt-1 block w-full border rounded px-3 py-2">
                </div>
            </div>
            <div class="mt-4">
                <button type="submit" class="bg-indigo-600 text-white px-6 py-2 rounded hover:bg-indigo-700">
                    Run Backtest
                </button>
            </div>
        </form>
    </div>

    <div id="results">
        {{if .Result}}
        <div class="bg-white rounded-lg shadow p-6">
            <h2 class="text-xl font-semibold mb-4">Results: {{.Result.Strategy}} on {{.Result.Symbol}}</h2>

            <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
                <div class="bg-gray-50 p-4 rounded">
                    <div class="text-sm text-gray-500">Total Trades</div>
                    <div class="text-2xl font-bold">{{.Result.Stats.TotalTrades}}</div>
                </div>
                <div class="bg-gray-50 p-4 rounded">
                    <div class="text-sm text-gray-500">Win Rate</div>
                    <div class="text-2xl font-bold text-green-600">{{printf "%.1f" .Result.Stats.WinRate}}%</div>
                </div>
                <div class="bg-gray-50 p-4 rounded">
                    <div class="text-sm text-gray-500">Total Return</div>
                    <div class="text-2xl font-bold {{if gt .Result.Stats.TotalReturn 0.0}}text-green-600{{else}}text-red-600{{end}}">
                        {{printf "%.1f" .Result.Stats.TotalReturn}}%
                    </div>
                </div>
                <div class="bg-gray-50 p-4 rounded">
                    <div class="text-sm text-gray-500">Max Drawdown</div>
                    <div class="text-2xl font-bold text-red-600">{{printf "%.1f" .Result.Stats.MaxDrawdown}}%</div>
                </div>
            </div>

            <h3 class="font-semibold mb-2">Trades</h3>
            <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                    <tr>
                        <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">Entry</th>
                        <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">Exit</th>
                        <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">Return</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Result.Trades}}
                    <tr>
                        <td class="px-4 py-2 text-sm">{{printf "%.2f" .EntryPrice}}</td>
                        <td class="px-4 py-2 text-sm">{{if .IsClosed}}{{printf "%.2f" .ExitPrice}}{{else}}-{{end}}</td>
                        <td class="px-4 py-2 text-sm {{if gt .Return 0.0}}text-green-600{{else}}text-red-600{{end}}">
                            {{printf "%.2f" .Return}}%
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
    </div>
</div>
{{end}}
```

**Step 2: Create backtest handler**

```go
// internal/api/handler/web/backtest.go
package web

import (
	"net/http"

	"github.com/newthinker/atlas/internal/backtest"
)

type BacktestData struct {
	Title  string
	Result *backtest.Result
}

func (h *Handler) Backtest(w http.ResponseWriter, r *http.Request) {
	data := BacktestData{
		Title:  "Backtest",
		Result: nil,
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
```

**Step 3: Run build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/api/templates/backtest.html internal/api/handler/web/backtest.go
git commit -m "feat: add backtest page with results display"
```

---

## Task 14: Wire Up Web Routes

**Files:**
- Modify: `internal/api/server.go`

**Step 1: Add web routes to server**

Add to the server setup:

```go
// Add to imports
import (
	"embed"
	"github.com/newthinker/atlas/internal/api/handler/web"
)

//go:embed templates/*.html
var templates embed.FS

// In server setup function, add:
func (s *Server) setupRoutes() {
	// ... existing API routes ...

	// Web UI routes
	webHandler, err := web.NewHandler("internal/api/templates")
	if err != nil {
		// Handle error
	}

	s.router.GET("/", webHandler.Dashboard)
	s.router.GET("/signals", webHandler.Signals)
	s.router.GET("/watchlist", webHandler.Watchlist)
	s.router.GET("/backtest", webHandler.Backtest)
}
```

**Step 2: Run build**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add internal/api/server.go
git commit -m "feat: wire up web UI routes"
```

---

## Task 15: Final Integration Test

**Step 1: Run full test suite**

```bash
go test ./... -cover
```

**Step 2: Build and verify**

```bash
go build -o bin/atlas ./cmd/atlas
go vet ./...
./bin/atlas version
./bin/atlas --help
./bin/atlas backtest --help
```

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "chore: Phase 3 final cleanup and integration"
```

---

## Summary

Phase 3 adds:
- **S3 Storage** (Tasks 1-5): Config, interface, LocalFS impl, S3 impl
- **Backtesting** (Tasks 6-9): Types, stats, engine, CLI command
- **Web UI** (Tasks 10-14): Layout, dashboard, signals, watchlist, backtest pages
- **Integration** (Task 15): Full test suite and verification
