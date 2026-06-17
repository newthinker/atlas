"""Atomic SQLite writer for the warehouse."""
import os
import sqlite3
from typing import Iterable

from . import schema
from .ingest import OhlcvRow


def _insert_group(conn, rows, funds, market, source, dumped_at):
    """Insert one market group's ohlcv + warehouse_meta + fundamentals."""
    conn.executemany(
        "INSERT INTO ohlcv(symbol,date,open,high,low,close,volume,adj_close)"
        " VALUES(?,?,?,?,?,?,?,?)",
        [(r.symbol, r.date, r.open, r.high, r.low, r.close, r.volume, r.adj_close)
         for r in rows],
    )
    last_date = {}
    for r in rows:
        if r.date > last_date.get(r.symbol, ""):
            last_date[r.symbol] = r.date
    conn.executemany(
        "INSERT INTO warehouse_meta(symbol,market,source,last_date,dumped_at)"
        " VALUES(?,?,?,?,?)",
        [(sym, market, source, ld, dumped_at) for sym, ld in last_date.items()],
    )
    if funds:
        conn.executemany(
            "INSERT INTO fundamentals_pit"
            "(symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield)"
            " VALUES(?,?,?,?,?,?,?,?,?)",
            [(f.symbol, f.report_period, f.observe_date, f.eps_ttm,
              f.pe, f.pb, f.ps, f.roe, f.dividend_yield) for f in funds],
        )


def write_groups(db_path: str, groups, dumped_at: str) -> None:
    """Atomically (re)build the warehouse from multiple market groups.

    groups: iterable of dicts, each:
      {"rows": [OhlcvRow...], "market": str, "source": str,
       "fundamentals": [FundRow...]?}
    All groups land in one temp DB, then a single os.replace swaps it in, so
    atlas never reads a partially-built DB and the markets coexist.
    """
    tmp = db_path + ".tmp"
    if os.path.exists(tmp):
        os.remove(tmp)
    conn = sqlite3.connect(tmp)
    try:
        schema.apply(conn)
        for g in groups:
            _insert_group(
                conn,
                list(g.get("rows") or []),
                list(g.get("fundamentals") or []),
                g["market"], g["source"], dumped_at,
            )
        conn.commit()
    finally:
        conn.close()
    os.replace(tmp, db_path)


def write(db_path: str, rows: Iterable[OhlcvRow], market: str,
          source: str, dumped_at: str, fundamentals=None) -> None:
    """Single-market convenience wrapper over write_groups (back-compat).

    fundamentals: optional iterable of FundRow — written to fundamentals_pit
    in the same atomic transaction. Revisions (same report_period, different
    observe_date) are kept as-is; no deduplication is performed.
    """
    write_groups(
        db_path,
        [{"rows": rows, "fundamentals": fundamentals,
          "market": market, "source": source}],
        dumped_at,
    )
