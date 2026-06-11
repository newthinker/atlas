# 指数/大宗商品采集 与 历史百分位监控策略 — 设计文档

> 日期：2026-06-11
> 状态：设计已确认（用户逐节批准）
> 前置分析：`docs/reviews/2026-06-11-asset-coverage-and-percentile-analysis.md`

## 1. 需求与范围

### 1.1 需求

1. **指数与大宗商品的数据采集**：使 watchlist 可以监控指数和大宗商品。
2. **基于历史百分位的监控策略**：对所有资产类型（股票/指数/ETF/商品/加密货币）提供历史百分位策略，输出标准买卖信号。

### 1.2 已确认的范围决定

| 决定点 | 结论 |
|---|---|
| 分位指标 | 价格分位 + 估值分位（PE），一期同时实现 |
| 指数/商品覆盖 | 国际主要指数（^GSPC 等）+ A 股指数（000300.SH 等）+ 国际商品期货（GC=F 等） |
| 估值分位数据源 | A 股（股票/指数）走理杏仁 cvpos 精确值；美/港股用「价格历史 ÷ 当前 EPS(TTM)」近似 |
| 输出语义 | 标准买卖信号（buy/sell/strong_buy/strong_sell），完全复用 Signal → router → notifier 链路 |
| 实现路线 | 方案三：扩展现有采集器 + 双策略 + 类型体系修补（不新建采集器） |

### 1.3 明确不做（一期边界）

- 国内期货（上期所/大商所/郑商所）数据源接入 — 留二期
- PB / 股息率分位
- 估值分位的本地基本面快照库（每日持久化积累）
- `pe_band` 的分位化改造（保留其绝对阈值语义）
- 独立的提醒（alert）模式 — 百分位策略只走信号链路

## 2. 采集层设计

### 2.1 Yahoo 采集器（指数 + 商品期货）

- 符号校验正则从 `^[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?$` 扩展为同时接受：
  - 指数前缀：`^` + 字母数字（`^GSPC`、`^IXIC`、`^DJI`、`^HSI`、`^N225`）
  - 期货后缀：字母 + `=F`（`GC=F`、`CL=F`、`SI=F`、`HG=F`）
- 构造请求 URL 时对 `^` 做 percent-encoding（`%5E`），统一在请求构造处处理
- `SupportedMarkets` 不变（US/HK），指数/商品归入对应市场
- 现有 chart 端点（`query1.finance.yahoo.com/v8/finance/chart`）原生支持这些符号，无需更换接口

### 2.2 eastmoney 采集器（A 股指数）

- `parseSymbol` 增加指数代码识别：维护 A 股指数代码表
  - `000300.SH` → secid `1.000300`（沪深 300）
  - `000905.SH` → secid `1.000905`（中证 500）
  - `000001.SH` → secid `1.000001`（上证指数；与 `000001.SZ` 平安银行靠市场前缀区分）
  - `399001.SZ` → secid `0.399001`（深证成指）等
- 行情/历史接口无需改动（secid 格式通用）

### 2.3 路由层（`internal/collector/selector.go`）

- `SelectForSymbol` 增加分支：`^` 前缀或 `=F` 后缀 → yahoo；指数代码表命中的 `.SH/.SZ` → eastmoney
- `MarketForSymbol` 同步更新

### 2.4 lixinger 采集器（估值分位数据）

- 新增方法 `FetchValuationPercentile(symbol string, lookbackYears int) (float64, error)`：
  - 股票走 `cn/company/fundamental` 接口，请求 `pe_ttm.y{N}.cvpos`（lookback 映射 y3/y5/y10）
  - 指数走 `cn/index/fundamental` 接口同字段
- `core.Fundamental` 新增字段 `PEPercentile float64`（0–100；-1 表示不可用）

### 2.5 资产类型自动识别（`internal/app.DetectType`）

- `^` 前缀 / 指数代码表命中 → `index`
- `=F` 后缀 → `commodity`
- 其余规则不变（crypto/stock）

## 3. 策略层与类型体系设计

### 3.1 `price_percentile` 策略（新增 `internal/strategy/price_percentile/`）

- 适用资产：全部（stock/index/etf/commodity/crypto）
- 数据需求：`PriceHistory = lookback_years × 252` 个交易日的日线收盘价，复用现有 `FetchHistory` + TTL 缓存，**不引入新存储**
- 计算：当前价格在历史收盘价序列中的百分位 `P = rank(price) / N × 100`
- 信号规则（参数可配，括号为默认值）：

  | 条件 | 信号 | 置信度 |
  |---|---|---|
  | P < `extreme_low`（10） | strong_buy | 0.8 + 距离加成，上限 0.95 |
  | P < `low`（25） | buy | 0.6–0.8 线性映射 |
  | P > `extreme_high`（90） | strong_sell | 与 buy 侧对称 |
  | P > `high`（75） | sell | 同上 |
  | 其余 | 不出信号 | — |

