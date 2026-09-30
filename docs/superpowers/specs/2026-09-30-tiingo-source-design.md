# Tiingo 美股备用数据源 设计

日期：2026-09-30 · 状态：已确认设计，待实施计划

## 1. 目标与范围

把 Tiingo（https://www.tiingo.com）接入为**美股日线价格**的备用数据源，覆盖两条路径：

1. **prism 估值刷新**：美股价格降级链由 `Yahoo → Twelve Data` 改为 `Yahoo → Tiingo → Twelve Data`。
2. **`atlas serve` 分析循环 / 快照**：注册为 collector，作为最后一个外部兜底源。

**人类已确认的决策（2026-09-30）**

- 两处都接（prism + serve）。
- 一期**只做美股**（含 ETF）；A 股二期。
- 方案 A：独立 collector 包；prism 价格链改为有序跳列表；`collector.Registry` 改为按注册顺序返回。

**不做（YAGNI / 二期）**

- A 股：实测有系统性缺日与残缺 K 线（§2），且残缺 K 线无法用成交量可靠识别（§2.3），二期再议。
- 港股、指数、期货：Tiingo 不覆盖。
- 加密货币、IEX 盘中、新闻、基本面（免费档仅道指 30）。
- 按日配额：一期只设小时配额，以 40×24=960 < 1000 满足日上限。

## 2. 实测依据（2026-09-30，免费 token）

### 2.1 覆盖与网络

- 公开清单 `supported_tickers.zip`：108,885 行；港股 0 只（HKD 标的全为深市 B 股）；无指数、期货。`/tiingo/daily/0700/prices` → 404 `Ticker '0700' not found`。
- **直连 `api.tiingo.com` 正常（~0.7s）；经本机代理 `127.0.0.1:7897` TLS 握手必然失败**（curl 与 python 均复现；同一代理对 Yahoo 可用）。部分 launchd 任务设置了进程级 `https_proxy`。
- 鉴权支持 `Authorization: Token <key>` 请求头。
- 免费档：500 标的/月、50 次/时、1000 次/日、1GB/月；条款 Internal Use Only。

### 2.2 美股数据质量

AAPL、SPY 2025-09-01…2026-09-29 与 Yahoo chart 逐日比对：271/271 天对齐；`close` 相对差 ≤ 6e-8；`adjClose` 与 Yahoo `adjclose` 相对差 ≤ 3.1e-5。

### 2.3 A 股（一期不做的原因）

600036、601658 对照 Tushare `daily`：缺 3 个真实交易日（2025-12-26、2026-02-04、2026-02-12，两只同缺）；600036 在 2026-06-10 为残缺 K 线（量为全天 57%，high 38.93 vs 39.08）。该残缺 bar 的「量 / 前 20 日中位」= 0.69，而真实日线一年中有 10 天低于 0.6 ⇒ 成交量判据无法区分。

### 2.4 价格口径

| 源 | `close` 含义 |
| --- | --- |
| Yahoo chart `quote.close`（atlas yahoo collector 取用） | 拆股调整、**不**含分红调整 |
| Tiingo `close` | 原始成交价（拆股也未调整） |
| Tiingo `adjClose` | 拆股 + 分红调整 |

折算公式：`t` 日价格 ÷ `t` 之后（不含 `t`）所有交易日 `splitFactor` 之积。NVDA 2024-06-10 拆股 10:1 实测：拆股前 5 天原始价 1150.0… 折算后 115.0…，与 Yahoo `close` 逐日相对差 ≤ 3e-8。**前提：请求不带 `endDate`**，否则 `end` 之后的拆股不可见。

## 3. 架构

```
prism refresh ──► fetchCloses: yahoo ─✗─► [tiingo] ─✗─► [twelvedata]   （有序 PriceHop 列表）
serve 分析/快照 ─► orderedCollectors: 首选 ─✗─► …按注册顺序… ─► tiingo（最后一个外部源）
                                         │
                          internal/collector/tiingo
                            ├ symbols.go   白名单：非美股形态直接拒绝，不发请求
                            ├ client.go    直连 + 头部鉴权 + 拆股折算 + policy gate(tiingo.daily)
                            └ collector.go collector.Collector 实现
```

### 3.1 `internal/collector/tiingo/client.go`

