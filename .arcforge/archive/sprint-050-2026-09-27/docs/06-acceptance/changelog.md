# Changelog — sprint 2026-09-27

## Added
- `atlas bank report` 子命令：按 `configs/bank-monitor.yaml` 拉取东方财富（经本地 aktools）银行股不良贷款率 / 拨备覆盖率 / 核心一级资本充足率，生成阈值预警、环比恶化预警、同期统计与排名，分段推送 Telegram；`--dry-run` 只打印；`--bank-config` 指定配置。
- 新包 `internal/bank`：配置校验（A 股 / A+H，纯港股拒绝）、EMSource 数据源（字段常量集中、字符串/null/Inf 容错、字段整列缺失标注）、按指标回退的最新值与环比/同比、A+H 去重、众数统计期与领先主体、纯文本渲染与按行分段。
- `configs/bank-monitor.yaml`（示例：招商银行、邮储银行、招商银行H）。
- `deploy/launchd/com.newthinker.atlas.bank-monthly.plist`（每月 1 日 09:00）。
- 集成冒烟测试 `internal/bank/source_integration_test.go`（`-tags integration`）。

## 退出码
0 全部成功；2 部分主体拉取失败（报告照推）；1 配置非法 / 全部失败（推错误摘要）/ 推送失败 / 非 dry-run 拿不到 Telegram sender。
