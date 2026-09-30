# Tiingo 美股备用数据源 设计

日期：2026-09-30 · 状态：**已实施并上线**（PR #63，sprint-051，2026-09-30 部署；serve 21:10 重启后注册生效）

> 2026-10-01 按实现同步修订：正文已改为与代码一致，修订处标 **〔实施后修订〕**；差异汇总见 §8，已知遗留见 §9。
> 2026-10-01 §9 的 C/D 组（L1、L2、L4、L5、L10、L11、L12、L14、S1）已由 `fix/tiingo-hardening`（`7ef6ac2`）解决，正文相应处同步为修复后的行为。
> 裁决出处：`.arcforge/archive/sprint-051-2026-09-30/docs/01-design/architecture-decisions.md`（AD-11…AD-24）、
> QA 报告 `…/05-review/qa-review-round1.md`（W2、W3、L1…L14）。

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
                            ├ client.go    直连 + 头部鉴权 + policy gate(tiingo.daily) + 错误映射
                            ├ normalize.go 拆股折算 + 按日截取〔实施后修订：从 client 拆出，AD-17〕
                            └ collector.go collector.Collector 实现
```

### 3.1 `internal/collector/tiingo/client.go` 与 `normalize.go`

- `New(apiKey string) *Client`、`NewWithBaseURL(apiKey, baseURL string) *Client`；gate 在构造时快照 `policy.Default()`（同 twelvedata）。
- **强制直连**：Transport 为 `http.DefaultTransport.(*http.Transport).Clone()` 再置 `Proxy = nil`，`Timeout: 30s`，不继承 `HTTP(S)_PROXY`。〔实施后修订：原写裸 `&http.Transport{Proxy: nil}`；Clone 保留默认的连接池与 TLS 握手超时，更稳健（QA L6）〕
- 请求：`GET {base}/tiingo/daily/{ticker}/prices?startDate=YYYY-MM-DD`，**不带 `endDate`**；请求头 `Authorization: Token <key>`；token 不进 URL。
- `FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error)`：经 `policy.Fetch(gate, "tiingo.daily", key, …)`；key = `symbol|start|end`（日粒度）。返回按时间升序、`[start, end]` 闭区间、已按 §2.4 折算的 OHLCV（O/H/L/C ÷ 累计因子，Volume × 累计因子并四舍五入），`Interval="1d"`，返回前 `slices.Clone`。
- `normalize`（独立文件 `normalize.go`，AD-17）：折算在**截取之前**对全量行进行。
  - 坏日期的行整行丢弃；但若它带非 1 的 `splitFactor` 则**整段失败**（无法定位在时间轴上的因子不能静默丢，§9 L1 已解决）；
  - 同一日期出现两行 → **整段失败**（无法判断哪行可信，§9 L2 已解决）；
  - O/H/L/C 任一为 null 的行**不输出，但其 `splitFactor` 仍参与累乘**〔实施后修订：spec 原只写「单行不可解析 → 跳过」，未覆盖拆股当日缺价这一边界；连因子一起丢会让此前全部价格错一个倍数〕；
  - 任一 `splitFactor` 非有限或 ≤ 0 → 整段失败。
- `wrapErr` 为唯一 error 出口：前缀 `tiingo: `，替换 token 为 `<redacted>`，`%v` 断链（同 twelvedata ADR#7）；`mapPolicyErr` 把 `policy.ErrTimeout` / `ErrQuotaExceeded` 映射为含 `retryable` 的临时错误。
- HTTP 状态映射：404 → `not found: <detail>`；400/401/403 → `permission/config error (not retryable): HTTP <code>: <detail>`；429 → `rate limited (retryable)`；其他非 200 → `HTTP <code>: <detail>`。`detail` 优先取响应的 `{"detail": …}`，否则取 body 前 200 字节——**先脱敏再截断**〔实施后修订：TASK-004 返工，先截断会让横跨截断点的 key 只剩前缀、脱敏匹配不到〕。

### 3.2 `internal/collector/tiingo/symbols.go`

- `Supported(symbol string) bool` = `^[A-Z]{1,5}([.-][A-C])?$` **且** `collector.MarketForSymbol(symbol) == core.MarketUS`。〔实施后修订：原写 `^[A-Z0-9.\-]{1,10}$`。人类裁决 P11（AD-19）+ Leader 细化（AD-22）：原正则放行 `SAP.DE`、`7203.T`、`931151.CSI` 等——`MarketForSymbol` 对它们兜底 US，会发请求、耗配额；基底排除数字、份额类后缀限 A–C，是因为 `HSBA.L` 与 `BRK.B` 同形，只能靠字母集合区分〕
- 形态约束带来的已知拒绝（失败方向安全：少覆盖、不耗配额）：
  - 份额类后缀 `.D` 及以上；
  - **小写代码**（atlas 代码约定全大写）；
  - 路由表加密前缀（`UNI*`、`LINK*`、`ADA*`、`DOT*` 等）会使部分真实美股被拒（AD-12，既有路由行为，不改）。
- `toTicker(symbol)`：`.` 转 `-`（`BRK.B` → `BRK-B`）。集成测试已覆盖 `BRK.B`（2026-10-01 真实调用通过）。
- 2026-10-01 对 runtime 配置全部 63 个代码实测：25 只美股/ETF 全部放行，38 个 A 股/港股/中证指数/美港指数全部拒绝。

### 3.3 `internal/collector/tiingo/collector.go`

实现 `collector.Collector`：

- `Name()` = `"tiingo"`；`SupportedMarkets()` = `[US]`；`Start`/`Stop` 为 no-op。
- `Init` 读 `APIKey`（空则报错），**沿用当前 baseURL** 重建 client〔实施后修订：生产行为不变，使「换 key 生效」可测〕。
- `FetchHistory(symbol, start, end, interval)`：`interval != "1d"` → 错误；`!Supported(symbol)` → `unsupported symbol` 错误，**不发请求**。
- `FetchQuote(symbol)`：取最近 10 个自然日的日线，用最后两根拼装：`Price`=最新收盘、`PrevClose`=前一根收盘、`Change`/`ChangePercent`（百分数，`PrevClose` 为 0 时不计算）、`Open/High/Low/Volume`=最新一根，`Time`=最新 K 线日期，`Source="tiingo_eod"`，`Market=US`；不足两根时 `PrevClose`/`Change*` 为 0。

### 3.4 policy 策略表

`internal/collector/policy/policy.go` 内置表登记：

```go
t.Set("tiingo.daily", Policy{TTL: builtinTTL, Coalesce: true,
    Quota: &Quota{Limit: 40, Window: time.Hour}})