- `New(apiKey string) *Client`、`NewWithBaseURL(apiKey, baseURL string) *Client`；gate 在构造时快照 `policy.Default()`（同 twelvedata）。
- `http.Client{Timeout: 30s, Transport: &http.Transport{Proxy: nil}}`：**强制直连**，不继承 `HTTP(S)_PROXY`。
- 请求：`GET {base}/tiingo/daily/{ticker}/prices?startDate=YYYY-MM-DD`，**不带 `endDate`**；请求头 `Authorization: Token <key>`；token 不进 URL。
- `FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error)`：经 `policy.Fetch(gate, "tiingo.daily", key, …)`；key = `symbol|start|end`（日粒度）。返回按时间升序、`[start, end]` 闭区间、已按 §2.4 折算的 OHLCV（Open/High/Low/Close 除以累计因子，Volume 乘以累计因子并取整），`Interval="1d"`，返回前 `slices.Clone`。
- 拆股折算在**截取之前**对全量行进行。
- 单行 `date`/`close` 不可解析 → 跳过该行；任一行 `splitFactor` 非有限或 ≤ 0 → 整段失败。
- `wrapErr` 为唯一 error 出口：前缀 `tiingo: `，替换 token 为 `<redacted>`，`%v` 断链（同 twelvedata ADR#7）；`mapPolicyErr` 把 `policy.ErrTimeout` / `ErrQuotaExceeded` 映射为含 `retryable` 的临时错误。
- HTTP 状态映射：404 → `not found: <detail>`；400/401/403 → `permission/config error (not retryable): <detail>`；429 → `rate limited (retryable)`；其他非 200 → `HTTP <code>: <截断 body>`。

### 3.2 `internal/collector/tiingo/symbols.go`

- `Supported(symbol string) bool`：`collector.MarketForSymbol(symbol) == core.MarketUS`，且不以 `^` 开头、不以 `=F` 结尾、不命中加密路由；另要求 `^[A-Z0-9.\-]{1,10}$`。
- `toTicker(symbol)`：大小写原样保留（Tiingo 不区分大小写）；`.` 转 `-`（Tiingo 份额类用连字符，如 `BRK.B` → `BRK-B`）。⚠ live 校验点：集成测试需覆盖 `BRK-B`。

### 3.3 `internal/collector/tiingo/collector.go`

实现 `collector.Collector`：

- `Name()` = `"tiingo"`；`SupportedMarkets()` = `[US]`；`Init` 读 `APIKey`（空则报错）；`Start`/`Stop` 为 no-op。
- `FetchHistory(symbol, start, end, interval)`：`interval != "1d"` → 错误；`!Supported(symbol)` → `unsupported symbol` 错误，**不发请求**。
- `FetchQuote(symbol)`：取最近约 10 个自然日的日线，用最后两根拼装：`Price`=最新收盘、`PrevClose`=前一根收盘、`Change`/`ChangePercent`、`Open/High/Low/Volume`=最新一根，`Time`=最新 K 线日期，`Source="tiingo_eod"`，`Market=US`；不足两根时 `PrevClose`/`Change*` 为 0。

### 3.4 policy 策略表

`internal/collector/policy/policy.go` 内置表登记：

```go
t.Set("tiingo.daily", Policy{TTL: builtinTTL, Coalesce: true,
    Quota: &Quota{Limit: 40, Window: time.Hour}})
```

- 配额走既有跨进程文件账本（prism 与 serve 共用一个 key 的额度）；超额即失败（`ErrQuotaExceeded`）。
- TTL：`ApplyTTL` 会用全局 `collector.cache.ttl`（默认 5m）覆盖内置 TTL，故不在内置表写长 TTL；在 `config.example.yaml` 建议 `collector.topics."tiingo.daily".ttl: 6h`（日线一天只更新一次）。

### 3.5 prism 接入

- `internal/prism/refresh.go`：新增

  ```go
  type PriceHop struct {
      Name   string
      Client TwelvedataClient // FetchHistory(symbol, start, end) ([]core.OHLCV, error)
  }
  ```

  `Refresh(…, ts TushareClient, td TwelvedataClient, now)` 的 `td` 参数改为 `usHops []PriceHop`。
- `fetchCloses(us, hops, symbol, start, end)`：yahoo 成功直接返回；否则按顺序试每一跳，零行视为失败（沿用 `errFallbackNoData`）；首个成功跳返回降级文案 `"<symbol>: yahoo price failed (<err>), <name> fallback ok"`；全部失败返回 `"price history: <yahoo err>; <name1> fallback: <err1>; <name2> fallback: <err2>"`；`hops` 为空时错误为 `"price history: <err>"`（与现状逐字一致）。
- `cmd/atlas/prism.go`：按 `tiingo`、`twelvedata` 顺序组装 `usHops`，key 为空的跳不加入（防 typed-nil，同 `twelvedataClientOrNil`）。

### 3.6 serve 接入

