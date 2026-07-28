---
title: Atlas-Prism 估值可视化模块设计方案
project: Atlas
module: Prism
status: reviewed
created: 2026-07-23
updated: 2026-07-25
tags: [atlas, valuation, visualization, design-doc]
---

# Atlas-Prism 估值可视化模块设计方案

> Prism(棱镜):把市场价格这束"白光"拆解成估值、基本面、增速等光谱分量,过滤噪音,看清本质。作为 Atlas 单体内的一个模块,与 Cassandra(宏观危机监控,`internal/crisis`)平行。
>
> **v2 修订说明**:v1 草案假设 Prism 为独立子系统(Python 采集 + TimescaleDB + React + NAS Docker),经与 Atlas 代码实况核对后全面修订工程架构——Prism 融入 Atlas Go 单体,复用已有采集器/估值引擎/通知/launchd 基建;金融方法论决策(D1/D3/D4/D6/D7)保持不变。

---

## 1. 总体定位与边界

**定位**:个人级估值观察工具,服务于中长期投资决策,不做实时行情、不做交易信号。数据粒度以「日」为最小单位,财务数据以「季度」为最小单位。

**三大功能域**:

| 功能域 | 对应需求 | 参考截图 |
|--------|---------|---------|
| ValuationBoard 估值面板 | 指数/ETF + 头部公司估值卡片、时序、百分位 | 图1/图2/图3 |
| FundamentalLens 基本面透镜 | 营收/净利/毛利率/ROE/增速趋势 + 股价叠加 | 图4/图5/图6 |
| EarningsBridge 财报桥 | 桑基图 + 分部营收堆叠柱状图,标注同环比 | 图7/图8 |

**覆盖范围**(按里程碑渐进,见 §9):
- 指数/ETF:约 20 个 —— 核心(SPY/QQQ/DIA/IWM/VTI/IJH)、行业(11 个 SPDR Sector)、主题(IGV/SOXX/SMH 等,列表配置化)
- 公司:A/H 头部公司(理杏仁现成数据,M1 即覆盖);美股公司首批 20~30 家,按需扩至 Top 100
- 财报桑基图:首批 5~10 家(MAG7 + TSM + AVGO + LLY),模板驱动逐家接入

---

## 2. 关键设计决策(先说结论)

### 2.1 金融方法论(v1 保留)

**D1 — 历史 PE 序列自己算,不依赖第三方现成序列(美股公司层)。**
第三方免费源没有可靠的长周期日频 PE 序列。正确做法:`PE(TTM)_t = Close_t / EPS_TTM_t`,其中 EPS_TTM 由季报滚动求和得到,在财报发布日之间保持阶梯状。这是整个系统最核心的计算,自己掌控口径才可审计。
**实现基础**:Atlas 已有 `internal/valuation/reconstruct.go`(`ReconstructPEPercentile`:EPS TTM 阶梯对齐日线 + 剔除非正 EPS + 分位计算),已服务于线上 `pe_percentile` 策略。Prism 在其上升级,而非重写(见 §5.1)。

**D2 — 美股基本面以 SEC EDGAR 为主源,yahoo 为快速通道。**
EDGAR `companyfacts` API 免费、权威、历史完整(XBRL 2009 年至今,足够覆盖 10Y/20Y 百分位需求);yahoo 的季报只有约 5 年,仅用于 M1 快速见效和数据交叉校验。

**D3 — ETF/指数估值用「成分股聚合」计算,不抓发行商页面。**
发行商披露的 PE 口径不一(是否剔除负盈利、调和平均 vs 加权平均),且无历史序列。自建口径:从发行商 CSV 拉取持仓权重(iShares/SSGA/Invesco 都提供每日 holdings 下载),对成分股 PE 做**加权调和平均**(剔除负 EPS 并重归一权重,口径固定并写入文档)。这样各指数的估值口径完全一致、可回溯。

**D4 — 桑基图走「模板 + 数据」分离的配置驱动路线。**
EDGAR 的分部(segment)数据在 XBRL 里极不规范,全自动解析不现实。方案:每家公司一个 YAML 模板定义收入分类结构和 XBRL tag 映射,数据自动拉取填充,模板人工维护。首批 5~10 家、上限 30~50 家热点公司的维护量可控。

**D4 现实注记(M3 落地时修正,2026-07-25 用户决策)**——原文只说「数据自动拉取填充」,没说从哪拉;实施时发现这一步比预想的重:

1. **companyfacts API 不含分部维度**。M2 已接入的 `companyfacts` 只返回合并口径的事实(每个 tag 一条时间序列),分部拆分存在于**报告实例文档**的 XBRL context 维度里,companyfacts 把它们压平丢弃了。**沿 M2 的数据通道拿不到分部数据,这不是参数问题而是接口能力问题。**
2. **改走 submissions API + 报告实例文档解析**(用户决策):先请求 `submissions` 拿到该公司的 filing 列表与 `primaryDocument`,再按 `.htm → _htm.xml` 取 inline XBRL 的实例文档,用 `encoding/xml` 单遍扫描,收集带**分部维度 `explicitMember`** 的营收事实。新增请求量:submissions 每标的每日 1 次 + 缺失报告期的实例文档(稳态每季 1 次/家),串行即天然 <10 req/s。
3. **公司自定义 member 名在模板中映射**——这正是 D4 原文「XBRL tag 映射」的本义,只是映射对象具体化为**实例文档 `explicitMember` 去命名空间后的 local name**(如 `IntelligentCloudMember`),而非 companyfacts 的 tag 名。分部维度轴默认 `StatementBusinessSegmentsAxis`,可在模板 `segment_axis` 逐家覆盖(首批 5 家实测:MSFT/GOOGL/AMZN 用默认轴,**AAPL/NVDA 必须覆盖为 `ProductOrServiceAxis`**,理由见 §7)。
4. **manual YAML 兜底**:`configs/prism/segments/{symbol}.yaml` 直接给出各报告期的分部数值。任一公司自动解析失败即转 manual,**不阻塞其余公司交付**;渲染层不区分来源。IFRS/20-F filer(如 TSM)无 US-GAAP inline XBRL 实例,自动解析路线不适用,只能纯 manual 或跳过。

