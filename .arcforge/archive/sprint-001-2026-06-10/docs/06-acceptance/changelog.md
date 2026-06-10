# Changelog — ATLAS 优化 Sprint（2026-06-10）

## 新功能

- **M4 paper-trading 闭环**（R1）：新增 `internal/broker/paper` PaperBroker（内存模拟券商，立即成交、资金/持仓演化、并发安全）；App 新增 `SignalExecutor` 接线点；serve.go 在 `broker.enabled=true && mode=paper` 时接通 信号→风控→下单→持仓 全链路并注入 API Dependencies（b18e81c, 5616579, cf27ec8）
- **分析循环并行化**（R2）：`analysis.workers` 配置（默认 4）worker pool 并行处理 watchlist；LLM 仲裁 `meta.arbitrator.timeout`（默认 15s）超时降级返回原信号；单标的失败/panic 隔离（9513908）
- **OHLCV TTL 缓存**（R3）：`internal/collector` CachedCollector 装饰器（TTL 默认 5m、容量 256、副本语义、错误不缓存），serve 按 `collector.cache.enabled` 接线（edda8e7, 76c1d87）
- **backtest CLI 接引擎**（R5）：`atlas backtest <strategy> --symbol --from --to` 真实运行回测并输出统计（d27ca9d）

## 测试与可测性（R4）

- eastmoney/lixinger/yahoo 采集器支持 baseURL 注入（NewWithBaseURL 模式），httptest 全套测试，覆盖率提升至 80%+（67818c0, cb42348, 9c5fc3b）

## 缺陷修复

- ExecutionManager 市价单未携带 Price 导致 paper 单永被拒（含于 cf27ec8）
- ma_crossover 信号不含价格导致生产执行链路惰性（784ed71）
- cooldown 抑制的信号仍会提交执行（16d52a8，Route 签名变更：返回 `(routed bool, err error)`）
- broker 启用但漏配 execution.mode 时静默不下单：Load 补默认 confirm（cc2f0ff）
- eastmoney/lixinger HTTP 非 200 + 合法 JSON 被当成功：10 条 fetch 路径加 StatusCode 守卫（c18c2eb, cfcdee1）

## 配置变更（均有默认值，向后兼容）

```yaml
analysis:
  workers: 4              # 新增；<=1 退化串行
meta:
  arbitrator:
    timeout: 15s          # 新增
collector:
  cache:
    enabled: true         # 新增
    ttl: 5m
broker:
  execution:
    mode: confirm         # 漏配时现在自动取 confirm
```

## API/接口变更

- `router.Route` 签名：`error` → `(routed bool, err error)`（内部接口，唯一调用方已同步）
- 新增 `app.SignalExecutor` 接口与 `App.SetExecutor`
- 新增 `collector.NewCached(c, ttl)`、`broker/paper.New(initialCash)`