```

- 配额走既有跨进程文件账本 `data/collector-quota.json`（prism 与 serve 的 WorkingDirectory 相同，共用同一份额度）；超额即失败（`ErrQuotaExceeded`）。账本**只按主题计数，不记录代码**。
- TTL：`ApplyTTL` 会用全局 `collector.cache.ttl`（默认 5m）覆盖内置 TTL，故内置表不写长 TTL；runtime 已配置 `collector.topics."tiingo.daily".ttl: 6h`。
- 容量：Yahoo 故障时 serve 一个周期约 60 次，超出 40/h——人类接受（AD-21）。实际消耗还有 ×3 缓存键乘数与 404 重复扣费（§9 W2，AD-24 接受、记入二期）。

### 3.5 prism 接入

- `internal/prism/refresh.go`：新增

  ```go
  type PriceHop struct {
      Name   string
      Client TwelvedataClient // FetchHistory(symbol, start, end) ([]core.OHLCV, error)
  }
  ```

  `Refresh(…, ts TushareClient, td TwelvedataClient, now)` 的 `td` 参数改为 `usHops []PriceHop`；`refreshEngine`、`refreshEdgar` 同步改为 `hops []PriceHop`。`PriceHop.Client` 的类型为 `PriceHistoryClient`，`TwelvedataClient` 保留为其类型别名（§9 L14 已解决）。
- `fetchCloses(us, hops, symbol, start, end)`：yahoo 成功直接返回；否则按顺序试每一跳，零行视为失败（沿用 `errFallbackNoData`），首个成功跳之后不再调用后续跳。
  - 成功文案：`"<symbol>: yahoo price failed (<err>)[, <前序跳> failed (<err>)…], <name> fallback ok"`。〔实施后修订：人类裁决 P17（AD-20）——附上前序失败跳的原因，否则 tiingo 配错会被 twelvedata 的成功永久掩盖；首跳即成功时与改动前逐字一致〕
  - 全部失败：`"price history: <yahoo err>; <name1> fallback: <err1>; <name2> fallback: <err2>"`。
  - `hops` 为空：`"price history: %w"`，与改动前逐字一致。
- `cmd/atlas/prism.go`：`usPriceHops(cfg.Collectors)` 按 `tiingo`、`twelvedata` 顺序组装。tiingo 需 `enabled` 且有 key；twelvedata 沿用既有判据（有 key 即用，不看 `enabled`；不改行为，已在 `config.example.yaml` 注明，§9 L10 已解决）。未配置的跳不加入（防 typed-nil）。

### 3.6 serve 接入

- `internal/collector/registry.go`：`Registry` 增加 `order []string`；`Register` 新名追加、同名覆盖保持原位置；`GetAll` 按 `order` 返回。
  - 〔实施后修订：原文「调用方 4 处……无代码依赖特定顺序」**有误**〕依赖注册顺序的调用方有 5 处：`app.orderedCollectors` 的兜底链（`internal/app/app.go:557`）、`selector.SelectExternalForSymbol` 的兜底（`selector.go:66`）、`buildArbitrator` 按市场取首个支持它的 collector（`cmd/atlas/serve.go:331`）、回测取 `collectors[0]`（`serve.go:171`）、API 把顺序复制进 symbol_detail 的新 registry（`internal/api/server.go:183`）。改动前这些位置的顺序是随机的；有序化让它们变得确定，也让后三处在特定配置下落到 tiingo（§9 W3、L9）。
- `cmd/atlas/collectors.go`：`collectors.tiingo` 启用且 `api_key` 非空时，在 tushare / baostock 之后、qlib 之前注册 `tiingo.NewCollector(apiKey)`，日志 `tiingo collector registered (US price last hop)`。`atlas watchlist` 同样走 `buildCollectors`，一并获得 tiingo 兜底（AD-13）。
- 〔实施后修订：**不做**〕原计划订正「tushare 2nd hop / baostock 3rd hop」日志——有序化后 A 股标的的兜底仍是 yahoo（最先注册）先于 tushare，该说法依旧不成立，改日志超出范围（AD-15、AD-21）。

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
      ttl: 6h            # runtime 已配置
```

