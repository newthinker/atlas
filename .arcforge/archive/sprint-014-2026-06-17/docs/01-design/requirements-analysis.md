# 需求分析 — Telegram 信号汇总表格

> 来源：`docs/superpowers/plans/2026-06-17-telegram-digest-table.md`
> 关联 spec：`docs/superpowers/specs/2026-06-17-telegram-digest-table-design.md`

## 目标

把一轮分析产生的多条放行信号，从「逐条独立 Telegram 消息」改为「一条按动作分组的等宽表格 digest」。

## 核心需求（功能性）

| # | 需求 | 验收要点 |
|---|------|---------|
| R1 | 一轮多条信号汇成**一条** Telegram 消息 | 一轮 N 条放行信号 → 1 次 `SendBatch` |
| R2 | 按「买入 / 卖出 / 持有」分组 | 买入=`strong_buy`+`buy`；卖出=`strong_sell`+`sell`；持有=`hold`；组顺序固定（买→卖→持） |
| R3 | 组内按置信度降序 | `Confidence` desc，稳定排序 |
| R4 | 含中文名时列对齐 | CJK 字符按显示宽度记 2，等宽代码块内对齐 |
| R5 | `batch_notify:false` 完全回退逐条即时发 | 非批量路径保持原语义，有测试 |
| R6 | 执行/冷却/信号存储语义不变 | 路由决策/冷却/执行仍逐信号 |
| R7 | 空轮不发消息 | `formatBatch(nil)==""`；空 flush 不触发 `SendBatch` |
| R8 | 默认开启 batch_notify | `router.batch_notify` 默认 `true` |

## 非功能 / 约束

- Go 1.24 标准库，**无新依赖**（不引第三方 runewidth）。
- 不改 email/webhook 批量格式；不做跨轮聚合 / 自定义列（范围边界）。
- 全部测试离线（httptest / 纯函数），变更包覆盖率 ≥ 80%。

## 风险评估

整体 **LOW**：计划已逐行给出实现代码，且已对照当前代码核验（SendBatch/sendMessage/NotifyAllBatch/Route/RouterConfig/runAnalysisCycle、nil-logger 安全均确认无误）。唯一需注意点：`batch_notify` 默认 true 会改变现网默认行为（从逐条变汇总），属预期变更，已在 R8/部署验证覆盖。
