# 需求 ↔ DoD 双向追溯矩阵 — Sprint 022

需求 ID 见 `01-design/requirements-analysis.md`（R1–R7 ↔ 设计 v1.1 §1–§7）。

## 正向：需求 → DoD

| REQ | 内容摘要 | 覆盖任务 | DoD 条目 |
|---|---|---|---|
| R1 | CLI report 子命令（参数校验/量控 31/回放前缀/落盘/发送与降级） | TASK-006 | functional 1–3、boundary 1–2、error_handling 1–2、non_functional 1（共 8 条全覆盖） |
| R2 | 回放引擎暖机 + replay 重构黄金对照 | TASK-001 | functional 1–2（暖机推进/黄金回归）、boundary 1–2（切片/空窗口）、error_handling 1、nf 1（零写入）、nf 2（手工黄金） |
| R3 | ReplayReport daily/monthly 装配 | TASK-002 | functional 1–2（PrevDay 链/Trends）、boundary 1–2（首日 nil/门控忽略）、error_handling 1（未知 form）、nf 1（纯函数口径） |
| R4 | RenderReplaySummary + replayFooter | TASK-003 | functional 1–2（转移/停留/峰值/极值方向/STALE 口径）、boundary 1、error_handling 1、nf 1（≤4096+禁词）、nf 2（专用尾注） |
| R5 | RenderReplayHTML 自包含 SVG | TASK-004 | functional 1–2（点阵/折线点数）、boundary 1（sofr_effr 注记/STALE 打点）、error_handling 1、nf 1（自包含亮暗）、nf 2（阈值读 cfg）、nf 3（目检） |
| R6 | telegram SendDocument | TASK-005 | functional 1（multipart 齐全）、boundary 1（caption 1024 rune）、error_handling 1（API 错误语义）、nf 1（Sender 不动） |
| R7 | 全局约束（横切） | 全任务 | 零写入=001.nf1；纯函数=002.nf1；禁词+4096=003.nf1、004.nf1；接口不动=005.nf1；零新依赖/GOTOOLCHAIN=流程性约束（QA 清单核对，`go.mod` diff 为证）；gitnexus 门禁与 code-simplifier=Dev 工作流步骤（QA 核对提交记录） |

## 反向：DoD → 需求（凭空 DoD 检查）

TASK-001..006 全部 DoD 条目均可回指 R1–R7 与实施计划对应 Task 章节的测试代码；
无不对应任何需求的凭空 DoD。

## 孤儿需求检查

- R1–R6：一一对应 TASK-006/001/002/003/004/005，无孤儿。
- R7 横切条目中「零新第三方依赖」「GOTOOLCHAIN=local」无独立 DoD 条目——判定为流程性约束
  而非可单测断言，由 QA 阶段清单核对（go.mod 无 diff、提交记录含 simplifier 痕迹），记录在案不算孤儿。

## verify_by 占比提示

review/manual 条目：001（2/7）、002（1/6）、004（1/7 manual）、005（1/4）——占比均低，
不影响全自动流程；manual 条目（黄金 diff、亮暗目检）列入验收阶段清单。
