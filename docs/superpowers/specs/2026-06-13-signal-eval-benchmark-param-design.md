# signal-eval 基准参数化（支持港股，美股就绪）设计

**日期**: 2026-06-13 | **状态**: 设计定稿，待实现
**动机**: signal-eval 事件研究的基准硬编码为 `SH000300`（`prices.py` QlibPriceSource.benchmark），
导致对 atlas_hk 评估时基准缺失、全部信号被丢弃（实测 8129/8129 丢弃）。本轮把基准参数化，
使 signal-eval 支持港股（atlas_hk + `^HSI`）；美股（atlas_us）推迟另开一轮，但参数化天然为其就绪。
**前置**: atlas_hk 数据包已交付（PR #28），港股取数/分析已验证正常。

## 目标
给 signal-eval 加 `--benchmark` 参数（atlas 形式），使事件研究可对任意 market 的 qlib 包评估；
立即启用港股（atlas_hk，基准 `^HSI`）；A股路径默认 `000300.SH` 零回归。

## 范围与非目标
- **范围**：仅基准参数化 + 港股验证 + Makefile signal-eval-hk target。
- **非目标（美股另开一轮）**：不建 atlas_us 数据包、不加美股 watchlist 标的、不实现
  export-ohlcv --market us。参数化使「一旦有 atlas_us 包即可用」，但本轮不建包。

## 方式
benchmark 参数用 **atlas 形式**（`000300.SH`/`^HSI`，与 export-ohlcv `--symbols` 一致），
`QlibPriceSource` 内部经 `to_qlib_instrument` 转 qlib 形式（`SH000300`/`HSI`）。默认 `000300.SH`。
（备选「直接传 qlib 形式」省一次转换但与 CLI atlas 形式约定不一致，不采用。）

## 组件

| 文件 | 改动 |
|---|---|
| `scripts/qlib_eval/qlib_eval/prices.py` | `QlibPriceSource.__init__` 加 `benchmark: str = "000300.SH"` 参数并存字段；`benchmark()` 用 `to_qlib_instrument(self._benchmark)` 替代硬编码 `["SH000300"]`；`PriceSource` Protocol 的 `benchmark()` docstring 由「SH000300 的 close 序列」泛化为「基准 instrument 的 close 序列」 |
| `scripts/qlib_eval/evaluate.py` | `_parse_args` 加 `--benchmark`（默认 `000300.SH`，help 注明 atlas 形式如 `^HSI`）；`main` 构造 `QlibPriceSource(..., benchmark=args.benchmark)`；`_meta` 的 `"benchmark"` 由硬编码 `SH000300` 改为 `args.benchmark` |
| `Makefile` | 加 `SIGNAL_BENCHMARK ?= 000300.SH`；`signal-eval` 的 evaluate 调用追加 `--benchmark $(SIGNAL_BENCHMARK)`；新增 `.PHONY` 项与 `signal-eval-hk` target：export-signals(港股集 $(SIGNAL_SYMBOLS_HK)，策略 price_percentile,ma_crossover) → evaluate --qlib-dir $(QLIB_DATA_HK_DIR) --benchmark ^HSI |

## 数据流（港股 signal-eval-hk）
```
export-signals --config configs/config.yaml --symbols $(SIGNAL_SYMBOLS_HK) \
  --strategies price_percentile,ma_crossover --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals_hk.csv
  → 经 yahoo 取港股 OHLCV，回放策略，产出信号
evaluate.py --signals signals_hk.csv --qlib-dir $(QLIB_DATA_HK_DIR) --benchmark ^HSI --out reports/
  → QlibPriceSource(benchmark="^HSI").benchmark() → to_qlib_instrument(^HSI)=HSI → 取恒生 close
  → 事件研究：超额收益相对恒生指数 → markdown 报告
```

## 错误处理 / 边界
- 默认 `000300.SH` → 既有 A股 signal-eval 完全不回归。
- 基准取数失败：沿用既有 `collect_outcomes` 的 try/except → `stats["benchmark_error"]` 优雅降级，不崩溃。
- benchmark 传入 `to_qlib_instrument` 不支持的符号（如 `^GSPC`，本轮未支持）→ benchmark() 内 ValueError →
  被既有 try/except 捕获为 benchmark_error（清晰提示，不崩溃）。
- 消费侧 `collect_outcomes` 已 benchmark-agnostic（调 `source.benchmark()`），无需改。

## 测试
- **prices.py**：`QlibPriceSource(benchmark="^HSI")` 的 `benchmark()` 向 D.features 请求 instrument `"HSI"`
  （mock D.features 断言传入 instrument）；默认构造请求 `"SH000300"`。
- **evaluate.py**：`--benchmark` 解析；`_meta` 反映传入基准；缺省为 `000300.SH`。
- **回归**：既有 `test_prices.py` / `test_report.py`（含 benchmark_error 降级、_meta benchmark）不回归。
- **集成（港股，本环境 yahoo 可达）**：`signal-eval-hk` → 非空事件研究报告（基准 = 恒生，超额收益可算、
  非「全部丢弃」）。

## 验收口径
signal-eval 对 atlas_hk + `--benchmark ^HSI` 产出**非空**事件研究（超额收益相对恒生）；A股默认路径零回归；
全量 Go/Python 测试无新增失败。
