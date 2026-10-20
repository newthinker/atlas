# 需求 ↔ DoD 追溯矩阵（sprint 2026-10-20 Pricewatch）

机器检查：DoD 共 80 条，标注表 80 行（两数须相等）；孤儿需求 0 个 []；凭空 DoD 0 条 []。R24（上线：真实 Telegram/LLM、plist 安装加载、合并 master）按 AD-9 为人类动作，刻意不入 DoD。R25 = 工程约束（plan Global Constraints：GOTOOLCHAIN=local、-race、vet、gofmt；config 覆盖率门槛）。

修订记录：独立 reviewer（只读需求，`scratchpad/reviewer-pricewatch-dod.md`，V1–V32 / P1–P22，判 insufficient、缺口 32）反审后：补入 V1/V2/V5–V10/V12–V15/V17/V19/V21/V23/V24/V26–V29/V31/V32、P2–P6/P9/P12/P13/P20；拆出 TASK-010（窗口统计）、TASK-011（健康追踪）；P7（KeepAlive 语义）待人类拍板；P10/P14/P16–P19/P22 作为已知限制（AD-28）。

## 需求 → DoD

| R | DoD |
|---|---|
| R1 | TASK-001.boundary[0], TASK-002.functional[0], TASK-002.functional[1], TASK-002.boundary[0] |
| R2 | TASK-001.boundary[0], TASK-001.error_handling[0], TASK-002.error_handling[0], TASK-009.error_handling[0] |
| R3 | TASK-002.functional[0], TASK-002.error_handling[1] |
| R4 | TASK-002.functional[0], TASK-002.boundary[0] |
| R5 | TASK-002.functional[2] |
| R6 | TASK-001.functional[0], TASK-003.functional[1], TASK-010.functional[3] |
| R7 | TASK-001.functional[1] |
| R8 | TASK-001.functional[3], TASK-010.functional[0], TASK-010.boundary[0] |
| R9 | TASK-001.functional[2], TASK-001.boundary[1] |
| R10 | TASK-007.functional[0] |
| R11 | TASK-005.error_handling[0], TASK-007.functional[1] |
| R12 | TASK-003.functional[0], TASK-003.functional[1], TASK-003.functional[2], TASK-003.functional[3], TASK-003.boundary[0], TASK-003.boundary[1], TASK-003.error_handling[0], TASK-003.non_functional[0], TASK-004.functional[2], TASK-004.non_functional[0], TASK-007.functional[2] |
| R13 | TASK-004.functional[0], TASK-004.functional[1], TASK-004.functional[2], TASK-004.functional[3], TASK-004.functional[4], TASK-004.functional[5], TASK-004.boundary[0], TASK-004.non_functional[0], TASK-009.functional[1] |
| R14 | TASK-004.functional[4], TASK-004.functional[5], TASK-005.functional[3], TASK-007.functional[3], TASK-010.functional[0], TASK-010.functional[1], TASK-010.functional[2], TASK-010.boundary[0] |
| R15 | TASK-004.boundary[0], TASK-007.boundary[0], TASK-009.functional[1] |
| R16 | TASK-005.functional[1], TASK-006.functional[0], TASK-006.functional[1], TASK-006.functional[2], TASK-006.boundary[0], TASK-006.error_handling[0], TASK-006.error_handling[1], TASK-007.functional[0], TASK-007.functional[3], TASK-007.error_handling[0], TASK-009.functional[1] |
| R17 | TASK-005.functional[0], TASK-005.functional[1], TASK-005.functional[2], TASK-005.functional[3], TASK-005.boundary[0], TASK-005.error_handling[0] |
| R18 | TASK-005.functional[4], TASK-007.error_handling[1], TASK-011.functional[0], TASK-011.functional[1], TASK-011.functional[2] |
| R19 | TASK-005.functional[4], TASK-007.functional[0], TASK-007.non_functional[0], TASK-008.functional[0], TASK-008.functional[2], TASK-008.boundary[0] |
| R20 | TASK-007.non_functional[0], TASK-009.functional[0], TASK-009.error_handling[0], TASK-009.error_handling[1], TASK-009.non_functional[1] |
| R21 | TASK-008.functional[1], TASK-008.functional[3], TASK-008.error_handling[0], TASK-009.error_handling[2] |
| R22 | TASK-009.functional[2] |
| R23 | TASK-002.functional[3] |
| R24 | （人类动作，AD-9） |
| R25 | TASK-001.non_functional[0], TASK-002.non_functional[0], TASK-003.non_functional[0], TASK-004.non_functional[0], TASK-005.non_functional[0], TASK-006.non_functional[0], TASK-007.non_functional[0], TASK-008.non_functional[0], TASK-009.non_functional[0], TASK-010.non_functional[0], TASK-011.non_functional[0] |

