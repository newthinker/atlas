# Context Checkpoint: done_criteria → test mapping
# functional[0] "parse_dir reads required and optional fields" → test_parse_dir_reads_required_and_optional
# boundary[0]   "skip non-csv files"                          → test_parse_dir_skips_non_csv
# boundary[1]   "eps_ttm 为空的行被跳过"                      → test_parse_dir_skips_empty_eps_ttm
import textwrap
from scripts.qlib_warehouse import fundamentals


def _write(tmp_path, name, content):
    p = tmp_path / name
    p.write_text(textwrap.dedent(content).lstrip())
    return p


def test_parse_dir_reads_required_and_optional(tmp_path):
    _write(tmp_path, "aapl.csv", """
        symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
        aapl,2024-03-31,2024-05-02,6.42,28.1,,,1.5,0.5
    """)
    rows = fundamentals.parse_dir(tmp_path)
    assert len(rows) == 1
    r = rows[0]
    assert r.symbol == "AAPL"
    assert r.report_period == "2024-03-31"
    assert r.observe_date == "2024-05-02"
    assert r.eps_ttm == 6.42
    assert r.pb is None
    assert r.roe == 1.5


def test_parse_dir_skips_non_csv(tmp_path):
    (tmp_path / "notes.txt").write_text("x")
    assert fundamentals.parse_dir(tmp_path) == []


def test_parse_dir_skips_empty_eps_ttm(tmp_path):
    """Leader 收紧：eps_ttm 为空（必填列）的行必须被 parse_file 跳过，不入主源。"""
    _write(tmp_path, "mixed.csv", """
        symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
        aapl,2024-03-31,2024-05-02,6.42,28.1,,,1.5,0.5
        aapl,2023-12-31,2024-02-01,,22.0,,,1.2,0.4
    """)
    rows = fundamentals.parse_dir(tmp_path)
    assert len(rows) == 1
    assert rows[0].eps_ttm == 6.42
