# 需求 ↔ DoD 双向追溯矩阵 — Sprint: crisis NORMAL 态周报

> 需求编号引用 01-design/requirements-analysis.md §2（源头：2026-07-23 定稿设计 §1/§3/§4/§5/§6）

## 正向：需求 → DoD

| 需求 | 内容 | 覆盖 DoD |
|---|---|---|
| R1 触发 | 周一→周报；首交易日→月报；撞日只发月报 | TASK-003.functional[0]（分支判定）、functional[1]（撞日 2026-06-01 / 2026-08-03） |
| R2 路由 | SummaryKind 三值枚举；cmd 一处判日、渲染纯函数 | TASK-001.functional[0]（枚举与字段）、functional[1]（Messages 路由） |
| R3 渲染 | NORMAL 复用骨架、去退出进度行、含下次周报行 | TASK-002.functional[0]、functional[1] |
| R4 成本不变量 | NORMAL 周报不拉 Trends；ClearStreak 仅 WATCH∧weekly | TASK-003.functional[2] |
| R5 回归不变量 | WATCH 周报逐字节不变；每日至多 1 条互斥 | TASK-002.boundary[0]（逐字节）、TASK-001.functional[1]（恰 1 条）、TASK-001.boundary[1]（变更优先） |
| R6 边界 | 枚举零值静默；坏日期/周末/BREWING/CRISIS→None；周一假日顺延 | TASK-001.boundary[0]（零值）、TASK-003.boundary[0]（坏日期/周六/非周一）、TASK-003.functional[0]（BREWING/CRISIS→None）。假日顺延=既有「数据未齐空跑」机制（设计 §4 明确零新增逻辑），不设新 DoD，QA 审查确认未引入新日历逻辑即可 |
| R7 文档 | ops 手册 §2/§5 同步 | TASK-004.functional[0]、functional[1] |

**孤儿需求检查：无**（R1–R7 均有 DoD 覆盖；R6 假日顺延为显式声明的零改动项）。

## 反向：DoD → 需求

| DoD | 对应需求 |
|---|---|
| TASK-001.functional[0..1] | R2 |
| TASK-001.boundary[0] | R6；boundary[1] → R5 |
| TASK-001.non_functional[0]（无残留引用+包测试绿） | R2 完整性（重命名完备）+ 质量门禁 |
| TASK-002.functional[0..1] | R3 |
| TASK-002.boundary[0] | R5 |
| TASK-002.non_functional[0] | 质量门禁 |
| TASK-003.functional[0..1] | R1；functional[2] → R4 |
| TASK-003.boundary[0] | R6 |
| TASK-003.non_functional[0] | 质量门禁（build+无残留+两包绿） |
| TASK-004.functional[0..1] | R7；non_functional[0..1] → 终验与全局提交规范（code-simplifier） |

**凭空 DoD 检查：无**（各 non_functional 质量门禁条目对应 config 的 tdd/coverage 门禁与全局提交规范，不属凭空）。
