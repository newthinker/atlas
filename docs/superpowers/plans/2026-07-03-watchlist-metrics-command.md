# atlas watchlist 指标命令 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `atlas watchlist` 命令,离线输出 watchlist 全部标的的行情/估值/百分位指标(表格或 JSON)。

**Architecture:** App 导出只读方法 `SnapshotMetrics`(复用 `orderedCollectors`/`buildFundamental`,与分析循环口径同源);serve 的 collector 装配段提取为 `buildCollectors` 供 serve 与新命令共用;CJK 表格对齐逻辑从 telegram 提取到共享包 `internal/text`。

**Tech Stack:** Go(cobra CLI、errgroup 并发、encoding/json),零新增第三方依赖。

**Spec:** `docs/superpowers/specs/2026-07-03-watchlist-metrics-command-design.md`

## Global Constraints

- 编辑任何已有函数/方法前先 `gitnexus_impact({target, direction:"upstream"})`;每个 task 提交前 `gitnexus_detect_changes()` 核对改动范围(项目 CLAUDE.md 强制)。
- 每个 task 提交代码前运行 code-simplifier sub-agent 检查本 task 修改的代码文件(用户全局规范;Task tool,subagent_type `code-simplifier:code-simplifier`)。
- `go test ./...` 必须离线全绿(不打真实网络)。
- 不新增任何第三方依赖;不改 `modernc.org/sqlite` 等现有依赖版本。
- 提交信息:`<type>(watchlist-cmd): <描述>`。
- 已核实的关键事实(实现时不要再"纠正"):`buildFundamental(symbol, appType string, ohlcv []core.OHLCV) *core.Fundamental` 只组装 `PEPercentile`(-1=不可用),**不填** PE/PB/DividendYield;估值三项唯一现成实现是 `lixinger.FetchFundamental`(仅 A 股),经本计划新增的 `FundamentalSource` 接口注入。

---

### Task 1: 提取 `internal/text` 共享宽度包

**Files:**
- Create: `internal/text/width.go`
- Create: `internal/text/width_test.go`
- Modify: `internal/notifier/telegram/telegram.go:198,210`(`displayWidth`→`text.DisplayWidth`、`padRight`→`text.PadRight`,并在 import 块加 `"github.com/newthinker/atlas/internal/text"`)
- Delete: `internal/notifier/telegram/width.go`、`internal/notifier/telegram/width_test.go`

**Interfaces:**
- Consumes: 无(纯函数迁移)
- Produces: `text.DisplayWidth(s string) int`、`text.PadRight(s string, width int) string`(Task 4 渲染表格使用;telegram 包同步改用)

- [ ] **Step 1: 写新包测试(从 telegram 迁移,改为导出名)**

`internal/text/width_test.go`:

```go
package text

import "testing"

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{
		"":        0,
		"AAPL":    4,
		"贵州茅台":    8,
		"茅台A":     5,
		"0700.HK": 7,
	}
	for s, want := range cases {
		if got := DisplayWidth(s); got != want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestPadRight(t *testing.T) {
	if got := PadRight("AAPL", 6); got != "AAPL  " {
		t.Errorf("PadRight(AAPL,6) = %q", got)
	}
	if got := PadRight("茅台", 6); got != "茅台  " {
		t.Errorf("PadRight(茅台,6) = %q", got)
	}
	if got := PadRight("AAPL", 3); got != "AAPL" {
		t.Errorf("PadRight overflow = %q", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/text/`
Expected: FAIL(package 不存在 / undefined: DisplayWidth)

- [ ] **Step 3: 创建 `internal/text/width.go`**

把 `internal/notifier/telegram/width.go` 全文迁入,仅三处改动:包名 `package text`;`displayWidth`→`DisplayWidth`、`padRight`→`PadRight`(`isWide` 保持不导出);注释中 "Telegram monospace code block" 措辞泛化为 "monospace table"。函数体逐字保留(East-Asian 区间表不动)。

- [ ] **Step 4: 运行新包测试**

Run: `go test ./internal/text/`
Expected: PASS

- [ ] **Step 5: telegram 改为引用共享包并删除旧文件**

`internal/notifier/telegram/telegram.go` import 块加 `"github.com/newthinker/atlas/internal/text"`;`:198` `displayWidth(c)`→`text.DisplayWidth(c)`;`:210` `padRight(c, widths[i])`→`text.PadRight(c, widths[i])`。然后:

