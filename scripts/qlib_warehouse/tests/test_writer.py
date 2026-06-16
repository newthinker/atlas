import sqlite3
from scripts.qlib_warehouse import writer
from scripts.qlib_warehouse.ingest import OhlcvRow
from scripts.qlib_warehouse.fundamentals import FundRow


def _rows():
    return [
        OhlcvRow("AAPL", "2024-01-02", 1, 2, 0.5, 1.5, 100, 1.5),
        OhlcvRow("AAPL", "2024-01-03", 1, 2, 0.5, 1.6, 110, 1.6),
    ]


def test_write_persists_ohlcv_and_meta(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), market="US", source="yahoo",
                 dumped_at="2024-01-04T00:00:00Z")
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 2
    meta = conn.execute(
        "SELECT market, source, last_date FROM warehouse_meta WHERE symbol='AAPL'"
    ).fetchone()
    assert meta == ("US", "yahoo", "2024-01-03")


def test_write_is_atomic_no_tmp_left(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), market="US", source="yahoo",
                 dumped_at="2024-01-04T00:00:00Z")
    assert not (tmp_path / "w.db.tmp").exists()


def test_write_overwrites_existing(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), "US", "yahoo", "2024-01-04T00:00:00Z")
    writer.write(str(db), _rows()[:1], "US", "yahoo", "2024-01-05T00:00:00Z")
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 1


def _funds():
    return [
        FundRow("AAPL", "2023-12-31", "2024-03-01", 3.0, None, None, None, None, None),
        FundRow("AAPL", "2024-03-31", "2024-05-15", 4.0, None, None, None, None, None),
        # 修订：同一 report_period 更晚 observe_date
        FundRow("AAPL", "2024-03-31", "2024-08-01", 4.2, None, None, None, None, None),
    ]


def test_write_persists_fundamentals_keeping_revisions(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), "US", "yahoo", "2024-09-01T00:00:00Z",
                 fundamentals=_funds())
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0] == 3
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 2  # ohlcv 不受影响


def test_write_without_fundamentals_is_backward_compatible(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), "US", "yahoo", "2024-09-01T00:00:00Z")
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0] == 0
