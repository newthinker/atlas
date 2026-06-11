# Changelog — sprint-002 指数/商品采集 + 历史百分位策略（2026-06-11）

## 新功能

- **国际指数与商品期货监控**：watchlist 支持 `^GSPC`/`^IXIC`/`^DJI`/`^HSI` 指数与 `GC=F` 类期货符号——yahoo 采集器符号校验扩展 + URL 百分号编码（9a63967），selector 自动路由至 yahoo 并映射市场归属（583cb8b），A 股六指数（上证/沪深300/中证500 等）经 eastmoney secid 表解析（7552246, ae8353e）
- **price_percentile 策略**（全资产类）：当前价格在自身多年分布中的位置，extreme/normal 分档信号，252 根门槛防新上市误报（6687cef）
- **pe_percentile 策略**（股票+指数）：PE 估值历史百分位信号，Source 携带获取方法与兜底原因（9ee0aed）
- **PE 估值编排兜底链**（f087741）：A 股与美/港指数走理杏仁 cvpos；美/港个股 Yahoo EPS(TTM) 重建主路径，数据缺失自动兜底理杏仁，真实亏损（EPS≤0）直接跳过不兜底
- **理杏仁多市场估值分位**：cn/hk/us × company/index 端点分派，港股代码 5 位补零（cfb4fe8）
- **yahoo EPS 历史序列**：fundamentals-timeseries 端点，trailingDilutedEPS 季度序列（9a63967）
- **internal/valuation 新包**：PercentileRank（strictly-less 口径）与 ReconstructPEPercentile（EPS 阶梯对齐、亏损季剔除、MinEPSPoints=8）（f8f5534）
- **AssetTypes 绑定校验 + 动态历史窗口**：策略声明资产类约束，错绑 warning+跳过；历史窗口按绑定策略最大需求自动放大（244280f）

## 缺陷修复

- 仲裁合成信号未携带价格（CARRYOVER I3，资金安全 latent）：referencePrice 取冲突信号首个正价，反例锁定测试（cc0182a）

## 配置变更（config.example.yaml）

```yaml
strategies:
  price_percentile: {enabled: true, params: {lookback_years: 5, low: 25, high: 75, extreme_low: 10, extreme_high: 90}}
  pe_percentile:    {enabled: true, params: {lookback_years: 5, low: 20, high: 80, extreme_low: 10, extreme_high: 90}}
watchlist:  # 新增示例
  - {symbol: "^GSPC",    name: "标普500",   type: "指数",     strategies: [price_percentile, pe_percentile]}
  - {symbol: "GC=F",     name: "COMEX黄金", type: "期货",     strategies: [price_percentile]}
  - {symbol: "BTC-USDT", name: "比特币",    type: "加密货币", strategies: [price_percentile]}
```

- 删除 pe_band 下从未生效的 `lookback_years`/`threshold_percentile` 死参数

## API/接口变更

- `core`：新增 `AssetCrypto`、`EPSPoint`、`Fundamental.PEPercentile`（负值=不可用）
- `collector`：新增 `AShareIndexSecIDs`/`IsAShareIndex`/`KnownIndexMarket`；`yahoo.NewWithBaseURLs(chart, eps)`、`FetchEPSHistory`；`lixinger.FetchValuationPercentile`
- `app`：新增 `ValuationSource`/`EPSSource` 接口与 `SetValuationSources`（必须 Start 前注入）；既有策略 `RequiredData` 增加 AssetTypes 声明
- 回测引擎注册 price_percentile（pe_percentile 依赖在线数据不注册）

## 已知边界（一期）

- 金融股理杏仁 non_financial 端点不适用 → 按不可用降级
- 表外 ^ 指数默认 US 市场 + 启动 warning
- 理杏仁 us/hk 指数代码（SPX/COMP/DJI/HSI）为候选值，待 API key 核对固化
