# TASK-010 验证报告（复验 / rework=1）— lixinger 可测性重构 + httptest 测试

- **Verifier**: test-agent-1 (Reality Checker)
- **验证时间**: 2026-06-10（复验，针对 commit cfcdee1 返工）
- **判定**: ✅ **VERIFIED**（前次 REJECTED 的 error_handling[0] 已真实修复）
- **被验包**: `./internal/collector/lixinger`
- **测试结果**: 全部通过，包覆盖率 **81.7%**（≥80% 门禁达标）

## 复验聚焦：error_handling[0] 反例是否真正修复
前次拒绝理由：实现无 `resp.StatusCode` 检查，`HTTPError_*` 测试靠非 JSON body（"upstream down"）
触发 decode 失败，HTTP 503 + 合法 JSON success body 会被当成功。

**本次硬证据（已修复）：**
1. **守卫真实存在且全覆盖**：lixinger.go **6 个 HTTP 端点**（行 109/193/270/352/433/553）
   `client.Do` 之后、`json.Decode` 之前均加状态码守卫：
   ```go
   if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("lixinger: unexpected HTTP status %d", resp.StatusCode) }
   ```
   （`fetchFundInfo` 因签名为 `*core.FundInfo` 单返回值，守卫为 `return nil`，语义一致）。
2. **测试命中我的反例**：`TestLixinger_HTTPError_Quote` 现发送
   `503 + {"code":0,"data":[{"stockCode":"600519","close":1800.5}]}`（**合法 JSON success body**），
   `TestLixinger_HTTPError_Fundamental` 发送 `502 + 合法 JSON`。测试注释明确引用
   「caching proxy / rate-limit gateway 返回 503」场景——即我报告里的反例。**通过 ⟹ 错误来自状态码守卫**，
   与 MalformedJSON 路径彻底区分。2/2 PASS（-race）。

## 业务逻辑零改动核对（返工约束）
- `git show cfcdee1 -- lixinger.go`：**新增全为状态码守卫，零删除行**；解析/业务码/空数据逻辑全未动。
- `lixinger_test.go`（原 5 测试文件）**不在返工 commit 中**——原 5 测试零修改（functional[0] 持续成立）。
- 返工仅改 `lixinger_httptest_test.go` 的 2 个 HTTPError 用例 body 为合法 JSON + 非 200。

## Done Criteria 覆盖矩阵（8 条，全 PASS）

| # | 维度 | 完成标准 | 对应测试 | 判定 |
|---|------|----------|----------|------|
| 1 | functional[0] | baseURL 注入后默认不变，现有 5 测试不改即过 | 原 lixinger_test.go(返工零改) + `TestLixinger_NewDefaultsBaseURL` | **PASS** |
| 2 | functional[1] | FetchQuote/FetchHistory 正确解析 | `FetchQuote_OK`/`FetchHistory_OK`（字段断言） | **PASS** |
| 3 | functional[2] | FetchFundamental(History) 正确解析 | `FetchFundamental_OK`/`FetchFundamentalHistory_OK` | **PASS** |
| 4 | boundary[0] | 空 data 返回空结果或明确错误，不 panic | `EmptyData_Quote`(err)/`_History`(空 slice)/`_Fundamental`(err) | **PASS** |
| 5 | error_handling[0] | HTTP 非 200 状态码返回 error | `HTTPError_Quote`(503+合法JSON)/`HTTPError_Fundamental`(502+合法JSON) | ✅ **PASS（已修复）** |
| 6 | error_handling[1] | 畸形 JSON 返回 error | `MalformedJSON_Quote`/`_History` | **PASS** |
| 7 | error_handling[2] | 200 但业务错误码返回 error，不 panic | `BusinessError_Quote`/`_Fundamental`(code:1) | **PASS** |
| 8 | non_functional[0] | 包覆盖率 ≥ 80% | 实测 81.7% | **PASS** |

## 备注（非阻塞）
- 6 个端点均加守卫，但显式「非 200」测试覆盖其中 2 个（stock-quote 503 / fundamental 502）。
  守卫结构在 6 处完全同构，2 个代表性端点已真实验证状态码驱动错误路径，error_handling[0] 标准达成。
  覆盖率较返工前 83.2%→81.7%（4 个未测端点的守卫分支未覆盖），仍高于 80% 门禁。如需更严可后续补
  其余 4 端点非 200 用例，但当前 DoD 已满足，不阻塞验收。

## 结论
返工精准命中前次拒绝点：6 端点状态码守卫真实落地（decode 前）、HTTPError 测试用合法 JSON+非 200
独立验证我的反例、业务逻辑零改动、原 5 测试零修改、覆盖率 81.7%≥80%。8/8 done_criteria 全部
真实非空洞断言并实跑通过。**VERIFIED。**
