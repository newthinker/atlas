# Atlas 宏观危机监控模块设计方案

**模块代号**：`macro-crisis-monitor`（建议子域命名沿用 Atlas 神话体系：**Cassandra** —— 预言危机但常被忽视的女祭司，恰好自嘲了领先指标的宿命）
**版本**：v0.2（经需求/边界评审修订：存储改 sqlite、调度改 launchd、包结构对齐现有惯例、通知不分级、回测分段降级验收）
**日期**：2026-07-12
**作者**：Vic / Claude 协作整理

---

## 1. 背景与目标

现有 Atlas 系统覆盖指数基金与大类资产的价格监控。本模块补充**系统性风险维度**：通过一组经过验证的市场压力指标，持续评估金融危机的酝酿与爆发状态，为家庭资产配置（BTC DCA、A/H 股持仓、网格策略）提供降杠杆/防御的前置信号。

设计原则有三条。第一，**概率语言而非确定性预测**——输出的是风险等级状态机，不是"X 月内必发"式的伪精确规则。第二，**共振才触发行动**——任何单指标红灯只改变检查频率，不产生行动建议。第三，**降噪优先**——延续公司告警治理的经验，宁可漏报边缘信号，也不制造告警疲劳。

## 2. 指标体系定义

体系分四层加一个旁证，共 7 个指标。分层对应"冰山模型"的修正版：情绪在表层（同步确认），流动性在水线（压力传导），信用在水下（违约定价，唯一兼具深度与领先性），领先层与旁证提供 3–12 个月视角。

### 2.1 指标清单与数据源

| # | 指标 | 层 | 数据源 | 序列/代码 | 频率 | 发布时点 (ET) |
|---|------|-----|--------|-----------|------|----------------|
| 1 | VIX | 情绪层 | FRED | `VIXCLS` | 日 | 收盘后，T+1 上午 |
| 2 | MOVE | 情绪层 | Yahoo Finance | `^MOVE` | 日 | 收盘后（FRED 无此序列） |
| 3 | SOFR−EFFR 利差 | 流动性层 | FRED（两序列本地相减） | `SOFR`, `EFFR` | 日 | T+1 约 8:00/9:00 |
| 4 | HY OAS | 信用层 | FRED | `BAMLH0A0HYM2` | 日 | T+1 |
| 5 | 10Y−2Y 利差 | 领先层 | FRED | `T10Y2Y` | 日 | T+1 |
| 6 | NFCI | 领先层 | FRED | `NFCI` | 周（周五截止） | 周三 8:30 |
| 7 | USD/JPY | 旁证 | Yahoo Finance | `JPY=X` | 日（实盘连续） | — |

数据源要点：

**FRED API** 为主通道。免费，需申请 API key（`https://fred.stlouisfed.org/docs/api/`），限速约 120 req/min，对本模块的 6 序列 × 日更完全够用。采集器实现为 `internal/collector/fred`，与既有 `collector/yahoo` 平级——沿用现有 collector 按数据源分包的惯例，不新增 adapter 层。

**关键坑：ICE BofA 序列的历史截断。** 自 2026 年 4 月起，FRED 上的 `BAMLH0A0HYM2`（HY OAS）只保留最近 3 年观测。这意味着滚动 5 年分位数无法直接从 FRED 拉取——必须在首次部署时导入一份更长的历史快照（可从 ICE 官方或已有 CSV 备份获取），此后由 Atlas 本地持续累积。**本地全量存储是本模块的硬需求，不是优化项。**

**第二个坑：SOFR 序列始于 2018-04。** 流动性层指标（SOFR−EFFR）在 2018 年之前无数据，这直接影响 2008-09 段的回测——状态机进入 BREWING 的条件（信用层 RED ∧ 流动性层 RED）在该段不可达，验收标准须分段降级（见第 6 节）。

**MOVE 指数** FRED 不提供，只能走 Yahoo `^MOVE`。Yahoo 非官方接口有脆弱性（既有 `collector/yahoo` 的测试覆盖率仅 17.2%，需注意），建议对 MOVE 做"缺数降级"处理：连续 3 日拉取失败时该指标标记 `STALE`，不参与共振计数，并发一条低优先级运维告警。

**兜底方案**：若 Yahoo 通道彻底不可用，MOVE 可暂以 VIX 单指标替代情绪层（体系退化但不中断）；USD/JPY 可切换到任一免费汇率 API（如 exchangerate.host / Frankfurter）。

### 2.2 派生指标计算

在 `internal/crisis` 中新增派生计算，不落在 collector 层：

