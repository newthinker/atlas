#!/usr/bin/env python
"""信号事件研究 CLI 入口。

用法::

    python evaluate.py --signals signals.csv \
      [--qlib-dir ~/.qlib/qlib_data/cn_data] [--out reports/]

启动即检测 qlib 数据目录，缺失则打印 get_data 下载命令并 exit(1)（设计 §5）。
非 A 股符号跳过收集进「数据缺口」节，不中断评估。报告写
``<out>/signal-eval-YYYYMMDD.md``。

注意：本模块顶层不 import qlib——qlib 仅在真实运行分支经 QlibPriceSource 惰性导入，
故 pytest（含 --qlib-dir 缺失路径）零 qlib 依赖。
"""

import argparse
import datetime as _dt
import os
import sys

import pandas as pd

from qlib_eval.event_study import Signal, aggregate, evaluate_signal
from qlib_eval.report import read_signals, render_report
from qlib_eval.symbols import to_qlib_instrument

DEFAULT_QLIB_DIR = "~/.qlib/qlib_data/cn_data"
DEFAULT_OUT = "reports/"


def check_qlib_dir(qlib_dir: str) -> bool:
    """qlib 数据目录是否存在（展开 ~）。"""
    return os.path.isdir(os.path.expanduser(qlib_dir))


def get_data_hint(qlib_dir: str) -> str:
    """qlib 数据目录缺失时打印的下载指引。"""
    return (
        f"qlib 数据目录不存在: {qlib_dir}\n"
        "请先下载中国市场日频数据包（托管在 SunsetWolf/qlib_dataset releases，"
        "国内可能需代理）：\n"
        f"  python -m qlib.cli.data qlib_data "
        f"--target_dir {qlib_dir} --region cn\n"
    )


def _empty_stats() -> dict:
    """零样本时的 stats 骨架（空信号短路、基准降级共用，避免两处 drift）。"""
    return {"dropped": 0, "data_gaps": 0, "non_ashare": [], "na_counts": {}}


def collect_outcomes(signals: pd.DataFrame, source, max_defer: int = 5):
    """对每个信号求 outcome。返回 ``(outcomes, stats)``。

    - 非 A 股符号（``to_qlib_instrument`` raise ValueError）不中断评估，收集进
      ``stats["non_ashare"]``；
    - ``evaluate_signal`` 返回 None（无入场 / 入场早于基准）计入 ``stats["dropped"]``；
    - ``stats["data_gaps"]`` 计 source 取价失败的符号数；
    - 基准取数失败（与逐 symbol 降级对称）不整跑崩溃，降级为空 outcomes +
      ``stats["benchmark_error"]`` 明确提示（无基准则全部超额无法计算）。
    """
    try:
        bench = source.benchmark()
    except Exception as e:  # noqa: BLE001 — 基准缺失整体降级，不抛栈给用户
        return [], {**_empty_stats(), "benchmark_error": str(e)}
    outcomes = []
    dropped = 0
    data_gaps = 0
    non_ashare = []
    for symbol, grp in signals.groupby("symbol", sort=False):
        try:
            to_qlib_instrument(symbol)
        except ValueError:
            non_ashare.append(symbol)
            continue
        try:
            prices = source.history(symbol)
        except Exception:  # noqa: BLE001 — 取价失败不应中断整体评估
            data_gaps += 1
            continue
        for _, r in grp.iterrows():
            sig = Signal(
                symbol=symbol, date=r["date"], strategy=r["strategy"],
                action=r["action"], confidence=float(r["confidence"]),
            )
            outcome = evaluate_signal(sig, prices, bench, max_defer=max_defer)
            if outcome is None:
                dropped += 1
            else:
                outcomes.append(outcome)
    stats = {
        "dropped": dropped,
        "data_gaps": data_gaps,
        "non_ashare": non_ashare,
        "na_counts": _na_counts(outcomes),
    }
    return outcomes, stats


def _na_counts(outcomes) -> dict:
    """各 horizon 收益为 None（越界）的样本数。"""
    counts: dict[int, int] = {}
    for o in outcomes:
        for h, ret in o.returns.items():
            if ret is None:
                counts[h] = counts.get(h, 0) + 1
    return counts


def _parse_args(argv):
    p = argparse.ArgumentParser(description="信号事件研究评估")
    p.add_argument("--signals", required=True, help="export-signals 产出的 CSV 路径")
    p.add_argument("--qlib-dir", default=DEFAULT_QLIB_DIR, help="qlib 数据目录")
    p.add_argument("--out", default=DEFAULT_OUT, help="报告输出目录")
    p.add_argument("--max-defer", type=int, default=5, help="入场顺延上限（交易日近似）")
    p.add_argument("--benchmark", default="000300.SH",
                   help="基准 symbol（atlas 形式：A股 000300.SH / 港股 ^HSI）")
    return p.parse_args(argv)


def _meta(args, n_signals: int) -> dict:
    return {
        "generated_at": _dt.date.today().isoformat(),
        "n_signals": n_signals,
        "benchmark": args.benchmark,
        "qlib_dir": args.qlib_dir,
    }


def _write_report(out_dir: str, report: str) -> str:
    os.makedirs(out_dir, exist_ok=True)
    out_path = os.path.join(out_dir, f"signal-eval-{_dt.date.today():%Y%m%d}.md")
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(report)
    return out_path


def main(argv=None) -> int:
    args = _parse_args(argv)

    if not check_qlib_dir(args.qlib_dir):
        sys.stderr.write(get_data_hint(args.qlib_dir))
        return 1

    signals = read_signals(args.signals)

    # 空信号文件（仅表头）：入口短路写「无信号」报告并正常退出——避免对空 date 列
    # 取 min/max 得到 NaT、再 strftime 触发 NaTType 崩溃（QA W1，实测复现）。
    if signals.empty:
        out_path = _write_report(
            args.out, render_report(aggregate([]), _empty_stats(), _meta(args, 0))
        )
        sys.stdout.write(f"无信号可评估，已写入空报告 {out_path}\n")
        return 0

    # 真实运行：此处才惰性构造 qlib 数据源（pytest 不触达）
    from qlib_eval.prices import QlibPriceSource

    start = signals["date"].min().strftime("%Y-%m-%d")
    end = signals["date"].max().strftime("%Y-%m-%d")
    source = QlibPriceSource(provider_uri=os.path.expanduser(args.qlib_dir),
                             start=start, end=end, benchmark=args.benchmark)

    outcomes, stats = collect_outcomes(signals, source, max_defer=args.max_defer)
    report = render_report(aggregate(outcomes), stats, _meta(args, len(signals)))
    out_path = _write_report(args.out, report)
    sys.stdout.write(f"报告已写入 {out_path}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
