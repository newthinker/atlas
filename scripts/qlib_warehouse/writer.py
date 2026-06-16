"""Atomic SQLite writer for the warehouse."""
import os
import sqlite3
from typing import Iterable

from . import schema
from .ingest import OhlcvRow


def write(db_path: str, rows: Iterable[OhlcvRow], market: str,
          source: str, dumped_at: str) -> None:
    """Write all rows to a temp DB then atomically replace db_path."""
    rows = list(rows)
    tmp = db_path + ".tmp"
    if os.path.exists(tmp):
        os.remove(tmp)
    conn = sqlite3.connect(tmp)
    try:
        schema.apply(conn)
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
        conn.commit()
    finally:
        conn.close()
    os.replace(tmp, db_path)
