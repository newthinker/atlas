# 需求分析 — 第三期 分位「自上市/起始日」全历史回看

> 来源: `docs/superpowers/plans/2026-06-16-percentile-since-inception-phase3.md`

## 目标
让 price_percentile / pe_percentile 支持「自上市/指数起始日」的全历史回看（`lookback_years: 0` = since inception），逐标的各按自己最早可得数据算分位，取代固定 5 年窗口（太短、分析意义不足）。

## 核心设计
`lookback_years: 0` → 策略 `RequiredData().PriceHistory` 返回 `SinceInceptionBars=100*252`（≈100 年交易日）。`historyWindowDays`（app.go:737，已能折算任意 PriceHistory）算出覆盖至 ~1926 的窗口；`FetchHistory` 对超出标的上市日的区间只返回实际存在的 bar → 逐标的全史天然成立，无需查精确 IPO 日。`minSampleBars=252` 仍兜底防新股误报。

PE 分位 lookback：app 硬编码常量 `valuationLookbackYears=5`（app.go:758）→ 改为可配字段（0=inception）。EPS 重建窗口（app.go:809）inception 时用早 floor；lixinger（785/799/822）inception 映射 y10。

## 功能模块（7 任务）
1. strategy 包 `SinceInceptionBars` 哨兵常量。
2. price_percentile 接受 lookback_years:0。
3. pe_percentile 接受 lookback_years:0。
4. config `ValuationConfig{LookbackYears}`。
5. app PE lookback 常量→可配字段 + inception 逻辑（EPS floor / lixinger y10）。
6. serve.go 装配 `SetValuationLookback(cfg.Valuation.LookbackYears)`。
7. 全史 dump（WAREHOUSE_FROM）+ config 示例 + 文档（best-effort，含 lixinger 10Y 上限说明）。

## 复杂度
中等。核心是「0=inception」语义贯通策略→app→数据三层，且保默认 5 年零回归。风险点：app 三处 lixinger 调用 + EPS floor 改动需保既有 5 年测试零回归；lixinger 10Y 上限是外部限制，诚实标注不绕过。

## 诚实边界
| 路径 | inception 能力 |
|---|---|
| 价格分位（全市场） | ✅ 真·自上市起（受 dump 数据约束） |
| PE·美/港个股（yahoo EPS） | ✅ 真·自上市起 |
| PE·A 股个股+所有指数（lixinger） | ⚠ 上限 10 年（y3/y5/y10 三档，inception→y10） |

数据是前提：需重 dump 全史，否则美股仍 ~5 年。

## 范围边界（本期不做）
不绕过 lixinger y10；不查精确 IPO 日；全史 dump 实际生产 best-effort（US 打通）；不改 minSampleBars。