```bash
rm internal/notifier/telegram/width.go internal/notifier/telegram/width_test.go
```

- [ ] **Step 6: 全量验证**

Run: `go build ./... && go test ./internal/text/ ./internal/notifier/telegram/`
Expected: 编译干净,两包 PASS(telegram 既有表格测试防回归)

- [ ] **Step 7: Commit**

```bash
git add internal/text/ internal/notifier/telegram/
git commit -m "refactor(watchlist-cmd): extract CJK width helpers to internal/text"
```

---

### Task 2: `App.SnapshotMetrics` 核心组装

**Files:**
- Create: `internal/app/snapshot.go`
- Create: `internal/app/snapshot_test.go`
- Modify: `internal/app/app.go`(App 结构体加一个字段,见 Step 3)

**Interfaces:**
- Consumes(已存在,勿改):`a.orderedCollectors(symbol) []collector.Collector`、`a.buildFundamental(symbol, appType string, ohlcv []core.OHLCV) *core.Fundamental`、`a.GetWatchlistItems() []WatchlistItem`、`clampToEpochFloor(t time.Time) time.Time`、`a.valuationLookback int`(app.New 默认 5,0=全历史)、`valuation.PercentileRank(series []float64, current float64) float64`(返回 0-100)、`a.cfg.Analysis.Workers`
- Produces(Task 3/4 依赖,签名必须逐字一致):

```go
type FundamentalSource interface {
	FetchFundamental(symbol string) (*core.Fundamental, error)
}
func (a *App) SetFundamentalSource(fs FundamentalSource)

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
func (a *App) SnapshotMetrics(ctx context.Context, symbols []string) []SymbolMetrics
```

- [ ] **Step 1: 写失败测试**

`internal/app/snapshot_test.go`(fake 采集器自带,不依赖 app_test.go 的桩):

