# 设计规格 — crisis NORMAL 态周报

> 本 Sprint 不重复造设计。**唯一权威设计文档**（已提交入库，b3039c2）：
> `docs/superpowers/specs/2026-07-23-crisis-normal-weekly-report-design.md`
>
> 行级实施计划（已提交入库，7d14d4a）：
> `docs/superpowers/plans/2026-07-23-crisis-normal-weekly-report.md`

## 摘要（供任务拆分与验证对照）

**方案 A：三值 SummaryKind 枚举**（否决 B：渲染层自行判日；否决 C：复用 SummaryDue + Trends 空判别）。

三层改动：

1. **crisis 包路由**（`internal/crisis/notify.go`、`notify_render.go`）：
   新增 `SummaryKind`（`SummaryNone`=零值 / `SummaryWeekly` / `SummaryMonthly`）；
   `NotifyContext.SummaryDue bool` → `Summary SummaryKind`；
   `Messages` 互斥 switch：`SummaryMonthly∧NORMAL → 月报`，`SummaryWeekly∧(NORMAL|WATCH) → 周报`。
2. **渲染分叉**（`notify_render.go` renderWeekly）：
   WATCH 尾段不变（退出进度行 + 下次周报行）；NORMAL 尾段仅「下次周报：下周一 · 状态变更即时通知」。
3. **cmd 判日与组装**（`cmd/atlas/crisis.go`）：
   `summaryDue` → `summaryKind(date, state)`：NORMAL 首交易日→Monthly、周一→Weekly；WATCH 周一→Weekly；其余 None（撞日由分支顺序归月报）。
   `buildNotifyContext`：ClearStreak 仅 `WATCH∧SummaryWeekly`；Trends 仅 `SummaryMonthly∧NORMAL`。

## 关键不变量

- WATCH 周报输出**逐字节**不变（既有 TestRenderWeekly 原样通过）。
- 结构化消息每日至多 1 条（switch 互斥结构不变）。
- 评估成本不变（NORMAL 周报不拉 Trends 窗口）。
- 「周一」= 评估日 `res.Date`，不改 launchd 调度。

## 测试要点（设计 §5，DoD 直接来源）

1. `summaryKind` 全分支（含撞日 2026-06-01、8/1 周六→8/3 撞日、坏日期、周六）。
2. `Messages` 路由：NORMAL∧weekly → 恰 1 条周报；NORMAL∧monthly → 恰 1 条月报；WATCH 回归。
3. `renderWeekly`：NORMAL 无「退出进度」行、含「下次周报」行；WATCH 逐字节回归。
4. `buildNotifyContext`：NORMAL∧weekly 时 Trends 为空、ClearStreak 为零。
