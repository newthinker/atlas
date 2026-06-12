# TASK-001 验证报告 — export-ohlcv 核心

- **验证者**: test-agent-1 (Reality Checker)
- **任务**: export-ohlcv 核心（qlib CSV 约定 + 符号契约 + 失败语义）
- **commit**: 7f2a080 / **package**: ./cmd/atlas / **coverage_minimum**: 35
- **判定**: ✅ **VERIFIED**（9/9 DoD 逐条有真实测试输出佐证，无 fantasy assertion）

## 机器证据（实跑输出）
- `go test ./cmd/atlas -race -cover` → `ok ... coverage: 60.0% of statements`（≥35 基线，race 全绿）
- 7 个相关测试函数全 PASS（-v 实跑）：TestToQlibInstrument_Contract / TestExportOHLCV_GoldenCSV /
  TestExportOHLCV_NonAShareRejectedIntoSummary / TestExportOHLCV_NonBenchmarkFailureDegrades /
  TestExportOHLCV_BenchmarkFailureIsFatal / TestResolveOHLCVSymbols_Default / TestResolveOHLCVSymbols_EmptyWatchlistIsError
- 契约交叉核对 symbols.py：`.SH→SH+code / .SZ→SZ+code / else raise` 与 Go toQlibInstrument 逐行镜像一致

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | toQlibInstrument 契约 000300.SH→SH000300(+sh000300.csv 派生)/600519.SH/399001.SZ；五类非A股拒绝 | TestToQlibInstrument_Contract | PASS（3 正样本+5 负样本+文件名派生断言；与 symbols.py 同样本同结果） |
| functional[1] | golden CSV 逐字节：8列 header + 三行互异 OHLCV + factor 恒1 + sh600519.csv | TestExportOHLCV_GoldenCSV | PASS（want 串 open=100.00/high=101.00/low=99.00/close=100.50/vol=1000，**互异非 0**） |
| functional[2] | resolveOHLCVSymbols：{600519.SH,BTC-USDT,^GSPC,000300.SH}→{600519.SH,000300.SH} | TestResolveOHLCVSymbols_Default | PASS（.SH/.SZ 过滤+基准保序去重，断言精确等值） |
| boundary[0] | 非A股在清单→不落盘+errOut摘要+返回error，已成功CSV保留 | TestExportOHLCV_NonAShareRejectedIntoSummary | PASS（error + errOut 含 AAPL + sh600519.csv 保留 + aapl.csv 不存在） |
| boundary[1] | 非基准A股失败(errs 600000.SH)→降级不中断+摘要含该符号+非0退出，已成功CSV保留 | TestExportOHLCV_NonBenchmarkFailureDegrades | PASS（**真实 errs 注入**，走与基准 fatal 不同的 append-continue 分支；sh600000.csv 不存在） |
| boundary[2] | watchlist 空且无 --symbols → resolver 报错（绝不退化为只导基准） | TestResolveOHLCVSymbols_EmptyWatchlistIsError | PASS（nil→error；仅非A股→error 双断言） |
| error_handling[0] | 基准 000300.SH 失败/空bars→立即返回error 且消息含 benchmark | TestExportOHLCV_BenchmarkFailureIsFatal | PASS（errs 000300.SH→err.Error() 含 "benchmark"） |
| non_functional[0] | sleep 经 deps 注入(测试零等待)；-race 通过 | 全部用例 sleep:func(){} + 全包 -race 绿 | PASS |
| —（trap⑥）| 核心层**不含**「清单含基准」校验（归 002 CLI 层） | 代码审查 export_ohlcv.go | PASS（resolver 显式 flag 原样透传，无 presence 校验；唯一 benchmark 逻辑是 fetch-fail fatal + 派生集追加） |

## team-lead 标注的 6 个坑 — 逐一核实
1. **①golden 用 makeOHLCVBars 而非 makeBars**：✅ 避坑。既有 makeBars（export_signals_test.go:27）只填 Symbol/Interval/Close/Time，Open/High/Low/Volume 为零值；若用它 golden 会出现 open=0.00（C1-2）。实际 golden 三行 OHLCV 互异非 0，且与 makeOHLCVBars 生成规律(Open=100+i…)逐字节互锁。
2. **②非基准A股失败降级真实存在**：✅。fakeOHLCVProvider.errs 注入 600000.SH，断言 error + 摘要含符号 + 其余 sh600519.csv 保留 + sh600000.csv 不落盘——非空洞断言，真实穿过 append-continue 分支。
3. **③基准失败 fatal 且消息含 benchmark**：✅。err.Error() 含 "benchmark"。（空 bars 变体：代码 `len(bars)==0→err="no data"` 后与 fetch-fail 收敛同一 fatal 分支，行为已实现——见下「次要观察」。）
4. **④契约样本与 symbols.py 一致含文件名派生**：✅。3 正样本逐字镜像 symbols.py；sh000300.csv 派生断言在测。
5. **⑤resolver 空集报错**：✅。nil 与「仅非A股」两种空集均 error，绝不退化为只导基准。
6. **⑥核心层不含「清单含基准」校验**：✅。grep + 通读确认无 presence 校验，正确留给 TASK-002 CLI。

## 次要观察（不影响判定，供 002/QA 留意）
- error_handling[0] 括注「(或返回空 bars)」：基准**空 bars→fatal** 路径由代码覆盖（`len(bars)==0` 置 err 后与 fetch-fail 共用同一 benchmark fatal 分支），但无独立测试函数显式断言「基准返回空 bars」这一输入。主条款（fetch 失败 fatal + 消息含 benchmark）已由 TestExportOHLCV_BenchmarkFailureIsFatal 实测。属覆盖完备性的小缺口，非行为缺陷、非 fantasy assertion，**不构成拒绝理由**。

## 结论
压倒性证据满足 PASS 门槛：全部 9 条 DoD 有对应有意义测试且实跑通过，coverage 60.0%≥35，race 绿，plan 评审拦截的 C1-1/C1-2 两坑均避开。**判定 VERIFIED。**
