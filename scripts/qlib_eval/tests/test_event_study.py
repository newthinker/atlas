"""Context Checkpoint: done_criteria -> test mapping
functional[0] "horizon 收益+超额 +10%/基准+2%→+8%"        -> test_horizon_return_and_excess
functional[1] "sell 规避 -(ret-bench) → +12%"              -> test_sell_avoidance_return
functional[2] "aggregate strategy×action n/mean/median/win_rate" -> test_aggregate_by_strategy_action
functional[3] "置信度三桶累积 n=3/2/1"                     -> test_confidence_buckets_are_cumulative
functional[4] "基准最近前值对齐（最易写错处）"            -> test_benchmark_aligns_to_last_available_before_entry
boundary[0]   "horizon 越界 → None 不污染聚合"             -> test_horizon_exceeds_data_returns_none
boundary[1]   "entry 早于 bench 首行 → 显式 None（防 -1 取末行）" -> test_entry_before_benchmark_returns_none

合成数据全部手工可验算（见各用例注释）。
"""

import pandas as pd
import pytest

from qlib_eval.event_study import (
    HORIZONS,
    Signal,
    SignalOutcome,
    aggregate,
    evaluate_signal,
)


def price_frame(dates, opens, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    return pd.DataFrame({"open": opens, "close": closes}, index=idx)


def bench_frame(dates, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    # 基准只用 close；带个 open 列以贴近真实形状
    return pd.DataFrame({"open": closes, "close": closes}, index=idx)


# 7 个连续交易日，入场在 index 1（次日开盘），horizon 5 的 exit 落在 index 6。
PRICE_DATES = [
    "2024-01-01", "2024-01-02", "2024-01-03", "2024-01-04",
    "2024-01-05", "2024-01-08", "2024-01-09",
]


def test_horizon_return_and_excess():
    # 标的：入场 index1 开盘 10.0，5 日后 index6 close 11.0 → 绝对收益 +10%
    # 基准：entry 日 3000 → exit 日 3060 → +2%；buy 超额 = +10% - +2% = +8%
    prices = price_frame(
        PRICE_DATES,
        opens=[9.0, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0],
        closes=[9.0, 10.0, 10.0, 10.0, 10.0, 11.0, 11.0],
    )
    bench = bench_frame(PRICE_DATES, closes=[3000, 3000, 3000, 3000, 3000, 3060, 3060])
    sig = Signal(symbol="600519.SH", date=pd.Timestamp("2024-01-01"),
                 strategy="ma", action="buy", confidence=0.9)
    out = evaluate_signal(sig, prices, bench, max_defer=5)
    assert out is not None
    assert out.returns[5] == pytest.approx(0.10)
    assert out.excess[5] == pytest.approx(0.08)


def test_sell_avoidance_return():
    # 标的 -10%（10.0 → 9.0）、基准 +2% → raw = -0.10 - 0.02 = -0.12
    # sell 规避收益 = -raw = +0.12（信号后跑输基准记为正）
    prices = price_frame(
        PRICE_DATES,
        opens=[9.0, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0],
        closes=[9.0, 10.0, 10.0, 10.0, 10.0, 9.0, 9.0],
    )
    bench = bench_frame(PRICE_DATES, closes=[3000, 3000, 3000, 3000, 3000, 3060, 3060])
    sig = Signal(symbol="600519.SH", date=pd.Timestamp("2024-01-01"),
                 strategy="ma", action="sell", confidence=0.9)
    out = evaluate_signal(sig, prices, bench, max_defer=5)
    assert out.returns[5] == pytest.approx(-0.10)
    assert out.excess[5] == pytest.approx(0.12)


def test_horizon_exceeds_data_returns_none():
    # 只有 7 根 bar：horizon 5 可算（index6），horizon 20/60 越界 → 该 horizon None
    # 整体仍返回有效 SignalOutcome（不污染聚合，由 aggregate 跳过 None）。
    prices = price_frame(
        PRICE_DATES,
        opens=[9.0, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0],
        closes=[9.0, 10.0, 10.0, 10.0, 10.0, 11.0, 11.0],
    )
    bench = bench_frame(PRICE_DATES, closes=[3000, 3000, 3000, 3000, 3000, 3060, 3060])
    sig = Signal(symbol="600519.SH", date=pd.Timestamp("2024-01-01"),
                 strategy="ma", action="buy", confidence=0.9)
    out = evaluate_signal(sig, prices, bench, max_defer=5)
    assert out is not None
    assert out.returns[5] == pytest.approx(0.10)
    assert out.returns[20] is None and out.excess[20] is None
    assert out.returns[60] is None and out.excess[60] is None


def test_benchmark_aligns_to_last_available_before_entry():
    # 基准缺 entry 日（最易写错处）：bench [1/2,1/4,1/8] close [3000,3030,3060]。
    # 标的入场 1/3（bench 缺）→ 起点取 ≤1/3 的最近前值 = 1/2 的 3000（不是 1/4 的 3030！）。
    # 标的 exit 日 1/8 → 终点取 ≤1/8 最近前值 = 1/8 的 3060 → 基准收益 +2%。
    # 标的 +10% → buy 超额 = +8%。若误取 1/4=3030 作起点，excess≈+9.01%，本用例翻车。
    prices = price_frame(
        ["2024-01-02", "2024-01-03", "2024-01-04", "2024-01-05",
         "2024-01-06", "2024-01-08", "2024-01-09"],
        opens=[9.0, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0],
        closes=[9.0, 10.0, 10.0, 10.0, 10.0, 11.0, 11.0],
    )
    bench = bench_frame(["2024-01-02", "2024-01-04", "2024-01-08"],
                        closes=[3000, 3030, 3060])
    sig = Signal(symbol="600519.SH", date=pd.Timestamp("2024-01-02"),
                 strategy="ma", action="buy", confidence=0.9)
    # entry index1=1/3，exit index 1+5=6=1/9 close 11.0 → ret +10%；
    # bench 起点 1/2=3000、终点 ≤1/9 最近前值 1/8=3060 → 基准 +2% → excess +8%
    out = evaluate_signal(sig, prices, bench, max_defer=5)
    assert out is not None
    assert out.excess[5] == pytest.approx(0.08)


def test_entry_before_benchmark_returns_none():
    # boundary[1]：entry 日早于 bench 首行 → 对齐索引 -1 → 显式 None 计入数据缺口。
    # 严防 Python 负索引 bench.iloc[-1] 静默取末行（会算出离谱基准收益）。
    prices = price_frame(
        PRICE_DATES,
        opens=[9.0, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0],
        closes=[9.0, 10.0, 10.0, 10.0, 10.0, 11.0, 11.0],
    )
    bench = bench_frame(["2024-02-01", "2024-02-02"], closes=[3000, 3060])
    sig = Signal(symbol="600519.SH", date=pd.Timestamp("2024-01-01"),
                 strategy="ma", action="buy", confidence=0.9)
    assert evaluate_signal(sig, prices, bench, max_defer=5) is None


def _outcome(strategy, action, confidence, excess5, ret5=0.0):
    return SignalOutcome(
        symbol="X", date=pd.Timestamp("2024-01-01"), strategy=strategy,
        action=action, confidence=confidence,
        returns={5: ret5, 20: None, 60: None},
        excess={5: excess5, 20: None, 60: None},
    )


def _row(agg, strategy, action, conf_bucket, horizon):
    m = (
        (agg["strategy"] == strategy)
        & (agg["action"] == action)
        & (agg["conf_bucket"] == conf_bucket)
        & (agg["horizon"] == horizon)
    )
    return agg[m]


def test_aggregate_by_strategy_action():
    # 4 条 buy（超额 +5/+1/-2/-4%）→ n=4, mean_excess=0.0%, win_rate=50%（超额>0 占比 2/4）
    # 2 条 sell（规避 +3/-1%）→ n=2, win_rate=50%（规避>0 占比 1/2）
    outcomes = [
        _outcome("s", "buy", 0.9, 0.05),
        _outcome("s", "buy", 0.9, 0.01),
        _outcome("s", "buy", 0.9, -0.02),
        _outcome("s", "buy", 0.9, -0.04),
        _outcome("s", "sell", 0.9, 0.03),
        _outcome("s", "sell", 0.9, -0.01),
    ]
    agg = aggregate(outcomes)
    buy = _row(agg, "s", "buy", 0.0, 5)
    assert int(buy["n"].iloc[0]) == 4
    assert buy["mean_excess"].iloc[0] == pytest.approx(0.0, abs=1e-9)
    assert buy["win_rate"].iloc[0] == pytest.approx(0.5)
    sell = _row(agg, "s", "sell", 0.0, 5)
    assert int(sell["n"].iloc[0]) == 2
    assert sell["win_rate"].iloc[0] == pytest.approx(0.5)


def test_confidence_buckets_are_cumulative():
    # conf 0.5/0.65/0.85 → 累积桶 ≥0.0 / ≥0.6 / ≥0.8 的 n 分别为 3/2/1
    outcomes = [
        _outcome("s", "buy", 0.5, 0.05),
        _outcome("s", "buy", 0.65, 0.05),
        _outcome("s", "buy", 0.85, 0.05),
    ]
    agg = aggregate(outcomes)
    assert int(_row(agg, "s", "buy", 0.0, 5)["n"].iloc[0]) == 3
    assert int(_row(agg, "s", "buy", 0.6, 5)["n"].iloc[0]) == 2
    assert int(_row(agg, "s", "buy", 0.8, 5)["n"].iloc[0]) == 1


def test_constants_and_horizons():
    assert HORIZONS == (5, 20, 60)
