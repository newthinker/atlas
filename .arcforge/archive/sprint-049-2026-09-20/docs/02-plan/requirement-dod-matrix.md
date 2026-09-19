# 需求 ↔ DoD 追溯矩阵（M4-crisis，2026-09-18）

| 需求 / 约束 | 覆盖任务 |
|---|---|
| C1 fetched_at NOT NULL | TASK-001, TASK-002 |
| C2 RENAME 后索引须重建 | TASK-002 |
| C3 旧表 _v1 保留 | TASK-002 |
| C4 AsOfQuery 不能建视图（两个形态） | TASK-006 |
| C5 五个读取点 WHERE/ORDER/LIMIT 不动 | TASK-004 |
| C6 裸 INSERT，冲突响亮失败 | TASK-005 |
| C7 SeriesReader/评估/RenderReplayHTML 不动 | TASK-006, TASK-007 |
| C8 t 前无行 ⇒ 排除而非回退最早 | TASK-006, TASK-007 |
| C9 迁移是显式命令，不在 NewStore 自动跑 | TASK-002, TASK-003 |
| 判据一 视图 31092 行且双向 EXCEPT 为 0 | TASK-002, TASK-008 |
| 判据二 迁移前后 replay 逐行相同 | TASK-008 |
| 判据三 --as-of now == 不带 | TASK-008 |
| 判据四 --as-of 早于全部 ⇒ 空序列 | TASK-006, TASK-007, TASK-008 |
| 判据五 人造修订 | HUMAN |
| 判据六 同键异值响亮失败 | TASK-005, HUMAN |
| 判据七 真实采集后行数增加 | HUMAN |
| R1 EvalDates 并入读改造 + 包内守卫 | TASK-004 |
| R2 agent 在人类在场时执行迁移与判据一～四 | TASK-008 |
| 部署顺序 AD-11（先迁移后部署） | TASK-008 |

## 机器检查
- 孤儿需求（无任务覆盖）：0
- 凭空任务（不对应任何需求）：[]
- 悬空引用（矩阵引用了不存在的任务）：[]
- `HUMAN` = 判据五/六/七的生产实测，人类裁决 R2 明确留给人类，不是遗漏。

## 任务规模与依赖
| 任务 | DoD | test | review | manual | wave | 依赖 |
|---|---|---|---|---|---|---|
| TASK-001 | 8 | 7 | 1 | 0 | 1 | — |
| TASK-002 | 8 | 8 | 0 | 0 | 2 | TASK-001 |
| TASK-003 | 8 | 6 | 2 | 0 | 3 | TASK-002 |
| TASK-004 | 8 | 7 | 1 | 0 | 4 | TASK-001,TASK-003 |
| TASK-005 | 8 | 8 | 0 | 0 | 5 | TASK-001,TASK-004 |
| TASK-006 | 8 | 7 | 1 | 0 | 6 | TASK-004,TASK-005 |
| TASK-007 | 7 | 6 | 1 | 0 | 7 | TASK-006 |
| TASK-008 | 8 | 0 | 3 | 5 | 8 | TASK-001,TASK-002,TASK-003,TASK-004,TASK-005,TASK-006,TASK-007 |
