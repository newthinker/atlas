// internal/context/types.go
package context

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// MarketRegime represents the overall market condition.
type MarketRegime string

const (
	RegimeBull     MarketRegime = "bull"
	RegimeBear     MarketRegime = "bear"
	RegimeSideways MarketRegime = "sideways"
)

// Trend represents a directional trend.
type Trend string

const (
	TrendUp   Trend = "up"
	TrendDown Trend = "down"
	TrendFlat Trend = "flat"
)

// MarketContext represents the current market conditions.
type MarketContext struct {
	Market       core.Market           `json:"market"`
	Regime       MarketRegime          `json:"regime"`
	Volatility   float64               `json:"volatility"`
	SectorTrends map[string]Trend      `json:"sector_trends,omitempty"`
	InterestRate float64               `json:"interest_rate,omitempty"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// NewsItem represents a news article or announcement.
type NewsItem struct {
	Title       string    `json:"title"`
	Summary     string    `json:"summary,omitempty"`
	Source      string    `json:"source"`
	URL         string    `json:"url,omitempty"`
	Symbols     []string  `json:"symbols,omitempty"`
	Sentiment   float64   `json:"sentiment"` // -1 to 1
	PublishedAt time.Time `json:"published_at"`
}

// StrategyStats represents the historical performance of a strategy.
type StrategyStats struct {
	Strategy     string  `json:"strategy"`
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
	AvgReturn    float64 `json:"avg_return"`
	SharpeRatio  float64 `json:"sharpe_ratio,omitempty"`
	MaxDrawdown  float64 `json:"max_drawdown,omitempty"`
}

// MarketContextProvider provides market context information.
type MarketContextProvider interface {
	GetContext(ctx context.Context, market core.Market) (*MarketContext, error)
}

// NewsProvider provides news for symbols and markets.
type NewsProvider interface {
	GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error)
	GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error)
}

// TrackRecordProvider provides strategy performance history.
type TrackRecordProvider interface {
	GetStats(ctx context.Context, strategy string) (*StrategyStats, error)
	GetAllStats(ctx context.Context) (map[string]*StrategyStats, error)
}
