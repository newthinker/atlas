# IC / IR 信号预测力评估管线 — 设计文档

> 日期：2026-06-22
> 状态：设计已确认（rev2 — superpowers 探讨后：IC 口径改为**时序为主**、前向收益改为 **next-open 对齐**、自验证用 **Oracle + 反转** 双 baseline、新增统计严谨性小节）
> 上游分析：`docs/reviews/2026-06-11-qlib-integration-analysis.md`（整合方向①、②）
> 关联：`docs/plans/2026-06-11-qlib-eval-pipeline-design.md`（事件研究管线，本文档与之平行）
> 定位：补齐整合方向①承诺但未交付的 IC/IR 度量，作为方向②（ML 信号源）上线的**量化验收前置**

## 0. 一句话目标

为「稠密分数面板」(`(symbol, date) → score`) 建立**时序 IC / Rank IC / ICIR** 评估能力，
**先用 baseline 因子验证 harness 可信，再用它给未来的 qlib ML 模型把关**——
没有这条度量腿，方向②的 ML 信号源上线后无法量化判断「分数到底有没有预测力」。

## 1. 背景与动机

### 1.1 为什么现在要做

整合分析（方向①）承诺产出 **IC/IR**，但一期落地的 `qlib_eval` 是**事件研究**
（每个 buy/sell 信号的后续收益/超额/胜率），刻意未做 IC——
一期设计文档 §1.2 明确记录该裁剪，理由是「标的太少无统计意义」。

该裁剪在一期成立（评估对象是规则策略的离散信号），但方向②的核心价值是 **ML 模型**：
每只 watchlist 标的每天出一个连续择时分数。**评估这种稠密分数的标准指标就是 IC/IR**，
事件研究无法胜任。因此：

> **没有 IC/IR，方向②（自评为"核心价值方向"）上线时缺乏验收抓手。**
> 先补这条腿，是用最小成本前置消化②的核心不确定性——IC 站得住再投 sidecar 工程。

### 1.2 关键口径决定：时序 IC，而非横截面 IC

IC 有两种「截面」方向，必须先选对，否则整套度量错位：

| | 横截面 IC（qlib 默认） | **时序 IC（本管线采用）** |
|---|---|---|
| 计算单元 | 同一天全市场标的互相排名比较 | 单标的、时间轴上 `score_t` 与其前向收益的相关 |
| 度量的能力 | 选股 alpha（今天买哪几只） | **单标的择时**（这只票现在该不该买） |
| 样本来源 | 单日截面的标的数 | 单标的累积的交易日数 |
| 适配 universe | 需宽 universe（几百上千只） | **适配小 watchlist**（每标的独立累积几百日样本） |

**atlas 的本质是 per-instrument 的择时监控系统**：watchlist 是精选的十来个标的，
信号是「茅台该不该买」这类单标的择时判断，不是「今天该买 A 股里哪 50 只」的选股。
方向② 的 ML 信号源同样是逐标的出择时分数。故**时序 IC 才是匹配的口径**；
横截面 IC 是把 qlib 的选股范式生搬过来，且在 13 只 watchlist 上单日截面仅十来个点、无统计意义
——这正是一期裁剪 IC 的理由，本口径选择从根上绕过它。

横截面 IC 降级为 universe 扩大后的可选项（§2.2 明确不做）。

### 1.3 IC / IR 是什么（时序口径，锚定避免实现漂移）

- **单标的 IC（time-series IC）**：对某标的，取其历史上每个交易日的分数 `score_t` 与
  **该日起算的前向收益** `ret(t→t+h)`，算这两条时间序列的相关系数。一个标的得一个 IC。
- **Pearson IC** = 线性相关；**Rank IC（Spearman）** = 排名相关，对极端值稳健，
  量化默认主报指标。本管线**两者都报**，与 qlib `SigAnaRecord` 命名对齐。
- 衍生指标（单标的层）：

  | 指标 | 公式 | 含义 |
  |---|---|---|
  | IC | 上述相关系数 | 该标的的择时预测力 |
  | t-stat | `IC·√(n_periods)` | 显著性（n=该标的有效交易日数） |

