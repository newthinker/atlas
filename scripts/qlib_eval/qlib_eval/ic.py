"""时序 IC 计算核心（逐标的）。

口径：score_t 由 close(t) 算 → 次日开盘入场（复用 align_entry，规避前视）→ h 交易日后
收盘出场。单标的在时间轴上累积 (score_t, 前向收益_t) 配对，算两序列的相关 = 时序 IC。

全部纯 pandas，无 qlib 依赖（顶层不得 import qlib）。
"""

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
