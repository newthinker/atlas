# Changelog — signal-eval 基准参数化（支持港股）（2026-06-13）

## Changed
- **signal-eval 基准参数化**：`evaluate.py` 加 `--benchmark`（atlas 形式，默认 `000300.SH`）；
  `QlibPriceSource.__init__` 加 `benchmark` 参数，`benchmark()` 经 `to_qlib_instrument` 解析
  （替代硬编码 `["SH000300"]`）；`_meta` 与 `report.py` 基准随参数变。
- **Makefile**：`signal-eval` 传 `--benchmark $(SIGNAL_BENCHMARK)`（默认 `000300.SH`）；
  新增 `signal-eval-hk` target（港股集 → atlas_hk → 基准 `^HSI`）。

## Added
- `signal-eval-hk` Makefile target：港股事件研究（export-signals 港股集 + evaluate atlas_hk/^HSI）。

## Tests
- prices.py：QlibPriceSource benchmark 参数 + **注入 fake qlib 断言 benchmark() 实际请求转换后 instrument**（^HSI→HSI / 默认→SH000300）。
- report.py/evaluate.py：`--benchmark` 解析、`_meta` 透传、render 基准文案断言。pytest 58 passed。

## Notes
- 港股 signal-eval 验证：丢弃 3（修复前 8129/8129 全丢弃），超额收益相对恒生可算。
- A股默认 `000300.SH` 零回归。
- 美股推迟另开一轮（参数化已就绪，未建 atlas_us）。
