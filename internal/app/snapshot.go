package app

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/valuation"
)

// FundamentalSource provides point-in-time valuation fields (PE/PB/dividend
// yield). Satisfied by *lixinger.Lixinger; declared narrow so app does not
// depend on the concrete collector package (mirrors ValuationSource).
type FundamentalSource interface {
	FetchFundamental(symbol string) (*core.Fundamental, error)
}

// SetFundamentalSource injects the valuation-fields source. Nil means the
// PE/PB/dividend-yield columns are unavailable.
func (a *App) SetFundamentalSource(fs FundamentalSource) {
	a.fundamentalSrc = fs
}

// SymbolMetrics is one watchlist symbol's metrics snapshot. Pointer fields are
// nil when the metric is unavailable; Gaps records why.
type SymbolMetrics struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	Market string `json:"market"`
	Type   string `json:"type"`

	Price     float64 `json:"price"`
	ChangePct float64 `json:"change_pct"`

	PE            *float64 `json:"pe"`
	PB            *float64 `json:"pb"`
	DividendYield *float64 `json:"dividend_yield"`

	PEPercentile    *float64 `json:"pe_percentile"`
	PricePercentile *float64 `json:"price_percentile"`

	Gaps []string `json:"gaps"`
}

// SnapshotMetrics assembles a read-only metrics snapshot for watchlist items.
// symbols nil means the full watchlist; otherwise it filters to that subset
// (watchlist order preserved). Read-only: no signals, no notifications.
//
// The PE and price percentiles are computed over a single history window whose
// length is the global valuation.lookback_years config (see snapshotHistoryStart),
// not the per-strategy lookback the live analysis loop uses.
func (a *App) SnapshotMetrics(ctx context.Context, symbols []string) []SymbolMetrics {
	items := a.snapshotItems(symbols)
	results := make([]SymbolMetrics, len(items))

	workers := 1
	if a.cfg != nil && a.cfg.Analysis.Workers > 1 {
		workers = a.cfg.Analysis.Workers
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	for i, item := range items {
		g.Go(func() error {
			results[i] = a.snapshotSymbolSafe(gctx, item)
			return nil
		})
	}
	_ = g.Wait() // workers never return errors; failures land in Gaps
	return results
}

// snapshotItems resolves the target items, preserving watchlist order.
func (a *App) snapshotItems(symbols []string) []WatchlistItem {
	all := a.GetWatchlistItems()
	if symbols == nil {
		return all
	}
	want := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		want[s] = true
	}
	var out []WatchlistItem
	for _, it := range all {
		if want[it.Symbol] {
			out = append(out, it)
		}
	}
	return out
}

// snapshotSymbolSafe isolates per-symbol panics (mirrors analyzeSymbolSafe).
func (a *App) snapshotSymbolSafe(ctx context.Context, item WatchlistItem) (m SymbolMetrics) {
	defer func() {
		if r := recover(); r != nil {
			m = SymbolMetrics{Symbol: item.Symbol, Name: item.Name, Market: item.Market, Type: item.Type,
				Gaps: []string{fmt.Sprintf("panic: %v", r)}}
		}
	}()
	return a.snapshotSymbol(ctx, item)
}

func (a *App) snapshotSymbol(_ context.Context, item WatchlistItem) SymbolMetrics {
	m := SymbolMetrics{Symbol: item.Symbol, Name: item.Name, Market: item.Market, Type: item.Type}

	cols := a.orderedCollectors(item.Symbol)
	if len(cols) == 0 {
		m.Gaps = append(m.Gaps, "no collector available")
		return m
	}

	// 行情:依序试(与分析循环同路由)。
	var quote *core.Quote
	var qErr error
	for _, c := range cols {
		quote, qErr = c.FetchQuote(item.Symbol)
		if qErr == nil && quote != nil {
			break
		}
	}
	if quote != nil {
		m.Price, m.ChangePct = quote.Price, quote.ChangePercent
	} else {
		m.Gaps = append(m.Gaps, fmt.Sprintf("quote unavailable: %v", qErr))
	}

	// 历史 K 线:窗口由 valuation lookback 决定(0 = 全历史,与策略语义一致)。
	end := time.Now()
	start := clampToEpochFloor(a.snapshotHistoryStart(end))
	var ohlcv []core.OHLCV
	var hErr error
	for _, c := range cols {
		ohlcv, hErr = c.FetchHistory(item.Symbol, start, end, "1d")
		if hErr == nil && len(ohlcv) > 0 {
			break
		}
	}
	if len(ohlcv) == 0 {
		m.Gaps = append(m.Gaps, fmt.Sprintf("history unavailable: %v", hErr))
	}

	// PE 百分位:复用分析循环的组装链(资产类型门控在 buildFundamental 内部)。
	if f := a.buildFundamental(item.Symbol, item.Type, ohlcv); f != nil {
		if f.PEPercentile >= 0 {
			v := f.PEPercentile
			m.PEPercentile = &v
		} else {
			m.Gaps = append(m.Gaps, "pe percentile unavailable")
		}
		// 估值三项:仅在有 FundamentalSource 时可得(当前 = A 股 lixinger)。
		// 取值成功时如实呈现——dyr=0(不分红)与 pe_ttm<0(亏损)都是 lixinger 的
		// 已知事实,不能掩盖成 nil("数据不可用"),否则与真正缺失混淆。仅 fetch
		// 出错时才置 nil 并记 gap。
		if a.fundamentalSrc != nil {
			if fd, err := a.fundamentalSrc.FetchFundamental(item.Symbol); err == nil && fd != nil {
				m.PE = &fd.PE
				m.PB = &fd.PB
				m.DividendYield = &fd.DividendYield
			} else {
				m.Gaps = append(m.Gaps, fmt.Sprintf("fundamental unavailable: %v", err))
			}
		} else {
			m.Gaps = append(m.Gaps, "fundamental source not configured for market")
		}
	}
	// buildFundamental 返回 nil = 资产类型无估值路径(crypto/商品/基金),
	// 属预期缺席,不记 gap。

	// 价格百分位:现价(缺行情时退回最后收盘)在窗口收盘序列中的分位。
	if len(ohlcv) > 0 {
		closes := make([]float64, 0, len(ohlcv))
		for _, b := range ohlcv {
			if b.Close > 0 {
				closes = append(closes, b.Close)
			}
		}
		cur := m.Price
		if cur <= 0 && len(closes) > 0 {
			cur = closes[len(closes)-1]
		}
		if len(closes) > 0 && cur > 0 {
			v := valuation.PercentileRank(closes, cur)
			m.PricePercentile = &v
		}
	}
	return m
}

// snapshotHistoryStart returns the history-window start for both the PE and
// price percentiles. The window baseline is the global valuation.lookback_years
// config (App.valuationLookback): >0 = that many years back; 0 = since inception
// (AddDate(-100y), epoch-floor clamped by the caller). This is a single global
// window, unlike the analysis loop where each strategy picks its own lookback.
func (a *App) snapshotHistoryStart(end time.Time) time.Time {
	if a.valuationLookback <= 0 {
		return end.AddDate(-100, 0, 0)
	}
	return end.AddDate(-a.valuationLookback, 0, 0)
}
