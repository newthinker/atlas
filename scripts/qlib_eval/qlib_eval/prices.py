"""价格数据源抽象 + 入场对齐。

`PriceSource` 是评估层与价格来源之间的注入点：pytest 注入合成 DataFrame，真实运行
注入 `QlibPriceSource`。后者**在方法体内惰性 import qlib**——这是 pytest 零 qlib
依赖的机制保证（守门测试 `test_no_qlib_at_module_level` 锁死，本模块顶层不得 import qlib）。
"""

from dataclasses import dataclass
from typing import Protocol

import pandas as pd


@dataclass
class Entry:
    """信号对应的入场点。"""

    date: pd.Timestamp
    price: float  # 入场日开盘价（next-day open）
    index: int  # 入场行在价格 frame 中的 positional index，供 horizon 计算复用


class PriceSource(Protocol):
    def history(self, symbol: str) -> pd.DataFrame:
        """index=交易日 DatetimeIndex，columns 含 open/close；symbol 为 atlas 形式（600519.SH）。"""
        ...

    def benchmark(self) -> pd.DataFrame:
        """基准 instrument 的 open/close 序列（A股 SH000300 / 港股 HSI 等）。"""
        ...


def align_entry(
    prices: pd.DataFrame, signal_date: pd.Timestamp, max_defer: int
) -> Entry | None:
    """入场对齐到 signal_date 之后【严格次日】的首个交易日开盘（next-day open，规避前视）。

    返回 None 的两种情形：
    - signal_date 之后再无 bar（searchsorted 越界）；
    - signal_date 与入场 bar 的间隔 ``> max_defer*2`` 个【日历日】（"顺延超过 max_defer
      个交易日" 的日历日近似：5 个交易日 ≈ 7-10 个日历日，取 ``*2`` 上界）。
      比较符为**严格大于**——``== max_defer*2`` 边界保留入场（DoD 双侧用例钉死）。

    丢弃与计数由调用方负责。
    """
    pos = prices.index.searchsorted(signal_date, side="right")
    if pos >= len(prices.index):
        return None
    entry_date = prices.index[pos]
    if (entry_date - signal_date).days > max_defer * 2:
        return None
    return Entry(
        date=entry_date,
        price=float(prices.iloc[pos]["open"]),
        index=int(pos),
    )


class QlibPriceSource:
    """真实运行用的价格数据源；惰性 import qlib，pytest 全程不触达。"""

    def __init__(self, provider_uri: str, start: str, end: str, benchmark: str = "000300.SH", region: str = "cn"):
        self._provider_uri = provider_uri
        self._start = start
        self._end = end
        self._benchmark = benchmark  # atlas 形式（000300.SH / ^HSI），benchmark() 内转 qlib instrument
        self._region = region  # qlib region：cn（A股/港股）/ us（美股）
        self._initialized = False

    def _ensure_init(self) -> None:
        if self._initialized:
            return
        import qlib  # 惰性 import：仅真实运行时触发

        qlib.init(provider_uri=self._provider_uri, region=self._region)
        self._initialized = True

    def history(self, symbol: str) -> pd.DataFrame:
        self._ensure_init()
        from qlib.data import D  # 惰性 import

        from .symbols import to_qlib_instrument

        df = D.features(
            [to_qlib_instrument(symbol)],
            ["$open", "$close"],
            start_time=self._start,
            end_time=self._end,
        )
        return self._normalize(df)

    def benchmark(self) -> pd.DataFrame:
        self._ensure_init()
        from qlib.data import D  # 惰性 import

        from .symbols import to_qlib_instrument

        df = D.features(
            [to_qlib_instrument(self._benchmark)],
            ["$open", "$close"],
            start_time=self._start,
            end_time=self._end,
        )
        return self._normalize(df)

    @staticmethod
    def _normalize(df: pd.DataFrame) -> pd.DataFrame:
        """qlib 返回 (instrument, datetime) MultiIndex + $open/$close 列 →
        标准化为 datetime 单层 index、open/close 列的 DataFrame。"""
        out = df.copy()
        out.columns = [c.lstrip("$") for c in out.columns]
        # 去掉 instrument 维度，仅保留 datetime
        out = out.reset_index(level=0, drop=True)
        out.index = pd.DatetimeIndex(out.index)
        return out[["open", "close"]]
