# 需求 ↔ DoD 追溯矩阵（M4，2026-09-17）

| 需求 | 覆盖任务 |
|---|---|
| D1 队列指标复用 collector 链路 | TASK-003, TASK-004 |
| D2 队列读不到=告警 | TASK-001, TASK-003, TASK-005 |
| D3 先判队列再唤起 | TASK-006 |
| D4 processing/ 互斥、每次一份 | TASK-006 |
| D5 告警规则（经 R1/R2 修订为五条） | TASK-005 |
| C1 纯文件系统读 | TASK-001 |
| C2 队列与 DB 失败互不牵连 | TASK-003, TASK-005 |
| C3 目录读不到不得退化成零 | TASK-001, TASK-004 |
| C4 队列空零 agent 调用 | TASK-006 |
| C5 不引锁文件 | TASK-006 |
| C6 告警走现成 notifier | TASK-005 |
| C7 导出面登记写口守卫 | TASK-001 |
| C8 plist 代理键不照抄 | TASK-007 |
| 判据一 go test 全绿 + 守卫精确相等 | TASK-001, TASK-002, TASK-003, TASK-004, TASK-005 |
| 判据二～六 生产实测 | HUMAN |
| R1 Snapshot 展开 state label | TASK-002, TASK-003, TASK-005 |
| R2 up gauge | TASK-003, TASK-005 |
| R3 plist 入库 deploy/launchd | TASK-007 |
| R4 只改 config.example.yaml | TASK-005 |

## 机器检查
- 孤儿需求（无任务覆盖）：0 条 []
- 凭空任务（不对应任何需求）：0 个 []
- 悬空引用（矩阵引用了不存在的任务）：0 个 []
- `HUMAN` = 人类裁决 R3 明确划出 sprint 的生产动作，不是遗漏。

## 任务 DoD 规模
| 任务 | DoD 条数 | test | review |
|---|---|---|---|
| TASK-001 | 8 | 6 | 2 |
| TASK-002 | 8 | 7 | 1 |
| TASK-003 | 8 | 7 | 1 |
| TASK-004 | 6 | 5 | 1 |
| TASK-005 | 8 | 6 | 2 |
| TASK-006 | 8 | 0 | 8 |
| TASK-007 | 6 | 0 | 6 |
