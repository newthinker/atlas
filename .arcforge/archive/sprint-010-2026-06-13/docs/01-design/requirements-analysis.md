# 需求分析 — 美股 signal-eval（atlas_us 全管线）

> 源需求：`docs/plans/2026-06-13-atlas-us-signal-eval-implementation.md`
> 设计依据：`docs/plans/2026-06-13-atlas-us-signal-eval-design.md`（rev2.1 终版）
> 分析者：Project Leader · 2026-06-13

## 1. 核心目标

让 signal-eval 事件研究管线支持美股：atlas 导出美股策略信号 + OHLCV → 自建
`atlas_us` qlib 包做事件研究，相对 `^GSPC`（标普500）算超额收益。**镜像 sprint-009
HK 模式**，仅给「市场差异」加 US 分支，**不碰已跑通的 CN/HK 路径**。

## 2. 核心功能列表

| # | 功能 | 落点 |
|---|---|---|
| F1 | US 裸 ticker（`[A-Z]{1,5}`）+ 指数（`^GSPC/^IXIC/^DJI`）→ qlib instrument | Go `toQlibInstrument` |
| F2 | US 市场键控：`benchmarkForMarket`→`^GSPC`、`inMarket` US 识别、`--market us` 白名单 | Go `export_ohlcv.go` |
| F3 | Python `to_qlib_instrument` 对称镜像 US 规则（`re.fullmatch` 锚定，与 Go 等价） | `symbols.py` |
| F4 | `QlibPriceSource` region 参数化（默认 cn，US 传 us）+ `evaluate.py --region` | `prices.py`/`evaluate.py` |
| F5 | Makefile `qlib-data-us` / `signal-eval-us` target | `Makefile` |
| F6 | config US watchlist（8 个股 + ^GSPC） | `configs/config.yaml` |

## 3. 非功能性需求

- **零回归**：CN/HK 现有契约/行为/测试全部不变（默认参数 region=cn、benchmark 缺省）。
- **跨语言契约对称**：Go `^[A-Z]{1,5}$` 与 Python `re.fullmatch(r"[A-Z]{1,5}")` 对相同样本
  输出一致（`AAPL→AAPL`、`^GSPC→GSPC`、`AAPL123`/`AAPL.B` 两侧均 reject）。
- **pytest 全程 qlib-free**：Python 测试不得在 module level import qlib（惰性导入保持）。
- **TDD 强制**：每个 Task 先写失败测试（RED）再实现（GREEN）。
- **venv 约束**：系统 python3 损坏，Python 测试必须走 `scripts/qlib_eval/.venv/bin/python`。

## 4. 模块识别与接口

| 模块 | package | 对外接口（被下游依赖） |
|---|---|---|
| Go export-ohlcv | `cmd/atlas` | `usTickerRe`、`toQlibInstrument`、`benchmarkForMarket`、`inMarket`、`--market us` |
| Python symbols | `scripts/qlib_eval` | `to_qlib_instrument`（契约对称 Go） |
| Python prices/eval | `scripts/qlib_eval` | `QlibPriceSource(region=...)`、`evaluate.py --region` |
| 编排 Makefile | root + `scripts/qlib_eval/tests` | `qlib-data-us`、`signal-eval-us` target |
| 配置 | `configs` | US watchlist 集 |

## 5. 模糊/风险点（已在设计中论证）

- **R1 region="us" 真实可用性**：qlib 对自建 atlas_us 包传 `region="us"` 可能报错/读空。
  设计 §4.1 已论证降级路径——HK 当年用 `region="cn"` 跑通非 cn 包；异常则 `--region us`
  回退 `--region cn`，记录于提交信息。属 Task 6 端到端验收项。
- **R2 锚定契约**：`[A-Z]{1,5}` 必须全串锚定（fullmatch），否则 `AAPL123`/`AAPL.B`
  会被误接。Go/Python 两侧均须锚定且对称。
