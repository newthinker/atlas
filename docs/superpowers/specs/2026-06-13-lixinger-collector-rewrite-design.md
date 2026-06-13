# Lixinger Collector 修复重写 — 设计文档

日期：2026-06-13
状态：待评审

## 背景

`internal/collector/lixinger` 的实现与理杏仁开放平台真实 API 几乎全面不符。用真实
token（config.yaml 中配置）逐接口验证后确认：除「个股 PE 估值分位」端点选对外，其余
端点、参数、字段映射、响应信封判定全部错误。lixinger 在系统中有两个角色：

1. **eastmoney 的 fallback**（`SetLixingerFallback`）：兜底 `FetchQuote` / `FetchHistory` /
   `FetchFundQuote` / `FetchFundInfoPublic` / `FetchFundHistory`。
2. **估值分位主数据源**（`SetValuationSources`）：`FetchValuationPercentile` 是 PE 分位
   的唯一来源，无替代。

验证依据为官方 skill 包 `~/Downloads/lixinger-open-skill` 的 `api-docs/*.json`，以及对
真实 API 的实测响应。

## 根因

1. **响应信封判定反了（影响全部 7 个方法）**：真实 API 成功返回 `code:1`；失败返回
   `code:0` + `error{message/name/messages}`。现有代码 7 处全写成 `if result.Code != 0
   { 报错 }`，把所有成功响应当错误丢弃。
2. **端点错误**：`cn/stock/real-time`、`cn/stock/hq`、`cn/fund/nav`、`cn/fund/fundamental`、
   `cn/fund/nav/history` 在真实 API 上均不存在（404）。
3. **参数形状错误**：`metrics` 应为 `metricsList`；K线/基金净值应用单数 `stockCode` 而非
   复数 `stockCodes`。
4. **指标名错误**：`roe_ttm` / `dividend_yield_ratio` / `market_value` 非法 → 应为
   `dyr`（股息率）/ `mc`（市值）；`roe_ttm` 该端点不提供。
5. **响应字段解析错误**：基金净值字段是 `netValue` 非 `nav`；日期是 RFC3339
   （`2026-06-10T00:00:00+08:00`）非 `2006-01-02`。
6. **指数估值缺权重段**：指数端点要求 `pe_ttm.y{N}.mcw.cvpos`，现缺 `.mcw`。
7. **缺必需请求头**：SKILL.md 要求 `User-Agent`（Chrome UA）。

## 设计决策（已与需求方确认）

- **范围**：7 个方法全部按 skill 文档修正确。
- **FetchQuote**：理杏仁无实时行情 API → 用 candlestick 最新收盘价近似，标注为延迟数据。
- **基金元数据**：多接口聚合填满 `FundInfo`（profile + manager + drawdown + net-value）。
- **测试**：httptest mock + 真实 API 抓取的响应体作 fixture。
- **速率限制**：完整实现 SKILL.md 的多级退避重试，并提供配置开关。

## 架构与文件拆分

`lixinger.go` 现 568 行职责过载。按域拆分为单一职责文件：

| 文件 | 职责 |
|---|---|
| `client.go`（新） | 传输层：请求头、`request()`、信封校验（code:1）、错误透出、退避重试 |
| `lixinger.go` | struct、`New`/`Init`、symbol 转换、`Name`/`SupportedMarkets`/生命周期 |
| `stock.go`（新） | `FetchQuote`（candlestick 最新收盘近似）、`FetchHistory`（candlestick） |
| `fundamental.go`（新） | `FetchFundamental` / `FetchFundamentalHistory`（non_financial） |
| `valuation.go` | `FetchValuationPercentile`（修 code 判定 + 指数 `.mcw`），其余沿用 |
| `fund.go`（新） | `FetchFundQuote` / `FetchFundHistory` / `fetchFundInfo`（多接口聚合） |

所有方法通过 `client.go` 的统一 helper 发请求，信封语义只定义一处。

## 组件设计

### 1. 传输层 `client.go`

```
request(endpoint string, payload any) ([]byte, error)
```

- 请求头：`Content-Type: application/json` + `User-Agent`（Chrome UA）。
- 信封：解出 `code`；`code != 1` 返回错误，透出 `error.message`，校验错误透出
  `error.messages[].message`。
- HTTP 非 200 视为错误（沿用现状）。
- 退避重试（见下）。

替换现有重复的 `postJSON` / `postJSONRaw` 两个 helper。

#### 重试策略

