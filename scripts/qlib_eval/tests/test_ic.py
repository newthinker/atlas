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


# ===========================================================================
# TASK-002: instrument_ic — done_criteria -> test mapping
#   functional[1]  "完全同序 → Spearman IC=1.0, n_periods=5, t_stat=sqrt(5)"
#                  -> test_instrument_ic_perfect_positive
#   functional[2]  "[强化 符号正确性] 反相关→IC≈-1; 随机→IC≈0; 常数→不漂移"
#                  -> test_instrument_ic_perfect_negative
#                   + test_instrument_ic_uncorrelated_near_zero
#   functional[3]  "[强化 gap3 重叠虚高校正] nonoverlap 每 h 行采样, n_no 步长正确,
#                   且与重叠 t_stat 数值不同"
#                  -> test_instrument_ic_nonoverlap_sampling_and_differs
#   boundary[4]    "有效配对 < min_periods → None"
#                  -> test_instrument_ic_below_min_periods_returns_none
#   boundary[5]    "非重叠样本 < 2 → t_stat_nonoverlap=None"
#                  -> test_instrument_ic_nonoverlap_lt2_returns_none
#   error[6]       "ic 为 NaN 时 ic/t_stat 字段返回 None 而非抛异常"
#                  -> test_instrument_ic_nan_returns_none_not_raise
#   non_functional[7] "pearson 与 spearman 两路径都实测, 非线性单调序列上结果不同"
#                  -> test_instrument_ic_pearson_vs_spearman_differ
# ===========================================================================

from qlib_eval.ic import instrument_ic  # noqa: E402


def _scores(dates, vals, symbol="AAA"):
    return pd.DataFrame(
        {"date": pd.to_datetime(dates), "symbol": symbol, "score": vals}
    )


def _fwd(dates, rets, symbol="AAA", h=5):
    return pd.DataFrame(
        {"date": pd.to_datetime(dates), "symbol": symbol, "horizon": h, "ret": rets}
    )


def _seq_dates(n):
    """n 个连续 Timestamp（用 2024 起，跨月避免日历对齐分歧）。"""
    return list(pd.date_range("2024-01-01", periods=n, freq="D").strftime("%Y-%m-%d"))


def test_instrument_ic_perfect_positive():
    # functional[1]: 分数与前向收益完全同序 → Spearman IC=1.0；t_stat=1.0*sqrt(5)
    ds = _seq_dates(5)
    res = instrument_ic(
        _scores(ds, [1, 2, 3, 4, 5]),
        _fwd(ds, [0.1, 0.2, 0.3, 0.4, 0.5]),
        "AAA", h=5, min_periods=3,
    )
    assert res is not None
    assert math.isclose(res["ic"], 1.0, rel_tol=1e-9)
    assert res["n_periods"] == 5
    assert math.isclose(res["t_stat"], math.sqrt(5), rel_tol=1e-9)


def test_instrument_ic_perfect_negative():
    # functional[2] 符号正确性: 完全反相关 → Spearman IC = -1.0; t_stat 同号(负)
    ds = _seq_dates(5)
    res = instrument_ic(
        _scores(ds, [1, 2, 3, 4, 5]),
        _fwd(ds, [0.5, 0.4, 0.3, 0.2, 0.1]),
        "AAA", h=5, min_periods=3,
    )
    assert res is not None
    assert math.isclose(res["ic"], -1.0, rel_tol=1e-9)
    assert math.isclose(res["t_stat"], -math.sqrt(5), rel_tol=1e-9)


def test_instrument_ic_uncorrelated_near_zero():
    # functional[2] 符号正确性: score 与 ret 排名无单调关系 → Spearman IC = 0（不漂移成假信号）。
    # score 秩 = [1,2,3,4,5]; ret=[0.2,0.5,0.3,0.1,0.4] → ret 秩 = [2,5,3,1,4]。
    # d = [-1,-3,0,3,1]; sum d^2 = 20; rho = 1 - 6*20/(5*24) = 1 - 1.0 = 0.0（精确 0）。
    ds = _seq_dates(5)
    res = instrument_ic(
        _scores(ds, [1, 2, 3, 4, 5]),
        _fwd(ds, [0.2, 0.5, 0.3, 0.1, 0.4]),
        "AAA", h=5, min_periods=3,
    )
    assert res is not None
    assert abs(res["ic"]) < 1e-9
    assert abs(res["t_stat"]) < 1e-9


