# TASK-201 metrics Registry.Snapshot() — 验证报告

- 验证者: test-agent-1
- 结论: **VERIFIED（PASS）**
- commit: bfc46fb（首版）→ dc8c291（AD-13a 增量），epoch=2, dev-agent-1 / 分支 feature/audit-optimization-wave1-cleanup
- 依据修订后 done_criteria（functional[3] 按 AD-13a 更新）

## 反向验收
- 两 commit 改动 = snapshot.go / snapshot_test.go（estimated_files）+ go.mod（client_model indirect→direct）。
- go.mod 提升合理连带：snapshot.go 直接 import dto（client_model/go）。
- metrics.go（RecordRequest 本体）与 internal/alert 零改动（git 确认未在 diff 中）。

## Done Criteria 覆盖矩阵

| # | 维度 | 标准 | verify_by | 证据（fresh 非缓存） | 判定 |
|---|---|---|---|---|---|
| F0 | functional | Snapshot 返回 map[string]float64：gauge 当前值 / counter 累计值 | test | TestSnapshot_GaugeAndCounter：gauge=42、counter=7 PASS | **PASS** |
| F1 | functional | 同名多 label 聚合求和为单键（3+5→8） | test | TestSnapshot_MultiLabelSum：test_multi_total=8 PASS | **PASS** |
| F2 | functional | histogram 展开 _count/_sum | test | TestSnapshot_Histogram：_count=2、_sum=6 PASS | **PASS** |
| F3 | functional | status 为 3 位数字或 [1-5]xx 字符串归入 _<N>xx 求和；status=ok 等不产额外键（AD-13/13a） | test | 正则 `^([1-5]xx\|[1-9][0-9]{2})$`，按 code[:1]+"xx" 归类。TestSnapshot_StatusClassKeys：500+502+"5xx"→_5xx=6、"200"→_2xx=4、"ok"→无 _okxx；base=19。**真实路径** TestSnapshot_RecordRequestProduces5xx：NewRegistry()+RecordRequest(503/200)→ snap["http_requests_total_5xx"]=1、["_2xx"]=1（真实键名，无 atlas_ 前缀）PASS | **PASS** |
| B0 | boundary | 空 registry → 空 map，非 nil，不 panic | test | TestSnapshot_EmptyRegistry：非 nil 且 len=0 PASS | **PASS** |
| E0 | error_handling | Gather 出错不 panic，处理已收集部分 | test | snapshot() 用 `mfs, _ := g.Gather()` 忽略 err 处理 partial；TestSnapshot_GatherError_NoPanic：partial_counter=3、非 nil、无 panic PASS。策略：返回已收集部分 | **PASS** |
| N0 | non_functional | Snapshot 签名不暴露 prometheus 类型；alert 零改动 | review | 公开签名 `Snapshot() map[string]float64`（prometheus 类型仅在未导出 snapshot()/addStatusClass 内部）；alert 未在 diff | **PASS** |
| N1 | non_functional | gauge/counter/histogram/多 label 四场景单测 + go test ./internal/metrics/... 全绿 | test | 四场景齐备；metrics 包 fresh 全 26 用例 PASS，ok | **PASS** |

## Leader 两关注点确认
1. 真实路径实证键名：TestSnapshot_RecordRequestProduces5xx 用真实 NewRegistry()+RecordRequest（非注入 mock），断言真实键 http_requests_total_5xx / http_requests_total_2xx（无 atlas_ 前缀，与 metrics.go 一致）。✓
2. 正则两形式覆盖：数字 500/502 与字符串 "5xx" 均测（TestSnapshot_StatusClassKeys）；RecordRequest 本体零改动。✓

## 非阻断观察（不影响判定）
- 正则数字分支 `[1-9][0-9]{2}` 匹配 100-999，含 600-999（如 "700"→_7xx），略宽于 [1-5]xx。
  但 statusToString 仅产 1xx-5xx、真实 HTTP 状态码不超 5xx，无实际影响。

全部条目 PASS，证据充分 → VERIFIED。
