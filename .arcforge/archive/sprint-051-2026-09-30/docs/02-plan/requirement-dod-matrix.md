# 需求 ↔ DoD 追溯矩阵（sprint 2026-09-30 Tiingo）

机器检查：DoD 共 63 条，标注表 63 行（两数须相等）；孤儿需求 0 个 []；凭空 DoD 0 条 []。R19（上线验证）按 AD-9 为人类动作，刻意不入 DoD。

修订记录：独立 reviewer（只读需求，`scratchpad/reviewer-tiingo-dod.md`，V1–V38 / P1–P21）反审后补入 P1–P10、P12、P20、P21；`normalize` 拆为 TASK-009。人类 2026-09-30 裁决：P11 收紧后缀（TASK-003）、P17 附前序跳原因（TASK-006）、P16 接受并记录（TASK-007 注释）、P14 不做。

## 需求 → DoD

| R | DoD |
|---|---|
| R1 | TASK-001.functional[0], TASK-001.functional[1], TASK-001.boundary[0], TASK-001.non_functional[0], TASK-001.non_functional[1], TASK-001.non_functional[2] |
| R2 | TASK-002.functional[0], TASK-002.functional[1], TASK-002.boundary[0], TASK-002.boundary[1], TASK-002.boundary[2], TASK-002.non_functional[0], TASK-004.functional[2] |
| R3 | TASK-003.functional[0], TASK-003.boundary[0], TASK-003.boundary[1], TASK-003.boundary[2], TASK-003.non_functional[0], TASK-005.error_handling[0], TASK-003.boundary[3] |
| R4 | TASK-003.functional[1], TASK-004.functional[0] |
| R5 | TASK-004.functional[0] |
| R6 | TASK-004.non_functional[0] |
| R7 | TASK-004.functional[1], TASK-004.boundary[0], TASK-009.functional[0], TASK-009.functional[1], TASK-009.boundary[0], TASK-009.boundary[1], TASK-009.boundary[3], TASK-009.non_functional[0] |
| R8 | TASK-009.boundary[2], TASK-009.error_handling[0] |
| R9 | TASK-004.error_handling[0], TASK-004.error_handling[1] |
| R10 | TASK-002.boundary[0], TASK-004.functional[2], TASK-004.error_handling[2] |
| R11 | TASK-005.functional[0], TASK-005.functional[3], TASK-005.error_handling[0], TASK-005.non_functional[0] |
| R12 | TASK-005.functional[1], TASK-005.functional[2], TASK-005.boundary[0] |
| R13 | TASK-006.functional[0], TASK-006.functional[1], TASK-006.boundary[0], TASK-006.boundary[1], TASK-006.error_handling[0], TASK-006.non_functional[0] |
| R14 | TASK-006.functional[2], TASK-006.non_functional[0], TASK-006.non_functional[1] |
| R15 | TASK-007.functional[0], TASK-007.functional[1], TASK-007.boundary[0], TASK-007.non_functional[0], TASK-007.non_functional[1], TASK-007.non_functional[2], TASK-007.non_functional[3] |
| R16 | TASK-007.functional[2] |
| R17 | TASK-008.functional[0], TASK-008.functional[1], TASK-008.functional[2], TASK-008.boundary[0], TASK-008.non_functional[0] |
| R18 | TASK-001.non_functional[1], TASK-006.boundary[1], TASK-007.boundary[0] |
| R19 | （人类动作，AD-9） |

## DoD → 需求

| DoD | R |
|---|---|
| TASK-001.functional[0] | R1 |
| TASK-001.functional[1] | R1 |
| TASK-001.boundary[0] | R1 |
| TASK-001.non_functional[0] | R1 |
| TASK-001.non_functional[1] | R1, R18 |
| TASK-001.non_functional[2] | R1 |
| TASK-002.functional[0] | R2 |
| TASK-002.functional[1] | R2 |
| TASK-002.boundary[0] | R2, R10 |
| TASK-002.boundary[1] | R2 |
| TASK-002.boundary[2] | R2 |
| TASK-002.non_functional[0] | R2 |
| TASK-003.functional[0] | R3 |
| TASK-003.functional[1] | R4 |
| TASK-003.boundary[0] | R3 |
| TASK-003.boundary[1] | R3 |
| TASK-003.boundary[2] | R3 |
| TASK-003.boundary[3] | R3 |
| TASK-003.non_functional[0] | R3 |
| TASK-004.functional[0] | R5, R4 |
| TASK-004.functional[1] | R7 |
| TASK-004.functional[2] | R2, R10 |
| TASK-004.boundary[0] | R7 |
| TASK-004.error_handling[0] | R9 |
| TASK-004.error_handling[1] | R9 |
| TASK-004.error_handling[2] | R10 |
| TASK-004.non_functional[0] | R6 |
| TASK-005.functional[0] | R11 |
| TASK-005.functional[1] | R12 |
| TASK-005.functional[2] | R12 |
| TASK-005.functional[3] | R11 |
| TASK-005.boundary[0] | R12 |
| TASK-005.error_handling[0] | R11, R3 |
| TASK-005.non_functional[0] | R11 |
| TASK-006.functional[0] | R13 |
| TASK-006.functional[1] | R13 |
| TASK-006.functional[2] | R14 |
| TASK-006.boundary[0] | R13 |
| TASK-006.boundary[1] | R13, R18 |
| TASK-006.error_handling[0] | R13 |
| TASK-006.non_functional[0] | R13, R14 |
| TASK-006.non_functional[1] | R14 |
| TASK-007.functional[0] | R15 |
| TASK-007.functional[1] | R15 |
| TASK-007.functional[2] | R16 |
| TASK-007.boundary[0] | R15, R18 |
| TASK-007.non_functional[0] | R15 |
| TASK-007.non_functional[1] | R15 |
| TASK-007.non_functional[2] | R15 |
| TASK-007.non_functional[3] | R15 |
| TASK-008.functional[0] | R17 |
| TASK-008.functional[1] | R17 |
| TASK-008.functional[2] | R17 |
| TASK-008.boundary[0] | R17 |
| TASK-008.non_functional[0] | R17 |
| TASK-009.functional[0] | R7 |
| TASK-009.functional[1] | R7 |
| TASK-009.boundary[0] | R7 |
| TASK-009.boundary[1] | R7 |
| TASK-009.boundary[2] | R8 |
| TASK-009.boundary[3] | R7 |
| TASK-009.error_handling[0] | R8 |
| TASK-009.non_functional[0] | R7 |
