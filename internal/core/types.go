package core

import "time"

// Market represents a trading market
type Market string

const (
	MarketUS  Market = "US"
	MarketHK  Market = "HK"
	MarketCNA Market = "CN_A"
	MarketEU  Market = "EU"
)

// AssetType represents the type of financial asset
type AssetType string

const (
	AssetStock     AssetType = "stock"
	AssetIndex     AssetType = "index"
	AssetETF       AssetType = "etf"
	AssetFund      AssetType = "fund"
	AssetCommodity AssetType = "commodity"
)

// Quote represents a real-time price quote
type Quote struct {
	Symbol string
	Market Market
	Price  float64
	Volume int64
	Bid    float64
	Ask    float64
	Time   time.Time
	Source string
}

// IsValid checks if the quote has required fields
func (q Quote) IsValid() bool {
	return q.Symbol != "" && q.Price > 0
}

// OHLCV represents a candlestick/bar
type OHLCV struct {
	Symbol   string
	Interval string // "1m", "5m", "1d"
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   int64
	Time     time.Time
}

// Action represents a trading signal action
type Action string

const (
	ActionBuy        Action = "buy"
	ActionSell       Action = "sell"
	ActionHold       Action = "hold"
	ActionStrongBuy  Action = "strong_buy"
	ActionStrongSell Action = "strong_sell"
)

// Signal represents a trading signal from a strategy
type Signal struct {
	Symbol      string
	Action      Action
	Confidence  float64
	Price       float64 // Price at signal generation
	Reason      string
	Strategy    string
	Metadata    map[string]any
	GeneratedAt time.Time
}
