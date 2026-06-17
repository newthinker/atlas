# Leader Decisions

## D1：覆盖率 DoD 基准修正（整包 → 变更文件）
- **背景**：TASK-001 width.go 自身 100% 覆盖，但整包 telegram 仅 70.3%（被既有未测的 Send/sendMessage/escapeMarkdown 等真实 HTTP 代码拉低，22.2% 的 SendBatch 待 T002 提升）。
- **决策**：把 TASK-001/002 的 non_functional 覆盖率基准从「整包 ≥80%」改为「变更文件/新增函数 ≥80% + 整包零回归（不下降）」。rework_count 不增。
- **rationale**：width.go 只会抬高包覆盖率 → 包在 TASK-001 前就 <80%，整包门槛对本任务范围外、结构性不可达；强行达标 = 要求 dev 给没动过的 HTTP 代码补 mock，纯范围蔓延。符合 CLAUDE.md「反复返工大概率是 DoD 本身不可实现」与 config dev_scope=changed-package 的精神（dev 自己改的代码必须覆盖）。
- **影响**：QA 终验与 final coverage 也按此基准；不把既有未测 HTTP 路径计入本 Sprint 责任。

## D2：串行路径 flush 修正（独立评审发现）
- TASK-004 用 defer FlushNotifications 覆盖 workers<=1 串行出口，防止 batch 信号在串行配置下永不发送+跨轮泄漏。详见 02-plan/independent-review.md #1。