```go
package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"go.uber.org/zap"
)

// snapFake implements collector.Collector for snapshot tests.
type snapFake struct {
	quote      map[string]*core.Quote
	history    map[string][]core.OHLCV
	panicOn    string
	failQuotes bool
}

func (f *snapFake) Name() string                    { return "snapfake" }
func (f *snapFake) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS, core.MarketHK, core.MarketCNA, core.MarketCrypto} }
func (f *snapFake) Init(collectorConfig) error      { return nil } // 如编译报错,以 collector.Config 实际类型为准
func (f *snapFake) Start(ctx context.Context) error { return nil }
func (f *snapFake) Stop() error                     { return nil }
func (f *snapFake) FetchQuote(symbol string) (*core.Quote, error) {
	if symbol == f.panicOn {
		panic("boom")
	}
	if f.failQuotes {
		return nil, fmt.Errorf("quote down")
	}
	if q, ok := f.quote[symbol]; ok {
		return q, nil
	}
	return nil, fmt.Errorf("no quote for %s", symbol)
}
func (f *snapFake) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if h, ok := f.history[symbol]; ok {
		return h, nil
	}
	return nil, fmt.Errorf("no history for %s", symbol)
}

type snapValuation struct{ pct float64 }

func (v snapValuation) FetchValuationPercentile(symbol string, lookbackYears int) (float64, error) {
	return v.pct, nil
}

type snapFundamental struct{ f *core.Fundamental }

func (s snapFundamental) FetchFundamental(symbol string) (*core.Fundamental, error) {
	if s.f == nil {
		return nil, fmt.Errorf("no fundamental")
	}
	return s.f, nil
}

func bars(closes ...float64) []core.OHLCV {
	out := make([]core.OHLCV, len(closes))
	t := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for i, c := range closes {
		out[i] = core.OHLCV{Time: t.AddDate(0, 0, i), Open: c, High: c, Low: c, Close: c, Volume: 100}
	}
	return out
}

func newSnapshotApp(t *testing.T, fake *snapFake, items ...WatchlistItem) *App {
	t.Helper()
	a := New(config.Defaults(), zap.NewNop())
	a.RegisterCollector(fake)
	for _, it := range items {
		a.AddToWatchlistWithDetails(it.Symbol, it.Name, it.Market, it.Type, it.Strategies)
	}
	return a
}

func TestSnapshotMetrics_FullPathAShare(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"600519.SH": {Price: 1400, ChangePercent: -0.5}},
		history: map[string][]core.OHLCV{"600519.SH": bars(100, 110, 120, 130, 140)},
	}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "600519.SH", Name: "贵州茅台", Market: "A股", Type: "股票"})
	a.SetValuationSources(snapValuation{pct: 12.3}, nil)
	a.SetFundamentalSource(snapFundamental{f: &core.Fundamental{PE: 19.5, PB: 5.96, DividendYield: 4.03}})

	ms := a.SnapshotMetrics(context.Background(), nil)
	if len(ms) != 1 {
		t.Fatalf("want 1 item, got %d", len(ms))
	}
	m := ms[0]
	if m.Price != 1400 || m.ChangePct != -0.5 {
		t.Errorf("quote fields = %v/%v", m.Price, m.ChangePct)
	}
	if m.PE == nil || *m.PE != 19.5 || m.PB == nil || *m.PB != 5.96 || m.DividendYield == nil || *m.DividendYield != 4.03 {
		t.Errorf("valuation fields = %+v", m)
	}
	if m.PEPercentile == nil || *m.PEPercentile != 12.3 {
		t.Errorf("pe percentile = %v", m.PEPercentile)
	}
	// 现价 1400 高于窗口内全部收盘 → 分位 100
	if m.PricePercentile == nil || *m.PricePercentile != 100 {
		t.Errorf("price percentile = %v", m.PricePercentile)
	}
	if len(m.Gaps) != 0 {
		t.Errorf("unexpected gaps: %v", m.Gaps)
	}
}

func TestSnapshotMetrics_MissingValuationDegrades(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"AAPL": {Price: 210, ChangePercent: 1.2}},
		history: map[string][]core.OHLCV{"AAPL": bars(200, 205, 215)},
	}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "AAPL", Name: "Apple", Market: "美股", Type: "股票"})
	// 不注入任何估值源

	m := a.SnapshotMetrics(context.Background(), nil)[0]
	if m.PE != nil || m.PB != nil || m.DividendYield != nil || m.PEPercentile != nil {
		t.Errorf("valuation should be nil: %+v", m)
	}
	if m.PricePercentile == nil {
		t.Error("price percentile should still compute")
	}
	if len(m.Gaps) == 0 {
		t.Error("expected gaps recorded")
	}
}

func TestSnapshotMetrics_CryptoNoValuationGap(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"BTC-USD": {Price: 65000, ChangePercent: 2.0}},
		history: map[string][]core.OHLCV{"BTC-USD": bars(60000, 62000, 64000)},
	}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "BTC-USD", Name: "Bitcoin", Market: "数字货币", Type: "加密货币"})

	m := a.SnapshotMetrics(context.Background(), nil)[0]
	if m.PE != nil || m.PEPercentile != nil {
		t.Errorf("crypto valuation should be nil: %+v", m)
	}
	for _, g := range m.Gaps {
		if strings.Contains(g, "valuation") || strings.Contains(g, "fundamental") {
			t.Errorf("crypto should not report valuation gap, got %q", g)
		}
	}
	if m.PricePercentile == nil {
		t.Error("price percentile should compute for crypto")
	}
}

func TestSnapshotMetrics_SymbolsFilter(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"AAPL": {Price: 210}, "MSFT": {Price: 400}},
		history: map[string][]core.OHLCV{"AAPL": bars(200), "MSFT": bars(390)},
	}
	a := newSnapshotApp(t, fake,
		WatchlistItem{Symbol: "AAPL", Type: "股票"},
		WatchlistItem{Symbol: "MSFT", Type: "股票"},
	)
	ms := a.SnapshotMetrics(context.Background(), []string{"MSFT"})
	if len(ms) != 1 || ms[0].Symbol != "MSFT" {
		t.Fatalf("filter failed: %+v", ms)
	}
}

func TestSnapshotMetrics_PanicIsolated(t *testing.T) {
	fake := &snapFake{
		panicOn: "BAD",
		quote:   map[string]*core.Quote{"GOOD": {Price: 10}},
		history: map[string][]core.OHLCV{"GOOD": bars(9, 10)},
	}
	a := newSnapshotApp(t, fake,
		WatchlistItem{Symbol: "BAD", Type: "股票"},
		WatchlistItem{Symbol: "GOOD", Type: "股票"},
	)
	ms := a.SnapshotMetrics(context.Background(), nil)
	if len(ms) != 2 {
		t.Fatalf("want 2 items, got %d", len(ms))
	}
	var bad, good SymbolMetrics
	for _, m := range ms {
		if m.Symbol == "BAD" {
			bad = m
		} else {
			good = m
		}
	}
	if len(bad.Gaps) == 0 {
		t.Error("panicked symbol should record a gap")
	}
	if good.Price != 10 {
		t.Errorf("healthy symbol should be unaffected: %+v", good)
	}
}

func TestSnapshotMetrics_AllCollectorsFail(t *testing.T) {
	fake := &snapFake{failQuotes: true}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "AAPL", Type: "股票"})
	m := a.SnapshotMetrics(context.Background(), nil)[0]
	if m.Price != 0 || m.PricePercentile != nil || len(m.Gaps) == 0 {
		t.Errorf("expected empty metrics with gaps: %+v", m)
	}
}
```

