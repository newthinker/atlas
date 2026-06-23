"""时序 IC 计算核心（逐标的）。

口径：score_t 由 close(t) 算 → 次日开盘入场（复用 align_entry，规避前视）→ h 交易日后
收盘出场。单标的在时间轴上累积 (score_t, 前向收益_t) 配对，算两序列的相关 = 时序 IC。

全部纯 pandas，无 qlib 依赖（顶层不得 import qlib）。
"""

import math

import pandas as pd

from .prices import align_entry

HORIZONS = (5, 20, 60)


def forward_returns(
    prices: dict[str, pd.DataFrame], horizons=HORIZONS, max_defer: int = 5
) -> pd.DataFrame:
    """每 symbol 每交易日的 next-open 前向收益。

    对每个 score 日 t（=价格 frame 的交易日）：入场=align_entry(df, t) 的次日开盘；
    horizon h 的出场行 = 入场行 + h（positional）。ret = close[出场]/open[入场] - 1。
    无次日 bar（align_entry None）或出场越界 → 该 (t,h) 不产行。

    返回长格式列 ["date","symbol","horizon","ret"]。
    """
    rows = []
    for symbol, df in prices.items():
        n_bars = len(df.index)
        for t in df.index:
            entry = align_entry(df, t, max_defer)
            if entry is None:
                continue
            for h in horizons:
                exit_idx = entry.index + h
                if exit_idx >= n_bars:
                    continue
                ret = float(df.iloc[exit_idx]["close"]) / entry.price - 1.0
                rows.append({"date": t, "symbol": symbol, "horizon": h, "ret": ret})
    return pd.DataFrame(rows, columns=["date", "symbol", "horizon", "ret"])


def instrument_ic(
    scores: pd.DataFrame, fwd: pd.DataFrame, symbol: str, h: int,
    method: str = "spearman", min_periods: int = 60,
) -> dict | None:
    """单标的时序 IC。

    取该 symbol 的 (date,score) 与 (date,ret@horizon=h) 按 date 内连接，dropna 后：
    - n = 有效配对数；< min_periods → None（该标的不参与汇总）。
    - ic = score 与 ret 的相关（method: 'pearson'|'spearman'）。
    - t_stat = ic * sqrt(n)（设计文档钉死的小 IC 近似）。
    - t_stat_nonoverlap = 非重叠采样（按 date 排序每 h 行取一点）的 ic * sqrt(n_no)；
      非重叠样本 < 2 → None。

    ic 计算无定义（如常数序列 corr → NaN）时 ic/t_stat 返回 None，不抛异常。
    """
    s = scores[scores["symbol"] == symbol][["date", "score"]]
    f = fwd[(fwd["symbol"] == symbol) & (fwd["horizon"] == h)][["date", "ret"]]
    merged = s.merge(f, on="date", how="inner").dropna(subset=["score", "ret"])
    n = len(merged)
    if n < min_periods:
        return None

    def t_stat_of(sample: pd.DataFrame) -> float | None:
        """sample 的 score/ret 相关 * sqrt(n)；样本无 corr 定义(NaN) → None。"""
        ic = sample["score"].corr(sample["ret"], method=method)
        return float(ic * math.sqrt(len(sample))) if pd.notna(ic) else None

    ic = merged["score"].corr(merged["ret"], method=method)
    nonov = merged.sort_values("date").iloc[::h]

    return {
        "ic": float(ic) if pd.notna(ic) else None,
        "n_periods": n,
        "t_stat": t_stat_of(merged),
        "t_stat_nonoverlap": t_stat_of(nonov) if len(nonov) >= 2 else None,
    }


def ic_summary_by_instrument(
    scores: pd.DataFrame, fwd: pd.DataFrame, h: int,
    method: str = "spearman", min_periods: int = 60,
) -> pd.DataFrame:
    """每标的一行的时序 IC 汇总；样本不足的标的被剔除，按 symbol 排序。

    列恰为 ["symbol","ic","n_periods","t_stat","t_stat_nonoverlap"]。
    """
    cols = ["symbol", "ic", "n_periods", "t_stat", "t_stat_nonoverlap"]
    rows = []
    for symbol in sorted(scores["symbol"].unique()):
        res = instrument_ic(scores, fwd, symbol, h, method, min_periods)
        if res is None:
            continue
        rows.append({"symbol": symbol, **res})
    return pd.DataFrame(rows, columns=cols)


def watchlist_summary(per_inst: pd.DataFrame) -> dict:
    """跨标的聚合：mean/median IC、ICIR（标的间 mean/std）、正 IC 广度。

    - 仅统计 ic 非 NaN 的标的；空 → 全字段 None、n_instruments=0。
    - icir = ics.mean() / ics.std(ddof=1)；n<2 或 std 为 0/NaN → icir=None（不抛）。
    """
    ics = per_inst["ic"].dropna()
    n = int(len(ics))
    if n == 0:
        return {"mean_ic": None, "median_ic": None, "icir": None,
                "positive_breadth": None, "n_instruments": 0}
    std = ics.std(ddof=1) if n >= 2 else None
    # std 为 None(n<2) / NaN / 0(含浮点残差近零) → ICIR 不可计算，返回 None 不抛。
    icir = (float(ics.mean() / std)
            if std is not None and pd.notna(std) and not math.isclose(std, 0.0, abs_tol=1e-12)
            else None)
    return {
        "mean_ic": float(ics.mean()),
        "median_ic": float(ics.median()),
        "icir": icir,
        "positive_breadth": float((ics > 0).sum() / n),
        "n_instruments": n,
    }
