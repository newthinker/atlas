# Crypto Collector Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a cryptocurrency data collector with multi-source support (Binance, CoinGecko, OKX) and automatic fallback.

**Architecture:** Provider-based design with a main CryptoCollector that delegates to multiple data source providers. Symbol normalization layer converts various input formats (BTC, btc-usdt, BTC/USDT) to internal format (BTCUSDT). Automatic fallback chain tries providers in priority order.

**Tech Stack:** Go, HTTP client, JSON parsing, existing collector interface

---

## Task 1: Add MarketCrypto Constant to Core Types

**Files:**
- Modify: `internal/core/types.go:8-13`

**Step 1: Add MarketCrypto constant**

Add `MarketCrypto` to the existing Market constants:

```go
const (
	MarketUS     Market = "US"
	MarketHK     Market = "HK"
	MarketCNA    Market = "CN_A"
	MarketEU     Market = "EU"
	MarketCrypto Market = "CRYPTO"  // Add this line
)
```

**Step 2: Commit**

```bash
git add internal/core/types.go
git commit -m "feat(core): add MarketCrypto constant"
```

---

## Task 2: Create Symbol Normalization Module

**Files:**
- Create: `internal/collector/crypto/symbol.go`
- Create: `internal/collector/crypto/symbol_test.go`

**Step 1: Write the failing tests**

Create `internal/collector/crypto/symbol_test.go`:

```go
package crypto

import (
	"testing"
)

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct {
		input        string
		defaultQuote string
		expected     string
	}{
		// Basic cases - add default quote
		{"BTC", "USDT", "BTCUSDT"},
		{"btc", "USDT", "BTCUSDT"},
		{"eth", "USDT", "ETHUSDT"},
		{"ETH", "USDT", "ETHUSDT"},

		// With separators
		{"BTC-USDT", "USDT", "BTCUSDT"},
		{"BTC/USDT", "USDT", "BTCUSDT"},
		{"btc-usdt", "USDT", "BTCUSDT"},
		{"btc/usdt", "USDT", "BTCUSDT"},
		{"BTC_USDT", "USDT", "BTCUSDT"},

		// Already normalized
		{"BTCUSDT", "USDT", "BTCUSDT"},
		{"btcusdt", "USDT", "BTCUSDT"},
		{"ETHUSDT", "USDT", "ETHUSDT"},

		// Different quote currencies
		{"BTC-BUSD", "USDT", "BTCBUSD"},
		{"ETH/BTC", "USDT", "ETHBTC"},
		{"BTC", "BUSD", "BTCBUSD"},

		// Edge cases
		{"SOLUSDT", "USDT", "SOLUSDT"},
		{"SOL", "USDT", "SOLUSDT"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeSymbol(tc.input, tc.defaultQuote)
			if got != tc.expected {
				t.Errorf("NormalizeSymbol(%q, %q) = %q, want %q",
					tc.input, tc.defaultQuote, got, tc.expected)
			}
		})
	}
}

func TestParseSymbol(t *testing.T) {
	tests := []struct {
		symbol       string
		expectedBase string
		expectedQuote string
	}{
		{"BTCUSDT", "BTC", "USDT"},
		{"ETHUSDT", "ETH", "USDT"},
		{"ETHBTC", "ETH", "BTC"},
		{"SOLUSDT", "SOL", "USDT"},
		{"BTCBUSD", "BTC", "BUSD"},
		{"DOGEUSDT", "DOGE", "USDT"},
	}

	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			base, quote := ParseSymbol(tc.symbol)
			if base != tc.expectedBase || quote != tc.expectedQuote {
				t.Errorf("ParseSymbol(%q) = (%q, %q), want (%q, %q)",
					tc.symbol, base, quote, tc.expectedBase, tc.expectedQuote)
			}
		})
	}
}

func TestFormatDisplay(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "BTC/USDT"},
		{"ETHUSDT", "ETH/USDT"},
		{"ETHBTC", "ETH/BTC"},
	}

	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			got := FormatDisplay(tc.symbol)
			if got != tc.expected {
				t.Errorf("FormatDisplay(%q) = %q, want %q", tc.symbol, got, tc.expected)
			}
		})
	}
}

func TestValidateCryptoSymbol(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		wantErr bool
	}{
		{"valid symbol", "BTCUSDT", false},
		{"valid lowercase", "btcusdt", false},
		{"empty symbol", "", true},
		{"too long", "VERYLONGSYMBOLNAME12345678901234567890", true},
		{"invalid chars", "BTC!USDT", true},
		{"path injection", "../etc/passwd", true},
		{"url injection", "BTC?foo=bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCryptoSymbol(tt.symbol)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCryptoSymbol(%q) error = %v, wantErr %v",
					tt.symbol, err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/collector/crypto/... -v`
