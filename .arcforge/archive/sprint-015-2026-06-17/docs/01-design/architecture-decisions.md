# 架构决策 — digest「PE%」列

## ADR-1：用展示专用键 pe_percentile_display，不复用 pe_percentile
- **决策**：新键 `pe_percentile_display`，而非复用 `Metadata["pe_percentile"]`。
- **理由**：router.percentileOf 按序读 `percentile`/`pe_percentile` 做百分位 step 门控；复用会有污染门控的风险。新键 router 永不读 → 门控零影响、解耦清晰。

## ADR-2：在 app 层富化（enrichSignalMetadata），不在策略层
- **决策**：从 `Fundamental.PEPercentile`（每标的算一次的权威值）在 enrichSignalMetadata 盖到每条信号。
- **理由**：让 PE% 列对**所有有 PE 的标的**填充，而非仅 pe_percentile 策略的信号；与 pe_percentile 策略展示值同源、自洽。

## ADR-3：ROE 列暂缓
- 无数据源（lixinger non_financial 不返回 ROE，Yahoo/qlibpit 仅 EPS），需新数据管道。本 sprint 只做 PE%。

## ADR-4：表头文案 PE%（用户指定）
- 语义为「PE 历史百分位」，非市盈率数值。值用 `%.1f%%` 渲染。

## 降级
ECC/codex/gemini/Go validator/write-hook 缺失 → 沿用上 sprint：人工 validator、QA 跨视角纯 Claude、Leader 单写者管理状态。
