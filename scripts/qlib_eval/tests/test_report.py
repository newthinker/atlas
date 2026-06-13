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
            {"symbol": "GC=F", "date": pd.Timestamp("2024-01-01"),
             "strategy": "ma", "action": "buy", "confidence": 0.9, "price": 1.0, "metadata": "{}"},
        ]
    )
    source = _FakeSource({"600519.SH": prices}, bench)
    outcomes, stats = evaluate.collect_outcomes(signals, source, max_defer=5)
    # 非 A 股不中断：600519 仍产出 outcome，GC=F（不可映射）收进数据缺口
    assert len(outcomes) == 1
    assert "GC=F" in stats["non_ashare"]


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


# ---- review_fix (QA W1/W2/S7) ----

def test_main_empty_signals_writes_report_exit0(tmp_path):
    # QA W1：仅表头的空信号文件曾在 signals['date'].min().strftime 触发 NaTType 崩溃。
    # 修复后：入口短路写「无信号」报告并 exit 0，且不构造 QlibPriceSource（不触 qlib）。
    header_only = "symbol,date,strategy,action,confidence,price,metadata\n"
    sig_path = _write(tmp_path, header_only)
    qlib_dir = tmp_path / "qlib"
    qlib_dir.mkdir()  # 存在的目录 → 通过 check_qlib_dir
    out_dir = tmp_path / "out"
    rc = evaluate.main(
        ["--signals", sig_path, "--qlib-dir", str(qlib_dir), "--out", str(out_dir)]
    )
    assert rc == 0
    reports = list(out_dir.glob("signal-eval-*.md"))
    assert len(reports) == 1
    assert "信号总数: 0" in reports[0].read_text()
    assert "qlib" not in sys.modules


class _BenchFailSource:
    def history(self, symbol):  # pragma: no cover - 基准先失败，不会走到
        raise AssertionError("history should not be called when benchmark fails")

    def benchmark(self):
        raise FileNotFoundError("SH000300 not found in qlib data")


def test_collect_outcomes_benchmark_failure_is_graceful():
    # QA W2：source.benchmark() 抛错不得整跑崩溃——降级为空 outcomes + benchmark_error 提示。
    signals = pd.DataFrame(
        [
            {"symbol": "600519.SH", "date": pd.Timestamp("2024-01-01"),
             "strategy": "ma", "action": "buy", "confidence": 0.9,
             "price": 10.0, "metadata": "{}"},
        ]
    )
    outcomes, stats = evaluate.collect_outcomes(signals, _BenchFailSource(), max_defer=5)
    assert outcomes == []
    assert "benchmark_error" in stats
    assert "SH000300" in stats["benchmark_error"]


def test_render_report_surfaces_benchmark_error():
    md = render_report(
        aggregate([]),
        {"dropped": 0, "data_gaps": 0, "non_ashare": [], "na_counts": {},
         "benchmark_error": "SH000300 not found in qlib data"},
        {"generated_at": "2024-06-12", "n_signals": 3, "benchmark": "SH000300",
         "qlib_dir": "~"},
    )
    assert "基准" in md and "SH000300 not found in qlib data" in md


def test_read_signals_tolerates_utf8_bom(tmp_path):
    # QA S7：Excel 导出的 CSV 带 UTF-8 BOM，read_signals 需用 utf-8-sig 容忍。
    p = tmp_path / "bom.csv"
    p.write_bytes(("﻿" + VALID_CSV).encode("utf-8"))
    df = read_signals(str(p))
    assert len(df) == 2
    assert list(df.columns)[0] == "symbol"  # BOM 不得污染首列名


# ---- TASK-002: --benchmark 参数化 ----
# done_criteria -> test mapping
# functional[0] "_parse_args 默认 000300.SH / --benchmark ^HSI 时 ^HSI"
#               -> test_parse_args_benchmark_default_and_override
# functional[1] "_meta 的 benchmark 反映 args.benchmark"
#               -> test_meta_reflects_benchmark_arg
# boundary[0]   "缺省即 000300.SH（A股零回归）"
#               -> test_parse_args_benchmark_default_and_override（默认分支）


def test_parse_args_benchmark_default_and_override():
    a = evaluate._parse_args(["--signals", "s.csv"])
    assert a.benchmark == "000300.SH"
    b = evaluate._parse_args(["--signals", "s.csv", "--benchmark", "^HSI"])
    assert b.benchmark == "^HSI"


def test_meta_reflects_benchmark_arg():
    args = evaluate._parse_args(["--signals", "s.csv", "--benchmark", "^HSI"])
    meta = evaluate._meta(args, 5)
    assert meta["benchmark"] == "^HSI"


# ---- TASK-002 review_fix (QA F3/F2): render 层端到端验基准文案 ----
# functional "render_report 渲染输出文案随 meta benchmark 变（不止 dict 断言）"
#            -> test_render_report_text_reflects_benchmark


def _empty_stats() -> dict:
    return {"dropped": 0, "data_gaps": 0, "non_ashare": [], "na_counts": {}}


@pytest.mark.parametrize("benchmark", ["000300.SH", "^HSI"])
def test_render_report_text_reflects_benchmark(benchmark):
    # F3：meta 始终带 benchmark，render 应直接用 meta 值而非 fallback 默认。
    meta = {"generated_at": "2024-06-12", "n_signals": 3,
            "benchmark": benchmark, "qlib_dir": "~"}
    md = render_report(aggregate([]), _empty_stats(), meta)
    assert f"基准: {benchmark}" in md
    # 超额收益口径行也应引用同一基准
    assert f"相对基准 {benchmark}" in md
