# Changelog — Telegram 信号汇总表格

## Added
- `internal/notifier/telegram/width.go`：CJK 感知的显示宽度工具（displayWidth/padRight/isWide），等宽表格对齐用，无第三方依赖。
- `internal/notifier/telegram` 的 `formatBatch`/`renderTable`：按动作分组（买入/卖出/持有）的等宽 digest 表格，组内按置信度降序，含中文名列对齐。
- `internal/router` 的 `Router.FlushNotifications()` 与 `Config.BatchNotify`：批量通知缓冲，cycle 末一次性 digest 发送。
- 配置项 `router.batch_notify`（默认 `true`），`configs/config.example.yaml` 已文档化。

## Changed
- `Telegram.SendBatch` 重写为渲染分组表格；新增 `sendRaw`（不转义）供 digest 路径用，`sendMessage` 保持 escapeMarkdown 供逐条 `Send`。
- `runAnalysisCycle` 在 `defer` 中调用 `FlushNotifications`，覆盖串行（workers≤1）/并行/取消所有出口。

## Behavior
- **默认行为变更**：一轮分析的多条信号现汇成**一条**表格 digest（原为逐条即时消息）。设 `router.batch_notify: false` 可回退逐条即时发。
- 路由决策/冷却/执行/信号存储语义不变（仍逐信号）；空轮不发消息。

## Commits
- 619faaf feat(TASK-001): CJK-aware display-width helpers
- 3f0bb11 feat(TASK-003): batch-notify buffer + FlushNotifications
- 6629618 feat(TASK-002): group-by-action digest table in SendBatch
- 39fb228 feat(TASK-004): default batch_notify on; flush at cycle end (incl. serial path)
- 611d8e4 fix(TASK-002): digest skips markdown escape + omit zero timestamp
