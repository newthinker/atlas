"""atlas 符号 <-> qlib instrument 映射。

A 股（设计 §1.1）：``600519.SH`` 形式（代码 + ``.SH``/``.SZ``/``.CSI`` 后缀）映射为
qlib 的 ``SH600519`` 形式（交易所前缀 + 代码）。
港股：``0700.HK`` -> ``HK00700``（``HK`` + 5 位补零）、``2800.HK`` -> ``HK02800``；
指数 ``^HSI`` -> ``HSI``、``^HSCE`` -> ``HSCEI``。
美股：裸 ticker（1-5 位大写字母）恒等 ``AAPL`` -> ``AAPL``；三个支持的美股指数剥离
``^`` 前缀 ``^GSPC`` -> ``GSPC``、``^IXIC`` -> ``IXIC``、``^DJI`` -> ``DJI``。其余一律拒绝。

与 Go 侧 ``cmd/atlas/export_ohlcv.go`` 的 ``toQlibInstrument`` 逐字对称（契约测试
共享样本），两边须同步修改。
"""

import re


def to_qlib_instrument(symbol: str) -> str:
    """600519.SH -> SH600519、399001.SZ -> SZ399001、930713.CSI -> CSI930713。

    港股：0700.HK -> HK00700（HK + 5 位补零）、2800.HK -> HK02800；指数
    ^HSI -> HSI、^HSCE -> HSCEI。美股：裸 ticker（1-5 位大写字母）恒等
    AAPL -> AAPL；^GSPC -> GSPC、^IXIC -> IXIC、^DJI -> DJI。其余（^HSTECH、
    期货 GC=F、加密 BTC-USDT、小写 aapl、6+ 字母如 ABCDEF 等）一律 raise ValueError。
    """
    if symbol.endswith(".SH"):
        return "SH" + symbol[:-3]
    if symbol.endswith(".SZ"):
        return "SZ" + symbol[:-3]
    if symbol.endswith(".CSI"):  # 中证跨市场指数 930713.CSI -> CSI930713
        return "CSI" + symbol[:-4]
    if symbol.endswith(".HK"):  # 港股 0700.HK -> HK00700（HK + 5 位补零）
        return "HK" + symbol[:-3].zfill(5)
    if symbol == "^HSI":  # 恒生指数
        return "HSI"
    if symbol == "^HSCE":  # 国企指数（业界 qlib 习惯命名 HSCEI）
        return "HSCEI"
    if symbol in ("^GSPC", "^IXIC", "^DJI"):  # 美股指数剥离 ^
        return symbol[1:]
    if re.fullmatch(r"[A-Z]{1,5}", symbol):  # 美股裸 ticker 恒等（全串锚定，等价 Go ^[A-Z]{1,5}$）
        return symbol
    raise ValueError(f"not a supported A-share/HK/US symbol: {symbol!r}")


# 逆映射：qlib instrument -> atlas symbol。
# 正向映射会丢失市场上下文（GSPC 既可能是 ^GSPC 也可能是叫 GSPC 的股票；
# 以 SH 开头的美股 ticker 会撞 A 股前缀），因此逆映射**必须带市场**才能消歧。
# market 取仓库的市场标签：US / CN_A（或 CN）/ HK。
_US_INDEX_FROM_QLIB = {"GSPC": "^GSPC", "IXIC": "^IXIC", "DJI": "^DJI"}
_HK_INDEX_FROM_QLIB = {"HSI": "^HSI", "HSCEI": "^HSCE"}


def from_qlib_instrument(instrument: str, market: str) -> str:
    """SH600519 ->(CN_A) 600519.SH、HK00700 ->(HK) 0700.HK、GSPC ->(US) ^GSPC。

    是 ``to_qlib_instrument`` 在给定 market 下的逆：对每个市场的合法符号满足
    ``from_qlib_instrument(to_qlib_instrument(s), market) == s``。market 提供上下文，
    使 US 组的 ``SHW`` 保持恒等（不误判为 A 股），US 组的 ``GSPC`` 还原为 ``^GSPC``。
    无法在该市场下解释的符号 raise ValueError（调用方可据此保留原值兜底）。
    """
    m = (market or "").upper()
    if m == "US":
        if instrument in _US_INDEX_FROM_QLIB:
            return _US_INDEX_FROM_QLIB[instrument]
        if re.fullmatch(r"[A-Z]{1,5}", instrument):
            return instrument
        raise ValueError(f"not a US instrument: {instrument!r}")
    if m in ("CN_A", "CN"):
        if instrument.startswith("SH"):
            return instrument[2:] + ".SH"
        if instrument.startswith("SZ"):
            return instrument[2:] + ".SZ"
        if instrument.startswith("CSI"):
            return instrument[3:] + ".CSI"
        raise ValueError(f"not a CN_A instrument: {instrument!r}")
    if m == "HK":
        if instrument in _HK_INDEX_FROM_QLIB:
            return _HK_INDEX_FROM_QLIB[instrument]
        if instrument.startswith("HK") and instrument[2:].isdigit():
            return f"{int(instrument[2:]):04d}.HK"  # 还原为 4 位补零的 Yahoo 港股代码
        raise ValueError(f"not an HK instrument: {instrument!r}")
    raise ValueError(f"unknown market: {market!r}")
