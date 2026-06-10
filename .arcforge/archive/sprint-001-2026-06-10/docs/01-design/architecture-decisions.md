# 架构决策记录 — ATLAS 优化 Sprint

## ADR-1: 新建 PaperBroker 而非改造现有 mock

**决策**：在 `internal/broker/paper` 新建实现新 `Broker` 接口的 PaperBroker。
**理由**：现有 `broker/mock` 实现的是 LegacyBroker 接口，与 ExecutionManager 需要的 `types.go:251` 新接口不兼容；mock 返回固定数据，无法维护状态化的资金/头寸演化。改造 mock 会破坏其现有消费者。
**后果**：FutuBroker 落地时 PaperBroker 仍可作为 paper 模式实现保留，不是一次性代码。

## ADR-2: 信号→执行用 SignalExecutor 接口解耦

**决策**：app 包定义 `SignalExecutor` 小接口，cmd/atlas 提供适配器（core.Signal → OrderRequest → ExecutionManager）。
**理由**：避免 app → broker 直接依赖（app 是编排层，broker 是基础设施层，经由 cmd 组装符合现有 Clean Architecture 方向）；接口在消费方定义符合 Go 惯例。
**备选被拒**：router 内触发下单 —— router 职责是通知分发，混入交易执行会让 cooldown/通知失败语义与下单语义纠缠。

## ADR-3: 并行化用 errgroup+SetLimit，保留 workers<=1 串行路径

**决策**：worker 数可配（默认 4），`workers <= 1` 走原串行代码路径。
**理由**：行为向后兼容、回退一键可达；调研确认下游（strategy/selector/router/store）线程安全，风险集中在 app 自身循环。errgroup 是项目可接受的轻量依赖（golang.org/x/sync）。
**后果**：DoD 强制 `-race`。LLM 仲裁随标的并行天然并行化，无需单独异步层（原 P2-8 的「LLM 异步化」由此覆盖）。

## ADR-4: 仲裁超时在 app 层包装，降级返回原信号

**决策**：`arbitrate` 内 `context.WithTimeout`（默认 15s），超时回退原信号。
**理由**：仲裁是增强而非必经路径，失败语义已是「回退原信号」，超时只是补全同一语义；放在 app 层而非 arbitrator 内，保持 meta 包无配置依赖。

## ADR-5: OHLCV 缓存用装饰器模式，仅缓存 FetchHistory

**决策**：`CachedCollector` 包装任意 Collector，cmd 组装时按配置启用。
**理由**：与既有 `CachedNewsProvider` 模式一致；装饰器对调用方透明，5+ 个拉取路径零改动受益。FetchQuote 实时性要求高不缓存。
**细节**：返回副本（防调用方修改共享 slice）；容量上限 256 防内存泄漏。

## ADR-6: 外部采集器只做「注入点重构 + httptest」，不改业务逻辑

**决策**：复制 crypto 采集器的 `NewWithBaseURL` 模式，覆盖率目标 80%（与 TaskCompleted hook 的 `dev_minimum=80` 门禁对齐，避免 DoD 与机制门禁冲突）。
**理由**：HTTP 适配包的主要风险在解析逻辑，httptest 可覆盖 fetch+parse 主路径与错误路径；若个别包确有不可达分支，Dev 通过 `blocked_clarification` 反馈，Leader 届时裁决（调整 DoD 或拆出豁免）。

## ADR-7: FutuBroker 与 live 模式明确出 Sprint 范围

**理由**：依赖富途 OpenD SDK 集成，工作量与风险（真金白银）需要独立评审；paper 闭环先验证链路正确性，正是需求文档「paper-trading 先行」的本意。
