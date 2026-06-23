# 需求分析 — IC/IR 信号预测力评估管线

## 来源
- 需求/计划文档：`docs/superpowers/plans/2026-06-22-ic-ir-eval.md`（本身即完整 7 任务 TDD 计划，含每步代码、测试、commit）。
- 设计依据：`docs/plans/2026-06-22-ic-ir-eval-design.md`（rev2）。

## 目标
为稠密分数面板 `(symbol,date)→score` 建立**时序 IC / Rank IC / ICIR** 离线评估管线，
先用 baseline 因子（oracle + 反转）自证管线可信，作为整合方向②（ML 信号源）的量化验收前置。

## 功能模块
1. **计算核心 `ic.py`**（纯 pandas）：forward_returns（next-open 前向收益）、instrument_ic（单标的时序 IC + t-stat）、ic_summary_by_instrument、watchlist_summary。
2. **baseline 因子 `baseline.py`**：reversal_scores（纯）、oracle_scores（纯）、load_prices（惰性 qlib）。
3. **报告 `report.py`（增量）**：read_scores（CSV 校验）、render_ic_report（markdown）。
4. **CLI `ic_evaluate.py`**：读 scores → 取价 → 算 IC → 写报告；缺目录 exit(1)、空面板 exit(0)。
5. **集成 `Makefile` + `runbook`**：signal-ic / baseline-scores target + 重叠收益读数告诫文档。

## 关键约束（Global Constraints）
- IC 口径：**时序 IC（逐标的）**，非横截面。
- 前向收益：**next-open 对齐**（score(t)←close(t) → open_{t+1} 入场 → h 交易日后收盘出场），复用 `align_entry`。
- horizons 固定 (5,20,60)，ic 模块独立定义常量，不 import event_study。
- t-stat = ic*sqrt(n_periods)（小 IC 近似，钉死）；每个 t-stat 并列一个非重叠采样 t-stat。
- 顶层禁止 import qlib（仅 prices.py/baseline.py 函数体内惰性导入）；守门测试锁死。
- 不改动 event_study.py 及现有 signal-eval 管线。

## 复杂度评估
- 整体：中等。计算核心逻辑清晰、可手工验算；难点在 next-open 边界与重叠收益 t-stat 校正。
- 单任务：均为简单/中等（≤1 文件 create/modify + 1 测试文件）。
- 风险：强顺序依赖（ic.py 三任务串行），并行度低（仅 T3∥T5）；纯 Python 项目但 config=go。