- `internal/collector/registry.go`：`Registry` 增加 `order []string`；`Register` 新名追加、同名覆盖保持原位置；`GetAll` 按 `order` 返回。调用方 4 处（`app.go:557/649/953`、`selector.go:66`），现有顺序为 map 随机，无代码依赖特定顺序。
- `cmd/atlas/collectors.go`：`collectors.tiingo` 启用且 `api_key` 非空时，在 tushare / baostock 之后、qlib 之前注册 `tiingo.New(apiKey)` 包装的 collector，并记日志 `tiingo collector registered (US price last hop)`。
- 同时订正同文件中「tushare 2nd hop / baostock 3rd hop」日志的前提：有序 `GetAll` 之后该说法才成立。

### 3.7 配置

```yaml
collectors:
  tiingo:
    enabled: true
    api_key: "<token>"   # 仅 runtime configs/config.yaml（gitignored）
    markets: ["US"]
collector:
  topics:
    tiingo.daily:
      ttl: 6h            # 可选建议
```

`configs/config.example.yaml` 增加同结构示例（key 留空）。未启用或缺 key：两处均不接入，行为与现状完全一致。

## 4. 错误处理

| 情形 | 行为 |
| --- | --- |
| 代码不在白名单 | `tiingo: unsupported symbol X`，不发请求、不占配额 |
| 404 | `tiingo: not found: <detail>` |
| 400 / 401 / 403 | `tiingo: permission/config error (not retryable): <detail>` |
| 429、gate 超额、gate 超时 | 含 `retryable` 的临时错误 |
| 200 且 0 行（截取后） | 返回空切片；prism 侧按 `errFallbackNoData` 判失败；serve 循环继续下一源 |
| 单行字段不可解析 | 跳过该行 |
| `splitFactor` 非有限或 ≤ 0 | 整段失败 |
| 所有错误 | 经 `wrapErr` 脱敏、断链 |

## 5. 测试（TDD）

- **client**（`httptest`）：请求头含 token 且 URL query 不含 token；不带 `endDate`；NVDA 拆股样本折算（`testdata/nvda_split_sample.json`，取 2024-06-03…2024-06-14 实测原始值）与 `[start,end]` 截取；404/403/429/500 的分类与脱敏（错误文本不含 token）；null 字段与坏日期行跳过；`splitFactor` ≤ 0 整段失败。
- **直连**：`t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")`、`HTTP_PROXY` 同设，请求仍到达 `httptest` 服务器。
- **白名单**：表驱动 `AAPL`、`BRK-B`、`BRK.B`、`SPY` 通过；`600036.SH`、`0700.HK`、`^GSPC`、`GC=F`、`BTC-USD`、`BTCUSDT` 拒绝，且 HTTP 计数为 0。
- **collector**：`FetchQuote` 拼装（Price/PrevClose/ChangePercent/Time/Source）；非 `1d` 拒绝；`Init` 空 key 报错。
- **Registry**：`GetAll` 顺序等于注册顺序（重复调用 100 次一致）；同名重注册覆盖且保持位置。
- **prism**：fake hops 覆盖 第 1 跳成功 / 第 2 跳成功（文案跳名正确）/ 全失败（错误串含每跳原因）/ 零行视为失败 / `hops` 为空时错误逐字等于现状。
- **serve 装配**：有 key 时 `GetAll` 中 tiingo 位于 tushare、baostock 之后；无 key 不注册。
- **集成**（`//go:build integration`，`ATLAS_TIINGO_TOKEN`）：AAPL 近 30 天非空、无 NaN；`BRK-B` 可取。

## 6. 上线验证

1. runtime `configs/config.yaml` 增加 `collectors.tiingo`（人类执行或授权）。
2. prism 演练：以 fake/不可达 Yahoo 地址触发降级，报告出现 `tiingo fallback ok`。
3. serve 快照：美股行情可用；检查配额账本，A 股/港股标的未产生 `tiingo.daily` 计数。

## 7. 风险

| 风险 | 缓解 |
| --- | --- |
| 进程级代理导致 TLS 失败 | Transport 显式 `Proxy: nil` + 单测守护 |
| 拆股折算遗漏 | 不带 `endDate` 取到最新；NVDA 样本单测 |
| 配额耗尽（Yahoo 长时间故障） | 小时配额 40 + 超额即失败；建议 6h TTL 覆盖 |
| token 外泄 | 只走请求头；`wrapErr` 脱敏断链；token 仅在 gitignored runtime 配置 |
| Registry 顺序变化影响既有兜底 | 现状即随机，改为确定性只减少不确定性；单测覆盖 |
