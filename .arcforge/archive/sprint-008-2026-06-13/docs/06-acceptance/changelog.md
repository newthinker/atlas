# Changelog — 港股 qlib 数据包扩展（atlas_hk）（2026-06-13）

## Added
- **港股 qlib 数据包 atlas_hk**：独立于 atlas_cn（HK 交易日历），经 `make qlib-data-hk` 从
  watchlist 港股集采集 yahoo OHLCV → dump_bin 构建。
- **export-ohlcv `--market cn|hk`**：按 market 选择 watchlist 子集与基准（cn=000300.SH、
  hk=^HSI），CLI 校验 market ∈ {cn,hk}。
- **HK 命名契约**（Go `toQlibInstrument` + Python `to_qlib_instrument` 对称）：`.HK`→`HK#####`
  （5 位补零）、`^HSI`→`HSI`、`^HSCE`→`HSCEI`；`^HSTECH` 拒绝（yahoo 无数据）。
- **watchlist 新增**：港股基金 2800/2828/3033/3181.HK（ETF）+ 港股指数 ^HSI/^HSCE，均 price_percentile。
- **Makefile `qlib-data-hk`** target → atlas_hk。
- `analyze_watchlist.py` 指数识别泛化为 A股(000/399) + 中证(CSI) + 港股(HSI/HSCEI)。

## Changed
- `export_ohlcv.go`：`benchmarkForMarket`/`inMarket` 助手；`resolveOHLCVSymbols`/`requireBenchmark`
  加 market 参数；`exportOHLCVParams.Benchmark`；`executeExportOHLCV` 用 p.Benchmark（非硬编码）。
- `selector.go`：`indexMarkets` 加 `^HSCE: MarketHK`（与 ^HSI 一致）。
- `analyze_watchlist.py`：qlib 改 `main()` 内惰性导入（对齐项目「no qlib at module level」约定）。

## Tests
- Go：toQlibInstrument HK 契约、resolveOHLCVSymbols HK 选择、benchmarkForMarket、
  hk 基准 fatal、--market 非法值拒绝、MarketForSymbol(^HSCE)==HK。
- Python：to_qlib_instrument HK 契约、analyze is_index HK 守门。pytest 52 passed。

## Notes
- 行情全部走 yahoo（.HK + ^HSI/^HSCE 路由经 SelectForSymbol 已就绪，无需改 selector 路由）。
- A股 atlas_cn 路径零回归（默认 market=cn）。
