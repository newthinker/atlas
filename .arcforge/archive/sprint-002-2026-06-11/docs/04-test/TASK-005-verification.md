# TASK-005 验证报告 — lixinger 多市场估值分位（FetchValuationPercentile）

- **Verifier**: test-agent-2
- **判定**: ✅ **VERIFIED**
- **时间**: 2026-06-11 (sprint-002)
- **被验对象**: `./internal/collector/lixinger`，commit cfb4fe8
- **复核参照**: plan rev3 Task 7；done_criteria 为验收口径；wisdom ISSUE-1 重点核查

## 测试执行证据（亲自运行）
```
go test ./internal/collector/lixinger/ -race -cover -count=1
  → PASS  coverage: 83.6% of statements  (ok 1.461s)
  19 测试全 PASS（含既有 11 个 lixinger_* 测试零回归），-race 无竞争
新文件 per-func: endpointFor 90% / lookbackGranularity 100% /
  FetchValuationPercentile 95.2% / digFloat 90% / postJSONRaw 78.6%
全部 httptest 离线，无真实网络。
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | endpointFor 七用例（600519.SH/000300.SH/0700.HK→00700/AAPL/^GSPC→SPX/^HSI→HSI/GC=F→空） | `TestEndpointFor`（7 用例完全对应，含港股补零与 GC=F 空端点） | PASS |
| functional[1] | 请求体含 pe_ttm.y5.cvpos（lookback=5）；cvpos 0.2345→23.45(±0.01) | `TestFetchValuationPercentile`（httptest 断言 body 含 metric + got∈[23.44,23.46]） | PASS |
| functional[2] | lookbackYears 映射 <=3→y3、<=5→y5、否则 y10 | `TestFetchValuationPercentile_Granularity`（1/3/4/5/6/10 断言请求体 metric 字符串） | PASS |
| boundary[0] | 商品符号 GC=F 返回 error 不发请求 | `TestFetchValuationPercentile_Unsupported`（endpoint=""短路 + 无效 baseURL 佐证未触网）+ `TestEndpointFor`(GC=F→"") | PASS |
| error_handling[0] | 业务码非0/data空/字段缺失 → (-1,error) | `_BusinessError`(HTTP200+code403) / `_EmptyData`(code0+data[]) / `_MissingMetric`(code0+无 pe_ttm) 三路径分离 | PASS |
| non_functional | 覆盖率 ≥ 80%、httptest 离线 | go test -cover = **83.6%**，全 httptest | PASS |

## ISSUE-1 专项核查（点④ —— lixinger 上 Sprint 因此被拒）
- **守卫真实存在**：`postJSONRaw`（valuation.go:152-154）在 `io.ReadAll`/解析**之前** `if resp.StatusCode != http.StatusOK { return error }`。
- **测试真实触发守卫**：`TestFetchValuationPercentile_HTTPError` 用 **HTTP 500 + 合法 JSON body** `{"code":0,"data":[{"pe_ttm":{"y5":{"cvpos":0.5}}}]}`——该 body 若无守卫会解析成功返回 50.0/nil；测试断言 err≠nil，**仅当 StatusCode 守卫存在才通过**。
- **路径分离（非 fantasy）**：HTTP500 守卫 / 业务码403(HTTP200) / data空(HTTP200) / metric缺失(HTTP200) 四条错误路径各自独立触发，不共用碰巧失败的 decode 路径——彻底规避上 Sprint 的 fantasy-pass 模式。
- **结论**：未重蹈覆辙，ISSUE-1 修复模板已正确落实并被有效测试守护。

## 其他核查
- **点⑤ 留痕确认**：discovery `caveats` 明确记录「成功码约定/请求体键名(metricsList vs metrics)/理杏仁国际指数码(SPX/COMP/DJI/HSI)/港股5位补零 均按既有代码与 plan 候选值实现，无 LIXINGER_API_KEY 未对真实 API 核验，首日需统一修正」——核对项已注明留痕。
- **零回归**：既有 11 个 lixinger_* 测试全过（FetchFundQuote/FetchFundHistory/FetchFundInfoPublic/Init 等）。
- **endpointFor 复用单一真相源**：A 股指数判定复用 TASK-003 的 `collector.IsAShareIndex`，与 selector/eastmoney 共用。

## 非阻断观察（不影响判定）
- error_handling 三测试断言 err≠nil 但未显式断言返回值 == -1。源码所有错误路径（行 72/87/96/99/102/107）均 `return -1, ...` 统一返回 -1，无返回异值分支，故哨兵语义实质成立。建议下游 011 消费 -1 哨兵处补一条值断言以锁死契约（非本任务阻断项）。

## 结论
6 条 done_criteria 逐条有真实、有意义、路径分离的测试并通过；覆盖率 83.6% ≥ 80%（全 httptest 离线）；-race 无竞争；**ISSUE-1 StatusCode 守卫已正确落实且被有效测试守护**；核对项 discovery 留痕。**判定 VERIFIED。**
