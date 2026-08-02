package baostock

import (
	"context"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// collector.Collector 实现:把 baostock 桥登记为 A 股行情三跳备源
// (spec §2「A股·行情」三跳三故障域,ADR#10)。Prism 编排不消费行情(ADR#6)。

func (c *Client) Name() string { return "baostock" }

func (c *Client) SupportedMarkets() []core.Market { return []core.Market{core.MarketCNA} }

// Init 无需鉴权:桥是本机侧车,匿名登录 baostock。
func (c *Client) Init(cfg collector.Config) error { return nil }

func (c *Client) Start(ctx context.Context) error { return nil }
func (c *Client) Stop() error                     { return nil }

// FetchHistory 转发到桥的日线接口。桥只有日线,其它粒度直接报错而非静默按日线返回。
func (c *Client) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if interval != "" && interval != "1d" {
		return nil, fmt.Errorf("baostock: interval %q not supported (daily only)", interval)
	}
	return c.FetchDaily(symbol, start, end)
}

// quoteWindowDays 是取最新报价的回看天数:够跨周末与连续休市。
const quoteWindowDays = 10

// FetchQuote 由最近的日线序列派生报价(桥无实时行情)。
func (c *Client) FetchQuote(symbol string) (*core.Quote, error) {
	end := time.Now()
	bars, err := c.FetchDaily(symbol, end.AddDate(0, 0, -quoteWindowDays), end)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("baostock: no quote data for %s", symbol)
	}
	last := bars[len(bars)-1]
	q := &core.Quote{Symbol: symbol, Market: core.MarketCNA,
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