- watchlist 汇总层（跨标的聚合）：

  | 指标 | 公式 | 含义 |
  |---|---|---|
  | mean IC | `mean(IC_symbol)` | 该信号源在 watchlist 上的平均择时力 |
  | median IC | `median(IC_symbol)` | 抗个别标的极端值 |
  | **ICIR** | `mean(IC_symbol)/std(IC_symbol)` | 预测力在标的间的**一致性**（比 mean 更关键） |
  | positive breadth | `% symbols IC>0` | 多少比例标的方向为正（广度） |

### 1.4 与事件研究的本质区别（决定本管线为独立腿）

| | 事件研究 `event_study.py`（已有） | IC/IR（本文档） |
|---|---|---|
| 输入 | 离散 buy/sell 信号（偶发） | 稠密分数面板（每票每天有分） |
| 计算 | 单信号入场后的后续收益 | 单标的 `score_t` 与其前向收益的时序相关 |
| 适配 | 规则策略 | ML 模型（连续分数） |

IC 无法施加于离散信号（每标的只有寥寥几条事件，时序样本不足）。故本管线**新增平行评估腿，
不改动 `event_study.py`**。其输入契约 `scores.csv` 即方向② sidecar 的输出契约。

## 2. 目标与范围

### 2.1 已确认的范围决定

| 决定点 | 结论 | 理由 |
|---|---|---|
| IC 口径 | **时序 IC（逐标的）** | 匹配 atlas 的 per-instrument 择时本质；绕过薄截面（§1.2） |
| 评估对象 | 稠密分数面板 `(symbol,date)→score` | IC 的前提；离散信号不适用 |
| 跨语言契约 | `scores.csv` 长格式（`date,symbol,score`） | = 方向② sidecar 输出格式，现在钉死 |
| 验证手段 | **Oracle + 反转**双 baseline（从 qlib bundle 直接算，**无需 ML**） | Oracle 证数学正确（IC≈1），反转证真数据通路（IC 量级合理） |
| 前向收益口径 | **next-open 对齐**：`close(t)` 出分 → `open_{t+1}` 入场 → `h` 日后收盘 | 可交易、无前视；与事件研究同口径，两腿可横向互证；复用 `align_entry` |
| IC 类型 | Pearson IC + Rank IC 都报 | 对齐 qlib SigAnaRecord 命名 |
| horizon | 复用 `5 / 20 / 60` 交易日 | 与事件研究一致，便于横向对照 |
| qlib 依赖 | 计算层零 qlib（纯 pandas）；取价层惰性 import | 复用一期 pytest 零依赖架构 |
| 形态 | Python 薄评估层，与事件研究并行；CSV 为唯一契约 | 与一期一致，外科手术式增量 |

### 2.2 明确不做（本期边界）

- **不做横截面 IC**——universe 扩大（watchlist → 几百只）后再议；本期口径锁定时序。
- 不做组合回测（年化/回撤/换手成本）——那是 TopkDropout 的范畴，远期方向⑤。
- 不改 `event_study.py` / 现有 `signal-eval` 管线。
- 不建 sidecar、不接 ML 模型——本期只建**度量能力**与**baseline 验证**；
  真模型接入是方向②独立子项目。
- 不做 IC 衰减曲线 / 分层回测（layered backtest）等进阶分析（按需后续）。

### 2.3 统计严谨性：重叠前向收益

时序 IC 的相邻样本用了重叠的 h 日前向收益（如 horizon=20，相邻两日的前向窗口重叠 19 天），
导致**序列自相关，使 t-stat 虚高**。应对：

1. 报告对每个 t-stat **显式标注「受重叠收益影响、偏乐观」**。
2. 同时提供**非重叠采样**的 t-stat（每 h 天取一个样本点）作旁证；二者差距大时提示读者审慎。
3. n_periods（单标的有效交易日数）随报告输出，让读者自行判断样本充分性（默认门槛 `min_periods=60`）。

> 注：时序口径下，一期「标的太少」的薄截面顾虑已基本消解——每只标的独立累积几百个交易日样本，
> 验证可直接在现有 `atlas_cn`（watchlist）包上做，**无需下载社区包或扩 universe**。

## 3. 架构与数据流