**结论未变、路径变了**:D4「模板人工维护 + 数据自动填充」的判断成立,但「自动」的实现代价从「调一个现成 API」上升为「自建一个 XBRL 实例文档解析器」。

**D6 — PE(FWD) 只做快照,不做历史序列。**
前瞻 EPS 一致预期的历史数据是付费数据(IBES/FactSet),免费源只有当前快照。卡片上展示当前 PE(FWD),历史序列和百分位只基于 PE(TTM),避免口径污染。

**D7 — 理杏仁开放平台(已购付费)的使用边界:A/H 全量 + 美股仅指数。**
理杏仁 API 覆盖 A/H 公司与指数的估值时序(pe_ttm/pb/ps_ttm/mc 等)及现成分位数据(y5/y10/y20/全历史 × 市值加权/等权/中位数等口径),并提供财务报表科目;美股仅有指数接口(宽基指数估值+分位),无公司接口。因此:A/H 标的与美股宽基指数的百分位不再自算(固定口径:市值加权 + 对应窗口分位);美股公司层维持 D1/D2 的 EDGAR+yahoo 自建;美股行业/主题 ETF(XLK/SOXX/SMH/IGV 等)理杏仁不覆盖,维持 D3 成分聚合。实施前先调用理杏仁指数列表接口导出实际覆盖的美股指数清单,覆盖到哪个、哪个移出 D3。同一总览页上理杏仁分位与自建分位并存,分位窗口必须对齐(统一 10Y),卡片 footnote 标注数据来源。理杏仁按「理杏豆」计量,采集必须增量拉取、落库即缓存,禁止重复请求历史区间。
**实现基础**:Atlas 已有完整的理杏仁 Go 客户端(`internal/collector/lixinger/`:valuation/fundamental/fund/stock/history),base path 已配置化,percentile-watchlist 已在生产使用——Prism 直接复用,无新接入成本。

### 2.2 工程架构(v2 修订)

**D5' — 前端为 Atlas dashboard 内的 HTMX + ECharts 岛屿(替代 v1 的独立 React 应用)。**
Atlas 现有前端是 HTMX + Go templates(`internal/api/templates`),并无 Grafana。Prism 页面直接挂进现有 dashboard:页面骨架、筛选、导航用 HTMX + Go templates(与现有代码风格一致);每个图表是一个 ECharts 实例(vendor 一份 `echarts.min.js`,不引外部 CDN),数据由 `/api/prism/*` 返回 JSON。桑基图、折线/柱状、PNG 导出(`toolbox.saveAsImage`)、深浅色主题均为 ECharts 原生能力。零 node 构建链,单二进制部署形态不变。若 M3 财报桥交互复杂度超出 HTMX 舒适区,届时再评估仅桑基页局部升级。

**D8(新)— Prism 融入 Atlas Go 单体,不建平行子系统。**
v1 草案的 Python 采集层会重复建设 Atlas 已有的生产代码(理杏仁客户端、yahoo/FRED 采集器、EPS 阶梯重建、分位引擎均为有测试覆盖的 Go 代码)。修订为:新增 `internal/collector/edgar`(companyfacts REST,实现现有 `FundamentalCollector` 接口)与 `internal/collector/etfholdings`(发行商 CSV);聚合计算与桑基模板放 `internal/prism`;API 挂在现有 `internal/api` 下。单语言、单二进制、复用配置/日志/告警/测试基建。

**D9(新)— 存储用 SQLite,不引入 TimescaleDB。**
Atlas 实际无 TimescaleDB 实例(go.mod 仅 `modernc.org/sqlite`,hot storage 默认内存,signal store 为 SQLite)。Prism 数据量级:百余标的 × 20 年日频 ≈ 每表几十万到百万行,SQLite 绰绰有余。独立 `data/prism.db`(与 signal store 分库,互不干扰),开 WAL 模式支持采集进程与 serve 进程并发读写。冷归档如需要,复用现有 `storage/archive`(localfs/s3)。

**D10(新)— 里程碑按「现成数据先行」排序。**
理杏仁 A/H 与美股宽基指数数据已打通,是成本最低、最快见效的部分;EDGAR 采集器才是最大的从零工程量。因此 M1 先用现成数据把页面跑起来,EDGAR 深度后置到 M2(v1 草案把 A/H 放 v2 的排序颠倒了成本结构)。

---

## 3. 数据源矩阵

