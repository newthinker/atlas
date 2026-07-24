# Sprint 027 Code Review — Prism M2.1 (yahoo 限流加固)

- 审查者: qa-agent-6
- 范围: master..feature/prism-m2.1 (cf147dd, fd0dedd, 6288a48)
- 变更: internal/collector/yahoo/yahoo.go、eps.go、throttle_test.go (10 测试)

## VERDICT: PASS (iteration 2 复核后维持)

已交付 done_criteria 全部有对应测试且通过；构建 exit 0；throttle 测试实跑全绿。无 CRITICAL、无未决 WARNING。

## iteration-2 小节 (Retry-After cap 修复复核 — commit 6288a48)

原 iteration-1 唯一 WARNING「Retry-After 无上界 cap」已修复并复核通过，WARNING 关闭。
- 修复: retryAfterWait 新增 maxWait 参数，头部路径改 `min(secs*Second, maxWait)`；
  常量 maxRetryAfterWait=60s；实例字段 maxRetryAfter 可测试覆盖，经 NewWithBaseURLs 初始化
  故生产 New() 路径同样生效。
- scope 精准: 仅封顶 Retry-After 头部路径；fallback 指数退避 backoffBase<<attempt (≤4s) 本已有界，
  未画蛇添足。
- 测试锚定: TestDoRetryAfterCapped(Retry-After:1 vs 100ms cap → 计时 <500ms) 本轮实跑 0.10s PASS；
  既有 TestDoRetryAfterInvalidFallsBack/TestDoHonorsRetryAfterHeader 零回归。既有测试零修改。
- 无新问题: min 为 Go 内置、字段初始化完整、并发/资源语义不变。

## iteration-1 遗留结论 (INFO,不变)
retryBudget=20 实例级共享、launchd StartCalendarInterval 每日新建 yh 重置预算——语义可接受。
四问 c/d(重发同一 GET、throttle 持锁 sleep、body 排干、25 标的 +~25s 节流开销)均无隐患。

## 结论
唯一 WARNING 已修复复核通过，交付质量扎实。维持 PASS。
