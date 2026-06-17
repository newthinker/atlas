# 需求 ↔ DoD 双向追溯矩阵

## 正向：每条需求 → 覆盖它的 DoD 任务

| 需求 | 描述 | 覆盖任务 / DoD 条目 | 状态 |
|------|------|---------------------|------|
| R1 | 一轮多条信号汇成一条消息 | TASK-003(flush 一次 batch n==2) + TASK-004(cycle 末 flush) | ✅ |
| R2 | 按买入/卖出/持有分组、顺序固定 | TASK-002(GroupsAndAligns 段顺序 + EmptyAndHold) | ✅ |
| R3 | 组内置信度降序 | TASK-002(600519.SH 在 0700.HK 之前) | ✅ |
| R4 | 含中文名列对齐 | TASK-001(displayWidth/padRight CJK) + TASK-002(贵州茅台 对齐) | ✅ |
| R5 | batch_notify:false 回退逐条即时发 | TASK-003(NonBatch_NotifiesImmediately) + TASK-004(false 覆盖默认) | ✅ |
| R6 | 执行/冷却/存储语义不变 | TASK-003(既有路由/冷却/门测试全过) | ✅ |
| R7 | 空轮不发消息 | TASK-002(formatBatch(nil)=="") + TASK-003(Flush_EmptyIsNoop) | ✅ |
| R8 | 默认开启 batch_notify | TASK-004(Load 默认 true) | ✅ |
| 约束:无新依赖 | 不引第三方 runewidth | TASK-001(isWide 自实现) | ✅ |
| 约束:覆盖率 | 变更包 ≥80% | TASK-001/002/003/004 non_functional | ✅ |
| 约束:构建 | go build ./... | TASK-004 non_functional | ✅ |

**孤儿需求检查（无 DoD 覆盖的需求）：无。** R1–R8 + 全部约束均有对应 DoD。

## 反向：每个 DoD 任务 → 它服务的需求

| 任务 | 服务需求 | 凭空 DoD？ |
|------|---------|-----------|
| TASK-001 | R4, 无新依赖约束, 覆盖率 | 否 |
| TASK-002 | R2, R3, R4, R7, 覆盖率 | 否 |
| TASK-003 | R1, R5, R6, R7, 覆盖率 | 否 |
| TASK-004 | R1, R5, R8, 构建约束, 覆盖率 | 否 |

**凭空 DoD 检查（不对应任何需求的 DoD）：无。** 每个任务的 DoD 均可回溯到至少一条需求。

## 机器检查结论

- 孤儿需求：0
- 凭空 DoD：0
- 双向闭合：通过
