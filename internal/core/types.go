package core

import "time"

// Market represents a trading market
type Market string

const (
	MarketUS     Market = "US"
	MarketHK     Market = "HK"
	MarketCNA    Market = "CN_A"
	MarketEU     Market = "EU"
	MarketCrypto Market = "CRYPTO"
)

// AssetType represents the type of financial asset
type AssetType string

const (
	AssetStock     AssetType = "stock"
	AssetIndex     AssetType = "index"
	AssetETF       AssetType = "etf"
	AssetFund      AssetType = "fund"
	AssetCommodity AssetType = "commodity"
	AssetCrypto    AssetType = "crypto"
)

// Quote represents a real-time price quote
type Quote struct {
	Symbol        string
	Market        Market
	Price         float64
	Open          float64
	High          float64
	Low           float64
	PrevClose     float64
	Change        float64
	ChangePercent float64
	Volume        int64
	Bid           float64
	Ask           float64
	Time          time.Time
	Source        string
	FundInfo      *FundInfo `json:",omitempty"` // Fund-specific info (only for funds)
}

// FundInfo represents fund-specific information (for open-end funds)
type FundInfo struct {
	Name              string    // 基金名称
	Manager           string    // 基金经理
	ManagementCompany string    // 基金公司
	InceptionDate     time.Time // 成立日期
	FundSize          float64   // 基金规模(亿元)
	FundSizeDate      string    // 规模日期
	AnnualizedReturn  float64   // 成立以来年化收益率(%)
	MaxDrawdown       float64   // 最大回撤(%)
	LatestNAV         float64   // 最新净值
	NAVDate           string    // 净值日期
	FundType          string    // 基金类型
}

// IsValid checks if the quote has required fields
func (q Quote) IsValid() bool {
	return q.Symbol != "" && q.Price > 0
}

// OHLCV represents a candlestick/bar
//
// ⚠ 新增字段必须是**值类型**（string / 数值 / bool / time.Time）。
//
// 各 collector 在 policy.Fetch 返回后用 slices.Clone 把缓存里的切片复制给调用方，
// 而 slices.Clone 是**浅元素拷贝**——「浅元素拷贝等于深拷贝」当且仅当元素是
// flat value type。一旦这里加了 map / slice / 指针字段（例如
// `Adjustments []float64`），那些 Clone 会**静默退化成浅拷贝**：多个调用方共享
// 同一底层数据，一方改写会污染缓存与其他调用方，而**不会有任何测试变红**。
//
// 若确需加引用类型字段，必须同步把各 collector 侧的 slices.Clone 改成逐元素深拷贝
// （yahoo/twelvedata/lixinger 的 FetchHistory、以及任何缓存 []OHLCV 的新接入点）。
//
// 这段限定原本写在 internal/collector/cache.go 的 cloneOHLCV 上（「OHLCV is a flat
// value type, so a shallow element copy is a deep copy」）。该函数已随 OHLCV 装饰器
// 一并删除，故把约束移到**会被违反的这一侧**——改本文件的人看不到写在 collector 侧的注释。
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

// EPSPoint is one point of a trailing-twelve-month diluted EPS series.
// FilingDate, when set, is the public-availability date the value becomes
// effective (防前视); zero value falls back to Date (yahoo/qlibpit 路径口径:
// 以报告期日期近似生效日,历史分位存在轻微前视,接受为已知限制).
type EPSPoint struct {
	Date       time.Time
	EPS        float64
	FilingDate time.Time
}

// Fundamental represents fundamental data for a stock
type Fundamental struct {
	Symbol        string
	Market        Market
	Date          time.Time // Report date
	PE            float64   // Price to Earnings ratio
	PB            float64   // Price to Book ratio
	PS            float64   // Price to Sales ratio
	ROE           float64   // Return on Equity (percentage)
	ROA           float64   // Return on Assets (percentage)
	DividendYield float64   // Dividend yield (percentage)
	MarketCap     float64   // Market capitalization
	Revenue       float64   // Total revenue
	NetIncome     float64   // Net income
	EPS           float64   // Earnings per share
	// PEPercentile is the position of current PE in its historical series,
	// 0-100. Negative means unavailable. Source encodes how it was obtained:
	// "lixinger_cvpos", "reconstructed", or "method:fallback_reason".
	PEPercentile float64
	Source       string // Data source
}

// IsValid checks if fundamental data has required fields
func (f Fundamental) IsValid() bool {
	return f.Symbol != "" && !f.Date.IsZero()
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
	ID          string `json:"id"`
	Symbol      string
	Action      Action
	Confidence  float64
	Price       float64 // Price at signal generation
	Reason      string
	Strategy    string
	Metadata    map[string]any
	GeneratedAt time.Time
}
