# 设计说明（指针）

本 sprint 的设计即人类已审阅的 spec，不另行重写（AD-1）：

- spec：`docs/superpowers/specs/2026-10-20-pricewatch-design.md`
- 实现计划（含接口签名与测试下限）：`docs/superpowers/plans/2026-10-20-pricewatch.md`
- Leader 增量与约定：`.arcforge/docs/01-design/architecture-decisions.md`

## 包结构（计划 File Structure，Leader 调整见 AD-7）

| 文件 | 任务 |
|---|---|
| `internal/pricewatch/rule.go`（含 `NotifyPolicy`/`RuleCfg`/`Rule`/`ParseRule`）、`state.go` | TASK-001 |
| `internal/pricewatch/config.go`、`testdata/valid.yaml`、`configs/pricewatch.yaml` | TASK-002 |
| `internal/pricewatch/store.go` | TASK-003 |
| `internal/pricewatch/sync.go` | TASK-004 |
| `internal/pricewatch/window.go` | TASK-010 |
| `internal/pricewatch/health.go` | TASK-011 |
| `internal/pricewatch/message.go` | TASK-005 |
| `internal/pricewatch/advisor.go` | TASK-006 |
| `internal/pricewatch/engine.go` | TASK-007 |
| `internal/pricewatch/runner.go` | TASK-008 |
| `cmd/atlas/pricewatch.go`、`deploy/launchd/com.newthinker.atlas.pricewatch.plist` | TASK-009 |

## 依赖图

| 任务 | 依赖 | wave |
|---|---|---|
| TASK-001 规则计算与状态机 | — | 1 |
| TASK-002 配置 | 001 | 2 |
| TASK-003 存储 | 001 | 2 |
| TASK-011 健康追踪器 | 001 | 2 |
| TASK-004 K 线补数 | 001, 003 | 3 |
| TASK-010 窗口统计 | 001, 003 | 3 |
| TASK-005 消息文本 | 001, 002 | 3 |
| TASK-006 AI 分析 | 002, 005 | 4 |
| TASK-007 单资产检测与健康 | 002–006, 010, 011 | 5 |
| TASK-008 调度与心跳 | 007 | 6 |
| TASK-009 子命令与 launchd | 008 | 7 |

调度 `dag`：依赖全部 verified 即派。最大并行度 3（wave 2：002∥003∥011；wave 3：004∥005∥010）。