def test_instrument_ic_nonoverlap_sampling_and_differs():
    # functional[3] gap3: 足够长重叠样本。h=3, n=10 → nonoverlap iloc[::3] = idx 0,3,6,9 → n_no=4。
    # 设计 score 与 ret 让重叠 IC 与非重叠 IC 数值不同（证明非重叠旁证真的取了子集）。
    ds = _seq_dates(10)
    scores = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
    # 整体大致正相关，但被采样点 (idx 0,3,6,9) 的秩关系与整体不同。
    rets = [0.10, 0.50, 0.20, 0.40, 0.90, 0.30, 0.70, 0.60, 1.00, 0.80]
    res = instrument_ic(
        _scores(ds, scores), _fwd(ds, rets, h=3), "AAA", h=3, min_periods=3,
    )
    assert res is not None
    assert res["n_periods"] == 10
    # 手工验算非重叠子集（按 date 排序后 iloc[::3]）：
    sub_scores = [scores[i] for i in (0, 3, 6, 9)]   # [1,4,7,10]
    sub_rets = [rets[i] for i in (0, 3, 6, 9)]        # [0.10,0.40,0.70,0.80]
    expected_n_no = len(sub_scores)                   # 4
    expected_ic_no = pd.Series(sub_scores).corr(
        pd.Series(sub_rets), method="spearman"
    )
    expected_t_no = expected_ic_no * math.sqrt(expected_n_no)
    assert math.isclose(res["t_stat_nonoverlap"], expected_t_no, rel_tol=1e-9)
    # 非重叠旁证与重叠 t_stat 数值不同（证明真的非重叠，步长 h 生效）。
    assert not math.isclose(
        res["t_stat_nonoverlap"], res["t_stat"], rel_tol=1e-9
    )


def test_instrument_ic_below_min_periods_returns_none():
    # boundary[4]: 有效配对 2 < min_periods 60 → None
    ds = _seq_dates(2)
    assert instrument_ic(
        _scores(ds, [1, 2]), _fwd(ds, [0.1, 0.2]),
        "AAA", h=5, min_periods=60,
    ) is None


def test_instrument_ic_nonoverlap_lt2_returns_none():
    # boundary[5]: 非重叠样本 < 2 → t_stat_nonoverlap=None。
    # n=5, h=20 → iloc[::20] 只取 idx 0 → n_no=1 < 2 → t_stat_nonoverlap=None。
    ds = _seq_dates(5)
    res = instrument_ic(
        _scores(ds, [1, 2, 3, 4, 5]),
        _fwd(ds, [0.1, 0.2, 0.3, 0.4, 0.5], h=20),
        "AAA", h=20, min_periods=3,
    )
    assert res is not None
    assert res["t_stat_nonoverlap"] is None
    # 重叠口径仍有正常数值，不受非重叠不足影响。
    assert math.isclose(res["ic"], 1.0, rel_tol=1e-9)


def test_instrument_ic_nan_returns_none_not_raise():
    # error[6]: score 恒定 → corr 为 NaN → ic/t_stat 返回 None 而非抛异常。
    ds = _seq_dates(5)
    res = instrument_ic(
        _scores(ds, [3, 3, 3, 3, 3]),          # 常数 score → 相关无定义 (NaN)
        _fwd(ds, [0.1, 0.2, 0.3, 0.4, 0.5]),
        "AAA", h=5, min_periods=3,
    )
    assert res is not None
    assert res["ic"] is None
    assert res["t_stat"] is None
    assert res["n_periods"] == 5


