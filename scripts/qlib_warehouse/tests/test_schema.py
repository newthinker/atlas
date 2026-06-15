import sqlite3
from scripts.qlib_warehouse import schema


def test_apply_creates_all_tables():
    conn = sqlite3.connect(":memory:")
    schema.apply(conn)
    names = {r[0] for r in conn.execute(
        "SELECT name FROM sqlite_master WHERE type='table'"
    )}
    assert {"ohlcv", "fundamentals_pit", "warehouse_meta"} <= names


def test_ohlcv_primary_key_is_symbol_date():
    conn = sqlite3.connect(":memory:")
    schema.apply(conn)
    conn.execute("INSERT INTO ohlcv(symbol,date,close) VALUES('AAPL','2024-01-02',1.0)")
    # 同 (symbol,date) 再插入应因主键冲突失败
    import pytest
    with pytest.raises(sqlite3.IntegrityError):
        conn.execute("INSERT INTO ohlcv(symbol,date,close) VALUES('AAPL','2024-01-02',2.0)")
