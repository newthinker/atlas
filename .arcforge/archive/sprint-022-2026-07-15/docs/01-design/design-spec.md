# 设计规格 — Cassandra 危机回放报告（Sprint 022）

> 权威规格 = `docs/plans/2026-07-15-crisis-replay-report-impl.md`（含逐任务参考实现与测试代码）。
> 本文档只记录跨任务接口契约，Dev 开工前必须通读计划中对应任务的完整章节。

## 架构

- 回放引擎与三个渲染器全部落在 `internal/crisis`（导出、纯函数、DB 读经 `SeriesReader`）。
- 落盘与发送在 cmd 层；telegram 扩展 `SendDocument`，`Sender` 接口不动（cmd 用 `documentSender` 类型断言）。
- 既有 `crisis replay` 子命令重构为调用同一引擎（暖机语义统一，全期窗口黄金对照）。
- 技术栈：Go 零新依赖、html/template + 手工 SVG、modernc.org/sqlite v1.38.2（只读）、cobra、testify。

## 跨任务接口契约

| 符号 | 定义方 | 消费方 |
|---|---|---|
| `type ReplayDay struct { Date string; Res *DayResult; StateDays int }` | TASK-001 | 002/003/004/006 |
| `func ReplayRange(cfg *Config, sr SeriesReader, from, to string) ([]ReplayDay, error)` | TASK-001 | 006 |
| `func ReplayReport(cfg *Config, form string, day ReplayDay, prev *ReplayDay, sr SeriesReader) (string, error)` | TASK-002 | 006 |
| `func RenderReplaySummary(cfg *Config, days []ReplayDay) string` + `replayFooter` 常量 | TASK-003 | 006 |
| `hasFreshReading(s Status) bool`（包内） | TASK-003 | 004（月度表读数口径） |
| `func RenderReplayHTML(cfg *Config, days []ReplayDay, sr SeriesReader) (string, error)` | TASK-004 | 006 |
| `func (t *Telegram) SendDocument(path, caption string) error` | TASK-005 | 006（类型断言签名一致） |
| 测试 helper `mkReplayDay(...)`（`replay_report_test.go`） | TASK-002 | 003/004 同包复用 |
| 既有：`EvalDay`/`NewMemHistory`/`SeriesReader`/`renderDaily`/`renderMonthly`/`formatReading`/`isColor`/`Store.EvalDates`/`openCrisisStore`/`buildCrisisSender`/`crisisEvalDeps` 模式 | 存量代码 | 各任务 |

## 关键语义（易错点摘录）

- 暖机：交易日历 = vix 观测日（`Store.EvalDates` 同口径）；StateDays 转移日=1、暖机期计入；窗口切片 `o.Date >= from`。
- 极值方向：`t10y2y`/`usdjpy` 取最小、其余取最大；只统计 `isColor(status) || StatusSuppressed` 的日。
- 量控：启动前用 `Store.EvalDates` 日历算条数，`--send` 且 >31 报错（字面值固定）；monthly 报告日 = 全库日历每月首交易日且落在窗口内。
- 尾注：回放家族用 `replayFooter`（含「历史回放」限定）；daily/monthly 文本报告保留消息家族既有页脚。
- HTML：无外链、`prefers-color-scheme`、表格 `overflow-x` 容器、色板 绿#16a34a 黄#eab308 橙#f97316 红#dc2626、阈值读 cfg、usdjpy 无水平阈值线、sofr_effr 无数据注记「该指标自 2018-04 起才有数据」。
- SendDocument：caption 按 rune 截 1024；错误 `telegram: API error (status %d)`；复用 `t.client`。
