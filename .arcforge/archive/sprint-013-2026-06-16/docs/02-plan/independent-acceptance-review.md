# 独立验收标准反审报告 — 第三期 全历史回看

> reviewer：独立 agent，仅读需求文档，不参考已生成 DoD。

## 总体判断
验收标准空间**充分、可测试、边界基本齐全**。计划 DoD 已覆盖主干，reviewer 指出对几个高风险假设的「钉死」颗粒度可加强。

## 最易遗漏项（reviewer 提出 → Leader 处置）

| # | 缺口 | 处置 |
|---|---|---|
| N2 | inception 下 EPS 取 100 年 floor，但 PE 分位窗口应由 ohlcv 决定——「EPS 多取无害」仅写在注释，无断言钉死。若 ReconstructPEPercentile 实际把 EPS 窗口纳入分位，inception 分位会被悄悄拉偏，且编译/5年回归都发现不了 | **已采纳**（最高价值）：T5 增 boundary「固定 ohlcv 下，含早期 floor 的长 EPS 与紧凑 EPS 返回相同分位」(TestReconstructPercentileUnaffectedByEPSOverfetch) |
| B3/B4 | minSampleBars=252 兜底（新股<1年不出信号）+ 逐标的上市日，是 inception 语义命门，但 T2/T3 只测 RequiredData/Reason，无针对性测试 | **B3 已采纳**：T2/T3 各增 boundary「inception 下 <252 bars 仍不出信号」；**B4**（逐标的窗口）属 app/FetchHistory 集成行为，由 T7 手工验证（bar 数）覆盖 |
| B5 | lixinger inception 是否越界请求 >10 年 | **已覆盖/措辞强化**：A 股/指数 PE 走 lixinger（不用 ohlcv 窗口），lixingerLookback() 封顶 10 已杜绝越界；T5 criterion 明确「永不超过 10」 |

## 次要项
- F12/N5（端到端实跑、数据 best-effort）已与单测分离：T7 用 verify_by:manual，不污染 CI 绿灯。
- N1 零回归（默认 5）、N3 lixinger 10Y 诚实标注、N4 默认 SIGNAL_FROM 不变 均已在 T5/T6/T7 DoD 覆盖。
