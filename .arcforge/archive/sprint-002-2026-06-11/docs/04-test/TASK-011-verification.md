# TASK-011 验证报告 — app 估值分位编排（buildFundamental 兜底链）

- **验证人**: test-agent-1 (Reality Checker)
- **日期**: 2026-06-11
- **被验 commit**: f087741 `feat(app): PE percentile orchestration with lixinger fallback chain`
- **包**: ./internal/app ｜ coverage_minimum=80 (default) ｜ 复杂度: complex（本 Sprint 语义最重）
- **deps**: TASK-010/006/002/005（均 verified）
- **施工图**: plan rev3 Task 13
- **判定**: ✅ VERIFIED

## 测试执行证据
- `go test ./internal/app/ -race -count=1 -cover` → **PASS, 95.9%** (≥80)，**race 干净**（warnOnce/sync 路径）。
- `go build ./...` 0；`go vet ./internal/app/` 0；`go test ./...` exit 0，**48 包零 FAIL/panic/race**。
- 错误哨兵真实区分：valuation/reconstruct.go 中 ErrNonPositiveEPS（line47, 当前 EPS≤0 真实亏损）与 ErrInsufficientEPS（line40/63, 数据不足）为两个独立 sentinel——亏损/不足语义未混淆。

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---------|---------|------|
| functional[0] | 六路径表全过：A股→lixinger_cvpos / 美股主路径→reconstructed / EPS不足→兜底成功(lixinger_cvpos:前缀) / 兜底也失败→不可用 / 真实亏损→跳过 / 美港指数→lixinger | TestBuildPEPercentile_Paths（6 子用例，实调 buildFundamental，Source 用 strings.HasPrefix 真断言） | PASS |
| functional[1] | 亏损用例断言 stubVal.calls == 0（不兜底硬约束） | TestBuildPEPercentile_Paths/"美股真实亏损→直接跳过不兜底"：stubVal 真有 calls++ 计数器，wantNoVal=true 断言 sv.calls==0；valPct=99 故若误兜底会 available 且 calls=1 双重失败 | PASS |
| functional[2] | epsSrc 未配置→Source=lixinger_cvpos:yahoo_not_configured | TestBuildFundamental_EPSNotConfigured（exact string 断言） | PASS |
| boundary[0] | 商品/加密/基金→nil；双 nil 源→PEPercentile=-1+warnOnce 不 panic | TestBuildFundamental_NilSourcesAndUnsupported（GC=F/BTC-USDT/510300.SH→nil；CN/US 股双 nil→-1 无 panic） | PASS |
| error_handling[0] | 理杏仁 fetch 失败→warnOnce+PEPercentile=-1 | TestBuildFundamental_LixingerFetchError（observer 断言 logs.Len()>0 + PEPercentile==-1） | PASS |
| non_functional[0] (verify_by:test) | internal/app 全包 -race 通过 | -race -count=1 PASS，无 DATA RACE | PASS |

## 反 fantasy-assertion 专项核查（Sprint 语义最重，最高强度）
1. **六路径真实区分**：每路径 wantSource 用 strings.HasPrefix 对真实 Source 断言；主路径(reconstructed)与兜底(lixinger_cvpos:*)前缀互斥，能捕获路径错配。
2. **硬约束 calls==0 真实**：stubVal.FetchValuationPercentile 内 `s.calls++` 真实计数；亏损用例 valPct=99（若误兜底会返回可用的 99）+ wantNoVal 断言 calls==0 + wantPct=false——双重防线，生产若漏掉 `case errors.Is(rerr,ErrNonPositiveEPS): return f` 早返回会即时失败。已对照生产代码确认早返回在任何 valuationSrc 调用之前。
3. **日期对齐 load-bearing 自校验**：epsBase=now-3y，validEPS8 从 epsBase 起每季度，sampleCloses 从 epsBase+1月起（晚于首 EPS 点）。"美股主路径重建"用例期望 Source=="reconstructed" 且 wantNoVal=true；若对齐错误该用例会塌缩为 ErrInsufficientEPS 走兜底→Source 前缀不符而失败。用例通过即反证对齐正确。
4. **analyzeSymbol 条件组装真实**：fakeStrategy 捕获 ctx.Fundamental；needed 用例断言 Source/PEPercentile 真实透传，not-needed 用例断言 nil——验证 needsFundamentals 门控真实生效（非仅函数存在）。
5. **错误哨兵未混淆**：grep 确认 ErrNonPositiveEPS / ErrInsufficientEPS 为独立 sentinel；亏损(ErrNonPositiveEPS)与不足(ErrInsufficientEPS)分别走「跳过」与「兜底」两路，测试用 lossEPS（末点 -1）vs nil eps 区分触发。

