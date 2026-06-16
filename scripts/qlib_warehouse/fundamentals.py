"""Parse normalized point-in-time fundamentals CSV into rows."""
import csv
from collections import namedtuple
from pathlib import Path
from typing import List

FundRow = namedtuple(
    "FundRow",
    "symbol report_period observe_date eps_ttm pe pb ps roe dividend_yield",
)


def _f(v):
    return float(v) if v not in (None, "") else None


def parse_file(path: Path) -> List[FundRow]:
    rows = []
    with open(path, newline="") as fh:
        for d in csv.DictReader(fh):
            eps_ttm = _f(d.get("eps_ttm"))
            # Leader 收紧：eps_ttm 为必填列，空值行跳过，不入主源。
            if eps_ttm is None:
                continue
            rows.append(FundRow(
                symbol=d["symbol"].strip().upper(),
                report_period=d["report_period"].strip(),
                observe_date=d["observe_date"].strip(),
                eps_ttm=eps_ttm,
                pe=_f(d.get("pe")), pb=_f(d.get("pb")), ps=_f(d.get("ps")),
                roe=_f(d.get("roe")), dividend_yield=_f(d.get("dividend_yield")),
            ))
    return rows


def parse_dir(directory) -> List[FundRow]:
    out = []
    for p in sorted(Path(directory).glob("*.csv")):
        out.extend(parse_file(p))
    return out
