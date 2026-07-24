# 需求 ↔ DoD 追溯矩阵(Sprint 027 / Prism M2.1)

| 需求 | 覆盖任务 | 关键 DoD 锚点 |
|---|---|---|
| R1 节流(500ms/持锁串行) | TASK-001 | TestDoThrottlesConsecutiveRequests(≥60ms 实测间隔) |
| R2 429/5xx 退避 + Retry-After 优先 | TASK-001 | Retries429/Retries5xx/HonorsRetryAfter 三用例 |
| R3 重试预算封顶 | TASK-002 | TestDoRetryBudgetExhausted(恰 3 请求,绝不发第 4 个) |
| R4 三调用点统一走 do 门 | TASK-001(Quote/History), TASK-002(EPS) | 调用点替换 + TestFetchEPSHistoryRetries429 |
| R5 发布验证(launchd 0 failed) | Step 7 交付阶段执行(非 dev 任务) | final-report 部署清单 |
| N1 常量不进 config | TASK-001 non_functional(review) | |
| N2 错误语义零变更 | TASK-001 error_handling(GivesUpAfterMaxRetries 保留 unexpected status) | |
| N3 既有测试零修改 | TASK-001/002 boundary | |

反向:两任务全部 DoD 均可回指 R1~R4/N1~N3,无凭空 DoD;R5 显式移交 Step 7(非孤儿)。

## 独立 reviewer 反审处置记录(dod-reviewer-027,NEEDS-FIX)

- 必补「并发安全 -race」→ TASK-002 non_functional(test):go test -race 全包。
- 建议「Retry-After 非法值回退」→ TASK-002 boundary 新增 TestDoRetryAfterInvalidFallsBack。
- 建议「FetchQuote 接线无直接测试」→ TASK-002 non_functional(review):grep 无残留 y.client.Do;test-agent-5 验 001 时附加核对。
- 模糊「网络层错误不重试无测试锚定」→ TASK-002 error_handling 独立测试条目。
- TASK-001 已被认领(in_progress,owner=dev-13),DoD 主体不动——增补全部落 TASK-002(pending,leader 可写),机制合规。validator 重跑 ✓。
