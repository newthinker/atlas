# 进度看板 — Qlib 数据仓库 第一期

> 真相源：`.arcforge/tasks/*.json` 的 status 字段。本文件由 Leader 维护。
> 状态：**✅ 完成交付 — 13/13 accepted，Step 7 归档中**。

## 最终结果
- **13/13 accepted**。transition-audit 全绿（单 owner / epoch≥1 / 依赖闭合 / 无残留）。
- QA 两轮审查 CONTESTED → 人类裁决「三条全修」→ review_fix 一轮闭环（#1 NULL 读崩 / #3 Covers-lastDate 口径 / #2 API 降级）→ **CONTESTED 解除**。
- rework 历史：T2/T4(boundary)、T6/T8(error 测试)、T6(QA#1/#3) 共 5 次返工均一轮闭环，无 blocked_human。
- 15 个 commit（7d79f95..98d3eb1）。Python 12 passed；qlib 95.6%/collector 98.2%/config 93.9%/app 96.4%；全仓零 FAIL；make warehouse-dump=20505 行。
- 交付文档：06-acceptance/{final-report,changelog}、07-deploy/deploy-notes。

## 调度模式
`dag`：任务就绪条件 = 所有 dependencies 均 verified；就绪即派。max_dev_agents=4。

## 任务总览（12 任务，5 wave）

| Task | 标题 | package | deps | wave | status |
|---|---|---|---|---|---|
| T1 | SQLite schema (Py) | scripts/qlib_warehouse | — | 1 | pending |
| T2 | CSV 摄取归一化 (Py) | scripts/qlib_warehouse | T1 | 2 | pending |
| T3 | 原子 SQLite 写入器 (Py) | scripts/qlib_warehouse | T2 | 3 | pending |
| T4 | build CLI + Makefile (Py) | scripts/qlib_warehouse | T3 | 4 | pending |
| T5 | 驱动+骨架+Covers (Go) | internal/collector/qlib | — | 1 | pending |
| T6 | FetchHistory 仓库读 (Go) | internal/collector/qlib | T5 | 2 | pending |
| T7 | 补尾+降级+陈旧 (Go) | internal/collector/qlib | T6 | 3 | pending |
| T8 | Quote/非日频委托 (Go) | internal/collector/qlib | T7 | 4 | pending |
| T9 | selector 优先 qlib (Go) | internal/collector | — | 1 | pending |
| T10 | QlibConfig (Go) | internal/config | — | 1 | pending |
| T11 | App.CollectorRegistry (Go) | internal/app | — | 1 | pending |
| T12 | serve.go 装配 (Go) | cmd/atlas | T4,T8,T9,T10,T11 | 5 | pending |

## 团队规划（待确认门通过后 spawn）
- dev-agent-1: T1→T2→T3→T4（Python 链）
- dev-agent-2: T5→T6→T7→T8（Go qlib 链）
- dev-agent-3: T9→T10
- dev-agent-4: T11→T12
- test-agent-1 / test-agent-2：按 dev_done 派验

## 校验结论
- 手工 validator 全绿：DAG 无环 / wave 序 / scope 互斥 / 依赖闭合 / DoD≤8 / 单 owner。
- 需求↔DoD 矩阵：孤儿需求 0、凭空 DoD 0。
- 独立 reviewer：验收空间充分；采纳 2 条强化（T3 原子 rename、T9 兜底专测），1 条残留风险（serve 装配自动化测试）待人类裁决。
