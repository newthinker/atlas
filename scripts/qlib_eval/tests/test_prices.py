"""Context Checkpoint: done_criteria -> test mapping
functional[0] "信号 1/2 → 入场 1/3 开盘 10.2（严格次日，规避前视）" -> test_align_entry_next_open
functional[1] 保留 "1/5→1/15 恰 10 日历日 == max_defer*2 保留"      -> test_align_entry_boundary_defer_kept
functional[1] 丢弃 "1/2→1/15 13 日历日 > 10 → None"                -> test_align_entry_drops_when_defer_exceeds_limit
functional[2] "最后一根 bar 之后的信号 → None"                     -> test_align_entry_drops_when_no_data
boundary[0]   "Entry 携带 positional index"                        -> test_align_entry_carries_positional_index
non_functional[0] "顶层不得 import qlib（lazy import 守门）"        -> test_no_qlib_at_module_level

TASK-001 done_criteria -> test mapping:
functional[0] "默认 _benchmark==000300.SH，转 SH000300"             -> test_qlib_price_source_benchmark_param
functional[1] "benchmark='^HSI' 时 _benchmark==^HSI，转 HSI"        -> test_qlib_price_source_benchmark_param
functional[2] "benchmark() 实际请求 instrument == 转换值（防硬编码）" -> test_benchmark_requests_converted_instrument
boundary[0]   "构造不触发 qlib（惰性 import 保持）"                  -> test_qlib_price_source_benchmark_param + test_no_qlib_at_module_level
"""

import pandas as pd

from qlib_eval.prices import align_entry


def frame(dates, opens, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    return pd.DataFrame({"open": opens, "close": closes}, index=idx)


PRICES = frame(
    ["2024-01-02", "2024-01-03", "2024-01-04", "2024-01-05", "2024-01-15"],
    [10.0, 10.2, 10.4, 10.6, 11.0],
    [10.1, 10.3, 10.5, 10.7, 11.1],
)


def test_align_entry_next_open():
    # 信号日 1/2 → 入场 1/3 开盘（次日开盘，规避前视）
    e = align_entry(PRICES, pd.Timestamp("2024-01-02"), max_defer=5)
    assert e.date == pd.Timestamp("2024-01-03") and e.price == 10.2


def test_align_entry_boundary_defer_kept():
    # 边界保留用例：信号 1/5 → 下一 bar 1/15，恰好 10 个日历日 == max_defer*2，
    # 规则是「> max_defer*2 才丢弃」→ 10 不丢，入场 1/15。
    # ⚠️ 此用例钉死边界比较符必须是 >（写成 >= 此测试即翻车）
    e = align_entry(PRICES, pd.Timestamp("2024-01-05"), max_defer=5)
    assert e.date == pd.Timestamp("2024-01-15")


def test_align_entry_drops_when_defer_exceeds_limit():
    # 丢弃用例（与上一例配对夹住边界）：信号 1/2 → 若价格序列中 1/2 之后
    # 下一 bar 直接是 1/15（13 个日历日 > 10）→ None
    gappy = frame(["2024-01-02", "2024-01-15"], [10.0, 11.0], [10.1, 11.1])
    assert align_entry(gappy, pd.Timestamp("2024-01-02"), max_defer=5) is None


def test_align_entry_drops_when_no_data():
    # 其后无 bar
    assert align_entry(PRICES, pd.Timestamp("2024-01-15"), max_defer=5) is None


def test_align_entry_carries_positional_index():
    # boundary[0]：Entry.index 是 prices 中入场行的 positional index，供 horizon 计算复用。
    # 信号 1/2 → 入场行 1/3 是第 1 行（0-based）；信号 1/4 → 入场行 1/5 是第 3 行。
    assert align_entry(PRICES, pd.Timestamp("2024-01-02"), max_defer=5).index == 1
    assert align_entry(PRICES, pd.Timestamp("2024-01-04"), max_defer=5).index == 3


def test_no_qlib_at_module_level():
    # 守门测试（Task 5 硬约束的机制保证）：评估模块顶层不得 import qlib
    import sys

    import qlib_eval.prices  # noqa: F401

    assert "qlib" not in sys.modules


def test_qlib_price_source_benchmark_param():
    """QlibPriceSource 存储基准(atlas 形式)且默认为 A股 CSI300；构造不触发 qlib。"""
    from qlib_eval.prices import QlibPriceSource
    from qlib_eval.symbols import to_qlib_instrument

    s_default = QlibPriceSource(provider_uri="x", start="2021-01-01", end="2021-12-31")
    assert s_default._benchmark == "000300.SH"
    assert to_qlib_instrument(s_default._benchmark) == "SH000300"

    s_hk = QlibPriceSource(
        provider_uri="x", start="2021-01-01", end="2021-12-31", benchmark="^HSI"
    )
    assert s_hk._benchmark == "^HSI"
    assert to_qlib_instrument(s_hk._benchmark) == "HSI"


def test_benchmark_requests_converted_instrument(monkeypatch):
    # functional[2] reviewer 最高风险点：benchmark() 必须用 to_qlib_instrument(self._benchmark)
    # 而非残留硬编码 ["SH000300"]。注入 fake qlib(sys.modules) 捕获 D.features 实际入参。
    import sys
    import types

    import pandas as pd
    from qlib_eval.prices import QlibPriceSource

    captured = {}
    fake_qlib = types.ModuleType("qlib")
    fake_qlib.init = lambda **kw: None
    fake_data = types.ModuleType("qlib.data")

    class _D:
        @staticmethod
        def features(instruments, fields, start_time=None, end_time=None):
            captured["instruments"] = list(instruments)
            idx = pd.MultiIndex.from_product(
                [list(instruments), [pd.Timestamp("2021-01-04")]],
                names=["instrument", "datetime"],
            )
            return pd.DataFrame({"$open": [1.0], "$close": [1.0]}, index=idx)

    fake_data.D = _D
    monkeypatch.setitem(sys.modules, "qlib", fake_qlib)
    monkeypatch.setitem(sys.modules, "qlib.data", fake_data)

    QlibPriceSource(
        provider_uri="x", start="2021-01-01", end="2021-12-31", benchmark="^HSI"
    ).benchmark()
    assert captured["instruments"] == ["HSI"]

    QlibPriceSource(
        provider_uri="x", start="2021-01-01", end="2021-12-31"
    ).benchmark()
    assert captured["instruments"] == ["SH000300"]