`configs/config.example.yaml` 有同结构示例（key 留空，TTL 为注释建议）。未启用或缺 key：两处均不接入，行为与改动前一致。

## 4. 错误处理

| 情形 | 行为 |
| --- | --- |
| 代码不在白名单 | `tiingo: unsupported symbol X`，不发请求、不占配额 |
| 404 | `tiingo: not found: <detail>`（失败不缓存，每个周期都会重新扣配额，§9 W2） |
| 400 / 401 / 403 | `tiingo: permission/config error (not retryable): HTTP <code>: <detail>` |
| 429、gate 超额、gate 超时 | 含 `retryable` 的临时错误 |
| 200 且 0 行（截取后） | 返回空切片；prism 侧按 `errFallbackNoData` 判失败；serve 循环继续下一源 |
| 200 但响应体是 `{"detail": …}` 错误对象 | 报 `error body with HTTP 200: <detail>`，保留原因（§9 S1 已解决） |
| 3xx 重定向 | 不跟随，报 `HTTP 3xx`（避免 Authorization 随重定向送出，§9 L5 已解决） |
| 坏日期行 | 整行丢弃；带非 1 的 `splitFactor` 时整段失败 |
| 同一日期两行 | 整段失败 |
| 价格为 null 的行 | 不输出，`splitFactor` 仍参与累乘 |
| `splitFactor` 非有限或 ≤ 0 | 整段失败 |
| 所有错误 | 经 `wrapErr` 脱敏（不区分大小写）、断链；状态错误先脱敏再截断 |

## 5. 测试（TDD）

