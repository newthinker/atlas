# ATLAS × Qlib 整合方向分析

> 日期：2026-06-11
> 范围：基于 atlas master 分支与本地 qlib 副本（/Users/zuowei/workspace/python/qlib，2025-06 版，pyqlib，Python 3.8-3.12）
> 关联文档：`2026-06-11-asset-coverage-and-percentile-analysis.md`、`docs/plans/2026-06-11-index-commodity-percentile-design.md`

## 一、总体判断

两个项目定位高度互补：

| | atlas | qlib |
|---|---|---|
| 定位 | 实时监控 + 信号通知 + 交易执行（在线系统） | 特征工程 + ML 建模 + 研究回测（离线平台） |
| 语言 | Go | Python |
| 数据频率 | 实时/分钟级行情轮询 | 日频为主（研究级，.bin 本地仓库） |
| 对外接口 | REST API + Web Dashboard | 仅 Python 库 + qrun YAML 工作流，**无原生 REST API** |

qlib 不能也不应替代 atlas 的实时链路；它的价值在于给 atlas 补上「研究端」的厚度。语言边界决定了集成形态只有三种：**文件交换、sidecar 服务、数据仓库共享**。

### qlib 能力地图（探索结论摘要）

- **数据层**：Provider 架构、ExpressionCache、`.bin` 列式存储（`scripts/dump_bin.py`）、**PIT 点对时间财务数据库**（`qlib/data/pit.py`）；数据采集脚本覆盖 A 股（BaoStock）、美股（Yahoo）、加密货币
- **特征工程**：Alpha158/Alpha360 预制特征集；54+ 算子的字符串表达式引擎（Ref/Mean/Std/**Rank**/Corr/Slope/EMA…，`qlib/data/ops.py`）
- **模型动物园**：35+ 模型（LightGBM/XGBoost/CatBoost、ALSTM/GATs/Transformer 等 PyTorch 系）
- **回测引擎**：日频+分钟频、嵌套执行、完整成本模型（滑点/手续费/涨跌停，`qlib/backtest/exchange.py`）、IC/IR 信号分析（`SigAnaRecord`）、组合分析（`PortAnaRecord`）
- **信号机制**：预测分格式 `(instrument, datetime) → score`，`SignalWCache` 可装载外部信号，`TopkDropoutStrategy` 消费
- **其他**：MLflow 实验管理、OnlineManager 在线学习、RL 订单执行框架

## 二、五个可扩展方向（按价值/成本排序）

### ① 用 qlib 回测引擎验证 atlas 策略（最高性价比，零侵入）

atlas 的 backtest 引擎是简化模型，没有滑点、手续费、涨跌停、组合层面的评估，也没有任何「信号预测能力」的量化度量。

**做法**：
- atlas 把策略信号导出为 qlib 预测分格式（`(instrument, datetime) → score` 的 CSV/pkl，置信度即 score）
- qlib 侧用 `SignalWCache` 装载 + `TopkDropoutStrategy` 回测，产出年化收益、最大回撤、换手成本、**IC/IR**
- 纯离线管线，不动 atlas 一行核心代码；percentile 新策略上线前即可用这条管线做历史验证

### ② qlib ML 模型作为 atlas 的新信号源（核心价值方向）

atlas 策略全是规则型。集成形态建议 **sidecar**：

```
qlib (Python sidecar, FastAPI 包一层)
  每日收盘后: Alpha158 特征 → 模型推理 → pred score
  GET /scores?symbols=...          ← atlas 拉取
atlas (Go)
  新增 qlib_ml 策略: score > 阈值 → Signal(buy, confidence=归一化score)
  → 现有 router → LLM arbitrator（与规则策略信号仲裁）→ notifier/broker
```

这正好踩在 atlas 现有架构的甜点上：`Strategy` 接口天然支持新增信号源，**LLM arbitrator 本来就是为多信号冲突仲裁设计的**——规则策略（可解释）+ ML 分数（高维特征）+ LLM 仲裁构成完整的三层决策结构。

约束：qlib 是日频，该策略只适合日级监控周期，不影响 atlas 分钟级行情监控。

### ③ qlib 作为 atlas 的历史数据仓库（解决 percentile 设计的二期缺口）

与本次 percentile 设计（rev6）直接相关：

- **PIT 财务数据库**：rev6 把「估值分位的本地基本面快照库」列为二期。qlib PIT（财务数据按报告期+观察时点双轴存储，避免前视偏差）就是这件事的现成实现——美/港股 PE 历史序列可从此而来，替代或增强 Yahoo EPS 重建路径
- **历史 K 线仓库**：atlas 不持久化 OHLCV（实时拉取 + TTL 缓存），5 年日线每轮依赖外部 API 可用性。用 qlib `.bin` 仓库（BaoStock A 股 + Yahoo 美股采集脚本现成）做本地历史库，atlas 增加「qlib 数据目录读取」collector 兜底，显著降低对 Yahoo/eastmoney 的实时依赖

### ④ 表达式引擎/算子库反哺 atlas 指标层

qlib 54 个算子比 atlas `internal/indicator` 丰富一个数量级。两种用法：

- **轻量（推荐）**：sidecar 暴露「表达式求值」端点，atlas 把 `"Rank($close, 1260)"` 这类表达式当配置传入——注意 `Rank` 算子干的就是 percentile 策略的核心计算
- **重做法**：按需移植算子到 Go，无运行时依赖但工作量大

建议轻量路线，且仅在 ② 的 sidecar 已存在后顺带提供。

### ⑤ 组合层与执行优化（远期，YAGNI 警告）

qlib 的 TopkDropout 组合构建、增强指数追踪优化器、RL 订单执行，对应 atlas broker 模块目前的简单仓位规则（`default_size_pct: 2%`）。在 atlas 真实接入 Futu 实盘且资金规模需要组合管理之前，不建议动。

## 三、建议的演进路径

| 阶段 | 内容 | 侵入性 |
|---|---|---|
| 第一步 | 信号导出脚本 + qlib 回测/IC 分析管线（验证现有 + percentile 新策略） | 零（纯离线） |
| 第二步 | qlib sidecar（FastAPI）+ atlas `qlib_ml` 策略 | 小（一个新策略 + HTTP client） |
| 第三步 | qlib 数据仓库 collector 兜底 + PIT 替代美/港 PE 重建 | 中（新 collector） |
| 远期 | 表达式求值端点、组合优化、RL 执行 | 按需 |

## 四、风险与约束

- **运维成本**：引入 Python 运行时与 qlib 数据更新管线（数据需每日 dump 更新）是真实负担
- **成熟度**：本地 qlib 副本为 alpha 状态的研究框架；sidecar 必须按「可降级的增强信号源」设计——qlib 不可用时 atlas 规则策略照常工作，与 atlas 现有兜底哲学一致
- **频率错位**：qlib 日频研究数据不可用于 atlas 实时告警路径，集成边界必须明确锚定在「日级决策增强」上
