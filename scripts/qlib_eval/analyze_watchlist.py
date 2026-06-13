#!/usr/bin/env python3
"""Watchlist analytics over the atlas-built qlib bundle.

Loads $close for every instrument in the bundle and produces a markdown report:
per-instrument return / annualized vol / max drawdown, plus a daily-return
correlation matrix across the A-share indexes. Reads only the local qlib data
directory — no network, no collectors.

Usage:
  python analyze_watchlist.py --qlib-dir ~/.qlib/qlib_data/atlas_cn --out reports/watchlist-analysis.md
"""
import argparse
import os

import numpy as np
import pandas as pd

TRADING_DAYS = 244  # A-share approximate annualization base


def is_index(c: str) -> bool:
    """指数 instrument 判定：A股 SH/SZ 的 000/399 段、中证 CSI 前缀、港股 HSI/HSCEI。

    港股证券 instrument 形如 HK00001/HK00700，其 c[2:5] 恰为 "000"，旧逻辑会把
    它们误判为指数。港股指数命名为 HSI/HSCEI(不带 HK 前缀)，故 HK 前缀的一律是证券。
    """
    if c.startswith("HK"):
        return False
    return c[2:5] in ("000", "399") or c.startswith("CSI") or c in ("HSI", "HSCEI")


def perf_row(close: pd.Series) -> dict:
    close = close.dropna()
    if len(close) < 2:
        return {}
    rets = close.pct_change().dropna()
    total = close.iloc[-1] / close.iloc[0] - 1
    years = len(close) / TRADING_DAYS
    cagr = (close.iloc[-1] / close.iloc[0]) ** (1 / years) - 1 if years > 0 else float("nan")
    ann_vol = rets.std() * np.sqrt(TRADING_DAYS)
    cummax = close.cummax()
    max_dd = ((close - cummax) / cummax).min()
    return {
        "bars": len(close),
        "first": close.iloc[0],
        "last": close.iloc[-1],
        "total_return": total,
        "cagr": cagr,
        "ann_vol": ann_vol,
        "max_drawdown": max_dd,
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--qlib-dir", default=os.path.expanduser("~/.qlib/qlib_data/atlas_cn"))
    ap.add_argument("--out", default="reports/watchlist-analysis.md")
    args = ap.parse_args()

    qlib_dir = os.path.expanduser(args.qlib_dir)
    if not os.path.isdir(qlib_dir):
        print(f"qlib bundle not found: {qlib_dir} — run `make qlib-data` first")
        return 1

    import qlib
    from qlib.data import D

    qlib.init(provider_uri=qlib_dir, region="cn")
    codes = D.list_instruments(D.instruments(market="all"), as_list=True)
    codes = sorted(codes)

    feat = D.features(codes, ["$close"], freq="day")
    closes = feat["$close"].unstack(level="instrument")  # index=datetime, cols=instrument
    closes = closes.sort_index()

    rows = {}
    for c in codes:
        r = perf_row(closes[c])
        if r:
            rows[c] = r
    perf = pd.DataFrame(rows).T

    idx_codes = [c for c in codes if is_index(c)]
    corr = closes[idx_codes].pct_change().corr() if len(idx_codes) >= 2 else pd.DataFrame()

    start, end = closes.index.min().date(), closes.index.max().date()
    lines = [
        "# Watchlist qlib 分析报告",
        "",
        f"数据源: `{qlib_dir}` | 区间: {start} → {end} | instruments: {len(codes)}",
        "",
        "## 个股/指数表现",
        "",
        "| instrument | bars | 期初 | 期末 | 累计收益 | 年化(CAGR) | 年化波动 | 最大回撤 |",
        "|---|--:|--:|--:|--:|--:|--:|--:|",
    ]
    for c in perf.index:
        r = perf.loc[c]
        lines.append(
            f"| {c} | {int(r['bars'])} | {r['first']:.2f} | {r['last']:.2f} | "
            f"{r['total_return']*100:.1f}% | {r['cagr']*100:.1f}% | "
            f"{r['ann_vol']*100:.1f}% | {r['max_drawdown']*100:.1f}% |"
        )

    if not corr.empty:
        lines += ["", "## 指数日收益相关性", "", "| | " + " | ".join(idx_codes) + " |",
                  "|---|" + "---|" * len(idx_codes)]
        for a in idx_codes:
            cells = " | ".join(f"{corr.loc[a, b]:.2f}" for b in idx_codes)
            lines.append(f"| {a} | {cells} |")

    os.makedirs(os.path.dirname(args.out) or ".", exist_ok=True)
    with open(args.out, "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"报告已写入 {args.out} ({len(codes)} instruments)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
