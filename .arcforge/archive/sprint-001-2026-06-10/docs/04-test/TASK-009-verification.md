# TASK-009 验证报告（复验 / rework=1）— eastmoney 可测性重构 + httptest 测试

- **Verifier**: test-agent-1 (Reality Checker)
- **验证时间**: 2026-06-10（复验，针对 commit c18c2eb 返工）
- **判定**: ✅ **VERIFIED**（前次 REJECTED 的 error_handling[0] 已真实修复）
- **被验包**: `./internal/collector/eastmoney`
- **测试结果**: 全部通过，包覆盖率 **86.5%**（较返工前 84.3% 提升，≥80% 门禁达标）

## 复验聚焦：error_handling[0] 的反例是否被真正修复
前次拒绝理由：实现无 `resp.StatusCode` 检查，`*_HTTPError` 测试靠非 JSON body 触发 decode 失败
（与 MalformedJSON 同路径），HTTP 非 200 + 合法 JSON 会被当成功。

**本次硬证据（已修复）：**
1. **守卫真实存在**：eastmoney.go 4 个 fetch 方法（行 190/254/348/432）均加：
   ```go
   if resp.StatusCode != http.StatusOK {
       return nil, fmt.Errorf("eastmoney: unexpected HTTP status %d", resp.StatusCode)
   }
   ```
   位置正确——在 `client.Get/Do` 与 `defer Body.Close()` 之后、`json.Decode` 之前。
2. **测试覆盖我的反例**：`TestFetchQuote_HTTPError` 现发送
   `validBody={"data":{"f43":1500,...}}`（**合法 JSON，200 下能成功解析为 Quote**）+ HTTP 500，
   断言返回 error。**测试通过 ⟹ 错误必来自 StatusCode 守卫，而非 decode 失败**，与 MalformedJSON 路径区分开。
3. **四端点全覆盖**：新增 `TestFetchQuote_HTTPError`/`TestFetchHistory_HTTPError`/
   `TestFetchQuote_Fund_HTTPError`/`TestFetchHistory_Fund_HTTPError`，4/4 PASS（-race）。

## Done Criteria 覆盖矩阵（8 条，全 PASS）

| # | 维度 | 完成标准 | 对应测试 | 判定 |
|---|------|----------|----------|------|
| 1 | functional[0] | baseURL 注入后默认不变，现有 5 测试不改即过 | 原 5 TestEastmoney_*(两次提交均 0 删除行) + `TestNewWithBaseURLs_DefaultsUnchanged` | **PASS** |
| 2 | functional[1] | FetchQuote 解析股票行情为 Quote | `TestFetchQuote_Stock`(divisor=100 价格全断言) | **PASS** |
| 3 | functional[2] | FetchHistory 解析 K 线为 []OHLCV | `TestFetchHistory_Stock`(2 bars 字段断言) | **PASS** |
| 4 | boundary[0] | 空数据/空列表返回空结果或明确错误，不 panic | `TestFetchQuote_NullData` + `TestFetchHistory_EmptyKlines` | **PASS** |
| 5 | error_handling[0] | HTTP 非 200 状态码返回 error | `TestFetchQuote_HTTPError` 等 4 个（合法 JSON + 500 → 守卫返 error） | ✅ **PASS（已修复）** |
| 6 | error_handling[1] | 畸形 JSON 返回 error | `TestFetchQuote_MalformedJSON`(`{not json`) | **PASS** |
| 7 | error_handling[2] | 200 但业务错误（data null）返回 error，不 panic | `TestFetchQuote_NullData`(200 + `{"data":null}`) | **PASS** |
| 8 | non_functional[0] | 包覆盖率 ≥ 80% | 实测 86.5% | **PASS** |

## 业务逻辑零改动核对（返工约束）
- `git show c18c2eb -- eastmoney.go`：**仅新增 4 段守卫，零删除行**，解析逻辑/SetLixingerFallback 全未动。
- `git show c18c2eb -- eastmoney_test.go`：原 5 测试零删除，仅改造 4 个 *_HTTPError 用例 body 为合法 JSON + 新增 fund 端点 HTTPError 测试。

## 结论
返工精准命中前次拒绝点：StatusCode 守卫真实落地、测试用合法 JSON+非 200 独立验证、业务逻辑零改动、
覆盖率提升至 86.5%。8/8 done_criteria 全部有真实非空洞断言并实跑通过。**VERIFIED。**
