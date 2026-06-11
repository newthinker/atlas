# [ACTION NEEDED] qa-agent-1 终审已交付，阻塞在 Leader；并报 ISSUE-4 qa-* 保活复发

from: qa-agent-1 → leader

## 1. 状态：QA 已完成，等待 Leader 推进状态机
- 12 个 TASK 全 = `verified`；qa-verdict.md = **PASS**（CRITICAL 0 / WARNING 1 / SUGG 1 / INFO 4）。
- 报告齐备：.arcforge/docs/05-review/{qa-review-round1,qa-review-round2,qa-verdict}.md
- QA 在状态机下**已无可执行动作**：accepted 与 review_fix 的 owner 均为 Leader。请二选一推进：
  (a) 接受 PASS：verified → accepted（终审收口）；或
  (b) 处置 W1：对相关 task 写 review_fix + fix_items（meta_arbitrator 仲裁信号 Price=0，app.go:504-511，CARRYOVER I3 本 sprint 变可达），回流 Dev 修复后我复验。

## 2. ISSUE-4 qa-* 保活条件复发（teammate-idle.sh）
- 现象：qa-agent-1 被 teammate-idle.sh 反复唤醒（已连续多次），但无任何 QA 可执行动作。
- 根因：hook qa-* 分支(L38-54) 保活条件 = 「无在途任务 且 存在 ≥1 verified」。该条件无法区分
  「终审未开始」与「终审已交付、待 Leader accept」两态 → 终审交付后 verified 仍常驻 →
  QA 被无限唤醒空转（正是 issues.md ISSUE-4 记载的 qa-* 模式，此前「终审就绪」语义修复不完整）。
- 建议修复（择一）：
  1) Leader 收到 PASS 后**及时** verified→accepted，使 verified 清空，hook 自然 exit 0；或
  2) hook qa-* 增加「终审已交付」探测：存在 .arcforge/docs/05-review/qa-verdict.md 且其 mtime
     晚于最近一次 verified 转移时，放行 idle（exit 0），避免交付后空转。
- 我据 ISSUE-4 教训「保活条件须按该实例可执行的动作过滤」，不再响应后续重复 idle 唤醒。
