# TASK-003 验证报告 — collector 根包：A 股指数表 + selector 指数/期货路由

- **Verifier**: test-agent-2
- **判定**: ✅ **VERIFIED**
- **时间**: 2026-06-11 (sprint-002)
- **被验对象**: `./internal/collector`，commits 7552246 + 583cb8b
- **复核参照**: plan rev3 Task 4(前半)+Task 5；done_criteria 为验收口径

## 测试执行证据（亲自运行）
```
go build ./...                                   → OK
go test ./internal/collector/ -race -cover -count=1
  → PASS  coverage: 99.0% of statements  (ok 1.650s)
  全部 21 个测试 PASS，含 -race 无数据竞争告警
```
新增/相关函数级覆盖（go tool cover -func）：
```
IsAShareIndex      100.0%
isIndexSymbol      100.0%
isCommoditySymbol  100.0%
KnownIndexMarket   100.0%
SelectForSymbol    100.0%
MarketForSymbol    100.0%
isAShareSymbol     100.0%
isCryptoSymbol     100.0%
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | IsAShareIndex 表驱动：000300.SH/000001.SH→true，000001.SZ/600519.SH→false | `TestIsAShareIndex`（6 用例，含全部 4 个指定符号） | PASS |
| functional[1] | MarketForSymbol：^GSPC/^IXIC/^DJI→US、**^HSI→HK**、^N225(表外)→US、GC=F/CL=F→US、000300.SH→CNA、AAPL→US、BTC-USDT→Crypto | `TestMarketForSymbol_IndexAndCommodity`（覆盖全部 10 用例，关键差异点 ^HSI→HK 显式断言） | PASS |
| functional[2] | SelectForSymbol：^GSPC/^HSI/GC=F→yahoo，000300.SH→eastmoney | `TestSelectForSymbol_IndexAndCommodityRouteToYahoo` | PASS |
| functional[3] | KnownIndexMarket 表内 (market,true)、表外 ^ (_,false) | `TestKnownIndexMarket`（表内 4 + 大小写 ^gspc + 表外 ^N225/^FTSE） | PASS |
| boundary[0] | A 股指数 000300.SH（无 ^ 前缀）仍走 .SH/.SZ→CNA 不受新路由影响（既有测试零回归） | 既有 `TestMarketForSymbol`/`TestSelectForSymbol` 保留且通过 + 新增 000300.SH→CNA / →eastmoney 用例 | PASS |
| non_functional | 包覆盖率 ≥ 80% (verify_by: test) | go test -cover = **99.0%** | PASS |

## 质量核查（Reality Checker）
- **非 fantasy assertion**：所有断言走真实路由函数（`MarketForSymbol`/`SelectForSymbol`/`KnownIndexMarket`），无硬编码绕过；fakeCollector 经 `newRegistryWith` 真实注册到 Registry，路由命中真实 `reg.Get`。
- **关键差异点 ^HSI→HK** 与 **表外 ^N225→US** 在同一表驱动用例中区分断言，不共用错误路径。
- **大小写不敏感**（^gspc）单独断言，覆盖 KnownIndexMarket 的 ToUpper 分支。
- **零回归**：既有 `TestSelectForSymbol`/`TestMarketForSymbol`/fallback/empty-registry 测试均保留且通过；A 股 .SH/.SZ→CNA 分支位于 index 分支之前，000300.SH 经断言确认仍归 CNA/eastmoney。
- **ISSUE-1（HTTP StatusCode）**：本任务为纯内存路由，无 HTTP 路径 → N/A。

## 结论
所有 done_criteria 逐条有真实、有意义的测试覆盖并通过；覆盖率 99.0% 远超 80% 门禁；-race 无竞争；零回归。**判定 VERIFIED。**
