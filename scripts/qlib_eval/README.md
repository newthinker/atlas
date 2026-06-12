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

## 运行

```bash
# 1) Go 侧导出信号
make export-signals            # 产出 signals.csv

# 2) Python 侧评估
cd scripts/qlib_eval
python evaluate.py --signals ../../signals.csv \
  [--qlib-dir ~/.qlib/qlib_data/cn_data] [--out ../../reports/]
```

报告写入 `reports/signal-eval-YYYYMMDD.md`。

## 评估口径

- **符号范围**：Phase 1 仅 A 股（`600519.SH` → qlib `SH600519`），非 A 股符号跳过收集进「数据缺口」节。
- **入场对齐**：信号日的**次日开盘**入场（规避前视）。
- **入场顺延上限**：信号日与入场 bar 的间隔 `> max_defer*2` 个**日历日**则丢弃并计数
  （`max_defer*2` 是「顺延超过 max_defer 个交易日」的日历日近似：5 个交易日 ≈ 7-10 个日历日，取 `*2` 上界）。
- **horizon**：5 / 20 / 60 个交易日。
- **超额收益**：相对基准 `SH000300`（沪深 300）；buy 为 `ret - bench_ret`，sell 为规避口径
  `-(ret - bench_ret)`（信号后跑输基准记为正）。
- **基准对齐**：个股停牌时取 bench 中 ≤ 目标日期的最近前值。

## 硬约束

`import qlib` **禁止出现在任何模块顶层**，只允许在 `QlibPriceSource` 方法体内惰性导入
——这是 pytest 零 qlib 依赖的机制保证。测试用守门用例 `assert "qlib" not in sys.modules` 锁死。
