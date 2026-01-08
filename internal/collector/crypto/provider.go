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
