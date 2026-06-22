"""Context Checkpoint: done_criteria -> test mapping (TASK-001)
functional[1] "next-open 前向收益 close_{t+h}/open_{t+1}-1；长格式列恰 [date,symbol,horizon,ret]"
              -> test_forward_returns_next_open + test_forward_returns_columns_exact
functional[2] "HORIZONS == (5,20,60)"                          -> test_horizons_constant
boundary[3]   "horizon 越界(entry_idx+h>=n_bars) 不产行"        -> test_forward_returns_horizon_out_of_range
boundary[4]   "align_entry 返回 None(无次日 bar) 时该 score 日不产行"
              -> test_forward_returns_no_next_bar_drops
boundary[5]   "[gap1 前视判别] ret 由 open_{t+1} 入场算，非 close_t" -> test_forward_returns_no_lookahead_open_entry
boundary[6]   "[gap2 停牌/max_defer] 次日停牌在 max_defer 内延迟入场；耗尽不产行"
              -> test_forward_returns_defers_entry_when_next_bar_suspended
                 + test_forward_returns_drops_when_defer_exhausted
non_functional[7] "ic.py 顶层不得 import qlib（纯 pandas 守门）"   -> test_no_qlib_at_module_level
"""

import math

import pandas as pd

from qlib_eval.ic import HORIZONS, forward_returns


