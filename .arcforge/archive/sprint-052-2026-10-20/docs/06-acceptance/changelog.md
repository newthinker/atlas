# Changelog — sprint 2026-10-20 Pricewatch

## 新增
- `internal/pricewatch`：加密货币价格波动提醒——按配置的规则组监控 24h / 7d / 30d / 4y / 全历史窗口的跌幅（较最高价）与涨幅（较最低价），分档提醒，带防抖与每档次数上限；同资产同轮多条规则合并为一条消息、AI 只调用一次。
- 本地 SQLite（`data/pricewatch.db`）：日线全量、5 分钟线保留 8 天、规则状态与 kv；分段补数（K 线回填 binance 优先、取价 okx 优先），缺口自动重取，近期段要求完整覆盖。
- AI 下单建议（沿用主配置 `llm` 段）：超时 / 出错 / 解析失败时提醒照发、标注「AI 分析不可用」。
- 健康告警（取价失败、数据缺口、K 线同步失败，超过 `fail_alert_after` 发「监控异常」、恢复发「已恢复」，送达后才确认）与每日心跳。
- 所有 Telegram 消息封顶 4096 UTF-16 码元。
- 子命令：`atlas pricewatch run`、`atlas pricewatch once [--dry-run]`、`atlas pricewatch backfill [--symbol]`，`--pricewatch-config` 默认 `configs/pricewatch.yaml`。
- 示例配置 `configs/pricewatch.yaml`；launchd 托管 `deploy/launchd/com.newthinker.atlas.pricewatch.plist`（KeepAlive、仓库内只放 Telegram 占位）。

## 配置校验
- 严格解析（未知键报错）；Telegram 只接受 `${ENV}` 写法，明文、展开为空、`REPLACE_WITH_` 占位均报错；规则阈值拒绝 NaN/Inf；超时与告警阈值须为正。

## 未改动
- 不修改任何既有文件或符号（`cmd/atlas` 以 `init()` 注册子命令）。
