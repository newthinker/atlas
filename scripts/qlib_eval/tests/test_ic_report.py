"""Context Checkpoint: done_criteria -> test mapping
functional[1] "render_ic_report 含 ICIR/t-stat/逐标的(AAA)/horizon 5 标题"
              -> test_render_ic_report_contains_metrics
functional[2] "report 含重叠收益 t-stat 告诫(含 '重叠')"
              -> test_render_ic_report_contains_metrics (assert '重叠')
boundary[3]   "n_instruments<2 → '标的不足' + ICIR 不可计算"
              -> test_render_ic_report_thin_watchlist
boundary[4]   "per_horizon 空 dict → '无可评估分数' 不抛"
              -> test_render_ic_report_empty
error[5]      "scores 表头不符 → ValueError(match 'header mismatch')"
              -> test_read_scores_bad_header
error[6]      "坏行(列数不符/解析失败) → ValueError 带 1-based 物理行号"
              -> test_read_scores_bad_column_count / test_read_scores_unparseable_date
              -> test_read_scores_unparseable_score
error[7]      "空文件(仅 BOM / 完全空) → ValueError('scores CSV is empty')"
              -> test_read_scores_empty_file / test_read_scores_bom_only
non_func[8]   "utf-8-sig 容忍 BOM；重复 (date,symbol) 不去重/不报错"
              -> test_read_scores_bom_header_ok / test_read_scores_duplicate_pairs_kept
"""

import pandas as pd
import pytest

from qlib_eval.report import read_scores, render_ic_report


# --- error_handling[5]: 表头不符 ---------------------------------------------
def test_read_scores_bad_header(tmp_path):
    p = tmp_path / "s.csv"
    p.write_text("date,sym,score\n2024-01-01,AAA,0.1\n", encoding="utf-8")
    with pytest.raises(ValueError, match="header mismatch"):
        read_scores(str(p))


# --- error_handling[6] gap4: 坏行带 1-based 物理行号 -------------------------
def test_read_scores_bad_column_count(tmp_path):
    # 第 3 物理行列数不符（4 列）
    p = tmp_path / "s.csv"
    p.write_text(
        "date,symbol,score\n2024-01-01,AAA,0.1\n2024-01-02,BBB,0.2,extra\n",
        encoding="utf-8",
    )
    with pytest.raises(ValueError, match="3") as exc:
        read_scores(str(p))
    assert "line" in str(exc.value)


def test_read_scores_unparseable_date(tmp_path):
    # 第 3 物理行 date 无法解析
    p = tmp_path / "s.csv"
    p.write_text(
        "date,symbol,score\n2024-01-01,AAA,0.1\nnot-a-date,BBB,0.2\n",
        encoding="utf-8",
    )
    with pytest.raises(ValueError, match="3"):
        read_scores(str(p))


def test_read_scores_unparseable_score(tmp_path):
    # 第 2 物理行 score 无法解析为 float
    p = tmp_path / "s.csv"
    p.write_text("date,symbol,score\n2024-01-01,AAA,notnum\n", encoding="utf-8")
    with pytest.raises(ValueError, match="2"):
        read_scores(str(p))


# --- error_handling[7] gap4: 空文件 ------------------------------------------
def test_read_scores_empty_file(tmp_path):
    p = tmp_path / "s.csv"
    p.write_text("", encoding="utf-8")
    with pytest.raises(ValueError, match="scores CSV is empty"):
        read_scores(str(p))


def test_read_scores_bom_only(tmp_path):
    # 仅 BOM、无任何内容 → utf-8-sig 剥掉 BOM 后为空 → empty
    p = tmp_path / "s.csv"
    p.write_bytes(b"\xef\xbb\xbf")
    with pytest.raises(ValueError, match="scores CSV is empty"):
        read_scores(str(p))


# --- non_functional[8]: utf-8-sig BOM 容忍 + 重复对保留 ----------------------
def test_read_scores_bom_header_ok(tmp_path):
    # 带 BOM 的合法表头：BOM 不应污染首列名而误报 header mismatch
    p = tmp_path / "s.csv"
    p.write_bytes("date,symbol,score\n2024-01-01,AAA,0.1\n".encode("utf-8-sig"))
    df = read_scores(str(p))
    assert list(df.columns) == ["date", "symbol", "score"]
    assert df.iloc[0]["date"] == pd.Timestamp("2024-01-01")
    assert df.iloc[0]["score"] == pytest.approx(0.1)


def test_read_scores_duplicate_pairs_kept(tmp_path):
    # 已知约束：重复 (date,symbol) 不去重、不报错，全部保留
    p = tmp_path / "s.csv"
    p.write_text(
        "date,symbol,score\n2024-01-01,AAA,0.1\n2024-01-01,AAA,0.2\n",
        encoding="utf-8",
    )
    df = read_scores(str(p))
    assert len(df) == 2


# --- functional[1,2] + boundary[3,4]: render_ic_report -----------------------
def _per_horizon():
    by = pd.DataFrame({"symbol": ["AAA", "BBB"], "ic": [0.10, 0.20],
                       "n_periods": [100, 100], "t_stat": [1.0, 2.0],
                       "t_stat_nonoverlap": [0.5, 1.0]})
    summary = {"mean_ic": 0.15, "median_ic": 0.15, "icir": 1.5,
               "positive_breadth": 1.0, "n_instruments": 2}
    return {5: {"by_instrument": by, "summary": summary}}


def test_render_ic_report_contains_metrics():
    md = render_ic_report(_per_horizon(), {"generated_at": "2026-06-22",
                          "n_scores": 200, "method": "spearman", "qlib_dir": "x"})
    assert "ICIR" in md and "t-stat" in md
    assert "重叠" in md          # 重叠收益 t-stat 告诫
    assert "AAA" in md           # 逐标的明细
    assert "horizon 5" in md


def test_render_ic_report_thin_watchlist():
    ph = _per_horizon()
    ph[5]["summary"] = {"mean_ic": 0.1, "median_ic": 0.1, "icir": None,
                        "positive_breadth": 1.0, "n_instruments": 1}
    md = render_ic_report(ph, {"generated_at": "x", "n_scores": 1,
                          "method": "spearman", "qlib_dir": "x"})
    assert "标的不足" in md
    # ICIR 不可计算时不应渲染出数字，应是占位符
    assert "**ICIR**: -" in md


def test_render_ic_report_empty():
    md = render_ic_report({}, {"generated_at": "x", "n_scores": 0,
                          "method": "spearman", "qlib_dir": "x"})
    assert "无可评估分数" in md
