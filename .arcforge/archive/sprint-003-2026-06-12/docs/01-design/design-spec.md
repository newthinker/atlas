# 设计规格 — sprint-003 Qlib 评估管线

> **权威施工图**: `docs/plans/2026-06-12-qlib-eval-pipeline-implementation.md`（9 Task，含测试代码与骨架）。
> 本文件只记录任务图重组、跨语言约定与环境替换，Dev 开工必读 plan 对应 Task 原文。

## plan Task → arcforge 任务映射（8 任务）

| arcforge | plan | packages | 要点 |
|----------|------|----------|------|
| TASK-001 | T1 | internal/backtest | 引擎统一盖戳 GeneratedAt=bar 时间（覆写策略自报值）+ Result.SkippedBars |
| TASK-002 | T2 | internal/strategy/ma_crossover | 两处 time.Now()→ctx.Now |
| TASK-003 | T3+T4 | cmd/atlas（+Makefile） | executeExport 核心（白名单/warm-up/golden CSV）+ cobra 接线 + Makefile export-signals |
| TASK-004 | T5 | scripts/qlib_eval | 脚手架 + to_qlib_instrument（仅 A 股） |
| TASK-005 | T6 | scripts/qlib_eval | PriceSource 协议 + align_entry（次日开盘、max_defer*2 日历日上界）+ QlibPriceSource（lazy import） |
| TASK-006 | T7 | scripts/qlib_eval | event_study：horizon 收益/超额/sell 规避（取向）/aggregate/置信度分桶 |
| TASK-007 | T8 | scripts/qlib_eval | read_signals 严格 schema + render_report + evaluate.py（缺数据 exit(1)+指引） |
| TASK-008 | T9 | scripts/qlib_eval（+Makefile/README） | make signal-eval 串联 + e2e + README 收尾 |

合并/串行依据：T3+T4 同 cmd/atlas 包；Python 四任务同 scripts/qlib_eval scope **强制串行链** 004→005→006→007→008（与 plan 依赖递进一致）；Go 三任务（001/002/003）与 Python 链并行。

## 跨语言约定

- **CSV 契约**（唯一接口）：`symbol,date,strategy,action,confidence,price,metadata` 七列；confidence/price %.2f；metadata JSON 串（nil→空串）；date=GeneratedAt 的 YYYY-MM-DD。
- **Python 解释器**：一律 `scripts/qlib_eval/.venv/bin/python`（默认 python3 损坏，见 requirements-analysis）。plan 中 `python -m pytest tests/` → `cd scripts/qlib_eval && .venv/bin/python -m pytest tests/ -v`。
- **门禁**：Go 任务走既有 80% 覆盖率门禁；Python 任务 hook 跑 pytest（通过/失败），覆盖率不设机器门禁、由 Test Agent 按 DoD 逐条核。

## 关键语义（QA/Test 重点）

- 盖戳是**机制性**的：测试用故意写错 GeneratedAt 的 stub 断言覆写（plan T1）
- warm-up：拉数起点 = from − (maxBars*365/252+30) 自然日；**只导出 GeneratedAt >= from** 的信号
- 白名单：`RequiredData().Fundamentals==true` 动态拒绝（不硬编码策略名）
- align_entry：searchsorted side="right" 严格次日；`(entry-signal).days > max_defer*2` → None（日历日近似，写 README）
- sell 规避收益 = −(ret − bench_ret)，胜率口径 buy=超额>0 / sell=规避>0
- 空序列/越界 → None/NA 并计数进报告「数据缺口」节