Expected: FAIL (package doesn't exist yet)

**Step 3: Implement symbol.go**

Create `internal/collector/crypto/symbol.go`:

```go
package crypto

import (
	"fmt"
	"regexp"
	"strings"
)

// Common quote currencies in order of priority for detection
var quoteCurrencies = []string{"USDT", "BUSD", "USDC", "BTC", "ETH", "BNB"}

// validSymbol matches crypto trading pairs
var validCryptoSymbol = regexp.MustCompile(`^[A-Za-z0-9]{2,20}$`)

// NormalizeSymbol converts various input formats to standard format (e.g., BTCUSDT)
// Input formats: "BTC", "btc", "BTC-USDT", "BTC/USDT", "btcusdt"
// Output: "BTCUSDT"
func NormalizeSymbol(input string, defaultQuote string) string {
	if input == "" {
		return ""
	}

	// Uppercase and remove common separators
	s := strings.ToUpper(input)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "_", "")

	// Check if already contains a quote currency
	for _, quote := range quoteCurrencies {
		if strings.HasSuffix(s, quote) {
			return s
		}
	}

	// No quote currency found, append default
	return s + strings.ToUpper(defaultQuote)
}

// ParseSymbol extracts base and quote from a normalized symbol
// "BTCUSDT" -> ("BTC", "USDT")
func ParseSymbol(symbol string) (base, quote string) {
	s := strings.ToUpper(symbol)

	// Try to find known quote currency
	for _, q := range quoteCurrencies {
		if strings.HasSuffix(s, q) {
			return strings.TrimSuffix(s, q), q
		}
	}

	// Fallback: assume last 4 chars are quote (USDT, BUSD, etc.)
	if len(s) > 4 {
		return s[:len(s)-4], s[len(s)-4:]
	}

	return s, ""
}

// FormatDisplay converts internal format to display format
// "BTCUSDT" -> "BTC/USDT"
func FormatDisplay(symbol string) string {
	base, quote := ParseSymbol(symbol)
	if quote == "" {
		return base
	}
	return base + "/" + quote
}

// ValidateCryptoSymbol checks if a symbol has valid format
func ValidateCryptoSymbol(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol cannot be empty")
	}
	if len(symbol) > 30 {
		return fmt.Errorf("symbol too long: %s", symbol)
	}

	// Remove separators for validation
	s := strings.ReplaceAll(symbol, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "_", "")

	if !validCryptoSymbol.MatchString(s) {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/crypto/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/collector/crypto/
git commit -m "feat(crypto): add symbol normalization module"
```

---

## Task 3: Create Provider Interface

**Files:**
- Create: `internal/collector/crypto/provider.go`

**Step 1: Create provider interface**

Create `internal/collector/crypto/provider.go`:

```go
package crypto

import (
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Provider defines the interface for cryptocurrency data sources
type Provider interface {
	// Name returns the provider identifier (e.g., "binance", "coingecko")
	Name() string

	// FetchQuote fetches real-time quote for a normalized symbol (e.g., "BTCUSDT")
	FetchQuote(symbol string) (*core.Quote, error)

	// FetchHistory fetches historical OHLCV data
	// symbol: normalized format (e.g., "BTCUSDT")
	// interval: "1m", "5m", "15m", "1h", "4h", "1d"
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}
```

**Step 2: Commit**

```bash
git add internal/collector/crypto/provider.go
git commit -m "feat(crypto): add Provider interface"
```

---

## Task 4: Implement Binance Provider

**Files:**
- Create: `internal/collector/crypto/binance/binance.go`
- Create: `internal/collector/crypto/binance/binance_test.go`

**Step 1: Write the failing tests**

Create `internal/collector/crypto/binance/binance_test.go`:

```go
package binance

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

func TestBinance_ImplementsProvider(t *testing.T) {
	var _ crypto.Provider = (*Binance)(nil)
}

func TestBinance_Name(t *testing.T) {
	b := New()
	if b.Name() != "binance" {
		t.Errorf("expected 'binance', got '%s'", b.Name())
	}
}

func TestBinance_ToInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1m", "1m"},
		{"5m", "5m"},
		{"15m", "15m"},
		{"1h", "1h"},
		{"4h", "4h"},
		{"1d", "1d"},
		{"unknown", "1d"},
	}

	b := New()
	for _, tc := range tests {
		got := b.toInterval(tc.input)
		if got != tc.expected {
			t.Errorf("toInterval(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

// Integration test - skip in CI
func TestBinance_FetchQuote_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	b := New()
	quote, err := b.FetchQuote("BTCUSDT")
	if err != nil {
		t.Fatalf("FetchQuote failed: %v", err)
	}

	if quote.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", quote.Symbol)
	}
	if quote.Price <= 0 {
		t.Errorf("expected positive price, got %f", quote.Price)
	}
	if quote.Market != core.MarketCrypto {
		t.Errorf("expected market CRYPTO, got %s", quote.Market)
	}
}

// Integration test - skip in CI
func TestBinance_FetchHistory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	b := New()
	end := time.Now()
	start := end.AddDate(0, 0, -7) // Last 7 days

	data, err := b.FetchHistory("BTCUSDT", start, end, "1d")
	if err != nil {
		t.Fatalf("FetchHistory failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected at least one OHLCV record")
	}

	for _, ohlcv := range data {
		if ohlcv.Symbol != "BTCUSDT" {
			t.Errorf("expected symbol BTCUSDT, got %s", ohlcv.Symbol)
		}
		if ohlcv.Close <= 0 {
			t.Errorf("expected positive close price, got %f", ohlcv.Close)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/collector/crypto/binance/... -v`
Expected: FAIL (package doesn't exist yet)

**Step 3: Implement binance.go**

Create `internal/collector/crypto/binance/binance.go`:

```go
package binance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

const (
	baseURL = "https://api.binance.com"
)

// Binance implements the crypto Provider interface for Binance exchange
type Binance struct {
	client  *http.Client
	baseURL string
}

// New creates a new Binance provider
func New() *Binance {
	return &Binance{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
	}
}

// NewWithBaseURL creates a Binance provider with custom base URL (for testing)
func NewWithBaseURL(url string) *Binance {
	b := New()
	b.baseURL = url
	return b
}

func (b *Binance) Name() string {
	return "binance"
}

// FetchQuote fetches real-time quote from Binance
func (b *Binance) FetchQuote(symbol string) (*core.Quote, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", b.baseURL, symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result ticker24hr
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	price, _ := strconv.ParseFloat(result.LastPrice, 64)
	open, _ := strconv.ParseFloat(result.OpenPrice, 64)
	high, _ := strconv.ParseFloat(result.HighPrice, 64)
	low, _ := strconv.ParseFloat(result.LowPrice, 64)
	prevClose, _ := strconv.ParseFloat(result.PrevClosePrice, 64)
	change, _ := strconv.ParseFloat(result.PriceChange, 64)
	changePercent, _ := strconv.ParseFloat(result.PriceChangePercent, 64)
	volume, _ := strconv.ParseFloat(result.Volume, 64)
	bidPrice, _ := strconv.ParseFloat(result.BidPrice, 64)
	askPrice, _ := strconv.ParseFloat(result.AskPrice, 64)

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCrypto,
		Price:         price,
		Open:          open,
		High:          high,
		Low:           low,
		PrevClose:     prevClose,
		Change:        change,
		ChangePercent: changePercent,
		Volume:        int64(volume),
		Bid:           bidPrice,
		Ask:           askPrice,
		Time:          time.UnixMilli(result.CloseTime),
		Source:        "binance",
	}, nil
}

// FetchHistory fetches historical OHLCV data from Binance
func (b *Binance) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	binanceInterval := b.toInterval(interval)
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=1000",
		b.baseURL, symbol, binanceInterval, start.UnixMilli(), end.UnixMilli())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var klines [][]any
	if err := json.NewDecoder(resp.Body).Decode(&klines); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	data := make([]core.OHLCV, 0, len(klines))
	for _, k := range klines {
		if len(k) < 6 {
			continue
		}

		openTime, _ := k[0].(float64)
		openStr, _ := k[1].(string)
		highStr, _ := k[2].(string)
		lowStr, _ := k[3].(string)
		closeStr, _ := k[4].(string)
		volumeStr, _ := k[5].(string)

		open, _ := strconv.ParseFloat(openStr, 64)
		high, _ := strconv.ParseFloat(highStr, 64)
		low, _ := strconv.ParseFloat(lowStr, 64)
		close, _ := strconv.ParseFloat(closeStr, 64)
		volume, _ := strconv.ParseFloat(volumeStr, 64)

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   int64(volume),
			Time:     time.UnixMilli(int64(openTime)),
		})
	}

	return data, nil
}

func (b *Binance) toInterval(interval string) string {
	switch interval {
	case "1m", "5m", "15m", "30m":
		return interval
	case "1h", "2h", "4h":
		return interval
	case "1d":
		return "1d"
	case "1w":
		return "1w"
	default:
		return "1d"
	}
}

// Binance API response types
type ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	OpenPrice          string `json:"openPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	PrevClosePrice     string `json:"prevClosePrice"`
	BidPrice           string `json:"bidPrice"`
	AskPrice           string `json:"askPrice"`
	CloseTime          int64  `json:"closeTime"`
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/crypto/binance/... -v`
Expected: PASS (unit tests)

**Step 5: Commit**

```bash
git add internal/collector/crypto/binance/
git commit -m "feat(crypto): implement Binance provider"
```

---

## Task 5: Implement CoinGecko Provider

**Files:**
- Create: `internal/collector/crypto/coingecko/coingecko.go`
- Create: `internal/collector/crypto/coingecko/coingecko_test.go`

**Step 1: Write the failing tests**

Create `internal/collector/crypto/coingecko/coingecko_test.go`:

```go
package coingecko

import (
	"testing"

	"github.com/newthinker/atlas/internal/collector/crypto"
)

func TestCoinGecko_ImplementsProvider(t *testing.T) {
	var _ crypto.Provider = (*CoinGecko)(nil)
}

func TestCoinGecko_Name(t *testing.T) {
	c := New("")
	if c.Name() != "coingecko" {
		t.Errorf("expected 'coingecko', got '%s'", c.Name())
	}
}

func TestCoinGecko_SymbolToID(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "bitcoin"},
		{"ETHUSDT", "ethereum"},
		{"BNBUSDT", "binancecoin"},
		{"SOLUSDT", "solana"},
		{"XRPUSDT", "ripple"},
		{"DOGEUSDT", "dogecoin"},
		{"ADAUSDT", "cardano"},
		{"UNKNOWN", "unknown"},
	}

	c := New("")
	for _, tc := range tests {
		got := c.symbolToID(tc.symbol)
		if got != tc.expected {
			t.Errorf("symbolToID(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}

func TestCoinGecko_SymbolToVsCurrency(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "usd"},
		{"ETHBTC", "btc"},
		{"SOLETH", "eth"},
		{"BTCBUSD", "usd"},
	}

	c := New("")
	for _, tc := range tests {
		got := c.symbolToVsCurrency(tc.symbol)
		if got != tc.expected {
			t.Errorf("symbolToVsCurrency(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/collector/crypto/coingecko/... -v`
Expected: FAIL

**Step 3: Implement coingecko.go**

Create `internal/collector/crypto/coingecko/coingecko.go`:

```go
package coingecko

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

const (
	baseURL = "https://api.coingecko.com/api/v3"
)

// Symbol to CoinGecko ID mapping
var symbolToIDMap = map[string]string{
	"BTC":  "bitcoin",
	"ETH":  "ethereum",
	"BNB":  "binancecoin",
	"SOL":  "solana",
	"XRP":  "ripple",
	"DOGE": "dogecoin",
	"ADA":  "cardano",
	"AVAX": "avalanche-2",
	"DOT":  "polkadot",
	"MATIC": "matic-network",
	"LINK": "chainlink",
	"UNI":  "uniswap",
	"ATOM": "cosmos",
	"LTC":  "litecoin",
	"ETC":  "ethereum-classic",
	"XLM":  "stellar",
	"ALGO": "algorand",
	"NEAR": "near",
	"FTM":  "fantom",
	"SAND": "the-sandbox",
	"MANA": "decentraland",
	"AAVE": "aave",
	"CRV":  "curve-dao-token",
	"APE":  "apecoin",
	"LDO":  "lido-dao",
	"ARB":  "arbitrum",
	"OP":   "optimism",
}

// CoinGecko implements the crypto Provider interface
type CoinGecko struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

// New creates a new CoinGecko provider
func New(apiKey string) *CoinGecko {
	return &CoinGecko{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

func (c *CoinGecko) Name() string {
	return "coingecko"
}

// symbolToID converts trading pair to CoinGecko coin ID
func (c *CoinGecko) symbolToID(symbol string) string {
	base, _ := crypto.ParseSymbol(symbol)
	if id, ok := symbolToIDMap[base]; ok {
		return id
	}
	return strings.ToLower(base)
}

// symbolToVsCurrency extracts the quote currency for CoinGecko API
func (c *CoinGecko) symbolToVsCurrency(symbol string) string {
	_, quote := crypto.ParseSymbol(symbol)
	switch quote {
	case "USDT", "USDC", "BUSD", "USD":
		return "usd"
	case "BTC":
		return "btc"
	case "ETH":
		return "eth"
	default:
		return "usd"
	}
}

// FetchQuote fetches real-time quote from CoinGecko
func (c *CoinGecko) FetchQuote(symbol string) (*core.Quote, error) {
	coinID := c.symbolToID(symbol)
	vsCurrency := c.symbolToVsCurrency(symbol)

	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=%s&include_24hr_vol=true&include_24hr_change=true&include_last_updated_at=true",
		c.baseURL, coinID, vsCurrency)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	coinData, ok := result[coinID]
	if !ok {
		return nil, fmt.Errorf("no data for coin: %s", coinID)
	}

	price := coinData[vsCurrency]
	volume := coinData[vsCurrency+"_24h_vol"]
	changePercent := coinData[vsCurrency+"_24h_change"]
	lastUpdated := coinData["last_updated_at"]

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCrypto,
		Price:         price,
		Volume:        int64(volume),
		ChangePercent: changePercent,
		Time:          time.Unix(int64(lastUpdated), 0),
		Source:        "coingecko",
	}, nil
}

// FetchHistory fetches historical OHLCV data from CoinGecko
func (c *CoinGecko) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	coinID := c.symbolToID(symbol)
	vsCurrency := c.symbolToVsCurrency(symbol)

	// CoinGecko uses days parameter
	days := int(end.Sub(start).Hours() / 24)
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}

	url := fmt.Sprintf("%s/coins/%s/ohlc?vs_currency=%s&days=%d",
		c.baseURL, coinID, vsCurrency, days)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// CoinGecko returns [[timestamp, open, high, low, close], ...]
	var ohlcData [][]float64
	if err := json.NewDecoder(resp.Body).Decode(&ohlcData); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	data := make([]core.OHLCV, 0, len(ohlcData))
	for _, ohlc := range ohlcData {
		if len(ohlc) < 5 {
			continue
		}

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     ohlc[1],
			High:     ohlc[2],
			Low:      ohlc[3],
			Close:    ohlc[4],
			Time:     time.UnixMilli(int64(ohlc[0])),
		})
	}

	return data, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/crypto/coingecko/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/collector/crypto/coingecko/
git commit -m "feat(crypto): implement CoinGecko provider"
```

---

## Task 6: Implement OKX Provider

**Files:**
- Create: `internal/collector/crypto/okx/okx.go`
- Create: `internal/collector/crypto/okx/okx_test.go`

**Step 1: Write the failing tests**

Create `internal/collector/crypto/okx/okx_test.go`:

```go
package okx

import (
	"testing"

	"github.com/newthinker/atlas/internal/collector/crypto"
)

func TestOKX_ImplementsProvider(t *testing.T) {
	var _ crypto.Provider = (*OKX)(nil)
}

func TestOKX_Name(t *testing.T) {
	o := New()
	if o.Name() != "okx" {
		t.Errorf("expected 'okx', got '%s'", o.Name())
	}
}

func TestOKX_ToInstID(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "BTC-USDT"},
		{"ETHUSDT", "ETH-USDT"},
		{"SOLUSDT", "SOL-USDT"},
		{"ETHBTC", "ETH-BTC"},
	}

	o := New()
	for _, tc := range tests {
		got := o.toInstID(tc.symbol)
		if got != tc.expected {
			t.Errorf("toInstID(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}

func TestOKX_ToInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1m", "1m"},
		{"5m", "5m"},
		{"1h", "1H"},
		{"4h", "4H"},
		{"1d", "1D"},
	}

	o := New()
	for _, tc := range tests {
		got := o.toInterval(tc.input)
		if got != tc.expected {
			t.Errorf("toInterval(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/collector/crypto/okx/... -v`
Expected: FAIL

**Step 3: Implement okx.go**

Create `internal/collector/crypto/okx/okx.go`:

```go
package okx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

const (
	baseURL = "https://www.okx.com"
)

// OKX implements the crypto Provider interface for OKX exchange
type OKX struct {
	client  *http.Client
	baseURL string
}

// New creates a new OKX provider
func New() *OKX {
	return &OKX{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
	}
}

func (o *OKX) Name() string {
	return "okx"
}

// toInstID converts normalized symbol to OKX instrument ID
// BTCUSDT -> BTC-USDT
func (o *OKX) toInstID(symbol string) string {
	base, quote := crypto.ParseSymbol(symbol)
	return base + "-" + quote
}

// FetchQuote fetches real-time quote from OKX
func (o *OKX) FetchQuote(symbol string) (*core.Quote, error) {
	instID := o.toInstID(symbol)
	url := fmt.Sprintf("%s/api/v5/market/ticker?instId=%s", o.baseURL, instID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result okxTickerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Code != "0" || len(result.Data) == 0 {
		return nil, fmt.Errorf("okx error: %s", result.Msg)
	}

	data := result.Data[0]
	price, _ := strconv.ParseFloat(data.Last, 64)
	open, _ := strconv.ParseFloat(data.Open24h, 64)
	high, _ := strconv.ParseFloat(data.High24h, 64)
	low, _ := strconv.ParseFloat(data.Low24h, 64)
	volume, _ := strconv.ParseFloat(data.Vol24h, 64)
	bidPrice, _ := strconv.ParseFloat(data.BidPx, 64)
	askPrice, _ := strconv.ParseFloat(data.AskPx, 64)
	ts, _ := strconv.ParseInt(data.Ts, 10, 64)

	change := price - open
	changePercent := 0.0
	if open > 0 {
		changePercent = (change / open) * 100
	}

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCrypto,
		Price:         price,
		Open:          open,
		High:          high,
		Low:           low,
		Change:        change,
		ChangePercent: changePercent,
		Volume:        int64(volume),
		Bid:           bidPrice,
		Ask:           askPrice,
		Time:          time.UnixMilli(ts),
		Source:        "okx",
	}, nil
}

// FetchHistory fetches historical OHLCV data from OKX
func (o *OKX) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	instID := o.toInstID(symbol)
	okxInterval := o.toInterval(interval)

	url := fmt.Sprintf("%s/api/v5/market/candles?instId=%s&bar=%s&before=%d&after=%d&limit=300",
		o.baseURL, instID, okxInterval, start.UnixMilli(), end.UnixMilli())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result okxCandleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Code != "0" {
		return nil, fmt.Errorf("okx error: %s", result.Msg)
	}

	data := make([]core.OHLCV, 0, len(result.Data))
	// OKX returns newest first, reverse for chronological order
	for i := len(result.Data) - 1; i >= 0; i-- {
		candle := result.Data[i]
		if len(candle) < 6 {
			continue
		}

		ts, _ := strconv.ParseInt(candle[0], 10, 64)
		open, _ := strconv.ParseFloat(candle[1], 64)
		high, _ := strconv.ParseFloat(candle[2], 64)
		low, _ := strconv.ParseFloat(candle[3], 64)
		closePrice, _ := strconv.ParseFloat(candle[4], 64)
		volume, _ := strconv.ParseFloat(candle[5], 64)

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    closePrice,
			Volume:   int64(volume),
			Time:     time.UnixMilli(ts),
		})
	}

	return data, nil
}

func (o *OKX) toInterval(interval string) string {
	switch interval {
	case "1m", "5m", "15m", "30m":
		return interval
	case "1h":
		return "1H"
	case "2h":
		return "2H"
	case "4h":
		return "4H"
	case "1d":
		return "1D"
	case "1w":
		return "1W"
	default:
		return "1D"
	}
}

// OKX API response types
type okxTickerResponse struct {
	Code string       `json:"code"`
	Msg  string       `json:"msg"`
	Data []okxTicker  `json:"data"`
}

type okxTicker struct {
	InstId  string `json:"instId"`
	Last    string `json:"last"`
	Open24h string `json:"open24h"`
	High24h string `json:"high24h"`
	Low24h  string `json:"low24h"`
	Vol24h  string `json:"vol24h"`
	BidPx   string `json:"bidPx"`
	AskPx   string `json:"askPx"`
	Ts      string `json:"ts"`
}

type okxCandleResponse struct {
	Code string     `json:"code"`
	Msg  string     `json:"msg"`
	Data [][]string `json:"data"`
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/crypto/okx/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/collector/crypto/okx/
git commit -m "feat(crypto): implement OKX provider"
```

---

## Task 7: Implement Main CryptoCollector

**Files:**
- Create: `internal/collector/crypto/crypto.go`
- Create: `internal/collector/crypto/crypto_test.go`

**Step 1: Write the failing tests**

Create `internal/collector/crypto/crypto_test.go`:

```go
package crypto

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

func TestCryptoCollector_ImplementsCollector(t *testing.T) {
	var _ collector.Collector = (*CryptoCollector)(nil)
}

func TestCryptoCollector_Name(t *testing.T) {
	c := New()
	if c.Name() != "crypto" {
		t.Errorf("expected 'crypto', got '%s'", c.Name())
	}
}

func TestCryptoCollector_SupportedMarkets(t *testing.T) {
	c := New()
	markets := c.SupportedMarkets()

	if len(markets) != 1 {
		t.Errorf("expected 1 market, got %d", len(markets))
	}
	if markets[0] != core.MarketCrypto {
		t.Errorf("expected MarketCrypto, got %s", markets[0])
	}
}

// Mock provider for testing
type mockProvider struct {
	name       string
	quote      *core.Quote
	history    []core.OHLCV
	quoteErr   error
	historyErr error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) FetchQuote(symbol string) (*core.Quote, error) {
	if m.quoteErr != nil {
		return nil, m.quoteErr
	}
	return m.quote, nil
}

func (m *mockProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	return m.history, nil
}

func TestCryptoCollector_FetchQuote_Fallback(t *testing.T) {
	failProvider := &mockProvider{
		name:     "fail",
		quoteErr: fmt.Errorf("provider error"),
	}
	successProvider := &mockProvider{
		name: "success",
		quote: &core.Quote{
			Symbol: "BTCUSDT",
			Price:  50000,
		},
	}

	c := New()
	c.providers = []Provider{failProvider, successProvider}

	quote, err := c.FetchQuote("BTC")
	if err != nil {
		t.Fatalf("expected success after fallback, got error: %v", err)
	}
	if quote.Price != 50000 {
		t.Errorf("expected price 50000, got %f", quote.Price)
	}
	if quote.Source != "crypto:success" {
		t.Errorf("expected source 'crypto:success', got %s", quote.Source)
	}
}

func TestCryptoCollector_FetchQuote_AllFail(t *testing.T) {
	fail1 := &mockProvider{name: "fail1", quoteErr: fmt.Errorf("error1")}
	fail2 := &mockProvider{name: "fail2", quoteErr: fmt.Errorf("error2")}

	c := New()
	c.providers = []Provider{fail1, fail2}

	_, err := c.FetchQuote("BTC")
	if err == nil {
		t.Error("expected error when all providers fail")
	}
}

func TestCryptoCollector_NormalizesSymbol(t *testing.T) {
	successProvider := &mockProvider{
		name: "test",
		quote: &core.Quote{
			Symbol: "BTCUSDT",
			Price:  50000,
		},
	}

	c := New()
	c.providers = []Provider{successProvider}
	c.defaultQuote = "USDT"

	// Should normalize "BTC" to "BTCUSDT"
	quote, err := c.FetchQuote("BTC")
	if err != nil {
		t.Fatalf("FetchQuote failed: %v", err)
	}
	if quote.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", quote.Symbol)
	}
}
```

**Step 2: Add missing import**

Add `"fmt"` to the imports in the test file.

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/collector/crypto/... -v -run TestCryptoCollector`
Expected: FAIL

**Step 4: Implement crypto.go**

Create `internal/collector/crypto/crypto.go`:

```go
package crypto

import (
	"context"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto/binance"
	"github.com/newthinker/atlas/internal/collector/crypto/coingecko"
	"github.com/newthinker/atlas/internal/collector/crypto/okx"
	"github.com/newthinker/atlas/internal/core"
)

// CryptoCollector implements collector.Collector for cryptocurrency markets
type CryptoCollector struct {
	providers    []Provider
	defaultQuote string
	config       collector.Config
}

// New creates a new CryptoCollector with default providers
func New() *CryptoCollector {
	return &CryptoCollector{
		providers: []Provider{
			binance.New(),
			coingecko.New(""),
			okx.New(),
		},
		defaultQuote: "USDT",
	}
}

// NewWithProviders creates a CryptoCollector with custom providers
func NewWithProviders(providers []Provider, defaultQuote string) *CryptoCollector {
	if defaultQuote == "" {
		defaultQuote = "USDT"
	}
	return &CryptoCollector{
		providers:    providers,
		defaultQuote: defaultQuote,
	}
}

func (c *CryptoCollector) Name() string {
	return "crypto"
}

func (c *CryptoCollector) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketCrypto}
}

func (c *CryptoCollector) Init(cfg collector.Config) error {
	c.config = cfg

	// Configure default quote from config if provided
	if quote, ok := cfg.Extra["default_quote"].(string); ok && quote != "" {
		c.defaultQuote = quote
	}

	// Configure providers from config if provided
	if providerNames, ok := cfg.Extra["providers"].([]string); ok && len(providerNames) > 0 {
		providers := make([]Provider, 0, len(providerNames))
		for _, name := range providerNames {
			switch name {
			case "binance":
				providers = append(providers, binance.New())
			case "coingecko":
				apiKey := ""
				if key, ok := cfg.Extra["coingecko_api_key"].(string); ok {
					apiKey = key
				}
				providers = append(providers, coingecko.New(apiKey))
			case "okx":
				providers = append(providers, okx.New())
			}
		}
		if len(providers) > 0 {
			c.providers = providers
		}
	}

	return nil
}

func (c *CryptoCollector) Start(ctx context.Context) error {
	return nil
}

func (c *CryptoCollector) Stop() error {
	return nil
}

// FetchQuote fetches real-time quote with automatic fallback
func (c *CryptoCollector) FetchQuote(symbol string) (*core.Quote, error) {
	// Validate and normalize symbol
	if err := ValidateCryptoSymbol(symbol); err != nil {
		return nil, err
	}
	normalized := NormalizeSymbol(symbol, c.defaultQuote)

	// Try each provider in order
	var lastErr error
	for _, p := range c.providers {
		quote, err := p.FetchQuote(normalized)
		if err == nil {
			quote.Symbol = normalized
			quote.Source = "crypto:" + p.Name()
			return quote, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("all providers failed for %s: %w", normalized, lastErr)
}

// FetchHistory fetches historical OHLCV data with automatic fallback
func (c *CryptoCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	// Validate and normalize symbol
	if err := ValidateCryptoSymbol(symbol); err != nil {
		return nil, err
	}
	normalized := NormalizeSymbol(symbol, c.defaultQuote)

	// Try each provider in order
	var lastErr error
	for _, p := range c.providers {
		data, err := p.FetchHistory(normalized, start, end, interval)
		if err == nil && len(data) > 0 {
			// Update symbol in all records
			for i := range data {
				data[i].Symbol = normalized
			}
			return data, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed for %s: %w", normalized, lastErr)
	}
	return nil, fmt.Errorf("no data available for %s", normalized)
}

// SetDefaultQuote sets the default quote currency
func (c *CryptoCollector) SetDefaultQuote(quote string) {
	c.defaultQuote = quote
}

// SetProviders sets custom providers (for testing or configuration)
func (c *CryptoCollector) SetProviders(providers []Provider) {
	c.providers = providers
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/collector/crypto/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/collector/crypto/crypto.go internal/collector/crypto/crypto_test.go
git commit -m "feat(crypto): implement CryptoCollector with fallback support"
```

---

## Task 8: Register CryptoCollector in serve.go

**Files:**
- Modify: `cmd/atlas/serve.go`

**Step 1: Add import**

Add to imports:

```go
"github.com/newthinker/atlas/internal/collector/crypto"
```

**Step 2: Add collector registration**

After the eastmoney registration block (around line 99), add:

```go
	// Register Crypto collector for digital assets
	if collectorCfg, ok := cfg.Collectors["crypto"]; ok && collectorCfg.Enabled {
		cryptoCollector := crypto.New()
		// Configure from config if available
		if collectorCfg.Extra != nil {
			cryptoCollector.Init(collector.Config{
				Enabled: true,
				Extra:   collectorCfg.Extra,
			})
		}
		application.RegisterCollector(cryptoCollector)
		log.Info("crypto collector registered")
	}
```

**Step 3: Commit**

```bash
git add cmd/atlas/serve.go
git commit -m "feat(serve): register crypto collector"
```

---

## Task 9: Update Config with Crypto Collector Settings

**Files:**
- Modify: `configs/config.yaml`

**Step 1: Add crypto collector config**

Add after the lixinger config section:

```yaml
  # Cryptocurrency collector for digital assets
  crypto:
    enabled: true
    markets: ["CRYPTO"]
    interval: "1m"
    default_quote: "USDT"
    providers:
      - binance
      - coingecko
      - okx
```

**Step 2: Add example watchlist item**

Add to watchlist section:

```yaml
  - symbol: "BTC"
    name: "Bitcoin"
    market: "数字货币"
    type: "加密货币"
    strategies: ["ma_crossover"]
```

**Step 3: Commit**

```bash
git add configs/config.yaml
git commit -m "feat(config): add crypto collector configuration"
```

---

## Task 10: Update CollectorConfig to Support Extra Fields

**Files:**
- Modify: `internal/config/config.go:62-67`

**Step 1: Add Extra field to CollectorConfig**

Update the struct:

```go
type CollectorConfig struct {
	Enabled  bool           `mapstructure:"enabled"`
	Markets  []string       `mapstructure:"markets"`
	Interval string         `mapstructure:"interval"`
	APIKey   string         `mapstructure:"api_key"`
	Extra    map[string]any `mapstructure:",remain"`  // Add this line
}
```

**Step 2: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add Extra field to CollectorConfig for flexible options"
```

---

## Task 11: Run Full Test Suite and Integration Test

**Step 1: Run all unit tests**

```bash
go test ./... -v -short
```

Expected: All tests pass

**Step 2: Run integration tests (optional)**

```bash
go test ./internal/collector/crypto/binance/... -v -run Integration
```

**Step 3: Start server and test manually**

```bash
go run ./cmd/atlas serve -c configs/config.yaml
```

Test API:
```bash
curl http://localhost:8080/api/v1/quote/BTC
```

**Step 4: Final commit**

```bash
git add -A
git commit -m "test: verify crypto collector integration"
```

---

## Summary

Total tasks: 11

Files created:
- `internal/collector/crypto/symbol.go`
- `internal/collector/crypto/symbol_test.go`
- `internal/collector/crypto/provider.go`
- `internal/collector/crypto/crypto.go`
- `internal/collector/crypto/crypto_test.go`
- `internal/collector/crypto/binance/binance.go`
- `internal/collector/crypto/binance/binance_test.go`
- `internal/collector/crypto/coingecko/coingecko.go`
- `internal/collector/crypto/coingecko/coingecko_test.go`
- `internal/collector/crypto/okx/okx.go`
- `internal/collector/crypto/okx/okx_test.go`

Files modified:
- `internal/core/types.go`
- `internal/config/config.go`
- `cmd/atlas/serve.go`
- `configs/config.yaml`
