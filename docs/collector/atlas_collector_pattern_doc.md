---
title: Atlas 行情数据采集层优化方案 —— 借鉴 daily_stock_analysis 数据模块模式
project: Atlas
module: Collector
status: draft
created: 2026-07-26
refs:
  - "[[Atlas-Prism 估值可视化模块设计方案]]"
  - "https://github.com/ZhuLinsen/daily_stock_analysis (MIT)"
tags: [atlas, prism, collector, design-doc, pattern]
---

# Atlas 行情数据采集层优化方案

> 目标:提炼 daily_stock_analysis(下称 DSA)行情数据模块(`data_provider/`)中经过大规模验证的工程模式,应用于 Atlas/Prism 的 Go 采集层。**只借模式与经验,不引入其代码或依赖。**

---

## 1. 参考项目分析结论

DSA(56k+ stars,MIT,持续活跃)的定位是 LLM 驱动的每日股票分析系统,与 Atlas/Prism 的"长历史估值序列"定位差异很大,但其行情数据模块是整个项目工程化最扎实的部分。其核心做法:

| DSA 做法 | 要点 |
|----------|------|
| 多源 Fetcher 体系 | 每个数据源一个 Fetcher,统一契约;免费源(AkShare/Baostock/YFinance)零配置兜底,token 源(Tushare/Longbridge/TickFlow)提升稳定性 |
| 按市场路由 | 市场代码识别集中到共享工具;每个市场只允许特定 fetcher 链(如日/韩/台股只走 yfinance,绝不尝试 A股专属源) |
| 能力边界显式化 | 市场不支持的能力按 `not_supported` 显式降级,而非静默缺数 |
| 质量元数据 | 行情返回携带 market / currency / data_quality / missing_fields / provider / as_of,向下游透传 |
| 韧性设计 | 全链路 fail-open(单源失败不中断主流程)、并发缓存防击穿、按端点分流的熔断保护 |
| 交易日历 | exchange-calendars 注册各交易所日历,识别交易日与盘中阶段,非交易日不执行 |

**定位错位(即不借的部分)**:DSA 服务"当日快照 + 近期 K 线"的即时分析,无长历史持久化概念,不碰 SEC EDGAR,实时行情类型与 LLM 上下文组装均与 Prism 无关。因此**不建议引入其模块作为依赖**(且语言栈不同),仅移植设计模式。

---

## 2. 借鉴原则

一、**借模式不借代码**:Atlas 为 Go 项目,DSA 为 Python,所有模式在 Go 语境下重新实现,复用 Atlas 已有的配置、日志、告警基建。
二、**保持小而可审计**:六个模式全部落在一个采集层 package 内,不引入框架级依赖。
三、**与 Prism 设计文档对齐**:本方案是 Prism 设计文档第 3~4 节(数据源矩阵/系统架构)的实施细化,决策编号沿用 D1~D7,优化点编号 O1~O6。

---

## 3. 六大优化模式

### O1 统一 Fetcher 契约

**模式**:所有数据源实现同一接口,声明自身支持的「能力」与「市场」;新增数据源 = 新增一个实现 + 一行配置,上层代码零改动。

**Go 落地**:定义 `Fetcher` 接口,包含三要素——名称标识、能力集与市场集的声明方法(`Supports(market, capability) bool`)、按能力分发的抓取方法。能力枚举三种:`daily_price`(日线)、`valuation`(现成估值/分位序列)、`fundamental`(季度财务事实)。首批四个实现:LxrFetcher(理杏仁)、EdgarFetcher(SEC)、YfFetcher(价格主源)、StooqFetcher(价格备源)。

**替代现状**:原方案四个采集脚本各写各的入口与错误处理,契约化后统一为一个调度循环。

### O2 配置驱动的路由与降级链

**模式**:`(市场, 能力) → [fetcher 优先级列表]` 全部写在配置文件中,运行时逐个尝试,失败降级到下一个;**路由表中缺失的组合即为 not_supported**,能力边界从代码里的 if/else 变成配置中的"留白"。

**Go 落地**:Router 组件读取 YAML 路由表(风格与 Cassandra 的 YAML 阈值配置一致)。关键路由决策直接对应 Prism 决策 D7:

| 市场 | daily_price | valuation | fundamental |
|------|-------------|-----------|-------------|
| cn | yfinance | lixinger | lixinger |
| hk | yfinance | lixinger | lixinger |
| us | yfinance → stooq | (留白=自算,D1) | edgar |
| us 宽基指数 | — | lixinger | — |

降级发生时记录结构化日志(原链首源 + 实际命中源),便于事后审计免费源的可用率。

### O3 质量元数据贯穿

**模式**:每次抓取返回统一结果结构,除数据本体外携带 provider、as_of(抓取时刻)、data_quality(ok/partial/empty 三态)、missing_fields、market、currency;落库时元数据一并入库,任何一条数据可追溯来源与完整度。

**Go 落地**:定义 `FetchResult` 结构体;TimescaleDB 表结构在 Prism 设计文档第 6 节基础上增加三列:`provider TEXT`、`fetched_at TIMESTAMPTZ`、`data_quality TEXT`,`fundamental_q` 额外加 `missing_fields TEXT[]`。

**Prism 特有价值**:总览页上理杏仁分位与自建分位并存(D7),元数据是前端 footnote 标注数据来源的直接依据;EDGAR 自定义 tag 导致的字段缺失靠 `data_quality=partial` 批量筛出问题公司。

### O4 集中式市场识别

**模式**:市场代码识别规则(cn: 6 位数字 / hk: hk 前缀或 .HK 后缀 / us: 字母代码)只在一个工具模块实现一次,路由、日历、代码标准化、落库共用,消除多市场扩展时的规则漂移——这是 DSA 支撑六个市场仍保持一致性的关键。

