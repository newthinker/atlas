# Prism M1 设计规格(摘要)

> 权威来源:`docs/superpowers/plans/2026-07-23-prism-m1.md`(含完整代码级规格)与
> `docs/prism/atlas_prism_design.md` v2。本文件只记跨任务契约,Dev 开工必读原计划对应 Task 节。

## 架构

Prism 融入 Atlas Go 单体:
`internal/storage/prism`(SQLite) ← `internal/prism`(刷新编排) ← `cmd/atlas prism refresh`(launchd 每日 08:30)
`internal/api/handler/api/prism.go`(JSON API) + `internal/api/handler/web`(HTMX + ECharts 页面) ← server.go 接线

## 跨任务接口契约(签名必须与计划一致,不得漂移)

- `config.PrismConfig{Enabled, DBPath, LookbackYears, USLookbackYears, LowPct, HighPct, Instruments}` + `ApplyDefaults()`;`PrismInstrument{Symbol,Name,Type,Market,Group,Source}`(Task 1 → 5/7/8/9)
- `prismstore`:`Open/Close/UpsertInstrument/LatestDate/UpsertValuations/Board/Series` + 类型 `Instrument/ValuationRow/BoardRow/SeriesData`;Task 8 补 `ErrNotFound`(Task 2 → 5/6/8/9)
- `lixinger.ValuationPoint{Date, PETTM, PB, PSTTM, Pctl5Y, Pctl10Y}`;`FetchValuationSeries(symbol, start, end)`(Task 3 → 5)
- `valuation.ReconstructPESeries(closes, eps)` + `RollingPercentile(dates, values, years, minPoints)`;重构提取 `alignPE`(Task 4 → 6)
- `prism.Store/LixingerClient/USClient` 接口 + `Report{Refreshed, Failed}` + `Refresh(cfg, store, lix, us, now)`(Task 5/6 → 7)
- HTTP:`GET /api/prism/board`、`GET /api/prism/series?symbol=&window=1y|3y|5y|max`;页面 `GET /prism/board`、`GET /prism/detail/{symbol}`、`GET /static/echarts.min.js`(Task 8/9)

## 数据流

1. 理杏仁路径:LatestDate → 增量区间 [latest+1, now](空则回填 lookback_years)→ FetchValuationSeries → UpsertValuations(幂等)。
2. 美股引擎路径:FetchEPSHistory(start-1y) + FetchHistory → ReconstructPESeries(阶梯对齐,跳 EPS≤0)→ RollingPercentile(5y, minPoints=252) → 整段 upsert;PB/PSTTM/Pctl10Y = NaN。
3. 读路径:Board() 每标的最新行 + status(10y 优先、缺用 5y;<low 低估、>high 高估、NaN→na)。
