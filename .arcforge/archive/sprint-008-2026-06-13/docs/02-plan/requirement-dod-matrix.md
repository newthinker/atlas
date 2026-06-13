# 需求 ↔ DoD 双向追溯矩阵 — HK qlib bundle

来源：`docs/superpowers/plans/2026-06-13-hk-qlib-bundle.md`（自查覆盖节）

## 正向：需求 → 覆盖任务

| # | 需求 | 任务 | DoD 锚点 |
|---|---|---|---|
| R1 | HK 命名契约 .HK→HK#####（Go） | TASK-001 | functional[0..1] |
| R2 | HK 命名契约（Python 对称） | TASK-002 | functional[0..1] |
| R3 | ^HSI→HSI、^HSCE→HSCEI（双侧） | TASK-001/002 | functional[1] |
| R4 | ^HSTECH 不纳入（拒绝） | TASK-001/002/004 | boundary |
| R5 | export-ohlcv --market cn\|hk | TASK-003 | functional[2] |
| R6 | 按 market 取基准（cn=000300.SH/hk=^HSI，缺失硬错误） | TASK-003 | functional[0]+boundary+error_handling |
| R7 | market 选择 watchlist 子集（hk=.HK+^HSI/^HSCE） | TASK-003 | functional[1] |
| R8 | A股零回归（默认 cn） | TASK-003/007 | functional[2]+boundary |
| R9 | watchlist 加 4 ETF + 2 指数 | TASK-004 | functional[0] |
| R10 | Makefile qlib-data-hk → atlas_hk | TASK-005 | functional |
| R11 | 独立 atlas_hk 包（HK 日历） | TASK-005/007 | TASK-007 functional[1] |
| R12 | analyze 识别 HK/CSI 指数 | TASK-006 | functional[0] |
| R13 | 端到端：建 atlas_hk + 分析报告 | TASK-007 | functional[0..2] |
| R14 | 行情走 yahoo（路由就绪，无需改 selector） | 设计已验证 | TASK-007 functional |

→ 无孤儿需求。

## 反向：任务 DoD → 需求

| 任务 | 映射需求 | 凭空 DoD？ |
|---|---|---|
| TASK-001 | R1,R3,R4 | 无 |
| TASK-002 | R2,R3,R4 | 无 |
| TASK-003 | R5,R6,R7,R8 | 无 |
| TASK-004 | R9,R4 | 无 |
| TASK-005 | R10,R11 | 无 |
| TASK-006 | R12 | 无 |
| TASK-007 | R8,R11,R13,R14 | 无 |

→ 无凭空 DoD（覆盖率/编译/语法类来源为团队规范，已注 verify_by）。

## 机器检查结论
孤儿需求：0；凭空 DoD：0。矩阵闭合。