**Go 落地**:`marketutil` 包提供 `DetectMarket(symbol)` 与 `NormalizeSymbol(symbol, market)` 两个纯函数,配单元测试锁定规则;指数标的用 `IDX:` 前缀与个股区分。

### O5 交易日历门控

**模式**:定时任务触发后先判断目标市场当日(或前一交易日)是否为交易日,非交易日直接跳过该市场的采集,消除节假日空跑与"数据未更新"误报。

**Go 落地**:注册三个交易所日历(XSHG 上交所 / XHKG 港交所 / XNYS 纽交所)。Go 生态无 exchange-calendars 等价库时,退化方案为:节假日表由采集器每年从交易所公告更新一次并落库,查询时读表。**语义取舍与 DSA 一致——fail-open**:日历数据缺失时放行采集,宁可空跑一次,不可漏采一天。

### O6 熔断与防击穿

**模式**:按 provider 维度熔断(连续 N 次失败进入冷却期,冷却后半开放行一次探测);同一(标的, 区间)请求在单轮采集内去重。

**Go 落地**:轻量熔断器(计数 + 时间戳,无需引入 hystrix 类库);进程内 map 做 in-flight 去重。**对理杏仁的特殊意义**:该源按理杏豆计费,熔断避免上游异常时把配额烧在无效重试上。熔断开启事件接入 Atlas 现有告警通道(与 Cassandra 同一路径)。

---

## 4. 配套机制(非 DSA 借鉴,但与模式协同)

**增量水位(省豆核心)**:每次采集前查询库内该标的的最大日期作为水位,只拉取水位之后的缺口;首次全量回补后,理杏仁的日常消耗降到最小请求量。这是 DSA 没有(也不需要)的机制,由 Prism 的持久化定位决定。

**采集统计与告警阈值**:每轮采集输出 fetched / skipped / empty 计数;empty 占比超过阈值(建议 10%)时经 Atlas 告警通道通知,提示上游源可能整体异常。

**限速**:免费源请求间隔配置化(默认 0.5s),与熔断分层——限速防"惹恼上游",熔断防"上游已怒"。

---

## 5. 模块划分(Go package 视角)

```
atlas/
└── internal/prism/collector/
    ├── marketutil/     # O4: DetectMarket / NormalizeSymbol + 单测
    ├── fetcher/        # O1: Fetcher 接口 + FetchResult (O3)
    │   ├── lixinger/   # 理杏仁 (D7): base URL 配置化(历史上改版过)
    │   ├── edgar/      # SEC companyfacts: filing date 生效, 防前视(设计文档5.1)
    │   ├── yfinance/   # 价格主源(经由内部 HTTP 或 sidecar, 见下)
    │   └── stooq/      # 价格备源: CSV 直下
    ├── router/         # O2: YAML 路由表 + 降级链
    ├── calendar/       # O5: 交易日历门控
    ├── breaker/        # O6: 熔断 + in-flight 去重
    └── pipeline/       # 调度循环: 水位 → 门控 → 路由 → 落库 → 统计
```

**yfinance 的 Go 适配说明**:yfinance 是 Python 库,Go 侧两个选择——直接调 Yahoo Finance 的 chart HTTP 端点(推荐,消除 Python 依赖,但需自行处理反爬与字段解析),或保留一个最小 Python sidecar 仅做价格拉取。建议先用纯 Go HTTP 方案,Stooq 作兜底已足够覆盖失效风险。

---

## 6. 实施顺序与验收

| 步骤 | 内容 | 验收标准 |
|------|------|----------|
| 1 | marketutil + fetcher 接口 + FetchResult | 单测覆盖三市场识别规则与边界代码 |
| 2 | 表结构补丁(O3 三列)+ 增量水位查询 | 重复执行采集不产生重复请求 |
| 3 | YfFetcher + StooqFetcher + Router 降级 | 人为断掉主源,链路自动降级且日志可审计 |
| 4 | LxrFetcher(A/H 公司 + 指数,含分位) | 茅台/腾讯 10Y 分位与理杏仁网页端一致 |
| 5 | EdgarFetcher | NVDA 季报行数 ≥ 60(2009 至今),partial 公司清单可导出 |
| 6 | 日历门控 + 熔断 + 告警接入 | 节假日跳过;模拟连续失败触发熔断并收到告警 |

完成后,valuation_engine(EPS_TTM / PE / 滚动百分位)在此数据层之上开发,不再关心数据来源细节。

---

## 7. 风险与边界

| 风险 | 对策 |
|------|------|
| Yahoo chart 端点反爬策略变化 | Stooq 备源 + 降级日志监控可用率;必要时再评估 Tiingo($10/月)替换主源 |
| 理杏仁接口路径/字段改版 | base URL 与 metrics 字段名全部配置化;改版属 breaking 变更,靠采集告警发现 |
| EDGAR 自定义 XBRL tag | tag 映射表配置化,partial 清单驱动逐家补映射,人工节奏可控 |
| Go 生态缺交易所日历库 | 年度节假日表落库方案兜底;fail-open 语义保证不漏采 |
| 熔断误伤(上游短暂抖动) | 半开探测 + 冷却期配置化(默认 10 分钟),阈值可按 provider 单独调 |

---

## 8. 附:致谢与许可

模式提炼自 [ZhuLinsen/daily_stock_analysis](https://github.com/ZhuLinsen/daily_stock_analysis)(MIT License)的 `data_provider` 模块与 `docs/market-support.md` 边界文档。本方案仅借鉴设计思想,未复制其代码。