def price_frame(dates, opens, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    return pd.DataFrame({"open": opens, "close": closes}, index=idx)


# 7 连续交易日。score 日 index0 → 入场 index1 开盘 10.0 → horizon 5 出场 index6 close 12.0。
DATES = ["2024-01-01", "2024-01-02", "2024-01-03", "2024-01-04",
         "2024-01-05", "2024-01-08", "2024-01-09"]


def test_forward_returns_next_open():
    # functional[1]: 入场 index1 开盘=10.0；index6 close=12.0 → ret = 12/10 - 1 = +0.2
    df = price_frame(DATES, opens=[9, 10, 11, 11, 11, 11, 11],
                     closes=[9, 10, 11, 11, 11, 11, 12])
    fwd = forward_returns({"AAA": df}, horizons=(5,))
    row = fwd[(fwd["symbol"] == "AAA")
              & (fwd["date"] == pd.Timestamp("2024-01-01"))
              & (fwd["horizon"] == 5)].iloc[0]
    assert math.isclose(row["ret"], 0.2, rel_tol=1e-9)


def test_forward_returns_columns_exact():
    # functional[1]: 长格式列恰为 [date, symbol, horizon, ret]（顺序与集合都钉死）
    df = price_frame(DATES, opens=[9, 10, 11, 11, 11, 11, 11],
                     closes=[9, 10, 11, 11, 11, 11, 12])
    fwd = forward_returns({"AAA": df}, horizons=(5,))
    assert list(fwd.columns) == ["date", "symbol", "horizon", "ret"]


def test_horizons_constant():
    # functional[2]
    assert HORIZONS == (5, 20, 60)


def test_forward_returns_horizon_out_of_range():
    # boundary[3]: 从 index2 起 horizon 5 越界（入场 index3 + 5 = 8 >= 7）→ 不产行
    df = price_frame(DATES, opens=[9, 10, 11, 11, 11, 11, 11],
                     closes=[9, 10, 11, 11, 11, 11, 12])
    fwd = forward_returns({"AAA": df}, horizons=(5,))
    assert fwd[(fwd["date"] == pd.Timestamp("2024-01-03"))
               & (fwd["horizon"] == 5)].empty


def test_forward_returns_no_next_bar_drops():
    # boundary[4]: 最后一根 bar 作为 score 日 → align_entry 无严格次日 bar → None → 不产任何行。
    df = price_frame(DATES, opens=[9, 10, 11, 11, 11, 11, 11],
                     closes=[9, 10, 11, 11, 11, 11, 12])
    fwd = forward_returns({"AAA": df}, horizons=(5,))
    # 最后一日 2024-01-09 无次日 bar → 该 score 日不出现在任何行
    assert fwd[fwd["date"] == pd.Timestamp("2024-01-09")].empty


def test_forward_returns_no_lookahead_open_entry():
    # boundary[5] [强化 gap1 前视判别]:
    # 用 open != close 的合成帧，钉死 ret 由 open_{t+1}（入场日开盘）算出，而非 close_t。
    # score 日 index0=2024-01-01；入场 index1 open=20.0，close=99.0（故意拉大区别）；
    # horizon 1 出场 index2 close=24.0。
    #   正确（next-open 入场）: ret = close[idx2]/open[idx1] - 1 = 24/20 - 1 = +0.20
    #   若错用 close_t（=score 日 index0 close=50.0）入场: 24/50 - 1 = -0.52（截然不同）
    #   若错用 close_{t+1}（入场日 close=99.0）: 24/99 - 1 = -0.758（亦不同）
    # 三个候选值互不相等 → 任何前视/口径错误都会被这条断言捕获。
    dates = ["2024-01-01", "2024-01-02", "2024-01-03"]
    df = price_frame(dates, opens=[5.0, 20.0, 7.0], closes=[50.0, 99.0, 24.0])
    fwd = forward_returns({"AAA": df}, horizons=(1,))
    row = fwd[(fwd["date"] == pd.Timestamp("2024-01-01"))
              & (fwd["horizon"] == 1)].iloc[0]
    correct = 24.0 / 20.0 - 1.0          # next-open 入场（无前视）
    wrong_close_t = 24.0 / 50.0 - 1.0    # 错用 score 日 close 入场（前视/口径错）
    wrong_close_tp1 = 24.0 / 99.0 - 1.0  # 错用入场日 close 入场
    assert math.isclose(row["ret"], correct, rel_tol=1e-9)
    assert not math.isclose(row["ret"], wrong_close_t, rel_tol=1e-9)
    assert not math.isclose(row["ret"], wrong_close_tp1, rel_tol=1e-9)


def test_forward_returns_defers_entry_when_next_bar_suspended():
    # boundary[6] [强化 gap2 停牌/max_defer] —— 延迟入场分支:
    # align_entry 取 signal_date 之后严格首个【存在于帧中】的 bar 当入场（searchsorted side=right）；
    # 缺 bar = 停牌。若与首个可交易 bar 间隔 <= max_defer*2 个日历日则保留（延迟入场）。
    # score 日 2024-01-05；次日 2024-01-06 起停牌（帧中缺失）；下一可交易 bar = 2024-01-13，
    # 间隔 = 8 个日历日 <= max_defer(5)*2=10 → align_entry 延迟入场到 2024-01-13 开盘。
    # horizon 1 出场 = 入场行 + 1 = 2024-01-14 close。断言该 score 日确实产了行，
    # 且 ret 由延迟入场日 2024-01-13 的 open 算出。
    dates = ["2024-01-05", "2024-01-13", "2024-01-14"]
    df = price_frame(dates, opens=[10.0, 30.0, 99.0], closes=[10.0, 31.0, 36.0])
    fwd = forward_returns({"AAA": df}, horizons=(1,), max_defer=5)
    rows = fwd[(fwd["date"] == pd.Timestamp("2024-01-05"))
               & (fwd["horizon"] == 1)]
    assert len(rows) == 1
    # 延迟入场开盘 = 2024-01-13 open = 30.0；出场 2024-01-14 close = 36.0 → 36/30 - 1 = +0.2
    assert math.isclose(rows.iloc[0]["ret"], 36.0 / 30.0 - 1.0, rel_tol=1e-9)


def test_forward_returns_drops_when_defer_exhausted():
    # boundary[6] [强化 gap2 停牌/max_defer] —— max_defer 耗尽分支:
    # score 日 2024-01-05；下一可交易 bar = 2024-01-20，间隔 = 15 日历日 > max_defer(5)*2=10
    # → align_entry 返回 None（停牌过久，丢弃）→ 该 score 日不产任何行。
    dates = ["2024-01-05", "2024-01-20", "2024-01-21"]
    df = price_frame(dates, opens=[10.0, 30.0, 99.0], closes=[10.0, 31.0, 36.0])
    fwd = forward_returns({"AAA": df}, horizons=(1,), max_defer=5)
    assert fwd[fwd["date"] == pd.Timestamp("2024-01-05")].empty


def test_no_qlib_at_module_level():
    # non_functional[7]: ic.py 纯 pandas，顶层不得 import qlib（守门）。
    import sys

    import qlib_eval.ic  # noqa: F401

    assert "qlib" not in sys.modules