```
sofr_effr_spread_bp = (SOFR − EFFR) × 100          // 单位 bp
usdjpy_wow_pct      = (close_t / close_t-5d − 1)   // 周环比，度量急升值
hy_oas_mom_bp       = (OAS_t − OAS_t-21d) × 100    // 月度走阔幅度
t10y2y_bp           = T10Y2Y × 100
percentile_5y(x)    = 当前值在滚动 5 年窗口内的分位  // 全指标通用
```

分位数计算依赖本地历史（见 2.1 的坑），窗口不足 5 年时用可得的最长窗口并在输出中标注 `window_actual`。

## 3. 状态判定规则

### 3.1 单指标三色规则

每个指标独立评为 GREEN / AMBER / RED。阈值采用"绝对阈值 + 分位数"双轨：绝对阈值来自历史危机的经验水位，分位数用于捕捉"相对本周期的异常"。**两轨任一触发即升级。**

| 指标 | GREEN | AMBER | RED |
|------|-------|-------|-----|
| VIX | < 25 | 25–30，或单周涨幅 > 50% | > 30 |
| MOVE | < 100 | 100–120 | > 120 |
| SOFR−EFFR | < +10bp | +10~+25bp 持续 ≥ 3 交易日 | > +25bp 持续 ≥ 5 交易日 |
| HY OAS | 350–500bp | **< 350bp（自满）** 或 500–600bp，或月走阔 > 100bp | > 600bp |
| 10Y−2Y | > +25bp | 0 ~ +25bp（趋平） | < 0（倒挂）；**倒挂后复陡 > +50bp 单独标记 `STEEPENING`** |
| NFCI | < −0.3 | −0.3 ~ 0 | > 0 |
| USD/JPY | 周变动 < 2% | 周升值 2–3%，或空头拥挤（价格处 52 周极端弱势）标记 `CROWDED` | 周升值 > 3% |

三处设计说明。其一，HY OAS 是**双向黄灯**——过紧（<350bp，自满）与偏宽（500–600bp）都挂黄，语义字段区分 `COMPLACENCY` 与 `STRESS`，两者的应对完全不同。其二，10Y−2Y 的 `STEEPENING` 标记很重要：历史上衰退往往发生在倒挂**结束后**的快速复陡阶段，而非倒挂期间，单纯"倒挂=红"会在最危险的窗口给出绿灯。其三，SOFR−EFFR 的持续性条件是降噪核心，配合 3.2 的日历抑制。

### 3.2 降噪与抑制规则

沿用告警治理的抑制思路：

1. **季末/年末抑制**：每季度最后 3 个交易日与新季度前 2 个交易日内，SOFR−EFFR 的 AMBER/RED 自动降为 `SUPPRESSED_SEASONAL`，仅记录不告警（回购市场技术性冲高是常态）。
2. **数据新鲜度**：任一序列超过预期发布时点 48 小时未更新 → 指标标记 `STALE`，退出共振计数，发运维级（P2）告警。
3. **单指标防抖**：状态升级立即生效；降级需连续 3 个观测日满足低档条件（不对称迟滞，防止在阈值附近来回翻转）。
4. **NFCI 周频对齐**：NFCI 在周三更新后参与当周所有日度评估，不做插值。
5. **无数据退出共振**：指标在评估日无观测（历史未回填或序列尚不存在，如 2018 前的 SOFR）→ 标记 `NO_DATA`，退出共振计数，不发告警。与 STALE 共用同一退出机制，回测早期段依赖此规则。

### 3.3 共振判定状态机（系统级）

系统整体处于四个状态之一，转移规则如下：

```
NORMAL ──[领先层任一 RED，或全系统 AMBER ≥ 3]──▶ WATCH
WATCH  ──[信用层 RED ∧ 流动性层 RED]──────────▶ BREWING
任意态 ──[情绪层双 RED（VIX>30 ∧ MOVE>120）]──▶ CRISIS
CRISIS ──[情绪层双指标回落至 GREEN 持续 10 交易日]──▶ WATCH
WATCH  ──[触发条件全部解除持续 20 交易日]─────▶ NORMAL
BREWING──[信用层与流动性层任一降回非 RED 持续 10 交易日]──▶ WATCH
```

各状态的行为差异：

| 状态 | 评估频率 | 通知策略 | 语义 |
|------|----------|----------|------|
| NORMAL | 每日评估，**月度摘要**推送 | 静默，仅月报 | 常态 |
| WATCH | 每日评估，**周度摘要**推送 | 状态变更即时通知（P1） | 观察期，提高警觉 |
| BREWING | 每日评估 + 盘中 USD/JPY 检查 | 每日推送（P1），进入时 P0 一次 | 危机酝酿，历史上此组合后 3–12 个月风险显著抬升 |
| CRISIS | 盘中多次评估 | P0 即时 | 危机进行中，执行预案而非预测 |

