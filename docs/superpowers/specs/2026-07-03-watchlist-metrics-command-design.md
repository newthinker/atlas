# watchlist 指标命令 — 设计文档

> 日期:2026-07-03
> 状态:设计已确认(brainstorming 逐节评审通过)
> 定位:新增 CLI 命令 `atlas watchlist`,离线拉取当前 watchlist 全部标的的行情/估值/百分位指标

## 0. 一句话目标

`atlas watchlist` 一条命令输出 watchlist 所有标的的最新价、涨跌幅、PE/PB/股息率、
PE 百分位与价格百分位——组装口径与分析循环**同源**(复用 `buildFundamental`),不另起炉灶。

## 1. 已确认的范围决定

| 决定点 | 结论 | 理由 |
|---|---|---|
| 运行形态 | 离线自装配(仿 export-signals),不依赖运行中的 serve | 随时可用;运行时内存 watchlist 属已知边界 |
| 指标范围 | 行情 + 估值 + PE/价格百分位 | 与策略/telegram digest 视角一致,复用现有组装链 |
| 输出格式 | 终端对齐表格(默认)+ `--json` | 人看 + 脚本消费;csv 不做 |
| 架构 | 方案 A:App 导出 `SnapshotMetrics` + 提取 `buildCollectors` | 与分析循环同一条组装链路,口径永不漂移 |
| 命令名 | `atlas watchlist` | 无参即显示指标表,为未来子命令留空间 |

## 2. 命令界面

新文件 `cmd/atlas/watchlist.go`,cobra 子命令。

### Flags

- `--config`:沿用全局约定(默认 `configs/config.yaml`)
- `--json`:输出 JSON 数组代替表格
- `--symbols 600519.SH,AAPL`:可选过滤,只查 watchlist 中的指定标的;
  不在 watchlist 里的标的打 warning 并跳过

### 表格输出(默认)

```
SYMBOL      NAME      MARKET  PRICE     CHG%    PE     PB    DYR%   PE%ILE  PX%ILE
600519.SH   贵州茅台   A股     1408.00   -0.5%   19.5   5.96  4.03   12.3    18.0
AAPL        Apple     美股    213.50    +1.2%   32.1   —     0.44   78.5    85.2
```

- CJK 对齐:复用 telegram notifier 的 `displayWidth`/`padRight`/`isWide`,
  **提取到新共享包 `internal/text`**,telegram 与 CLI 共同引用;
- 拿不到的指标显示 `—`(指数无 PB、加密货币无估值等);
- 末行打印数据缺口摘要(哪些标的哪些指标缺失及原因),风格对齐评估管线 data-gap。

### JSON 输出(`--json`)

- 每标的一个对象,字段 snake_case(`symbol/name/market/type/price/change_pct/pe/pb/
  dividend_yield/pe_percentile/price_percentile`),缺失字段为 `null`;
- 缺口摘要放顶层 `gaps` 数组;
- 日志走 stderr,stdout 只有 JSON。

### 退出码

- 全部标的拉取失败 → exit 1;
- 部分失败 → exit 0 + 缺口摘要(对齐评估管线"单标的失败不中断整体"的降级哲学);
- config 缺失/解析失败、无任何 collector 可用 → exit 1。

## 3. 核心组装 `App.SnapshotMetrics`

`internal/app` 新文件 `snapshot.go`:

```go
type SymbolMetrics struct {
    Symbol, Name, Market, Type string
    Price, ChangePct           float64  // 行情
    PE, PB, DividendYield      *float64 // 估值,nil = 不可用
    PEPercentile               *float64 // 0-100,nil = 不可用
    PricePercentile            *float64 // 0-100,nil = 不可用
    Gaps                       []string // 该标的的数据缺口原因
}

func (a *App) SnapshotMetrics(ctx context.Context, symbols []string) []SymbolMetrics
```

### 每标的组装流程(全部复用现有私有方法)