注:`snapFake.Init` 参数类型以 `internal/collector/interface.go` 的 `Init(cfg Config)` 实际签名为准(`collector.Config`),编写时对照修正。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/app/ -run TestSnapshotMetrics -v`
Expected: FAIL(undefined: SymbolMetrics / SnapshotMetrics / SetFundamentalSource)

- [ ] **Step 3: 实现 `internal/app/snapshot.go` + App 字段**

先在 `internal/app/app.go` 的 App 结构体(`valuationSrc`/`epsSrc` 字段旁)加一行:

```go
	fundamentalSrc FundamentalSource // 估值三项(PE/PB/股息率)来源,可为 nil
```

然后创建 `internal/app/snapshot.go`:

```go
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
		i, item := i, item
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
		if a.fundamentalSrc != nil {
			if fd, err := a.fundamentalSrc.FetchFundamental(item.Symbol); err == nil && fd != nil {
				m.PE = positivePtr(fd.PE)
				m.PB = positivePtr(fd.PB)
				m.DividendYield = positivePtr(fd.DividendYield)
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

// snapshotHistoryStart mirrors the strategy-side lookback semantics:
// valuationLookback 0 = since inception (epoch-floor clamped by caller).
func (a *App) snapshotHistoryStart(end time.Time) time.Time {
	if a.valuationLookback <= 0 {
		return end.AddDate(-100, 0, 0)
	}
	return end.AddDate(-a.valuationLookback, 0, 0)
}

func positivePtr(v float64) *float64 {
	if v > 0 {
		return &v
	}
	return nil
}
```

注意:美股测试用例 `TestSnapshotMetrics_MissingValuationDegrades` 走 `buildFundamental` 的美/港个股分支——无 epsSrc、无 valuationSrc 时返回 `PEPercentile=-1` 的非 nil Fundamental,随后记 gap,符合测试预期。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/app/ -run TestSnapshotMetrics -v`
Expected: 6 个用例全 PASS

- [ ] **Step 5: 全包回归**

Run: `go test ./internal/app/`
Expected: PASS(既有分析循环测试不受影响)

- [ ] **Step 6: Commit**

```bash
git add internal/app/snapshot.go internal/app/snapshot_test.go internal/app/app.go
git commit -m "feat(watchlist-cmd): add App.SnapshotMetrics read-only metrics assembly"
```

---

### Task 3: 提取 `buildCollectors`(serve 装配重构)

**Files:**
- Create: `cmd/atlas/collectors.go`
- Create: `cmd/atlas/collectors_test.go`
- Modify: `cmd/atlas/serve.go:99-170`(整段替换为一次调用)

**Interfaces:**
- Consumes: Task 2 的 `application.SetFundamentalSource`;既有 `wireQlibWarehouse`、`maybeCache`、`valuationSourceOrNil`、`epsSourceOrNil`(均已在 cmd/atlas 包内)
- Produces: `buildCollectors(cfg *config.Config, application *app.App, log *zap.Logger) (cleanup func(), err error)`(Task 4 使用;err 当前恒为 nil,签名按 spec 预留)

- [ ] **Step 1: 写失败测试**

`cmd/atlas/collectors_test.go`:

```go
package main

import (
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// buildCollectors with an empty config must succeed, register nothing that
// requires network, and return a nil-safe cleanup.
func TestBuildCollectors_EmptyConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collectors = nil // 无任何采集器配置
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	cleanup() // 必须 nil-safe,不 panic
}

// With defaults (yahoo/eastmoney enabled), collectors register and cleanup is safe.
func TestBuildCollectors_Defaults(t *testing.T) {
	cfg := config.Defaults()
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	defer cleanup()
	if len(application.GetCollectors()) == 0 {
		t.Error("expected collectors registered from default config")
	}
}
```

注:若 `config.Defaults()` 默认未启用任何采集器,`TestBuildCollectors_Defaults` 改为手工启用 yahoo(`cfg.Collectors["yahoo"] = config.CollectorConfig{Enabled: true}`)后断言 `len >= 1`。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run TestBuildCollectors -v`
Expected: FAIL(undefined: buildCollectors)

- [ ] **Step 3: 创建 `cmd/atlas/collectors.go`**

将 `cmd/atlas/serve.go:99-170` 的整段(cache 设置、yahoo/lixinger/eastmoney/crypto 注册、`wireQlibWarehouse`、估值/EPS 源注入、`SetValuationLookback`)**原样搬入**下述函数体(代码逐字迁移,不改行为),并追加两处新内容(注释标出):

```go
package main

import (
	"database/sql"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/qlibpit"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// buildCollectors wires every configured collector plus the valuation/EPS
// sources into application — the single shared assembly for `atlas serve`
// and offline commands (`atlas watchlist`). cleanup closes the qlib
// warehouse handle (nil-safe); err is reserved by the spec and currently
// always nil (collector wiring degrades instead of failing).
func buildCollectors(cfg *config.Config, application *app.App, log *zap.Logger) (cleanup func(), err error) {
	// —— 以下为 serve.go:99-170 原文迁移(此处省略,执行时逐字剪切)——
	//   cacheEnabled/cacheTTL 读取与日志
	//   yahoo 注册(yahooCollector 留在函数作用域,后面注 epsSrc)
	//   lixinger 构建(retry 开关)
	//   eastmoney 注册(lixinger fallback)
	//   crypto 注册(Extra 透传 Init)
	//   qlibWarehouseDB, _ := wireQlibWarehouse(...)
	//   epsSrc 组装(qlibpit 包裹 yahoo)+ SetValuationSources + SetValuationLookback

	// 新增①:估值三项来源(A 股经 lixinger;typed-nil 防护同 valuationSourceOrNil)
	if lixingerCollector != nil {
		application.SetFundamentalSource(lixingerCollector)
	}

	// 新增②:cleanup 关闭 qlib 仓库句柄
	cleanup = func() {
		if qlibWarehouseDB != nil {
			_ = qlibWarehouseDB.Close()
		}
	}
	return cleanup, nil
}
```

迁移要点:
1. serve.go 原文中 `var yahooCollector *yahoo.Yahoo`、`var lixingerCollector *lixinger.Lixinger`、`qlibWarehouseDB` 都在被迁移段内声明,函数内直接可用;
2. `sql.Open("sqlite", ...)` 回调原样保留;
3. serve.go 中被迁移段之后没有任何代码引用这三个变量(已核实:backtester 用 `yahoo.New()` 新建),迁移安全。

- [ ] **Step 4: serve.go 替换为调用**

`cmd/atlas/serve.go` 原 99-170 行替换为:

```go
	// Register collectors + valuation sources (shared with `atlas watchlist`).
	cleanupCollectors, err := buildCollectors(cfg, application, log)
	if err != nil {
		return fmt.Errorf("wiring collectors: %w", err)
	}
	defer cleanupCollectors()
```

同时移除 serve.go 顶部因迁移而不再使用的 import(`database/sql`、`qlibpit`、`crypto`、`eastmoney`、`lixinger` 等以 `goimports`/编译器提示为准;`yahoo` 仍被 backtester 段使用,保留)。

- [ ] **Step 5: 运行测试与全量回归**

Run: `go build ./... && go test ./cmd/atlas/`
Expected: 编译干净;TestBuildCollectors 两用例 PASS;既有 serve/executor/export 测试全 PASS(行为零变化)

- [ ] **Step 6: Commit**

```bash
git add cmd/atlas/collectors.go cmd/atlas/collectors_test.go cmd/atlas/serve.go
git commit -m "refactor(watchlist-cmd): extract buildCollectors shared wiring from serve"
```

---

### Task 4: `atlas watchlist` 命令与渲染

**Files:**
- Create: `cmd/atlas/watchlist.go`
- Create: `cmd/atlas/watchlist_test.go`

**Interfaces:**
- Consumes: Task 2 `app.SymbolMetrics` / `application.SnapshotMetrics`;Task 3 `buildCollectors`;Task 1 `text.DisplayWidth` / `text.PadRight`;既有 `loadConfigOrDefaults()`(cmd/atlas/export_ohlcv.go:283)、`application.AddToWatchlistWithDetails`
- Produces: 终端命令 `atlas watchlist [--json] [--symbols A,B]`

- [ ] **Step 1: 写失败测试**

`cmd/atlas/watchlist_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/text"
)

func f64(v float64) *float64 { return &v }

func fixtureMetrics() []app.SymbolMetrics {
	return []app.SymbolMetrics{
		{Symbol: "600519.SH", Name: "贵州茅台", Market: "A股", Type: "股票",
			Price: 1408, ChangePct: -0.5,
			PE: f64(19.5), PB: f64(5.96), DividendYield: f64(4.03),
			PEPercentile: f64(12.3), PricePercentile: f64(18)},
		{Symbol: "AAPL", Name: "Apple", Market: "美股", Type: "股票",
			Price: 213.5, ChangePct: 1.2,
			PEPercentile: f64(78.5), PricePercentile: f64(85.2),
			Gaps: []string{"fundamental source not configured for market"}},
	}
}

func runExecute(t *testing.T, params watchlistParams, ms []app.SymbolMetrics) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	deps := watchlistDeps{
		snapshot: func(ctx context.Context, symbols []string) []app.SymbolMetrics { return ms },
		known:    []string{"600519.SH", "AAPL"},
		out:      &out,
		errOut:   &errOut,
	}
	err := executeWatchlist(context.Background(), deps, params)
	return out.String(), errOut.String(), err
}

func TestExecuteWatchlist_TableAligned(t *testing.T) {
	out, _, err := runExecute(t, watchlistParams{}, fixtureMetrics())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header + 2 rows, got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "SYMBOL") || !strings.Contains(lines[0], "PX%ILE") {
		t.Errorf("bad header: %q", lines[0])
	}
	// CJK 对齐:每行 NAME 列起点一致 → 各行 SYMBOL 段显示宽度一致
	nameIdx := strings.Index(lines[0], "NAME")
	headWidth := text.DisplayWidth(lines[0][:nameIdx])
	for _, ln := range lines[1:3] {
		cut := strings.Index(ln, "贵州茅台")
		if cut < 0 {
			cut = strings.Index(ln, "Apple")
		}
		if cut < 0 {
			continue
		}
		if w := text.DisplayWidth(ln[:cut]); w != headWidth {
			t.Errorf("misaligned NAME column: width %d vs header %d in %q", w, headWidth, ln)
		}
	}
	// 缺失指标显示 —
	if !strings.Contains(out, "—") {
		t.Error("missing metrics should render as —")
	}
	// 缺口摘要
	if !strings.Contains(out, "AAPL") || !strings.Contains(out, "fundamental source") {
		t.Error("gap summary missing")
	}
}

