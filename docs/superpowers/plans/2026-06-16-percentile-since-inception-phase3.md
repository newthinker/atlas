# 分位分析「自上市/起始日」全历史回看 第三期实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 让 price_percentile / pe_percentile 支持「自上市/指数起始日」的全历史回看（`lookback_years: 0` = since inception），逐标的各按自己的最早可得数据算分位，取代当前固定 5 年窗口（太短、分析意义不足）。

**Architecture:** 三层改动——① 两个百分位策略接受 `lookback_years: 0`，其 `RequiredData().PriceHistory` 返回「全历史哨兵」`SinceInceptionBars`（≈100 年交易日），`historyWindowDays` 自然算出覆盖至 ~1926 的窗口，`FetchHistory` 按各标的实际上市日返回（已有 collector/warehouse 行为，无需改）；② app 的 PE 分位 lookback 从硬编码常量 `valuationLookbackYears=5` 改为可配字段（`0`=inception），EPS 重建窗口用早 floor、lixinger 映射到其上限 `y10`；③ 数据侧把 `SIGNAL_FROM` dump 起点前移到全历史，重建仓库。

**Tech Stack:** Go 1.24（GOTOOLCHAIN=local，`modernc.org/sqlite` 已在 go.mod）；Makefile/数据管线。

> 前置：第一期（OHLCV 仓库）、第二期（PIT 基本面）已合并入 master。

---

## 关键设计与边界

**「since inception」语义：** `lookback_years: 0` 表示用该标的全部可得历史。实现上不引入复杂哨兵分发，而是让策略在 0 时声明一个足够大的历史需求 `SinceInceptionBars = 100 * 252`；`historyWindowDays`（app.go:737）已能把任意 `PriceHistory` 折算成自然日窗口，`FetchHistory` 对超出标的上市日的区间只返回实际存在的 bar——于是「逐标的各按自己上市日」天然成立，无需知道每个标的的精确 IPO 日。`minSampleBars=252` 仍兜底（不足 1 年不出信号，防新股误报）。

**各路径能到多早（诚实边界）：**
| 路径 | 数据源 | inception 能力 |
|---|---|---|
| 价格分位（全市场） | warehouse/eastmoney/yahoo OHLCV | ✅ 真·自上市起（受 dump 数据范围约束） |
| PE 分位·美/港个股 | yahoo EPS 重建（qlibpit PIT 优先） | ✅ 真·自上市起（yahoo EPS 按 start 过滤可返全史） |
| PE 分位·A 股个股 + 所有指数 | lixinger cvpos | ⚠ **上限 10 年**（理杏仁只有 y3/y5/y10 三档，无全史档）→ inception 时映射到 y10 |

lixinger 10Y 上限是外部 API 限制，本期不绕过；`lookback_years: 0` 对这些标的等价于「最多 10 年」，需在配置文档与 Reason 文案中诚实标注。

**数据是前提：** 现 `qlib_csv_us` 只到 2021。代码改完后若不重 dump 全历史，inception 模式对美股仍只有 ~5 年。Task 6 负责把 dump 起点前移并重建仓库。

---

## 文件结构

- Modify: `internal/strategy/interface.go`（新增 `SinceInceptionBars` 常量 + 文档）
- Modify: `internal/strategy/price_percentile/strategy.go` + `_test.go`
- Modify: `internal/strategy/pe_percentile/strategy.go` + `_test.go`
- Modify: `internal/config/config.go`（新增 `ValuationConfig`）+ `_test.go`
- Modify: `internal/app/app.go`（常量→字段，inception 逻辑）+ `_test.go`
- Modify: `cmd/atlas/serve.go`（装配 valuation lookback）
- Modify: `Makefile`（dump 起点）、`configs/config.example.yaml`、`scripts/qlib_warehouse/ADAPTERS.md`（文档）

---

## Task 1: strategy 包新增 SinceInceptionBars 哨兵常量（Go）

**Files:** Modify `internal/strategy/interface.go`

