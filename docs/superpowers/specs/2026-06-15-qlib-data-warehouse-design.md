# ATLAS × Qlib 数据仓库整合（扩展方向③）设计

> 日期：2026-06-15
> 范围：实现 `docs/reviews/2026-06-11-qlib-integration-analysis.md` 的扩展方向③
> 关联：方向① 已实现于 `scripts/qlib_eval`（离线信号回测/IC 分析）
> 集成形态：**共享数据仓库**（非 sidecar），atlas 进程直接读本地文件

## 一、目标与动机

atlas 当前两处数据脆弱点，均在 rev6 percentile 设计与集成分析中已记录：

1. **OHLCV 不持久化**：`Collector.FetchHistory` 每轮依赖 Yahoo/eastmoney 实时拉取
   + TTL 缓存。5 年日线每个监控周期都受外部 API 可用性约束。
2. **无本地基本面历史**：PE 分位由 `valuation.ReconstructPEPercentile` 用每日收盘
   对齐最近 EPS(TTM) 重建，EPS 来自 Yahoo 重建 + lixinger 兜底，脆弱且有前视风险。

方向③用 qlib 的成熟数据能力（`.bin` 列存、dump 脚本、PIT 点对时间财务库）在本地
建一个历史数据仓库，降低 atlas 对外部 API 的实时依赖，并为 PE 分位提供消除前视偏差
的本地基本面历史。

本设计同时覆盖两个**子件**：
- **Part A — 历史 K 线仓库**：喂 `FetchHistory`，惠及所有策略
- **Part B — PIT 财务数据库**：喂 `ReconstructPEPercentile`，专补 PE 分位二期缺口

覆盖市场：**美股、A 股、港股**。

## 二、核心架构判断

**数据源异构性完全封闭在 Python dump 管线内，atlas 只面对一张统一的 SQLite schema。**

```
┌─ Python dump 管线（每日收盘后）──────────────────────┐
│  各市场各自采集（异构）：                              │
│   A 股 → qlib BaoStock + dump_pit                     │
│   美股 → Yahoo (OHLCV) + Yahoo EPS (PIT best-effort)  │
│   港股 → Yahoo (OHLCV) + lixinger (PIT best-effort)   │
│                    ↓ 归一化                            │
│  写入统一 SQLite（原子写：临时库 → rename）：          │
│   ohlcv / fundamentals_pit / warehouse_meta           │
└───────────────────────────────────────────────────────┘
                     ↓ atlas 只读这张库，与市场/来源无关
┌─ atlas Go 侧 ─────────────────────────────────────────┐
│  ① 新 qlib collector：实现 FetchHistory               │
│     仓库主源 + 外部 API 补新鲜尾巴                      │
│  ② 新 PIT 基本面源：读 fundamentals_pit，时点查询      │
│     喂 ReconstructPEPercentile（reconstruct.go 不改）  │
└───────────────────────────────────────────────────────┘
```

**为什么这样切**：atlas 不关心数据来自 `.bin`/BaoStock/Yahoo/lixinger，只查 SQLite；
三市场异构、美/港 PIT 的 best-effort 全部留在 Python 侧。Go 侧两个消费通路对市场无感知。

**被否决的备选**：atlas 直读 qlib 原生 `.bin`/PIT 格式——会把 alpha 阶段研究框架的
内部二进制布局耦合进 Go，格式一变即脆。统一 SQLite schema 提供稳定契约。

## 三、数据契约（SQLite Schema）

单库文件，默认路径 `data/qlib_warehouse.db`（atlas 侧可配置）。

```sql
-- Part A：历史日线
CREATE TABLE ohlcv (
  symbol     TEXT NOT NULL,
  date       TEXT NOT NULL,        -- YYYY-MM-DD
  open       REAL, high REAL, low REAL, close REAL,
  volume     REAL,
  adj_close  REAL,
  PRIMARY KEY (symbol, date)
);

-- Part B：PIT 双轴基本面（报告期 × 观测时点）
CREATE TABLE fundamentals_pit (
  symbol        TEXT NOT NULL,
  report_period TEXT NOT NULL,     -- 报告期，e.g. 2025-Q1
  observe_date  TEXT NOT NULL,     -- 该数据首次可知日（避免前视）
  eps_ttm       REAL,
  pe REAL, pb REAL, ps REAL, roe REAL, dividend_yield REAL,
  PRIMARY KEY (symbol, report_period, observe_date)
);

-- 元数据：陈旧度判定 + 来源审计
CREATE TABLE warehouse_meta (
  symbol     TEXT PRIMARY KEY,
  market     TEXT NOT NULL,
  source     TEXT NOT NULL,        -- e.g. baostock / yahoo / lixinger
  last_date  TEXT NOT NULL,        -- ohlcv 覆盖到的最新交易日
  dumped_at  TEXT NOT NULL         -- 本次 dump 时间戳（RFC3339）
);
```

**PIT 时点查询语义**（消除前视偏差的关键）：给定观测日 `D`，
取 `observe_date <= D` 的所有行中、每个 `report_period` 的最新 `observe_date` 那条，
再按 `report_period` 升序取其 `eps_ttm`，形成 `[]EPSPoint`。
即「站在 D 这天能合法看到的、各报告期最新修订值」。

## 四、Part A：qlib Collector

新增包 `internal/collector/qlib`，实现现有 `collector.Collector` 接口。
核心是 `FetchHistory` 的「仓库主源 + API 补新鲜尾巴」：