- 429 / 5xx：固定调度重试，最多 5 次，间隔 1 / 2 / 4 / 8 / 16 秒。
- 4xx（参数验证、鉴权、会员过期）：不重试，直接返回错误。
- **配置开关**：默认开启（遵循 SKILL.md）；可关闭。

#### 配置开关接线

- YAML `collectors.lixinger` 下新增 `retry: true`（落入 `CollectorConfig.Extra`，
  `mapstructure:",remain"`）。
- 构造器改为函数式选项：`lixinger.New(apiKey string, opts ...Option)`，新增
  `WithRetry(enabled bool)`。保留 `NewWithBaseURL` 供 httptest 注入。
- `cmd/atlas/serve.go` 读 `collectorCfg.Extra["retry"]`（缺省 `true`）并传入构造器。

### 2. 个股行情与 K线 `stock.go`

| 方法 | 端点 | 关键点 |
|---|---|---|
| `FetchHistory` | `cn/company/candlestick` | 单数 `stockCode`；`type: "fc_rights"`（标准前复权，与 eastmoney 一致）；`startDate`/`endDate`；删 `metrics`；响应日期 **RFC3339** 解析；`volume` 整数；空数据不报错 |
| `FetchQuote` | `cn/company/candlestick` | 拉最近约 7 天取最后一条；close→Price，open/high/low 填齐；上一条 close 算 `Change`/`ChangePercent`；`Source: "lixinger-delayed"` |

### 3. 个股基本面与估值 `fundamental.go` + `valuation.go`

| 方法 | 端点 | 关键点 |
|---|---|---|
| `FetchFundamental` | `cn/company/fundamental/non_financial` | `metricsList`；`dyr`/`mc`/`pe_ttm`/`pb`/`ps_ttm`；ROE 不提供 → `core.Fundamental.ROE` 置零并注释 |
| `FetchValuationPercentile`（个股） | `cn/company/fundamental/non_financial` | 仅经统一信封修复即生效 |
| `FetchValuationPercentile`（指数） | `cn/index/fundamental` | 指标 `pe_ttm.y{N}.mcw.cvpos`；`digFloat` 路径加 `mcw` 层 |

> ROE 保留字段置零，不改 `core.Fundamental` 类型，避免影响其他 collector。

### 4. 基金 `fund.go`（多接口聚合）

| 方法 | 端点 | 关键点 |
|---|---|---|
| `FetchFundHistory` | `cn/fund/net-value` | 单数 `stockCode`；字段 `netValue`；RFC3339 |
| `FetchFundQuote` | `cn/fund/net-value` + `fetchFundInfo` | 取最近一条净值当最新价 |
| `fetchFundInfo` | 聚合 4 接口 | `profile`(c_name/f_c_name/inception_date/op_mode) + `manager`(现任) + `drawdown`(最大回撤) + `net-value`(最新净值)；AnnualizedReturn 与 FundSize 无直接字段，本轮留空（不自算）；**任一子接口失败不致命，缺哪个留空哪个** |

## 数据流

- eastmoney 主路径失败 → 调用 lixinger fallback（quote/history/fund）→ 经 `client.request`
  → 信封校验 → 字段映射 → 返回 `core.Quote`/`core.OHLCV`/`core.FundInfo`。
- 估值分位：`app` → `valuationSrc.FetchValuationPercentile` → `endpointFor` 选端点
  （个股/指数/HK/US）→ `client.request` → `digFloat` 解析 cvpos → 返回百分位。

## 错误处理

- 不支持的 symbol（commodity/crypto/非一期指数）：`FetchValuationPercentile` 返回
  `(-1, error)`，调用方降级为「分位不可用」（沿用现状）。
- 子接口失败（基金聚合）：降级留空，不阻断主数据。
- 429/5xx：按开关退避重试；4xx 直接失败。

## 测试策略

每个方法一组 httptest 用例，fixture 用真实 API 抓取的响应体，断言：

- 请求落在正确端点；
- 参数形状正确（单/复数 stockCode、metricsList、type、mcw）；
- `code:1` 解析为成功、`code:0` 解析为失败；
- 字段映射正确；RFC3339 日期解析正确；
- 空数据不报错；
- 重试：429/5xx 触发重试、4xx 不重试、开关关闭时不重试。

已存在的 `internal/collector/lixinger/history_test.go` 是该模式范本，对齐补齐其余方法。

## 非目标（YAGNI）

- 不新增 ROE 等 non_financial 端点不提供的指标来源。
- 不实现基金规模与年化收益（API 无直接字段，不自算）。
- 不做指数估值的等权/算术加权变体（仅市值加权 mcw）。
- 不改 `core` 类型定义。
