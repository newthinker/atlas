# 部署说明 — ATLAS 优化 Sprint（2026-06-10）

无 DB 迁移、无外部依赖变更。新增 Go module 依赖：golang.org/x/sync（errgroup）。

## 配置变更（全部有默认值，不改配置文件可直接升级）

| 配置项 | 默认 | 说明 |
|--------|------|------|
| `analysis.workers` | 4 | 分析循环并行度；<=1 退化为旧串行行为 |
| `meta.arbitrator.timeout` | 15s | LLM 仲裁单次调用超时，超时降级返回原信号 |
| `collector.cache.enabled` | true | OHLCV FetchHistory TTL 缓存开关 |
| `collector.cache.ttl` | 5m | 缓存 TTL |
| `broker.execution.mode` | confirm | 漏配时自动取 confirm（原为静默失效） |

## 启用 paper-trading（可选）

```yaml
broker:
  enabled: true
  provider: mock     # 或留 futu + mode: paper
  mode: paper
```

启动日志确认：`broker mode=paper provider=...`。注意：confirm 模式下单仅入队，需经 ExecutionManager 确认接口消费（自动 confirm UI 未在本 Sprint 范围，见 final-report 遗留项 I1）。

## 回滚

- 并行化：`analysis.workers: 1` 即回退串行
- 缓存：`collector.cache.enabled: false`
- paper 交易：`broker.enabled: false`