- **client**（`httptest`）：请求头含 token 且 URL query 不含 token；不带 `endDate`；NVDA 拆股样本折算（`testdata/nvda_split_sample.json`）与 `[start,end]` 截取；404/400/401/403/429/500 的分类与脱敏；null 字段与坏日期行；`splitFactor` ≤ 0 整段失败；缓存与配额行为（`gate_test.go`）。独立 reviewer 发现计划测试放过 14/21 个变异，缺口已补入 DoD（AD-18）。
- **直连**：〔实施后修订〕改为结构断言 `Transport.Proxy == nil`。原写法「设代理环境变量后请求仍到达 httptest」对 loopback 恒通过——Go 的 `ProxyFromEnvironment` 对 localhost/127.0.0.1 本就不走代理，测不出任何东西。
- **白名单**：表驱动，覆盖放行（`AAPL`、`SPY`、`BRK.B`、`BRK-B`）与拒绝（A 股、港股、指数、期货、加密、数字代码、外国交易所后缀、小写），被拒时 HTTP 计数为 0。
- **collector**：`FetchQuote` 拼装；非 `1d` 拒绝；`Init` 空 key 报错、换 key 生效。
- **Registry**：`GetAll` 顺序等于注册顺序（重复调用 100 次一致）；同名重注册覆盖且保持位置。
- **prism**：第 1 跳成功 / 第 2 跳成功（文案含前序失败原因）/ 全失败 / 零行视为失败 / `hops` 为空时错误逐字等于改动前；新旧 `fetchCloses` 用同一组探针逐字对照。
- **serve 装配**：有 key 时 tiingo 位于 tushare、baostock 之后；未启用或缺 key 不注册。
- **集成**（`//go:build integration`，`ATLAS_TIINGO_TOKEN`）：AAPL、BRK.B 近 30 天非空无 NaN；prism 演练（fake yahoo 失败 + 真实 tiingo）出现 `tiingo fallback ok`。2026-10-01 用生产 token 实跑均通过。

## 6. 上线验证

1. ✅ runtime `configs/config.yaml` 已加 `collectors.tiingo` 与 `tiingo.daily` 6h TTL。
2. ✅ 二进制 2026-09-30 20:57 部署；serve 21:10 重启，日志确认 tiingo 注册于 baostock 之后、qlib 之前。
3. ✅ prism 降级演练（集成测试 `TestFetchClosesTiingoLiveDrill`）通过。
4. 〔实施后修订〕原第 3 条「用配额账本确认 A 股/港股未产生计数」**不可行**：账本只按主题计数、不记录代码。改为对 runtime 全部代码逐个过 `Supported`（§3.2 末条）。
5. ⏳ 生产首次真实降级尚未发生（账本 tiingo 计数为 0，与 Yahoo 健康一致）。观察 prism 日报：出现 `tiingo fallback ok` 即生效。

## 7. 风险

| 风险 | 缓解 |
| --- | --- |
| 进程级代理导致 TLS 失败 | Transport Clone + `Proxy = nil`，结构断言守护 |
| 拆股折算遗漏 | 不带 `endDate` 取到最新；缺价行保留因子；NVDA 样本单测 |
| 配额耗尽（Yahoo 长时间故障） | 小时配额 40 + 超额即失败；runtime 6h TTL。仍有 ×3 乘数与 404 重复扣费（§9 W2） |
| token 外泄 | 只走请求头；`wrapErr` 脱敏断链、先脱敏再截断；token 仅在 gitignored runtime 配置；脱敏不区分大小写、不跟随重定向（§9 L4、L5 已解决） |
| Registry 有序化改变既有行为 | 5 处调用方的顺序由随机变为确定；特定配置下仲裁与回测会落到 tiingo（§9 W3、L9） |

## 8. 实施后差异汇总（2026-10-01）

| # | 位置 | spec 原文 | 实现 | 依据 |
| --- | --- | --- | --- | --- |
| D1 | §3.2 白名单 | `^[A-Z0-9.\-]{1,10}$` + 排除规则 | `^[A-Z]{1,5}([.-][A-C])?$` + US | P11 / AD-19、AD-22 |
| D2 | §3.5 降级文案 | 只写成功跳 | 附前序失败跳原因 | P17 / AD-20 |
| D3 | §3.6 调用方 | 「4 处，无代码依赖顺序」 | 5 处依赖顺序（原文有误） | QA L8、TASK-001 P12 |
| D4 | §3.6 日志订正 | 订正 2nd/3rd hop 日志 | 不做 | AD-15、AD-21 |
| D5 | §3.1 Transport | 裸 `http.Transport` | `DefaultTransport.Clone()` | QA L6 |
| D6 | §3.1 normalize | 在 client.go 内 | 拆为 normalize.go | AD-17 |
| D7 | §3.1 缺价行 | 跳过 | 不输出但保留 splitFactor | 计划自查 |
| D8 | §3.1 状态错误 | 截断 body | 先脱敏再截断 | TASK-004 返工 |
| D9 | §3.3 Init | 重建生产 client | 沿用当前 baseURL | 可测性 |
| D10 | §5 直连测试 | 代理环境变量 + httptest | 结构断言 | loopback 恒通过 |
| D11 | §6 账本核查 | 按代码核查计数 | 不可行，改为白名单实测 | 账本无代码维度 |
| D12 | §3.6 watchlist | 未提及 | 同样获得 tiingo 兜底 | AD-13 |

