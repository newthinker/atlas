# 需求分析 — 落地核查优化轮

> Sprint: 017（2026-07-02 启动）
> 需求来源: `docs/superpowers/specs/2026-07-02-audit-optimization-round-design.md`
> 设计状态: 上游已通过 brainstorming 逐节评审确认，本轮**不重复设计**，直接进入任务拆分。
> ECC 不可用（capabilities.ecc=false）→ 按降级路径执行；设计已确认，无需再走 brainstorming。

## 1. 目标

用三个独立 PR（清理加固 → alert 接线 → 信号持久化）消化落地核查报告的全部 5 个非 sidecar 优化项，每波可独立合并、独立回滚。ML sidecar 单独立项，不在本轮。

## 2. 功能模块识别

| 模块 | 内容 | 复杂度 | 涉及包 |
|---|---|---|---|
| M-1a FutuBroker 清理 | 删 futu case/FutuConfig/示例配置；Provider 缺省 futu→mock；m4 文档撤回标注 | 简单 | cmd/atlas, internal/config, configs, docs |
| M-1b 集成测试隔离 | okx/coingecko/binance 真实 API 测试拆到 `//go:build integration` 文件；Makefile 加 test-integration | 简单 | internal/collector/crypto/{okx,coingecko,binance}, Makefile |
| M-1c 文档治理 | runbook 补 analysis LaunchAgent；架构文档 superseded 注记；crypto 设计三处"未实现"标注 | 简单 | docs |
| M-2a 指标快照 | Registry.Snapshot() map[string]float64，prometheus 封闭在 metrics 包内 | 中等 | internal/metrics |
| M-2b 派生指标 | http_error_rate（delta）、signals_24h（Count） | 简单 | cmd/atlas（runner 内） |
| M-2c notifier 适配 + serve 装配 | alert.Notifier ← notifier.Notifier 适配器（SendText 直发/系统信号回退）；serve 起评估循环 | 中等 | internal/notifier, cmd/atlas |
| M-3a sqlite SignalStore | NewSQLiteStore + WAL + 单表 schema + ListFilter 直译 | 中等 | internal/storage/signal |
| M-3b 配置与装配 | storage.signals 节（默认 sqlite）；serve 按配置构造；打开失败快速失败 | 简单 | internal/config, cmd/atlas |
| M-3c 契约测试 | store_contract_test.go 同套用例跑 memory+sqlite | 中等 | internal/storage/signal |

## 3. 代码落点核实（2026-07-02 实测）

- `cmd/atlas/broker.go:108` 存在 `case "futu"`；`internal/config/config.go:356` Provider 缺省 `"futu"` ✓
- `internal/alert/` evaluator+rules+单测齐备，serve.go 未实例化 ✓（死配置确认）
- `internal/config/config.go:240-248` AlertsConfig/AlertRule 已存在 ✓
- `cmd/atlas/serve.go:86` `signalstore.NewMemoryStore(...)` 固定内存 ✓
- `internal/notifier/` 有 registry + telegram/email/webhook 三实现 ✓
- `go.mod` `modernc.org/sqlite v1.38.2` 已锁定 ✓（GOTOOLCHAIN=local 约束，不升版）
- `Makefile:123` 只有 `test` target，无 test-integration ✓

## 4. 依赖关系与交付组织

- PR#1（Wave1 清理加固）三任务互不依赖，可并行。
- PR#2（alert 接线）：snapshot 与 notifier 适配可并行；serve 装配依赖两者。
- PR#3（sqlite）：store 实现+契约测试先行；配置装配依赖它。
- PR 之间逻辑独立，但 serve.go / config.go / config.example.yaml 在 PR#2、PR#3 均被触碰；
  **单工作树串行执行三波**（Wave2/3 理论可并行，实操串行避免分支/文件冲突）。

## 5. 明确不做（边界，防止 Dev 越界）

不做 retention 清理、不做内存 store 迁移、不动 track_record provider、
不补 crypto 的 SupportsSymbol/sentinel/可重试 fallback（仅文档标注）、不做 ML sidecar、
不做 `up` 恒 1 指标、不做 analysis_failures_1h 专门指标。
