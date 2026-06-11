# Qlib 回测验证管线（atlas × qlib 整合一期）— 设计文档

> 日期：2026-06-11
> 状态：设计已确认（用户逐节批准）；rev2 — 按 spec 审查修正：get_data 命令改为本版本的 qlib.cli.data 入口、CSV date 钉死取自重放 bar 时间（修复 ma_crossover 的 GeneratedAt 墙钟问题并列入范围）、基本面依赖策略显式拒绝
> 上游分析：`docs/reviews/2026-06-11-qlib-integration-analysis.md`
> 定位：qlib 整合演进路径的第一步（零侵入回测验证）；ML sidecar 与数据仓库/PIT 为后续独立子项目

## 1. 目标与范围

**目标**：为 atlas 的策略信号建立严肃的历史验证能力——每个 buy/sell 信号的后续收益、超额收益（vs 沪深 300）、胜率——回答「atlas 的信号赚不赚钱」。

### 1.1 已确认的范围决定

| 决定点 | 结论 |
|---|---|
| 首期子项目 | ① 回测验证管线（ML sidecar、数据仓库/PIT 留后续独立迭代） |
| 信号来源 | atlas 新增 `export-signals` CLI 导出**真实实现**产生的信号（拒绝 qlib 表达式复刻，避免实现漂移） |
| 市场范围 | 仅 A 股（BaoStock 生态成熟、qlib 成本模型对 A 股最准） |
| 数据获取 | qlib 社区数据包起步（get_data 一条命令）；BaoStock 增量更新留二期 |
| 评估方法 | **事件研究为主**（atlas 信号是稀疏事件型、watchlist 标的少，与 qlib TopkDropout 截面选股范式错配，不强行套用） |
| 管线形态 | 方案一：Go 导出 + Python 薄评估层，CSV 为唯一跨语言契约 |

### 1.2 明确不做（一期边界）

- TopkDropout 组合回测、IC 截面分析（标的太少无统计意义）
- 美股 / 港股 / 加密货币市场
- BaoStock 增量数据更新管线（社区数据包截止日局限记录在 README）
- 定时调度 / CI 集成（手动 Makefile 触发）
- qlib 表达式复刻策略的交叉校验（二期可选）

## 2. Go 导出端（`atlas export-signals`）

### 2.1 CLI 子命令（`cmd/atlas/export_signals.go`）

```
atlas export-signals --strategies ma_crossover,price_percentile \
  --symbols 600519.SH,000300.SH --from 2021-01-01 --to 2026-06-01 \
  --out signals.csv
```

- 复用 `internal/backtest` 引擎的逐日滚动重放并**直接读取 `Result.Signals`**——已核实 `backtester.go` 的 `Run()` 按 bar 滚动调用 `strat.Analyze` 且原始信号在仓位模拟之前就收集于 `Result.Signals`，无需改引擎即可截获；引擎会把 `sig.Price` 覆写为当日收盘价、`sig.Strategy` 覆写为策略名，与 CSV 契约一致
- 历史数据经现有 collector 路径获取（A 股 → eastmoney `FetchHistory`，指数符号由 `AShareIndexSecIDs` 表支持），与 serve 模式同源
- **策略白名单**：仅接受可离线回测的策略（数据需求不含 Fundamentals，即 ma_crossover、price_percentile）；`--strategies` 传入 pe_band/dividend_yield/pe_percentile 等依赖基本面的策略时**显式报错拒绝**（backtest 的 AnalysisContext 不填基本面，静默输出 0 信号会被误读为「策略从不触发」）
- 引擎对单 bar `Analyze` 错误静默跳过（`backtester.go:71-73`）；导出端对跳过的 bar 计数并在结束时输出 stderr 摘要，与 §3.2 的丢弃样本计数口径对齐

### 2.2 导出口径：router 过滤前的原始信号

冷却、置信度阈值过滤属于执行层决策；评估对象是策略本身的预测能力。导出原始信号流并在报告中注明口径。CSV 保留 confidence，Python 侧做阈值敏感性分析（信息量大于 Go 侧预过滤）。

### 2.3 CSV 契约（唯一跨语言接口）

```csv
symbol,date,strategy,action,confidence,price,metadata
600519.SH,2024-03-15,ma_crossover,buy,0.72,1688.00,"{""fast_ma"":1690.2}"
```

