import sqlite3
import textwrap
from scripts.qlib_warehouse import build_warehouse


def test_main_builds_warehouse_from_csv_dir(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    (csv_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,date,open,high,low,close,volume,factor
        aapl,2024-01-02,1,2,0.5,1.5,100,1
    """).lstrip())
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--csv-dir", str(csv_dir), "--market", "US",
        "--source", "yahoo", "--db", str(db),
        "--dumped-at", "2024-01-03T00:00:00Z",
    ])
    assert rc == 0
    conn = sqlite3.connect(str(db))
    assert conn.execute(
        "SELECT last_date FROM warehouse_meta WHERE symbol='AAPL'"
    ).fetchone()[0] == "2024-01-02"


def test_main_errors_on_missing_csv_dir(tmp_path):
    rc = build_warehouse.main([
        "--csv-dir", str(tmp_path / "nope"), "--market", "US",
        "--source", "yahoo", "--db", str(tmp_path / "w.db"),
    ])
    assert rc != 0
