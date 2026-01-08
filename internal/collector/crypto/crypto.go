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
// Provider order: OKX first (accessible in China), then CoinGecko, then Binance
func New() *CryptoCollector {
	return &CryptoCollector{
		providers: []Provider{
			okx.New(),
			coingecko.New(""),
			binance.New(),
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
