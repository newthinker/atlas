# 美股 signal-eval（atlas_us 全管线）— 设计文档

> 日期：2026-06-13
> 状态：设计已确认（用户逐节批准）
> 上游：`docs/plans/2026-06-11-qlib-eval-pipeline-design.md`（CN 一期）+ sprint-009 基准参数化（HK，commit 51b8592）
> 定位：为已实现的 signal-eval 管线增加第三个市场（美股），镜像 HK 模式

## 1. 目标与范围

**目标**：让 signal-eval 事件研究管线支持美股——atlas 导出美股策略信号（逐日重放真实实现）→ 自建 qlib `atlas_us` 数据包做事件研究评估，相对 ^GSPC 计算超额收益。

### 1.1 背景：管线现状

signal-eval 管线已实现并支持 CN + HK：

- **Go**：`export-signals` 逐日重放导出真实信号；`export-ohlcv`（`toQlibInstrument`）导出 per-instrument OHLCV CSV 供建包
- **Python `scripts/qlib_eval/`**：`build_data.py`（调官方 dump_bin 自建 qlib 包）、`symbols.py`（atlas↔qlib instrument 映射，与 Go 侧逐字对称、共享契约测试）、`prices.py`（`QlibPriceSource` 读包）、`event_study.py`（次日开盘入场、5/20/60 日 horizon、相对基准超额）、`evaluate.py`（CLI 入口 + markdown 报告）
- **自建包**：`atlas_cn`（默认）、`atlas_hk`；社区包 cn_data 截止 2020-09 产不出 2021+ 结果，故全部走自建包
- **基准参数化**（sprint-009）：`evaluate.py --benchmark`，CN 用 `000300.SH`、HK 用 `^HSI`

### 1.2 已确认的设计决定

| 决定点 | 结论 |
|---|---|
| 美股基准 | `^GSPC`（标普500，宽基代表，与 CN 沪深300/HK 恒生口径一致） |
| qlib region | **参数化**（`QlibPriceSource` 加 `region` 参数，默认 `cn`；evaluate.py 加 `--region`），美股传 `us` |
| 市场差异组织 | **方案 A：镜像 HK 模式加 US 分支**（不抽象 market 配置表，不重构 CN/HK） |
| 美股标的 | 烘一份精选清单 + Makefile target，开箱即跑 |

### 1.3 不变的边界（与 CN/HK 一致，非美股特有）

signal-eval 只重放 OHLCV 策略（`price_percentile` + `ma_crossover`）；`pe_percentile` 依赖基本面、离线重放拿不到，被 `export-signals` 白名单拒绝——对所有市场成立，美股不引入新例外。

## 2. 符号映射（Go/Python 对称契约）

美股标的两类：裸 ticker（`AAPL`/`MSFT`，无后缀）和指数（`^GSPC`，`^` 前缀）。

| atlas 符号 | qlib instrument | 规则 |
|---|---|---|
| `AAPL` | `AAPL` | 裸 ticker 恒等（已大写） |
| `MSFT` | `MSFT` | 同上 |
| `^GSPC` | `GSPC` | 美股指数：剥离 `^` |

### 2.1 Go 侧（`cmd/atlas/export_ohlcv.go` `toQlibInstrument`）

当前在 HK 分支后 `default` 返回错误。在 `return "", error` 之前插入 US 分支：

```go
case symbol == "^GSPC", symbol == "^IXIC", symbol == "^DJI":
    return strings.TrimPrefix(symbol, "^"), nil   // 美股指数剥离 ^
case usTickerRe.MatchString(symbol):
    return symbol, nil                            // 裸 ticker 恒等
```

`usTickerRe = regexp.MustCompile("^[A-Z]{1,5}$")` —— 只接受 1-5 位纯大写字母。

### 2.2 Python 侧（`qlib_eval/symbols.py` `to_qlib_instrument`）

**逐字镜像** Go 规则（同一正则 `^[A-Z]{1,5}$`、同一美股指数清单 `^GSPC`/`^IXIC`/`^DJI`），由现有共享契约测试覆盖（两侧喂相同样本断言相同输出）。

### 2.3 符号边界（设计的「不做」）

- **含 `.`/`-` 的类别股**（`BRK.B`、`BRK-B`）不被 `[A-Z]{1,5}` 接受 → 精选清单避开；未来需要则单独扩规则并同步两侧
- `^` 前缀指数在 HK（`^HSI`/`^HSCE`）与 US（`^GSPC` 等）间靠**显式清单**区分，不用「剥离 `^`」通配规则——避免 `^HSI` 被误判为美股指数
- `^[A-Z]{1,5}$` 不匹配 A 股数字代码（`600519`）或带后缀符号，三市场互不串台

## 3. region 参数化 + 自建包 + 编排 + 配置

### 3.1 region 参数化（`prices.py`）

