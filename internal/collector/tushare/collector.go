package tushare

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// collector.Collector 实现:把 tushare 登记为 A 股行情二跳备源(spec §2「A股·行情」,
// ADR#10)。Prism 编排不消费行情(ADR#6),这一层只服务 collector registry。

func (c *Client) Name() string { return "tushare" }

func (c *Client) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketCNA, core.MarketHK}
}

// Init 接受配置注入的 token。缺 token 直接报错:登记一个必然拿 40203 的采集器,
// 只会让降级链多一跳无效等待。
func (c *Client) Init(cfg collector.Config) error {
	if cfg.APIKey != "" {
		c.token = cfg.APIKey
	}
	if c.token == "" {
		return fmt.Errorf("tushare: api_key is required")
	}
	return nil
}

func (c *Client) Start(ctx context.Context) error { return nil }
func (c *Client) Stop() error                     { return nil }

// FetchHistory 取日线收盘序列。symbol 决定走哪个 api —— 三者不通用且走错不会报错,
// 只会返回空 items(实测),故这里按形态硬路由:
// 港股 → hk_daily;A 股指数 → index_daily;其余 A 股 → daily。
func (c *Client) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if interval != "" && interval != "1d" {
		return nil, fmt.Errorf("tushare: interval %q not supported (daily only)", interval)
	}
	var (
		pts []PricePoint
		err error
	)
	switch {
	case strings.HasSuffix(strings.ToUpper(symbol), ".HK"):
		pts, err = c.FetchHKDaily(symbol, start, end)
	case collector.IsAShareIndex(symbol):
		pts, err = c.FetchIndexDaily(symbol, start, end)
	default:
		pts, err = c.FetchDaily(symbol, start, end)
	}
	if err != nil {
		return nil, err
	}
	out := make([]core.OHLCV, len(pts))
	for i, p := range pts {
		out[i] = core.OHLCV{Symbol: symbol, Interval: "1d", Close: p.Close, Time: p.Date}
	}
	return out, nil
}

// quoteWindowDays 是取最新报价的回看天数:够跨周末与连续休市。
const quoteWindowDays = 10

// FetchQuote 由最近的日线序列派生报价(tushare 无实时行情接口)。
func (c *Client) FetchQuote(symbol string) (*core.Quote, error) {
	end := time.Now()
	bars, err := c.FetchHistory(symbol, end.AddDate(0, 0, -quoteWindowDays), end, "1d")
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("tushare: no quote data for %s", symbol)
	}
	last := bars[len(bars)-1]
	q := &core.Quote{Symbol: symbol, Market: collector.MarketForSymbol(symbol),
		Price: last.Close, Time: last.Time, Source: c.Name()}
	if len(bars) > 1 {
		prev := bars[len(bars)-2].Close
		q.PrevClose = prev
		q.Change = q.Price - prev
		if prev != 0 {
			q.ChangePercent = q.Change / prev * 100
		}
	}
	return q, nil
}
