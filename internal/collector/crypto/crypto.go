package crypto

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto/binance"
	"github.com/newthinker/atlas/internal/collector/crypto/coingecko"
	"github.com/newthinker/atlas/internal/collector/crypto/okx"
	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/core"
)

// topicHistory 是 FetchHistory 的策略主题，经 `<域>.*` 通配命中内置 crypto.* 条目
// （仅 TTL 缓存，无节流无配额）。
//
// **域段（crypto）写错会静默失效**：Lookup 落到「未登记 → 零策略」时 Gate 直通，
// 缓存彻底没了而不报任何错。接口段（history）在通配登记下写成什么都能命中。
const topicHistory = "crypto.history"

// CryptoCollector implements collector.Collector for cryptocurrency markets
type CryptoCollector struct {
	providers    []Provider
	defaultQuote string
	config       collector.Config
	// gate 在构造时快照 policy.Default()，不是每次调用现取（契约陷阱 4）：
	// 要替换默认闸门的调用方必须在构造之前 SetDefault。
	gate *policy.Gate
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
		gate:         policy.Default(),
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
		gate:         policy.Default(),
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

// FetchHistory fetches historical OHLCV data with automatic fallback.
//
// 闸门包在**整个** fallback 链外层——被删除的 maybeCache 装饰的正是 Collector 接口，
// 缓存的是这个方法的最终结果。包进单个 provider 会让 fallback 本身丢缓存，
// 也会把「哪一家返回的」错误地写进缓存键：providers 是 fallback 语义（依次尝试、
// 首个成功即返），同 symbol 无论由哪家返回都是同一份数据，共用一条缓存槽是正确的。
//
// 返回值一律 slices.Clone 后交出：缓存命中时多个调用方拿到同一个切片，
// 直接交出会让一方改写污染缓存与其他调用方。core.OHLCV 是 flat value type
// （见 core/types.go 定义处的约束），故浅元素拷贝等于深拷贝。
func (c *CryptoCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	// Validate and normalize symbol
	if err := ValidateCryptoSymbol(symbol); err != nil {
		return nil, err
	}
	normalized := NormalizeSymbol(symbol, c.defaultQuote)

	data, err := policy.Fetch(c.gate, topicHistory,
		historyKey(normalized, start, end, interval),
		func() ([]core.OHLCV, error) {
			return c.fetchHistoryFromProviders(normalized, start, end, interval)
		})
	if err != nil {
		// policy 包的错误不得出现在调用方可见的错误链上：用 %v 而非 %w 收口，
		// 保留可读文本但断开 errors.Is 链。crypto 今天没有 Quota/Timeout，
		// 但 config 能给任何主题加配额，泄漏点随接入家数增长。
		if errors.Is(err, policy.ErrQuotaExceeded) || errors.Is(err, policy.ErrTimeout) {
			return nil, fmt.Errorf("crypto: %s 暂不可用（闸门限制，稍后自动恢复）: %v", normalized, err)
		}
		return nil, err
	}
	return slices.Clone(data), nil
}

// historyKey 是 FetchHistory 的缓存键。它要同时满足两个方向：
//
//   - **区分度**：覆盖全部影响结果的参数（symbol / 区间 / interval）。丢掉任一维度
//     会让不同查询落进同一槽、静默返回错标的或错区间的数据。
//   - **聚合度**：时间截断到**分钟**。上层 app.go 传的 end 是 `time.Now()`，
//     若按原始精度入键，每次调用键都不同 ⇒ **缓存命中率恒为零**，而单测里用固定
//     start/end 调两次是全绿的 —— 只有生产路径才失效。
//
// 分钟这个粒度不是随便取的：沿用被删除的 CachedCollector.cacheKey 的口径
// （它用 `Truncate(time.Minute).UTC().Format(time.RFC3339)`），与 yahoo、eastmoney
// 现行实现一致。粒度改粗（小时/天）会让相邻分钟的不同查询串槽，故两个方向都有测试钉住。
func historyKey(symbol string, start, end time.Time, interval string) string {
	return fmt.Sprintf("%s|%d|%d|%s", symbol,
		start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix(), interval)
}

// fetchHistoryFromProviders 是被缓存的取数函数：依次尝试各 provider，首个成功即返。
// 校验（空结果视为失败）留在这里面——挪到闸门外会让失败结果被当成功写进缓存。
func (c *CryptoCollector) fetchHistoryFromProviders(normalized string, start, end time.Time, interval string) ([]core.OHLCV, error) {
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
