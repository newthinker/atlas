"""事件研究计算核心。

对每个信号：次日开盘入场（复用 ``align_entry``）→ 各 horizon 的绝对收益与相对
``SH000300`` 的超额收益（sell 取规避口径）→ 按 strategy×action×置信度桶×horizon 聚合。

全部纯 pandas，无 qlib 依赖。
"""

from dataclasses import dataclass
from statistics import mean, median

import pandas as pd

from .prices import align_entry

HORIZONS = (5, 20, 60)
CONF_BUCKETS = (0.0, 0.6, 0.8)  # 累积阈值（≥bucket），非互斥区间
_SELL_ACTIONS = ("sell", "strong_sell")


@dataclass
class Signal:
    """单条待评估信号（read_signals 的一行 / 引擎导出的一条）。"""

    symbol: str
    date: pd.Timestamp
    strategy: str
    action: str
    confidence: float


@dataclass
class SignalOutcome:
    symbol: str
    date: pd.Timestamp
    strategy: str
    action: str
    confidence: float
    returns: dict[int, float | None]  # horizon → 绝对收益（越界为 None）
    excess: dict[int, float | None]  # horizon → 超额（sell 为规避口径，已取向）


def _last_le(index: pd.DatetimeIndex, when: pd.Timestamp) -> int:
    """index 中 ≤ when 的最近前值的 positional index；when 早于首行时返回 -1。

    严格用 searchsorted(side="right") - 1：调用方必须显式检查 < 0，
    绝不能让 -1 落进 ``iloc[-1]`` 静默取末行（事件研究最易写错处）。
    """
    return int(index.searchsorted(when, side="right")) - 1


def evaluate_signal(
    sig: Signal, prices: pd.DataFrame, bench: pd.DataFrame, max_defer: int = 5
) -> SignalOutcome | None:
    """评估单信号。返回 None = 无可用结果（调用方计入 dropped / 数据缺口）：

    - ``align_entry`` 无入场 bar（无次日 bar / 顺延过久）；
    - 入场日早于基准首行（对齐索引 -1）→ 显式 None 计入数据缺口，严防负索引取末行。

    各 horizon h：exit 行 = 入场行 + h（positional），越界 → returns[h]/excess[h]=None。
    基准对齐：起点 = bench 中 ≤ entry_date 的最近前值；终点 = bench 中 ≤ exit_date 的最近前值。
    sell/strong_sell 的 excess 取规避口径 ``-(ret - bench_ret)``。
    """
    entry = align_entry(prices, sig.date, max_defer)
    if entry is None:
        return None

    start_pos = _last_le(bench.index, entry.date)
    if start_pos < 0:
        return None  # 入场早于基准首行 → 数据缺口
    bench_start = float(bench.iloc[start_pos]["close"])

    is_sell = sig.action in _SELL_ACTIONS
    n_bars = len(prices.index)
    returns: dict[int, float | None] = {}
    excess: dict[int, float | None] = {}
    for h in HORIZONS:
        exit_idx = entry.index + h
        if exit_idx >= n_bars:
            returns[h] = None
            excess[h] = None
            continue
        ret = float(prices.iloc[exit_idx]["close"]) / entry.price - 1.0
        returns[h] = ret

        end_pos = _last_le(bench.index, prices.index[exit_idx])
        if end_pos < 0:
            excess[h] = None
            continue
        bench_ret = float(bench.iloc[end_pos]["close"]) / bench_start - 1.0
        raw = ret - bench_ret
        excess[h] = -raw if is_sell else raw

    return SignalOutcome(
        symbol=sig.symbol,
        date=sig.date,
        strategy=sig.strategy,
        action=sig.action,
        confidence=sig.confidence,
        returns=returns,
        excess=excess,
    )


def aggregate(outcomes: list[SignalOutcome]) -> pd.DataFrame:
    """按 (strategy, action, conf_bucket, horizon) 聚合。

    桶为累积阈值：一条 outcome 计入所有 ``confidence >= bucket`` 的桶。
    某 horizon 的 None（越界）被跳过，不污染该格的统计（n 只数有效样本）。
    胜率 = excess > 0 的占比（excess 已对 sell 取规避向，故 buy/sell 同口径）。

    列：strategy, action, conf_bucket, horizon, n, mean_ret, median_ret,
    mean_excess, win_rate。
    """
    # cell key → {"ret": [...], "excess": [...]}
    cells: dict[tuple[str, str, float, int], dict[str, list[float]]] = {}
    for o in outcomes:
        for bucket in CONF_BUCKETS:
            if o.confidence < bucket:
                continue
            for h in HORIZONS:
                ret = o.returns.get(h)
                exc = o.excess.get(h)
                if ret is None or exc is None:
                    continue
                cell = cells.setdefault(
                    (o.strategy, o.action, bucket, h), {"ret": [], "excess": []}
                )
                cell["ret"].append(ret)
                cell["excess"].append(exc)

    rows = []
    for (strategy, action, bucket, horizon), vals in cells.items():
        rets = vals["ret"]
        excs = vals["excess"]
        n = len(rets)
        rows.append(
            {
                "strategy": strategy,
                "action": action,
                "conf_bucket": bucket,
                "horizon": horizon,
                "n": n,
                "mean_ret": mean(rets),
                "median_ret": median(rets),
                "mean_excess": mean(excs),
                "win_rate": sum(1 for e in excs if e > 0) / n,
            }
        )

    columns = [
        "strategy", "action", "conf_bucket", "horizon",
        "n", "mean_ret", "median_ret", "mean_excess", "win_rate",
    ]
    df = pd.DataFrame(rows, columns=columns)
    return df.sort_values(
        ["strategy", "action", "conf_bucket", "horizon"]
    ).reset_index(drop=True)
