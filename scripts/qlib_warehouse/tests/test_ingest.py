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


def test_parse_csv_empty_numeric_fields_yield_none(tmp_path):
    _write(tmp_path, "aapl.csv", """
        symbol,date,open,high,low,close,volume,factor
        aapl,2024-01-02,,,,1.5,,
    """)
    rows = ingest.parse_dir(tmp_path)
    assert len(rows) == 1
    r = rows[0]
    assert r.open is None
    assert r.high is None
    assert r.low is None
    assert r.volume is None
    assert r.close == 1.5


def test_parse_dir_skips_non_csv(tmp_path):
    (tmp_path / "README.md").write_text("not csv")
    assert ingest.parse_dir(tmp_path) == []


def test_parse_dir_maps_qlib_to_atlas_symbol_by_market(tmp_path):
    # 真实 CSV 的 symbol 是 qlib 实例名；带 market 应逆映射回 atlas 符号。
    _write(tmp_path, "hk00700.csv", """
        symbol,date,open,high,low,close,volume,factor
        HK00700,2024-01-02,1,2,0.5,1.5,100,1
    """)
    rows = ingest.parse_dir(tmp_path, market="HK")
    assert rows[0].symbol == "0700.HK"
    # 无 market 时保持原 qlib 名（向后兼容）
    rows2 = ingest.parse_dir(tmp_path)
    assert rows2[0].symbol == "HK00700"


def test_parse_dir_keeps_symbol_when_market_mismatch(tmp_path):
    # 符号在该市场下无法解释 → 保留原值（可降级，不丢数据）。
    _write(tmp_path, "hk00700.csv", """
        symbol,date,open,high,low,close,volume,factor
        HK00700,2024-01-02,1,2,0.5,1.5,100,1
    """)
    rows = ingest.parse_dir(tmp_path, market="US")  # HK00700 非 US 符号
    assert rows[0].symbol == "HK00700"
