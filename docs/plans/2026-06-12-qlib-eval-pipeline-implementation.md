# Qlib 回测验证管线 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立「atlas export-signals 导出真实策略信号 → Python/qlib 事件研究评估」管线，量化回答每个 buy/sell 信号的后续收益、超额收益与胜率。

**Architecture:** Go 侧新增 export-signals CLI 子命令，复用 `internal/backtest` 引擎逐日重放并读取 `Result.Signals`（引擎做两处最小改动：统一盖戳 GeneratedAt、统计跳过 bar 数）；CSV 为唯一跨语言契约；Python 侧 `scripts/qlib_eval/` 为薄评估层，价格数据源可注入（pytest 不依赖真实 qlib 数据包），事件研究按「次日开盘入场、5/20/60 日 horizon、相对 SH000300 超额」口径计算并产出 markdown 报告。

**Tech Stack:** Go 1.21 + cobra + encoding/csv（沿用标准库测试风格）；Python 3.10+ + pandas + pytest + pyqlib（本地副本 /Users/zuowei/workspace/python/qlib，仅运行时依赖，测试不依赖）。

**设计依据（必读）：** `docs/plans/2026-06-11-qlib-eval-pipeline-design.md`（rev3 终版）

**执行纪律：**
- 严格 TDD：失败测试 → 验证失败 → 最小实现 → 验证通过 → 提交
- 全部 Task 完成后、最终集成提交前运行 code-simplifier sub-agent（全局规范）
- Go 测试 `go test ./...`；Python 测试 `cd scripts/qlib_eval && python -m pytest tests/ -v`

---

## Chunk 1: Go 导出端

### Task 1: backtest 引擎最小改动（盖戳 GeneratedAt + 跳过 bar 计数）

**Files:**
- Modify: `internal/backtest/backtester.go`（Run :69-81、Result 构造 :89-97）
- Modify: `internal/backtest/result.go`（Result 结构，若字段定义在别处则以实际为准）
- Test: `internal/backtest/backtester_test.go`

- [ ] **Step 1: 写失败测试**

```go
// stubStrategy 产生固定信号且故意保留错误的 GeneratedAt，
// 断言引擎统一盖戳为 bar 时间（设计 §2.1：机制性保证，不依赖策略自觉）
type stampStub struct{ failOn int }

func (s *stampStub) Name() string                            { return "stamp_stub" }
func (s *stampStub) Description() string                     { return "" }
func (s *stampStub) RequiredData() strategy.DataRequirements { return strategy.DataRequirements{PriceHistory: 1} }
func (s *stampStub) Init(strategy.Config) error              { return nil }
func (s *stampStub) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if len(ctx.OHLCV) > 0 && ctx.OHLCV[len(ctx.OHLCV)-1].Close == float64(s.failOn) {
		return nil, errors.New("boom") // 触发引擎的 skip 路径
	}
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy,
		GeneratedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)}}, nil // 错误时间，应被覆写
}

func TestRun_StampsGeneratedAtAndCountsSkips(t *testing.T) {
	bars := []core.OHLCV{
		{Symbol: "T", Close: 1, Time: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{Symbol: "T", Close: 2, Time: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)}, // failOn=2 → skip
		{Symbol: "T", Close: 3, Time: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)},
	}
	bt := New(staticProvider{bars}) // 若包内已有同类 stub provider 则复用
	res, err := bt.Run(context.Background(), &stampStub{failOn: 2}, "T",
		bars[0].Time, bars[2].Time)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Signals) != 2 {
		t.Fatalf("signals = %d, want 2", len(res.Signals))
	}
	if !res.Signals[0].GeneratedAt.Equal(bars[0].Time) || !res.Signals[1].GeneratedAt.Equal(bars[2].Time) {
		t.Errorf("GeneratedAt not stamped with bar time: %+v", res.Signals)
	}
	if res.SkippedBars != 1 {
		t.Errorf("SkippedBars = %d, want 1", res.SkippedBars)
	}
}
```

（`staticProvider`：实现 OHLCVProvider 返回固定 bars 的 3 行 stub，包内已有同类则复用。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/backtest/ -run TestRun_Stamps -v`
Expected: FAIL（GeneratedAt 未盖戳 / SkippedBars 字段不存在）

- [ ] **Step 3: 最小实现**

`backtester.go` Run() 中两处改动：

```go
		// :70-73 跳过计数
		signals, err := strat.Analyze(analysisCtx)
		if err != nil {
			skipped++
			continue // Skip bars with analysis errors
		}

		// :76-80 信号覆写处统一盖戳
		for _, sig := range signals {
			sig.Price = ohlcv[i].Close
			sig.Strategy = strat.Name()
			sig.GeneratedAt = ohlcv[i].Time // bar time, never wall clock
			allSignals = append(allSignals, sig)
		}