`DataRequirements.PriceHistory`（interface.go:15-22）当前是「所需交易日数」。新增导出常量供两个策略在 `lookback_years==0` 时返回：

```go
// SinceInceptionBars is the PriceHistory value a percentile strategy declares
// when configured with lookback_years: 0 ("since inception"). It is a large
// trading-day count (~100 years) so historyWindowDays computes a window that
// reaches back past any real listing date; FetchHistory then returns only the
// bars that actually exist, giving each symbol its own full-history window.
const SinceInceptionBars = 100 * 252
```

- [ ] Step 1: 写测试 `internal/strategy/interface_test.go`（若无则建）：`TestSinceInceptionBarsIsLarge` 断言 `SinceInceptionBars >= 100*252`（保证折算后起点早于任何实际上市日）。
- [ ] Step 2: RED（undefined）。
- [ ] Step 3: 加常量。
- [ ] Step 4: GREEN。
- [ ] Step 5: 提交 `git commit -m "feat(strategy): SinceInceptionBars sentinel for full-history lookback"`

---

## Task 2: price_percentile 支持 lookback_years: 0（Go）

**Files:** Modify `internal/strategy/price_percentile/strategy.go` + `_test.go`

改三处：
1. `Init`（:55）校验从 `lookbackYears <= 0` 放宽为 `lookbackYears < 0`（0 合法 = inception；负数仍非法）。
2. `RequiredData`（:36）：`lookbackYears==0` 时 `PriceHistory: strategy.SinceInceptionBars`，否则 `lookbackYears*252`。
3. `Analyze`（:89）Reason 文案：`lookbackYears==0` 时显示 `"price at %.1f%% of full history (%d bars)"`，否则保持 `"%d-year range"`；metadata 的 `lookback_years` 仍写 0（下游可识别）。

```go
func (s *Strategy) RequiredData() strategy.DataRequirements {
	ph := s.lookbackYears * 252
	if s.lookbackYears == 0 {
		ph = strategy.SinceInceptionBars
	}
	return strategy.DataRequirements{PriceHistory: ph, AssetTypes: []core.AssetType{ /* 原样 */ }}
}
```

- [ ] Step 1: 测试 `TestRequiredData_SinceInception`（lookback_years=0 → PriceHistory==SinceInceptionBars）、`TestInit_AcceptsZeroLookback`（Init 不报错）、`TestInit_RejectsNegativeLookback`（-1 报错）、`TestAnalyze_FullHistoryReasonText`（0 时 Reason 含 "full history"）。RED。
- [ ] Step 2: 实现。GREEN：`GOTOOLCHAIN=local go test ./internal/strategy/price_percentile/ -v` 全绿，既有 5-year 用例零回归。
- [ ] Step 3: 提交 `git commit -m "feat(price_percentile): lookback_years:0 = since inception"`

---

## Task 3: pe_percentile 支持 lookback_years: 0（Go）

**Files:** Modify `internal/strategy/pe_percentile/strategy.go` + `_test.go`

与 Task 2 完全对称（同样三处：Init:61 放宽、RequiredData:44 用 SinceInceptionBars、Analyze:96 Reason 文案 `"PE at %.1f%% of full history (%s)"`）。PE 分位的实际窗口由 app 侧 ohlcv 窗口 + EPS 窗口共同决定（见 Task 5），本任务只让策略声明全历史的 PriceHistory 并接受 0。

- [ ] Step 1: 测试 `TestRequiredData_SinceInception` / `TestInit_AcceptsZeroLookback` / `TestInit_RejectsNegativeLookback` / `TestAnalyze_FullHistoryReasonText`。RED。
- [ ] Step 2: 实现。GREEN：`go test ./internal/strategy/pe_percentile/ -v` 全绿零回归。
- [ ] Step 3: 提交 `git commit -m "feat(pe_percentile): lookback_years:0 = since inception"`

---

## Task 4: ValuationConfig 配置结构（Go）

**Files:** Modify `internal/config/config.go` + `_test.go`

