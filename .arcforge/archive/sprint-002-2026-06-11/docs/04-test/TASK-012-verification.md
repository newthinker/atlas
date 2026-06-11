# TASK-012 验证报告 — cmd 装配 + 配置示例 + 回测冒烟 + README（Sprint 最后一棒）

- **Verifier**: test-agent-2
- **判定**: ✅ **VERIFIED**
- **时间**: 2026-06-11 (sprint-002)
- **被验对象**: `./cmd/atlas`，commit 0a65f83，coverage_minimum=35
- **复核参照**: plan rev3 Task 14 + Task 15（部分）；done_criteria 为验收口径

## 测试执行证据（亲自运行）
```
go build ./...   → exit 0（BUILD_OK）
go vet ./...     → exit 0（VET_OK，全量是 plan T15 Step 3 承接）
go test ./...    → 全 48 包 ok，无 FAIL/panic（全 Sprint 集成回归零回归）
go test ./cmd/atlas/ -race (typed-nil 4 测试) → 4 PASS
go test ./cmd/atlas/ -cover → coverage 51.8% ≥ coverage_minimum 35
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 验证证据 | 判定 |
|---|---|---|---|
| functional[0] | serve 注册两新策略 + 估值源 typed-nil 防护（nil 指针不得变非 nil 接口，测试断言） | serve.go:163-168 经 registerConfiguredStrategy 读 cfg 注册；`TestValuationSourceOrNil_NilStaysNil`/`TestEPSSourceOrNil_NilStaysNil`（typed-nil 指针→断言接口==nil）+ `_RealNonNil` 4 测试全 PASS | PASS |
| functional[1] | backtest 注册 price_percentile；AAPL+^GSPC 冒烟无错退出且 percentile 计算发生（区间 2021-01-01..2026-06-01，>252 交易日） | backtest.go:85 注册 price_percentile（pe_percentile 注释说明不注册）；**亲自跑 AAPL=880 signals、^GSPC=836 signals**（均成功退出、数百信号证明非数据不足全程跳过；^GSPC 承接 plan 验收对照第 1 条指数端到端） | PASS |
| functional[2] | config.example.yaml 含两策略参数块 + 三 watchlist 示例，pe_band 死参数两行已删 | price_percentile{25/75/10/90}+pe_percentile{20/80/10/90} 两块；watchlist 追加 ^GSPC/GC=F/BTC-USDT；pe_band 仅余 enabled:false（lookback_years/threshold_percentile 已删） | PASS |
| boundary[0] | lixinger/yahoo 未配置时 serve 正常启动（注入 nil 接口路径不 panic） | typed-nil helper 返回 untyped-nil → SetValuationSources 收 nil → buildFundamental `valuationSrc!=nil` 守卫成立，无 typed-nil 陷阱；load-bearing 不变量由 NilStaysNil 单测固化 | PASS |
| non_functional[0]（review） | README Multiple Strategies / Multi-Market 两行已更新 | README 新增「Multi-Market…plus indexes & commodities」「Multiple Strategies…Price/PE Percentile」两行 | PASS |
| non_functional[1]（test） | go build/vet/test 全量通过 | 见上：build/vet exit 0，test 48 包全 ok | PASS |

## 重点核查
- **点 1 typed-nil 防护真实**：`TestValuationSourceOrNil_NilStaysNil` 传入 `var c *lixinger.Lixinger`（typed nil 指针），断言 `valuationSourceOrNil(c) != nil` 失败 → 接口必须是 untyped nil。若实现 `return c` 直接透传会得到「非 nil 接口包 nil 指针」→ got!=nil → 测试失败。源码 line 306-311 `if c==nil { return nil }` 正确。**非 fantasy，真实守护不变量。**
- **点 2 冒烟亲自跑**：AAPL 首轮 Yahoo 返回瞬时 EOF（反爬/限流），重试第 2 轮成功 880 signals；^GSPC 首轮即成功 836 signals。EOF 系外部依赖瞬时性，URL 转义正确（%5EGSPC），非代码缺陷（与 discovery 一致）。两符号 percentile 端到端计算确凿发生。
- **点 3 config**：两策略块 + 三 watchlist + pe_band 死参数删除均核对属实。
- **点 4 serve nil 路径**：typed-nil 机制规避 panic，单测固化（全量 serve 启动阻塞于信号不便冒烟，但 panic 风险点即 nil 注入已单测覆盖）。
- **点 5 理杏仁核对留痕**：discovery downstream_notes 明确「本环境无 LIXINGER_API_KEY 已跳过；usHKIndexCodes(SPX/COMP/DJI/HSI) 已固化于 lixinger/valuation.go；线上核对用 hk/index、us/index basic-info/samples 接口复核」——留痕完整。
- **点 6 全量回归**：build/vet/test ./... 三件套亲自跑，全绿零回归。

## 结论
6 条 done_criteria 逐条有真实证据并通过；typed-nil 防护单测真实守护、回测冒烟两符号（含指数 ^GSPC 端到端）亲自跑通、config/README 核对属实、全量 build/vet/test 零回归、覆盖率 51.8% ≥ 35。**判定 VERIFIED。Sprint 全部任务验证完成，可进入 QA。**

## 非阻断观察
- 回测冒烟依赖 Yahoo 实时接口，存在瞬时 EOF 需重试（外部依赖性，非代码缺陷）；QA/CI 若纳入端到端冒烟建议加重试或 fixture 化。
