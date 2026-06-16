# Changelog — Qlib 数据仓库 第二期（Part B PIT 基本面）

## Added
- **PIT 财务数据库**：`fundamentals_pit` 表（第一期建空）现由 dump 管线填充。归一化基本面 CSV 契约 `symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield`。
  - `scripts/qlib_warehouse/fundamentals.py`：解析基本面 CSV → `FundRow`；必填 eps_ttm 空值行跳过（数据质量）。
  - `writer.write(..., fundamentals=None)`：fundamentals 与 OHLCV **同一原子写**落库，修订（同 report_period 不同 observe_date）保留不去重。向后兼容第一期调用。
  - `build_warehouse.py --fundamentals-dir`（可选）：解析并随 OHLCV 同写；目录不存在 return 3。
- **qlibpit EPS 源**（`internal/collector/qlibpit/`）：实现 `app.EPSSource`，`observe_date <= 窗口末` 点对时间查询消除前视偏差；升序保留修订；仓库无符号基本面时委托内层 EPSSource(yahoo) 兜底。
- **serve 装配**：`wireQlibWarehouse` 重构返回 `(*sql.DB, bool)` 暴露已开只读句柄；qlibpit 包装 yahoo EPS 源注入（仓库 PIT 优先、yahoo 兜底）。
- **ADAPTERS.md**：A股(qlib dump_pit)/美股(Yahoo asOfDate+45天近似)/港股(lixinger) 三市场 observe_date 产出契约。
- Makefile `warehouse-dump` 增 US fundamentals 透传（`$(wildcard)` 守卫，目录不存在则只写 OHLCV）。

## Fixed
- 消除 PE 分位重建的前视偏差：原 Yahoo 路径用报告期末 `asOfDate` 对齐，现用真实可知日 `observe_date` 截断。

## Unchanged (零回归)
- qlib 未启用（`db==nil`）时 EPS 源维持纯 yahoo；writer 不传 fundamentals / build 不传 --fundamentals-dir 行为与第一期一致；第一期全部测试全绿。

## Notes
- 各市场 fundamentals CSV 实际生产为 best-effort 适配器；美股 observe_date 为披露滞后近似；PB/PS/ROE 入库本期 Go 侧不消费。