```
（A）验证路径（本期重点，无 ML）
  qlib bundle (~/.qlib/qlib_data/atlas_cn)
     └─ baseline.py:
          ├─ oracle 因子：未来收益 + 噪声  → IC≈1（验证数学正确）
          └─ reversal 因子：-Ref($close,5)/$close → IC 量级合理（验证真数据通路）
        → scores.csv (date,symbol,score)
  ic_evaluate.py --scores scores.csv --qlib-dir ...
     ├─ QlibPriceSource.history()      → 复用现有取价层
     ├─ ic.py: forward_returns(next-open) → instrument_ic → watchlist_summary
     └─ report.render_ic_report        → reports/signal-ic-YYYYMMDD.md

（B）生产路径（方向②落地后，零改动复用）
  qlib ML sidecar  → GET /scores  → scores.csv（同契约）
  ic_evaluate.py（同一入口）→ 该模型的时序 IC/ICIR 报告 = ②的验收抓手
```

## 4. 文件改动清单（落在真实代码上）

| 动作 | 文件 | 内容 |
|---|---|---|
| 新增 | `scripts/qlib_eval/qlib_eval/ic.py` | 时序 IC 计算核心，纯 pandas 零 qlib |
| 新增 | `scripts/qlib_eval/qlib_eval/baseline.py` | 从 bundle 算 oracle + 反转因子面板（惰性 import qlib） |
| 新增 | `scripts/qlib_eval/ic_evaluate.py` | CLI 入口（仿 `evaluate.py`：缺目录打印下载指引并 exit(1)） |
| 改 | `scripts/qlib_eval/qlib_eval/report.py` | 加 `render_ic_report(summary, meta)`，复用现有 markdown 风格 |
| 改 | `Makefile` | 加 `baseline-scores`（生成验证面板）+ `signal-ic`（消费 scores.csv）两个 target |
| 改 | `docs/ops/qlib-warehouse-runbook.md` | 加 IC 评估章节 + 重叠收益 t-stat 读数告诫 |
| 新增 | `scripts/qlib_eval/tests/test_ic.py` | TDD 测试（见 §6） |
| 新增 | `scripts/qlib_eval/tests/test_baseline.py` | baseline 因子计算测试 |

## 5. 接口设计

### 5.1 `ic.py`（纯 pandas，可单测）

```python
def forward_returns(prices: dict[str, pd.DataFrame], horizons) -> pd.DataFrame:
    """每 symbol 每交易日的 next-open 前向收益：close_{t+h} / open_{t+1} − 1。
    入场对齐复用 prices.align_entry（次日开盘入场，规避前视）。
    长格式 (date, symbol, horizon, ret)；无次日 bar 或越界（无 t+h bar）不产行。"""

def instrument_ic(scores: pd.DataFrame, fwd: pd.DataFrame, symbol: str, h: int,
                  method: str = "spearman", min_periods: int = 60) -> dict | None:
    """单标的时序 IC：该标的 score_t 与其 next-open 前向收益的相关。
    返回 {ic, n_periods, t_stat, t_stat_nonoverlap}；
    有效样本 < min_periods → 返回 None（该标的不参与汇总）。"""

def ic_summary_by_instrument(scores: pd.DataFrame, fwd: pd.DataFrame, h: int,
                             method: str, min_periods: int = 60) -> pd.DataFrame:
    """每标的一行：symbol, ic, n_periods, t_stat, t_stat_nonoverlap。
    样本不足的标的被剔除并由调用方计入 stats（dropped_thin）。"""

def watchlist_summary(per_inst: pd.DataFrame) -> dict:
    """{mean_ic, median_ic, icir, positive_breadth, n_instruments}；
    n_instruments<2 时 icir 为 None（标的间 std 不可计算）。"""
```

### 5.2 数据契约 `scores.csv`（长格式）

```csv
date,symbol,score
2026-06-20,600519.SH,0.82
2026-06-19,600519.SH,0.77
2026-06-20,000001.SZ,0.31
```

- `symbol` 为 atlas 形式；非可识别市场标的按 `to_qlib_instrument` 失败时跳过并计入缺口。
- 时序口径要求每个 symbol 有**跨日的稠密分数序列**（非偶发事件）。
- schema 严格校验，错行带 1-based 物理行号报错（复用 `read_signals` 风格）。

