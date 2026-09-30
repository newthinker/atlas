# 需求分析（sprint 2026-09-30 Tiingo 美股备用数据源）

- 需求源：`docs/superpowers/specs/2026-09-30-tiingo-source-design.md`（人类已确认设计）+ `docs/superpowers/plans/2026-09-30-tiingo-source.md`（8 任务计划，含测试与实现草稿）
- 分支：`feature/tiingo-source`，起点 `8d1c6cc7edf573c9879b53125f486eb6d515f01f`
- 复杂度：中等（1 个新包 + 既有符号改动：`Registry.Register/GetAll`、`prism.Refresh/refreshEngine/refreshEdgar/fetchCloses`、`buildCollectors`、`runPrismRefresh`）

## 需求清单（追溯矩阵的行）

| R | 需求 | 出处 |
|---|---|---|
| R1 | `Registry.GetAll` 按首次注册顺序返回（确定性）；同名重注册覆盖实例且保持原位置 | spec §3.6 |
| R2 | policy 内置主题 `tiingo.daily`：Domain tiingo、`Quota{40, 1h}`、Coalesce、`TTL=builtinTTL`、无 MinInterval | spec §3.4 |
| R3 | 白名单 `Supported`：仅美股/ETF 形态；A 股、港股、指数、期货、加密、空串、小写、超长拒绝 | spec §3.2 |
| R4 | `toTicker`：`.`→`-`（BRK.B→BRK-B） | spec §3.2 |
| R5 | 请求形状：`GET {base}/tiingo/daily/{ticker}/prices?startDate=`，**不带 endDate**，`Authorization: Token <key>`，token 不进 URL | spec §3.1 |
| R6 | 强制直连：Transport `Proxy == nil` | spec §3.1 / §7 |
| R7 | 拆股折算（t 日 OHLC ÷ t 之后因子积，Volume × 积取整），折算在截取前对全量行做；`[start,end]` 闭区间、升序、`Interval=1d`、返回 Clone | spec §2.4 / §3.1 |
| R8 | 坏行跳过；`splitFactor` 非有限或 ≤0 整段失败；缺价行的因子仍参与累乘 | spec §3.1 / §4；计划有意差异 3 |
| R9 | HTTP 状态映射（404 / 400·401·403 / 429 / 其他）；`wrapErr` 前缀 `tiingo: `、token→`<redacted>`、`%v` 断链 | spec §3.1 / §4 |
| R10 | gate 超额/超时映射为含 `retryable` 的临时错误；超额不发 HTTP | spec §3.1 / §3.4 |
| R11 | `Collector`：Name/SupportedMarkets/Init(空 key 报错)/Start/Stop；非 `1d` 拒绝；不支持代码拒绝且**不发请求** | spec §3.3 |
| R12 | `FetchQuote`：最近约 10 天日线末两根拼装 Price/PrevClose/Change/ChangePercent/OHLV/Time/Source=`tiingo_eod`/Market=US；单根时 PrevClose/Change* 为 0；空则报错 | spec §3.3 |
| R13 | prism `PriceHop` 有序多跳：yahoo 成功直返；逐跳尝试、零行视为失败；首个成功跳文案；全失败错误串按跳列原因；`hops` 为空时 `price history: %w` 与现状逐字一致 | spec §3.5 |
| R14 | `usPriceHops`：tiingo(enabled+key) → twelvedata(有 key)；未配置不入链、无 typed-nil | spec §3.5 |
| R15 | serve 装配：tiingo(enabled+key) 注册在 tushare/baostock 之后、qlib 之前，日志 `tiingo collector registered (US price last hop)`；未启用/缺 key 不注册 | spec §3.6 |
| R16 | `configs/config.example.yaml` 增加 `collectors.tiingo` 示例（key 留空）与 TTL 建议 | spec §3.7 |
| R17 | 集成测试（`//go:build integration`，`ATLAS_TIINGO_TOKEN`，缺失 Skip）：AAPL、BRK.B 近 30 天非空、无 NaN/非正；prism 降级演练 | spec §5 |
| R18 | 未启用或缺 key：prism 与 serve 行为与现状逐字一致 | spec §3.7；计划 Global Constraints |
| R19 | 上线验证（runtime 配置、部署、重启、次日报告）——**人类动作，不入 DoD**（AD-9） | spec §6 |

## Leader 前提核对（2026-09-30，于 `8d1c6cc`）
1. `Refresh` 生产调用方仅 `cmd/atlas/prism.go:202`；测试里传非 nil `td` 的只有 `refresh_test.go:1189`、`:1401` 两处 —— 与计划一致。
2. `fetchCloses` 调用方 `refresh.go:277`（refreshEngine）、`:664`（refreshEdgar）。
3. `GetAll` 生产调用方：`app.go:557`（orderedCollectors）、`:649`（计数）、`:953`（GetCollectors）、`selector.go:66`（SelectExternalForSymbol 最后兜底，仅在 route 与 yahoo 都缺席时生效）。
4. **闸门接线**：prism 经 `loadConfigOrDefaults`（`export_ohlcv.go:297` 内 `initPolicyGate`）、serve（`serve.go:85`）、watchlist（经 `loadConfigOrDefaults`）都在构造 collector 之前装配 ⇒ tiingo 的 40/h 配额会进跨进程文件账本。见 AD-11。
5. **`gate_wiring_test.go` 的 `collectorCtors` 只在 crisis/backtest 三个入口函数体内按 `pkg.Fn` 字面名扫描** ⇒ 计划加的 `"tiingo.New"` 不守护 prism（调用点在 `usPriceHops` 内）也不守护 serve（调用名是 `tiingo.NewCollector`）。见 AD-11。
6. 路由表加密前缀（`UNI*`、`LINK*`、`ADA*`、`DOT*`、`SOL*`、`ATOM*`、`LTC*` 等，`route.go:55-67`）会把部分真实美股代码判为 crypto ⇒ `Supported` 对它们返回 false（不耗配额，只是漏覆盖；既有路由行为，不改）。见 AD-12。
7. `atlas watchlist` 同样调用 `buildCollectors` ⇒ 也会获得 tiingo 兜底（spec 只写了 prism + serve）。见 AD-13。
8. 基线覆盖率（`8d1c6cc`，`-func` / profile 逐块求和）：internal/collector 100.0 / 100.00；policy 94.4 / 94.42；prism 94.0 / 93.95；**cmd/atlas 79.4 / 79.27（< 80）**。
