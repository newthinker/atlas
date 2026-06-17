# 需求 ↔ DoD 追溯矩阵 — 第三期 全历史回看

## 正向：需求 → DoD（无孤儿）

| 需求条目 | 任务 | 关键 DoD |
|---|---|---|
| SinceInceptionBars 哨兵 | T1 | 常量 >= 100*252 + 文档 |
| price_percentile 接受 lookback:0 | T2 | RequiredData=SinceInceptionBars + Init 接受 0 拒负 + Reason full history |
| pe_percentile 接受 lookback:0 | T3 | 同 T2 对称 |
| 既有 N-year 零回归 | T2,T3 | lookback>0 时 PriceHistory=N*252 |
| valuation lookback 可配 | T4 | ValuationConfig.LookbackYears + 默认 5 |
| app PE lookback 常量→字段 | T5 | valuationLookback 字段默认 5 + setter |
| inception EPS floor | T5 | 0 时 EPS start≈100年前 |
| lixinger y10 上限 | T5 | lixingerLookback() 0→10 |
| 默认 5 零回归 | T5,T6 | 删 const 后默认 5 行为一致 |
| serve 装配 | T6 | SetValuationLookback 在 Start 前 |
| 全史 dump | T7 | WAREHOUSE_FROM=1970 + 手工验证 bar>1260 |
| config 示例 + lixinger 上限文档 | T7 | config.example + ADAPTERS Lookback modes |

## 反向：DoD → 需求（无凭空）
逐条核对 T1-T7 done_criteria 均对应计划 Task 的 Step/DoD 或诚实边界（lixinger y10）。无凭空。

## 机器检查
- 孤儿需求：0 ｜ 凭空 DoD：0
- best-effort 范围（全史数据实际生产、A/HK 全史 dump、lixinger 上限）已标注，T7 用 verify_by:manual。