```

Result 增加字段 `SkippedBars int`，构造处带上（`skipped` 在循环前声明为 0）。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/backtest/ -v`
Expected: 全部 PASS（既有用例不回归）

- [ ] **Step 5: 提交**

```bash
git add internal/backtest/
git commit -m "feat(backtest): stamp GeneratedAt with bar time and count skipped bars"
```

### Task 2: 修复 ma_crossover 的 GeneratedAt 墙钟问题

**Files:**
- Modify: `internal/strategy/ma_crossover/strategy.go:100,117`（`GeneratedAt: time.Now()` → `GeneratedAt: ctx.Now`）
- Test: `internal/strategy/ma_crossover/strategy_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestAnalyze_GeneratedAtUsesCtxNow(t *testing.T) {
	// 构造一段产生金叉信号的序列（复用既有测试的造数方式），
	// ctx.Now 设为历史时间，断言信号 GeneratedAt == ctx.Now 而非墙钟
	past := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	ctx := buildCrossoverContext(past) // 复用/仿照既有测试 helper 构造金叉场景
	sigs, err := s.Analyze(ctx)
	if err != nil || len(sigs) == 0 {
		t.Fatalf("expected signal, got %v err=%v", sigs, err)
	}
	if !sigs[0].GeneratedAt.Equal(past) {
		t.Errorf("GeneratedAt = %v, want ctx.Now (%v)", sigs[0].GeneratedAt, past)
	}
}
```

- [ ] **Step 2: 确认失败 → Step 3: 两处 `time.Now()` 改 `ctx.Now` → Step 4: 确认通过**

Run: `go test ./internal/strategy/ma_crossover/ -v` → PASS

- [ ] **Step 5: 提交**

```bash
git add internal/strategy/ma_crossover/
git commit -m "fix(ma_crossover): use ctx.Now for GeneratedAt instead of wall clock"
```

### Task 3: 导出核心逻辑（白名单 + warm-up + CSV golden）

**Files:**
- Create: `cmd/atlas/export_signals.go`
- Test: `cmd/atlas/export_signals_test.go`

设计要点（spec §2 + 一个 spec 未明说的必要细节）：

- **warm-up 预热**：策略需要 `PriceHistory` 根 bar 才能出信号，若只拉 `from..to`，`from` 后的最初一段（ma_crossover 约 200 个交易日、price_percentile 约 5 年）窗口不足、信号全缺失。导出端必须把数据拉取起点前移 `maxBars*365/252 + 30` 自然日（与 app.historyWindowDays 同口径），重放后**只导出 `GeneratedAt >= from` 的信号**。
- **白名单**：`RequiredData().Fundamentals == true` 的策略直接报错拒绝并列出可用清单（动态判定，不硬编码）。

- [ ] **Step 1: 写失败测试（golden CSV + 白名单 + warm-up 过滤）**

```go
// fundamentalsStub: RequiredData().Fundamentals == true 的最小策略 stub
// flatStub: 每根 bar 都出 buy 信号（confidence 0.7, metadata {"k":1}）的 stub

func TestExportSignals_GoldenCSV(t *testing.T) {
	bars := makeBars(t, "2024-01-02", 5, 100) // 5 根连续交易日 bar，close=100..104
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: bars},
		strategies: engineWith(&flatStub{}),
		out:        &bytes.Buffer{},
		errOut:     &bytes.Buffer{},
	}
	var buf bytes.Buffer
	err := executeExport(deps, &buf, exportParams{
		Strategies: []string{"flat_stub"}, Symbols: []string{"600519.SH"},
		From: "2024-01-03", To: "2024-01-08", // from 晚于首 bar：warm-up 期信号须被过滤
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := `symbol,date,strategy,action,confidence,price,metadata
600519.SH,2024-01-03,flat_stub,buy,0.70,101.00,"{""k"":1}"
600519.SH,2024-01-04,flat_stub,buy,0.70,102.00,"{""k"":1}"
600519.SH,2024-01-05,flat_stub,buy,0.70,103.00,"{""k"":1}"
600519.SH,2024-01-08,flat_stub,buy,0.70,104.00,"{""k"":1}"
`
	if got != want {
		t.Errorf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportSignals_RejectsFundamentalStrategies(t *testing.T) {
	deps := exportDeps{strategies: engineWith(&fundamentalsStub{}), ...}
	err := executeExport(deps, io.Discard, exportParams{Strategies: []string{"funda_stub"}, ...})
	if err == nil || !strings.Contains(err.Error(), "requires fundamentals") {
		t.Errorf("want explicit rejection, got %v", err)
	}
}
```