def test_instrument_ic_pearson_vs_spearman_differ():
    # non_functional[7]: 非线性单调序列上 pearson 与 spearman 两路径都跑通且结果不同。
    # 完全单调（同序）→ Spearman=1.0 恒定；pearson 受非线性曲率影响 < 1.0。
    ds = _seq_dates(5)
    scores = [1, 2, 3, 4, 5]
    rets = [0.01, 0.02, 0.04, 0.08, 0.16]   # 指数增长：单调但非线性
    sp = instrument_ic(_scores(ds, scores), _fwd(ds, rets),
                       "AAA", h=5, method="spearman", min_periods=3)
    pe = instrument_ic(_scores(ds, scores), _fwd(ds, rets),
                       "AAA", h=5, method="pearson", min_periods=3)
    assert sp is not None and pe is not None
    assert math.isclose(sp["ic"], 1.0, rel_tol=1e-9)   # 秩相关对单调不敏感
    assert pe["ic"] < 1.0                                # 线性相关被非线性削弱
    assert not math.isclose(sp["ic"], pe["ic"], rel_tol=1e-9)


# ===========================================================================
# TASK-003: ic_summary_by_instrument + watchlist_summary — done_criteria -> test
#   functional[1]  "每标的一行, 列恰 [symbol,ic,n_periods,t_stat,t_stat_nonoverlap],
#                   按 symbol 排序"
#                  -> test_summary_by_instrument_columns_and_sorted
#   functional[2]  "watchlist_summary 聚合 mean=0,median=0.10,breadth=2/3,n=3 数值正确"
#                  -> test_watchlist_summary_aggregates
#   boundary[3]    "样本不足标的被剔除(BBB 仅 2<3 不出现)"
#                  -> test_summary_by_instrument_drops_thin
#   boundary[4]    "n_instruments<2 → icir=None; ics 为空 → 全字段 None, n_instruments=0"
#                  -> test_watchlist_summary_single_instrument_icir_none
#                   + test_watchlist_summary_empty_all_none
#   non_functional[5] "icir=mean/std(ddof=1); std=0 或 NaN → icir=None 不抛"
#                  -> test_watchlist_summary_icir_formula
#                   + test_watchlist_summary_zero_std_icir_none
#                   + test_watchlist_summary_nan_ic_dropped
# ===========================================================================

from qlib_eval.ic import ic_summary_by_instrument, watchlist_summary  # noqa: E402


def test_summary_by_instrument_columns_and_sorted():
    # functional[1]: 每标的一行, 列恰为指定 5 列且按 symbol 排序。
    # 故意以非排序顺序 concat (CCC 先于 AAA), 验证输出按 symbol 升序。
    ds = _seq_dates(5)
    scores = pd.concat([
        _scores(ds, [1, 2, 3, 4, 5], "CCC"),
        _scores(ds, [1, 2, 3, 4, 5], "AAA"),
    ])
    fwd = pd.concat([
        _fwd(ds, [0.5, 0.4, 0.3, 0.2, 0.1], "CCC"),
        _fwd(ds, [0.1, 0.2, 0.3, 0.4, 0.5], "AAA"),
    ])
    out = ic_summary_by_instrument(scores, fwd, h=5, min_periods=3)
    assert list(out.columns) == [
        "symbol", "ic", "n_periods", "t_stat", "t_stat_nonoverlap"
    ]
    assert list(out["symbol"]) == ["AAA", "CCC"]   # 按 symbol 排序
    assert len(out) == 2                            # 每标的恰一行


def test_summary_by_instrument_drops_thin():
    # boundary[3]: BBB 仅 2 < 3 个有效配对 → 被剔除, 只剩 AAA。
    ds = _seq_dates(5)
    scores = pd.concat([
        _scores(ds, [1, 2, 3, 4, 5], "AAA"),
        _scores(ds[:2], [1, 2], "BBB"),
    ])
    fwd = pd.concat([
        _fwd(ds, [0.1, 0.2, 0.3, 0.4, 0.5], "AAA"),
        _fwd(ds[:2], [0.1, 0.2], "BBB"),
    ])
    out = ic_summary_by_instrument(scores, fwd, h=5, min_periods=3)
    assert list(out["symbol"]) == ["AAA"]


