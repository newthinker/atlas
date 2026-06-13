"""Context Checkpoint: done_criteria -> test mapping (TASK-002, 与 Go TASK-001 对称)
functional[0] "0700.HK->HK00700、2800.HK->HK02800（zfill(5)）"      -> test_to_qlib_instrument
functional[1] "^HSI->HSI、^HSCE->HSCEI"                             -> test_to_qlib_instrument
functional[2] "既有 SH/SZ/CSI 映射不回归"                          -> test_to_qlib_instrument
boundary[0]   "^HSTECH 与 AAPL/^GSPC/GC=F/BTC-USDT 仍 raise"        -> test_to_qlib_instrument_rejects_non_ashare
error_handling "非支持符号 raise ValueError"                        -> test_to_qlib_instrument_rejects_non_ashare
"""

import pytest

from qlib_eval.symbols import to_qlib_instrument


def test_to_qlib_instrument():
    assert to_qlib_instrument("600519.SH") == "SH600519"
    assert to_qlib_instrument("000300.SH") == "SH000300"
    assert to_qlib_instrument("399001.SZ") == "SZ399001"
    assert to_qlib_instrument("930713.CSI") == "CSI930713"  # 中证跨市场指数
    assert to_qlib_instrument("0700.HK") == "HK00700"   # 港股股票
    assert to_qlib_instrument("2800.HK") == "HK02800"   # 港股 ETF
    assert to_qlib_instrument("^HSI") == "HSI"           # 恒生指数
    assert to_qlib_instrument("^HSCE") == "HSCEI"        # 国企指数


def test_to_qlib_instrument_rejects_non_ashare():
    for bad in ("AAPL", "^GSPC", "GC=F", "BTC-USDT", "^HSTECH"):
        with pytest.raises(ValueError):
            to_qlib_instrument(bad)
