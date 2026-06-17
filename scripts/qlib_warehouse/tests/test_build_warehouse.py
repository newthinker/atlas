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
    # Backward compatibility: without --fundamentals-dir, no PIT rows are written.
    assert conn.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0] == 0


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


def _mkcsv(d, name, sym, date, close):
    d.mkdir(exist_ok=True)
    (d / name).write_text(
        "symbol,date,open,high,low,close,volume,factor\n"
        f"{sym},{date},1,2,0.5,{close},100,1\n"
    )


def test_main_multi_market_add_coexist(tmp_path):
    us = tmp_path / "us"; hk = tmp_path / "hk"
    _mkcsv(us, "aapl.csv", "aapl", "2024-01-02", 1.5)
    _mkcsv(hk, "tx.csv", "0700.HK", "2024-01-02", 9.9)
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--db", str(db), "--dumped-at", "2024-01-03T00:00:00Z",
        "--add", "US", "yahoo", str(us),
        "--add", "HK", "yahoo", str(hk),
    ])
    assert rc == 0
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 2
    markets = {r[0] for r in conn.execute("SELECT DISTINCT market FROM warehouse_meta")}
    assert markets == {"US", "HK"}


def test_main_multi_market_skips_missing_dirs(tmp_path):
    us = tmp_path / "us"
    _mkcsv(us, "aapl.csv", "aapl", "2024-01-02", 1.5)
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--db", str(db), "--dumped-at", "2024-01-03T00:00:00Z",
        "--add", "US", "yahoo", str(us),
        "--add", "HK", "yahoo", str(tmp_path / "missing_hk"),  # 缺目录 → 跳过，不报错
    ])
    assert rc == 0
    conn = sqlite3.connect(str(db))
    assert {r[0] for r in conn.execute("SELECT DISTINCT market FROM warehouse_meta")} == {"US"}


def test_main_multi_market_all_empty_errors(tmp_path):
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--db", str(db),
        "--add", "US", "yahoo", str(tmp_path / "nope1"),
        "--add", "HK", "yahoo", str(tmp_path / "nope2"),
    ])
    assert rc == 2
