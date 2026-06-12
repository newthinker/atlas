# qlib_eval — atlas 信号事件研究评估

`atlas export-signals` 导出真实策略信号（CSV）→ 本评估层用 qlib 价格数据做事件研究，
量化每个 buy/sell 信号的后续收益、超额收益与胜率。

CSV 是唯一的跨语言契约；本目录为薄评估层，价格数据源可注入，**pytest 不依赖真实
qlib 数据包与 qlib 安装**。

## 安装

```bash
# 测试期依赖（运行 pytest 所需，零 qlib 依赖）
pip install -r requirements.txt
```

pyqlib 仅在真实评估运行时需要（`QlibPriceSource` 内惰性导入），两种安装方式任选：

```bash
pip install pyqlib
# 或本地副本（开发推荐）
pip install -e /Users/zuowei/workspace/python/qlib
```

## 数据包下载（真实运行前一次性准备）

qlib 中国市场日频数据包（托管在 GitHub `SunsetWolf/qlib_dataset` releases）：

```bash
python -m qlib.cli.data qlib_data \
  --target_dir ~/.qlib/qlib_data/cn_data \
  --region cn
```

注意事项：
- 数据托管在境外，国内拉取**可能需要代理**。
- 数据包有**数据截止日局限**（非实时，最新若干交易日可能缺失）。
- 缺数据目录时 `evaluate.py` 会打印上述下载命令并 `exit(1)`。

## 自建数据包（atlas → qlib，默认路径）

社区包截止 ~2020-09，`make signal-eval` 默认评估区间 2021–2026 在社区包上**产不出结果**。
atlas 用自有采集器构建 qlib 数据包 `~/.qlib/qlib_data/atlas_cn`，与信号生成**同源**
（同一套 collector / 同一 `FetchHistory`），覆盖至当天——这正是本评估链存在的理由。

### 用法

```bash
# 从仓库根：导出 per-instrument OHLCV CSV → 官方 dump_bin 编译为 qlib 数据目录
make qlib-data
# 产出 qlib_csv/*.csv 与 ~/.qlib/qlib_data/atlas_cn/{instruments,calendars,features}
# build_data.py 会只读校验 instruments/calendar 覆盖区间，不通过则非 0 退出

make signal-eval    # 默认 QLIB_DIR 已指向 atlas_cn，直接产出非空报告
```

`qlib-data` 显式传 `--symbols $(SIGNAL_SYMBOLS)`（与 signal-eval 共用），天然保证
「评估符号 ⊆ 数据包」；只传 `--from`、覆盖至当天。

### 复权口径

- 价格 = eastmoney **`fqt=1` 前复权**，与信号侧 `FetchHistory` 天然同源（同一次取数）。
- CSV `factor` 列**恒为 `1`**：价格在源头已前复权，评估端**从不**再乘 `$factor`。
- 与上文「评估口径」一节交叉引用：事件研究的 open/close 即前复权价，超额收益口径不变。

### 社区包 vs 自建包

| 维度 | 社区包 `cn_data` | 自建包 `atlas_cn` |
|------|------------------|-------------------|
| 数据截止 | ~2020-09（非实时） | 当天（每次 `make qlib-data` 重建） |
| 数据源 | 第三方托管 | atlas 自有采集器（与信号同源） |
| 覆盖符号 | 全市场 | `SIGNAL_SYMBOLS`（评估所需子集） |
| 默认 2021–2026 区间 | **产不出结果** | 非空结果 |

**QLIB_DIR 切换方法**：`signal-eval` 默认 `QLIB_DIR = $(QLIB_DATA_DIR)`（即 atlas_cn）。
回退社区包：`make signal-eval QLIB_DIR=~/.qlib/qlib_data/cn_data`。

### 定时重建（crontab 示例）

atlas 不内置调度，用系统 cron 在每个交易日收盘后重建：

```cron
# 每个交易日 16:30 重建 qlib 数据包（日志追加到 /tmp/qlib-data.log）
30 16 * * 1-5 cd /path/to/atlas && make qlib-data >> /tmp/qlib-data.log 2>&1
```

### 直接调用 evaluate.py 的注意

