# Changelog — Sprint 017 落地核查优化轮（2026-07-03）

## Added
- `internal/metrics`: `Registry.Snapshot() map[string]float64`——gauge/counter 聚合、histogram `_count`/`_sum`、status 类键 `_2xx/_4xx/_5xx`（数字码与 `"5xx"` 类字符串均识别）（#42, TASK-201）
- `internal/notifier`: `NewAlertAdapter`（alert.Notifier ← notifier.Notifier，SendText 直发/系统信号回退）；telegram `SendText`（纯文本无 parse_mode）（#42, TASK-202）
- `cmd/atlas`: alert 评估循环（`alerts.enabled` 时随 serve 启动，check_interval 周期评估，ctx 优雅退出）；派生指标 `http_error_rate`（增量）与 `signals_24h`（#42, TASK-203）
- `internal/alert`: `Evaluator.SetLogger`；Notify 失败记 Warn、全部失败不进冷却下轮重试（#42, TASK-204）
- `internal/storage/signal`: `NewSQLiteStore`（WAL/busy_timeout/父目录自建/契约测试钉死语义）（#43, TASK-301）
- `internal/config`: `storage.signals` 节（backend: memory|sqlite 默认 sqlite、path 默认 data/signals.db）（#43, TASK-302）
- `Makefile`: `test-integration` target（#41, TASK-102）

## Changed
- **[行为变化]** 信号存储缺省 内存 → sqlite 持久化（重启不丢信号；失败快速退出不降级）（#43）
- **[行为变化]** `broker.provider` 缺省 `futu` → `mock`；`mode: live` 直接报错（paper-only 定格）（#41）
- `MemoryStore.List` 补显式稳定排序 `generated_at ASC, id ASC`（与 sqlite 契约一致）（#43, TASK-301）
- okx/coingecko/binance 的 `Test*_Integration` 移入 `//go:build integration` 文件，默认门禁不联网（#41, TASK-102）
- config.example.yaml：删 futu 段、补 alert 示例规则与 storage.signals 说明（#41/#42/#43）

## Removed
- `FutuConfig` / `BrokerConfig.Futu` / broker.go `case "futu"`（FutuBroker 不实现，2026-07-02 决策）（#41, TASK-101）

## Fixed
- telegram 告警文本含奇数下划线/方括号/反引号时被 Telegram 400 静默拒收（告警路径去 parse_mode）（#42, QA-W1）
- alert Evaluator 吞 Notify 错误且失败仍进 5min 冷却导致告警静默丢失（#42, QA-W2）

## Docs
- runbook 补记 analysis LaunchAgent；架构设计文档 superseded 注记（六项）；crypto 设计三处"未实现，实施时裁剪"标注；m4 live-trading 设计撤回标注（#41, TASK-103/101）
