# Sprint 进度看板 — sprint-003 Qlib 回测验证管线（2026-06-12）

**状态**: ✅ Sprint 完成（QA PASS，8/8 accepted，已交付归档）
**需求源**: docs/plans/2026-06-12-qlib-eval-pipeline-implementation.md（施工图）| 设计 rev3
**调度**: dag | **首个跨语言 Sprint**（Go 门禁 80%/cmd 35%；Python 走 hook pytest + DoD 矩阵）

## 任务总览

| 任务 | 标题 | plan | scope | 依赖 | wave | 状态 |
|------|------|------|-------|------|------|------|
| TASK-001 | 引擎盖戳+SkippedBars | T1 | internal/backtest | — | 1 | assigned → dev-agent-1 |
| TASK-002 | ma_crossover ctx.Now | T2 | strategy/ma_crossover | — | 1 | assigned → dev-agent-2 |
| TASK-003 | export-signals CLI | T3+T4 | cmd/atlas | 001 | 2 | pending |
| TASK-004 | Py 脚手架+symbols+conftest | T5 | scripts/qlib_eval | — | 1 | assigned → dev-agent-3（Python 链专员） |
| TASK-005 | prices+align_entry | T6 | scripts/qlib_eval | 004 | 2 | pending |
| TASK-006 | event_study | T7 | scripts/qlib_eval | 005 | 3 | pending |
| TASK-007 | report+evaluate CLI | T8 | scripts/qlib_eval | 006 | 4 | pending |
| TASK-008 | e2e+Makefile+README | T9 | scripts/qlib_eval | 003,007 | 5 | pending |

## 依赖图

```
Go:  001 引擎 ──> 003 CLI ──────────────┐
     002 ma_crossover（独立）            ├─> 008 e2e
Py:  004 ──> 005 ──> 006 ──> 007 ───────┘   （Python 链同 scope 强制串行）
```

## 环境与机制（本 Sprint 特有）

- Python 统一 `scripts/qlib_eval/.venv/bin/python`（3.11.2+pandas 3.0.3+pytest 9.0.3，默认 python3 损坏）
- hook 2e 分流：Python scope 跑 pytest 门禁（从仓库根执行——conftest.py 必须存在，H2 反审项）
- qlib 数据包真实运行 = 可选验收（ADR-S3-4）

## 质量门禁记录

- [x] 8 任务 27 条 DoD（test 24/review 3）锚定 plan 原文
- [x] reviewer 反审：NEEDS_REVISION → 6 项修订采纳（H2 conftest 跨语言新风险为最关键拦截）→ PASS
- [x] validator 两次通过
- [ ] **人工确认门（dod-gate）← 当前位置**

## 事件日志

- 2026-06-12: 环境适配（venv 预置/hook 跨语言分流/.gitignore）；01-design 落盘（4 ADR）
- 2026-06-12: 8 任务拆分、反审 6 修订、validator 通过；进入 dod-gate
- 2026-06-12: 人工确认通过；旧团队 TeamDelete 重建；wave1 派发（001→dev-1、002→dev-2、004→dev-3）；spawn dev×3 + test×2（dev-3 为 Python 链专员承接 004-008 串行链）
- 2026-06-12: wave1 三任务齐 dev_done → 全 verified；003→dev-1、005→dev-3 双线推进
- 2026-06-12: 003（024c195，含 H1 全量注册）/005（7de4730，边界双侧）/006（d92994d，六钉死点）/007（592bfbf）/008（038f49b）依次 verified
- 2026-06-12: **8/8 全 verified，零返工零阻塞**（连续第二个 Sprint）。进入 Step 6 QA
- 2026-06-12: QA round1+2 **PASS** + 2 WARNING（空信号崩溃/基准缺失崩溃，实测复现）→ 裁决即修（+S3/S7 顺手），TASK-007 review_fix
- 2026-06-12: 修复 35c18c9 → Test 复验 verified → QA round3 **PASS**（崩溃反例亲证闭合）
- 2026-06-12: Step 7 交付：final-report/changelog/07-deploy 落盘；8/8 accepted；S4/S5/S6+数据包端到端入 CARRYOVER；团队关闭；归档
