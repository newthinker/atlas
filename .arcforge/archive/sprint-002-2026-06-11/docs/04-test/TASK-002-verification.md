# TASK-002 验证报告 — yahoo 指数/期货符号 + FetchEPSHistory

- **验证人**: test-agent-1 (Reality Checker)
- **日期**: 2026-06-11
- **被验 commit**: 9a63967 `feat(yahoo): index(^)/futures(=F) symbols with URL escaping + FetchEPSHistory`
- **包**: ./internal/collector/yahoo ｜ coverage_minimum=80 (default)
- **施工图**: plan rev3 Task 2 + Task 3
- **判定**: ✅ VERIFIED

## 测试执行证据
- `go test ./internal/collector/yahoo/ -race -cover` → **PASS, coverage 81.7%** (≥80)，全部 httptest 离线（无真实网络 dial，grep 确认无 finance.yahoo/http.Get）。
- `go build ./...` exit 0；`go vet ./internal/collector/yahoo/` exit 0。
- `go test ./...` 全量 exit 0，46 包全 ok，零 FAIL/panic（消费方零回归）。
- yahoo_test.go diff 为**纯新增**（既有用例未改一行 → 满足「既有测试零修改」）。

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---------|---------|------|
| functional[0] | 符号正则全用例：^GSPC/^IXIC/^HSI/GC=F/CL=F/SI=F 接受；空串/^/=F/^GSPC.SH/GC=X=F/注入串拒绝 | TestValidateSymbol_IndexAndFutures（表驱动 15 例，含 `AAPL; DROP` 注入） | PASS |
| functional[1] | FetchQuote(^GSPC) 路径将 ^ 编码为 %5E（httptest 断言） | TestFetchQuote_EscapesIndexSymbol（`r.URL.EscapedPath()` 断言含 `%5EGSPC`） | PASS |
| functional[2] | FetchEPSHistory 解析 trailingDilutedEPS(asOfDate+reportedValue.raw) 升序返回，请求含 type=trailingDilutedEPS | TestFetchEPSHistory（handler 校验 RawQuery 含 type；断言 2 点 Date/EPS 映射 + 顺序） | PASS（见 Advisory-1） |
| functional[3] | NewWithBaseURLs 注入双端点；NewWithBaseURL 兼容既有测试零修改 | TestFetchEPSHistory 用 NewWithBaseURLs；既有 TestFetchQuote_* 仍用 NewWithBaseURL 全过 | PASS |
| boundary[0] | 空/缺字段返回空 slice+nil error；raw<=0 点保留 | TestFetchEPSHistory_EmptyAndIndexSymbol（空 result→len0,nil）+ TestFetchEPSHistory_KeepsNonPositive（-1.5/0 保留，len=2） | PASS |
| error_handling[0] | 指数符号(^)调 FetchEPSHistory 不发请求直接 error | TestFetchEPSHistory_EmptyAndIndexSymbol（^GSPC→err；代码入口 `strings.HasPrefix(symbol,"^")` 早返回，先于 client.Do） | PASS |
| non_functional[0] (verify_by:test) | 包覆盖率≥80%，全测试 httptest 离线 | 81.7%，全 httptest | PASS |

## ISSUE-1 专项核查（HTTP StatusCode 守卫 — 重点）
- **守卫真实存在**：eps.go `client.Do` 后、`json.Decode` 前有 `if resp.StatusCode != http.StatusOK { return error }`（位置正确，非占位）。
- **分路径断言（反 fantasy assertion）**：
  - TestFetchEPSHistory_NonOKStatus → handler 返回 **503 + 合法 JSON body**，触发 error 的必是 StatusCode 守卫（合法 JSON 不会 decode 失败）。
  - TestFetchEPSHistory_MalformedJSON → handler 返回 **200 + 非法 body**，触发 decode 失败路径。
  - 两测试用独立 httptest handler，错误来源路径明确分离 → 不复现 sprint-001 ISSUE-1 fantasy 模式。✅

## UA/Accept 头复用核查（plan T3 要求⑤）
- 抽出 `(y *Yahoo) newRequest(reqURL)` helper，统一设置 User-Agent/Accept/Accept-Language。
- FetchQuote、FetchHistory、FetchEPSHistory 三路径均改为调 `y.newRequest(...)`（diff 确认旧的内联 header 已删、统一走 helper）→ EPS 端点共享 UA，满足「真实端点无 UA 会 403」防护。✅

## Advisory（非阻塞，建议后续硬化）
- **Advisory-1**：TestFetchEPSHistory 的输入序列本身已升序（2022-12-31 → 2023-03-31），故 `sort.Slice` 的**重排**能力未被实际触发——若 sort 被误删该用例仍会通过。排序代码本身正确，输出顺序也被断言，故不阻塞；建议补一个乱序输入用例固化升序契约。

## 结论
7 项 done_criteria + ISSUE-1 + UA 复用全部 PASS，均有真实命令/测试输出佐证，无 fantasy assertion。判定 **VERIFIED**（附 Advisory-1 测试硬化建议）。
