# 需求 ↔ DoD 双向追溯矩阵

来源需求：`docs/superpowers/plans/2026-06-22-ic-ir-eval.md`（Task 1-7 + Self-Review §spec coverage）。

## 正向：需求条目 → DoD 覆盖

| 需求（计划条目 / 设计 §） | 覆盖 task | DoD 维度 |
|---|---|---|
| §1.2 时序 IC（逐标的） | T2, T3 | functional |
| §2.1 next-open 前向收益（复用 align_entry） | T1 | functional + boundary |
| §2.1 horizons 5/20/60（HORIZONS 常量） | T1 | functional |
| §2.1 Pearson + Rank IC 都报（method 参数） | T2, T3, T4(注明) | functional/non_functional |
| §2.1 Oracle + 反转双 baseline | T5 | functional |
| §2.3 重叠收益 t-stat 校正（t_stat_nonoverlap + 报告告诫） | T2, T4 | functional/boundary |
| §5.1 计算核心全部签名 | T1, T2, T3 | functional |
| §5.2 scores.csv 契约 + 行号校验 | T4 | error_handling |
| §5.3 CLI（缺目录 exit1 / 空面板 exit0） | T6 | error_handling/boundary |
| 守门：顶层不得 import qlib | T1, T5, T6 | non_functional |
| 不改 event_study / 既有 signal-eval | T1, T4, T6 | non_functional |
| §7 Makefile + runbook 集成 + 读数告诫 | T7 | functional/non_functional |

## 反向：DoD → 需求（凭空 DoD 检查）
逐条核对：7 个 task 的全部 done_criteria 均可追溯到上表某需求条目，**无凭空 DoD**。

## 孤儿需求检查
Self-Review §spec coverage 列出的 11 项需求均有 task 覆盖，**无孤儿需求**。

## 结论
双向闭合：无孤儿需求、无凭空 DoD。待独立 reviewer 反审 + 人类确认。
