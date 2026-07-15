# Changelog — 通知设计 v1.1 落地（2026-07-15，sprint-021）

## Added
- `staleDowngradeWarning`：状态降级当日有指标断更且断更前为 RED/AMBER 时的溯源警示行（R1a）。
- P2 运维速报断更前 RED/AMBER 条件警示行（R1b）。
- N3 集成测试：装配层联合验证降级转移消息与 P2 速报同时携带警示。

## Changed
- 降级首行条件符号：异常区为空 → `✅ 状态解除`；非空 → `🔽 状态回落`（R2）。
- 语义句：CRISIS→WATCH「危机状态退出…其余层面可能仍异常，见下」；WATCH→BREWING 删「3–12 个月」时间窗、
  加「此为状态描述而非预测，不构成操作依据」（R2/R3）。
- diffLine 双非色彩迁移改用具体说明（如「转季末抑制（原数据断更(STALE)）」），不再「转白（原白）」（R4）。
- 盘中速报去「疑似 carry trade 快速平仓」归因，改「成因未核实，非交易信号」内联限定语（R5）。
- P2 术语外化：「已标记 STALE、不再计入触发判定；数据恢复后自动重新计入」（R6）。
- 测试：全家族页脚归属断言由「非交易信号」子串改为 HasSuffix(notifyFooter) + 家族计数 5+2（R5 连锁）。

## Fixed
- cmd/atlas/crisis_test.go 既有 gofmt 漂移（T3 顺带，backlog B4）。

## 提交清单（3）
783b39a / a7e41f9 / da72f04
