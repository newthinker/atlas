# 港股 qlib 数据包扩展（atlas_hk）设计

**日期**: 2026-06-13 | **状态**: 设计定稿，待实现
**动机**: 现有 qlib 管线为 Phase-1 A股 only（atlas_cn）。扩展支持香港交易所的股票、
基金（场内 ETF/REIT）、指数，建立独立的港股数据包 atlas_hk，并把港股标的纳入 watchlist
跑通全管线（取数 → dump_bin → 分析）。
**前置**: atlas_cn 管线已交付（PR #27）；lixinger 港股端点已修复（`hk/company/fundamental/non_financial`、`hk/index/fundamental`）；eastmoney 指数路由与 isFund 修复已合并（PR #26）。

## 目标
把港股股票/ETF/指数纳入 config watchlist，经 yahoo 采集 OHLCV，构建**独立于
atlas_cn 的** qlib 数据包 `~/.qlib/qlib_data/atlas_hk`，并产出港股 watchlist 分析报告。

## 核心架构决策

**独立 atlas_hk 数据包（HK 交易日历）**：港股交易日历与 A股不同（不同假期/交易日）。
dump_bin 按 CSV 日期并集生成 `calendars/day.txt`——若把港股 CSV 混入 atlas_cn，会污染
A股日历、破坏 qlib 的日历对齐运算。因此港股单独 dump 成 atlas_hk，与 atlas_cn 并存。

**export-ohlcv market 参数化（方案 A）**：新增 `--market cn|hk` 标志。
- `cn`（默认）：现状不变——watchlist A股集（.SH/.SZ/.CSI）+ 基准 000300.SH → qlib_csv。
- `hk`：watchlist 港股集（.HK + 港股指数 ^HSI/^HSCE）+ 基准 ^HSI → qlib_csv_hk。
- 基准从写死 `000300.SH` 改为按 market 取（cn=000300.SH，hk=^HSI）。
- A股路径行为零回归（默认 market=cn）。

## 命名契约（Go + Python 双侧对称，含契约测试）

| atlas 符号 | qlib instrument | CSV 文件名 | 规则 |
|---|---|---|---|
| `0700.HK`（股票） | `HK00700` | `hk00700.csv` | `HK` + 5 位补零代码（对齐 lixinger 港股 5 位口径） |
| `2800.HK`（ETF/REIT） | `HK02800` | `hk02800.csv` | 同股票（场内基金按证券处理） |
| `^HSI`（指数） | `HSI` | `hsi.csv` | 小映射表 `^HSI→HSI` |
| `^HSCE`（指数） | `HSCEI` | `hscei.csv` | 小映射表 `^HSCE→HSCEI` |

`toQlibInstrument`（Go）/`to_qlib_instrument`（Python）扩展：
- `.HK` → `"HK" + 5位补零(TrimSuffix(.HK))`（如 `0700.HK`→`HK00700`、`2800.HK`→`HK02800`）。
- `^HSI`→`HSI`、`^HSCE`→`HSCEI`（仅支持这两个 HK 指数，其余 `^` 拒绝）。
- 非以上 + 非 A股 → 仍 `error`/`ValueError`。

## watchlist 新增（configs/config.yaml）

港股基金（ETF/REIT，type "基金"，仅 `price_percentile`——无基本面 PE）：
- `2800.HK` 盈富基金（跟踪 HSI）
- `2828.HK` 恒生中国企业（跟踪 HSCEI）
- `3033.HK` 恒生科技ETF（代理恒生科技指数）
- `3181.HK` AI 行业指数基金

港股指数（type "指数"，仅 `price_percentile`）：
- `^HSI` 恒生指数
- `^HSCE` 国企指数（HSCEI）

> `^HSTECH` 恒生科技指数**不纳入**：yahoo 实测 404（无数据），由 `3033.HK` 恒生科技ETF 代理。
> 现有 5 只港股个股（3288/0700/9988/0883/6886.HK）保持不变（已有 pe_percentile 走 yahoo EPS）。

## 数据流（market=hk）