在 `Config`（config.go:13-29）新增字段与类型：
```go
	Valuation ValuationConfig `mapstructure:"valuation"`
```
```go
// ValuationConfig configures the app-side PE-percentile lookback used for EPS
// reconstruction and lixinger cvpos. LookbackYears: 0 means "since inception"
// (EPS reconstruction uses full history; lixinger is capped at its y10 bucket).
type ValuationConfig struct {
	LookbackYears int `mapstructure:"lookback_years"`
}
```
`config.Defaults()` 里 `Valuation.LookbackYears = 5`（保持现状默认，不改变未配置时行为）。

- [ ] Step 1: 测试 `TestLoad_ValuationConfig`（YAML `valuation.lookback_years: 0` 解析为 0）+ `TestDefaults_ValuationLookbackIs5`。RED（字段不存在）。
- [ ] Step 2: 实现。GREEN：`go test ./internal/config/ -v` 全绿零回归。
- [ ] Step 3: 提交 `git commit -m "feat(config): valuation.lookback_years (0=since inception)"`

---

## Task 5: app PE 分位 lookback 可配 + inception 逻辑（Go）

**Files:** Modify `internal/app/app.go` + `_test.go`

把硬编码常量 `valuationLookbackYears`（app.go:758）替换为 App 字段 `valuationLookback int`（默认 5），并在 `buildFundamental` 的三处消费点按 inception 调整：

1. App 结构加字段 + setter（紧邻现有 setters）：
```go
// SetValuationLookback sets the PE-percentile lookback in years (0 = since inception).
func (a *App) SetValuationLookback(years int) { a.valuationLookback = years }
```
`New(...)` 中默认 `valuationLookback: 5`（保持未配置时行为）。

2. EPS 重建窗口（app.go:809）：
```go
	end := time.Now()
	epsStart := end.AddDate(-a.valuationLookback, 0, -90)
	if a.valuationLookback == 0 { // since inception: 取足够早的 floor，yahoo/qlibpit 返回各标的全史
		epsStart = end.AddDate(-100, 0, 0)
	}
	eps, err := a.epsSrc.FetchEPSHistory(symbol, epsStart, end)
```
> 注：ReconstructPEPercentile(ohlcv, eps) 的有效 PE 分位窗口 = ohlcv 窗口（已由策略 PriceHistory→historyWindowDays 在 inception 下覆盖全史）；EPS 多取无害（仅用于阶梯对齐）。故 EPS floor 取 100 年即可。

3. lixinger 三处（app.go:785/799/822）：把 `valuationLookbackYears` 换成一个映射函数 `a.lixingerLookback()`：
```go
// lixingerLookback maps the configured lookback to a value lixinger can serve.
// lixinger cvpos only has y3/y5/y10 buckets; 0 (since inception) maps to its
// deepest bucket y10 (a documented limitation for CN stocks and all indices).
func (a *App) lixingerLookback() int {
	if a.valuationLookback == 0 {
		return 10
	}
	return a.valuationLookback
}
```
三处 `FetchValuationPercentile(symbol, valuationLookbackYears)` → `FetchValuationPercentile(symbol, a.lixingerLookback())`。删除 `const valuationLookbackYears`。

- [ ] Step 1: 测试 `internal/app/app_test.go`：
  - `TestValuationLookback_DefaultIs5`（New 后字段==5）。
  - `TestLixingerLookback_InceptionMapsToY10`（SetValuationLookback(0) → lixingerLookback()==10）。
  - `TestLixingerLookback_PassesThrough`（设 7 → 7）。
  - 若 buildFundamental 可在测试中以 fake epsSrc/valuationSrc 驱动：`TestBuildFundamental_InceptionEPSWindowSpansFull`（fake epsSrc 记录收到的 start，断言 inception 下 start 远早于 5 年前）。
  RED。
