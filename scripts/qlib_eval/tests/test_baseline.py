"""Context Checkpoint: done_criteria -> test mapping
functional[1] "reversal_scores 公式 close 翻倍 lookback=1 第2点 score=-1.0"
              -> test_reversal_scores_formula
functional[2] "oracle_scores 端到端 → instrument_ic IC≈1.0"
              -> test_oracle_scores_give_ic_one
functional[3] "load_prices 单 symbol 取价异常被吞、不中断整体"
              -> test_load_prices_skips_failing_symbol
non_functional[4] "守门：import baseline/ic/report 后 'qlib' 均不在 sys.modules"
              -> test_no_qlib_at_module_level
non_functional[5] "baseline.py 顶层不得出现 import qlib" -> review (see test above)
"""

import math
import sys

import pandas as pd

from qlib_eval.baseline import load_prices, oracle_scores, reversal_scores
from qlib_eval.ic import forward_returns, instrument_ic


def price_frame(dates, opens, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    return pd.DataFrame({"open": opens, "close": closes}, index=idx)


DATES = ["2024-01-01", "2024-01-02", "2024-01-03", "2024-01-04", "2024-01-05",
         "2024-01-08", "2024-01-09", "2024-01-10", "2024-01-11", "2024-01-12"]


def test_reversal_scores_formula():
    # close 翻倍序列；lookback 1：score_t = -(c_t/c_{t-1} - 1)。第 2 点 = -(2/1-1) = -1.0
    df = price_frame(DATES, opens=[1] * 10, closes=[1, 2, 4, 8, 16, 32, 64, 128, 256, 512])
    out = reversal_scores({"AAA": df}, lookback=1)
    v = out[out["date"] == pd.Timestamp("2024-01-02")].iloc[0]["score"]
    assert math.isclose(v, -1.0, rel_tol=1e-9)


def test_oracle_scores_give_ic_one():
    # oracle: score = 该日前向收益本身 → 时序 IC 必为 1.0（plumbing 验证）
    closes = [10, 11, 12, 13, 14, 15, 16, 17, 18, 19]
    df = price_frame(DATES, opens=closes, closes=closes)
    fwd = forward_returns({"AAA": df}, horizons=(2,))
    scores = oracle_scores(fwd, horizon=2)
    res = instrument_ic(scores, fwd, "AAA", h=2, min_periods=3)
    assert math.isclose(res["ic"], 1.0, rel_tol=1e-9)


class _FakeSource:
    """注入用价格源：对 BAD symbol 抛异常，其余返回合成 frame。"""

    def __init__(self, frames, failing):
        self._frames = frames
        self._failing = failing

    def history(self, symbol):
        if symbol in self._failing:
            raise RuntimeError(f"no data for {symbol}")
        return self._frames[symbol]


def test_load_prices_skips_failing_symbol(monkeypatch):
    # 单 symbol 取价异常被吞、不中断整体：注入 fake source（不触发真实 qlib import）。
    frames = {"AAA": price_frame(DATES, opens=[1] * 10, closes=[1] * 10)}
    fake = _FakeSource(frames, failing={"BAD"})

    import qlib_eval.prices as prices_mod

    monkeypatch.setattr(prices_mod, "QlibPriceSource",
                        lambda *a, **k: fake, raising=True)

    out = load_prices("ignored", ["AAA", "BAD"], "2024-01-01", "2024-01-12")
    assert set(out) == {"AAA"}  # BAD 取价失败被吞，AAA 仍返回
    assert "qlib" not in sys.modules  # fake source 未触发真实 qlib import


def test_no_qlib_at_module_level():
    # 强化守门：依次 import 所有新/改模块顶层，'qlib' 均不在 sys.modules（防间接 import 链）。
    import qlib_eval.baseline  # noqa: F401
    import qlib_eval.ic  # noqa: F401
    import qlib_eval.report  # noqa: F401

    assert "qlib" not in sys.modules