`evaluate.py` 内置 `DEFAULT_QLIB_DIR` 仍指向社区包 `cn_data`（未改动）。**绕过 Makefile
直接运行时必须自带 `--qlib-dir`** 指向 atlas_cn，否则会读到截止 2020-09 的社区包：

```bash
scripts/qlib_eval/.venv/bin/python scripts/qlib_eval/evaluate.py \
  --signals signals.csv --qlib-dir ~/.qlib/qlib_data/atlas_cn --out reports/
```

`make signal-eval` 路径总是显式传 `--qlib-dir $(QLIB_DIR)`，不受该默认值影响。

## 运行

一键端到端（推荐，从仓库根执行）：

```bash
make signal-eval   # export-signals 导出 signals.csv → evaluate.py 评估 → reports/
```

`signal-eval` 依赖 `export-signals`，并用预置 venv 的 Python 调 `evaluate.py`
（**系统 python3 已损坏，务必走 venv，勿用裸 python**）。可覆盖变量：
`QLIB_DIR`（数据目录）、`SIGNAL_OUT`（报告输出目录）、`SIGNAL_SYMBOLS/FROM/TO`。

分步执行：

```bash
# 1) Go 侧导出信号
make export-signals            # 产出 signals.csv

# 2) Python 侧评估（用 venv python，不要用系统 python3）
scripts/qlib_eval/.venv/bin/python scripts/qlib_eval/evaluate.py \
  --signals signals.csv [--qlib-dir ~/.qlib/qlib_data/cn_data] [--out reports/]
```

报告写入 `reports/signal-eval-YYYYMMDD.md`。qlib 数据目录缺失时 `evaluate.py`
打印下载指引并以非 0 退出（不 panic、不静默）。

## 测试

```bash
# 从仓库根执行（与 TaskCompleted hook 同款命令）
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q
```

测试**全程不依赖 qlib 安装与数据包**。

## 评估口径

- **符号范围**：Phase 1 仅 A 股（`600519.SH` → qlib `SH600519`），非 A 股符号跳过收集进「数据缺口」节。
- **入场对齐**：信号日的**次日开盘**入场（规避前视）。
- **入场顺延上限**：信号日与入场 bar 的间隔 `> max_defer*2` 个**日历日**则丢弃并计数
  （`max_defer*2` 是「顺延超过 max_defer 个交易日」的日历日近似：5 个交易日 ≈ 7-10 个日历日，取 `*2` 上界）。
- **horizon**：5 / 20 / 60 个交易日。
- **超额收益**：相对基准 `SH000300`（沪深 300）；buy 为 `ret - bench_ret`，sell 为规避口径
  `-(ret - bench_ret)`（信号后跑输基准记为正）。
- **基准对齐**：个股停牌时取 bench 中 ≤ 目标日期的最近前值（若入场日早于基准首行 →
  计入数据缺口，不静默取末行）。
- **基准收益口径**：基准为**收盘到收盘**（entry 日 close → exit 日 close）；而个股为
  **开盘到收盘**（entry 日 open → exit 日 close）。两者在入场日存在日内偏置（个股多算了
  entry 日 open→close 一段，基准未算），属已知近似——超额绝对值不宜过度解读，重在横向对比。
- **置信度分桶**：`≥0.0 / ≥0.6 / ≥0.8` 累积阈值（非互斥区间）；一条信号计入所有
  `confidence ≥ 阈值` 的桶。
- **胜率（win_rate）**：超额收益 `> 0` 的样本占比。因 sell 的超额已取规避向，
  buy 与 sell 胜率口径统一为「超额 > 0」（buy=跑赢基准，sell=成功规避下跌）。
- **数据缺口分类**：`dropped`（无入场 bar / 顺延过久 / 入场早于基准）、`data_gaps`
  （价格/基准取数失败）、非 A 股符号（Phase 1 跳过）在报告「数据缺口」节分行展示。
- **数据局限**：qlib 数据包非实时，有**数据截止日局限**（最新若干交易日可能缺失），
  落在数据截止日附近的 horizon 会越界计 NA；样本期取足够长（如 2021–2026）以保证样本数。

## 硬约束

`import qlib` **禁止出现在任何模块顶层**，只允许在 `QlibPriceSource` 方法体内惰性导入
——这是 pytest 零 qlib 依赖的机制保证。测试用守门用例 `assert "qlib" not in sys.modules` 锁死。
