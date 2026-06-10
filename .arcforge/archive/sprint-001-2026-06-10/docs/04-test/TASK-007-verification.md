# TASK-007 验证报告 — serve.go 接线 OHLCV 缓存装饰器

- 验证者: test-agent-2 (Reality Checker)
- packages: ./cmd/atlas
- 覆盖率门禁: 任务级 coverage_minimum=35（ISSUE-3）
- commit: 76c1d87

## 实跑证据（亲自复跑）
- `go test ./cmd/atlas/ -race -count=1` → **ok**，4 个 TASK-007 用例全 PASS（整包亦 ok，含 TASK-003 既有 e2e + broker_test 重构，无回归）
- `go test ./cmd/atlas/ -cover` → **45.4% ≥ 35 floor** ✅
- `go build ./...` exit=0；`go vet ./cmd/atlas/` clean

## 判定: VERIFIED ✅

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 断言核验 | 判定 |
|---|---------|------|---------|------|
| functional[0] | cache.enabled=true 普通 collector 被 CachedCollector 包装（TTL 来自配置） | TestMaybeCache_EnabledWrapsPlain | maybeCache(plain,true,5m) 结果断言为 `*collector.CachedCollector`，且 Name()=="yahoo"、SupportedMarkets()==[US] 透传 | PASS |
| functional[1] | cache.enabled=false 原样注册不包装 | TestMaybeCache_DisabledNoWrap | 结果非 *CachedCollector 且 `got == collector.Collector(plain)`（同实例） | PASS |
| **boundary[0]** | FundamentalCollector 扩展接口 collector 不被包装破坏断言路径（须真实测试） | TestMaybeCache_FundamentalNotWrapped | ① 结果非 *CachedCollector；② **`got.(collector.FundamentalCollector)` 断言仍成立**（真实类型断言，非口头声明）；③ 同实例返回 | PASS |
| non_functional[0] (verify_by:test) | 包装后 SelectForSymbol 市场匹配不变（Name/Markets 透传） | TestMaybeCache_SelectorRoutingUnchanged | 包装 crypto 注册到 registry，SelectForSymbol("BTCUSDT") 路由到 "crypto" 且返回被包装实例 | PASS |

## 关键核查（应 Leader 要求）
1. **类型断言路径真实被测，非口头声明**：boundary[0] 的 TestMaybeCache_FundamentalNotWrapped
   用 fundamentalCollectorStub（实现 Collector + FundamentalCollector）经 maybeCache 后，
   **实跑断言 `got.(collector.FundamentalCollector)` 为真**——直接证明断言路径未被包装破坏。
2. **测试非空洞（non-vacuous）**：核查 internal/collector/cache.go，`CachedCollector` 仅嵌入
   `Collector` 接口、无 `FetchFundamental` 方法 → 若 fundamental collector 被包装，断言必失败。
   故 maybeCache 的 FundamentalCollector 守卫是必要的，boundary 测试有真实判别力。
3. **enabled=false 零包装**：functional[1] 断言 disabled 时返回原始实例（非任何包装），实测通过。
4. **实现守卫逻辑**：maybeCache 三段守卫——`!enabled`→原样；`c.(FundamentalCollector)` ok→原样；
   else→NewCached(c,ttl)。serve.go 三处注册（yahoo/eastmoney/crypto，line 95/113/126）统一经 maybeCache；
   eastmoney 的 lixinger fundamental fallback 在包装前 SetLixingerFallback 注入，不受影响。
5. **覆盖率**：45.4% ≥ 任务级 35 floor，未按 80% 整包门禁拒（ISSUE-3）。broker_test.go 重构属
   覆盖率补强（特征化测试），非 TASK-007 DoD，整包回归 ok 佐证未破坏 broker handler。

## 备注
此为倒数第二个任务，verified 后解锁 TASK-008。
