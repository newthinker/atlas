# TASK-010 验证报告 — app 资产类型识别 + AssetTypes 绑定校验 + 动态历史窗口

- **验证人**: test-agent-1 (Reality Checker)
- **日期**: 2026-06-11
- **被验 commit**: 244280f `feat(app): detect index/futures types, asset-type binding validation, dynamic history window`
- **包**: ./internal/app ｜ coverage_minimum=80 (default)
- **施工图**: plan rev3 Task 6 + Task 12 ｜ 复杂度: complex
- **判定**: ✅ VERIFIED

## 测试执行证据
- `go test ./internal/app/ -race -count=1 -cover` → **PASS, coverage 95.5%** (≥80)，**race 干净**（warnOnce sync.Map 并发安全）。
- `go build ./...` exit 0；`go vet ./internal/app/` exit 0。
- `go test ./...` 全量 exit 0，**47 包全 ok，零 FAIL/panic/DATA RACE**（消费方零回归）。

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---------|---------|------|
| functional[0] | DetectType 全用例：^GSPC/^HSI/000300.SH/000001.SH→指数；000001.SZ/600519.SH/AAPL→股票；GC=F→期货；BTC-USDT→加密 | TestDetectType_IndexAndCommodity（9 例表驱动，含 000300.SH 指数 vs 000001.SZ 股票判别——依赖 collector.IsAShareIndex 正确区分，实测通过） | PASS |
| functional[1] | assetTypeOf 七映射(含 TypeBond→空)；DetectMarket(^HSI)→H股 | TestAssetTypeOf（7 例含 Bond→""）+ TestDetectMarket_HSI | PASS |
| functional[2] | effectiveStrategies：stock_only 对 GC=F 过滤、AssetTypes 空=不限保留；同一(symbol,strategy)二次调用仅 1 warning | TestEffectiveStrategies_FiltersByAssetType（zaptest observer 捕获日志：filter 后 [all_assets]；二次调用后 skip-warning 计数 ==1 → 真实 dedup 验证） | PASS |
| functional[3] | historyWindowDays：5*252 bars→≥1825 天；无策略→365 | TestHistoryWindowDays（5*252→实算 1855≥1825；空策略→365） | PASS |
| boundary[0] | Strategies 非空但 effective 空→analyzeSymbol 直接返回不分析；表外 ^ 绑定 warnOnce | TestAnalyzeSymbol_AllFilteredReturnsEarly（stock_only 绑 GC=F，该策略**本会发 buy 信号**，断言 notifier 收到 0 信号→证明真正短路而非空返回）+ TestEffectiveStrategies_UnknownIndexWarnsOnce（^N225 出表，warning 计数 ==1） | PASS |
| error_handling[0] | 未注册策略名透传给 engine 报错路径(不在过滤层吞) | TestEffectiveStrategies_UnregisteredPassThrough（"ghost" 未注册→effective 含 "ghost"） | PASS |
| non_functional[0] (verify_by:test) | internal/app 全包 -race 通过(warnOnce 并发安全) | -race -count=1 PASS，无 DATA RACE | PASS |

## 反 fantasy-assertion 专项核查（complex 任务重点）
- **dedup 断言非空洞**：functional[2]/boundary[0] 的「仅 1 warning」用 `zaptest/observer` 真实捕获日志条数并 `FilterMessage(...).Len()==1` 断言——能捕获 warnOnce 失效（若去重失效计数会变 2）。非占位。
- **早返回断言有行为证据**：TestAnalyzeSymbol_AllFilteredReturnsEarly 故意让被过滤策略**携带可触发的 buy 信号**，以 notifier 收信数==0 反证「分析被真正跳过」，而非只检查返回值——强行为级断言，能捕获「过滤未短路仍执行分析」的回归。
- **race 真实执行**：用 `-count=1` 绕开缓存，确认 race detector 实跑于 warnOnce/sync.Map 路径。
- 无 HTTP 路径，ISSUE-1 不适用。

## 生产代码与 plan 一致性
- DetectType 重写为 switch（^/IsAShareIndex→指数；=F→期货；crypto；default 股票），与 plan Task 6 一致。
- historyWindowDays 用 `maxBars*365/252+30`（折算系数 365/252，含 plan 注释说明 ×7/5 漏节假日的缺陷规避），1260 bars→1855≥1825。
- 用标准库 `slices.Contains` 替代 plan 骨架自建 contains（go 1.24，合理简化，单一真相源）。
- 未绑定 strategies 项保持既有全策略 + 365 天行为（plan 行为边界），未被破坏（全量回归零失败佐证）。

## 结论
7 项 done_criteria 全部 PASS，含 dedup/早返回两处易出 fantasy assertion 的点均以真实日志捕获 + 行为副作用断言佐证，-race 干净，47 包零回归。判定 **VERIFIED**。
