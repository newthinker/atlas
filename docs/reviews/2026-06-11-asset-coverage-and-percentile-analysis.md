# ATLAS 资产覆盖与历史百分位策略支持分析

> 日期：2026-06-11
> 范围：基于 master 分支（d8a3c8c）的代码静态分析
> 主题：① 股票/指数/大宗商品/加密货币等资产的监控方式与策略支持现状；② 历史百分位估值监控的支持情况

---

## 一、各资产类型的监控方式与策略支持

### 1.1 总体结论

ATLAS 是一个多资产监控系统，但各资产类型的支持程度差异很大：

- **股票（美/港/A 股）支持最完整**；
- **加密货币有专门的采集通道**，可用技术面策略；
- **指数和大宗商品只有类型定义，没有实际可用的数据采集路径**，目前无法监控。

### 1.2 监控方式（对所有资产统一）

监控核心链路位于 `internal/app/app.go`：

1. **Watchlist 驱动**：在 `config.yaml` 的 `watchlist` 中配置要监控的 symbol 及其关联策略；也可通过 REST API / Web Dashboard（HTMX）动态增删（`internal/api/handler/api/watchlist.go`）。
2. **定时轮询**：`App.Start` 启动 ticker 循环（默认 5 分钟，可配置 interval），每个周期对 watchlist 逐个 symbol 拉取行情和历史 K 线，交给策略引擎分析。
3. **采集器自动路由**（`internal/collector/selector.go`）：
   - `.SH` / `.SZ` → eastmoney（A 股）
   - BTC/ETH/`-USD`/`USDT` 等加密符号 → crypto 采集器
   - 其余 → yahoo（美股/港股）
   - lixinger 额外提供 A 股基本面数据
4. **信号与通知**：策略产出 `Signal`（buy/sell/strong_buy 等 + 置信度），经 router 过滤（`min_confidence: 0.6`、4 小时冷却）后由 Telegram / Email / Webhook 推送；可接 LLM 仲裁（arbitrator）和 Futu 券商执行（paper/live）。
5. **系统级告警**：Prometheus metrics 与 `internal/alert` 模块提供 error_rate、api_down 等**系统健康告警**，不是价格告警。

### 1.3 各资产类型支持矩阵

| 资产类型 | 类型定义 | 数据采集 | 可用策略 | 实际可监控 |
|---|---|---|---|---|
| 股票（美/港/A） | ✅ `AssetStock` | ✅ yahoo + eastmoney + lixinger | 全部 | ✅ 完整 |
| 加密货币 | ⚠️ 见下 | ✅ 专门 collector（OKX/CoinGecko/Binance） | ma_crossover | ✅ 可用 |
| 指数 | ✅ `AssetIndex` | ❌ 无通道 | — | ❌ 实际不可用 |
| 大宗商品 | ✅ `AssetCommodity` | ❌ 无通道 | — | ❌ 实际不可用 |
| ETF / 基金 | ✅ `AssetETF` / `AssetFund` | ⚠️ 部分（`Quote.FundInfo` 含净值信息） | 技术面 | ⚠️ 部分 |

关键发现：

- **加密货币**：`internal/collector/crypto/` 下有 OKX、CoinGecko、Binance 三个 provider 做容灾（OKX 优先，考虑国内可达性），符号自动识别并路由到 `MarketCrypto`。但核心枚举 `core.AssetType`（`internal/core/types.go:19`）**没有 crypto 值**，只有 app 层的中文常量 `TypeCrypto = "加密货币"`，类型体系不一致。
- **指数与大宗商品是「纸面支持」**：虽然定义了 `AssetIndex`、`AssetCommodity`，但 yahoo 采集器的符号校验正则（`internal/collector/yahoo/yahoo.go:21`）为 `^[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?$`，会直接拒绝 Yahoo 的指数符号（如 `^GSPC`，含 `^`）和商品期货符号（如 `GC=F`，含 `=`），且没有任何专门的指数/商品采集器。
- **WatchlistItem 的 Type 字段**列出「股票/基金/债券/ETF/期权/期货/加密货币」，但自动检测函数 `DetectType`（`internal/app/app.go:552`）只能识别 crypto 和 stock 两类，其余需手工指定且无对应数据源。