func TestExecuteWatchlist_JSON(t *testing.T) {
	out, _, err := runExecute(t, watchlistParams{jsonOut: true}, fixtureMetrics())
	if err != nil {
		t.Fatal(err)
	}
	var got []app.SymbolMetrics
	if uerr := json.Unmarshal([]byte(out), &got); uerr != nil {
		t.Fatalf("invalid json: %v\n%s", uerr, out)
	}
	if len(got) != 2 || got[1].PE != nil {
		t.Errorf("json roundtrip: %+v", got)
	}
	if !strings.Contains(out, `"pe": null`) && !strings.Contains(out, `"pe":null`) {
		t.Error("missing metrics must serialize as null")
	}
}

func TestExecuteWatchlist_UnknownSymbolWarns(t *testing.T) {
	_, errOut, err := runExecute(t, watchlistParams{symbols: []string{"AAPL", "NOPE"}}, fixtureMetrics()[1:])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "NOPE") {
		t.Errorf("expected warning for unknown symbol, got %q", errOut)
	}
}

func TestExecuteWatchlist_AllUnknownSymbolsErrors(t *testing.T) {
	_, _, err := runExecute(t, watchlistParams{symbols: []string{"NOPE"}}, nil)
	if err == nil {
		t.Fatal("expected error when no requested symbol is in watchlist")
	}
}

