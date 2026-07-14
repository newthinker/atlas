# TASK-005 验证报告 — eastmoney ChangePercent /100 换算修复（W1）

- 验证者: test-agent-1 | 判定: **VERIFIED (PASS)** | commit: ddc543c (epoch=1) | 2026-07-03

## Done Criteria 覆盖矩阵
| # | 完成标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| a | FetchQuote f170=204→ChangePercent==2.04(股票); ETF 同除 100(不受 divisor=1000 影响) | test | TestFetchQuote_ChangePercentScale 4 子用例: stock 204→2.04 / etf(510300.SH,divisor=1000) 204→2.04 / 0→0 / -153→-1.53 全 PASS。路由核实: isFund→fund, 否则(含 ETF)→fetchStockQuote; ChangePercent 用固定 /100 非 divisor | PASS |
| b | 生产代码仅改 ChangePercent 一处(Price/Change 等零改动) | review | git show ddc543c eastmoney.go: 唯一功能改动 `ChangePercent: d.F170` → `d.F170 / 100`(+注释); Price/Open/High/Low/PrevClose/Change/Bid/Ask 仍 `/divisor` 未动 | PASS |
| c | 基金路径 Gszzl 语义核查一致无需改 | review | fetchFundQuoteFromEastmoney: `changePercent=ParseFloat(fund.Gszzl)` 直取; Gszzl=估算涨跌幅(直接百分比字符串,如"2.04"即 2.04%),非 ×100,无需除。dev 结论正确 | PASS |
| d | eastmoney 既有测试零回归 + 全量离线全绿 | test | eastmoney 全量(Stock/Fund/NullData/HTTPError/MalformedJSON 等)ok 零回归; go build/vet/test ./... 离线全绿 | PASS |

## 覆盖率
- eastmoney: 87.0%（超 80）

## 结论
4/4 done_criteria PASS，四子用例覆盖 stock/ETF/零/负边界，生产改动最小(仅 ChangePercent)，基金路径核查正确。判定 VERIFIED。
