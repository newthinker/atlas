# 需求分析 — Hestia 队列可观测与自动触发（2026-09-17）

需求原文：`hestia/docs/superpowers/plans/2026-09-17-hestia-queue-observability-and-trigger.md`
（设计在会话中经人批准，D1–D5、C1–C8、验收判据一～六）

## 功能模块
| 模块 | 需求 | 任务 |
|---|---|---|
| 队列健康度纯函数 | D1/D2、C1/C3 | TASK-001 |
| 指标快照的 state 展开（新增，裁决 R1） | D5 规则可求值 | TASK-002 |
| collector 队列指标 + up gauge | D1、C2、裁决 R2 | TASK-003 |
| serve 接线 | 计划 TASK-003 | TASK-004 |
| 告警规则（五条） | D5、C6、裁决 R1/R2 | TASK-005 |
| 触发脚本 | D3/D4、C4/C5 | TASK-006 |
| plist 入库 | C8、裁决 R3 | TASK-007 |

## Leader 核实后发现的需求缺陷（已由人类裁决）
1. **R1 规则语法**：`internal/alert/rules.go:28` 只认 `^(\w+)\s*op\s*数字$`；`snapshot.go` 跨 label 求和 ⇒ 计划的
   `hestia_queue_items{state="failed"} > 0` 与 `a > 0 or b > 0` 会恒 false。→ Snapshot 展开 `state` label。
2. **R2 累计计数器**：`*_errors_total > 0` 一次瞬时失败后恒真至重启。→ `hestia_db_up` / `hestia_queue_up` gauge。
3. **R3 范围**：plist 进 `deploy/launchd/`；生产侧动作（runtime config、launchctl、判据二～六、CONTRACTS）归人类。
4. **R4**：`configs/config.yaml` 未被 git 跟踪，仓库侧只改 `config.example.yaml`。
5. 计划 TASK-003 的「queue.dir 为空 ⇒ 跳过」不可达：`internal/hestia/config.go:257` 拒绝空值。

## 不在本 sprint（人类执行）
验收判据二～六的生产实测、runtime `configs/config.yaml` 追加规则、`install-services.sh` 实际装载、`internal/hestia/CONTRACTS.md` 登记。