- 历史样本不足 1 年（约 252 个交易日）时不出信号，避免新上市资产误报
- `Signal.Metadata` 携带 `percentile`、`lookback_years`、`sample_size`

### 3.2 `pe_percentile` 策略（新增 `internal/strategy/pe_percentile/`）

- 适用资产：仅 stock + index（通过 `AssetTypes` 声明）
- 分位获取两条路径，策略本身不感知差异（由 AnalysisContext 提供 `PEPercentile`）：
  - **A 股**（股票/指数）：理杏仁 cvpos 精确值
  - **美/港股**：近似计算 — 价格历史 ÷ 当前 EPS(TTM) 重建 PE 序列后求分位；`Metadata` 标注 `method: "approximated"`，通知文案注明近似
- 信号规则与阈值结构同 price_percentile（默认 low=20 / high=80，极值 10/90）
- EPS ≤ 0（亏损）或分位不可用时不出信号
- 现有 `pe_band` 保留不动；配置示例中删除从未生效的 `lookback_years`/`threshold_percentile` 误导参数

### 3.3 类型体系修补

- `core.AssetType` 增加 `AssetCrypto AssetType = "crypto"`
- 策略引擎（`internal/strategy/engine.go`）启用 `AssetTypes` 过滤：watchlist 项的资产类型不在策略声明范围内时，**启动时输出 warning 日志并跳过该绑定**（不静默、不崩溃）
- 既有策略补填 `AssetTypes`：ma_crossover 全资产；pe_band / dividend_yield 仅 stock

## 4. 数据流

以沪深 300 为例：

```
watchlist: {symbol: 000300.SH, type: index, strategies: [price_percentile, pe_percentile]}
   │ ticker 周期触发
   ▼
selector → eastmoney（指数代码表命中）
   ├─ FetchQuote   → 当前点位
   ├─ FetchHistory → 5 年日线（TTL 缓存）
   └─ lixinger.FetchValuationPercentile → PE 分位（cvpos）
   ▼
AnalysisContext{OHLCV, LatestQuote, Fundamental.PEPercentile}
   ▼
price_percentile / pe_percentile 各自 Analyze
   ▼
Signal（含 percentile 元数据）→ router（min_confidence、冷却）→ Telegram / Email / Webhook
```

- 商品（GC=F）走 yahoo，只绑定 price_percentile
- 美股 pe_percentile 在 Context 组装阶段以价格序列 ÷ 当前 EPS 近似重建 PE 序列

## 5. 错误处理

| 故障场景 | 行为 |
|---|---|
| 历史数据不足（< 1 年） | 不出信号，debug 日志说明样本量 |
| 理杏仁不可用 / 无 API key | pe_percentile 对 A 股降级为近似计算路径，`method` 标注降级 |
| EPS ≤ 0 或缺失 | pe_percentile 跳过该标的，不影响 price_percentile |
| Yahoo 对 `^`/`=F` 符号返回异常 | 与现有股票路径相同的错误传播；单标的失败不影响其他标的（沿用 `analyzeSymbolSafe` 隔离） |
| 策略绑定到不支持的资产类型 | 启动时 warning + 跳过该绑定 |

## 6. 测试策略

- **采集层**：
  - yahoo 符号校验表驱动测试（`^GSPC` / `GC=F` / 非法符号）；URL percent-encoding 断言
  - eastmoney 指数 secid 映射测试（含 `000001.SH` 与 `000001.SZ` 歧义用例）
  - lixinger cvpos 走 httptest 模拟（沿用 `lixinger_httptest_test.go` 模式）
- **策略层**：
  - 百分位计算边界：空序列、单点、全相同值、当前价为历史最高/最低
  - 四档信号阈值与置信度映射
  - 样本不足 / EPS 为负的跳过路径
- **引擎层**：AssetTypes 过滤的 warning + 跳过行为
- **回测兼容**：两个新策略仅依赖 Strategy 接口，可被现有 backtest 引擎直接驱动；补一条回测冒烟用例

## 7. 配置示例

```yaml
strategies:
  price_percentile:
    enabled: true
    params: {lookback_years: 5, low: 25, high: 75, extreme_low: 10, extreme_high: 90}
  pe_percentile:
    enabled: true
    params: {lookback_years: 5, low: 20, high: 80, extreme_low: 10, extreme_high: 90}

watchlist:
  - {symbol: "^GSPC",     name: "标普500",   type: index,     strategies: [price_percentile, pe_percentile]}
  - {symbol: "000300.SH", name: "沪深300",   type: index,     strategies: [price_percentile, pe_percentile]}
  - {symbol: "GC=F",      name: "COMEX黄金", type: commodity, strategies: [price_percentile]}
  - {symbol: "BTC-USDT",  name: "比特币",    type: crypto,    strategies: [price_percentile]}
```
