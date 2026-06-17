# 需求分析 — digest「PE%」列

> 来源：`docs/superpowers/plans/2026-06-17-telegram-digest-pe-percentile-column.md`
> spec：`docs/superpowers/specs/2026-06-17-telegram-digest-pe-percentile-column-design.md`

## 目标
Telegram 信号 digest 表格末尾增加 `PE%` 列，显示每个标的当前市盈率在其历史 PE 中的百分位（0–100%）。

## 需求
| # | 需求 | 验收要点 |
|---|------|---------|
| R1 | digest 末列出现 `PE%` | renderTable 表头加 `PE%`，列顺序在末尾 |
| R2 | 每个有 PE 的行都填，跨策略 | enrichSignalMetadata 给每条信号盖 `Fundamental.PEPercentile` |
| R3 | 无 PE 的行留空 | ETF/金融指数/nil Fundamental/PEPercentile<0 → 该格空，表格不破 |
| R4 | 不影响 router 门控 | 用展示专用键 `pe_percentile_display`，router 的 percentileOf 不读它 |
| R5 | CJK 对齐 / 末列无尾随空格不变 | 复用 width.go；末列规则保留 |

## 范围外
ROE 列（无数据源）、PE 原始数值列、同标的去重、router/email/webhook 改动。

## 风险：LOW
spec/plan 已逐行给出代码并对照现状核验；唯一注意点是 enrichSignalMetadata 去掉 `name==""` 提前返回（因可能无 name 但有 PE），已在 plan 处理。