func TestExecuteWatchlist_AllFailedErrors(t *testing.T) {
	ms := []app.SymbolMetrics{
		{Symbol: "AAPL", Gaps: []string{"quote unavailable"}},
		{Symbol: "MSFT", Gaps: []string{"quote unavailable"}},
	}
	_, _, err := runExecute(t, watchlistParams{}, ms)
	if err == nil {
		t.Fatal("expected error when every symbol failed")
	}
}

func TestExecuteWatchlist_EmptyWatchlist(t *testing.T) {
	out, errOut, err := runExecute(t, watchlistParams{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" && !strings.Contains(errOut+out, "empty") {
		t.Errorf("expected empty-watchlist notice, out=%q err=%q", out, errOut)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run TestExecuteWatchlist -v`
Expected: FAIL(undefined: watchlistParams / watchlistDeps / executeWatchlist)

- [ ] **Step 3: 实现 `cmd/atlas/watchlist.go`**

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/text"
)

var (
	watchlistJSON    bool
	watchlistSymbols string
)

var watchlistCmd = &cobra.Command{
	Use:   "watchlist",
	Short: "Show quote/valuation/percentile metrics for all watchlist symbols",
	Long: `Fetches the latest price, change %, PE/PB/dividend yield and PE/price
percentiles for every watchlist symbol, offline (no running serve needed).
Assembly reuses the analysis loop's exact valuation pipeline.`,
	RunE: runWatchlist,
}

func init() {
	watchlistCmd.Flags().BoolVar(&watchlistJSON, "json", false, "output JSON instead of a table")
	watchlistCmd.Flags().StringVar(&watchlistSymbols, "symbols", "", "comma-separated subset of watchlist symbols")
	rootCmd.AddCommand(watchlistCmd)
}

// watchlistParams are the parsed CLI inputs.
type watchlistParams struct {
	jsonOut bool
	symbols []string // nil = full watchlist
}

// watchlistDeps injects the snapshot function and writers so executeWatchlist
// is unit-testable (mirrors exportDeps).
type watchlistDeps struct {
	snapshot func(ctx context.Context, symbols []string) []app.SymbolMetrics
	known    []string // watchlist symbols, for --symbols validation
	out      io.Writer
	errOut   io.Writer
}

func runWatchlist(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return err
	}
	log := newStderrLogger()
	application := app.New(cfg, log)
	cleanup, err := buildCollectors(cfg, application, log)
	if err != nil {
		return err
	}
	defer cleanup()
	known := make([]string, 0, len(cfg.Watchlist))
	for _, item := range cfg.Watchlist {
		application.AddToWatchlistWithDetails(item.Symbol, item.Name, item.Market, item.Type, item.Strategies)
		known = append(known, item.Symbol)
	}

	deps := watchlistDeps{snapshot: application.SnapshotMetrics, known: known, out: os.Stdout, errOut: os.Stderr}
	params := watchlistParams{jsonOut: watchlistJSON}
	if s := strings.TrimSpace(watchlistSymbols); s != "" {
		params.symbols = strings.Split(s, ",")
	}
	return executeWatchlist(cmd.Context(), deps, params)
}

// newStderrLogger keeps stdout clean for table/JSON output; warnings from the
// valuation pipeline stay visible on stderr.
func newStderrLogger() *zap.Logger {
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.Lock(os.Stderr), zapcore.WarnLevel)
	return zap.New(core)
}

