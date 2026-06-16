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
    assert rc == 1


def test_main_errors_on_empty_csv_dir(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    # 存在但只含表头、无数据行的 .csv → parse 出 0 行 → return 2
    (csv_dir / "aapl.csv").write_text("symbol,date,open,high,low,close,volume,factor\n")
    rc = build_warehouse.main([
        "--csv-dir", str(csv_dir), "--market", "US",
        "--source", "yahoo", "--db", str(tmp_path / "w.db"),
    ])
    assert rc == 2


def test_main_ingests_fundamentals_when_dir_given(tmp_path):
    csv_dir = tmp_path / "csv"; csv_dir.mkdir()
    (csv_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,date,open,high,low,close,volume,factor
        aapl,2024-01-02,1,2,0.5,1.5,100,1
    """).lstrip())
    f_dir = tmp_path / "fund"; f_dir.mkdir()
    (f_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
        aapl,2023-12-31,2024-02-01,3.0,,,,,
    """).lstrip())
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--csv-dir", str(csv_dir), "--fundamentals-dir", str(f_dir),
        "--market", "US", "--source", "yahoo", "--db", str(db),
        "--dumped-at", "2024-03-01T00:00:00Z",
    ])
    assert rc == 0
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT eps_ttm FROM fundamentals_pit WHERE symbol='AAPL'").fetchone()[0] == 3.0


def test_main_errors_on_missing_fundamentals_dir(tmp_path):
    csv_dir = tmp_path / "csv"; csv_dir.mkdir()
    (csv_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,date,open,high,low,close,volume,factor
        aapl,2024-01-02,1,2,0.5,1.5,100,1
    """).lstrip())
    rc = build_warehouse.main([
        "--csv-dir", str(csv_dir), "--fundamentals-dir", str(tmp_path / "nope"),
        "--market", "US", "--source", "yahoo", "--db", str(tmp_path / "w.db"),
    ])
    assert rc == 3
