# 需求 ↔ DoD 追溯矩阵 — digest PE% 列

## 正向
| 需求 | 覆盖任务 / DoD | 状态 |
|------|---------------|------|
| R1 末列 PE% | TASK-001 functional(表头含 PE% 且末列) | ✅ |
| R2 每个有 PE 行都填(跨策略) | TASK-002 functional(每条信号盖键) + TASK-001(渲染) | ✅ |
| R3 无 PE 留空 | TASK-001 boundary(无键空串) + TASK-002 boundary(nil/-1 不盖) | ✅ |
| R4 不影响 router 门控 | TASK-002 non_functional(router 零回归) + 展示专用键 | ✅ |
| R5 CJK 对齐/末列无尾随空格 | TASK-001 functional/boundary | ✅ |
| 约束 覆盖率/构建 | TASK-001/002 non_functional | ✅ |

孤儿需求：0。

## 反向
| 任务 | 服务需求 | 凭空 DoD？ |
|------|---------|-----------|
| TASK-001 | R1,R3,R5,覆盖率 | 否 |
| TASK-002 | R2,R3,R4,构建/覆盖率 | 否 |

凭空 DoD：0。双向闭合通过。

## 键名一致性检查（关键）
生产端 TASK-002 写 `pe_percentile_display`(float64) ↔ 消费端 TASK-001 读 `pe_percentile_display`(float64)。两任务描述均显式锁定此契约，避免并行各自臆测。
