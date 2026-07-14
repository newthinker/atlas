# 设计规格 — 落地核查优化轮

> 权威设计文档: `docs/superpowers/specs/2026-07-02-audit-optimization-round-design.md`（已确认，本文件只做补充与执行细化，冲突时以权威文档为准）

## Wave 1 — 清理加固（PR #1，零功能影响）

### 2a FutuBroker 清理
- `cmd/atlas/broker.go` 删 `case "futu"`（现 :108），futu 落 default → `unknown broker provider: futu`。
- `internal/config/config.go` 删 `FutuConfig` 与 `BrokerConfig.Futu`；`Broker.Provider` 缺省 `"futu"`→`"mock"`（:356）。
- `configs/config.example.yaml` 删 futu 段；`docs/plans/2026-01-01-m4-live-trading-design.md` 头部加撤回标注。
- 兼容性：老配置 `futu:` 段被 viper 静默忽略，不崩。

### 2b 集成测试隔离
- okx/coingecko/binance 三包内打真实 API 的 `Test*_Integration` → 各自 `*_integration_test.go`，头部 `//go:build integration`；httptest 单测不动。
- Makefile 新增 `test-integration`: `go test -tags integration ./internal/collector/crypto/...`。

### 2c 文档治理
- runbook 补 analysis LaunchAgent（plist / trigger-analysis.sh / services.sh 子命令）。
- 架构设计文档头部 superseded 注记（六项 + 现实替代）。
- crypto 设计文档三处"未实现，实施时裁剪"标注。

## Wave 2 — alert.Evaluator 接线（PR #2）

### 3a `internal/metrics/snapshot.go`
- `Registry.Snapshot() map[string]float64`：遍历 `Gather()`；gauge/counter 取值（多 label 求和）；histogram 展开 `_count`/`_sum`。prometheus 类型封闭在 metrics 包内。

### 3b 派生指标（serve 内 runner 计算）
- `http_error_rate`：保留上次快照，5xx 与总请求**增量** delta 算区间错误率。
- `signals_24h`：`SignalStore.Count(from=now-24h)`。
- 裁剪：不做 `up`、不专门造 analysis_failures_1h。

### 3c notifier 适配 + serve 装配
- 适配器实现 alert 侧 Notifier 接口，底层为 notifier.Notifier：
  - 底层实现 `interface{ SendText(string) error }` → 直发 `[SEVERITY] name: message` 文本；telegram 补公开 `SendText`（内部走已有 sendRaw，约 3 行）。
  - 否则回退包装 `core.Signal{Symbol:"SYSTEM", Strategy:"alert", Reason:告警文本}` 走 `Send`。email/webhook 走回退，零改动。
  - 依赖方向约束：**internal/alert 不得 import internal/notifier**（适配器放 notifier 侧或 cmd/atlas，由 Dev 定，记入 discovery）。
- `cmd/atlas/serve.go`：`cfg.Alerts.Enabled` 时构造 Evaluator，goroutine 按 `check_interval` 循环 `Snapshot() → 补派生指标 → SetMetrics → EvaluateAll(rules)`，随 app ctx 优雅退出。
- `config.AlertRule` ↔ `alert.Rule` 一一映射；config.example.yaml 补示例规则（`http_error_rate > 0.1` + `for: 5m`）。

### 3d 测试
snapshot 单测（假 registry 四场景）；delta 派生指标单测；适配器双路径单测；serve 装配用 `observer.ObservedLogs`。

## Wave 3 — 信号 sqlite 持久化（PR #3）

### 4a `internal/storage/signal/sqlite.go`
- `NewSQLiteStore(path)`：modernc.org/sqlite **锁定 v1.38.2**；`journal_mode=WAL` + `busy_timeout`；单写多读靠 database/sql 连接池。
- 单表 schema（权威文档 §4a 原文 SQL，CREATE IF NOT EXISTS，两索引，无迁移框架）。
- ID 生成沿用 `sig_<unixnano>_<counter>`；List/Count 把 ListFilter 直译 WHERE；排序分页语义以内存实现为基准。

### 4b 配置与装配
- config 新增 `storage.signals`：`backend: memory|sqlite`（**默认 sqlite**）、`path`（默认 `data/signals.db`）。
- serve.go 按配置构造（替换 :86 固定 memory）；sqlite 打开失败**启动即退**，不降级内存。
- **行为变化**（缺省从内存变 sqlite）在 PR 描述与 config.example.yaml 显著说明。

### 4c 契约测试
`store_contract_test.go` 表驱动，同套用例跑 memory 与 sqlite：Save 赋 ID、GetByID、filter 组合、排序、分页、Count、metadata JSON 往返；sqlite 用 `t.TempDir()`。

## 整体验收（对应权威文档 §6）
- 每 PR `go test ./...` 全绿；Wave1 后不联网、确定性。
- `make test-integration` 手动可跑（限流性失败不进门禁）。
- Wave2 后：配置规则+触发条件 → telegram 收到 `[SEVERITY] name: message`。
- Wave3 后：重启进程 `/api/v1/signals` 仍查到旧信号。
- 每波提交前 `gitnexus_detect_changes()` 确认改动范围。
