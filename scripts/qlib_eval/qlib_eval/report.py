"""CSV 信号读取 + markdown 报告生成。纯标准库 + pandas，无 qlib 依赖。"""

import csv

import pandas as pd

SIGNAL_COLUMNS = [
    "symbol", "date", "strategy", "action", "confidence", "price", "metadata"
]


def read_signals(path: str) -> pd.DataFrame:
    """读取 export-signals 产出的 7 列 CSV，严格校验 schema。

    - 表头必须恰为 SIGNAL_COLUMNS；
    - 每行列数必须为 7，date 解析为 Timestamp、confidence/price 解析为 float，
      metadata **保留原串**（不反序列化）；
    - 缺列 / 坏行 → ValueError，消息含**行号**（含表头的 1-based 物理行号）。

    encoding 用 ``utf-8-sig`` 以容忍 Excel 导出的 UTF-8 BOM（否则首列名被 BOM 污染）。
    """
    with open(path, newline="", encoding="utf-8-sig") as f:
        reader = csv.reader(f)
        try:
            header = next(reader)
        except StopIteration as e:
            raise ValueError("signals CSV is empty") from e
        if header != SIGNAL_COLUMNS:
            raise ValueError(
                f"signals CSV header mismatch: expected {SIGNAL_COLUMNS}, got {header}"
            )
        rows = []
        for lineno, raw in enumerate(reader, start=2):
            if not raw:  # 跳过完全空行
                continue
            if len(raw) != len(SIGNAL_COLUMNS):
                raise ValueError(
                    f"signals CSV line {lineno}: expected {len(SIGNAL_COLUMNS)} "
                    f"columns, got {len(raw)}"
                )
            rec = dict(zip(SIGNAL_COLUMNS, raw))
            try:
                rec["date"] = pd.Timestamp(rec["date"])
                rec["confidence"] = float(rec["confidence"])
                rec["price"] = float(rec["price"])
            except (ValueError, TypeError) as e:
                raise ValueError(f"signals CSV line {lineno}: {e}") from e
            rows.append(rec)
    return pd.DataFrame(rows, columns=SIGNAL_COLUMNS)


def _strategy_table(rows: pd.DataFrame) -> str:
    """单策略的 markdown 表格（按 action/conf_bucket/horizon 排序）。"""
    cols = ["action", "conf_bucket", "horizon", "n", "mean_ret", "median_ret",
            "mean_excess", "win_rate"]
    lines = ["| " + " | ".join(cols) + " |",
             "|" + "|".join(["---"] * len(cols)) + "|"]
    for _, r in rows.iterrows():
        lines.append(
            "| {action} | {cb:.1f} | {h} | {n} | {mr:.4f} | {md:.4f} | "
            "{me:.4f} | {wr:.2%} |".format(
                action=r["action"], cb=r["conf_bucket"], h=int(r["horizon"]),
                n=int(r["n"]), mr=r["mean_ret"], md=r["median_ret"],
                me=r["mean_excess"], wr=r["win_rate"],
            )
        )
    return "\n".join(lines)


def render_report(agg: pd.DataFrame, stats: dict, meta: dict) -> str:
    """渲染评估报告 markdown 字符串。

    agg：``aggregate`` 输出（strategy/action/conf_bucket/horizon/n/mean_ret/
    median_ret/mean_excess/win_rate）。
    stats：``{"dropped", "data_gaps", "non_ashare": [...], "na_counts": {h: n}}``。
    meta：``{"generated_at", "n_signals", "benchmark", "qlib_dir"}``。
    """
    benchmark = meta.get("benchmark", "000300.SH")
    parts = [
        "# 信号事件研究报告",
        "",
        f"- 生成时间: {meta.get('generated_at', '')}",
        f"- 信号总数: {meta.get('n_signals', '')}",
        f"- 基准: {benchmark}",
        f"- qlib 数据目录: {meta.get('qlib_dir', '')}",
        "",
        "## 评估口径",
        "- 入场: 信号次日开盘（规避前视）。",
        "- horizon: 5 / 20 / 60 个交易日。",
        f"- 超额收益: 相对基准 {benchmark}；sell/strong_sell 取规避口径 `-(ret - bench_ret)`。",
        "- 置信度桶: ≥0.0 / ≥0.6 / ≥0.8（累积阈值，非互斥区间）。",
        "- 入场顺延上限: 信号日与入场 bar 间隔 > max_defer*2 个日历日则丢弃。",
        "- 基准对齐: 取基准中 ≤ 目标日期的最近前值（个股停牌时）。",
        "",
        "## 数据缺口",
        f"- 丢弃（无入场 bar / 顺延过久 / 入场早于基准）: {stats.get('dropped', 0)}",
        f"- 数据缺口（价格/基准缺失）: {stats.get('data_gaps', 0)}",
    ]
    if stats.get("benchmark_error"):
        parts.append(
            f"- ⚠ 基准 {benchmark} 数据缺失，全部超额收益无法计算: "
            f"{stats['benchmark_error']}"
        )
    non_ashare = stats.get("non_ashare") or []
    parts.append(f"- 非 A 股符号（Phase 1 跳过，未评估）: {', '.join(non_ashare) or '无'}")
    na_counts = stats.get("na_counts") or {}
    if na_counts:
        na_desc = ", ".join(f"horizon {h}: {n}" for h, n in sorted(na_counts.items()))
        parts.append(f"- horizon 越界（NA）: {na_desc}")
    parts.append("")

    if agg.empty:
        parts.append("## 策略结果")
        parts.append("（无可聚合的有效信号）")
    else:
        for strategy in sorted(agg["strategy"].unique()):
            parts.append(f"## 策略: {strategy}")
            parts.append(_strategy_table(agg[agg["strategy"] == strategy]))
            parts.append("")

    return "\n".join(parts).rstrip() + "\n"
