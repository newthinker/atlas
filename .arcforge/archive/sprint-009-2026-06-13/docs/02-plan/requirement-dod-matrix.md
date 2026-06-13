# 需求 ↔ DoD 追溯矩阵 — signal-eval 基准参数化

| # | 需求 | 任务 | DoD 锚点 |
|---|---|---|---|
| R1 | QlibPriceSource benchmark 参数化（默认 000300.SH）| TASK-001 | functional[0] |
| R2 | benchmark() 经 to_qlib_instrument 解析（^HSI→HSI）| TASK-001 | functional[1..2] |
| R3 | 构造不触发 qlib（惰性保持）| TASK-001 | boundary |
| R4 | evaluate.py --benchmark 参数 | TASK-002 | functional[0] |
| R5 | _meta 反映传入基准 | TASK-002 | functional[1] |
| R6 | main 透传 benchmark 到 QlibPriceSource | TASK-002 | functional[2] |
| R7 | Makefile signal-eval --benchmark + signal-eval-hk | TASK-003 | functional[0..1] |
| R8 | 港股事件研究非空（基准恒生）| TASK-004 | functional[1..2] |
| R9 | A股默认基准零回归 | TASK-001/002/004 | boundary + 单测 |
| R10 | 基准失败 benchmark_error 优雅降级（消费侧不改）| TASK-002 | error_handling |
| R11 | 美股推迟（仅参数化就绪）| 非目标 | — |

正向：R1-R10 均有任务覆盖；R11 非目标无任务。反向：各任务 DoD 均映射到 R1-R10，无凭空 DoD（覆盖率/编译/语法类来源团队规范，已注 verify_by）。

孤儿需求 0；凭空 DoD 0。矩阵闭合。
