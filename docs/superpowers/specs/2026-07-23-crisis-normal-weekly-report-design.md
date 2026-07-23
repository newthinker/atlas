# Cassandra NORMAL 态周报 设计方案

> 日期：2026-07-23
> 需求：危机监控在 NORMAL 状态下也发送周报（现状：NORMAL 只有月报，周报为 WATCH 态专属）
> 关联：`docs/plans/2026-07-14-crisis-notification-templates-design.md`（消息矩阵定稿）、
> `docs/ops/crisis-monitor-notifications.md`（通知频率现状手册）

## 1. 需求与已确认决策

NORMAL 态平静期每月一条月报的静默期过长，用户希望每周有一次「平安确认」。

与用户确认的两项产品决策：

1. **月报关系**：周报 + 月报并行，撞日只发月报。NORMAL 态每周一发周报；当月首个
   交易日仍发月报；若周一恰是当月首个交易日，只发月报——保持「结构化消息每日
   至多 1 条」的既有不变量。
2. **内容形态**：复用 WATCH 周报骨架（`renderWeekly` 五段结构），去掉「退出进度
   n/20」行（NORMAL 态无退出概念）。首行沿用
   `[P1] 📅 Cassandra 周报 · MM-DD 当周 · NORMAL 已持续 N 个评估日`。

**「周一」语义**：与现有 WATCH 周报一致，指**评估日**（`res.Date`，即被评估的交易
日）为周一；消息实际发出于周一晚间 22:45+ 或次日 07:30 兜底唤起（eval 评估前一
交易日数据的既有机制，不改）。

## 2. 方案选型

采用**方案 A：三值 SummaryKind 枚举**——cmd 层一处判日，渲染层保持纯函数。

否决项：B）渲染层从 `Res.Date` 自行判周一/月初——日历逻辑在 cmd 与 crisis 包两处
重复，违背「cmd 组装输入、渲染纯函数」的架构约定（通知设计 §8）；C）NORMAL 周一
复用 `SummaryDue=true` 并靠 `Trends` 是否为空区分月报/周报——语义隐晦、易碎。

## 3. 设计

### 3.1 触发（cmd/atlas/crisis.go）

`summaryDue(date, state) bool` 升级为：

```go
// crisis 包内定义
type SummaryKind int
const (
    SummaryNone SummaryKind = iota
    SummaryWeekly
    SummaryMonthly
)
```

cmd 层 `summaryKind(date, state)` 判定：

| 状态 | 规则 |
|---|---|
| NORMAL | 当月首个交易日 → `SummaryMonthly`；否则周一 → `SummaryWeekly`；否则 `SummaryNone` |
| WATCH | 周一 → `SummaryWeekly` |
| BREWING / CRISIS | `SummaryNone`（日报已覆盖） |

撞日（月初首交易日恰为周一）由分支顺序天然归月报。

### 3.2 路由（internal/crisis/notify.go）

`NotifyContext.SummaryDue bool` 替换为 `Summary SummaryKind`。`Messages` 互斥
switch 改为：

- `Transitioned` → 变更消息（不变）
- `BREWING/CRISIS` → 日报（不变）
- `Summary == SummaryMonthly && NORMAL` → 月报
- `Summary == SummaryWeekly && (NORMAL || WATCH)` → 周报

### 3.3 渲染（internal/crisis/notify_render.go）

`renderWeekly` 尾段按状态分叉：

- WATCH（不变）：`退出进度：触发条件已连续解除 N 日（回 NORMAL 需连续 20 日）\n下次周报：下周一 · 状态变更即时通知`
- NORMAL（新增）：`下次周报：下周一 · 状态变更即时通知`（无退出进度行）

### 3.4 组装（cmd/atlas/crisis.go buildNotifyContext）

- `ClearStreak` 仍仅 `WATCH ∧ SummaryWeekly` 时计算（NORMAL 无退出进度）。
- `Trends`（21 观测日窗口）仅 `SummaryMonthly` 时组装——NORMAL 周报不拉趋势
  窗口，评估成本不变。

## 4. 边界与错误处理

- **周一为美国假日**：required 序列缺观测 → 既有「数据未齐空跑」机制生效，该周
  周报随评估一起顺延（评估日仍是周一则照发，评估被跳过则该周无周报）——与
  WATCH 周报现状一致，不新增逻辑。
- **周一发生状态变更**：变更消息优先（switch 首分支），当周周报让位——现状语义
  不变。
- **枚举默认值**：`SummaryKind` 零值为 `SummaryNone`，未显式赋值的
  `NotifyContext`（如测试构造）安全静默。

## 5. 测试要点

1. `summaryKind` 全分支：NORMAL 周一→weekly、NORMAL 月初首交易日→monthly、
   撞日（月初首交易日 ∧ 周一）→monthly、NORMAL 普通日→none、WATCH 周一→weekly、
   WATCH 非周一→none、BREWING/CRISIS 任意日→none、非法日期→none。
2. `Messages` 路由：NORMAL ∧ weekly → 恰 1 条周报；NORMAL ∧ monthly → 恰 1 条
   月报；WATCH ∧ weekly 回归不变。
3. `renderWeekly`：NORMAL 输出无「退出进度」行且含「下次周报」行；WATCH 输出
   逐字节回归不变（golden 对比现有测试）。
4. `buildNotifyContext`：NORMAL ∧ weekly 时 `Trends` 为空、`ClearStreak` 为零。

## 6. 文档更新

- `docs/ops/crisis-monitor-notifications.md`：§2 频率表 NORMAL 行加周报、撞日
  规则；§5 排障速查「NORMAL 态非月初静默属正常」改为「非周一静默属正常」。
- `docs/plans/2026-07-14-crisis-notification-templates-design.md` 不改（历史定
  稿），本设计文档即为增量修订记录。

## 7. 范围外

- 周报内容增强（如 NORMAL 周报加 AMBER 计数、趋势摘要）——本期只复用现有骨架。
- notifier 优先级路由、其他状态的频率调整。
