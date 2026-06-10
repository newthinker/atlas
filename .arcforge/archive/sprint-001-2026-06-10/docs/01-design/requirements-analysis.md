# 需求分析 — ATLAS 优化 Sprint（来源：docs/reviews/2026-06-03-project-status-and-optimization.md）

**分析日期**: 2026-06-10
**规划模式**: ECC 不可用 → 内置 requirement-analysis 降级（单模型 + 代码现状调研）

## 需求清单

| ID | 需求 | 优先级 | 复杂度 | 来源章节 |
|----|------|--------|--------|----------|
| R1 | M4 实盘闭环：paper-trading 模式接线 ExecutionManager，打通信号→下单链路 | P1 | 复杂 | §3.2 / §五.5 |
| R2 | 分析循环并行化：worker pool 并行处理标的 + LLM 仲裁超时控制 | P1 | 中等 | §四 / §五.7 |
| R3 | OHLCV 历史数据 TTL 缓存 | P2 | 简单 | §五.8 |
| R4 | 外部采集器（eastmoney/lixinger/yahoo）测试覆盖提升 | P2 | 中等 | §四 / §五.9 |
| R5 | backtest CLI 接入回测引擎 | P2 | 简单 | §四 / §五.10 |

## 代码现状关键事实（调研结论）

### R1 — M4 实盘链路
- `internal/broker/`：ExecutionManager（`execution.go:103`）、RiskChecker（`risk.go:43`）、PositionTracker（`position.go:21`）已实现，execution_test.go 34 用例覆盖充分。
- **接口断层**：`internal/broker/mock` 实现的是 LegacyBroker 接口，而 ExecutionManager 需要 `types.go:251` 的新 `Broker` 接口（14 方法）→ 现有 mock **不能直接用于接线**。
- `cmd/atlas/serve.go:180-200`：接线代码被注释，等待真实 broker。
- `cmd/atlas/broker.go:109`：FutuBroker TODO（依赖 OpenD SDK，本 Sprint 不做）。
- **信号→执行链路完全缺失**：router.Route() 只通知不触发下单；`api.Dependencies.ExecutionManager` 字段已定义但从未赋值。
- config 已有 `BrokerConfig.Mode`（"paper"/"live"，默认 paper）、ExecutionConfigOpts、RiskConfigOpts（`config.go:126-148`）。

### R2 — 分析循环
- `app.go:196-216` runAnalysisCycle 完全串行：for 循环逐个 `analyzeSymbol`（采集→分析→仲裁→路由）。
- `app.go:319-349` arbitrate → `meta/arbitrator.go:125` `a.llm.Chat(ctx)` 同步阻塞，**无 context.WithTimeout 包装**。
- 并发安全已就绪：strategy.Engine（RWMutex）、selector.SelectForSymbol（纯函数）、router（cooldowns 加锁）、SignalStore（全程 Lock）。
- config 无任何 worker/并发/超时配置项。
- app_test.go 有可复用 mock（mockCollector 带调用计数 / mockStrategy / mockNotifier）。

### R3 — OHLCV 缓存
- Collector 接口：`FetchHistory(symbol, start, end, interval) ([]core.OHLCV, error)`（`interface.go:32`）。
- 重复拉取路径：app 分析循环（每 tick 拉 365 天）、backtest、API GetHistory/GetIndicators（同范围拉两次）、market context。
- 可参考模式：`internal/context/news.go:61` CachedNewsProvider（map + cacheAt + ttl）。

### R4 — 外部采集器
- 现状覆盖：eastmoney 5 测试/503 行（FetchQuote/FetchHistory 等 HTTP 路径全部未测）；lixinger 5 测试/531 行；yahoo 8 测试/275 行（仅符号校验测过）。
- **阻塞点**：三者 client/baseURL 均私有硬编码，无法注入 → 必须先加 `NewWithBaseURL` 类重构（参考 `crypto/binance/binance.go:36` 模式）才能用 httptest。

### R5 — backtest CLI
- `cmd/atlas/backtest.go:62` TODO；CLI 参数已齐（strategy/--symbol/--from/--to）。
- 引擎入口：`backtest.Run(ctx, strat, symbol, start, end) (*Result, error)`（`backtester.go:29`）；`New(provider)` 只需一个 OHLCVProvider。
- 测试中有完整构造示例（`backtester_test.go:63-114`）。

## 范围边界（明确不做）

- ❌ FutuBroker 真实实现（依赖 OpenD SDK，留待后续 Sprint）
- ❌ live 模式下单（本 Sprint 仅 paper 闭环）
- ❌ 执行确认 UI / API endpoint 扩展（ExecutionManager 已有 confirm 队列，UI 后续做）
- ❌ 持久化缓存（仅内存 TTL 缓存）

## 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| 并行化引入数据竞争 | 中 | 调研确认下游组件线程安全；DoD 强制 `-race` 测试 |
| paper broker 行为与真实 broker 偏差 | 低 | 仅验证链路连通性，明确标注模拟语义 |
| cmd/atlas 多任务改同一包 | 中 | 任务依赖链强制串行（scope 互斥） |
| 外部采集器重构破坏现有行为 | 低 | 仅加注入点不改逻辑，现有测试守护 |