（helper：`makeBars` 生成连续工作日 bar；`engineWith` 注册 stub 后返回 Engine；golden 中 2024-01-06/07 是周末，makeBars 按工作日生成。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run TestExportSignals -v`
Expected: FAIL（executeExport 未定义）

- [ ] **Step 3: 实现 export_signals.go 核心**

```go
type exportParams struct {
	Strategies []string
	Symbols    []string
	From, To   string
}

type exportDeps struct {
	provider   backtest.OHLCVProvider // 测试注入；CLI 路径按 symbol 经 SelectForSymbol 选取
	strategies *strategy.Engine
	out        io.Writer // CLI 信息输出
	errOut     io.Writer // 跳过摘要
}

// executeExport replays each (strategy, symbol) pair through the backtest
// engine and writes raw signals (pre-router, per design §2.2) as CSV to w.
func executeExport(deps exportDeps, w io.Writer, p exportParams) error {
	from, err := parseBacktestDate("from", p.From) // 复用 backtest.go 的解析
	...
	// 1. 白名单校验（全部策略先校验完再开始拉数据）
	for _, name := range p.Strategies {
		strat, ok := deps.strategies.Get(name)
		if !ok { return fmt.Errorf("unknown strategy %q (available: %v)", ...) }
		if strat.RequiredData().Fundamentals {
			return fmt.Errorf("strategy %q requires fundamentals and cannot be replayed offline (available: %v)",
				name, offlineNames(deps.strategies))
		}
	}
	// 2. CSV header
	cw := csv.NewWriter(w)
	cw.Write([]string{"symbol", "date", "strategy", "action", "confidence", "price", "metadata"})
	// 3. 逐 (symbol × strategy)：warm-up 起点 = from - (maxBars*365/252+30) 天；
	//    bt.Run(ctx, strat, symbol, warmupStart, to)；
	//    仅写出 sig.GeneratedAt >= from 的行；
	//    confidence/price 格式 %.2f；metadata 用 encoding/json Marshal（nil → 空串）
	// 4. 汇总 SkippedBars > 0 时向 deps.errOut 输出一行摘要
	cw.Flush()
	return cw.Error()
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/atlas/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/export_signals*.go
git commit -m "feat(cli): export-signals core with whitelist, warm-up and golden CSV"
```

### Task 4: CLI 接线 + Makefile

**Files:**
- Modify: `cmd/atlas/export_signals.go`（cobra 命令与 flags，模式照抄 backtest.go :31-49）
- Modify: `Makefile`

- [ ] **Step 1: cobra 命令**

```go
var exportCmd = &cobra.Command{
	Use:   "export-signals",
	Short: "Replay strategies over history and export raw signals as CSV",
	RunE:  runExportSignals,
}
// flags: --strategies (comma list, required), --symbols (comma list, required),
//        --from/--to (required), --out (default "signals.csv", "-" = stdout)
// runExportSignals: 组 registry（照抄 backtest.go:70-73），逐 symbol SelectForSymbol，
//                   engine 注册 ma_crossover.New(50,200) + price_percentile.New()（同 backtest.go:81-85），
//                   打开输出文件后调 executeExport
```

- [ ] **Step 2: Makefile target**

```makefile
SIGNAL_SYMBOLS ?= 600519.SH,000300.SH
SIGNAL_FROM    ?= 2021-01-01
SIGNAL_TO      ?= 2026-06-01

.PHONY: export-signals
export-signals: build
	./bin/atlas export-signals --strategies ma_crossover,price_percentile \
	  --symbols $(SIGNAL_SYMBOLS) --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals.csv
```

- [ ] **Step 3: 编译 + 冒烟**

```bash
go build -o bin/atlas ./cmd/atlas
./bin/atlas export-signals --help            # flags 齐全
# 网络可用时：make export-signals 应产出非空 signals.csv（首行为 header）
```

- [ ] **Step 4: 提交**

```bash
git add cmd/atlas/ Makefile
git commit -m "feat(cli): wire export-signals command and Makefile target"
```

---

## Chunk 2: Python 评估端

### Task 5: 脚手架 + 符号映射

**Files:**
- Create: `scripts/qlib_eval/README.md`、`scripts/qlib_eval/requirements.txt`、`scripts/qlib_eval/qlib_eval/__init__.py`、`scripts/qlib_eval/qlib_eval/symbols.py`
- Test: `scripts/qlib_eval/tests/test_symbols.py`

- [ ] **Step 1: 写失败测试**

```python
from qlib_eval.symbols import to_qlib_instrument

def test_to_qlib_instrument():
    assert to_qlib_instrument("600519.SH") == "SH600519"
    assert to_qlib_instrument("000300.SH") == "SH000300"
    assert to_qlib_instrument("399001.SZ") == "SZ399001"

def test_to_qlib_instrument_rejects_non_ashare():
    import pytest
    for bad in ("AAPL", "^GSPC", "GC=F", "BTC-USDT", "0700.HK"):
        with pytest.raises(ValueError):
            to_qlib_instrument(bad)
```

- [ ] **Step 2: 确认失败**

Run: `cd scripts/qlib_eval && python -m pytest tests/ -v`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现**

```python
# qlib_eval/symbols.py
def to_qlib_instrument(symbol: str) -> str:
    """600519.SH -> SH600519. Raises ValueError for non-A-share symbols
    (phase 1 is A-share only, design §1.1)."""
    if symbol.endswith(".SH"):
        return "SH" + symbol[:-3]
    if symbol.endswith(".SZ"):
        return "SZ" + symbol[:-3]
    raise ValueError(f"not an A-share symbol: {symbol}")
```

requirements.txt：`pandas>=1.5` + 注释说明 pyqlib 两种安装方式（`pip install pyqlib` 或 `pip install -e /Users/zuowei/workspace/python/qlib`），**qlib 仅运行时依赖、测试不需要**。README：数据包下载命令（`python -m qlib.cli.data qlib_data --target_dir ~/.qlib/qlib_data/cn_data --region cn`，注明托管在 SunsetWolf/qlib_dataset releases、国内可能需代理、数据截止日局限）、运行步骤、口径说明。

- [ ] **Step 4: 确认通过 → Step 5: 提交**

```bash
git add scripts/qlib_eval/
git commit -m "feat(qlib_eval): scaffold with symbol mapping"
```

### Task 6: 价格数据源抽象 + 入场对齐

**Files:**
- Create: `scripts/qlib_eval/qlib_eval/prices.py`（PriceSource 协议 + QlibPriceSource + 入场对齐函数）
- Test: `scripts/qlib_eval/tests/test_prices.py`

- [ ] **Step 1: 写失败测试**

```python
import pandas as pd
from qlib_eval.prices import align_entry

def frame(dates, opens, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    return pd.DataFrame({"open": opens, "close": closes}, index=idx)

PRICES = frame(
    ["2024-01-02", "2024-01-03", "2024-01-04", "2024-01-05", "2024-01-15"],
    [10.0, 10.2, 10.4, 10.6, 11.0],
    [10.1, 10.3, 10.5, 10.7, 11.1],
)

def test_align_entry_next_open():
    # 信号日 1/2 → 入场 1/3 开盘（次日开盘，规避前视）
    e = align_entry(PRICES, pd.Timestamp("2024-01-02"), max_defer=5)
    assert e.date == pd.Timestamp("2024-01-03") and e.price == 10.2

def test_align_entry_defers_over_holiday():
    # 信号日 1/6（周六）→ 顺延到 1/15? 不：下一交易日是 1/15 之前无数据，
    # 1/5 信号 → 次日 1/15 间隔超 5 个交易日? 数据中 1/5 后下一行就是 1/15，
    # 顺延仅 1 个数据行 → 允许；用 max_defer 控制按交易日行数计
    e = align_entry(PRICES, pd.Timestamp("2024-01-05"), max_defer=5)
    assert e.date == pd.Timestamp("2024-01-15")

def test_align_entry_drops_when_no_data():
    assert align_entry(PRICES, pd.Timestamp("2024-01-15"), max_defer=5) is None  # 其后无 bar
```

- [ ] **Step 2: 确认失败 → Step 3: 实现**

```python
# qlib_eval/prices.py
from dataclasses import dataclass
from typing import Protocol
import pandas as pd

@dataclass
class Entry:
    date: pd.Timestamp
    price: float   # next-day open
    index: int     # positional index into the price frame

class PriceSource(Protocol):
    def history(self, symbol: str) -> pd.DataFrame: ...
    """index=交易日 DatetimeIndex，columns 含 open/close；symbol 为 atlas 形式（600519.SH）"""
    def benchmark(self) -> pd.DataFrame: ...
    """SH000300 的 close 序列"""

def align_entry(prices: pd.DataFrame, signal_date: pd.Timestamp, max_defer: int) -> Entry | None:
    """Entry at the first trading day strictly after signal_date (next-day open,
    no look-ahead). Returns None when no bar follows; defer is bounded by
    max_defer positional rows (drop & count by caller)."""
    pos = prices.index.searchsorted(signal_date, side="right")
    if pos >= len(prices.index):
        return None
    # 顺延语义：searchsorted 已落在下一交易日；max_defer 限制的是信号日与入场日
    # 之间的「日历跨度对应的缺数据行」，此处直接判定 pos 行有效即可——
    # 停牌缺行体现为 next bar 距信号日远；用交易日行数无法表达，按
    # (entry.date - signal_date).days > max_defer*2 视为顺延过久 → None
    entry_date = prices.index[pos]
    if (entry_date - signal_date).days > max_defer * 2:
        return None
    return Entry(date=entry_date, price=float(prices.iloc[pos]["open"]), index=int(pos))

class QlibPriceSource:
    """Lazy-imports qlib; only used in real runs, never in pytest."""
    def __init__(self, provider_uri: str, start: str, end: str): ...
    # qlib.init(provider_uri=..., region="cn")
    # D.features([to_qlib_instrument(sym)], ["$open", "$close"], start, end) → 标准化 DataFrame
```

（`max_defer*2` 的日历日近似：5 个交易日 ≈ 7-10 个日历日，取 `*2` 上界；该规则写进 README 口径说明。）

- [ ] **Step 4: 确认通过 → Step 5: 提交**

```bash
git add scripts/qlib_eval/
git commit -m "feat(qlib_eval): price source protocol and entry alignment"
```

### Task 7: 事件研究计算核心

**Files:**
- Create: `scripts/qlib_eval/qlib_eval/event_study.py`
- Test: `scripts/qlib_eval/tests/test_event_study.py`

- [ ] **Step 1: 写失败测试（覆盖全部口径）**

```python
# 合成场景（数值手工可验算）：
# 标的：入场开盘 10.0，5 个交易日后 close 11.0 → 绝对收益 +10%
# 基准：同期 close 3000 → 3060 → +2%；超额 = +8%
def test_horizon_return_and_excess(): ...

# sell 信号规避口径：标的 -10%、基准 +2% → 规避收益 = -( -10% - 2% ) = +12%
def test_sell_avoidance_return(): ...

# horizon 越界 → NA 并计数
def test_horizon_exceeds_data_returns_none(): ...

# 聚合：按 strategy×action 的 mean/median/win_rate/n；
# buy 胜率口径 = 超额 > 0 占比；sell 胜率 = 规避收益 > 0 占比
def test_aggregate_by_strategy_action(): ...

# 置信度分桶：>=0.6 / >=0.8 两桶重复聚合
def test_confidence_buckets(): ...
```

- [ ] **Step 2: 确认失败 → Step 3: 实现**

```python
# qlib_eval/event_study.py
HORIZONS = (5, 20, 60)
CONF_BUCKETS = (0.0, 0.6, 0.8)

@dataclass
class SignalOutcome:
    symbol: str; date: pd.Timestamp; strategy: str; action: str; confidence: float
    returns: dict[int, float | None]        # horizon → 绝对收益
    excess: dict[int, float | None]         # horizon → 超额（sell 为规避口径，已取向）

def evaluate_signal(sig, prices, bench, max_defer=5) -> SignalOutcome | None:
    # align_entry → None 则调用方计入 dropped
    # h 日收益 = close[entry.index+h] / entry.price - 1（越界 → None）
    # 基准收益 = bench_close[entry_date+h 对齐位置] / bench_close[entry_date] - 1
    #   （基准按日期对齐到自身索引；bench 缺该日期时取 searchsorted 最近前值）
    # buy/strong_buy: excess = ret - bench_ret
    # sell/strong_sell: excess = -(ret - bench_ret)  # 规避收益：信号后跑输基准记为正
    ...

def aggregate(outcomes) -> pd.DataFrame:
    # 行 = (strategy, action, conf_bucket, horizon)
    # 列 = n, mean_ret, median_ret, mean_excess, win_rate
    ...
```

- [ ] **Step 4: 确认通过 → Step 5: 提交**

```bash
git add scripts/qlib_eval/
git commit -m "feat(qlib_eval): event study core with excess/avoidance semantics"
```

### Task 8: CSV 读取、报告生成与 CLI 入口

**Files:**
- Create: `scripts/qlib_eval/qlib_eval/report.py`、`scripts/qlib_eval/evaluate.py`
- Test: `scripts/qlib_eval/tests/test_report.py`

- [ ] **Step 1: 写失败测试**

```python
# read_signals: 合法 CSV → DataFrame（date 解析、confidence float、metadata 保留原串）；
#               缺列/坏行 → ValueError 且消息含行号
def test_read_signals_valid_and_invalid(): ...

# render_report: 给定 aggregate 结果 + 统计（dropped/data_gaps/na_counts），
#                输出含「评估口径」「数据缺口」节与每策略小节的 markdown 字符串
def test_render_report_sections(): ...
```

- [ ] **Step 2: 确认失败 → Step 3: 实现**

- `report.py`：`read_signals(path)`（csv.DictReader 严格校验 7 列 schema）、`render_report(agg, stats, meta) -> str`
- `evaluate.py`（argparse 入口）：

```
python evaluate.py --signals signals.csv \
  [--qlib-dir ~/.qlib/qlib_data/cn_data] [--out reports/]
```

  - 启动即检测 qlib 数据目录，缺失则打印 get_data 命令并 exit(1)（设计 §5）
  - 非 A 股符号 → 跳过收集进「数据缺口」节
  - 报告写 `reports/signal-eval-YYYYMMDD.md`；`reports/` 加入根 .gitignore

- [ ] **Step 4: 确认通过 → Step 5: 提交**

```bash
git add scripts/qlib_eval/ .gitignore
git commit -m "feat(qlib_eval): CSV ingestion, markdown report and CLI entry"
```

### Task 9: 端到端串联与收尾

**Files:**
- Modify: `Makefile`
- Modify: `scripts/qlib_eval/README.md`

- [ ] **Step 1: Makefile signal-eval target**

```makefile
.PHONY: signal-eval
signal-eval: export-signals
	cd scripts/qlib_eval && python evaluate.py --signals ../../signals.csv --out ../../reports/
```

- [ ] **Step 2: 端到端验证**

```bash
go test ./... && (cd scripts/qlib_eval && python -m pytest tests/ -v)   # 全绿
# 数据包已就位时（一次性准备照 README）：
make signal-eval
# 预期：reports/signal-eval-*.md 生成，含 ma_crossover 与 price_percentile 两节、
# 样本数>0（区间 2021-2026 足够长）、口径与数据缺口说明齐全
```

- [ ] **Step 3: 运行 code-simplifier**（全局规范）

Task tool: `subagent_type: "code-simplifier:code-simplifier"`，prompt 列出本计划全部新增/修改文件。

- [ ] **Step 4: gitnexus + 全量回归 + 最终提交**

```bash
npx gitnexus analyze && go vet ./... && go test ./...
git add -A
git commit -m "feat: qlib signal evaluation pipeline (export-signals + event study)

Implements docs/plans/2026-06-11-qlib-eval-pipeline-design.md (rev3)"
```

---

## 验收对照（design §1.1/§6）

- [ ] 引擎盖戳：任意策略（含故意写错 GeneratedAt 的 stub）导出的 date 均为 bar 时间（Task 1 测试）
- [ ] 白名单：`--strategies pe_band` 显式报错并列出可用清单（Task 3 测试）
- [ ] warm-up：`from` 之后首日即可有信号（golden 测试覆盖 from 过滤边界）
- [ ] 事件研究口径全部有合成数据单测：次日开盘入场、顺延丢弃、超额、sell 规避、置信度分桶
- [ ] pytest 全程不依赖 qlib 安装与数据包；真实运行缺数据时给出明确下载指引
- [ ] `make signal-eval` 在数据就位的环境产出完整 markdown 报告