| 数据 | 主源(Atlas 模块) | 备源 | 频率 | 状态 |
|------|------|------|------|------|
| 美股日线收盘价 | yahoo(`collector/yahoo`,已有) | Stooq | 日 | ✅ 复用;复权价用于收益,**未复权价+对应股本**用于市值/PE |
| A/H 公司估值时序+分位 | akshare(`collector/akshare`,经本地 aktools 侧车) | 理杏仁/eastmoney | 日 | ✅ M1 落地;茅台/腾讯经 aktools 拉取(source=akshare) |
| A/H 指数估值+分位 | 理杏仁(`collector/lixinger`,已有) | akshare(aktools)降级 | 日 | ✅ 复用;理杏仁官方口径主源,不可用时降级 akshare(`fallback_source: akshare`) |
| 美股宽基指数估值+分位 | 理杏仁(已有) | 成分聚合(D3) | 日 | ✅ 复用;覆盖清单以实测为准(M1 首项任务) |
| A/H 财务报表科目 | 理杏仁(已有) | eastmoney | 季 | ✅ 复用;供财报桥模板填充 |
| 宏观利率(估值背景) | FRED(`collector/fred`,已有,Cassandra 在用) | — | 日 | ✅ 复用;10Y 收益率用于 ERP 展示 |
| 美股季报(营收/净利/EPS/股本) | **EDGAR companyfacts(`collector/edgar`)** | yahoo engine 重建 | 季 | ✅ M2 已落地;companyfacts 10Y + 真实 filing date 生效(防前视)+ 拆股归一化;失败降级 yahoo engine |
| ETF 持仓权重 | 发行商 CSV(`collector/etfholdings`,拟建) | — | 周 | ★ M3(原 M2,经用户决策推迟成分聚合到 M3) |
| 分部营收(segment) | 公司模板 + EDGAR/理杏仁科目 | 手工录入 | 季 | ★ M3;D4 配置驱动 |
| 分析师前瞻 EPS | yahoo(已有,补充 info 快照) | — | 快照 | 仅当前值 |

**限频与计费纪律**:yahoo 全量约 600 只(去重后),日线增量每天一次,批量 + 间隔 + 复用现有 `collector/cache`,单日约 700 次,安全——但**日总量安全 ≠ 突发安全**:2026-07-24 v1.4.0 首跑实测,prism-daily 20 家美股背靠背拉 10Y 行情即触发 yahoo 429,且 engine fallback 同依赖 yahoo 形成双失败(对策见 §9 M2.1 与 §10「yahoo 突发限流」)。理杏仁按理杏豆计量:所有请求先查本地库,只拉增量区间,禁止重复请求历史。

---

## 4. 系统架构

```
┌────────────────────────────────────────────────────────┐
│              Atlas 单体 (Go, launchd on macOS)          │
│                                                        │
│  internal/collector/                                   │
│  ├── yahoo, lixinger, fred, eastmoney   (已有, 复用)    │
│  ├── edgar/          ★新增  companyfacts REST          │
│  └── etfholdings/    ★新增  发行商 CSV 周更             │
│           │                                            │
│           ▼                                            │
│  internal/prism/     ★新增                             │
│  ├── engine/    估值序列计算(调用 internal/valuation)    │
│  ├── aggregate/ ETF 成分聚合 (D3)                       │
│  └── sankey/    桑基模板加载与填充 (D4)                  │
│           │                                            │
│           ▼                                            │
│  data/prism.db (SQLite, WAL)                           │
│           │                                            │
│           ▼                                            │
│  internal/api/                                         │
│  ├── /api/prism/*    ★新增  REST JSON                  │
│  └── /prism/*        ★新增  HTMX 页面 + ECharts 岛屿    │
│                                                        │
│  调度: launchd                                          │
│  ├── com.newthinker.atlas.refresh-us (已有, 行情)       │
│  └── com.newthinker.atlas.prism-daily ★新增            │
│      (采集增量 → 估值计算 → 落库; 失败走 notifier 告警)   │
│                                                        │
│  Tailscale ── Mac Studio / iPhone 访问                  │
└────────────────────────────────────────────────────────┘
```

采集、计算、服务全部为 Go,复用 Atlas 已有的配置(`internal/config`)、日志(`internal/logger`)、告警(`internal/notifier`:telegram/email/webhook)与 launchd 部署方式。采集失败通过现有告警通道通知。

### 与 Atlas 现有代码的映射

| Prism 需要 | Atlas 现状 | 动作 |
|-----------|-----------|------|
| 理杏仁估值/财务/分位 | `internal/collector/lixinger`(完整客户端) | 复用 |
| 美股/港股行情 | `internal/collector/yahoo` | 复用 |
| A 股行情 | `internal/collector/eastmoney` | 复用 |
| 宏观利率 | `internal/collector/fred` | 复用 |
| EPS 阶梯 + PE 分位 | `internal/valuation`(reconstruct/percentile) | **升级**:filing_date 生效、滚动分位、PB/PS(§5.1) |
| 基本面采集接口 | `internal/collector.FundamentalCollector` | edgar 采集器实现该接口 |
| 告警 | `internal/notifier` + `internal/alert` | 复用 |
| Web 框架 | `internal/api`(HTMX + templates) | 新增 prism handler/模板 |
| 调度 | `deploy/launchd/*.plist` | 新增 prism-daily plist |
| 存储 | `modernc.org/sqlite`(signal store 先例) | 新建 `data/prism.db` |

---

## 5. 核心计算逻辑

### 5.1 EPS_TTM 阶梯序列(升级现有 `internal/valuation`)

```
对每家公司:
1. 从 EDGAR 取季度 net income 与 diluted shares
2. EPS_q = NetIncome_q / DilutedShares_q
3. EPS_TTM(报告期) = 最近4个季度 EPS_q 之和
4. 生效日 = 财报 filing date(不是财季结束日!)
5. 日频序列:在两个 filing date 之间 EPS_TTM 保持常数
```

注意两个坑:一是**财年错位**(微软 FY 结束于 6 月、苹果 9 月、英伟达 1 月),统一按自然日历的 filing date 对齐,不按财季标签对齐;二是**避免前视偏差**,历史百分位计算必须用 filing date 生效,否则回看的估值分位会失真。

