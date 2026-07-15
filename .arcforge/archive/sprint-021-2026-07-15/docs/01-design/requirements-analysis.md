# 需求分析 — crisis 通知设计 v1.1 落地（sprint-021）

**需求来源（唯一真相源）**：`docs/plans/2026-07-15-crisis-notify-v1.1-impl.md`
**上游设计**：`docs/plans/2026-07-15-crisis-notify-v1.1-design.md`（R1–R6 + 4 条新增原则）
**动机**：sprint-020 QA 对抗审查的 6 条设计反馈。

## 降级/流程说明

- ECC 不可用 → 本设计已在当前 session 经 superpowers:brainstorming 完成（用户逐项确认
  W1 双侧增强 / I3 去归因+短免责 / W2 条件符号 / 全量 6 条范围），**不重复 brainstorming**。
- validator/arcforge-write.sh 缺失 → 沿 sprint-020 降级路径（人工核查 + with-task-lock.sh 原子写）。
- gitnexus 门禁：索引最新（98ef8ca 后仅文档提交）；teammate 会话 MCP 版本若再不匹配则沿用 Leader 代跑机制。

## 核心功能（3 项，对应 impl Task 1–3）

| # | 功能 | 设计条目 |
|---|---|---|
| F1 | 语义句 2 键修订 + 降级条件符号 ✅/🔽 + 断更降级溯源警示行（staleDowngradeWarning） | R1a/R2/R3 |
| F2 | diffLine 双非色彩迁移具体文案 + P2 速报术语外化与断更前状态条件警示 | R1b/R4/R6 |
| F3 | 盘中速报去因果归因 + 内联限定语 + 全家族页脚断言改 HasSuffix(notifyFooter) | R5 及测试连锁 |

## 非功能性需求

- 零接口/签名/NotifyContext 字段变更；cmd 生产代码不动（仅测试断言措辞适配）。
- 禁词/页脚归属/4096/纯文本约束不变；新文案禁词零引入。
- sprint-020 测试纪律全量适用（落界+变异自检+逐支断言+异值锁——impl 文档 Global Constraints 已列）。
- GOTOOLCHAIN=local；每任务全仓可编译。

## 模糊/缺失需求点

无。6 项修订的措辞、条件、排序、测试连锁在设计 v1.1 与 impl 文档中全部字面给定
（含常量名核实 types.go:13-15、多指标 AllIndicators 序、AMBER 落界归属）。

## 复杂度评估

全部「简单~中等」：F1 中等（新助手+条件分支），F2/F3 简单（分支+措辞+断言重构）。
