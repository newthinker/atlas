# 进度看板 — 第三期 分位「自上市起」全历史回看

> 真相源：`.arcforge/tasks/*.json` status。Leader 维护。
> 状态：**等待人类确认门（dod-gate）**。

## 调度
dag；max_dev_agents=4。

## 任务总览（7 任务，4 wave）

| Task | 标题 | package | deps | wave |
|---|---|---|---|---|
| T1 | SinceInceptionBars 常量 | internal/strategy | — | 1 |
| T2 | price_percentile lookback:0 | internal/strategy/price_percentile | T1 | 2 |
| T3 | pe_percentile lookback:0 | internal/strategy/pe_percentile | T1 | 2 |
| T4 | ValuationConfig | internal/config | — | 1 |
| T5 | app valuation lookback 可配 | internal/app | — | 1 |
| T6 | serve 装配 | cmd/atlas | T2,T3,T4,T5 | 3 |
| T7 | 全史 dump+config+文档 (best-effort) | scripts/qlib_warehouse | T6 | 4 |

## 团队规划（确认门后 spawn）
- dev-agent-1: T1→T2→T6→T7
- dev-agent-2: T4→T3
- dev-agent-3: T5
- test-agent-1 / test-agent-2

## 校验
- 手工 validator 全绿：DAG/wave/scope/依赖/DoD≤8/单owner。
- DoD 矩阵：孤儿 0 / 凭空 0。
- 诚实边界：lixinger PE 分位上限 10 年（A 股个股+指数）；数据需重 dump 全史。
