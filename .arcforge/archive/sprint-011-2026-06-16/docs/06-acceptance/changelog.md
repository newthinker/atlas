# Changelog — Qlib 数据仓库 第一期

## Added
- **本地 SQLite 历史行情仓库**：Python dump 管线（`scripts/qlib_warehouse/`，仅 stdlib）从 `qlib_csv_*` per-instrument CSV 归一化、原子写入统一 SQLite（`ohlcv` + `warehouse_meta`，`fundamentals_pit` 建空留第二期）。
  - `schema.py` DDL、`ingest.py`（`adj_close=close*factor`，symbol 大写，空字段→NULL）、`writer.py`（临时库 + `os.replace` 原子覆盖）、`build_warehouse.py` CLI。
- **Makefile `warehouse-dump` target**：`make warehouse-dump` 从 `qlib_csv_us` 生成 `data/qlib_warehouse.db`。
- **qlib collector**（`internal/collector/qlib/`）：实现 collector 接口，仓库主源读 + 外部补新鲜尾巴 + 完全可降级 + 陈旧度告警；`FetchQuote`/非日频委托外部源。
- **selector qlib 优先路由**：`SelectForSymbol` 命中 `Covers` 走仓库；新增 `SelectExternalForSymbol`（永不返回 qlib，防补尾递归）。
- **配置 `QlibConfig`**（`enabled`/`db_path`/`max_staleness_days`）与 serve.go 可降级装配（`?mode=ro` 只读打开，open/Ping 失败跳过注册）。
- **`App.CollectorRegistry()`** 导出 live registry 供补尾委托。
- 依赖 `modernc.org/sqlite v1.38.2`（纯 Go 驱动，go1.24 兼容）。

## Fixed (QA review)
- `readRange` 对 NULL 数值列崩溃 → `sql.NullFloat64/NullInt64` 扫描（NULL→0）。
- `Covers` 与 `lastDate` 判定口径统一（校验 `last_date` 可解析，解析失败记 Warn）。
- `symbol_detail` API 对 qlib 错误无降级（HTTP 500）→ 新增 `fetchHistoryWithFallback` 回落外部源。

## Unchanged (零回归保证)
- `qlib.enabled=false` 或缺库时行为与现状完全一致；所有既有 collector/selector/strategy 测试全绿。

## Notes
- 范围边界：Part B PIT 基本面、A 股/港股 dump、实时/分钟频入库均留第二期。
