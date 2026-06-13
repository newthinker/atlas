"""atlas 符号 <-> qlib instrument 映射。

Phase 1 仅支持 A 股（设计 §1.1）：``600519.SH`` 形式（6 位代码 + ``.SH``/``.SZ``
交易所后缀）映射为 qlib 的 ``SH600519`` 形式（交易所前缀 + 代码）。其余一律拒绝。
"""


def to_qlib_instrument(symbol: str) -> str:
    """600519.SH -> SH600519、399001.SZ -> SZ399001。

    非 A 股符号（美股 AAPL、指数 ^GSPC、期货 GC=F、加密 BTC-USDT、港股
    0700.HK 等）一律 raise ValueError —— Phase 1 为 A 股 only（设计 §1.1）。
    """
    if symbol.endswith(".SH"):
        return "SH" + symbol[:-3]
    if symbol.endswith(".SZ"):
        return "SZ" + symbol[:-3]
    if symbol.endswith(".CSI"):  # 中证跨市场指数 930713.CSI -> CSI930713
        return "CSI" + symbol[:-4]
    raise ValueError(f"not an A-share symbol: {symbol!r}")
