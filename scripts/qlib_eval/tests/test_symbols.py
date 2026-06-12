"""Context Checkpoint: done_criteria -> test mapping
functional[0] "to_qlib_instrument 三正例（SH/SZ 前缀换位）" -> test_to_qlib_instrument
functional[1] "五类非 A 股符号均 raise ValueError"          -> test_to_qlib_instrument_rejects_non_ashare
"""

import pytest

from qlib_eval.symbols import to_qlib_instrument


def test_to_qlib_instrument():
    assert to_qlib_instrument("600519.SH") == "SH600519"
    assert to_qlib_instrument("000300.SH") == "SH000300"
    assert to_qlib_instrument("399001.SZ") == "SZ399001"


def test_to_qlib_instrument_rejects_non_ashare():
    for bad in ("AAPL", "^GSPC", "GC=F", "BTC-USDT", "0700.HK"):
        with pytest.raises(ValueError):
            to_qlib_instrument(bad)