### 1.4 策略支持情况

`internal/strategy/` 下有三个内置策略：

| 策略 | 类型 | 数据依赖 | 适用资产 |
|---|---|---|---|
| `ma_crossover` | 技术面（均线交叉） | K 线历史 + SMA | 任何能采到行情历史的资产（股票、加密货币） |
| `pe_band` | 基本面（PE 估值带） | `Fundamental`（PE） | 仅股票 |
| `dividend_yield` | 基本面（股息率） | `Fundamental`（分红） | 仅股票 |

另有：LLM 元策略（信号仲裁 arbitrator、策略合成 synthesizer）、回测引擎（`internal/backtest`）、指标库（`internal/indicator`）。

**架构缺口**：策略接口的 `DataRequirements` 定义了 `AssetTypes` 字段（`internal/strategy/interface.go:18`）用于声明策略适用的资产类型，但三个策略均未填写——「策略 ↔ 资产类型」的匹配过滤机制只搭了架子、没有启用。配置上把 `pe_band` 挂到加密货币不会被拦截，只会因缺基本面数据而无法出信号。

### 1.5 补齐指数与大宗商品支持的改造点

1. 放宽或重写 yahoo 符号校验，接受 `^GSPC`、`GC=F` 类符号（或新增专门 collector）；
2. `SelectForSymbol` 增加指数/商品路由分支；
3. `core.AssetType` 补上 crypto，并统一 app 层的中文类型常量；
4. 各策略声明 `AssetTypes`，并在策略引擎中启用过滤。

---

## 二、历史百分位估值监控的支持情况

### 2.1 结论

**不支持。历史百分位监控目前只是「配置上的承诺」，实现中完全没有落地。**

### 2.2 证据

1. **配置声明了百分位参数，但代码从不读取。** `configs/config.example.yaml` 中 pe_band 写着：

   ```yaml
   pe_band:
     params:
       lookback_years: 5
       threshold_percentile: 20
   ```

   但 `pe_band` 的 `Init`（`internal/strategy/pe_band/strategy.go:31`）只读取 `low_threshold` 和 `high_threshold`，`lookback_years` 与 `threshold_percentile` 被静默忽略。

2. **实际判断逻辑是绝对阈值比较，不是历史分位。** `Analyze`（strategy.go:41）将当前 PE 与固定阈值比较：`pe < lowThreshold` 出买入信号、`pe > highThreshold` 出卖出信号，不涉及任何历史 PE 序列。

3. **全代码库没有任何百分位计算。** 搜索 `percentile` / `分位` / `quantile` / `cvpos`，仅在两个 yaml 配置文件命中，Go 代码零命中；`lookback` 同样无人使用。

4. **数据层未准备好。** 历史分位需要历史基本面序列（如每日 PE），但：
   - pe_band 的 `RequiredData` 声明 `PriceHistory: 0`，只取当前一份 `Fundamental` 快照；
   - lixinger 采集器（`internal/collector/lixinger/lixinger.go`）只拉取 OHLCV、行情和基金净值字段——理杏仁 API 本身提供现成的 PE/PB 历史分位字段（如 `pe_ttm.cvpos`），但当前代码未请求这些 metrics。

### 2.3 两条可行的实现路径

| 路径 | 思路 | 优点 | 缺点 |
|---|---|---|---|
| **数据源现成分位** | lixinger 请求 metrics 时加 `pe_ttm.cvpos`（理杏仁直接返回近 N 年估值分位），字段加进 `core.Fundamental`，pe_band 改为按分位阈值判断 | 改动最小，无需历史数据积累 | 只覆盖 A 股 |
| **本地计算** | storage 层持久化每日基本面快照，策略侧按 `lookback_years` 取历史 PE 序列自算百分位 | 通用，可覆盖美股/港股 | 工程量大，需先积累或回填历史数据 |