```
FetchHistory(symbol, start, end, interval):
  1. 查 warehouse_meta.last_date（仓库覆盖到哪天）；无记录 → 整体回落外部 API
  2. 从 ohlcv 读 [start, min(end, last_date)] 区间          ← 权威主源
  3. if end > last_date:  调外部 collector 补 (last_date, end] ← 新鲜尾巴
  4. 拼接返回；补尾 API 失败 → 仅返回仓库段 + 记 warning（可降级）
  - FetchQuote：不由仓库提供，委托外部 collector（实时行情不入仓库）
```

- 补尾**不重复实现** Yahoo/eastmoney 逻辑：通过注入一个「外部 collector 选择器」
  （复用 `collector.SelectForSymbol`）拿到该符号的外部 collector 并调用其 `FetchHistory`。
- `interval`：仓库只存日线；非日频请求直接委托外部 collector。
- SQLite 以**只读**模式打开（`mode=ro`）。

`selector.go` 调整：仓库覆盖（`warehouse_meta` 有该符号）的符号优先返回 qlib collector；
否则维持现有路由。qlib collector 未注册（库不存在）时路由完全不变。

## 五、Part B：PIT 基本面源

新增 `qlibpit` 源，实现现有 EPS 历史接口形状
（`app.go:151` 的 `FetchEPSHistory(symbol, start, end) ([]core.EPSPoint, error)`）：

- 内部对 `fundamentals_pit` 执行第三节的 PIT 时点查询，产出按 `observe_date` 排序的
  `[]core.EPSPoint`（`{Date, EPS}`，Date 取 observe_date）。
- **直接替换/优先于 Yahoo EPS** 喂给 `app.go:807` 的
  `valuation.ReconstructPEPercentile(ohlcv, eps)`——`reconstruct.go` 一行不改，只换数据源。
- 仓库无该符号基本面（如美/港 best-effort 缺失）→ 回落现有 Yahoo/lixinger 路径。

## 六、降级与陈旧度策略（贴合 atlas 兜底哲学）

| 情况 | 行为 |
|---|---|
| SQLite 文件不存在/损坏 | qlib collector 与 PIT 源**不注册**，系统等同今天（完全可降级） |
| 仓库缺该符号 | 该符号整体回落外部 API 路径（零行为变化） |
| 仓库有该符号但 `last_date` 过旧（> `max_staleness_days`，默认 7） | 仍用仓库历史段 + API 补尾，记 warning |
| 补尾外部 API 失败 | 仅返回仓库段 + 记 warning，不报错 |

新增配置（atlas 侧）：
```
collectors.qlib:
  enabled: false           # 默认关闭，缺库时不影响现有路径
  db_path: data/qlib_warehouse.db
  max_staleness_days: 7
```

Go 侧 SQLite 驱动选用纯 Go 的 `modernc.org/sqlite`，避免 cgo 破坏交叉编译。

## 七、Dump 管线（Python 侧）

- 新目录 `scripts/qlib_warehouse/`，与 `scripts/qlib_eval` 并列，复用其 venv 与
  `qlib_csv_*` 采集成果作为 OHLCV 来源，避免重造采集逻辑。
- 流程：各市场异构采集 → 归一化为统一 schema → 写 SQLite。
- **原子写**：写临时库文件再 `os.replace`（rename）覆盖目标库，避免 atlas 读到半成品。
- 美/港 PIT 标注 **best-effort**：基本面缺失时只写 `ohlcv` 与 `warehouse_meta`，
  不写 `fundamentals_pit`（Go 侧据此回落）。
- 提供 `make warehouse-dump` target + 文档化每日 cron（收盘后触发）。

## 八、测试策略

**Python（pytest）：**
- 归一化正确性（各市场字段映射）
- PIT 时点正确性：构造跨报告期的**前视陷阱**用例（晚到的修订值不得污染早期观测日）
- 原子写：dump 中途中断不留半成品库

**Go：**
- qlib collector：用临时 SQLite fixture 测主源命中、补尾拼接、API 失败降级、
  缺符号回落、库不存在不注册等分支
- PIT 源：时点查询正确性（与 Python 用例对称）+ 喂入 `ReconstructPEPercentile` 端到端
- `valuation/reconstruct` 既有测试不动（验证数据源替换零回归）

## 九、范围与风险

- **最大不确定点**：美/港股 PIT 基本面无 qlib 原生 PIT 采集，依赖 Yahoo EPS / lixinger
  重建，标注 best-effort——这两个市场的 Part B 可能数据稀疏或暂缺，Go 侧降级路径保证
  不影响现有行为。A 股 Part B 可行性最高。
- **运维负担**：引入每日 dump 管线是真实成本（与集成分析风险章节一致）。
- **演进切分建议**：本 spec 覆盖 A+B 全量；writing-plans 阶段建议拆为
  (1) Schema + Python dump 管线 + Part A collector，(2) Part B PIT 源，两期推进，
  使 Part A（低风险、惠及全部策略）可先独立交付。

## 十、不做（YAGNI）

- 不做实时行情入仓库（`FetchQuote` 始终走外部 API）。
- 不做分钟频/盘中数据（仓库仅日线）。
- 不做 atlas 侧写库（仓库对 atlas 只读，写入唯一由 Python 管线负责）。
- 不引入 sidecar HTTP 服务（那是方向②的形态）。
