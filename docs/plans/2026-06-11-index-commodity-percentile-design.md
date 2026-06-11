# 指数/大宗商品采集 与 历史百分位监控策略 — 设计文档

> 日期：2026-06-11
> 状态：设计已确认（用户逐节批准）；rev2 — 按 spec 审查结论将美/港股估值分位从「当前 EPS 近似」改为「Yahoo 历史 EPS(TTM) 重建真实 PE 序列」（用户确认）；rev3 — 美/港指数估值分位移入一期边界（Yahoo 财报端点不覆盖指数符号），PE 重建逻辑落点定为 `internal/valuation` 纯函数包；rev4 — 按用户补充需求引入理杏仁多市场兜底（hk/us company 与 index 接口），美/港指数估值分位恢复一期范围（理杏仁为唯一路径），并补充部分季度亏损的 PE 序列剔除规则；rev5 — 修复 rev4 审查发现的三处一致性缺陷（悬空引用、理杏仁不可用影响面、EPS 不足兜底口径），补指数映射表与 fallback_reason 元数据
> 前置分析：`docs/reviews/2026-06-11-asset-coverage-and-percentile-analysis.md`

## 1. 需求与范围

### 1.1 需求

1. **指数与大宗商品的数据采集**：使 watchlist 可以监控指数和大宗商品。
2. **基于历史百分位的监控策略**：对所有资产类型（股票/指数/ETF/商品/加密货币）提供历史百分位策略，输出标准买卖信号。

### 1.2 已确认的范围决定

| 决定点 | 结论 |
|---|---|
| 分位指标 | 价格分位 + 估值分位（PE），一期同时实现 |
| 指数/商品覆盖 | 国际主要指数（一期清单：^GSPC、^IXIC、^DJI、^HSI）+ A 股指数（000300.SH 等）+ 国际商品期货（GC=F 等） |
| 估值分位数据源 | A 股（股票/指数）走理杏仁 cvpos 精确值；美/港个股接入 Yahoo fundamentals-timeseries 历史 EPS(TTM) 序列重建真实 PE 历史后求分位（spec 审查否决了「当前 EPS 近似法」——其分位与价格分位逐点等价，无信息增量），**理杏仁 hk/us 接口作为兜底**；美/港指数走理杏仁 hk/index、us/index cvpos（唯一路径） |
| 理杏仁兜底（用户补充需求） | 理杏仁开放平台覆盖港美股、指数、基金等市场，作为 Yahoo 路径失败时的兜底数据源；覆盖范围以账号开通的 API 权限为准 |
| 输出语义 | 标准买卖信号（buy/sell/strong_buy/strong_sell），完全复用 Signal → router → notifier 链路 |
| 实现路线 | 方案三：扩展现有采集器 + 双策略 + 类型体系修补（不新建采集器） |

### 1.3 明确不做（一期边界）

- 国内期货（上期所/大商所/郑商所）数据源接入 — 留二期
- 美/港市场以外的国际指数（^N225、^GDAXI 等）— 留二期，避免市场归属语义（交易时段/市场过滤）含混
- 债券/可转债的采集与策略 — 理杏仁有可转债接口，能力记录在案，留二期
- A 股金融股（银行/券商/保险）的 PE 分位 — 理杏仁对金融股使用独立端点（bank/security/insurance），一期沿用 non_financial 端点，金融股 pe_percentile 不出信号，留二期
- PB / 股息率分位
- 估值分位的本地基本面快照库（每日持久化积累）
- `pe_band` 的分位化改造（保留其绝对阈值语义）
- 独立的提醒（alert）模式 — 百分位策略只走信号链路

## 2. 采集层设计

### 2.1 Yahoo 采集器（指数 + 商品期货）

