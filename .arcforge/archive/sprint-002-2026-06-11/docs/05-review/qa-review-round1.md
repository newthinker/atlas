# QA 终审 · 第一轮（常规 Code Review）— ATLAS sprint-002

- 审查者: qa-agent-1（Reality Checker：默认 NEEDS WORK，PASS 须带证据）
- 范围: `git log 80c43df..HEAD` 12 commit / 34 files / +2284 行
- 依据: 施工图 rev3、design rev6、各 TASK done_criteria、issues.md(ISSUE-1/CARRYOVER I3)

## 0. 硬性门禁结果（证据）

| 项 | 命令 | 结果 |
|---|---|---|
| 静态检查 | `go vet ./...` | clean，无告警 |
| 全量测试 | `go test ./...` | 全部 ok（sprint-002 涉及包全绿） |
| 竞态 | `go test -race ./internal/{app,collector/...,valuation,strategy/...}` | sprint-002 包全 PASS 无 race |
| 竞态-噪声 | 同上 | 仅 crypto binance/coingecko/okx **集成**测试 FAIL=真实网络 EOF（sandbox 无外网），非本 sprint 改动，判定为环境噪声 |
| gitnexus | MCP `gitnexus_detect_changes()` | **本 QA 会话工具集未提供 gitnexus MCP 工具**；以 `go vet`+`go test`+`-race`+人工 diff 复核替代，变更范围与 plan 声明的 34 文件一致，无越界符号 |

## 1. 正确性

### 1.1 分位数学口径（strictly-less）— PASS
`internal/valuation/percentile.go:7-18`：`v < current` 计数 / `len` × 100；空序列返回 -1。
- all-equal→0、single→0、empty→-1，与 design §6 及 `percentile_test.go` 用例一致。**证据**：`TestPercentileRank` 全过。

### 1.2 PE 重建阶梯对齐 + 边界 — PASS
`reconstruct.go:26-79`：
- EPS 升序拷贝排序（不改入参）；正点 < `MinEPSPoints(8)` → `ErrInsufficientEPS`（reconstruct.go:39）。
- 当前 EPS = 末点；≤0 → `ErrNonPositiveEPS`（:46）。
- 逐 bar `latestEPSAtOrBefore`（sort.Search `Date.After(t)`，边界日含当日）对齐，剔除 EPS≤0 日。
- **load-bearing 守卫**（:62-64）：剔除后 peSeries 为空 → 返回 `ErrInsufficientEPS`，**杜绝 PercentileRank 的 -1 带 nil error 冒充成功**。符合 plan Task 8 第 4 点硬约束。**证据**：`TestReconstructPEPercentile_*` 全过。

### 1.3 双哨兵错误语义传播 — PASS
`app.go:759-769` buildFundamental：`rerr==nil`→reconstructed；`ErrNonPositiveEPS`→直接 return（**不兜底**，真实亏损）；其余（ErrInsufficientEPS 等）→落理杏仁兜底。`fallbackReason()`(:785) 正确编码 `yahoo_eps_insufficient/yahoo_eps_error`。与 design §3.2/§5 一致。

### 1.4 兜底链路径完备性 — PASS
buildFundamental(`app.go:718-781`) 路径表逐条核对 plan Task 13：
- CN 股/指数 + 美港指数 → 理杏仁唯一路径；valuationSrc==nil → warnOnce + PEPercentile=-1。
- 美港个股 → Yahoo 重建主路径；epsSrc==nil 视为"主路径不可用·数据缺失"→ 理杏仁兜底（Source=`lixinger_cvpos:yahoo_not_configured`）。
- 商品/加密/基金 → `at` 非 stock/index → 返回 nil（无估值路径）。
全部与 plan/design 对齐，无遗漏分支。

### 1.5 ISSUE-1 全局 StatusCode 复查 — PASS（重点项）
- `lixinger/valuation.go:152-154` `postJSONRaw`：Do 后 Decode 前 `if resp.StatusCode != 200 { error }`。
- `yahoo/eps.go:62-64` `FetchEPSHistory`：同样 StatusCode 守卫先于 Decode。
两条本 sprint 新增 HTTP 路径均已闭合 ISSUE-1「合法 JSON+非 200 被当成功」缺陷。`postJSONRaw` 注释明确"non-200 是错误，与 body 无关"。

## 2. 并发安全

- `warnOnce`(`app.go:645-650`)：`sync.Map.LoadOrStore` 去重，并行 loop 安全；告警 key 按 (symbol,strategy)/(symbol) 维度。
- 并行 worker(`runAnalysisCycle:300-314` errgroup.SetLimit) 内 `analyzeSymbolSafe` panic 隔离；`buildFundamental` 仅用局部 `f` + 无状态 HTTP client；executor/arbitrator 经 `a.mu.RLock` 快照读取。**证据**：`-race` 全过。
- ⚠ 见 round2 运维视角 S1：`valuationSrc/epsSrc` 读取未走 `a.mu`（set-once 故当前无 race，但与 executor 模式不一致）。

## 3. 与 plan 的偏差（逐 Task 对照）— 无material偏差
- T1 core 类型：AssetCrypto/EPSPoint/PEPercentile 均按 plan 落地（types.go:25 等）。
- T2/T3 yahoo：validSymbol 正则含 ^/=F；URL `url.PathEscape`；局部变量改名 `reqURL` 避免遮蔽 net/url；FetchEPSHistory 指数符号前置拒绝。
- T4/T5 indexes/selector：共享表 `AShareIndexSecIDs`、`IsAShareIndex`、`KnownIndexMarket` 导出，路由/市场归属按表；^HSI→HK。
- T6 DetectType/assetTypeOf：与 plan 完全一致；DetectMarket 补 ^HSI→H股保证 UI 与 collector 归属一致。
- T7 lixinger endpointFor：cn/hk/us × company/index 分派；HK `%05s` 补零（已验证产出 `00700`，非空格）。
- T8 valuation：见 1.1/1.2。
- T9/T10 双策略：classify 边界、confidence 区间、metadata(method/fallback_reason 经 strings.Cut 拆解) 与 plan 一致；**未抽公共基类**（符合 plan 显式要求）。
- T11 既有策略 AssetTypes：ma_crossover 6 类、pe_band/dividend_yield [stock]。
- T12/T13 装配：effectiveStrategies 过滤+表外指数 warning；historyWindowDays `maxBars*365/252+30`（5×252→1855≥1825 满足 5 年覆盖）；buildFundamental 见上。
- T14 cmd/config：serve 注册双策略 + typed-nil 守卫 `valuationSourceOrNil/epsSourceOrNil`；backtest 仅注册 price_percentile；config 删除 pe_band 失效的 lookback_years/threshold_percentile，新增双策略与 watchlist 示例；README 更新。

## 第一轮结论
无 CRITICAL。核心正确性（分位口径 / PE 重建 / 双哨兵 / 兜底链 / StatusCode）均带证据 PASS。
遗留 1 WARNING(W1) + 1 SUGGESTION(S1) 见 round2 详述。