**注意状态机的输出语义**：BREWING ≠ "必发"。通知文案统一使用概率表述，模板中禁止出现"必然/一定/即将"字样——这是对原文章"3-12月内必发"伪精确的显式修正。

## 4. Atlas 集成设计

### 4.1 模块落位

```
atlas/
├── internal/
│   ├── collector/
│   │   └── fred/              # 新增：FRED API 采集器（key、限速、重试）；MOVE/JPY 复用 collector/yahoo
│   ├── crisis/                # 新增：独立包（有状态逻辑与 internal/alert 的无状态规则引擎语义差异大，不合并）
│   │   ├── ingest.go          # 7 序列采集编排，写入 sqlite
│   │   ├── rules.go           # 阈值规则引擎（配置驱动）
│   │   ├── statemachine.go    # 状态机（以 sqlite 为唯一真相源，进程无状态）
│   │   ├── suppress.go        # 日历抑制、防抖、新鲜度、NO_DATA
│   │   └── store.go           # macro_observations / crisis_evaluations 存取
│   └── notifier/              # 复用：telegram 通道，零改动
├── configs/
│   └── crisis-monitor.yaml    # 全部阈值与调度配置
├── deploy/launchd/            # 新增 3 个 plist（crisis-daily / crisis-nfci / crisis-intraday-jpy）
└── cmd/
    └── crisis.go              # CLI: atlas crisis status / backfill / eval（平铺，沿用现有 cmd 惯例）
```

规则引擎必须**配置驱动**——阈值写进 YAML 而非代码，未来调参不发版：

```yaml
# configs/crisis-monitor.yaml（节选）
indicators:
  hy_oas:
    source: fred
    series: BAMLH0A0HYM2
    thresholds:
      amber_low:  {value: 350, unit: bp, direction: below, tag: COMPLACENCY}
      amber_high: {value: 500, unit: bp, direction: above, tag: STRESS}
      red:        {value: 600, unit: bp, direction: above}
      amber_momentum: {change_bp: 100, window_days: 21}
    percentile: {window_years: 5, amber: 0.90, red: 0.97}
  sofr_effr_spread:
    derived: [SOFR, EFFR]
    thresholds:
      amber: {value: 10, unit: bp, persist_days: 3}
      red:   {value: 25, unit: bp, persist_days: 5}
    suppress:
      calendar: quarter_end   # 季末±规则见 suppress.go
state_machine:
  crisis_exit_days: 10
  watch_exit_days: 20
  demote_hysteresis_days: 3
```

### 4.2 存储模型

沿用 Atlas 现有 sqlite（`modernc.org/sqlite`，注意仓库约束：固定 v1.38.2 + GOTOOLCHAIN=local），新建两张表。数据量极小（20 年 × 7 指标 < 10 万行），不引入新数据库。时间列存 TEXT：观测日用 `YYYY-MM-DD`，时间戳用固定宽度 UTC RFC3339（参照 signal store 的排序惯例，字典序 = 时间序）：

```sql
-- 原始与派生观测
CREATE TABLE IF NOT EXISTS macro_observations (
    ts          TEXT NOT NULL,     -- 观测日 YYYY-MM-DD
    indicator   TEXT NOT NULL,     -- vix / move / sofr_effr / hy_oas / t10y2y / nfci / usdjpy
    value       REAL,
    source      TEXT,              -- fred / yahoo / manual_backfill
    fetched_at  TEXT,              -- UTC RFC3339 固定宽度
    PRIMARY KEY (ts, indicator)
);

-- 评估结果（审计与回测用，只追加）
CREATE TABLE IF NOT EXISTS crisis_evaluations (
    eval_at        TEXT NOT NULL,      -- UTC RFC3339 固定宽度
    indicator      TEXT,               -- NULL 表示系统级状态机记录
    status         TEXT,               -- GREEN/AMBER/RED/STALE/SUPPRESSED_SEASONAL/NO_DATA
    tag            TEXT,               -- COMPLACENCY/STRESS/CROWDED/STEEPENING
    value          REAL,
    pct_5y         REAL,
    system_state   TEXT,               -- NORMAL/WATCH/BREWING/CRISIS
    detail         TEXT                -- JSON 文本
);
```

审计表是刻意冗余的：每次评估全量落库，未来可以直接对历史危机（2008/2020/2024.8）做规则回测，验证阈值与状态机的召回率和误报率。

### 4.3 调度设计

Atlas 没有内置 cron 调度器，沿用现有部署模式：**launchd plist 定时唤起 CLI，进程无状态，状态机以 sqlite 为唯一真相源**（崩溃/重启零影响）。新增 3 个 plist：

