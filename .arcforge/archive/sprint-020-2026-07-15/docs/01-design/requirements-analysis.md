# 需求分析 — Cassandra 危机监控通知模板重写

**需求来源（唯一真相源）**：`docs/plans/2026-07-14-crisis-notification-templates-impl.md`（实施方案）
**上游设计**：`docs/plans/2026-07-14-crisis-notification-templates-design.md` v1.0
**分析日期**：2026-07-14

## 降级说明（capabilities）

- ECC 不可用（config `capabilities.ecc=false`）→ 未调用 `/multi-plan`。
- 上游设计已经过 brainstorming 精炼（commit `2edac07` "add notification template design (brainstormed)"），
  实施方案含 8 条"已核实、执行者不需重新决策"的补充决策 → **不重复 brainstorming**，
  直接以实施方案为需求基线。
- Go validator（`validator/`）不存在 → 任务图校验降级为 Leader 人工核查（DAG 无环/wave 序/scope 互斥），
  记录于 02-plan。
- `arcforge-write.sh` hook 不存在 → 状态写入降级为各 owner 直接原子写 + `with-task-lock.sh` 临界区。

## 核心功能列表

| # | 功能 | 设计条目 |
|---|---|---|
| F1 | IndicatorResult 暴露 PersistDays/Wow/WowOK，规则层填充并写入评估行 detail JSON | §8、§6.3 |
| F2 | `ClearStreakDays` 导出助手（any_trigger=false 连续历史日数） | §6.6 |
| F3 | 格式化原语：层名/冰山层序/emoji/tag/读数与变化量格式/趋势箭头/sparkline | §6.1–6.4 |
| F4 | `NotifyContext`/`Trend` 类型 + 指标行渲染 + 异常/其余分区 | §8、§6.2、§5 |
| F5 | 语义句查表（8 转移、%d 注入配置）+ 状态升级/降级渲染（消息 1/2） | §4.1、§5.1/5.2 |
| F6 | BREWING/CRISIS 日报（较昨日差异行）+ WATCH 周报（退出进度）（消息 3/5） | §5.3/5.5、§6.5/6.6 |
| F7 | NORMAL 月报（趋势区 sparkline）+ P2 数据断更速报（消息 4/6） | §5.4/5.6 |
| F8 | cmd 层 `buildNotifyContext`（AppendEvaluations 之前组装） | §8 |
| F9 | 切换：`Messages(cfg,nc)` 装配 + `FormatIntradayAlert`（消息 7）+ cmd 接线 + 旧接口删除 | §2、§5.7 |

## 非功能性需求

- **文案合规**：禁词"必然/一定/即将"；结构化家族必含 `非交易信号` 页脚，速报家族不带（§7/§2）。
- **纯函数渲染**：渲染层无 IO、无状态，输入收拢为 `NotifyContext`。
- **配置驱动**：阈值一律来自 `configs/crisis-monitor.yaml`，语义句 %d 渲染时注入；
  `persistLookbackObs=30` 是显示口径常量，允许留在代码（补充决策 3）。
- **消息长度** ≤ 4096（telegram 上限）；全部 `SendText` 纯文本无 parse_mode。
- **错误语义不变**：发送失败仅记 stderr、评估已落库。
- **兼容性**：Task 1–8 期间旧 `Messages` 不动，任一提交点全仓可编译；Task 9 一次性切换。
- **构建约束**：所有 go build/test 前缀 `GOTOOLCHAIN=local`；sqlite 固定 v1.38.2。
- **流程约束**：改既有 symbol 前跑 `gitnexus_impact`（Task 1/9 明确列出目标）；
  每任务提交前跑 `gitnexus_detect_changes()` + code-simplifier sub-agent。

## 模糊/缺失需求点

无。设计 v1.0 的歧义已由实施方案的 8 条补充决策全部消解（sparkline 分桶、StateDays 语义、
ClearStreak 含当日、周跌触发判定、5y 分位显示范围、全绿标题条件、StaleLastObs 来源、persistLookbackObs）。

## 复杂度评估

- 简单：F2（T2）、F3（T3）
- 中等：F1（T1）、F4（T4）、F5（T5）、F6（T6）、F7（T7）、F8（T8）
- 复杂：F9（T9，唯一签名切换点，跨 internal/crisis 与 cmd/atlas 两包）
