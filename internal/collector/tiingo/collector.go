package tiingo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// Collector 把 Client 适配为 collector.Collector，供 serve 注册为美股最后一个外部兜底。
type Collector struct {
	client *Client
	now    func() time.Time
}

func NewCollector(apiKey string) *Collector {
	return &Collector{client: New(apiKey), now: time.Now}
}

func (c *Collector) Name() string                    { return "tiingo" }
func (c *Collector) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS} }
func (c *Collector) Start(context.Context) error     { return nil }
func (c *Collector) Stop() error                     { return nil }

// Init 换 key 时沿用当前端点（NewCollector 即生产端点）。
func (c *Collector) Init(cfg collector.Config) error {
	if cfg.APIKey == "" {
		return errors.New("tiingo: api_key is required")
	}
	c.client = NewWithBaseURL(cfg.APIKey, c.client.baseURL)
	return nil
}

// FetchHistory 只支持日线；不在白名单的代码直接拒绝，不发请求、不占配额。
func (c *Collector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if interval != "1d" {
		return nil, fmt.Errorf("tiingo: unsupported interval %q (only 1d)", interval)
	}
	if !Supported(symbol) {
		return nil, fmt.Errorf("tiingo: unsupported symbol %s", symbol)
	}
	return c.client.FetchHistory(symbol, start, end)
}

// FetchQuote 用最近两根日线拼装行情（免费档无实时）。Time 取 K 线日期、Source 标
// tiingo_eod，使用方据此能看出这不是实时价。
func (c *Collector) FetchQuote(symbol string) (*core.Quote, error) {
	if !Supported(symbol) {
		return nil, fmt.Errorf("tiingo: unsupported symbol %s", symbol)
	}
	now := c.now()
	bars, err := c.client.FetchHistory(symbol, now.AddDate(0, 0, -10), now)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("tiingo: %s: no recent bars", symbol)
	}
	last := bars[len(bars)-1]
	q := &core.Quote{
		Symbol: symbol, Market: core.MarketUS,
		Price: last.Close, Open: last.Open, High: last.High, Low: last.Low, Volume: last.Volume,
		Time: last.Time, Source: "tiingo_eod",
	}
	if len(bars) >= 2 {
		prev := bars[len(bars)-2].Close
		q.PrevClose = prev
		q.Change = last.Close - prev
		if prev != 0 {
			q.ChangePercent = q.Change / prev * 100
		}
	}
	return q, nil
}
