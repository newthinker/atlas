# TASK-008 验证报告 — backtest CLI 接入回测引擎

- **Verifier**: test-agent-1 (Reality Checker)
- **验证时间**: 2026-06-10（commit d27ca9d）
- **判定**: ✅ **VERIFIED**（Sprint 最后一棒）
- **被验包**: `./cmd/atlas`
- **测试结果**: 全部通过，包覆盖率 **52.8%**（≥ 任务 coverage_minimum=35）

## 验证方法（亲自运行）
```
go test ./cmd/atlas/ -race -cover -count=1                          # ok, 52.8%
go test ./cmd/atlas/ -race -v -run TestExecuteBacktest             # 6/6 PASS
HTTP_PROXY=http://127.0.0.1:1 HTTPS_PROXY=http://127.0.0.1:1 \
  GODEBUG=netdns=go go test ./cmd/atlas/ -run TestExecuteBacktest  # ok 0.245s（离线证明）
go build ./...                                                      # 全仓 build 干净
git show --stat d27ca9d / 代码精读 backtest.go                      # 真实引擎接线 + scope 核对
```

## Done Criteria 覆盖矩阵（6 条，全 PASS）

| # | 维度 | 完成标准 | 对应测试 | 实测断言 | 判定 |
|---|------|----------|----------|----------|------|
| 1 | functional[0] | 实跑回测引擎并输出信号数/交易数/Stats | `TestExecuteBacktest_RunsEngineAndOutputs` | prov.calls>0（引擎确被调用）+ 输出含 Signals/Trades/Win Rate/Total Return（源自真实 `r.Stats`/`len(r.Signals)`，非占位） | **PASS** |
| 2 | functional[1] | 策略不存在列出可用策略 + 非 0 退出 | `TestExecuteBacktest_UnknownStrategy` | 返回非 nil error + 输出含 "ma_crossover" + prov.calls==0（拉取前短路） | **PASS** |
| 3 | boundary[0] | --from 晚于 --to 参数错误（非 0 退出） | `TestExecuteBacktest_FromAfterTo` + `TestExecuteBacktest_InvalidDate` | from>to → error 且 prov.calls==0；非法日期 → error | **PASS** |
| 4 | boundary[1] | 空 OHLCV 友好提示，不 panic | `TestExecuteBacktest_EmptyData` | data=nil → 返回 nil（非 error）+ 输出含 "no historical data"，无 panic | **PASS** |
| 5 | error_handling[0] | 拉取历史失败错误 + 非 0 退出 | `TestExecuteBacktest_FetchError` | stub 返回 err → executeBacktest 返回非 nil error | **PASS** |
| 6 | non_functional[0] | CLI 测试不依赖真实外部 API，离线可重复 | 全部用 stubProvider + bytes.Buffer | 见下「离线确定性硬证据」 | **PASS** |

## Leader 三关注点核实

**(a) CLI 真实调用回测引擎（非占位输出）**：
- `executeBacktest`(backtest.go:130) 真实调用 `backtest.New(staticOHLCVProvider{data}).Run(ctx, strat, symbol, from, to)`，
  `printBacktestResult` 渲染的 Signals/Trades/WinningTrades/WinRate/TotalReturn/MaxDrawdown/SharpeRatio
  全部取自引擎返回的 `*backtest.Result`（`r.Stats`、`len(r.Signals)`、`len(r.Trades)`）——**非硬编码占位**。
- mockBtStrategy 在 sampleOHLCV(closes 102→106…) 上产生 buy→sell，引擎真实处理数据，prov.calls>0 佐证。

**(b) 四个错误路径退出码与提示**：
- 未知策略 → 打印可用列表 + 返回 error（非 0 退出）；非法/逆序日期 → error；空数据 → 友好提示 + 退 0（合理：空数据非故障）；
  拉取失败 → error。退出码语义经 cobra RunE 返回 non-nil error 实现（main 据此非 0 退出）。四路径各有独立测试断言。

**(c) 测试离线确定性（不打真实 API）**：
- **硬证据**：设 `HTTP_PROXY=HTTPS_PROXY=http://127.0.0.1:1`（无效代理，任何真实出网调用都会失败）
  + `GODEBUG=netdns=go` 重跑，测试仍 **ok 0.245s** 通过 → 证明零网络依赖。
- 结构佐证：测试只通过 `backtestDeps{provider: stubProvider}` 注入内存数据，**从不调用 `runBacktest`**
  （后者才装配真实 yahoo/eastmoney/crypto 采集器）；`grep` 确认测试文件无 net/http / 真实 collector 引用。

## scope / 回归
- git d27ca9d 改动 = `cmd/atlas/backtest.go` + `cmd/atlas/backtest_test.go`（2 文件，符合预期）。
- `go build ./...` 全仓干净（Sprint 收尾构建无破坏）。
- 注：`ld: warning ... malformed LC_DYSYMTAB` 为 macOS 链接器既知无害告警，测试结果为 `ok`，不影响判定。

## 结论
6/6 done_criteria 均有真实非空洞断言、实跑通过、覆盖率 52.8%≥35。CLI 真实接入回测引擎输出真实统计，
四错误路径退出码/提示齐备，离线确定性经无效代理重跑实证。**VERIFIED。** Sprint 全部开发任务验证完成，可进入 QA 阶段。
