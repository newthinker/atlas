"""Context Checkpoint: done_criteria -> test mapping (TASK-003, 与 Go TASK-001 对称)
functional[0] "0700.HK->HK00700、2800.HK->HK02800（zfill(5)）"      -> test_to_qlib_instrument
functional[1] "^HSI->HSI、^HSCE->HSCEI"                             -> test_to_qlib_instrument
functional[2] "既有 SH/SZ/CSI 映射不回归"                          -> test_to_qlib_instrument
functional[3] "美股 AAPL/GOOGL 恒等、^GSPC/^IXIC/^DJI 剥离 ^"      -> test_to_qlib_instrument
boundary[0]   "锚定 off-by-one: ABCDEF(6)/TOOLONG(7)/aapl 小写 raise" -> test_to_qlib_instrument_rejects_non_ashare
boundary[1]   "^HSTECH/GC=F/BTC-USDT/AAPL123/AAPL.B 仍 raise"        -> test_to_qlib_instrument_rejects_non_ashare
error_handling "非支持符号 raise ValueError"                        -> test_to_qlib_instrument_rejects_non_ashare
"""

import pytest

from qlib_eval.symbols import to_qlib_instrument, from_qlib_instrument


def test_to_qlib_instrument():
    assert to_qlib_instrument("600519.SH") == "SH600519"
    assert to_qlib_instrument("000300.SH") == "SH000300"
    assert to_qlib_instrument("399001.SZ") == "SZ399001"
    assert to_qlib_instrument("930713.CSI") == "CSI930713"  # 中证跨市场指数
    assert to_qlib_instrument("0700.HK") == "HK00700"   # 港股股票
    assert to_qlib_instrument("2800.HK") == "HK02800"   # 港股 ETF
    assert to_qlib_instrument("^HSI") == "HSI"           # 恒生指数
    assert to_qlib_instrument("^HSCE") == "HSCEI"        # 国企指数
    assert to_qlib_instrument("AAPL") == "AAPL"          # 美股裸 ticker 恒等
    assert to_qlib_instrument("GOOGL") == "GOOGL"
    assert to_qlib_instrument("^GSPC") == "GSPC"         # 美股指数剥离 ^
    assert to_qlib_instrument("^IXIC") == "IXIC"
    assert to_qlib_instrument("^DJI") == "DJI"


def test_to_qlib_instrument_rejects_non_ashare():
    for bad in ("GC=F", "BTC-USDT", "^HSTECH", "AAPL123", "AAPL.B", "aapl", "ABCDEF", "TOOLONG"):
        with pytest.raises(ValueError):
            to_qlib_instrument(bad)


# --- 逆映射（qlib instrument -> atlas symbol），带市场上下文消除歧义 ---

def test_from_qlib_instrument_roundtrip_all_markets():
    cases = {
        "US": ["AAPL", "GOOGL", "JPM", "^GSPC", "^IXIC", "^DJI"],
        "CN_A": ["600519.SH", "000300.SH", "399001.SZ", "930713.CSI"],
        "HK": ["0700.HK", "2800.HK", "9988.HK", "^HSI", "^HSCE"],
    }
    for market, syms in cases.items():
        for s in syms:
            q = to_qlib_instrument(s)
            assert from_qlib_instrument(q, market) == s, f"{market}:{s}->{q}->?"


def test_from_qlib_instrument_market_disambiguates_us_ticker():
    # 美股 ticker 以 SH/HK 开头时，US 市场上下文必须保持恒等，
    # 不能误判成 A 股/港股（盲目逆映射的陷阱）。
    assert from_qlib_instrument("SHW", "US") == "SHW"
    assert from_qlib_instrument("SHOP", "US") == "SHOP"
    # 同名字符串在不同市场含义不同：US 视作 ticker，CN_A 视作 SH 前缀
    assert from_qlib_instrument("SH600519", "CN_A") == "600519.SH"


def test_from_qlib_instrument_us_index_vs_ticker():
    assert from_qlib_instrument("GSPC", "US") == "^GSPC"   # 索引表优先
    assert from_qlib_instrument("DJI", "US") == "^DJI"
    assert from_qlib_instrument("AAPL", "US") == "AAPL"    # 普通 ticker 恒等


def test_from_qlib_instrument_hk_zero_padding():
    assert from_qlib_instrument("HK00700", "HK") == "0700.HK"
    assert from_qlib_instrument("HK09988", "HK") == "9988.HK"
    assert from_qlib_instrument("HSI", "HK") == "^HSI"
    assert from_qlib_instrument("HSCEI", "HK") == "^HSCE"


def test_from_qlib_instrument_rejects_unknown():
    with pytest.raises(ValueError):
        from_qlib_instrument("HK00700", "US")   # 市场不符
    with pytest.raises(ValueError):
        from_qlib_instrument("X", "MARS")       # 未知市场
