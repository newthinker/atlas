import textwrap
from scripts.qlib_warehouse import ingest


def _write(tmp_path, name, content):
    p = tmp_path / name
    p.write_text(textwrap.dedent(content).lstrip())
    return p


def test_parse_csv_computes_adj_close_and_uppercases(tmp_path):
    _write(tmp_path, "aapl.csv", """
        symbol,date,open,high,low,close,volume,factor
        aapl,2021-01-04,133.52,133.61,126.76,129.41,143301900,2
    """)
    rows = ingest.parse_dir(tmp_path)
    assert len(rows) == 1
    r = rows[0]
    assert r.symbol == "AAPL"
    assert r.date == "2021-01-04"
    assert r.close == 129.41
    assert r.volume == 143301900
    assert r.adj_close == 129.41 * 2


def test_parse_csv_without_factor_defaults_adj_close_to_close(tmp_path):
    _write(tmp_path, "x.csv", """
        symbol,date,open,high,low,close,volume
        x,2024-01-02,1,2,0.5,1.5,100
    """)
    rows = ingest.parse_dir(tmp_path)
    assert rows[0].adj_close == 1.5


def test_parse_dir_skips_non_csv(tmp_path):
    (tmp_path / "README.md").write_text("not csv")
    assert ingest.parse_dir(tmp_path) == []
