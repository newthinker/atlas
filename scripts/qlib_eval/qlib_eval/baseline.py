"""baseline 因子生成 —— harness 自验证用，非上线策略。

oracle：分数=未来收益本身 → IC≈1，证明相关计算端到端正确。
reversal：分数=负的过去收益（超卖反弹）→ 真 bundle 上 IC 量级合理，证明真数据通路。

顶层不得 import qlib：load_prices 在函数体内惰性导入 QlibPriceSource（其内部再惰性 import qlib）。
"""

import pandas as pd


def reversal_scores(prices: dict[str, pd.DataFrame], lookback: int = 5) -> pd.DataFrame:
    """短期反转因子：score_t = -(close_t / close_{t-lookback} - 1)。越超卖分越高。"""
    rows = []
    for symbol, df in prices.items():
        c = df["close"]
        score = -(c / c.shift(lookback) - 1.0)
        for d, v in score.dropna().items():
            rows.append({"date": d, "symbol": symbol, "score": float(v)})
    return pd.DataFrame(rows, columns=["date", "symbol", "score"])


def oracle_scores(fwd: pd.DataFrame, horizon: int) -> pd.DataFrame:
    """oracle 因子：分数=该 horizon 的前向收益本身（无噪声 → IC=1）。"""
    f = fwd[fwd["horizon"] == horizon][["date", "symbol", "ret"]].copy()
    return f.rename(columns={"ret": "score"})


def load_prices(qlib_dir: str, symbols, start: str, end: str,
                region: str = "cn") -> dict[str, pd.DataFrame]:
    """从 qlib bundle 取每个 symbol 的 open/close（惰性 import qlib）。"""
    from .prices import QlibPriceSource  # 惰性：触发 prices 内的惰性 qlib import

    source = QlibPriceSource(provider_uri=qlib_dir, start=start, end=end, region=region)
    out = {}
    for sym in symbols:
        try:
            out[sym] = source.history(sym)
        except Exception:  # noqa: BLE001 — 单 symbol 取价失败不中断整体
            continue
    return out
