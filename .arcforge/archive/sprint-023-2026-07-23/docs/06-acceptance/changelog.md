# Changelog — Sprint: crisis NORMAL 态周报（2026-07-23）

## Added
- NORMAL 态每周一（评估日）发送 Cassandra 周报（无退出进度行）；撞月初首交易日只发月报（8ae94fc, b717e9b, f5d7b82）。
- `internal/crisis.SummaryKind` 三值枚举（None/Weekly/Monthly），`NotifyContext.Summary` 字段替换 `SummaryDue bool`。
- `cmd/atlas` `summaryKind(date, state)` 判日（替换 `summaryDue`），Trends 仅月报组装、ClearStreak 仅 WATCH 周报计算。

## Changed
- `renderWeekly` 尾段按状态分叉：退出进度行仅 WATCH 渲染；WATCH 输出逐字节不变（回归锁定）。
- 运维手册 docs/ops/crisis-monitor-notifications.md：§2 频率表 NORMAL 拆周报/月报两行、§5 排障速查改「非周一静默属正常」、§2 末尾调度说明改 summaryKind 语义（cbfbed9, d59bc74）。

## 不变
- launchd 调度、DB schema、配置文件、notifier 通道均零改动；评估成本不变。