- 符号校验正则从 `^[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?$` 扩展为同时接受：
  - 指数前缀：`^` + 字母数字（`^GSPC`、`^IXIC`、`^DJI`、`^HSI`、`^N225`）
  - 期货后缀：字母 + `=F`（`GC=F`、`CL=F`、`SI=F`、`HG=F`）
  - 注：符号校验只做格式合法性判断，与一期覆盖清单（§1.3）解耦——`^N225` 格式合法但不在一期清单，由 §2.3 的「表外 `^` 符号 warning」兜底
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
- `MarketForSymbol`：为 `^` 前缀符号维护显式市场映射表（`^GSPC`/`^IXIC`/`^DJI` → US，`^HSI` → HK）；表外的 `^` 符号默认 US 并输出 warning（与一期指数清单边界一致）；`=F` 后缀 → US

### 2.4 lixinger 采集器（估值分位数据）

- 新增方法 `FetchValuationPercentile(symbol string, lookbackYears int) (float64, error)`，按符号市场分派端点：
  - A 股股票走现有 `cn/company/fundamental/non_financial` 接口（与 lixinger.go 现状一致），请求 `pe_ttm.y{N}.cvpos`（lookback 映射 y3/y5/y10）；金融股该端点不适用，返回不可用（一期边界，见 §1.3）
  - A 股指数走 `cn/index/fundamental` 接口同字段
  - **港/美股票与指数（兜底/补缺，用户补充需求）**：分派到 `hk/company`、`us/company`、`hk/index`、`us/index` 对应 fundamental 接口同字段；个股符号做理杏仁代码映射（如 `0700.HK` → `00700`）；一期 4 个指数的映射表如下（理杏仁代码列在实现首日经对应 basic-info / samples 接口核对后固化）：

    | watchlist 符号 | 指数 | 理杏仁接口 | 理杏仁代码（待核对固化） |
    |---|---|---|---|
    | `^GSPC` | 标普 500 | `us/index` | SPX（候选） |
    | `^IXIC` | 纳斯达克综合 | `us/index` | COMP（候选） |
    | `^DJI` | 道琼斯工业 | `us/index` | DJI（候选） |
    | `^HSI` | 恒生指数 | `hk/index` | HSI（候选） |

  - 注：港美股数据的可用性取决于理杏仁账号开通的 API 权限；权限不足时返回不可用，按 §5 降级
  - 注：理杏仁 hk/us company 接口是否如 cn 一样区分非金融/银行/券商/保险端点未核实——美/港金融股的主路径（Yahoo EPS 重建）不受影响，仅兜底路径可能受限，实现时核实并按实际情况收窄兜底范围
- `core.Fundamental` 新增字段 `PEPercentile float64`（0–100；-1 表示不可用）
- **兜底角色**：理杏仁在本设计中承担两类角色——① 美/港指数估值分位的**唯一路径**（Yahoo 财报端点不覆盖指数）；② 美/港个股估值分位在 Yahoo EPS 重建失败时的**兜底路径**（cvpos 精确值，`method` 标注 `lixinger_cvpos`）

### 2.5 Yahoo 历史 EPS 采集（美/港个股估值分位的数据基础）

- **仅适用于个股**（含 `.HK`）；指数符号（`^` 前缀）无财报数据，不在本接口能力范围——美/港指数的估值分位由理杏仁 hk/us index 接口提供（§2.4、§3.2）
- yahoo 采集器新增方法 `FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)`：
  - 端点：`query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries/{symbol}`，请求 `type=trailingDilutedEPS`（TTM 摊薄 EPS，按季度时间戳返回），`period1/period2` 覆盖 lookback 窗口
  - 该端点对 `.HK` 符号同样有效（Yahoo 覆盖港股基本面）
- 新增类型 `core.EPSPoint{Date time.Time; EPS float64}`
- 数据质量门槛：lookback 窗口内有效（EPS > 0）季度点 < 8 个时视为数据不足，估值分位不可用
- 风险注记：该端点为非官方接口，存在反爬/格式变更风险；实现需带 UA 头与现有 yahoo 采集器一致的限流/重试，失败时按 §5 降级

### 2.6 资产类型自动识别（`internal/app.DetectType`）

- `^` 前缀 / 指数代码表命中 → `index`
- `=F` 后缀 → `commodity`
- 其余规则不变（crypto/stock）