| 任务 | plist | 唤起策略（本地时区） | 内容 |
|------|-------|---------------------|------|
| `daily_fetch_eval` | `crisis-daily` | 工作日多时点唤起（覆盖 ET 上午 10:30 前后） | 拉取 FRED 全部序列 + Yahoo MOVE/JPY，跑派生计算与全量评估，驱动状态机；应用内校验 T+1 数据是否已发布，未齐则退出等下次唤起（幂等由库保证） |
| `nfci_refresh` | `crisis-nfci` | 每周三（覆盖 ET 8:30 后） | NFCI 发布后单独刷新 |
| `intraday_jpy` | `crisis-intraday-jpy` | 每 30 分钟唤起 | 进程启动先读库中系统状态，**非 BREWING/CRISIS 立即退出**（空跑成本近零）；否则做 USD/JPY 盘中监测，捕捉 carry trade 急平仓 |

**时区与夏令时**：launchd 的 `StartCalendarInterval` 用机器本地时区，无法表达 ET，美国夏令时切换每年造成两次 ±1 小时漂移。缓解手段即上表的「多时点唤起 + 应用内数据齐备性校验 + 幂等」，不追求唤起时刻精确。

**月度摘要触发**：不加第 4 个 plist——daily eval 内判断「NORMAL 态 ∧ 当月首个交易日」即发月报。

**冷启动**：backfill 完成后系统状态直接初始化为 NORMAL（与附录基线一致），不做历史状态机重放；防抖计数从首次 eval 起累积。

失败重试：指数退避 3 次，最终失败走 2.1 的 STALE 流程。首次部署执行 `atlas crisis backfill --from 2006-01-01` 导入历史（HY OAS 需外部快照，经 `backfill --csv` 人工导入；其余 FRED 可直接回填）。

### 4.4 通知集成

复用 `internal/notifier` 的 telegram 通道，**不新增优先级路由**——现有 notifier 无 Priority 概念，本期不做公共化改造。所有通知走同一通道，文案前缀 `[P0]/[P1]/[P2]` 区分紧急度。未来若其他模块也需要分级路由，再提升为 notifier 公共能力。通知模板包含：当前系统状态、触发指标及读数、5 年分位、状态持续天数、下一评估时间。月度摘要（NORMAL 态唯一推送）附全部 7 指标的迷你趋势。

## 5. 边界声明

本模块输出的是**风险状态而非交易信号**。三点内建局限需要写进文档和通知页脚：其一，指标组合基于历史危机样本（样本量小），对新型传导路径（如 2020 外生冲击型）领先层可能完全失效，情绪层才是最后防线；其二，阈值存在过拟合风险，任何调参都应先过 `crisis_evaluations` 表的历史回测；其三，BREWING/CRISIS 状态的资产操作决策（降杠杆幅度、BTC 网格暂停与否）不在本模块范围内，属于 strategy 层的独立决策，本模块只提供输入。

**部署前置的人工依赖**：HY OAS 历史快照（2006 年起，来源为 ICE 官方或第三方 CSV 备份）必须在首次 backfill 前人工准备好——5 年分位和历史回测都依赖它，没有快照则第二阶段验收无法进行。

**本期范围外**：Grafana 面板（试运行两周后再评估）、strategy 层联动、notifier 优先级路由公共化。

## 6. 实施排期建议

分三步走。第一阶段（1–2 个晚上）：`collector/fred` + `crisis/ingest` + sqlite 存储表 + backfill CLI（含 `--csv` 人工导入），先把数据流跑通并核对读数与 FRED 官网一致。第二阶段（1–2 个晚上）：规则引擎 + 状态机 + 抑制逻辑，用 2008-09、2020-03、2024-08 三段历史数据做回测。**验收标准分段降级**（SOFR 始于 2018-04、MOVE 的 Yahoo 历史深度不保证覆盖早期）：

- **2020-03 与 2024-08**：全量验收——均在情绪层红灯前进入 WATCH 或 BREWING；
- **2008-09**：仅验收有数据的指标（VIX / HY OAS / 10Y−2Y / NFCI），要求它们在情绪层红灯前把系统推入 WATCH（BREWING 条件因流动性层 `NO_DATA` 不可达，不作要求）；
- **平静期误报**：2015–2019 误进 BREWING 不超过 1 次（缺数指标按 `NO_DATA` 规则退出共振）。

第三阶段（1 个晚上）：通知接入 + 3 个 launchd plist + 部署，试运行两周后再决定是否接 Grafana 面板。

---

*附：当前基线读数（2026-07-12 核实）——VIX 15.0，MOVE 69.6，SOFR−EFFR −10bp，HY OAS 267bp（COMPLACENCY 黄灯），10Y−2Y +35bp，NFCI −0.52，USD/JPY 161.7（CROWDED 黄灯）。系统状态应初始化为 NORMAL，AMBER 计数 2。*