```
export-ohlcv --market hk --config configs/config.yaml --from <date> --out-dir qlib_csv_hk
  ├─ resolveOHLCVSymbols(hk): 选 watchlist 中 .HK 标的 + 港股指数(^HSI/^HSCE)，
  │   去重并保证基准 ^HSI 在内（基准缺失=硬错误）
  ├─ 逐 symbol: SelectForSymbol → yahoo.FetchHistory → CSV(HK 命名) → qlib_csv_hk/
  └─ build_data.py --csv-dir qlib_csv_hk --target-dir ~/.qlib/qlib_data/atlas_hk
        dump_bin 按 HK CSV 日期自动生成 HK 交易日历（独立 calendars/day.txt）
analyze_watchlist.py --qlib-dir ~/.qlib/qlib_data/atlas_hk --out reports/watchlist-hk-analysis.md
```

行情**全部走 yahoo**（已验证：.HK 股票/ETF + `^HSI`/`^HSCE` 均可取，3181.HK last≈162.75、
^HSI≈24718、^HSCE≈8374）。lixinger 港股基本面属估值/PE 路径，不进 OHLCV 数据包。

## 组件

| 组件 | 职责/改动 |
|---|---|
| `configs/config.yaml` watchlist | +4 ETF（基金）+ 2 指数（HSI/HSCEI），均仅 price_percentile |
| `cmd/atlas/export_ohlcv.go` | `--market cn\|hk` flag；按 market 取基准（cn=000300.SH/hk=^HSI）与 watchlist 子集；`toQlibInstrument` 扩展 .HK/^HSI/^HSCE；out-dir 由调用方指定（hk→qlib_csv_hk） |
| `cmd/atlas/export_ohlcv_test.go` | HK 命名契约样本（0700.HK→HK00700、^HSI→HSI、^HSCE→HSCEI）；resolveOHLCVSymbols hk 选择 + 基准用例 |
| `scripts/qlib_eval/qlib_eval/symbols.py` + `tests/test_symbols.py` | `to_qlib_instrument` 镜像 HK 命名 + 契约样本 |
| `Makefile` | 新增 `qlib-data-hk` target：export-ohlcv --market hk → qlib_csv_hk → build_data → atlas_hk |
| `scripts/qlib_eval/analyze_watchlist.py` | 指数识别泛化（让 HK 指数 HSI/HSCEI 进相关性矩阵；当前 `c[2:5] in 000/399` 仅认 A股，非阻塞） |

## 错误处理 / 边界

- HK 基准 `^HSI` 取数失败 = **硬错误**（无基准则分析无意义，不降级），与 000300.SH 同口径。
- `^HSTECH` 不纳入（yahoo 404）。
- 指数/ETF 无基本面 → 不配 pe_percentile，仅 price_percentile。
- **实现期须验证**：`SelectForSymbol(^HSI)` / `MarketForSymbol(^HSI)` 是否把 HK 指数路由到
  yahoo。probe 已证 `yahoo.FetchHistory("^HSI")` 本身可取；若 registry 未把 `^` 开头 HK
  指数归到 yahoo（MarketHK），需在 `MarketForSymbol`/selector 中补 `^HSI`/`^HSCE` → HK 的归类
  （仅这两个受支持指数，不放开任意 `^`）。
- 空 CSV 不写盘；单标的失败降级 + 失败摘要 + 非 0 退出（沿用现有 executeExportOHLCV 逻辑）。

## 测试

- **命名契约**（Go + Python 双侧，同样本）：`0700.HK→HK00700`、`2800.HK→HK02800`、
  `^HSI→HSI`、`^HSCE→HSCEI`；`^HSTECH`/其它 `^`/非 A股非 HK → 拒绝。
- **resolveOHLCVSymbols(hk)**：从 watchlist 选出 .HK + ^HSI/^HSCE，含基准 ^HSI，去重保序。
- **基准按 market**：cn→000300.SH、hk→^HSI；缺失基准报错。
- **集成**：`export-ohlcv --market hk` → 生成 HK CSV → `build_data` 建 atlas_hk →
  `analyze_watchlist` 产报告（本环境 yahoo 可达，可真跑验证）。

## 非目标（YAGNI）

- 不实现港股场外基金 NAV（lixinger 无此产品；港股基金=场内 ETF/REIT）。
- 不把港股并入 atlas_cn（日历冲突）。
- 不实现 `^HSTECH`（yahoo 无数据，ETF 代理）。
- 港股 PE 分位评估（pe_percentile 离线回放）不在本期——signal-eval 仍仅 ma_crossover/price_percentile。
- 不引入 atlas 内调度；定时重建沿用系统 cron 调 make target。