## 3. 策略层与类型体系设计

### 3.1 `price_percentile` 策略（新增 `internal/strategy/price_percentile/`）

- 适用资产：全部（stock/index/etf/fund/commodity/crypto；fund 以 NAV 历史作为价格序列）
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

- 适用资产：stock + index（通过 `AssetTypes` 声明）
- 分位获取路径（策略本身不感知差异，由 AnalysisContext 提供 `PEPercentile`）：
  - **A 股**（非金融股票/指数）：理杏仁 cn 接口 cvpos 精确值，`Metadata` 标注 `method: "lixinger_cvpos"`
  - **美/港个股**：主路径为真实 PE 序列重建 — 取 §2.5 的历史 EPS(TTM) 季度序列，对 lookback 窗口内每个日线收盘价 `PE_t = price_t / EPS_TTM(t)`（EPS 取该日之前最近一个季度点，阶梯函数对齐），对当前 PE 在该序列中求分位，`Metadata` 标注 `method: "reconstructed"`；主路径失败时**兜底走理杏仁 hk/us 接口 cvpos**（§2.4）
  - **美/港指数**：理杏仁 hk/index、us/index cvpos（唯一路径，Yahoo 无指数财报数据）
  - 部分季度亏损的重建规则：EPS(TTM) ≤ 0 的季度所对应区间的交易日**剔除出 PE 序列**（PE 无意义），剔除后有效季度点仍按 ≥ 8 个门槛兜底，不足则该路径不可用
  - 重建逻辑实现为独立纯函数包 `internal/valuation`（`ReconstructPEPercentile(closes []core.OHLCV, eps []core.EPSPoint) (float64, error)`），由 app 层 Context 组装阶段调用，策略只消费 `Fundamental.PEPercentile`
  - 注：早期方案「价格 ÷ 当前（常数）EPS」已被 spec 审查否决——常数缩放不改变分位，结果与价格分位逐点等价，无信息增量
- 信号规则与阈值结构同 price_percentile（默认 low=20 / high=80，极值 10/90）
- 美/港个股主路径不可用（当前 EPS ≤ 0、有效 EPS 季度点 < 8、端点失败）时走理杏仁兜底（§2.4）；兜底亦不可用（无 key / 无权限）才不出信号——与 §5 错误处理表口径一致
- 兜底产出的信号在 `Signal.Metadata` 额外记录降级原因（如 `fallback_reason: "yahoo_eps_insufficient"`），便于区分主路径与兜底信号的数据质量
- 现有 `pe_band` 保留不动；配置示例中删除从未生效的 `lookback_years`/`threshold_percentile` 误导参数

### 3.3 类型体系修补

- `core.AssetType` 增加 `AssetCrypto AssetType = "crypto"`
- 启用 `AssetTypes` 过滤，**校验落点在 app 装配层**（engine 不感知 watchlist）：App 启动及 watchlist 增改时，将每个绑定与策略 `RequiredData().AssetTypes` 交叉校验，不匹配则输出 warning 日志并跳过该绑定（不静默、不崩溃）；engine 仅提供 `RequiredData` 查询，不改其分析职责
- 既有策略补填 `AssetTypes`：ma_crossover 全资产（含 fund/etf）；pe_band / dividend_yield 仅 stock

## 4. 数据流

以沪深 300 为例：

```
watchlist: {symbol: 000300.SH, type: index, strategies: [price_percentile, pe_percentile]}
   │ ticker 周期触发
   ▼
selector → eastmoney（指数代码表命中）
   ├─ FetchQuote   → 当前点位
   └─ FetchHistory → 5 年日线（TTL 缓存）
lixinger.FetchValuationPercentile → PE 分位（cvpos）
   （Context 组装阶段的独立调用，与行情采集器并列，非 eastmoney 内部步骤）
   ▼
AnalysisContext{OHLCV, LatestQuote, Fundamental.PEPercentile}
   ▼
price_percentile / pe_percentile 各自 Analyze
   ▼
Signal（含 percentile 元数据）→ router（min_confidence、冷却）→ Telegram / Email / Webhook
```

