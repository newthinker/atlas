# GitNexus detect-changes（compare vs master）—— ⚠ Risk: CRITICAL（如实上报，未自行豁免）

- 命令：`npx -y gitnexus@latest detect-changes --scope compare --base-ref master --repo atlas`（`.gitnexus/run.cjs` 的 pnpm runner 报 MODULE_NOT_FOUND，本 sprint 全程改用 npx）
- 索引：重建到 `b3232d6f3e2b07610bafe31e7381add623fdb442`（meta.json `lastCommit` 直读核实）
- 输出摘要：**28 files / 123 symbols / 518 affected processes / Risk level: critical**；未见 partial/truncated 标记
- git 对照：`git diff --name-only master...HEAD` = **27** 个文件（25 代码/配置 + spec + plan）；工具多 1 个，来源未查明（上一 sprint 同样差 1）

## 成分分析（Leader，不构成豁免）
1. 大量受影响流程标注 `changed: order`（如 `RefreshLixinger → WindowStart`、`FetchQuote → Take`）。本 sprint 新增的 `order` 标识符**只有 `Registry.order` 一处**（diff 中 7 次出现全在 `internal/collector/registry.go`），而 `internal/broker/*`、`internal/core/errors.go`、`internal/app/app.go` 等处本就有同名 `order` ⇒ **疑似按名字匹配把无关的 `order` 串进来**；未能从工具内部证实。
2. 真实的语义变化只有两类，均已在开发期上报人类：`Registry.GetAll` 顺序由随机变为注册序（影响 orderedCollectors / SelectExternalForSymbol / buildArbitrator / backtest `collectors[0]` / api/server.go，test-tg-a 逐个读码核实前后行为）；`policy.NewTable` 新增 `tiingo.daily` 主题（纯新增，既有主题零改动）。
3. 开发期多次 impact 查询对 prism 符号失效（按名 CRITICAL 210–229、按文件 not found、混入 build_warehouse.py），dev 判 UNKNOWN 并以 grep 补查，验证者独立复核一致。

## 原始输出
```

  GitNexus Detect Changes (1.6.12)
Changes: 28 files, 123 symbols
Affected processes: 518
Risk level: critical

Changed symbols:
  Function collectorSeq → cmd/atlas/collectors_test.go
  Function buildSeq → cmd/atlas/collectors_test.go
  Function TestBuildCollectors_RegistersTiingoLast → cmd/atlas/collectors_test.go
  Function TestBuildCollectors_TiingoBeforeQlib → cmd/atlas/collectors_test.go
  Function TestBuildCollectors_SkipsTiingoWhenUnconfigured → cmd/atlas/collectors_test.go
  Function base → cmd/atlas/collectors_test.go
  Function TestExampleConfigDeclaresTiingo → cmd/atlas/collectors_test.go
  Variable collectorCtors → cmd/atlas/gate_wiring_test.go
  Function usPriceHops → cmd/atlas/prism.go
  Function TestUSPriceHopsOrderAndSkips → cmd/atlas/prism_test.go
  Function names → cmd/atlas/prism_test.go
  Section Tiingo 美股备用数据源 Implementation Plan → docs/superpowers/plans/2026-09-30-tiingo-source.md
  Section Global Constraints → docs/superpowers/plans/2026-09-30-tiingo-source.md
  Section Review Focus → docs/superpowers/plans/2026-09-30-tiingo-source.md
  Section File Structure → docs/superpowers/plans/2026-09-30-tiingo-source.md
... and 108 more

Affected execution flows:
  • RefreshLixinger → WindowStart (10 steps) — changed: order, order, order, order, order, order
  • RefreshLixinger → LedgerEntry (10 steps) — changed: order, order, order, order, order
  • FetchHistory → Take (10 steps) — changed: order, order, order
  • FetchQuote → EvictOldest (10 steps) — changed: order, order, order
  • FetchQuote → CacheEntry (10 steps) — changed: order, order, order
  • FetchQuote → Take (10 steps) — changed: order, order, order
  • FetchQuote → Take (10 steps) — changed: order, order, order
  • FetchQuote → Take (10 steps) — changed: order, order, order
  • FetchQuote → DomainState (10 steps) — changed: order, order, order
  • RefreshLixinger → Lock (9 steps) — changed: order, order, order, order
... and 508 more
```