- [ ] Step 2: 实现（替换常量、加字段/setter、三处 lixinger 调用、EPS floor）。GREEN：`go test ./internal/app/ -v` 全绿零回归（既有用 5 年窗口的测试若断言具体起点需同步——默认仍 5，不应回归）。
- [ ] Step 3: 提交 `git commit -m "feat(app): configurable valuation lookback with since-inception mode"`

---

## Task 6: serve.go 装配 valuation lookback（Go）

**Files:** Modify `cmd/atlas/serve.go`

在 serve.go 构造 App、注册策略之后、`SetValuationSources` 附近，加：
```go
	application.SetValuationLookback(cfg.Valuation.LookbackYears)
```
（`cfg.Valuation.LookbackYears` 默认 5；配 0 即 inception。）确认调用在 `application.Start` 之前（与 QA S1 不变量一致）。

- [ ] Step 1: 编译 + 全量测试 `GOTOOLCHAIN=local go build ./... && go test ./internal/... ./cmd/...` 全绿零回归。
- [ ] Step 2: 提交 `git commit -m "feat(serve): wire valuation.lookback_years into app"`

---

## Task 7: 数据全历史 dump + 配置示例 + 文档（Makefile/docs，best-effort）

**Files:** Modify `Makefile`、`configs/config.example.yaml`、`scripts/qlib_warehouse/ADAPTERS.md`

> 本任务不含 Go 单测（数据/文档）。

1. Makefile（:7）新增全历史起点变量（不改默认 `SIGNAL_FROM`，避免影响既有 signal-eval 数据包），供仓库 dump 用：
```makefile
WAREHOUSE_FROM ?= 1970-01-01   # 全历史 dump 起点；yahoo 按各标的实际上市日返回
```
并新增/调整一个生成全历史 per-instrument CSV 的便捷 target（或在文档说明用 `export-ohlcv --from $(WAREHOUSE_FROM)` 重生成 `qlib_csv_us` 后再 `make warehouse-dump`）。
2. `configs/config.example.yaml`：在 `strategies.price_percentile.params` / `pe_percentile.params` 加注释示例 `lookback_years: 0  # 0 = 自上市/起始日全历史`；新增 `valuation: { lookback_years: 0 }` 块并注明「A 股个股 + 指数走 lixinger，PE 分位上限 10 年」。
3. `ADAPTERS.md`：补一节「Lookback modes」说明 inception 各路径能力与 lixinger y10 上限。

- [ ] Step 1: 改 Makefile + config.example + ADAPTERS。
- [ ] Step 2: 手工验证：`export-ohlcv --from 1970-01-01 --market us ...` 生成的 AAPL CSV 首行日期远早于 2021（接近 1980）；`make warehouse-dump` 成功；启动 serve 配 `lookback_years: 0` 日志正常、`/history` 返回全历史 bar 数远超 5 年。
- [ ] Step 3: 提交 `git commit -m "docs(warehouse): full-history dump + since-inception config + lixinger y10 note"`

---

## 完成标准（DoD）

- 两个百分位策略接受 `lookback_years: 0`，`RequiredData().PriceHistory == SinceInceptionBars`，负值仍拒绝；既有 N-year 行为零回归。
- app PE 分位 lookback 由 `valuation.lookback_years` 驱动；0=inception 时 EPS 重建用全史 floor、lixinger 映射 y10；默认 5 时行为与现状完全一致。
- `go build ./... && go test ./internal/... ./cmd/...` 全绿零回归。
- 配 `lookback_years: 0` 实跑：`/history` 返回标的全历史、price/pe 分位按全史计算（pe 美/港个股全史、CN/指数≤10Y）。
- 文档诚实标注 lixinger 10Y 上限。

## 范围边界（本期不做）

- 不绕过 lixinger 的 y10 上限（A 股个股/指数 PE 分位最多 10 年）。
- 不做 per-symbol 精确 IPO 日查询（用早 floor + 数据源天然裁剪即可）。
- 全历史数据的实际生产（dump）为 best-effort（Task 7 文档化 + US 打通）；A/HK 全史 dump 随需扩展。
- 不改 `minSampleBars=252`（不足 1 年仍不出信号）。