- `date`：信号产生的交易日（YYYY-MM-DD）。**必须取自重放 bar 的时间（`ohlcv[i].Time`），严禁信任 `Signal.GeneratedAt`**——已核实 ma_crossover 设置的是 `time.Now()` 墙钟时间，信任它会把全部历史信号标成导出当日，事件研究静默失效。顺带修复 `ma_crossover/strategy.go:100,117` 的 `GeneratedAt: time.Now()` → `ctx.Now`（与 price_percentile 口径一致），列入实现范围
- `metadata`：JSON 字符串（percentile 值等）。Go 侧用 `encoding/csv` 写、Python 侧用 `csv`/`pandas` 默认 quoting 读，不手写拼接引号
- schema 由 Go 单测（golden file）钉死，字段变更必须显式改契约

## 3. Python 评估端（`scripts/qlib_eval/`）

### 3.1 目录与依赖

```
scripts/qlib_eval/
├── README.md          # 数据包下载、运行步骤、数据截止日局限
├── requirements.txt   # pyqlib（本地 /Users/zuowei/workspace/python/qlib 可编辑安装）、pandas
├── evaluate.py        # 入口：CSV → 事件研究 → markdown 报告
└── tests/test_eval.py # 合成数据驱动的核心计算单测
```

数据初始化（一次性，命令已对照本地 qlib 副本 README:231 核实——`qlib.run.get_data` 是旧版形式，本版本入口是 `qlib.cli.data`）：
`python -m qlib.cli.data qlib_data --target_dir ~/.qlib/qlib_data/cn_data --region cn`

注：社区数据包托管在 GitHub releases（SunsetWolf/qlib_dataset），国内网络可能需要代理；README 局限说明中注明。

### 3.2 事件研究计算口径

- **符号映射**：`600519.SH → SH600519`、`000300.SH → SH000300`（qlib instrument 格式），映射函数独立可测
- **入场价**：信号日的**次日开盘价**（规避前视偏差）；信号日为非交易日/停牌时顺延至下一交易日，顺延超 5 日丢弃该样本并计数
- **指标**：每信号计算 5/20/60 交易日的绝对收益与相对沪深 300（`SH000300`）的超额收益
- **聚合**：按 策略 × action 分组输出 均值/中位数/胜率/样本数；buy 类看正超额，sell 类看规避收益（信号后标的相对基准的下跌幅度）
- **置信度敏感性**：按 confidence 分桶（≥0.6 / ≥0.8）重复聚合，验证 router 的 0.6 阈值是否合理

### 3.3 报告产出

`reports/signal-eval-YYYYMMDD.md`（reports/ 目录 gitignore，报告手动挑选归档）：

- 首页汇总表 + 评估口径说明（原始信号、次日开盘入场、社区数据包截止日）
- 每策略一节：样本数、各 horizon 收益表、胜率、置信度分桶表、丢弃样本统计

### 3.4 Makefile 集成

`make signal-eval`：串联 export-signals（参数从环境变量/默认 watchlist 取）→ evaluate.py。

## 4. 数据流总览

```
eastmoney FetchHistory ──► atlas export-signals ──► signals.csv
                                                        │
~/.qlib/qlib_data/cn_data（社区数据包）──► evaluate.py ──┤
                                                        ▼
                                          reports/signal-eval-*.md
```

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| qlib 数据目录缺失 | evaluate.py 启动时检测，打印 get_data 下载命令后退出（exit code 非 0） |
| 信号标的不在 qlib 数据中 | 跳过并在报告「数据缺口」节列出（如非 A 股符号混入） |
| 信号日期晚于数据包截止日 | 按 horizon 可计算部分输出，不足的标 N/A 并计数 |
| CSV 格式不符 | 显式报错指出行号与字段，不静默跳过 |
| 导出端历史数据拉取失败 | 与现有 backtest 相同的错误传播，单标的失败不中断其他标的导出 |
| `--strategies` 含依赖基本面的策略 | 启动时显式报错拒绝并列出可用白名单（防止静默 0 信号被误读，见 §2.1） |
| 重放中单 bar Analyze 失败 | 引擎静默跳过；导出端计数并输出 stderr 摘要 |

## 6. 测试

- **Go**：export-signals 的 golden CSV 单测（固定合成 OHLCV → 固定信号输出）；CSV schema 字段顺序断言
- **Python**：`test_eval.py` 用合成价格序列验证——次日开盘入场、节假日顺延、超额收益计算、sell 规避收益口径、置信度分桶聚合；不依赖真实 qlib 数据包（数据接口可注入 mock）
