"""Parse qlib_csv_* per-instrument CSV into normalized OHLCV rows."""
import csv
from collections import namedtuple
from pathlib import Path
from typing import List

OhlcvRow = namedtuple("OhlcvRow", "symbol date open high low close volume adj_close")


def _f(v):
    return float(v) if v not in (None, "") else None


def parse_file(path: Path) -> List[OhlcvRow]:
    rows = []
    with open(path, newline="") as fh:
        for d in csv.DictReader(fh):
            close = _f(d.get("close"))
            factor = _f(d.get("factor"))
            adj = close * factor if (close is not None and factor is not None) else close
            rows.append(OhlcvRow(
                symbol=d["symbol"].strip().upper(),
                date=d["date"].strip(),
                open=_f(d.get("open")), high=_f(d.get("high")),
                low=_f(d.get("low")), close=close,
                volume=int(float(d["volume"])) if d.get("volume") else None,
                adj_close=adj,
            ))
    return rows


def parse_dir(directory) -> List[OhlcvRow]:
    """Parse every *.csv in a directory into OHLCV rows."""
    out = []
    for p in sorted(Path(directory).glob("*.csv")):
        out.extend(parse_file(p))
    return out