## DoD → 需求

| DoD | R |
|---|---|
| TASK-001.functional[0] | R6 |
| TASK-001.functional[1] | R7 |
| TASK-001.functional[2] | R9 |
| TASK-001.functional[3] | R8 |
| TASK-001.boundary[0] | R1, R2 |
| TASK-001.boundary[1] | R9 |
| TASK-001.error_handling[0] | R2 |
| TASK-001.non_functional[0] | R25 |
| TASK-002.functional[0] | R1, R3, R4 |
| TASK-002.functional[1] | R1 |
| TASK-002.functional[2] | R5 |
| TASK-002.functional[3] | R23 |
| TASK-002.boundary[0] | R1, R4 |
| TASK-002.error_handling[0] | R2 |
| TASK-002.error_handling[1] | R3 |
| TASK-002.non_functional[0] | R25 |
| TASK-003.functional[0] | R12 |
| TASK-003.functional[1] | R12, R6 |
| TASK-003.functional[2] | R12 |
| TASK-003.functional[3] | R12 |
| TASK-003.boundary[0] | R12 |
| TASK-003.boundary[1] | R12 |
| TASK-003.error_handling[0] | R12 |
| TASK-003.non_functional[0] | R12, R25 |
| TASK-004.functional[0] | R13 |
| TASK-004.functional[1] | R13 |
| TASK-004.functional[2] | R13, R12 |
| TASK-004.functional[3] | R13 |
| TASK-004.functional[4] | R13, R14 |
| TASK-004.functional[5] | R13, R14 |
| TASK-004.boundary[0] | R13, R15 |
| TASK-004.non_functional[0] | R12, R13, R25 |
| TASK-005.functional[0] | R17 |
| TASK-005.functional[1] | R16, R17 |
| TASK-005.functional[2] | R17 |
| TASK-005.functional[3] | R14, R17 |
| TASK-005.functional[4] | R18, R19 |
| TASK-005.boundary[0] | R17 |
| TASK-005.error_handling[0] | R11, R17 |
| TASK-005.non_functional[0] | R25 |
| TASK-006.functional[0] | R16 |
| TASK-006.functional[1] | R16 |
| TASK-006.functional[2] | R16 |
| TASK-006.boundary[0] | R16 |
| TASK-006.error_handling[0] | R16 |
| TASK-006.error_handling[1] | R16 |
| TASK-006.non_functional[0] | R25 |
| TASK-007.functional[0] | R10, R16, R19 |
| TASK-007.functional[1] | R11 |
| TASK-007.functional[2] | R12 |
| TASK-007.functional[3] | R14, R16 |
| TASK-007.boundary[0] | R15 |
| TASK-007.error_handling[0] | R16 |
| TASK-007.error_handling[1] | R18 |
| TASK-007.non_functional[0] | R19, R20, R25 |
| TASK-008.functional[0] | R19 |
| TASK-008.functional[1] | R21 |
| TASK-008.functional[2] | R19 |
| TASK-008.functional[3] | R21 |
| TASK-008.boundary[0] | R19 |
| TASK-008.error_handling[0] | R21 |
| TASK-008.non_functional[0] | R25 |
| TASK-009.functional[0] | R20 |
| TASK-009.functional[1] | R13, R15, R16 |
| TASK-009.functional[2] | R22 |
| TASK-009.error_handling[0] | R2, R20 |
| TASK-009.error_handling[1] | R20 |
| TASK-009.error_handling[2] | R21 |
| TASK-009.non_functional[0] | R25 |
| TASK-009.non_functional[1] | R20 |
| TASK-010.functional[0] | R8, R14 |
| TASK-010.functional[1] | R14 |
| TASK-010.functional[2] | R14 |
| TASK-010.functional[3] | R6 |
| TASK-010.boundary[0] | R8, R14 |
| TASK-010.non_functional[0] | R25 |
| TASK-011.functional[0] | R18 |
| TASK-011.functional[1] | R18 |
| TASK-011.functional[2] | R18 |
| TASK-011.non_functional[0] | R25 |
