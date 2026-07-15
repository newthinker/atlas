# Changelog — crisis 通知模板重写（2026-07-15）

## Added
- `internal/crisis/notify_format.go`：层名/冰山层序/emoji/tag/读数与变化量格式/趋势箭头/sparkline 等 13 个无状态格式化原语。
- `internal/crisis/notify_render.go`：导出 `NotifyContext`/`Trend` + 七类消息渲染器（状态升级/降级、日报、周报、月报、P2 运维速报）与语义句查表（8 转移、%d 注入配置）。
- `internal/crisis/statemachine.go`：`ClearStreakDays(hist, state, max)` 态内免触发连续日数（周报退出进度）。
- `cmd/atlas/crisis.go`：`buildNotifyContext`（AppendEvaluations 之前组装渲染输入）。
- `IndicatorResult` 新字段 `PersistDays`/`Wow`/`WowOK`，评估行 detail JSON 同步携带（omitempty）。

## Changed
- `internal/crisis/notify.go` 重写：`Messages(cfg, nc)` 装配矩阵（结构化家族至多一条 + NewStale 各一条 P2）；
  `FormatIntradayAlert` 盘中速报按设计 §5.7。
- `executeCrisisEvalDaily` 接线新通知链路（组装先于落库）；`executeCrisisIntraday` 切换新文案。
- `evalVIX`/`evalSOFREFFR`/`evalUSDJPY` 填充持续性/周环比字段（判定语义不变，sofr 窗口经 lastN 还原）。
- `splitZones` 异常区排序为显式三级全序比较器（severity → 冰山层 → AllIndicators 序）。

## Removed
- 旧 `footer` 常量、旧 `Messages(res, days, due, stale)`、`indicatorLines`、cmd 的 `staleIndicators`（含其测试）。

## Fixed
- `ClearStreakDays` 态内计数（QA C1）：修复前 CRISIS 尾段免触发日会虚高 WATCH 周报退出进度。
- impl 参考实现 2 处：NO_DATA 行分隔符、含 ⚪ 时的全绿标题误判（allGreen）。

## 提交清单（16）
d43d029 / 0e9ee0a / f0c85cf / 00c1baf / c73778f / 83bf0d4 / 0beae94 / ce52fa4 / e9e1222 /
569933e / 9cf9e34 / e71159e / 28a7cca / 058765f / caab292 / 2955906