def test_watchlist_summary_aggregates():
    # functional[2]: mean=0, median=0.10, breadth=2/3, n=3 数值正确。
    per_inst = pd.DataFrame({
        "symbol": ["AAA", "BBB", "CCC"], "ic": [0.10, 0.20, -0.30],
        "n_periods": [100, 100, 100], "t_stat": [1.0, 2.0, -3.0],
        "t_stat_nonoverlap": [0.5, 1.0, -1.5],
    })
    s = watchlist_summary(per_inst)
    assert math.isclose(s["mean_ic"], 0.0, abs_tol=1e-9)
    assert math.isclose(s["median_ic"], 0.10, rel_tol=1e-9)
    assert s["n_instruments"] == 3
    assert math.isclose(s["positive_breadth"], 2 / 3, rel_tol=1e-9)
    # icir = mean / std(ddof=1) = 0 / 0.2645751311... = 0.0
    assert math.isclose(s["icir"], 0.0, abs_tol=1e-9)


def test_watchlist_summary_icir_formula():
    # non_functional[5]: icir = ics.mean() / ics.std(ddof=1)（精确手工验算）。
    per_inst = pd.DataFrame({
        "symbol": ["AAA", "BBB", "CCC"], "ic": [0.10, 0.20, 0.30],
        "n_periods": [100, 100, 100], "t_stat": [1.0, 2.0, 3.0],
        "t_stat_nonoverlap": [0.5, 1.0, 1.5],
    })
    s = watchlist_summary(per_inst)
    ics = pd.Series([0.10, 0.20, 0.30])
    assert math.isclose(s["icir"], ics.mean() / ics.std(ddof=1), rel_tol=1e-9)
    assert math.isclose(s["positive_breadth"], 1.0, rel_tol=1e-9)   # 全正


def test_watchlist_summary_single_instrument_icir_none():
    # boundary[4]: n_instruments < 2 → icir=None（std(ddof=1) 单点无定义）。
    per_inst = pd.DataFrame({
        "symbol": ["AAA"], "ic": [0.15], "n_periods": [100],
        "t_stat": [1.5], "t_stat_nonoverlap": [0.7],
    })
    s = watchlist_summary(per_inst)
    assert s["n_instruments"] == 1
    assert s["icir"] is None
    assert math.isclose(s["mean_ic"], 0.15, rel_tol=1e-9)
    assert math.isclose(s["median_ic"], 0.15, rel_tol=1e-9)
    assert math.isclose(s["positive_breadth"], 1.0, rel_tol=1e-9)


def test_watchlist_summary_empty_all_none():
    # boundary[4]: ics 为空 → 全字段 None, n_instruments=0。
    per_inst = pd.DataFrame({
        "symbol": [], "ic": [], "n_periods": [],
        "t_stat": [], "t_stat_nonoverlap": [],
    })
    s = watchlist_summary(per_inst)
    assert s == {
        "mean_ic": None, "median_ic": None, "icir": None,
        "positive_breadth": None, "n_instruments": 0,
    }


def test_watchlist_summary_zero_std_icir_none():
    # non_functional[5]: 所有 ic 相同 → std(ddof=1)=0 → icir=None, 不抛 ZeroDivision。
    per_inst = pd.DataFrame({
        "symbol": ["AAA", "BBB", "CCC"], "ic": [0.20, 0.20, 0.20],
        "n_periods": [100, 100, 100], "t_stat": [2.0, 2.0, 2.0],
        "t_stat_nonoverlap": [1.0, 1.0, 1.0],
    })
    s = watchlist_summary(per_inst)
    assert s["icir"] is None
    assert math.isclose(s["mean_ic"], 0.20, rel_tol=1e-9)
    assert s["n_instruments"] == 3


def test_watchlist_summary_nan_ic_dropped():
    # non_functional[5] / boundary[4]: 含 NaN 的 ic 行被 dropna 后剩 1 个有效 → icir=None。
    # 也验证 std 为 NaN 路径(若只剩单点)不抛异常。
    per_inst = pd.DataFrame({
        "symbol": ["AAA", "BBB"], "ic": [0.15, float("nan")],
        "n_periods": [100, 100], "t_stat": [1.5, None],
        "t_stat_nonoverlap": [0.7, None],
    })
    s = watchlist_summary(per_inst)
    assert s["n_instruments"] == 1              # NaN ic 被剔除
    assert s["icir"] is None
    assert math.isclose(s["mean_ic"], 0.15, rel_tol=1e-9)
