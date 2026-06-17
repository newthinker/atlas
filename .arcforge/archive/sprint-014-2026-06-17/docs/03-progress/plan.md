# 进度计划 — Telegram 信号汇总表格

> 真相源：`.arcforge/tasks/*.json` 的 status 字段。本文件由 Leader 维护。
> 调度模式：`dag`（依赖全部 verified 即就绪派发）。autonomy：`dod-gate`。

## 当前阶段

**✅ 交付完成（全部 accepted）**
- TASK-001 ✅ accepted @619faaf（width.go 100%）
- TASK-003 ✅ accepted @3f0bb11（cov 87.6%，-race 无竞争）
- TASK-002 ✅ accepted @611d8e4（review_fix 后整包 90.3%）
- TASK-004 ✅ accepted @39fb228（串行 flush 回归 PASS）
- QA verdict=PASS（无 CRITICAL）；2 条 WARNING 经 1 轮 review_fix 全修；终验全仓 50 包零回归
- 交付物：06-acceptance/{final-report,changelog,qa-review-round1}.md
- **待用户决策**：合并到 master / 建 PR / 保持分支（见对话）；之后 /arcforge-archive 归档

> 覆盖率 DoD 修正见 wisdom/decisions-leader.md D1（整包→变更文件基准）。
> 串行 flush 修正见 D2（独立评审发现）。

## 任务图（DAG / wave）

```
wave1:  TASK-001 (telegram/width)      TASK-003 (router/buffer)
            │                                 │
wave2:  TASK-002 (telegram/formatBatch) ──────┤
            │                                 │
wave3:           TASK-004 (config+app wiring) ┘  [deps: 002, 003]
```

| 任务 | 标题 | wave | deps | packages | status | rework |
|------|------|------|------|----------|--------|--------|
| TASK-001 | 显示宽度工具(CJK) | 1 | — | ./internal/notifier/telegram | pending | 0 |
| TASK-002 | 分组表格 formatBatch | 2 | 001 | ./internal/notifier/telegram | pending | 0 |
| TASK-003 | router 缓冲+Flush | 1 | — | ./internal/router | pending | 0 |
| TASK-004 | config+app 接线 | 3 | 002,003 | ./internal/config, ./internal/app | pending | 0 |

## 校验状态

- 任务图校验（**降级人工**，Go validator 缺失）：DAG 无环 ✅ / wave 序 ✅ / 单 owner ✅ / context_from 闭合 ✅ / scope 非空+并发互斥 ✅
- 需求↔DoD 双向追溯：孤儿需求 0、凭空 DoD 0 ✅（`02-plan/requirement-dod-matrix.md`）
- 独立评审：完成，2 个 HIGH 发现已核验并修订计划，3 个设计判断待人类定夺（`02-plan/independent-review.md`）
- 基线 `go build ./...`：OK

## 团队规划

- Dev Agent ×2（wave1 两任务可并行）+ Test Agent ×1。max_dev_agents=4 未触顶。
- 降级：ECC 不可用（已据定稿 spec 拆分）；codex/gemini 不可用（QA 跨视角退纯 Claude）；validator 缺失（人工校验）。

## 计划修正记录

- **TASK-004**：独立评审发现串行路径(`workers<=1`)不 flush 的 bug，改用 `defer FlushNotifications()` 覆盖全部出口 + 加串行回归 DoD。
- **TASK-002**：加「特殊字符不破坏 ``` 代码块」边界 DoD + 部署人工核验。