func executeWatchlist(ctx context.Context, deps watchlistDeps, params watchlistParams) error {
	filter, err := resolveSymbolFilter(deps, params.symbols)
	if err != nil {
		return err
	}
	ms := deps.snapshot(ctx, filter)
	if len(ms) == 0 {
		fmt.Fprintln(deps.errOut, "watchlist is empty — add symbols in config.yaml or via the web UI")
		return nil
	}
	if allFailed(ms) {
		renderGaps(deps.errOut, ms)
		return fmt.Errorf("all %d symbols failed to fetch any metric", len(ms))
	}
	if params.jsonOut {
		encodeErr := json.NewEncoder(deps.out).Encode(ms)
		return encodeErr
	}
	renderTable(deps.out, ms)
	renderGaps(deps.out, ms)
	return nil
}

// resolveSymbolFilter validates --symbols against the watchlist: unknown
// symbols warn and are dropped; nothing left is an error; no filter = nil.
func resolveSymbolFilter(deps watchlistDeps, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	known := make(map[string]bool, len(deps.known))
	for _, s := range deps.known {
		known[s] = true
	}
	var valid []string
	for _, s := range requested {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if known[s] {
			valid = append(valid, s)
		} else {
			fmt.Fprintf(deps.errOut, "warning: %s is not in the watchlist, skipping\n", s)
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("none of the requested symbols are in the watchlist")
	}
	return valid, nil
}

// allFailed reports whether every symbol yielded no metric at all.
func allFailed(ms []app.SymbolMetrics) bool {
	for _, m := range ms {
		if m.Price != 0 || m.PricePercentile != nil || m.PEPercentile != nil || m.PE != nil {
			return false
		}
	}
	return true
}

const naCell = "—"

func renderTable(w io.Writer, ms []app.SymbolMetrics) {
	headers := []string{"SYMBOL", "NAME", "MARKET", "PRICE", "CHG%", "PE", "PB", "DYR%", "PE%ILE", "PX%ILE"}
	rows := make([][]string, 0, len(ms))
	for _, m := range ms {
		rows = append(rows, []string{
			m.Symbol, m.Name, m.Market,
			fmt.Sprintf("%.2f", m.Price),
			fmt.Sprintf("%+.1f%%", m.ChangePct),
			fmtPtr(m.PE, "%.1f"),
			fmtPtr(m.PB, "%.2f"),
			fmtPtr(m.DividendYield, "%.2f"),
			fmtPtr(m.PEPercentile, "%.1f"),
			fmtPtr(m.PricePercentile, "%.1f"),
		})
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = text.DisplayWidth(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if w := text.DisplayWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	writeRow := func(cells []string) {
		var sb strings.Builder
		for i, c := range cells {
			if i == len(cells)-1 {
				sb.WriteString(c) // 末列不补尾空格(对齐 telegram renderTable 约定)
			} else {
				sb.WriteString(text.PadRight(c, widths[i]))
				sb.WriteString("  ")
			}
		}
		fmt.Fprintln(w, sb.String())
	}
	writeRow(headers)
	for _, row := range rows {
		writeRow(row)
	}
}

func fmtPtr(v *float64, format string) string {
	if v == nil {
		return naCell
	}
	return fmt.Sprintf(format, *v)
}

// renderGaps prints the per-symbol data-gap summary after the table.
func renderGaps(w io.Writer, ms []app.SymbolMetrics) {
	for _, m := range ms {
		if len(m.Gaps) > 0 {
			fmt.Fprintf(w, "! %s: %s\n", m.Symbol, strings.Join(m.Gaps, "; "))
		}
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./cmd/atlas/ -run TestExecuteWatchlist -v`
Expected: 7 个用例全 PASS

- [ ] **Step 5: 手工冒烟(可选,需真实配置与网络)**

```bash
go run ./cmd/atlas watchlist --config configs/config.yaml | head -20
go run ./cmd/atlas watchlist --config configs/config.yaml --json | head -5
go run ./cmd/atlas watchlist --config configs/config.yaml --symbols 600519.SH
```

Expected: 对齐表格 / 合法 JSON / 单标的输出;网络失败时表格照出、缺口摘要注明原因。

- [ ] **Step 6: 全量回归**

Run: `go build ./... && go test ./...`
Expected: 全绿(离线)

- [ ] **Step 7: Commit**

```bash
git add cmd/atlas/watchlist.go cmd/atlas/watchlist_test.go
git commit -m "feat(watchlist-cmd): add atlas watchlist metrics command"
```

---

## Self-Review 结论(已执行)

1. **Spec 覆盖**:命令界面(§2)→ Task 4;SnapshotMetrics(§3)→ Task 2;buildCollectors(§4)→ Task 3;internal/text 提取(§2/§5)→ Task 1;退出码/降级(§2)→ Task 4 测试;YAGNI 边界(§6)未引入。spec "顶层 gaps 数组" 与 "输出 JSON 数组" 存在歧义,计划裁定:**JSON 为数组、gaps 内嵌每标的对象**(信息等价,消歧)。
2. **占位符扫描**:Task 3 Step 3 的"原文迁移省略"是对既有代码的剪切指令(源位置 serve.go:99-170 精确),非 TBD;其余步骤代码完整。
3. **类型一致性**:`SymbolMetrics`/`SnapshotMetrics`/`SetFundamentalSource`/`buildCollectors`/`text.DisplayWidth`/`text.PadRight` 在各 Task 的 Interfaces 块签名逐字一致。
