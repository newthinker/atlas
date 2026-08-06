package baostock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/policy"
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

// topicDaily 是 FetchHistory 的策略主题。内置表以 `baostock.*` 通配登记
// （仅 TTL，无节流无配额）——被删除的 maybeCache 装饰器原本就为本包提供 OHLCV
// 缓存，这里是把那份能力接回来。
const topicDaily = "baostock.daily"

// FetchHistory 转发到桥的日线接口。桥只有日线,其它粒度直接报错而非静默按日线返回。
//
// 经 Gate 走 TTL 缓存。缓存键**不含 interval**：本方法只接受 "" 与 "1d"，二者语义
// 相同、取到同一份日线，其余值在进 Gate 之前就被拒 ⇒ interval 不影响结果，放进键
// 只会让两个等价调用各占一个槽（见 TestIntervalDoesNotSplitSlot）。
//
// FetchQuote 不经 Gate：它由最近日线派生，而被删除的 CachedCollector 本就只缓存
// FetchHistory（原注释：FetchQuote is intentionally not cached (real-time freshness)）。
func (c *Client) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if interval != "" && interval != "1d" {
		return nil, fmt.Errorf("baostock: interval %q not supported (daily only)", interval)
	}
	key := fmt.Sprintf("%s|%d|%d",
		symbol, start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix())
	data, err := policy.Fetch(c.gate, topicDaily, key, func() ([]core.OHLCV, error) {
		return c.FetchDaily(symbol, start, end)
	})
	if err != nil {
		return nil, mapPolicyError(symbol, err)
	}
	// Gate 不复制返回值：缓存命中时多个调用方共享同一底层数组，故在此 clone。
	// core.OHLCV 今天全是值类型，浅元素拷贝即深拷贝——**该前提由 core/types.go
	// 保证，不是这里**（那边的注释已写明新增字段必须是值类型）。
	return slices.Clone(data), nil
}

// mapPolicyError 把 policy 包的内部错误挡在本包边界内（policy 错误不外泄）。
//
// 本包**没有哨兵错误**（它是 A 股行情降级链的第三跳，上层失败即整跳跳过、不区分
// 错误类型），故按「包一层本包前缀」处理，并**断链**——用 %w 会让 policy.ErrXxx
// 仍留在调用方可见的 errors.Is 链上，那不算挡住。
//
// ⚠ 配额与超时都是**临时性**错误（窗口过后 / 下次调用即可自愈），措辞不得暗示
// 永久失败：那会让运维照着去改配置，而问题只是要等窗口。
// 非 policy 错误原样返回，保留 FetchDaily 的原始错误链。
func mapPolicyError(symbol string, err error) error {
	switch {
	case errors.Is(err, policy.ErrQuotaExceeded):
		return fmt.Errorf("baostock: daily %s: 本地配额预判未通过，本次未发出请求（临时性，窗口过后自愈）", symbol)
	case errors.Is(err, policy.ErrTimeout):
		return fmt.Errorf("baostock: daily %s: 请求超时（临时性，可重试）", symbol)
	}
	return err
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
