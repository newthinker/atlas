# TASK-002 验证报告 — App.SnapshotMetrics 只读指标组装 + FundamentalSource 注入

- 验证者: test-agent-1
- 判定: **VERIFIED (PASS)**
- commit: 9e9a431 `feat(watchlist-cmd): add App.SnapshotMetrics read-only metrics assembly`
- 验证时间: 2026-07-03

## Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 对应测试/证据 | 判定 |
|---|---|---|---|---|
| F1 | 全路径 A 股 行情/估值三项/PE 百分位/价格百分位就位且值精确 | test | TestSnapshotMetrics_FullPathAShare PASS。精确断言 PE=19.5/PB=5.96/DividendYield=4.03/PEPercentile=12.3/PricePercentile=100/Gaps=0，非空洞 | PASS |
| F2 | 无估值源估值全 nil+价格百分位仍可算+Gaps 有记录; crypto 不记估值 gap | test | _MissingValuationDegrades(PE/PB/DY/PEPct 全 nil, PricePct 非 nil, Gaps>0) + _CryptoNoValuationGap(估值 nil, 无含 valuation/fundamental 的 gap 串, PricePct 可算) 均 PASS | PASS |
| F3 | symbols 过滤保 watchlist 顺序; ≥3 标的并发保序(B4) | test | _SymbolsFilter(过滤到 MSFT) + _OrderPreservedConcurrent(5 标的 AAA..EEE, 断言 Workers≥2, 逐位 ms[i].Symbol==order[i]) 均 PASS | PASS |
| F4 | 单标的 panic 隔离记 gap 其余不受影响; 全采集器失败指标空但 Gaps 有因 | test | _PanicIsolated(BAD panic→gap, GOOD.Price=10 不受影响, 2 items) + _AllCollectorsFail(Price=0/PricePct nil/Gaps>0) 均 PASS | PASS |
| B1 | 只读: 不产信号/不发通知/不写 store | review | snapshot.go grep Save/Store/Notify/signal 仅命中注释；组装链仅调 FetchQuote/FetchHistory/buildFundamental/FetchFundamental/PercentileRank（皆读操作）；无 store/notify 写副作用 | PASS |
| B2 | 价格百分位极值(PercentileRank 严格小于): >全部→100; ==最小→0; 内部→50 | test | _PricePercentileExtremes 三子用例全 PASS。核 valuation.PercentileRank 实现: less=count(v<current), less/len*100（严格 <）——200→4/4=100, 100→0/4=0, 115→2/4=50，数学正确 | PASS |
| N1 | 接口签名与计划 Interfaces 块逐字一致; App 仅加 fundamentalSrc 一字段 | review | FundamentalSource/SetFundamentalSource/SymbolMetrics(全 12 字段+json tag)/SnapshotMetrics 与计划 line 121-140 逐字比对一致；app.go diff 仅加 `fundamentalSrc FundamentalSource` 一字段(另一行为 gofmt 对齐) | PASS |
| N2 | app 包全绿; -race -run TestSnapshotMetrics 无 race; Workers=1 vs 4 逐元素相等(B3) | test | go test ./internal/app/ 全绿(分析循环零回归); go test -race 无 data race; _ConcurrentEquivalence 比对 Symbol/Price/ChangePct/PricePercentile/PEPercentile 逐元素(非仅长度) PASS | PASS |

## 测试运行证据
- `go test -v ./internal/app/ -run TestSnapshotMetrics`: 9 个测试函数(含 3 子用例)全 PASS
- `go test -race ./internal/app/ -run TestSnapshotMetrics`: ok，无 data race
- `go test ./internal/app/`: ok(既有分析循环测试零回归)
- `go build ./...`: 干净
- `go test ./...`: 全仓离线全绿(无一失败)

## 覆盖率
- 变更包 internal/app: **95.8%**（超 ≥80 门槛）
- snapshot.go 新增函数: SetFundamentalSource/SnapshotMetrics/snapshotItems/snapshotSymbolSafe = 100%; snapshotSymbol 91.7%

## 非阻断观察(不影响判定)
- snapshot.go 三处防御/替代分支未覆盖: snapshotHistoryStart lookback>0 路径(66.7%)、positivePtr 非正值分支(66.7%)、snapshotSymbol 一个 error 分支(91.7%)。均非 8 条 done_criteria 要求的测试场景，包级覆盖 95.8% 远超门槛，记为观察，不阻断。
- B3 _ConcurrentEquivalence 比对 5 个载荷字段(Symbol+全部并发敏感数值)而非 reflect.DeepEqual；因该场景无 FundamentalSource(PE/PB/DY 恒 nil)且 Name/Market/Type/Gaps 确定性，足以捕获并发错位/竞态，非空洞。

## 反向验收
- packages(internal/app)与 estimated_files(snapshot.go/snapshot_test.go/app.go)一致，无越界。
- 只读语义经 review 实证，无 YAGNI 误实现(未产信号/未发通知)。

## 结论
8/8 done_criteria PASS，AD-8 三项增补(B3 并发等价/B4 保序/B5 极值)逐条实证，-race 无竞态，接口签名逐字一致。判定 VERIFIED。
