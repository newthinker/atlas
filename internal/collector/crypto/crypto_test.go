package crypto

import (
	"fmt"
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

func TestCryptoCollector_FetchHistory_Fallback(t *testing.T) {
	failProvider := &mockProvider{
		name:       "fail",
		historyErr: fmt.Errorf("provider error"),
	}
	successProvider := &mockProvider{
		name: "success",
		history: []core.OHLCV{
			{Symbol: "BTCUSDT", Close: 50000},
			{Symbol: "BTCUSDT", Close: 51000},
		},
	}

	c := New()
	c.providers = []Provider{failProvider, successProvider}

	start := time.Now().AddDate(0, 0, -7)
	end := time.Now()
	data, err := c.FetchHistory("BTC", start, end, "1d")
	if err != nil {
		t.Fatalf("expected success after fallback, got error: %v", err)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 records, got %d", len(data))
	}
}

func TestCryptoCollector_Init(t *testing.T) {
	c := New()

	cfg := collector.Config{
		Enabled: true,
		Extra: map[string]any{
			"default_quote": "BUSD",
		},
	}

	err := c.Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if c.defaultQuote != "BUSD" {
		t.Errorf("expected default_quote BUSD, got %s", c.defaultQuote)
	}
}
