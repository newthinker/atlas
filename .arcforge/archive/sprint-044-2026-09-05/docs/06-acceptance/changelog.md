# Changelog · Sprint M1.5（Sprint 044）

基线 `037d1eb` → master `c18bf11`（代码 28 文件 +1883/−43，另 W1 返工 `health.go`/`health_test.go` +87/−22、CONTRACTS §C +14）

## Added
- `internal/hestia`：`hestia_runs` 表（`runsDDL`，13 列 + 时间索引）、`RunOutcome` 五常量、`Run`、`Store.RecordRun`（第二个写方法，登记进两条写口守卫）、`Store.RecentRuns`、`HealthSummary(ctx, Querier)`、`RenderStatus` 的 `runs` 段。
- `internal/hestia/ingest.go`：`Ingest` 逐候选写运行表，零行记 `no_new` 心跳；`ingestOne` 返回 `runResult`；`RecordRun` 失败不影响已入库行（表级触发器测试钉住）。
- `internal/metrics/hestia_collector.go`：`HestiaCollector` 九指标（`hestia_last_run_timestamp` / `hestia_last_ingest_timestamp` / `hestia_hours_since_last_run` / `hestia_hours_since_last_ingest` / `hestia_runs_total{outcome}` / `hestia_validation_blocked_total{check_id}` / `hestia_pending_review` / `hestia_notify_failures_total` / `hestia_collect_errors_total`）；空表不输出时间戳类；出错只计 `collect_errors`。
- `internal/alert`：`Rule.Cooldown`，未写退回全局 5 分钟。
- `internal/config`：`AlertRule.Cooldown`、`HestiaConfig{ConfigPath}`、`Config.Hestia`。
- `cmd/atlas`：`buildHestiaHealth`（未设跳过 / 装不上即启动失败 / 成功注册）接进 `serve`；`mapRules` 透传 `Cooldown`；`hestia status` 读最近 5 次运行（`runsLimit`）。
- `configs/config.example.yaml`：`hestia_stalled`（>30h，for 10m，cooldown 24h，critical）、`hestia_no_ingest`（>960h，for 1h，cooldown 24h，warning）；`hestia.config_path`。
- `docs/deployment.md`：serve 一行补 `hestia.config_path` 语义。
- `internal/hestia/CONTRACTS.md`：`## Sprint M1.5` §A（A1–A7）、§B、§C（QA 挂账 C1–C5 + M6 口径订正）。

## Fixed（QA）
- W1：`HealthSummary` 两段 GROUP BY 经 `groupCount` 检查 `rows.Err()`，ctx 中断不再返回部分计数（`53f1412`）。

## Unchanged（守卫钉住）
- `parse.go` / `extract.go` / `validate.go` / `fields.go` diff 为空；`Save` 函数体不动；`go.mod` / `go.sum` 不动。

## Not delivered（结转）
- 需求 TASK-009 投递与验收（前置首期验收 2026-09-09～09-15）；未跑 `deploy.sh`。
