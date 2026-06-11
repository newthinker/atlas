# 设计规格 — sprint-002（指数/商品 + 百分位策略）

> **本 Sprint 的权威施工图是 `docs/plans/2026-06-11-index-commodity-percentile-implementation.md`（rev3）**，
> 其中每个 plan Task 含完整 TDD 步骤、测试代码与实现骨架。本文件只记录 arcforge 任务图重组与跨任务接口约定，
> **不重复抄写实现细节**——Dev 开工时必须读 plan 对应 Task 原文。

## plan Task → arcforge 任务映射

| arcforge | plan Task | package(s) | 要点 |
|----------|-----------|------------|------|
| TASK-001 | T1 | internal/core | AssetCrypto/EPSPoint/PEPercentile；纯类型，消费方测试覆盖 |
| TASK-002 | T2+T3 | internal/collector/yahoo | 符号校验正则 + URL PathEscape（局部变量 url→reqURL）+ FetchEPSHistory + NewWithBaseURLs |
| TASK-003 | T4(前半)+T5 | internal/collector | indexes.go 共享表 + selector 路由（^/=F→yahoo）+ MarketForSymbol + KnownIndexMarket |
| TASK-004 | T4(后半) | internal/collector/eastmoney | parseSymbol 用 AShareIndexSecIDs 解析指数 secid |
| TASK-005 | T7 | internal/collector/lixinger | endpointFor 分派 + FetchValuationPercentile（cvpos×100；postJSONRaw 注意事项见 plan） |
| TASK-006 | T8 | internal/valuation | PercentileRank（strictly-less，空→-1）+ ReconstructPEPercentile（阶梯对齐/MinEPSPoints=8/双哨兵错误） |
| TASK-007 | T9 | internal/strategy/price_percentile | 全资产；minSampleBars=252；分档 25/75/10/90；numParam helper |
| TASK-008 | T10 | internal/strategy/pe_percentile | 股票+指数；20/80/10/90；Source 解析 method:fallback_reason；**不抽公共基类** |
| TASK-009 | T11 | strategy/ma_crossover + pe_band + dividend_yield | 三包各补 AssetTypes 声明（3 packages 一任务） |
| TASK-010 | T6+T12 | internal/app | TypeIndex/DetectType/assetTypeOf/DetectMarket ^HSI + effectiveStrategies/warnOnce/historyWindowDays |
| TASK-011 | T13 | internal/app | ValuationSource/EPSSource 窄接口 + SetValuationSources + buildFundamental 兜底链 |
| TASK-012 | T14+T15(部分) | cmd/atlas + configs | serve 注册策略 + typed-nil 防护注入 + backtest 注册 price_percentile + config.example + README |

plan T15 其余收尾项归属：理杏仁指数代码核对（无 key 跳过）→ TASK-012 DoD 注明；code-simplifier → Dev 各任务 commit 前已有流程；gitnexus_detect_changes + 全量回归 → QA/终验收。

## 跨任务接口约定（context_from 传递）

- TASK-001 产出：`core.AssetCrypto`、`core.EPSPoint{Date,EPS}`、`Fundamental.PEPercentile float64`（负值=不可用，Source 编码方法）
- TASK-002 产出：`yahoo.FetchEPSHistory(symbol, start, end) ([]core.EPSPoint, error)`（^ 前缀拒绝）、`NewWithBaseURLs(chartURL, epsURL)`
- TASK-003 产出：`collector.AShareIndexSecIDs`、`collector.IsAShareIndex(symbol) bool`、`collector.KnownIndexMarket(symbol) (core.Market, bool)`
- TASK-005 产出：`(*Lixinger).FetchValuationPercentile(symbol string, lookbackYears int) (float64, error)`
- TASK-006 产出：`valuation.PercentileRank`、`valuation.ReconstructPEPercentile`、`ErrInsufficientEPS`（可兜底）/`ErrNonPositiveEPS`（不兜底）、`MinEPSPoints=8`
- TASK-010 产出：`assetTypeOf`、`effectiveStrategies`、`historyWindowDays`、`warnOnce`
- TASK-011 产出：`app.ValuationSource`/`app.EPSSource` 接口 + `SetValuationSources(vs, es)`（nil 容忍）

## 兜底链语义（plan T13，QA 重点）

```
CN 股票/指数、US/HK 指数 → 理杏仁 cvpos 唯一路径（失败→不可用）
US/HK 个股 → Yahoo EPS 重建主路径
  ├─ 成功 → Source="reconstructed"
  ├─ ErrNonPositiveEPS（真实亏损）→ 直接不可用，绝不兜底
  └─ ErrInsufficientEPS / fetch 失败 / epsSrc 未配置 → 理杏仁兜底
       Source="lixinger_cvpos:<fallback_reason>"
```

## 覆盖率基线

- TASK-001（core 纯类型）: coverage_minimum=78（现状 80.0% 踩线留余量）
- TASK-012（cmd/atlas+configs）: coverage_minimum=35（沿用 sprint-001 裁决）
- 其余任务：默认 80
