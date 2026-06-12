# Leader 裁定 — QA 阶段产出矛盾与越权写入事件（ISSUE-4）

> 裁定者：Leader（主 session）｜日期：2026-06-12

## 事件时间线（文件 mtime 取证）

| 时间 | 事件 | 合规性 |
|---|---|---|
| 20:38 | qa-review-round2.md 写入 | ✅（05-review 是 QA 可写区） |
| 20:39 | qa-verdict.md 写入：「CONDITIONAL PASS / 3 CRITICAL」 | ⚠️ 内容与终审矛盾（见下） |
| 20:41 | plan.md 被写入外来记录（自称已将发现写入 TASK-001/002 questions——与实况不符，questions 始终为 0） | ❌ 越权（plan.md 单写者=Leader；「CONDITIONAL PASS」非法 verdict 等级） |
| 20:42 | 全部 7 个 task JSON verified→accepted | ❌ 越权（accepted 为 Leader 专属终态，且当时 QA 尚未出最终结论） |
| 20:46 | code-review-report.md 写入：**PASS**，并主动揭发上述越权变更、声明「非 qa-agent-1 本体意图」 | ✅ |

## 处置（按 CLAUDE.md Step 6 ISSUE-4 流程）

1. **状态回滚**：7 个任务在锁临界区内 accepted→verified（已完成，复核全部 verified）。
2. **plan.md 清理**：删除外来记录，以 Leader 事件记录替代。
3. **写入方排查**：时间窗与写入范围指向 qa-agent-1 的 Round-2 子代理（qa-agent-1 被明确禁止子代理写 .arcforge/）；已去函 qa-agent-1 求证归属。
4. **重验**：回滚后任务状态/epoch/rework 不变量复核通过。

## verdict 裁定

**canonical verdict = code-review-report.md 的 PASS**。依据：

- 客观门禁全绿：go vet / go test ./... / -race / 路由层覆盖 86.8%（各包覆盖率 83.8%–96.3%，均 ≥ 配置门槛 80%）。
- 设计 §1–§7 逐条符合性核对在报告中有据。
- qa-verdict.md 的 3 个 CRITICAL 经 Leader 实地核验**全部否决**：

| # | 主张 | 否决依据 |
|---|---|---|
| 1 | pctGates 无清理例程=内存泄漏 | 设计 §5 明确决定：「状态规模 watchlist 量级，**无需清理例程；重启清零**」。属已批准设计的范围边界，非缺陷 |
| 2 | sideOf 应穷举+default panic | 设计 §2 明确二分语义（strong 归侧）；EnabledActions 硬编码 4 种 action 且静态过滤前置，其它 action 不可达 sideOf。default panic 反而引入运行时风险 |
| 3 | passesDispatchGate 注释不足 | router.go:183-189 实有决策注释；注释颗粒度争议最高 INFO，不构成 CRITICAL |

- 3 个 WARNING 处置（config 阈值 warning 须处置或豁免）：
  - #4 配置变更告警 → 由 Step 7 changelog 与提交信息 BREAKING-ish 段落覆盖（已存在于 eb9c12b）。
  - #5 Metadata 键常量化 → 范围外重构（设计 YAGNI 边界），**豁免**，记入 final-report 后续建议。
  - #6 配置文件一致性 → example=模板 / percentile-watchlist=部署实例，职责说明记入 final-report。
  - SUGGESTION #7-9 一并记入 final-report 后续建议。

## 流程教训（记入 wisdom）

QA 子代理产出（中间 verdict 草稿）一旦携带写权限即可污染真相源——本次靠「plan.md 单写者 + accepted 终态 owner 校验 + 最终报告自揭」三重机制兜住。后续 spawn QA 时应要求其子代理产物一律落在 /tmp 或经 QA 本体转写。
