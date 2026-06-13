# Changelog — Lixinger Collector 修复重写（2026-06-13）

## Fixed
- **传输层**：新增 `client.go` 统一 `request()`，修正响应信封判定（真实 API 成功为
  `code:1`，原代码 7 处误判 `code!=0` 为错误）；4xx 透出 `error.message`。
- **退避重试**：429/5xx 按 1/2/4/8/16s 退避（最多 5 次），4xx 不重试；新增
  `collectors.lixinger.retry` 配置开关（默认开）。
- **K线/行情**（`stock.go`）：`FetchHistory` 端点 `cn/stock/hq`→`cn/company/candlestick`
  （单数 `stockCode` + `type:fc_rights`，RFC3339 日期）；`FetchQuote` 改用 candlestick
  最新收盘近似（`Source: lixinger-delayed`），理杏仁无实时行情 API。
- **个股基本面**（`fundamental.go`）：`metrics`→`metricsList`；指标 `roe_ttm/dividend_yield_ratio/
  market_value`→`dyr/mc`（ROE 该端点不提供，置零）；`DividendYield` ×100 归一为百分数。
- **估值分位**（`valuation.go`）：改用 `request()`，按扁平 dotted key 解析；指数端点用
  `pe_ttm.y{N}.mcw.cvpos`（市值加权）；港股个股端点补 `/non_financial`；美股个股无端点
  →降级；美股指数 `^GSPC` 码 `SPX`→`.INX`。
- **基金**（`fund.go`）：端点 `cn/fund/nav*`/`cn/fund/fundamental`（均 404）→
  `cn/fund/net-value` + 聚合 `cn/fund/profile`/`cn/fund/manager`/`cn/fund/drawdown`；
  基金名取 `e_t_short_name`（`c_name` 实为托管行）；`MaxDrawdown` ×100 归一；
  `fetchFundInfo` 加 apiKey 守卫；子接口失败优雅降级。
- **接线**（`serve.go`）：lixinger 构造读取 `retry` 配置开关。

## Changed
- 构造器 `lixinger.New(apiKey, opts...)` + `WithRetry(bool)`；`NewWithBaseURL` 保留供测试。
- 移除 `postJSON`/`postJSONRaw`/`digFloat`/`lixingerResponse`/`lixingerMetric` 及全部旧错误端点。

## Removed
- 删除 `lixinger_httptest_test.go`（旧反转语义 + 已废弃端点）。
- 删除临时探测目录 `_probe/`。

## Tests
- 新增 `client_test.go`/`stock_test.go`/`fundamental_test.go`/`fund_test.go`，重写
  `valuation_test.go`；fixture 改用 live 实测响应形状。lixinger 包覆盖率 90.9%。

## Notes
- 不改 `core` 类型定义。
- 美股指数 ^DJI/^IXIC 代码待后续核实（当前安全降级）。
