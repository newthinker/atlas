# Changelog — 第三期 分位「自上市/起始日」全历史回看

## Added
- **`lookback_years: 0` = since inception**：price_percentile / pe_percentile 配 0 时按标的全历史算分位（逐标的各按自己上市日，受数据范围约束）。
  - `strategy.SinceInceptionBars`（100*252）哨兵；inception 时 `RequiredData().PriceHistory` 返回它，`historyWindowDays` 算出全史窗口。
  - 信号 Reason 显示「full history (N bars)」，metadata `lookback_years: 0`。
- **`valuation.lookback_years` 配置**（默认 5）：app PE 分位 lookback 由硬编码常量改为可配字段，0=inception。
- **inception EPS/价格取数起点钳到 1970-01-01**（epochFloor），消除负 Unix period1；EPS 取数覆盖整个 ohlcv 价格窗口（无静默截断）。
- ADAPTERS.md「Lookback modes」节 + config.example 自洽示例（含 lixinger y10 上限说明）。

## Fixed (QA review)
- **C1**（零回归）：`config.Load()` 未对 valuation.lookback_years 设默认 → 存量配置静默切 inception。已加 `SetDefault(5)`。
- **W1**：example.yaml 两层 lookback 不自洽（开箱误导）→ 自洽默认 5。
- **W2**：策略全史窗但 valuation 5 年时早期收盘静默丢弃 → EPS 覆盖价格窗口。
- **W3**：inception 负 Unix period1 → 1970 floor 钳制。

## Unchanged (零回归)
- 默认/未配置 `lookback_years` 仍 5 年，行为与现状完全一致（Load 与 Defaults 两路径均成立）；负数仍拒绝；minSampleBars=252 兜底不变。

## Notes
- 全史回看需重 dump 全史数据；A 股个股+指数 PE 分位 inception 上限 10 年（lixinger）。
- 既有 bug（非本期）：export-ohlcv 在 yahoo.go:229 nil-deref panic，阻碍全史 dump。
