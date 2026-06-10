# 设计规格 — ATLAS 优化 Sprint

## D1. PaperBroker + M4 接线（R1）

### D1.1 新包 `internal/broker/paper`

实现 `broker.Broker` 接口（types.go:251 的新接口，非 LegacyBroker）的内存模拟券商：

```go
type PaperBroker struct {
    mu        sync.RWMutex
    connected bool
    cash      float64            // 初始资金（构造参数，默认 1,000,000）
    positions map[string]broker.Position
    orders    map[string]broker.Order
    counter   int64              // 订单 ID 序号
    handler   broker.OrderUpdateHandler
}

func New(initialCash float64) *PaperBroker
```

语义：
- `PlaceOrder`：市价单**立即全额成交**（fill price = request.LimitPrice，若为 0 则要求调用方传参；paper 模式下用信号价格）；更新 cash/positions；触发 handler 回调。
- 卖出超过持仓 → 返回错误（不支持做空）。
- `GetBalance`/`GetPositions`/`GetOrder` 等从内存状态返回。
- 全部方法并发安全（RWMutex）。
- `SupportedMarkets()` 返回全市场（US/CNA/HK/CRYPTO）。

### D1.2 App 执行接线点（`internal/app`）

```go
// app.go 新增
type SignalExecutor interface {
    SubmitSignal(ctx context.Context, sig core.Signal) error
}

func (a *App) SetExecutor(e SignalExecutor)   // 加锁，与 SetArbitrator 同模式
```

`analyzeSymbol` 路由阶段之后：若 executor 非 nil，对每个已路由信号调用 `SubmitSignal`；错误记日志不中断循环。

`internal/broker` 增加薄适配把 `core.Signal` 转 `OrderRequest` 并调用 `ExecutionManager.Execute`（方法名以现有 ExecutionManager API 为准，归属 TASK-003 在 cmd/atlas 写适配 struct，避免 broker↔app 循环依赖；适配器实现 SignalExecutor）。

### D1.3 serve.go 接线（`cmd/atlas`）

`cfg.Broker.Enabled == true` 时：
- `Provider == "mock"` 或 `Mode == "paper"` → 构造 PaperBroker；`Provider == "futu"` → 维持现状（warning + 不接线）。
- 构造链：PaperBroker → RiskChecker(cfg.Broker.Risk) → PositionTracker → ExecutionManager(cfg.Broker.Execution)。
- 信号适配器实现 `app.SignalExecutor`，注入 `app.SetExecutor`。
- `execManager` 赋值给 `api.Dependencies.ExecutionManager`（字段已存在）。
- 启动日志明确打印 `broker mode=paper provider=...`。

## D2. 分析循环并行化 + 仲裁超时（R2）

### D2.1 新配置（`internal/config`）

```yaml
analysis:
  workers: 4            # 并行 worker 数，<=1 表示串行（向后兼容）
meta:
  arbitrator:
    timeout: 15s        # LLM 仲裁单次调用超时
collector:
  cache:
    enabled: true
    ttl: 5m             # OHLCV 缓存 TTL（R3 用）
```

- `AnalysisConfig{Workers int}`；默认 4。
- `ArbitratorConfig` 增加 `Timeout time.Duration`；默认 15s。
- `CacheConfig{Enabled bool; TTL time.Duration}`；默认 enabled=true, ttl=5m。

### D2.2 worker pool（`internal/app`）

`runAnalysisCycle`：
- `workers <= 1` → 保持现有串行路径（行为完全兼容）。
- `workers > 1` → 信号量模式（`golang.org/x/sync/errgroup` + SetLimit 或带缓冲 channel），并行执行 `analyzeSymbol`；ctx 取消时停止派发。
- watchlist 快照仍在 RLock 下复制，循环外无锁。

### D2.3 仲裁超时（`internal/app`）

`arbitrate` 中：`ctx, cancel := context.WithTimeout(ctx, cfg.Meta.Arbitrator.Timeout)`；超时/出错 → 记 warning 日志并**降级返回原信号**（现有行为已是出错回退，只补超时包装）。

## D3. OHLCV TTL 缓存（R3）

新文件 `internal/collector/cache.go`：

```go
type CachedCollector struct {
    collector.Collector            // 嵌入，透传其余方法
    ttl     time.Duration
    mu      sync.RWMutex
    entries map[string]cacheEntry  // key = symbol|start|end|interval（时间截断到分钟）
}

func NewCached(c Collector, ttl time.Duration) *CachedCollector
```

- 仅缓存 `FetchHistory`；命中且未过期直接返回（**返回副本防止调用方修改底层数组**）。
- 并发安全；简单容量上限（如 256 entries，超过时淘汰最旧）防泄漏。
- 接线（cmd/atlas）：`cfg.Collector.Cache.Enabled` 时在 collector 注册处用 `NewCached` 包装。

## D4. 外部采集器可测性 + 测试（R4）

对 eastmoney / lixinger / yahoo 各自：
1. 重构：增加 `NewWithBaseURL(...)`（参考 `crypto/binance` 模式），把硬编码 URL 常量改为实例字段，默认值不变；**不改任何业务逻辑**。
2. 用 `httptest.Server` 写 FetchQuote / FetchHistory 测试：正常响应、HTTP 错误码、畸形 JSON、空数据。
3. 目标：每包覆盖率 ≥ 60%（从 ~5%-25% 提升；80% 对纯 HTTP 适配包性价比低，hook 门禁按 changed-package 80% 要求 —— 若达不到在任务中说明并以 60% 为 DoD 基线，由 Test Agent 按 DoD 验收）。

注：eastmoney 有 4 个 baseURL 常量与 lixinger fallback，重构注意保持 fallback 行为。

**空数据/业务错误语义（统一钉死，避免实现分歧）**：
- API 正常返回但数据为空（空列表/空 result）→ 返回空 slice + nil error（合法的「无数据」）。
- HTTP 200 但响应体携带业务错误（eastmoney data=null/rc 非成功、lixinger code 非成功）→ 返回 error。
- HTTP 非 200 / 畸形 JSON → 返回 error。

## D5. backtest CLI 接引擎（R5）

`cmd/atlas/backtest.go` TODO 处：
1. 构造 collector registry（复用 serve.go 的注册逻辑中可独立调用的部分；如不可复用则按 symbol 用 `collector.SelectForSymbol` 选择）。
2. 按名字从 strategy registry 取策略，不存在 → 友好错误 + 列出可用策略。
3. `backtest.New(provider).Run(ctx, strat, symbol, from, to)`。
4. 输出 Result：信号数、交易数、Stats（胜率/收益等字段按 types.go 实际定义），表格化 stdout。

## 模块依赖关系

```
config (D2.1) ──> app 并行化 (D2.2/D2.3)
                  app 执行接线点 (D1.2) ──┐
broker/paper (D1.1) ─────────────────────┼──> cmd/atlas serve 接线 (D1.3)
collector/cache (D3) ────────────────────┼──> cmd/atlas 缓存接线
                                          └──> cmd/atlas backtest (D5)
eastmoney / lixinger / yahoo (D4) — 相互独立、无上游依赖
```
