# 设计规格 — IC/IR 评估管线

> 唯一权威实现依据为需求文档 `docs/superpowers/plans/2026-06-22-ic-ir-eval.md`（含逐步代码）。
> 本文仅做接口与契约的汇总索引，不重复代码。

## 架构
与现有事件研究管线（event_study.py）平行的第二条评估腿。纯 pandas 计算核心 + baseline 因子（惰性 qlib）
+ CLI 入口 + 报告渲染（report.py 增量）。CSV（长格式 `date,symbol,score`）为唯一跨语言契约。

## 模块接口契约
| 模块 | 函数 | 签名要点 |
|---|---|---|
| ic.py | `HORIZONS` | 常量 `(5,20,60)` |
| ic.py | `forward_returns` | `(prices: dict[str,DataFrame], horizons=HORIZONS, max_defer=5) -> DataFrame[date,symbol,horizon,ret]` |
| ic.py | `instrument_ic` | `(scores, fwd, symbol, h, method="spearman", min_periods=60) -> dict\|None`；键 ic/n_periods/t_stat/t_stat_nonoverlap |
| ic.py | `ic_summary_by_instrument` | `(scores, fwd, h, method, min_periods) -> DataFrame[symbol,ic,n_periods,t_stat,t_stat_nonoverlap]` |
| ic.py | `watchlist_summary` | `(per_inst) -> dict[mean_ic,median_ic,icir,positive_breadth,n_instruments]` |
| baseline.py | `reversal_scores` | `(prices, lookback=5) -> DataFrame[date,symbol,score]`，score=-(close_t/close_{t-lookback}-1) |
| baseline.py | `oracle_scores` | `(fwd, horizon) -> DataFrame[date,symbol,score]`，score=ret@horizon |
| baseline.py | `load_prices` | `(qlib_dir, symbols, start, end, region="cn") -> dict[str,DataFrame]`（惰性 qlib） |
| report.py | `read_scores` | `(path) -> DataFrame`，表头恰为 [date,symbol,score]，坏行 1-based 行号 ValueError |
| report.py | `render_ic_report` | `(per_horizon: dict[int,{by_instrument,summary}], meta) -> str`（markdown） |
| ic_evaluate.py | `collect_ic` | `(scores, source, horizons, method, min_periods) -> (per_horizon, stats)` |
| ic_evaluate.py | `main` | `(argv=None) -> int` |

## 数据契约
- scores.csv：长格式 `date,symbol,score`，等于方向② sidecar 输出格式。
- 报告输出：`reports/signal-ic-YYYYMMDD.md`。

## 验收口径（读数告诫）
- t-stat 用重叠前向收益，偏乐观；以并列 t_stat_nonoverlap 为审慎旁证。
- watchlist 仅十来个标的，跨标的 ICIR/广度是小样本，作参考非硬门槛。
- oracle baseline 应得 IC≈1，可随时自检管线未坏。
