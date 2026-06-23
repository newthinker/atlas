#!/usr/bin/env python
"""时序 IC 评估 CLI 入口。

读分数面板 CSV → 从 qlib bundle 取价 → 逐标的时序 IC → markdown 报告。
缺 qlib 目录打印下载指引 exit(1)；空面板写空报告 exit(0)。

顶层不 import qlib——取价经 QlibPriceSource 惰性触发(pytest 不触达)。
"""

import argparse
import datetime as _dt
import os
import sys

import pandas as pd

from evaluate import check_qlib_dir, get_data_hint
from qlib_eval.ic import (
    HORIZONS,
    forward_returns,
    ic_summary_by_instrument,
    watchlist_summary,
)
from qlib_eval.report import read_scores, render_ic_report

DEFAULT_QLIB_DIR = "~/.qlib/qlib_data/atlas_cn"
DEFAULT_OUT = "reports/"


def collect_ic(scores, source, horizons=HORIZONS, method="spearman", min_periods=60):
    """取价 → 前向收益 → 逐 horizon IC 汇总。返回 (per_horizon, stats)。"""
    prices, data_gaps = {}, 0
    for symbol in scores["symbol"].unique():
        try:
            prices[symbol] = source.history(symbol)
        except Exception:  # noqa: BLE001 — 单 symbol 取价失败不中断
            data_gaps += 1
    fwd = forward_returns(prices, horizons=horizons)
    per_horizon = {}
    for h in horizons:
        by = ic_summary_by_instrument(scores, fwd, h, method, min_periods)
        per_horizon[h] = {"by_instrument": by, "summary": watchlist_summary(by)}
    stats = {"n_symbols": len(prices), "data_gaps": data_gaps}
    return per_horizon, stats


def _parse_args(argv):
    p = argparse.ArgumentParser(description="信号时序 IC 评估")
    p.add_argument("--scores", required=True, help="分数面板 CSV(date,symbol,score)")
    p.add_argument("--qlib-dir", default=DEFAULT_QLIB_DIR, help="qlib 数据目录")
    p.add_argument("--out", default=DEFAULT_OUT, help="报告输出目录")
    p.add_argument("--method", default="spearman", help="相关方法 spearman/pearson")
    p.add_argument("--min-periods", type=int, default=60, help="单标的最小时序样本")
    p.add_argument("--region", default="cn", help="qlib region：cn/us")
    return p.parse_args(argv)


def _write_report(out_dir, report):
    os.makedirs(out_dir, exist_ok=True)
    out_path = os.path.join(out_dir, f"signal-ic-{_dt.date.today():%Y%m%d}.md")
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(report)
    return out_path


def _meta(args, n_scores):
    return {"generated_at": _dt.date.today().isoformat(), "n_scores": n_scores,
            "method": args.method, "qlib_dir": args.qlib_dir}


def main(argv=None) -> int:
    args = _parse_args(argv)
    if not check_qlib_dir(args.qlib_dir):
        sys.stderr.write(get_data_hint(args.qlib_dir))
        return 1
    scores = read_scores(args.scores)
    if scores.empty:
        out_path = _write_report(args.out, render_ic_report({}, _meta(args, 0)))
        sys.stdout.write(f"无分数可评估，已写入空报告 {out_path}\n")
        return 0

    from qlib_eval.prices import QlibPriceSource  # 惰性
    start = scores["date"].min().strftime("%Y-%m-%d")
    end = scores["date"].max().strftime("%Y-%m-%d")
    source = QlibPriceSource(provider_uri=os.path.expanduser(args.qlib_dir),
                             start=start, end=end, region=args.region)
    per_horizon, _stats = collect_ic(scores, source, method=args.method,
                                     min_periods=args.min_periods)
    out_path = _write_report(args.out, render_ic_report(per_horizon, _meta(args, len(scores))))
    sys.stdout.write(f"报告已写入 {out_path}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
