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


def collect_outcomes(signals: pd.DataFrame, source, max_defer: int = 5):
    """对每个信号求 outcome。返回 ``(outcomes, stats)``。

    - 非 A 股符号（``to_qlib_instrument`` raise ValueError）不中断评估，收集进
      ``stats["non_ashare"]``；
    - ``evaluate_signal`` 返回 None（无入场 / 入场早于基准）计入 ``stats["dropped"]``；
    - ``stats["data_gaps"]`` 计 source 取价失败的符号数。
    """
    bench = source.benchmark()
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
    return p.parse_args(argv)


def main(argv=None) -> int:
    args = _parse_args(argv)

    if not check_qlib_dir(args.qlib_dir):
        sys.stderr.write(get_data_hint(args.qlib_dir))
        return 1

    signals = read_signals(args.signals)

    # 真实运行：此处才惰性构造 qlib 数据源（pytest 不触达）
    from qlib_eval.prices import QlibPriceSource

    start = signals["date"].min().strftime("%Y-%m-%d")
    end = signals["date"].max().strftime("%Y-%m-%d")
    source = QlibPriceSource(provider_uri=os.path.expanduser(args.qlib_dir),
                             start=start, end=end)

    outcomes, stats = collect_outcomes(signals, source, max_defer=args.max_defer)
    agg = aggregate(outcomes)

    today = _dt.date.today()
    meta = {
        "generated_at": today.isoformat(),
        "n_signals": len(signals),
        "benchmark": "SH000300",
        "qlib_dir": args.qlib_dir,
    }
    report = render_report(agg, stats, meta)

    os.makedirs(args.out, exist_ok=True)
    out_path = os.path.join(args.out, f"signal-eval-{today:%Y%m%d}.md")
    with open(out_path, "w") as f:
        f.write(report)
    sys.stdout.write(f"报告已写入 {out_path}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
