# 进度看板 — Qlib 数据仓库 第二期（Part B PIT 基本面）

> 真相源：`.arcforge/tasks/*.json` 的 status。Leader 维护。
> 状态：**等待人类确认门（dod-gate）** —— 尚未 spawn dev team。

## 调度
`dag`：依赖全 verified 即就绪派发。max_dev_agents=4。

## 任务总览（7 任务，4 wave）

| Task | 标题 | package | deps | wave | status |
|---|---|---|---|---|---|
| T1 | 基本面 CSV 摄取 (Py) | scripts/qlib_warehouse | — | 1 | pending |
| T2 | writer 同次原子写 fundamentals_pit (Py) | scripts/qlib_warehouse | T1 | 2 | pending |
| T3 | build CLI --fundamentals-dir (Py) | scripts/qlib_warehouse | T2 | 3 | pending |
| T4 | qlibpit EPS 源 PIT 查询 (Go) | internal/collector/qlibpit | — | 1 | pending |
| T5 | qlibpit 兜底委托测试 (Go) | internal/collector/qlibpit | T4 | 2 | pending |
| T6 | serve 装配 qlibpit (Go) | cmd/atlas | T4,T5 | 3 | pending |
| T7 | 适配器 ADAPTERS.md + Makefile (best-effort) | scripts/qlib_warehouse | T3 | 4 | pending |

## 团队规划（确认门通过后 spawn）
- dev-agent-1: T1→T2→T3→T7（Python 链）
- dev-agent-2: T4→T5（Go qlibpit 链）
- dev-agent-3: T6（serve 装配，待 T5）
- test-agent-1 / test-agent-2

## 校验结论
- 手工 validator 全绿：DAG/wave/scope/依赖/DoD≤8/单 owner。
- DoD 矩阵：孤儿 0 / 凭空 0。
- 关键适配：T6 须重构第一期 wireQlibWarehouse 暴露 db 句柄（架构决策 AD-5）。