**对现有代码的具体升级**(而非重写):
- `core.EPSPoint` 增加 `FilingDate` 字段;`ReconstructPEPercentile` 的阶梯对齐从报告期日期改为 filing date 生效。EDGAR 数据带真实 filing date;yahoo 路径缺 filing date 时回退报告期日期 + 固定滞后(如 45 天),并在结果里标注口径。
- 该升级同时让已上线的 `pe_percentile` 监控策略受益(消除前视偏差)。
- 新增:滚动 5Y/10Y 分位序列、PB/PS(TTM) 序列计算,与现有 `PercentileRank` 同包共存。

### 5.2 估值指标

| 指标 | 公式 | 序列 |
|------|------|------|
| PE(TTM) | Close / EPS_TTM | 日频历史 |
| PE(FWD) | Close / 前瞻EPS | 仅快照 |
| PB | Close / BVPS(最新季报) | 日频历史 |
| PS(TTM) | 市值 / 营收TTM | 日频历史 |
| ROE(TTM) | 净利TTM / 平均净资产 | 季频 |

### 5.3 百分位体系(对齐图2的指标卡)

对任意估值序列 X 和当前值 x:
- **当前区间百分位**:x 在用户所选时间窗(1Y/3Y/5Y/10Y/20Y/MAX)内的分位
- **全历史百分位**:x 在全部可得历史内的分位
- **滚动百分位(5Y/10Y)**:每一天都计算"当天值在其之前 N 年窗口内的分位",形成图2右侧的百分位走势曲线

**估值状态标签**(卡片右上角):百分位 <15% → 低估(绿),15%~85% → 中性(灰),>85% → 高估(红)。阈值写入 YAML 配置,与现有 `percentile-watchlist.yaml` / `crisis-monitor.yaml` 风格一致。注意:Prism 展示阈值(15/85)与 percentile-watchlist 通知阈值(50/80)用途不同,各自独立配置,页面 footnote 说明口径。

### 5.4 ETF/指数 PE(D3 口径)

```
PE_etf = 1 / Σ( w_i / PE_i )        # 加权调和平均
其中:剔除 EPS_TTM ≤ 0 的成分股,权重重归一
     记录剔除比例 excluded_weight,>15% 时在卡片上标注 ⚠
```

历史序列:用当前持仓权重回算近 1 年(权重漂移可接受),更早历史用每周持仓快照逐段计算。

### 5.5 同环比标注(桑基图/柱状图)

每个收入分类节点标注:`YoY = (X_q / X_{q-4}) - 1`、`QoQ = (X_q / X_{q-1}) - 1`。季节性强的业务(如消费电子)默认展示 YoY,QoQ 灰显。

### 5.6 财报桥多期分析(M3,2026-07-25 需求确认)

**报告期模型**:两种粒度——季报(Q)与年报(FY)。年报**不新增采集**:流量科目(营收/成本/利润)由财年内 4 季聚合求和;EDGAR 的 FY 原值采集时可得,用作聚合校验。财年归属用 `fundamental_q.fiscal_period` 已存的 EDGAR fy/fp 标签(正确处理财年错位:NVDA 1 月、MSFT 6 月、AAPL 9 月),A/H 标的按自然年。**数据模型零变更**(`segment_revenue`/`fundamental_q` 主键本就含 fiscal_period)。

**范围并行视图(小倍数桑基网格 + 对比矩阵)**:用户指定报告期范围(from~to)与粒度(季/年),范围内每期渲染一张小桑基成网格;**跨图统一比例尺**——金额→流宽映射以范围内最大营收定标,结构变迁可直接目测。网格下方为对比矩阵:行 = 模板节点(分部营收/毛利/营业利润/净利)+ 派生比率(毛利率/营业利润率/净利率),列 = 各期,末列为同比(季度粒度)或区间 CAGR(年度粒度)。同屏期数上限 8,超出翻页。

**智能对比上下文(默认视图)**:不指定范围时,按最新财报类型自动确定对比集:
- 最新为**季报** → 对比集 = 当前财年内已发布各季(FYTD,如看 FY2026Q3 → Q1/Q2/Q3 并列),每列附去年同期同比;
- 最新为**年报**(财年第 4 季发布后)→ 对比集 = 库中全部历史年报(上限 10 年)+ YoY 列。

范围选择器是对默认视图的覆盖手段;单期渲染是范围=1 的特例,一套实现。范围跨越分部口径重述时,矩阵在断点列标注「口径变更」,桑基网格按各期所属模板版本渲染(§10 模板版本机制)。

---

## 6. 数据模型(SQLite: data/prism.db)

