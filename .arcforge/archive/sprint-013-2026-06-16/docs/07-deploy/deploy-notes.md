# 部署说明 — 第三期 分位全历史回看

## 零回归（默认无需任何操作）
本期默认行为与现状完全一致：不配 `valuation:` 块、不改 strategies 的 `lookback_years` → 仍是 5 年窗口。`config.Load()` 已对 `valuation.lookback_years` 设默认 5，存量配置文件无需改动即零回归。

## 启用全历史回看（opt-in）
1. **配置**（需三处同时设 0，否则价格窗口与 PE/EPS 窗口不一致）：
   ```yaml
   strategies:
     price_percentile:
       params: { lookback_years: 0, ... }
     pe_percentile:
       params: { lookback_years: 0, ... }
   valuation:
     lookback_years: 0
   ```
2. **数据**（关键前提）：全史回看需仓库/数据包含全史 OHLCV。当前数据仅 ~5 年。重 dump：
   ```bash
   ./bin/atlas export-ohlcv --config configs/config.yaml --market us --from 1970-01-01 --out-dir qlib_csv_us
   make warehouse-dump
   ```
   > ⚠ 已知 bug（非本期）：`export-ohlcv` 当前在 `yahoo.FetchHistory`(yahoo.go:229) nil-deref panic，全史 dump 受阻，需先修该既有 bug。修复前 inception 模式只用已有数据（~5 年）。
3. **重启 serve**：inception 模式生效，信号 Reason 显示「full history (N bars)」。

## 各市场能力（诚实边界）
| 路径 | inception 能力 |
|---|---|
| 价格分位（全市场） | 真·自上市起（受数据范围约束，起点钳 1970） |
| PE·美/港个股（yahoo EPS 重建） | 真·自上市起（受数据约束） |
| PE·A 股个股 + 所有指数（lixinger） | 上限 10 年（y10），inception 等价「最多 10 年」 |

## 回滚
删除 `valuation:` 块或把三处 `lookback_years` 改回 5（或删除）→ 重启即回 5 年窗口。

## 运维提示
- inception 起点钳到 1970-01-01：^GSPC 等 1957 起指数损失 1957-1970 价格历史（指数 PE 走 lixinger 不受影响）。
- 新上市标的（<252 bars）即使 inception 也不出价格信号（minSampleBars 兜底）。
