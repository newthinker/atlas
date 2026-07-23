---
title: Atlas-Prism 估值可视化模块设计方案
project: Atlas
module: Prism
status: reviewed
created: 2026-07-23
updated: 2026-07-23
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
| A/H 公司估值时序+分位 | 理杏仁(`collector/lixinger`,已有) | eastmoney/yahoo 重建 | 日 | ✅ 复用;现成 pe_ttm/pb 及分位,口径固定为市值加权 |
| A/H 指数估值+分位 | 理杏仁(已有) | — | 日 | ✅ 复用;官方口径序列,替代成分聚合 |
| 美股宽基指数估值+分位 | 理杏仁(已有) | 成分聚合(D3) | 日 | ✅ 复用;覆盖清单以实测为准(M1 首项任务) |
| A/H 财务报表科目 | 理杏仁(已有) | eastmoney | 季 | ✅ 复用;供财报桥模板填充 |
| 宏观利率(估值背景) | FRED(`collector/fred`,已有,Cassandra 在用) | — | 日 | ✅ 复用;10Y 收益率用于 ERP 展示 |
| 美股季报(营收/净利/EPS/股本) | **EDGAR companyfacts(`collector/edgar`,新建)** | yahoo | 季 | ★ M2 新建;EDGAR 免费(10 req/s 礼貌限速) |
| ETF 持仓权重 | **发行商 CSV(`collector/etfholdings`,新建)** | — | 周 | ★ M2 新建;权重变化慢,每周更新足够 |
| 分部营收(segment) | 公司模板 + EDGAR/理杏仁科目 | 手工录入 | 季 | ★ M3;D4 配置驱动 |
| 分析师前瞻 EPS | yahoo(已有,补充 info 快照) | — | 快照 | 仅当前值 |

**限频与计费纪律**:yahoo 全量约 600 只(去重后),日线增量每天一次,批量 + 间隔 + 复用现有 `collector/cache`,单日约 700 次,安全。理杏仁按理杏豆计量:所有请求先查本地库,只拉增量区间,禁止重复请求历史。

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

```yaml
# configs/prism/templates/msft.yaml
company: MSFT
cik: "0000789019"
currency: USD
structure:
  revenue_segments:                  # 左侧收入来源
    - key: productivity
      name_zh: 生产力与业务流程
      name_en: Productivity and Business Processes
    - key: intelligent_cloud
      name_zh: 智能云
      name_en: Intelligent Cloud
    - key: personal_computing
      name_zh: 更多个人计算
      name_en: More Personal Computing
  flow:                              # 收入 → 毛利 → 营业利润 → 净利
    - revenue -> [cogs, gross_profit]
    - gross_profit -> [opex, operating_income]
    - opex -> [rnd, sales_ga]
    - operating_income -> [tax_other, net_income]
data_mapping:
  source: edgar_segment              # 优先 XBRL segment, 失败转 manual
  fallback: manual
render:
  default_lang: zh
  color_scheme: msft                 # 复用图7的绿=利润/红=成本配色
```

新增一家公司 = 新增一个 YAML + 校验一次数据,预计 30 分钟/家。

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
├── /prism/sankey/:symbol 财报桥(图7/8)
│   ├── ECharts sankey(YoY/QoQ 标注) ↔ 分部堆叠柱状图切换
│   └── PNG 导出(ECharts toolbox.saveAsImage) + 中英文切换
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

**M2 — 美股深度(2~3 周)**
- `collector/edgar`(companyfacts,实现 FundamentalCollector 接口),首批 20~30 家。
- `internal/valuation` filing_date 升级(EPSPoint.FilingDate + 阶梯对齐改造,回归现有 pe_percentile 策略测试)。
- `collector/etfholdings` + D3 成分聚合引擎 → 行业/主题 ETF 估值。
- `/prism/compare` 多标的对比页。
- 验收:NVDA 10Y PE 曲线与图2形态一致,百分位数字可复算;XLK 聚合 PE 的 excluded_weight 正确标注。

**M3 — 财报桥(2 周)**
- `internal/prism/sankey` 模板加载/校验/填充 + ECharts 桑基渲染。
- 首批 5~10 家模板(MAG7 + TSM/AVGO/LLY 按需)。
- `/prism/fundamental` 财务趋势页(含股价叠加)+ 堆叠柱状图 + PNG 导出。

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
