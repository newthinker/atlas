# 需求分析 — sprint-003 Qlib 回测验证管线（2026-06-12）

**需求源**: docs/plans/2026-06-12-qlib-eval-pipeline-implementation.md（定稿实现计划，9 Task / 2 Chunk，含 TDD 测试代码与骨架）
**设计依据**: docs/plans/2026-06-11-qlib-eval-pipeline-design.md（rev3 终版）
**规划模式**: plan 即施工图（沿用 sprint-002 模式），本阶段工作 = 任务图重组 + 跨语言环境适配。

## 目标

`atlas export-signals` 导出真实策略信号（CSV 唯一跨语言契约）→ Python/qlib 事件研究评估（次日开盘入场、5/20/60 日 horizon、相对 SH000300 超额、sell 规避口径），量化每个 buy/sell 信号的后续收益与胜率。

## 需求清单

| ID | 需求 | plan Task | 语言 |
|----|------|-----------|------|
| R1 | 引擎盖戳 GeneratedAt（bar 时间，机制性）+ SkippedBars 计数 | T1 | Go |
| R2 | ma_crossover 墙钟修复（time.Now→ctx.Now） | T2 | Go |
| R3 | export-signals：白名单（Fundamentals 动态拒绝）+ warm-up 前移 + golden CSV + cobra/Makefile | T3,T4 | Go |
| R4 | Python 脚手架 + 符号映射（600519.SH→SH600519，仅 A 股） | T5 | Py |
| R5 | PriceSource 协议 + 次日开盘入场对齐（max_defer 顺延/丢弃） | T6 | Py |
| R6 | 事件研究核心（horizon 收益/超额/sell 规避/聚合/置信度分桶） | T7 | Py |
| R7 | CSV 严格读取 + markdown 报告 + evaluate.py CLI（缺数据指引） | T8 | Py |
| R8 | make signal-eval 端到端 + README 口径/数据包文档 | T9 | 混合 |

## 关键环境事实（Leader 已处置）

- **默认 python3（pyenv 3.10.12）dyld 损坏不可用**；可用 Python 3.11.2（/Library/Frameworks）。已创建 `scripts/qlib_eval/.venv`（pandas 3.0.3 + pytest 9.0.3）——**全 Sprint 统一用 `scripts/qlib_eval/.venv/bin/python`**，plan 中的 `python -m pytest` 一律替换。
- **task-completed.sh 已适配跨语言**：scope 含 scripts/qlib_eval 时跑 pytest 门禁（失败阻断）；无 .go 的 scope 不进 Go 覆盖率门禁（Python 覆盖率由 Test Agent 按 DoD 核对）。
- .gitignore 已加 .venv/reports/signals.csv。
- qlib 数据包（~/.qlib/qlib_data/cn_data）未确认存在——`make signal-eval` 真实运行属验收可选项（plan §5 设计了缺数据指引），pytest 全程不依赖。

## 范围边界（plan 原文）

- 一期仅 A 股符号评估（非 A 股进「数据缺口」节）
- Fundamentals 策略（pe_band/dividend_yield/pe_percentile）不可离线重放 → 白名单动态拒绝
- qlib 仅运行时依赖，pytest 不依赖 qlib 安装与数据包

## 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| pandas 3.0.3 较新（plan 按 1.5+ 写）API 行为差异 | 低 | pytest 全绿为准；遇 API 变更按 3.x 修正 |
| Python 任务同目录强串行（scripts/qlib_eval 单 scope） | 低 | T5→T8 本就依赖递进；Go 三任务并行补偿 |
| qlib 数据包缺失 | 低 | e2e 真实运行降级为指引验证（plan §5 口径） |
