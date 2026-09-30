# Plan — sprint 2026-09-30 Tiingo 美股备用数据源

- 分支 `feature/tiingo-source`，起点 `8d1c6cc7edf573c9879b53125f486eb6d515f01f`，当前 `b3232d6f3e2b07610bafe31e7381add623fdb442`（+1799/−37，25 文件）
- **阶段：Step 6 QA（qa-tg 两轮审查中）**；9/9 verified；validator rc=0；transition 审计 84 行执行者合法、末态一致
- AD-23 simplifier 兜底：22 文件 0 改动（指纹前后一致）
- 返工：TASK-003 R1（基底排除数字无守卫）、TASK-004 R1（token 前缀泄露，截断先于脱敏）

| 任务 | 标题 | 状态 | rework | dev | verifier |
|---|---|---|---|---|---|
| TASK-001 | Registry 按注册顺序 | verified | 0 | dev-tg-a | test-tg-a |
| TASK-002 | policy tiingo.daily | verified | 0 | dev-tg-b | test-tg-b |
| TASK-003 | 白名单 symbols.go | verified | 1 | dev-tg-c | test-tg-a |
| TASK-009 | normalize.go 拆股折算 | verified | 0 | dev-tg-d | test-tg-b |
| TASK-004 | client.go 直连/Gate/错误 | verified | 1 | dev-tg-d | test-tg-b |
| TASK-005 | collector.go | verified | 0 | dev-tg-d | test-tg-a |
| TASK-006 | prism 有序多跳 | verified | 0 | dev-tg-a | test-tg-a |
| TASK-007 | serve 装配 + 配置示例 + gate_wiring | verified | 0 | dev-tg-b | test-tg-b |
| TASK-008 | 集成测试（integration 标签） | verified | 0 | dev-tg-c | test-tg-a |