1. **行情**:`orderedCollectors(symbol)` 依序试 `FetchQuote`
   (与分析循环同一路由链:qlib 仓库优先、外部兜底);
2. **估值 + PE 百分位**:调用现成 `buildFundamental`
   (内含 A 股 lixinger cvpos、美/港 Yahoo EPS 重建、兜底链与 fallback_reason,零重复实现);
3. **价格百分位**:拉 `cfg.Valuation.LookbackYears` 窗口的日线收盘序列
   (`lookback_years:0` = 全历史,语义与策略一致),
   `valuation.PercentileRank(closes, 最新价)`;
4. 任一步失败 → 对应字段置 nil、原因记入 `Gaps`,
   不中断该标的其余指标,也不中断其他标的。

### 并发与隔离

- errgroup 模式,worker 数复用 `cfg.Analysis.Workers`(默认串行,配置即并行);
- 每标的 panic 恢复隔离(仿 `analyzeSymbolSafe`)。

### symbols 参数语义

nil = 全 watchlist;非 nil = 过滤子集(命令层已校验属于 watchlist)。

### 为什么放 App 而非独立包

`buildFundamental`/`orderedCollectors`/lookback 都是 App 私有状态(估值源、warnOnce、
lookback 配置),提取是大手术;加一个只读导出方法是最小切口,且保证与分析循环口径同源。

## 4. 装配提取与命令实现

### `buildCollectors`(cmd/atlas 包内,serve 与新命令共用)

- 把 serve.go 的 collector 装配段原样搬出:yahoo/eastmoney/crypto 注册(含 `maybeCache`
  缓存装饰)、`wireQlibWarehouse`、lixinger 估值源 `SetValuationSources`、
  `SetValuationLookback`;
- 签名:`buildCollectors(cfg *config.Config, application *app.App, log *zap.Logger) (cleanup func(), err error)`
  (cleanup 关 qlib 仓库 DB 句柄);
- serve.go 改为调用它,行为零变化(由现有 serve 测试兜底);
- **不搬**:notifier、策略、broker、alert、API server——新命令都不需要。

### 命令流程

```
loadConfig → app.New(静默 logger;--json 时日志走 stderr)
→ buildCollectors → SetWatchlist(config watchlist)
→ SnapshotMetrics(ctx, filterSymbols) → render(表格 or JSON)
```

不调用 `app.Start()`:无分析循环、无信号、无通知副作用。
仿 `export_signals.go` 的 deps 注入模式(`watchlistDeps{snapshot, out}`),
`executeWatchlist` 可单测。

## 5. 测试

- `internal/app/snapshot_test.go`:注入 fake collector/估值源——
  全指标成功、估值缺失置 nil、单标的 panic 隔离、symbols 过滤、并发与串行等价;
- `cmd/atlas/watchlist_test.go`:deps 注入测渲染——表格对齐(含 CJK 名称)、`—` 占位、
  JSON schema(null 字段、gaps 数组)、全失败 exit 1、`--symbols` 不在 watchlist 的 warning;
- `internal/text` 提取:迁移 `displayWidth/padRight/isWide` 及原 telegram 对应测试,
  telegram 侧改为引用共享包(既有测试防回归)。

## 6. 明确不做(YAGNI,本期边界)

- 不做 `--watch` 轮询刷新;
- 不做排序/着色 flag;
- 不读运行时 serve 的内存 watchlist(离线形态的已知边界);
- 不做 csv 输出;
- 不加技术指标列(MA 等,symbol_detail 已有按需查询路径)。

## 7. 验收标准

- `atlas watchlist` 对含 A 股/美股/指数/加密货币的混合 watchlist 输出对齐表格,
  估值缺失以 `—` 呈现且缺口摘要注明原因;
- `atlas watchlist --json | jq .` 可解析,缺失字段为 null;
- `atlas watchlist --symbols 600519.SH` 只输出该标的;
- serve 行为零变化(现有测试全绿);
- `go test ./...` 全绿(离线,fake 数据源)。
