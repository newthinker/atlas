# 终验收报告 — digest「PE%」列

> 分支 feature/digest-pe-percentile-column。autonomy=dod-gate / scheduling=dag。
> 降级：ECC·codex·gemini·Go validator·write-hook 缺失（沿用上 sprint 降级路径）。

## 结论：✅ 全部 accepted，可交付

## 完成任务
| 任务 | 标题 | commit | 验收 |
|------|------|--------|------|
| TASK-001 | renderTable 加 PE% 列 | 7889c8b | renderTable 100%，telegram 整包 90.5% |
| TASK-002 | enrichSignalMetadata 盖 pe_percentile_display | cebea8a | enrichSignalMetadata 100%，6 包零回归 |

零返工（rework_count 全 0）。

## 质量门
- **独立评审**（只读需求）补强 4 个测试缺口，全部落地通过：
  - **B4** PEPercentile==0.0（历史最低 PE，合法买点）必须显示 0.0%，不被 >=0 误吞
  - **B6** name 空+PE 有 → 只盖 PE 不盖 name（presence-check 断言）
  - **F5** formatBatch 端到端（分组+排序后）PE% 列
  - **N1** router 对照回归：带/不带 pe_percentile_display，Route 决策一致
- **QA 两轮**（常规 + 跨视角对抗，cross-model 降级纯 Claude）：verdict=PASS，无 CRITICAL、无 WARNING，仅 2 条 INFO（非问题）。
- 终验：go build ./... 通过；全仓 -race 无数据竞争。

## DoD 达成
- ✅ digest 末列 PE%，有 PE 行显示 xx.x%，无 PE 行（ETF/金融指数/nil）留空，CJK 对齐、末列无尾随空格
- ✅ 每个有 PE 的行都填（跨策略，从 Fundamental.PEPercentile）
- ✅ 同标的两条信号显示相同 PE
- ✅ router 门控零影响（展示专用键 pe_percentile_display）
- ✅ go build 通过；app/telegram/router 包测试全绿

## 关键设计
- 展示专用键 pe_percentile_display（router 不读）；ADR 见 01-design/architecture-decisions.md
- ROE 列暂缓（无数据源）

## 部署验证（实现后）
```
bash scripts/ops/deploy.sh && bash scripts/ops/services.sh restart
bash scripts/ops/services.sh analysis-now
# 预期：Telegram digest 表格末列出现 PE%，有 PE 的标的显示历史百分位、ETF/金融指数留空
```
