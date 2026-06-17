"""Parse qlib_csv_* per-instrument CSV into normalized OHLCV rows.

CSV 里的 symbol 是 qlib 实例名（export-ohlcv 的 to_qlib_instrument 产出，如 HK00700、
SH600519、GSPC）。仓库被 atlas 用**原始符号**消费，故摄取时按市场逆映射回 atlas 符号
（0700.HK、600519.SH、^GSPC）。market 缺省时不映射（保持向后兼容）。
"""
import csv
from collections import namedtuple
from pathlib import Path
from typing import List

from scripts.qlib_eval.qlib_eval.symbols import from_qlib_instrument

OhlcvRow = namedtuple("OhlcvRow", "symbol date open high low close volume adj_close")


def _f(v):
    return float(v) if v not in (None, "") else None


def _to_atlas(symbol: str, market) -> str:
    """qlib 实例名 -> atlas 符号；market 缺省或符号无法解释时原样返回（可降级）。"""
    if not market:
        return symbol
    try:
        return from_qlib_instrument(symbol, market)
    except ValueError:
        return symbol


def parse_file(path: Path, market=None) -> List[OhlcvRow]:
    rows = []
    with open(path, newline="") as fh:
        for d in csv.DictReader(fh):
            close = _f(d.get("close"))
            factor = _f(d.get("factor"))
            adj = close * factor if (close is not None and factor is not None) else close
            rows.append(OhlcvRow(
                symbol=_to_atlas(d["symbol"].strip().upper(), market),
                date=d["date"].strip(),
                open=_f(d.get("open")), high=_f(d.get("high")),
                low=_f(d.get("low")), close=close,
                volume=int(float(d["volume"])) if d.get("volume") else None,
                adj_close=adj,
            ))
    return rows


def parse_dir(directory, market=None) -> List[OhlcvRow]:
    """Parse every *.csv in a directory into OHLCV rows (qlib->atlas symbol by market)."""
    out = []
    for p in sorted(Path(directory).glob("*.csv")):
        out.extend(parse_file(p, market))
    return out
