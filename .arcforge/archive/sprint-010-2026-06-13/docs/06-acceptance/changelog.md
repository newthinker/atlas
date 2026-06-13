# Changelog — atlas_us signal-eval

## [unreleased] 2026-06-13 — 美股 signal-eval 全管线

### Added
- **Go export-ohlcv 美股支持**（cmd/atlas/export_ohlcv.go）
  - `toQlibInstrument`：US 裸 ticker（`^[A-Z]{1,5}$` 恒等）+ 指数（`^GSPC/^IXIC/^DJI` 剥 ^）。
  - 包级 var `usTickerRe`（单一真相源，inMarket 复用）。
  - `benchmarkForMarket("us")` → `^GSPC`；`inMarket` us 分支；`--market us` 校验放开 + flag help。
- **Python 评估端美股对称**（scripts/qlib_eval/）
  - `symbols.py to_qlib_instrument`：US 镜像（`re.fullmatch(r"[A-Z]{1,5}")`，与 Go 等价）。
  - `prices.py QlibPriceSource(region=...)` + `evaluate.py --region`（默认 cn，us 显式）。
- **编排**（Makefile）：`qlib-data-us`、`signal-eval-us` target（镜像 hk，走 venv python）。
- **配置**（configs/config.yaml，本地）：美股 watchlist 8 个股 + ^GSPC。

### Changed
- `toQlibInstrument`/`to_qlib_instrument` error 文案：`A-share/HK` → `A-share/HK/US`。
- 契约测试：`AAPL`/`^GSPC` 由 reject 迁 accept；补锚定负向样本（含 6 字符 off-by-one）。
- `TestRunExportOHLCV_RejectsUnknownMarket`：移除 `us`（已成合法市场）。
- `test_report.py` fixture：`AAPL` → `GC=F`（AAPL 合法后改用真正不可映射符号，测试意图不变）。
- benchmarkForMarket/inMarket：`if market=="hk"` 单分支重构为 `switch`（3 市场分派）。

### Unchanged（零回归）
- CN/HK 全部路径：benchmarkForMarket cn→000300.SH/hk→^HSI；region 默认 cn；
  既有契约/测试全绿（Go + 63 Python tests）。

### Verified
- 端到端 region=us 真实跑通：atlas_us 包 9 instruments + 非空事件研究报告（5/20/60 相对 ^GSPC 超额）。
- QA 两轮 PASS（33 样本跨语言差分对称，无 CRITICAL/WARNING）。
