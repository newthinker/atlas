# Changelog — Sprint 018 atlas watchlist 指标命令（2026-07-03）

## Added
- `atlas watchlist [--json] [--symbols A,B]`：离线输出 watchlist 行情/估值/百分位（CJK 对齐表格 / JSON 数组，缺失指标 `—`/null，gaps 摘要；空表提示 exit 0、全失败 exit 1）（#44, TASK-004）
- `internal/app`: `SnapshotMetrics`（只读指标组装：errgroup 并发保序、panic 隔离、串并行等价）、`FundamentalSource` 窄接口 + `SetFundamentalSource`（#44, TASK-002）
- `internal/text`: `DisplayWidth`/`PadRight` 共享 CJK 宽度包（#44, TASK-001）
- `cmd/atlas`: `buildCollectors` 共享装配（serve 与离线命令同源；cleanup 托管 qlib 句柄）（#44, TASK-003）

## Changed
- telegram 表格渲染改用 `internal/text`（函数体逐字迁移，行为零变化）（#44, TASK-001）
- `MemoryStore`—无（本轮未涉及）；serve.go 装配段迁出为 `buildCollectors` 调用（行为零变化）（#44, TASK-003）

## Fixed
- **eastmoney A 股涨跌幅显示放大 100 倍**（f170 为百分比×100 未换算；watchlist 与 web UI symbol_detail 均受益）（#44, TASK-005 / QA-W1）
- watchlist 估值字段掩盖合法值：0 股息率与负 PE 现如实显示，不再与"数据不可用"混淆（#44, QA-W2）
- watchlist `allFailed` 判据漏检 PB/股息率导致可能误判全失败 exit 1（#44, QA-W4）
- 命令 help 文案去除"exact pipeline"过度承诺，注明百分位窗口基准为 `valuation.lookback_years`（#44, QA-W3b）