```sql
-- 标的主表
CREATE TABLE instrument (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  symbol      TEXT NOT NULL,
  type        TEXT NOT NULL,        -- 'stock' | 'etf' | 'index'
  market      TEXT NOT NULL,        -- 'US' | 'HK' | 'CN_A'
  name        TEXT,
  cik         TEXT,                  -- EDGAR CIK, 美股股票适用
  category    TEXT,                  -- '核心'|'行业'|'主题'
  meta        TEXT,                  -- JSON
  UNIQUE(symbol, type)
);

-- 日频价格
CREATE TABLE price_daily (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  d           TEXT NOT NULL,         -- 'YYYY-MM-DD'
  close       REAL,                  -- 未复权
  adj_close   REAL,
  volume      INTEGER,
  PRIMARY KEY (instrument_id, d)
) WITHOUT ROWID;

-- 季度财务事实(点位=报告期, 但带 filing_date 防前视)
CREATE TABLE fundamental_q (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  fiscal_period TEXT NOT NULL,       -- '2026Q1'
  period_end    TEXT NOT NULL,
  filing_date   TEXT NOT NULL,       -- 生效日
  revenue       REAL,
  net_income    REAL,
  gross_profit  REAL,
  equity        REAL,
  diluted_shares REAL,
  source        TEXT,                -- 'edgar' | 'yahoo' | 'lixinger' | 'manual'
  PRIMARY KEY (instrument_id, period_end)
) WITHOUT ROWID;

-- 日频估值序列(计算结果; 理杏仁标的直接落原始序列+分位)
CREATE TABLE valuation_daily (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  d           TEXT NOT NULL,
  pe_ttm      REAL,
  pb          REAL,
  ps_ttm      REAL,
  pctl_5y     REAL,                  -- 滚动5Y百分位
  pctl_10y    REAL,
  source      TEXT,                  -- 'engine' | 'lixinger'
  PRIMARY KEY (instrument_id, d)
) WITHOUT ROWID;

-- 分部营收(桑基/堆叠柱数据)
CREATE TABLE segment_revenue (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  fiscal_period TEXT NOT NULL,
  segment_key   TEXT NOT NULL,       -- 对应模板中的节点 key
  revenue       REAL,
  PRIMARY KEY (instrument_id, fiscal_period, segment_key)
) WITHOUT ROWID;

-- ETF 持仓周快照
CREATE TABLE etf_holdings (
  etf_id      INTEGER NOT NULL REFERENCES instrument(id),
  snap_date   TEXT NOT NULL,
  holdings    TEXT NOT NULL,         -- JSON: [{symbol, weight}, ...]
  PRIMARY KEY (etf_id, snap_date)
) WITHOUT ROWID;
```

连接参数:`_journal_mode=WAL`(采集进程与 serve 进程并发读写)、`_busy_timeout=5000`。理杏仁返回的现成分位落 `valuation_daily`(source='lixinger'),与自建引擎结果同表不同 source,查询侧按 source 标注数据来源。

## 7. 桑基图模板(D4 示例)

**以下为 M3 落地后的实际 schema**(`internal/prism/sankey/template.go`),相对本节初稿**移除了 `flow` / `data_mapping` / `render` 三段**,理由见下方 AD-5 说明:

```yaml
# configs/prism/templates/msft.yaml
# Prism 财报桥模板:只定义分部与 XBRL member 映射。
# 主干流(收入→毛利→营利→净利)不在此配置(AD-5),由 periods 引擎从 fundamental_q 自动构建。
# xbrl_member = 实例文档 explicitMember 去命名空间后的 local name。
company: MSFT
cik: "789019"                                  # 纯数字,不带前导零填充
segment_axis: StatementBusinessSegmentsAxis    # 可省略,默认即此值
version: 1                                     # 分部口径重述时递增(§10 断点机制)
segments:
  - {key: productivity,       name_zh: 生产力与业务流程, name_en: Productivity and Business Processes, xbrl_member: ProductivityAndBusinessProcessesMember}
  - {key: intelligent_cloud,  name_zh: 智能云,          name_en: Intelligent Cloud,                   xbrl_member: IntelligentCloudMember}
  - {key: personal_computing, name_zh: 更多个人计算,     name_en: More Personal Computing,             xbrl_member: MorePersonalComputingMember}
# since_fy: 2024                               # 可选,该模板版本自哪个财年起生效
```

**AD-5 — 主干流不进模板。** 初稿的 `flow` 段(revenue→cogs/gross_profit→opex/operating_income→tax/net_income)对**所有公司完全同构**,且这些科目已由 companyfacts 扩展科目落进 `fundamental_q`。写进模板等于让每家公司重复抄一遍同样的结构,并制造「模板写的流 ≠ 引擎实际构建的流」这一类脱节风险。故主干流由 `periods` 引擎硬编码构建,模板只负责**左侧分部定义与 XBRL member 映射**这一真正逐家不同的部分。

同时移除的另两段:`data_mapping`(`source: edgar_segment` / `fallback: manual`)不必逐家声明——刷新编排固定「先自动解析、失败转 manual」;`render`(语言/配色)属前端展示偏好,已落在页面交互(中英切换按钮)与统一主题里,不需要每家配一份。

**逐家真正要填的只有两件事**:该公司的分部维度轴、以及各分部的 member 名。两者都必须**读真实实例文档实测确定,不能靠猜**,M3 首批模板的两类实测陷阱:

- **默认轴对部分公司零产出**:AAPL/NVDA 的**可报告分部**挂在 `ConsolidationItemsAxis × StatementBusinessSegmentsAxis` 的**双 member** context 上,而解析器只收「恰好一个 `explicitMember`」的 context(避免交叉维重复计数),故默认轴对它们**零产出**。改用单 member 的 `ProductOrServiceAxis`(产品线/市场平台维度)才可解析,且更适合收入桑基。
- **必须排除汇总项 member**:实例文档里汇总 member 与其明细 member 并存(实测 AAPL `ProductMember` = iPhone+Mac+iPad+Wearables;NVDA `DataCenterMember` = Compute+Networking,均分毫不差)。一并映射会重复计算——AAPL 朴素合计为真实营收的 **1.74 倍**、NVDA 为 **1.90 倍**。模板只映射互不重叠的项,汇总 member 会作为「未映射 member」进 refresh 的 Degraded 文本,**属预期行为**。

新增一家公司 = 新增一个 YAML + 用实例文档校验一次数字(Σ 分部 = 合并营收) + **重启 serve**(模板在启动时读入一次)。改模板后重拉历史数据需 `atlas prism refresh --full-segments`(AD-12)。

---

## 8. 前端页面结构(HTMX + ECharts 岛屿)

挂载在现有 Atlas dashboard(`internal/api`)内,与现有页面共用布局与深浅色主题;vendor 一份 `echarts.min.js`(不引外部 CDN)。

