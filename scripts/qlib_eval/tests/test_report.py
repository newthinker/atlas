"""Context Checkpoint: done_criteria -> test mapping
functional[0] "read_signals 合法 CSV→DataFrame(date=Timestamp/conf=float/metadata 原串)" -> test_read_signals_valid
error_handling "缺列/坏行 ValueError 含行号"        -> test_read_signals_invalid_*
functional[1] "render_report 含评估口径/数据缺口/每策略小节" -> test_render_report_sections
boundary[0]   "非 A 股不中断评估，收集进数据缺口节"  -> test_non_ashare_collected_into_data_gaps
error_handling "qlib 数据目录缺失→打印下载命令 exit(1)" -> test_main_exits_when_qlib_dir_missing
non_functional "import evaluate 不触发 qlib"          -> test_import_evaluate_no_qlib
"""

import sys

import pandas as pd
import pytest

import evaluate
from qlib_eval.event_study import Signal, aggregate, evaluate_signal
from qlib_eval.report import read_signals, render_report

VALID_CSV = (
    "symbol,date,strategy,action,confidence,price,metadata\n"
    '600519.SH,2024-01-03,flat,buy,0.70,101.00,"{""k"":1}"\n'
    '600519.SH,2024-01-04,flat,buy,0.85,102.00,"{""k"":2}"\n'
)


def _write(tmp_path, text, name="signals.csv"):
    p = tmp_path / name
    p.write_text(text)
    return str(p)


def test_read_signals_valid(tmp_path):
    df = read_signals(_write(tmp_path, VALID_CSV))
    assert list(df.columns) == [
        "symbol", "date", "strategy", "action", "confidence", "price", "metadata"
    ]
    assert len(df) == 2
    assert df["date"].iloc[0] == pd.Timestamp("2024-01-03")
    assert isinstance(df["confidence"].iloc[0], float) and df["confidence"].iloc[0] == 0.70
    # metadata 保留原串（解析后的 JSON 文本，不做反序列化）
    assert df["metadata"].iloc[0] == '{"k":1}'


def test_read_signals_invalid_header(tmp_path):
    bad = "symbol,date,strategy,action,confidence,price\n600519.SH,2024-01-03,flat,buy,0.7,101.0\n"
    with pytest.raises(ValueError, match="header"):
        read_signals(_write(tmp_path, bad))


def test_read_signals_invalid_missing_column_has_line_number(tmp_path):
    # 第 3 行（数据第 2 行）少一列 → ValueError 且消息含行号 3
    bad = (
        "symbol,date,strategy,action,confidence,price,metadata\n"
        '600519.SH,2024-01-03,flat,buy,0.70,101.00,"{}"\n'
        "600519.SH,2024-01-04,flat,buy,0.85,102.00\n"
    )
    with pytest.raises(ValueError, match="line 3"):
        read_signals(_write(tmp_path, bad))


def test_read_signals_invalid_confidence_has_line_number(tmp_path):
    bad = (
        "symbol,date,strategy,action,confidence,price,metadata\n"
        '600519.SH,2024-01-03,flat,buy,notafloat,101.00,"{}"\n'
    )
    with pytest.raises(ValueError, match="line 2"):
        read_signals(_write(tmp_path, bad))


def _sample_agg():
    outcomes = [
        # 两条 buy，超额 +5%/-1%，conf 高，落入 ma 策略
        _mk("ma", "buy", 0.9, 0.05),
        _mk("ma", "buy", 0.9, -0.01),
        # 一条 sell，规避 +2%，pe 策略
        _mk("pe", "sell", 0.9, 0.02),
    ]
    return aggregate(outcomes)


def _mk(strategy, action, confidence, excess5):
    from qlib_eval.event_study import SignalOutcome

    return SignalOutcome(
        symbol="600519.SH", date=pd.Timestamp("2024-01-01"), strategy=strategy,
        action=action, confidence=confidence,
        returns={5: excess5, 20: None, 60: None},
        excess={5: excess5, 20: None, 60: None},
    )


def test_render_report_sections():
    agg = _sample_agg()
    stats = {"dropped": 3, "data_gaps": 1, "non_ashare": ["AAPL", "0700.HK"], "na_counts": {20: 2, 60: 3}}
    meta = {"generated_at": "2024-06-12", "n_signals": 10, "benchmark": "SH000300", "qlib_dir": "~/.qlib"}
    md = render_report(agg, stats, meta)
    assert isinstance(md, str)
    assert "评估口径" in md
    assert "数据缺口" in md
    # 每策略小节
    assert "ma" in md and "pe" in md
    # 数据缺口节体现非 A 股跳过与 dropped
    assert "AAPL" in md and "0700.HK" in md
    # 表头列名出现
    assert "win_rate" in md


# ---- evaluate.py orchestration ----

class _FakeSource:
    def __init__(self, frames, bench):
        self._frames = frames
        self._bench = bench

    def history(self, symbol):
        return self._frames[symbol]

    def benchmark(self):
        return self._bench


def _price_frame(dates, opens, closes):
    idx = pd.DatetimeIndex(pd.to_datetime(dates))
    return pd.DataFrame({"open": opens, "close": closes}, index=idx)


def test_non_ashare_collected_into_data_gaps():
    dates = ["2024-01-01", "2024-01-02", "2024-01-03", "2024-01-04",
             "2024-01-05", "2024-01-08", "2024-01-09"]
    prices = _price_frame(dates, [9, 10, 10, 10, 10, 10, 10], [9, 10, 10, 10, 10, 11, 11])
    bench = _price_frame(dates, [3000] * 7, [3000, 3000, 3000, 3000, 3000, 3060, 3060])
    signals = pd.DataFrame(
        [
            {"symbol": "600519.SH", "date": pd.Timestamp("2024-01-01"),
             "strategy": "ma", "action": "buy", "confidence": 0.9, "price": 10.0, "metadata": "{}"},
            {"symbol": "AAPL", "date": pd.Timestamp("2024-01-01"),
             "strategy": "ma", "action": "buy", "confidence": 0.9, "price": 1.0, "metadata": "{}"},
        ]
    )
    source = _FakeSource({"600519.SH": prices}, bench)
    outcomes, stats = evaluate.collect_outcomes(signals, source, max_defer=5)
    # 非 A 股不中断：600519 仍产出 outcome，AAPL 收进数据缺口
    assert len(outcomes) == 1
    assert "AAPL" in stats["non_ashare"]


def test_main_exits_when_qlib_dir_missing(tmp_path, capsys):
    signals = _write(tmp_path, VALID_CSV)
    missing = str(tmp_path / "no_such_qlib_dir")
    rc = evaluate.main(["--signals", signals, "--qlib-dir", missing, "--out", str(tmp_path)])
    assert rc == 1
    err = capsys.readouterr().err
    # 打印 get_data 下载命令指引
    assert "qlib_data" in err
    # 未触发 qlib import（main 在缺目录时提前返回）
    assert "qlib" not in sys.modules


def test_import_evaluate_no_qlib():
    import evaluate  # noqa: F401
    assert "qlib" not in sys.modules