- 商品（GC=F）走 yahoo，只绑定 price_percentile
- 美/港指数（^GSPC、^HSI）：行情走 yahoo，估值分位走理杏仁 us/index、hk/index
- 美/港个股 pe_percentile：Context 组装阶段调用 `yahoo.FetchEPSHistory`（§2.5），经 `internal/valuation.ReconstructPEPercentile` 重建真实 PE 序列后计算分位；失败时兜底理杏仁 hk/us cvpos

## 5. 错误处理

| 故障场景 | 行为 |
|---|---|
| 价格历史不足（< 1 年） | price_percentile 不出信号，debug 日志说明样本量 |
| 理杏仁不可用 / 无 API key | 三类影响：① A 股 pe_percentile **不出信号**（eastmoney 无 EPS 能力，不存在可用的近似路径）；② 美/港**指数** pe_percentile **不出信号**（理杏仁是唯一路径）；③ 美/港**个股**失去兜底，仅剩 Yahoo EPS 重建主路径。warning 日志按标的去重（每标的仅首次告警，避免每轮 ticker 重复刷日志） |
| Yahoo EPS 端点失败 / EPS 季度点 < 8 | 美/港个股 pe_percentile 先兜底理杏仁 hk/us cvpos；兜底也不可用（无 key / 无权限）才不出信号，不影响 price_percentile |
| 理杏仁港美股权限不足 | 对应标的估值分位返回不可用，按上行降级；warning 按标的去重 |
| 当前 EPS ≤ 0 或缺失 | pe_percentile 跳过该标的，不影响 price_percentile |
| Yahoo 对 `^`/`=F` 符号返回异常 | 与现有股票路径相同的错误传播；单标的失败不影响其他标的（沿用 `analyzeSymbolSafe` 隔离） |
| 策略绑定到不支持的资产类型 | 启动/增改 watchlist 时 warning + 跳过该绑定（app 装配层校验） |

## 6. 测试策略

- **采集层**：
  - yahoo 符号校验表驱动测试（`^GSPC` / `GC=F` / 非法符号）；URL percent-encoding 断言
  - yahoo `FetchEPSHistory` 走 httptest 模拟：正常季度序列、空数据、字段缺失、EPS 为负
  - eastmoney 指数 secid 映射测试（含 `000001.SH` 与 `000001.SZ` 歧义用例）
  - lixinger cvpos 走 httptest 模拟（沿用 `lixinger_httptest_test.go` 模式）；端点分派表驱动测试（A 股股票/A 股指数/港股/美股/美港指数 → cn/hk/us × company/index）与符号代码映射用例；权限不足（API 错误码）返回不可用
- **valuation 包（`internal/valuation`）**：
  - PE 序列重建：EPS 阶梯函数对齐正确性（季度边界前后取值）；重建分位与价格分位**不等价**的回归用例（EPS 变动期）；EPS 点不足 / 全负的错误返回
- **策略层**：
  - 百分位计算边界：空序列、单点、全相同值、当前价为历史最高/最低
  - 四档信号阈值与置信度映射
  - 样本不足 / EPS 为负 / PEPercentile 不可用的跳过路径
- **装配层**：AssetTypes 过滤的 warning + 跳过行为（app 启动与 watchlist 增改两条路径）
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
  - {symbol: "^GSPC",     name: "标普500",   type: index,     strategies: [price_percentile, pe_percentile]}  # 估值分位走理杏仁 us/index
  - {symbol: "000300.SH", name: "沪深300",   type: index,     strategies: [price_percentile, pe_percentile]}
  - {symbol: "AAPL",      name: "苹果",      type: stock,     strategies: [price_percentile, pe_percentile]}  # 主路径 PE 序列重建，兜底理杏仁
  - {symbol: "GC=F",      name: "COMEX黄金", type: commodity, strategies: [price_percentile]}
  - {symbol: "BTC-USDT",  name: "比特币",    type: crypto,    strategies: [price_percentile]}
```
