# 验收报告 — IC/IR 信号预测力评估管线

- Sprint 分支：`feature/ic-ir-eval-design`
- 验收日期：2026-06-23
- 需求/计划：`docs/superpowers/plans/2026-06-22-ic-ir-eval.md`（设计 rev2：`docs/plans/2026-06-22-ic-ir-eval-design.md`）
- autonomy：dod-gate（DoD 定稿后人工确认 1 次）

## 结论：交付通过（PASS）

为稠密分数面板 `(symbol,date)→score` 建立了时序 IC/Rank IC/ICIR 离线评估管线，
oracle baseline 端到端 IC≈1 自证管线正确，作为整合方向②（ML 信号源）的量化验收前置。

## 完成任务（7/7 accepted）

| Task | 标题 | commit | rework |
|---|---|---|---|
| TASK-001 | ic.py forward_returns（next-open 前向收益） | 2ae38c7 | 0 |
| TASK-002 | ic.py instrument_ic（单标的时序 IC + t-stat） | a9efe85 | 0 |
| TASK-003 | ic.py 汇总（ic_summary_by_instrument + watchlist_summary） | a3e5269 | 0 |
| TASK-005 | baseline.py（oracle + reversal 因子） | d43354a | 0 |
| TASK-004 | report.py（read_scores + render_ic_report） | cffde70 | 1 (QA W1) |
| TASK-006 | ic_evaluate.py CLI + collect_ic | 99ad940 | 0 |
| TASK-007 | Makefile target + runbook + makefile 测试 | cffde70 | 1 (QA W3) |

## 调度
- DAG 调度；唯一并行点 T3∥T5（dev-agent-1 与 dev-agent-2 并行，GIT 锁串行化提交）。
- 其余强顺序串行（ic.py 三任务同文件）。

## 测试与覆盖
- 测试命令：`scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q`
- 结果：**117 passed, 1 warning**（warning 为预期 ConstantInputWarning：常数序列 Spearman 无定义，由 test_instrument_ic_nan_returns_none_not_raise 断言返回 None 不抛）。
- 新增测试：test_ic.py(实现后 25+ 用例)、test_baseline.py(4)、test_ic_report.py(16)、test_makefile.py(+3)。
- 覆盖率工具降级说明：config.language=go 但本工作为 Python，且 venv 无 pytest-cov；覆盖以「done_criteria 逐条 test 覆盖 + Test Agent 逐条 Reality-Checker 验证」替代数值门槛。

## DoD 强化（人类在 DoD Gate 选「补强测试，不改实现」）
reviewer 反审发现的 gap 已折入并验证：
- gap1 前视判别（open≠close 三候选值互异钉死无前视）— T1
- gap2 停牌/max_defer 延迟入场与耗尽不产行 — T1
- gap3 重叠虚高校正（非重叠 t-stat 真非重叠、与重叠数值不同）— T2
- gap4 scores.csv 坏行行号 / 空文件 / BOM — T4
- gap5 CLI 退出码 main() 缺目录 exit1 / 空面板 exit0 — T6
- gap6 守门覆盖 baseline/ic/report 全模块 + collect_ic data_gaps — T5/T6
- Pearson≠Spearman 双路径实测 — T2

## QA Code Review（两轮：常规 + 跨视角对抗）
- verdict：PASS（无 CRITICAL）。一个 CRITICAL 误报（渲染崩溃）经实证驳回。
- 一轮 cheap review_fix 折入：W1（report.py NaN 守护，nan→'-'）、W3（test_makefile.py 覆盖新 target）；均已验证。
- **推迟项 W2**（非阻塞，已记录）：`instrument_ic` 对重复 (date,symbol) 会笛卡尔膨胀 n_periods/IC。
  baseline 生产者（reversal_scores）不会产生重复；此风险仅在**未来方向② sidecar 生产路径**（buggy 上游 scores.csv）出现。
  → 推迟到 sidecar 集成时处理（建议 instrument_ic 内 drop_duplicates(subset=['date']) 或 read_scores 重复计数告警）。

## 其他已知约束 / 后续
- calendar-day `max_defer*2` 是 trading-day defer 的近似（跨假期可能多保留停牌>5日标的）；Phase1 可接受。
- overlap t-stat 告诫为定性；可选注明 effective dof≈n/h。
- 覆盖率数值门槛因 Python+无 pytest-cov 降级为分支论证。

## 降级记录
- validator/cmd/arcforge-validate 缺失 → Leader 手工校验任务图（DAG 无环/wave 序/scope 互斥，全通过）。
- .claude/hooks/arcforge-write.sh 缺失 → Leader 独占 .arcforge/ 写入（dev/test 全程无 .arcforge/ 写权限，transition-audit 无越权）。
- ecc/codex_cli/gemini_cli 均不可用 → 无 multi-plan、QA 跨视角对抗降级为纯 Claude。

## 交付物
- 实现：ic.py / baseline.py / report.py(增量) / ic_evaluate.py
- 集成：Makefile（signal-ic / baseline-scores target）、runbook（时序 IC 评估章节 + 读数告诫）
- 用法：`make baseline-scores` → `make signal-ic SCORES=baseline_scores.csv` → `reports/signal-ic-YYYYMMDD.md`
