# 设计规格 — atlas watchlist 指标命令

> 权威文档: 实施计划 `docs/superpowers/plans/2026-07-03-watchlist-metrics-command.md`（含完整测试与实现代码）
> 与上游 spec `docs/superpowers/specs/2026-07-03-watchlist-metrics-command-design.md`。
> 本文件只记执行要点，冲突时以实施计划为准（计划已消解 spec 的 JSON 歧义）。

## 接口契约（跨 Task，签名必须逐字一致）

- T1 → T4: `text.DisplayWidth(s string) int`、`text.PadRight(s string, width int) string`
- T2 → T3/T4:
  - `type FundamentalSource interface { FetchFundamental(symbol string) (*core.Fundamental, error) }`
  - `func (a *App) SetFundamentalSource(fs FundamentalSource)`
  - `type SymbolMetrics struct {...}`（指针字段 nil=不可用；Gaps 记录原因；json tag 见计划）
  - `func (a *App) SnapshotMetrics(ctx context.Context, symbols []string) []SymbolMetrics`（symbols nil=全表；保持 watchlist 顺序）
- T3 → T4: `buildCollectors(cfg *config.Config, application *app.App, log *zap.Logger) (cleanup func(), err error)`（err 当前恒 nil，签名按 spec 预留）

## 关键语义

- SnapshotMetrics 只读：不产信号、不发通知；workers 取 cfg.Analysis.Workers（≥1）；per-symbol panic 隔离进 Gaps。
- 行情/历史按 orderedCollectors 依序试（与分析循环同路由）；历史窗口 = valuationLookback（0=全历史，clampToEpochFloor）。
- PE 百分位复用 buildFundamental（资产类型门控内置：crypto/商品/基金返回 nil = 预期缺席不记 gap）。
- 估值三项（PE/PB/DividendYield）仅在 FundamentalSource 注入时可得（现状 = A 股 lixinger）；`positivePtr` 过滤非正值。
- 价格百分位 = 现价（缺行情退回最后收盘）在窗口收盘序列的 `valuation.PercentileRank`。
- T3 迁移三变量（yahooCollector/lixingerCollector/qlibWarehouseDB）均在被迁段内声明，段后无引用（已核实）；
  新增①lixinger 非 nil 时 SetFundamentalSource（typed-nil 防护同 valuationSourceOrNil）②cleanup 关 qlib 句柄。
- T4：stdout 只出表格/JSON（logger 走 stderr，WarnLevel）；缺失指标表格显示 `—`、JSON 为 null；
  --symbols 未知标的 warn 后跳过、全未知报错；全标的失败报错（退出非零）；空 watchlist 提示不报错；
  表格末列不补尾空格（对齐 telegram 约定）；表后打印 `! SYMBOL: gaps` 摘要。

## 验证锚点

每 Task 的测试代码在计划中逐字给出（T2 六用例 / T3 两用例 / T4 七用例 / T1 两用例），
Dev 按 TDD 先落测试（RED）再实现（GREEN）；`go test ./...` 全程离线全绿。