`QlibPriceSource.__init__` 当前硬编码 `qlib.init(region="cn")`。改为：

```python
def __init__(self, provider_uri, start, end, benchmark="000300.SH", region="cn"):
    self._region = region
    # _ensure_init 内：qlib.init(provider_uri=..., region=self._region)
```

`evaluate.py` 新增 `--region`（默认 `cn`，向后兼容），透传给 `QlibPriceSource`。默认值保证 CN/HK 行为零变化（其 Makefile target 不传 `--region`）。

### 3.2 自建包（`build_data.py`）

机制与 CN/HK 完全相同（dump_bin 区域无关，日历从 CSV 日期推导）。`atlas_us` 是又一个 target 目录。export-ohlcv 导出 `AAPL.csv`…`GSPC.csv` → dump_bin 编译为 `~/.qlib/qlib_data/atlas_us`。

### 3.3 Makefile

```makefile
QLIB_DATA_US_DIR ?= $(HOME)/.qlib/qlib_data/atlas_us
QLIB_CSV_US_DIR  ?= qlib_csv_us
# 须与 config 的美股集一致
SIGNAL_SYMBOLS_US ?= AAPL,MSFT,NVDA,GOOGL,AMZN,META,JNJ,JPM,^GSPC

qlib-data-us: build          # 导出美股 OHLCV → dump 成 atlas_us 包
signal-eval-us: build        # 导出美股信号 → 对 atlas_us 评估，benchmark ^GSPC, --region us
```

`signal-eval-us` 内：`export-signals --symbols $(SIGNAL_SYMBOLS_US) --strategies price_percentile,ma_crossover` → `evaluate.py --qlib-dir $(QLIB_DATA_US_DIR) --benchmark ^GSPC --region us`。

### 3.4 美股 watchlist 精选清单（config 示例）

watchlist 绑全策略（含 `pe_percentile`，供实盘监控——美股 PE 分位走 Yahoo EPS 重建在实盘 app 工作）；signal-eval-us 只重放 OHLCV 两策略。拟选：

| 类别 | 标的 |
|---|---|
| 科技大盘 | AAPL 苹果、MSFT 微软、NVDA 英伟达、GOOGL 谷歌、AMZN 亚马逊、META |
| 价值/防御 | JNJ 强生、JPM 摩根大通 |
| 基准 | ^GSPC 标普500 |

## 4. 风险、错误处理、测试

### 4.1 核心风险：region="us" 与自建包兼容性（需端到端验证）

参数化 region 语义更正确，但 `region="us"` 路径**无单测覆盖**（qlib 路径惰性导入、单测全程 qlib-free）。理论分析：qlib `D.features` 从包自带日历读取，region 主要影响 backtest/Exchange 默认值——管线只读 `$open/$close`、不用 Exchange，HK 用 `region="cn"` 跑通非 cn 市场已是佐证。但 `region="us"` 可能触发 qlib 默认美股日历与自建包日历的交互（未实测）。

**缓解**：列为交付验收**必跑项**——`make qlib-data-us && make signal-eval-us` 须对真实 atlas_us 包产出非空报告；若 `region="us"` 异常，**回退 `region="cn"`**（HK 已证明可行，日历来自包而非 region）。回退是配置级（改 Makefile 一个参数），不动代码结构。

### 4.2 错误处理（复用现有机制，无新增）

| 场景 | 行为（沿用 CN/HK） |
|---|---|
| atlas_us 包缺失 | evaluate.py 启动检测，打印 build 指引并非 0 退出 |
| US 符号映射拒绝（类别股 BRK.B） | toQlibInstrument 返回错误，export-ohlcv 跳过并计入降级摘要 |
| ^GSPC 行情拉取失败 | 单标的失败不中断，与现有 export 一致 |
| 信号日晚于包数据截止 | horizon 不足标 N/A 并计数 |

### 4.3 测试

- **Go**：`toQlibInstrument` 表驱动加 US 用例（`AAPL→AAPL`、`^GSPC→GSPC`、`BRK.B→error`、`600519.SH` 不回归）；export_ohlcv 既有契约测试样本加 US 行
- **Python**：`test_symbols.py` 镜像同样的 US 用例（Go/Python 对称契约——同输入同输出）；`prices.py` region 参数透传加轻量断言（构造时 region 存字段，不触发 qlib）
- **Makefile**：`test_makefile.py` 加 `signal-eval-us`/`qlib-data-us` target 存在性与关键参数（`--region us`、`--benchmark ^GSPC`、`atlas_us`）断言
- **端到端**：§4.1 必跑项作为人工验收，不进 CI（依赖真实 qlib 数据与网络）

### 4.4 不做（一期边界）

- 含 `.`/`-` 的美股类别股
- 美股期权/ETF 特殊处理
- US PE 分位进 signal-eval（基本面离线不可得，全市场统一边界）
- CI 化端到端