## 生产代码与 plan Task 13 一致性
- 窄接口 ValuationSource/EPSSource 定义于 app 包，避免 import 具体 collector；SetValuationSources nil 容忍。
- buildFundamental 路径分派：非 Stock/Index→nil；market==MarketCNA||at==AssetIndex→理杏仁唯一路径；US/HK 个股→Yahoo 重建主路径，ErrNonPositiveEPS 立即 return（不兜底），ErrInsufficientEPS/fetch 失败/epsSrc 未配置→理杏仁兜底（Source 编码 fallbackReason）。
- analyzeSymbol：analysisCtx.Market 恒设 MarketForSymbol；needsFundamentals(effective) 为真才 buildFundamental（避免无谓网络调用），未绑定项保持 legacy。

## 结论
6 项 done_criteria 全部 PASS。本 Sprint 语义最重任务的两处易出 fantasy 的核心点——亏损不兜底硬约束(calls==0)与 EPS 日期对齐——均经真实计数 stub + 自校验用例确认，Source 路径前缀真断言，错误哨兵独立，-race 干净，48 包零回归。判定 **VERIFIED**。

---

## 复验 Round 2 — QA W1/S1 review_fix（2026-06-11）

- **被验 fix commit**: cc0182a `fix(TASK-011): price arbitrated signals and pin valuation-source injection invariant (QA W1/S1)`
- **epoch**: 2 ｜ rework_count: 1
- **判定**: ✅ VERIFIED（复验通过）

### fix_items 逐条核验
| fix_item | 修复内容 | 验证 | 判定 |
|---|---|---|---|
| ① W1/I3：meta_arbitrator 信号 Price=0 不可下单 | arbitrate() 合成信号补 `Price: referencePrice(signals)`（取冲突输入首个正价，冲突信号均按本周期末根收盘定价）；referencePrice 无定价输入时返 0 由 executor 正价守卫兜底 | TestApp_ArbitrateSignalIsPriced：okArbitrator 决策 Sell，冲突输入 Price=123.45，断言 out[0].Strategy=="meta_arbitrator" 且 Price>0 且 ==123.45（无修复则为 0 必失败）；TestReferencePrice：[0,50,99]→50、[0,0]→0 直测 helper | PASS |
| ② S1：注入点不变量注释 | SetValuationSources 加注释固化「必须 Start 前注入·worker 无锁读·set-once-before-Start 保证无竞争」 | diff 确认注释已加；-race 干净佐证无锁读安全 | PASS |
| ③ 不破坏既有六路径测试 + -race | 纯新增（app_test.go 无删改行） | TestBuildPEPercentile_Paths 及全部 BuildFundamental 测试复跑全 PASS；app -race -count=1 干净，覆盖率 96.3%（↑from 95.9%）；go test ./... exit 0 零回归 | PASS |

### 反 fantasy-assertion 核查
- TestApp_ArbitrateSignalIsPriced 断言精确价 123.45（= 冲突输入价），非仅 >0 占位——能捕获补价取错来源；commit msg 自述「verified it fails without the fix」与断言逻辑一致（无 referencePrice 则 Price=0）。
- referencePrice 0-fallback 属文档化 fail-safe：post-W1 策略信号恒带末根收盘价，0 分支仅安全网，且 executor 正价守卫再次兜底，不会放行 Price=0 错单。`或末根收盘`备选因冲突信号已按末根收盘定价而无需显式取 close（arbitrate 签名亦无 ohlcv），合理。

### 复验结论
3 项 fix_items 全部 PASS，W1 补价有精确价断言、S1 注释到位、六路径零回归、-race 干净 96.3%。**VERIFIED**。
