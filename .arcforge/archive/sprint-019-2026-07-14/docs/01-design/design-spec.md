# 设计规格（指针文档）

> 本 Sprint 不重复誊抄设计——以下两份文档即规格本体，Dev/Test/QA 开工前必读：

1. **`docs/plans/2026-07-13-macro-crisis-monitor-impl.md`**（实施方案，唯一需求源）
   - 「核心接口契约」（行 79–189）：跨任务公共签名，**后续任务以此为准**。
   - 「存储单位约定」（行 23–37）：canonical units 表。
   - 「对设计 v0.2 的三处已核实偏差」（行 39–43）：ts 列 / typed config / 分位最小窗 60。
   - 「状态机语义补充」（行 191–195）。
   - 各任务节含完整 TDD 步骤与参考实现代码（执行时按节内 checkbox 逐步走）。
2. **`docs/plans/atlas-macro-crisis-monitor-design.md`**（设计 v0.2）：规则表 §3.1、抑制 §3.2、状态机 §3.3、架构 §4、边界 §5、验收 §6。

## 架构摘要

- 新增 `internal/collector/fred`（API 客户端）与 `internal/crisis`（types/dates/store/config/derive/suppress/rules/statemachine/memhistory/eval/ingest/notify）。
- CLI 子命令平铺 `cmd/atlas/crisis.go`；配置 `configs/crisis-monitor.yaml`；部署 `deploy/launchd/*.plist`。
- 规则引擎与状态机为**纯函数**（SeriesReader/EvalHistory 窄接口），live 评估与 replay 回测共用。
- sqlite（WAL，惯例照抄 `internal/storage/signal/sqlite.go`）为唯一真相源，进程无状态。

## 任务节行号索引（impl 文档内）

| Task | 节 | 行号 |
|---|---|---|
| 1 | crisis 基础类型/日期/Store | 203–909 |
| 2 | FRED 采集器 | 910–1138 |
| 3 | yahoo JPY=X | 1139–1200 |
| 4 | 配置与加载 | 1201–1517 |
| 5 | derive.go | 1518–1654 |
| 6 | ingest.go | 1655–1973 |
| 7 | CLI + backfill（一阶段收口） | 1974–2260 |
| 8 | suppress.go | 2263–2473 |
| 9 | rules.go | 2474–2958 |
| 10 | statemachine + memhistory | 2959–3319 |
| 11 | eval.go | 3320–3529 |
| 12 | eval/status 子命令 | 3530–3888 |
| 13 | replay 回测（二阶段收口） | 3889–4046 |
| 14 | notify.go + telegram | 4049–4335 |
| 15 | intraday + launchd（三阶段收口） | 4336–4583 |