### 5.3 CLI

```bash
python ic_evaluate.py --scores scores.csv --qlib-dir ~/.qlib/qlib_data/atlas_cn \
  [--method spearman] [--min-periods 60] [--out reports/]
```

## 6. TDD done_criteria（验收唯一依据）

- **functional**
  - 单标的合成序列：分数与前向收益完全正相关 → Rank IC ≈ 1.0；完全反相关 → ≈ −1.0；随机 → ≈ 0。
  - `t_stat = ic·√n_periods` 数值精确匹配手算。
  - `watchlist_summary` 的 mean/median/icir/breadth 在已知多标的输入上精确匹配手算。
  - Pearson 与 Spearman 两条路径都覆盖。
  - Oracle 因子（未来收益+噪声）经 baseline.py → ic.py 全链路得 IC≈1（端到端 plumbing 验证）。
- **boundary**
  - 单标的有效样本 < `min_periods` → 该标的返回 None 且不计入 watchlist 汇总。
  - 空面板 → 写「无可评估分数」报告并 exit 0（对齐 `evaluate.py` 空信号短路）。
  - `n_instruments < 2` → icir 为 None，报告显式标注「标的不足，跨标的一致性不可计算」。
  - 非重叠采样样本点 < 2 → `t_stat_nonoverlap` 为 None。
- **error_handling**
  - `scores.csv` schema 不符 → 带行号 ValueError。
  - qlib 目录缺失 → 打印 get_data 下载指引并 exit(1)。
  - 单 symbol 取价失败 → 计入数据缺口，不中断整体（对齐事件研究降级）。
- **non_functional**
  - pytest 零 qlib 依赖（守门测试，仿 `test_no_qlib_at_module_level`）。
  - 前向收益无前视：`score_t` 由 `close(t)` 算、收益从 `open_{t+1}` 起算。
  - 报告对每个 t-stat 标注「受重叠收益影响、偏乐观」并并列非重叠 t-stat（§2.3）。
  - 报告标注每标的 n_periods，便于判断样本充分性。

## 7. 阶段与验收

| 阶段 | 内容 | 验收标准 |
|---|---|---|
| 1 | `ic.py` + `test_ic.py`（纯计算，合成数据） | pytest 全绿，单标的 IC/t-stat 与 watchlist 汇总数值正确 |
| 2 | `baseline.py`（oracle+反转）+ `ic_evaluate.py` + `make baseline-scores` | oracle 因子端到端 IC≈1；反转因子对 atlas_cn 跑出非空报告且 IC 量级合理（0.0x）；harness 自证可信 |
| 3 | `report.py` IC 章节 + Makefile `signal-ic` + runbook 更新 | `make signal-ic SCORES=scores.csv` 端到端产出 markdown |

完成后，方向②上线只需 sidecar 输出 `scores.csv`，`make signal-ic` 即给出该 ML
模型的时序 IC/ICIR——这就是②的量化验收抓手。

## 8. 风险与约束

- **重叠前向收益致 t-stat 虚高**（见 §2.3）：本期主要统计陷阱。报告须标注并并列非重叠 t-stat。
- **口径分叉**：IC 与事件研究都用 next-open 入场（同口径，可横向互证）；但 IC 是时序相关、
  事件研究是事件后续收益，度量对象不同，文档须讲清避免误比。
- **baseline 因子非目的**：baseline 仅用于自证 harness，不是要上线的策略；
  其 IC 数值只用于「管线是否产出合理数字」的 sanity check。
- **小 watchlist 的跨标的一致性**：时序口径消解了单标的样本不足问题，但 watchlist 仅十来个标的，
  跨标的的 ICIR/breadth 仍是小样本，应作参考而非硬门槛。
- **运维**：复用现有 qlib bundle 与 `.venv`，无新增常驻依赖（本期纯离线批处理）。

## 9. 与演进路径的关系

整合分析的演进路径中，本管线是「方向①补全 + 方向②前置」：

```
方向① 事件研究（已落地） ──┐
                          ├─→ 方向② ML sidecar（IC 验收通过后再投工程）
本文档 时序 IC/IR 度量 ────┘
方向③ 数据仓库/PIT（已落地，为本管线与②共享数据源）
```