## 9. 已知遗留（未排期）

来源：QA 第 1 轮（`qa-review-round1.md`）与终验收裁决 AD-24。

仍未排期：W2、W3（A/B 组，待单独设计）、L9（归入 B 组）、L13（AD-11 二期）；不改：L3、L7、S2、测试冗余 M。

| 编号 | 内容 | 现状 / 裁决 |
| --- | --- | --- |
| W2 | 配额乘数：同一美股在一个 TTL 周期内有 3 个缓存键（FetchQuote 的 10 天窗、snapshot 的 FetchHistory、分析循环），最多扣 3 次；gate 先扣配额后请求、失败不缓存，404 标的每周期重复扣；prism 那一跳可能被 serve 饿死 | AD-24 接受，记入二期（负缓存 / 配额优先级） |
| W3 | Yahoo 关闭 ∧ tiingo 与 arbitrator 开启时，仲裁的 US 市场上下文由 tiingo 提供，每次仲裁拉 SPY 耗配额 | AD-24 接受；二期考虑 arbitrator 跳过纯兜底源。当前 runtime 不触发 |
| L1 | 坏日期行带 `splitFactor ≠ 1` 时因子随行丢失，此前价格错倍且 `err=nil` | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：带非 1 因子时整段失败 |
| L2 | 同一日期出现两行时因子乘两次、输出重复 bar | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：整段失败（选报错不选去重：无法判断哪行可信） |
| L3 | 按调用方时区取日历日，上海早上调用时窗口左端可能少一根 bar，与 yahoo 一致 | 不改 |
| L4 | 脱敏区分大小写 | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：`(?i)` + `QuoteMeta` 替换 |
| L5 | 未设 `CheckRedirect`：同主机换端口或 https→http 的重定向会带 Authorization | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：不跟随任何重定向；生产 token 集成测试确认真实接口不受影响 |
| L7 | 小写代码被拒 | 已写入 §3.2，失败方向安全 |
| L9 | 只配 tiingo 与 qlib 时，回测固定用 tiingo，消耗配额 | 待记录 / 待定 |
| L10 | `usPriceHops` 两跳启用判据不一致（tiingo 看 enabled，twelvedata 不看） | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：仅在 `config.example.yaml` 注明，不改行为（改了会让「有 key 但 enabled: false」的配置悄悄失去一跳） |
| L11 | `FetchQuote` 的 `PrevClose != 0` 守卫无测试（删掉会产生 ±Inf 进入 snapshot JSON） | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：补用例，删守卫的变异可杀 |
| L12 | `FetchQuote` 对 `BRK.B` 返回的 `Symbol` 无断言（实现正确） | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：补断言，改用 ticker 形态的变异可杀 |
| L13 | 装配壳（runPrismRefresh 传 nil、传空 key）只做了 diff 审查，`gate_wiring` 扫描不到 | 二期扩展 gate_wiring 扫描范围时覆盖（AD-11） |
| L14 | `PriceHop.Client` 类型名 `TwelvedataClient` 误导 | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：新增 `PriceHistoryClient`，旧名为别名（gitnexus 标 CRITICAL，经文本补查仅 2 处使用、人类同意后实施） |
| S1 | 200 响应体为 `{"detail": …}` 时原因被 decode 错误吞掉 | 已解决（`fix/tiingo-hardening` `7ef6ac2`）：解码失败时回退读取 detail（Tiingo 是否真会这样返回仍未查实） |
| S2 | 只有一根 bar 时 `ChangePercent` 为 0，snapshot 丢掉 `Source=tiingo_eod` | spec §3.3 明确规定不足两根取 0 |
| M | 测试冗余若干（policy 与 tiingo 重复断言 40/h、NVDA 折算三层各测一遍、对 YAML 注释中文字面量断言） | 删不删不影响正确性 |
