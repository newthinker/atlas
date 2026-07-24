# M1.5 设计规格(摘要)

> 权威来源: specs/2026-07-24-prism-akshare-source-design.md(决策记录/口径取舍)与
> plans/2026-07-24-prism-akshare-source.md(代码级规格)。Dev 开工必读计划对应 Task 节。

## 跨任务契约(不得漂移)

- `config`: PrismInstrument.FallbackSource(`fallback_source`);PrismConfig.AkshareBaseURL(`akshare_base_url`,默认 http://127.0.0.1:8180)(T1 → T4/5/6/7)
- `akshare`: ValuationPoint{Date, PETTM, PB, PSTTM};New(baseURL);FetchStockValuationSeries(symbol, market, start, end);FetchIndexValuationSeries(symbol, start, end)(T2/3 → T4)
- `prism`: Store 接口 + Series(symbol, from);AkshareClient 两方法;Report{Refreshed, Failed, Degraded};Refresh(cfg, store, lix, us, ak, now) 六参;incrementalStart 提取复用(T4 → T5/6)
- Degraded 元素: "SYMBOL: lixinger failed (<原因>), akshare fallback ok";兜底成功计 Refreshed
- CLI 告警: Failed 或 Degraded 非空即发;exit 语义不变

## 数据流

1. akshare 公司/指数: 增量(latest+1,日历日守卫)→ 拉取 → 读回全历史合并(同日新值覆盖,剔 NaN PE)→ RollingPercentile(5/10, 252)→ 仅写新点。
2. 指数降级链: lixinger 失败且 fallback_source=akshare → 当场 refreshAkshare;成功记 Degraded;双败 Failed 含两原因。
3. store/API/Web 零改动。