```
Atlas dashboard
├── /prism/board          估值总览(图1)
│   ├── Go template 渲染卡片矩阵: PE(TTM)/PE(FWD)/PB/1Y PE变化/百分位/状态标签
│   ├── 组别筛选(核心/行业/主题/A股/港股) + 排序 + 搜索 → hx-get 局部刷新
│   └── 卡片底部: 数据区间 + 百分位滑轨条(纯 CSS) + 数据来源 footnote
├── /prism/detail/:symbol 估值时序(图2)
│   ├── 左: PE(TTM) 历史曲线(ECharts) + 时间窗切换(MAX/20Y/10Y/5Y/3Y/1Y)
│   ├── 右: 滚动百分位走势(ECharts)
│   └── 底: 指标卡(当前值/各口径百分位/区间变动/区间高低, Go template)
├── /prism/compare        多标的对比(图3)
│   ├── 标的多选(≤8) + 指标选择 + 区间对齐
│   └── ECharts 折线对比 + 当前横截面表(值/百分位/状态)
├── /prism/fundamental    财务趋势(图4/5/6)
│   ├── 指标切换: 营收/净利/毛利率/PE/ROE/营收增速/利润增速
│   ├── ECharts 折线/柱状切换 + 股价叠加(双轴)
│   └── 季度/年度/滚动年化 + ECharts dataZoom 双端时间滑块
├── /prism/sankey/:symbol 财报桥(图7/8 + §5.6 多期分析)
│   ├── 报告期范围选择器(from~to)+ 粒度切换(季/年);默认视图 = 智能对比上下文(§5.6)
│   ├── 小倍数桑基网格(跨图统一比例尺,≤8 期) ↔ 分部堆叠柱状图切换
│   ├── 对比矩阵(模板节点+比率 × 各期,末列 YoY/CAGR;重述断点标注)
│   └── 单期大图模式(YoY/QoQ 节点标注) + PNG 导出 + 中英文切换
└── /prism/settings       偏好设置(标的池/阈值/自选池)

API: /api/prism/{board,series,compare,fundamental,sankey}  → JSON,供 ECharts 消费
```

