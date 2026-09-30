# 设计规格（指针 + 增量）

设计正文即 `docs/superpowers/specs/2026-09-30-tiingo-source-design.md`（人类 2026-09-30 已确认），实现草稿即 `docs/superpowers/plans/2026-09-30-tiingo-source.md`。本文件只记**相对二者的增量**，不复述。

## 计划已声明的有意差异（沿用）
1. spec §5「直连」测试改为结构断言 `Transport.Proxy == nil`（loopback 恒不走代理，原写法测不出东西）。
2. spec §3.6「订正 tushare 2nd hop / baostock 3rd hop 日志」不做（有序化后 A 股兜底顺序仍是 yahoo 先于 tushare，该说法仍不字面成立）——**在 DoD 确认门向人类显式列出**（AD-15）。
3. `normalize` 对 O/H/L 任一缺失的行也不输出；该行 `splitFactor` 仍参与累乘。

## Leader 增量
- `cmd/atlas/gate_wiring_test.go` 的 `collectorCtors` 登记从计划 Task 5 挪到 TASK-007（包边界：TASK-005 只写 tiingo 包）；并如实注明它**不守护** prism/serve 的接线（AD-11）。
- 真正的接线性质（配额进跨进程账本）由三条入口「先装配后构造」保证，已由 Leader 读代码核实（requirements-analysis 前提核对第 4 条）；TASK-007 DoD 要求 dev 在 discovery 中复核并给出行号。
- 路由表加密前缀导致的美股假阴（AD-12）、watchlist 也获得 tiingo 兜底（AD-13）：接受，不改，记入 final-report。

## 任务图（DAG，scheduling=dag）
```
TASK-001 Registry 顺序 ─────────────────────────────┐
TASK-002 policy tiingo.daily（独立，无下游代码依赖）   │
TASK-003 symbols ─► TASK-004 client ─► TASK-005 collector ─► TASK-007 serve 装配 + 配置示例
                          └──────────► TASK-006 prism 多跳 ─► TASK-008 集成测试
                          └─────────────────────────────────► TASK-008
```
