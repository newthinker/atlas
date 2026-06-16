"""SQLite schema for the qlib local data warehouse."""
import sqlite3

DDL = """
CREATE TABLE IF NOT EXISTS ohlcv (
  symbol TEXT NOT NULL, date TEXT NOT NULL,
  open REAL, high REAL, low REAL, close REAL,
  volume INTEGER, adj_close REAL,
  PRIMARY KEY (symbol, date)
);
CREATE TABLE IF NOT EXISTS fundamentals_pit (
  symbol TEXT NOT NULL, report_period TEXT NOT NULL, observe_date TEXT NOT NULL,
  eps_ttm REAL, pe REAL, pb REAL, ps REAL, roe REAL, dividend_yield REAL,
  PRIMARY KEY (symbol, report_period, observe_date)
);
CREATE TABLE IF NOT EXISTS warehouse_meta (
  symbol TEXT PRIMARY KEY, market TEXT NOT NULL, source TEXT NOT NULL,
  last_date TEXT NOT NULL, dumped_at TEXT NOT NULL
);
"""


def apply(conn: sqlite3.Connection) -> None:
    """Create all warehouse tables if absent."""
    conn.executescript(DDL)
    conn.commit()