交互模式:页面骨架与表格由服务端渲染(HTMX 局部刷新);图表容器内嵌少量 vanilla JS 初始化 ECharts,时间窗/指标切换重拉 JSON 重渲染。若 M3 桑基页交互复杂度超出该模式,再评估局部升级(见 D5')。

---

## 9. 实施路线(现成数据先行)

**M1 — 现成数据上墙(1~2 周)**
- 首项任务:实测理杏仁美股指数覆盖清单(决定哪些指数走理杏仁、哪些留给 M2 的 D3 聚合)。
- prism.db 建库 + 理杏仁增量采集落库(A/H 公司+指数、美股宽基指数,含现成分位)。
- 美股公司先走 yahoo + 现有 `ReconstructPEPercentile` 路径(近 5Y,标注口径)。
- `/prism/board` 卡片页 + `/prism/detail` 时序页上线;prism-daily launchd plist。
- 验收:茅台/沪深300 的 PE 与分位和理杏仁官网一致;NVDA 5Y PE 曲线形态正确;停掉采集一天,页面显示数据日期而非报错。

**M2 — 美股深度(已落地)**
- `collector/edgar`(companyfacts,实现 FundamentalCollector 接口),首批 20 家 US-GAAP filer。
- `internal/valuation` filing_date 升级(EPSPoint.FilingDate + 阶梯对齐改造,回归现有 pe_percentile 策略测试)。
- EDGAR 季度化(单季/Q4 按实际期间推导)+ **每股值拆股归一化**(拆股后每股值重述统一到最新基准,防止派生 Q4 符号矛盾与跨拆股日毛刺)。
- `/prism/compare` 多标的对比页。
- **范围变更**:`collector/etfholdings` + D3 成分聚合(行业/主题 ETF 估值)经用户决策**推迟到 M3**,不在 M2 交付。
- 验收:NVDA 10Y PE 曲线与图2形态一致,百分位数字可复算(见本文件同目录计划 `docs/superpowers/plans/2026-07-24-prism-m2.md` Task 7 验收记录)。

**M2.1 — yahoo 客户端限流加固(小补丁,生产问题驱动)**
- 背景:2026-07-24 v1.4.0 首跑,launchd 直连环境下 prism-daily 20 家美股连续拉 10Y 行情触发 yahoo 429;refreshEdgar 卡在价格阶段,fallback engine 同走 yahoo → 20 家双失败不降级(`20 failed / 0 degraded`);手动经代理(不同出口 IP)重跑恢复。M1 时代每日仅 3~4 个 yahoo 请求故从未暴露,美股扩容(M4 Top 100)前必须解决。
- 设计(改 `internal/collector/yahoo` 客户端层,所有调用方受益,零调用点改动):
  1. **全局请求节流**:同 host 相邻请求最小间隔约 500ms(串行刷新 20 家仅增约 10s,08:30 调度窗口无感;M4 扩到 100 家约 +50s,仍可接受)。
  2. **429/5xx 指数退避重试**:1s→2s→4s 最多 3 次,优先尊重 `Retry-After` 响应头;仅对幂等 GET 生效。
  3. **全局重试预算封顶**(如单次 refresh 累计 ≤20 次重试):极端持续限流下快速失败,不拖长调度窗口;超出预算的标的按既有语义走 fallback/Failed。
  4. 参数取包内常量,不进 config(无按环境调参需求,简单优先)。
- 验收:httptest 单测——首响 429(带/不带 Retry-After)→ 退避后重试成功;连续 429 → 3 次后失败并保留原始错误;节流生效(相邻请求时间差 ≥ 间隔)。生产观察:prism-daily 定时首跑 20 家 `0 failed`(XOM 优雅降级除外)。
- 明确不做:plist 注入 proxy env 的代理方案——引入「代理进程可用性」这一新故障面,劣于客户端自愈;Stooq 备源仍按 §10 既有条目独立演进。
- **发布后记(2026-07-25,v1.4.1)**:节流+退避已上线并保留,但「不做代理」的前提被证伪——launchd 直连复测仍 20 failed,区分实验证实根因是 **yahoo 对本机直连出口的持续 IP 级 403 封禁**(非突发限流,客户端自愈无解)。经用户决策改为「代理先行+Stooq 立项」:6 个 yahoo 依赖服务 plist 注入代理 env(含 no_proxy 豁免本机侧车)后 launchd 实测 `25 ok, 0 failed, 5 degraded`(degraded=EDGAR tag 差异类,见 §10)。附带发现并救回 refresh-us 主管线(已静默停更 ~23 天)。

**M3 — 财报桥主线(已落地;2026-07-25 拆批决策,计划与验收记录见 `docs/superpowers/plans/2026-07-25-prism-m3.md`)**

落地状态一览(判定对象 `ba40d70`,20 个任务 / 30 个 commit;**验收按 AD-7 二分,「实测」与「待人工验收」不混称**):

| M3 范围项 | 状态 | 实测证据 / 缺口 |
|---|---|---|
| EDGAR tag 回退扩展 + Degraded 明细日志 | ✅ 已落地 | 回退链与明细打印均有单测 + 变异守护;**COST/V/CRM/WMT/AVGO 的生产实效未在本 sprint 实测**(不在首批模板池内),待人工在 prism-daily 日志确认 |
| `internal/prism/sankey` 模板加载/校验/填充 + 主干流自动构建 | ✅ 已落地 | 模板 schema 见 §7(含 AD-5);主干流由 periods 引擎从 `fundamental_q` 构建,不进模板 |
| 分部营收 XBRL filings 自动解析(submissions + 实例文档) | ✅ 已落地 | 见 D4 现实注记;首批 **5/5 全自动解析成功** |
| manual YAML 兜底 | ⚠ 已实现但**无实证** | 5/5 自动成功 → `configs/prism/segments/` 目录至今未创建,该路径本 sprint **未被触发验证** |
| 多期分析(§5.6:范围并行 / 小倍数网格 / 对比矩阵 / 智能对比上下文 / 年报由季度聚合) | ✅ 已落地 | 引擎侧有单测与变异守护(含 TASK-016/017 的标签冲突与跨季相加防御);**页面渲染效果待人工目检** |
| 首批模板 5~10 家 | ✅ **下限达成:5 家**(MSFT/AAPL/GOOGL/AMZN/NVDA) | 逐家经真实 10-K/10-Q 实例文档实测(Σ 分部 = 合并营收);**META/TSLA/AVGO/LLY 未做;TSM 为 IFRS/20-F,自动解析不适用,未纳入本批** |
| `/prism/fundamental` 财务趋势页 + `price_daily` 落库 | ✅ 已落地 | HTTP 200、DOM 锚点、404 语义均有变异守护;**双轴目视、中英切换、曲线形态待人工验收** |
| 堆叠柱状图 + PNG 导出 | ✅ 已落地,**位置与本节初稿不同** | 两者都在**桑基页**(`sankey-stack` + 单期图/堆叠图各一个 `saveAsImage` toolbox);`/prism/fundamental` 页是否需要 PNG 导出未列入其 DoD,**未做**。**实际导出行为待人工验收** |
| **null 语义的三处缺口(M-1/M-2/M-3)** | ⚠ **无自动化守护,已立人工验收清单** | 纯 JS 逻辑 Go 测试无法执行;静态守卫止于文本级。三处经变异实证「存活 0/10」,清单见计划文件验收记录 §3.2 |
| Stooq 备源 / ETF 成分聚合(D3) | ➡ **移出 M3,归 M3.5** | 见下 |

- **EDGAR tag 回退扩展(前置,2026-07-25 立项)**:EPS tag 回退链(EarningsPerShareBasicAndDiluted → NetIncome÷DilutedShares 推算)与股本/equity tag 扩展,救回 COST/V/CRM/WMT 的 EDGAR 全功能与 AVGO 的 PB;同时给 prism refresh 日志打印 Degraded/Failed 明细(观测缺口)。财报桥依赖 fundamental_q 数据完整性,故置于主线首位。
- `internal/prism/sankey` 模板加载/校验/填充 + ECharts 桑基渲染;主干流(收入→毛利→营利→净利)由 companyfacts 扩展科目全自动。
- **分部营收数据源(2026-07-25 用户决策)**:companyfacts API 不含分部(segment)维度——走 **XBRL filings 自动解析**(submissions API + 报告实例文档),公司自定义 member 名在模板中映射(即 D4「XBRL tag 映射」本义),manual YAML 数据文件兜底;任一公司解析失败转 manual,不阻塞交付。
- **多期分析(§5.6,2026-07-25 需求确认)**:范围并行视图(小倍数桑基网格 + 对比矩阵)、智能对比上下文(季报→FYTD 各季;年报→历史年报 ≤10 年)、年报由季度聚合(数据模型零变更)。
- 首批 5~10 家模板(MAG7 + AVGO/LLY 按需;**TSM 为 IFRS/20-F,自动解析不适用,纯 manual 或本批跳过**)。
- `/prism/fundamental` 财务趋势页(含股价叠加,新增 price_daily 落库)+ 堆叠柱状图 + PNG 导出。

**M3.5 — 数据链路基建(与财报桥解耦,主线后另拆计划)**
- **Stooq 备源接入(2026-07-25 用户立项)**:新增 `internal/collector/stooq` 作 yahoo 价格失败备源,消除「本机代理可用性」单点;先调研 Stooq 对首批标的的覆盖/复权口径。
- ETF 成分聚合(D3)+ etfholdings(自 M2 顺延)。

**M4 — 扩展(按需)**
- 美股扩容至 Top 100;桑基模板逐家增补(上限 30~50 家)。
- ERP(PE 倒数 - FRED 10Y 国债)展示;Obsidian 周报导出(带 frontmatter 的估值周报存入 ClawdVault)。
- v1 草案中的 AKShare 备源仅在理杏仁不可用时再评估。

---

## 10. 已知风险与对策

| 风险 | 影响 | 对策 |
|------|------|------|
| 理杏仁美股指数覆盖不及预期 | M1 宽基指数缺口 | M1 首项任务实测清单;缺口指数回退 M2 的 D3 成分聚合 |
| 理杏仁 URL/接口改版(历史发生过) | 采集中断 | base path 已配置化(现有 lixinger 客户端);失败走 notifier 告警 |
| yahoo 非官方 API 变动 | 价格采集中断 | Stooq 备源 + 采集失败告警(复用现有通道) |
| yahoo 突发限流(429;2026-07-24 v1.4.0 首跑实测) | prism-daily 美股批量刷新整批失败——fallback engine 同依赖 yahoo,双失败不降级;EDGAR 事实层不受影响,仅价格阶段受阻 | M2.1 客户端节流 + 退避(见 §9);扩容(M4)前为硬前置 |
| **yahoo 直连持续 403(根因修正,2026-07-25)**:本网络直连出口被 yahoo IP 级封禁(间隔 14h 单发仍 403),非突发限流——launchd 无代理 env 致 refresh-us 主管线静默停更 ~23 天(health check 自报 age 23d 无人看)、prism 美股整批失败;crisis plist 早已发现并修复(注释在案)但未推广到其余服务 | 美股行情全链路(refresh-us/prism-daily/serve backtest/crisis yahoo 快照) | 已修复:6 个 yahoo 依赖服务 plist 注入 http(s)_proxy=127.0.0.1:7897 + no_proxy 豁免本机侧车(install-services 重载);M2.1 节流退避保留(防真实突发限流);**Stooq 备源立项 M3(用户决策,消除代理单点)**;教训:环境级修复必须全服务推广并留档,勿只修出问题的那个 |
| EDGAR EPS/股本 tag 形态差异:COST/V/CRM/WMT 的 EarningsPerShareDiluted 单季条目缺失(70-71/71 NULL,V 连 diluted_shares 全缺),AVGO equity 29/36 缺失 | 4 家走 engine 降级(PE 有/PB-PS 缺)+AVGO PB 整列 NaN——首批 20 家实测 15 家 EDGAR 全功能,5 degraded/1 部分降级 | 按设计优雅降级已兜底(refresh 报告可见 degraded 计数);M3 follow-up:EPS tag 回退链(EarningsPerShareBasicAndDiluted/NetIncome÷shares 推算)与股本 tag 扩展;顺带修 refresh 日志不打印 Degraded 明细的观测缺口 |
| EDGAR 瞬态网络失败(2026-07-28 prism-daily 实测:经代理 TLS handshake timeout/EOF/200+空响应三种形态) | 单次抖动即整家失败(edgar 曾是三个采集器中唯一无重试的);200+空响应命中 ErrNotUSGAAP 误判,把 XOM 误报为「IFRS filer」 | 已修复(2026-07-28 hotfix):companyfacts 拉取加退避重试(1s/2s/4s,覆盖传输层错误/429/5xx/截断 JSON/空 facts);空 facts 判为瞬态错误参与重试,仅当存在其他命名空间(ifrs-full 等)而无 us-gaap 时才判 IFRS。yahoo 传输层错误仍不重试(记录在案的设计决定,如再成为双失败主因另行评估) |
| EDGAR XBRL tag 不一致(公司自定义 tag) | 财务数据缺漏 | 逐家校验 + source 字段标记 + manual 兜底 |
| 分部口径重述(公司调整业务分类) | 桑基历史断层 | 模板带版本号,重述时新旧模板并存,图上标注断点 |
| ETF 成分聚合 PE 与市面数字有差异 | 用户困惑 | 口径写入页面 footnote,一致性比"对齐别人"重要 |
| 前瞻 PE 无历史 | 百分位只有 TTM 口径 | 明确标注,不混用 |
| 股本变动(回购/增发)导致 EPS 阶梯跳变 | PE 曲线毛刺 | 用 diluted shares 逐季更新,跳变属真实信号,不平滑 |
| SQLite 并发写(采集 job 与 serve 同时写) | 偶发 busy | WAL + busy_timeout;采集统一走 prism-daily 单进程串行 |
| filing_date 升级影响线上 pe_percentile 策略 | 分位数字变动引发误报 | 升级带回归测试;上线首周对比新旧分位差异并在通知中标注口径切换 |

---

## 11. 与 Atlas 生态的关系

Prism 是 Atlas 单体内的平级模块:复用采集器框架、`internal/valuation` 估值引擎、notifier 告警、HTMX dashboard 与 launchd 调度;与 Cassandra(`internal/crisis`)互补——Cassandra 回答"系统性风险在哪个状态",Prism 回答"具体资产贵不贵"。两者结合即:宏观状态机 × 微观估值分位,构成完整的买卖决策参考面板。现有 dashboard 导航各留一个入口。

与现有 percentile-watchlist 监控的关系:同一套数据与引擎,两种消费方式——watchlist 是**推**(分位越线主动通知),Prism 是**拉**(打开面板主动查看);阈值各自独立配置。
