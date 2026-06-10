# TASK-011 验证报告 — yahoo 采集器可测性重构 + httptest 测试

- 验证者: test-agent-2 (Reality Checker)
- 包: ./internal/collector/yahoo
- 认领说明: 派验时 verifier=null（Leader 未指定，test-agent-1 正忙于 TASK-010）。在锁内
  以「仅当 status==dev_done && verifier==null」防御式认领，置 verifier=test-agent-2，
  未抢占他人任务。
- 验证命令（亲自复跑）: `go test -race -cover -count=1 ./internal/collector/yahoo/`
  → ok, coverage **82.0%**（≥ 80% 门禁；与 dev 自报一致），13 个 Test 函数全部通过。

## 判定: VERIFIED ✅

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 断言核验 | 判定 |
|---|---------|------|---------|------|
| functional[0] | baseURL 注入构造后默认行为不变，现有 8 测试不修改即通过 | git show 67818c0：yahoo.go 仅 const baseURL→defaultBaseURL + 实例字段 baseURL + NewWithBaseURL，New() 委托 defaultBaseURL；yahoo_test.go 无删改既有测试（diff 无 `-func Test`/`-t.Error`）；8 个原测试函数均 present 且通过 | 仅重构注入点，无业务逻辑改动；默认 New() 行为不变 | PASS |
| functional[1] | httptest 下 FetchQuote 正确解析 chart 响应为 core.Quote | TestFetchQuote_ParsesResponse | 逐字段断言 Symbol/Price/Open/High/Low/PrevClose/Change/Volume/Time/Source | PASS |
| functional[2] | httptest 下 FetchHistory 正确解析为 []core.OHLCV（时间戳/OHLC 断言） | TestFetchHistory_ParsesResponse | 2 根 bar：Time、OHLC、Volume、Interval 全断言，bars[1].Close 亦断言 | PASS |
| boundary[0] | result 空或 timestamp 空时返回空结果/明确错误，不 panic | TestFetchHistory_EmptyResult{empty result array, empty timestamps} | result=[]→error；timestamp=[] 且 quote 提供空数组→返回 len0 无 panic 无 error | PASS（注1） |
| error_handling[0] | HTTP 非 200 返回 error | TestFetch_NonOKStatus | 500 状态下 FetchQuote 与 FetchHistory 均返回 error | PASS |
| error_handling[1] | 畸形 JSON 返回 error | TestFetch_MalformedJSON | 非法 JSON 下两个 Fetch 均返回 error | PASS |
| non_functional[0] (verify_by:test) | 包覆盖率 ≥ 80% | 实跑 -cover = 82.0% | 满足门禁 | PASS |

## 注记（非阻塞）
- 注1 — 潜在 panic（不在 DoD 范围，故不阻塞）：FetchHistory line ~197 `r.Indicators.Quote[0]`
  在 `indicators.quote` 为空数组 `[]` 时会 index out of range panic。boundary[0] 的
  "empty timestamps" 用例提供了非空 quote 数组（含空 OHLC 切片），故未触发。done_criteria
  仅要求覆盖「result 空 / timestamp 空」两种情形，已满足；但建议（可选，后续加固）对
  `indicators.quote` 为空的响应增加防御与用例。
- 防御式认领记录：本任务原 verifier=null，我未越权改写他人 verifier；若 Leader 本意派给
  test-agent-1，请知悉我已在其忙碌期间接手并完成验证。

## 复核：fantasy-assertion 专项核查（应 Leader 要求，参照 TASK-010 案例）

**问题1：yahoo.go 是否有真实的 resp.StatusCode 检查？**
有，且是真实分支（非装饰）：
- FetchQuote: `if resp.StatusCode != http.StatusOK { return ... "unexpected status: %d" }`（line 113-115）
- FetchHistory: 同上（line 178-180）
状态检查位于 JSON 解码**之前**，非 200 立即返回，不会落到 decode/no-data 分支。

**问题2：HTTP 错误测试与畸形 JSON 测试是否走同一失败路径（fantasy assertion）？**
否。用 per-test 覆盖率剖面（`go test -run <单测> -coverprofile`）实证两者路径互斥：

| 代码块 (yahoo.go) | 含义 | TestFetch_NonOKStatus(500,`{}`) | TestFetch_MalformedJSON(200,坏JSON) |
|---|---|---|---|
| 113.38–115.3 | FetchQuote `return unexpected status`(114) | **HIT** | MISS |
| 118.67–120.3 | FetchQuote `return decoding response`(119) | MISS | **HIT** |
| 126.35–128.3 | FetchQuote `return no data`(127) | MISS | MISS |
| 178.38–180.3 | FetchHistory `return unexpected status`(179) | **HIT** | MISS |
| 183.67–185.3 | FetchHistory `return decoding response`(184) | MISS | **HIT** |

结论：非 200 测试**专走**状态检查返回行（114/179），畸形 JSON 测试**专走**解码错误返回行（119/184），
两条路径在真实代码中证明互斥。**不是** TASK-010 那种同路径假断言。error_handling[0]/[1] 均真实覆盖。

**残留弱点（非阻塞，建议加固）：** TestFetch_NonOKStatus 的响应体用 `{}`，断言仅 `err != nil`。
若未来有人误删状态检查，控制流会落到 no-data 分支仍返回 error，该测试**不会**变红（mutation 不敏感）。
当前真实代码下测试走的是状态检查路径（已由覆盖率证明），故判 PASS；建议（可选）把断言强化为
`strings.Contains(err.Error(), "unexpected status")`，或改用含有效数据的响应体，使状态检查成为唯一错误来源。

**判定维持：VERIFIED ✅**（status 已置 verified；本节为应 Leader 要求补充的实证证据）
