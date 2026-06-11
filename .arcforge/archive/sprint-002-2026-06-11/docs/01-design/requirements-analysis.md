# 需求分析 — 指数/商品采集 + 历史百分位策略 Sprint（sprint-002）

**分析日期**: 2026-06-11
**需求源**: docs/plans/2026-06-11-index-commodity-percentile-implementation.md（**rev3 终版实现计划**，已 3 轮评审）
**设计依据**: docs/plans/2026-06-11-index-commodity-percentile-design.md（rev6 终版）
**规划模式**: 需求文档本身已是定稿实现计划（含 TDD 步骤/测试代码/实现骨架）——本阶段不做设计探索，工作是将 15 个 plan Task 重组为满足 arcforge Realistic Scope 与 package 互斥约束的任务图。

## 目标（plan Goal 原文）

使 atlas 能监控国际指数（^GSPC/^IXIC/^DJI/^HSI）、A 股指数、国际商品期货（GC=F 等），并对全部资产提供价格历史百分位策略、对股票+指数提供 PE 估值百分位策略（理杏仁多市场兜底）。

## 需求清单（按 plan Chunk/Task 归纳）

| ID | 需求 | plan Task | 复杂度 |
|----|------|-----------|--------|
| R1 | core 类型扩展（AssetCrypto/EPSPoint/PEPercentile） | T1 | 简单 |
| R2 | yahoo 支持指数 ^ 与期货 =F 符号 + URL 编码 + FetchEPSHistory | T2,T3 | 中等 |
| R3 | A 股指数 secid 表（collector 共享）+ eastmoney 指数解析 + selector 路由/市场归属 | T4,T5 | 中等 |
| R4 | lixinger 多市场估值分位（cn/hk/us × company/index） | T7 | 中等 |
| R5 | internal/valuation 纯函数包（PercentileRank + PE 序列重建） | T8 | 中等 |
| R6 | price_percentile 策略（全资产）+ pe_percentile 策略（股票+指数） | T9,T10 | 中等 |
| R7 | 既有策略补 AssetTypes 声明 | T11 | 简单 |
| R8 | app 装配：DetectType/assetTypeOf + AssetTypes 绑定校验 + 动态历史窗口 + 估值编排兜底链 | T6,T12,T13 | 复杂 |
| R9 | cmd 装配 + 配置 + 回测冒烟 + README | T14,T15 | 简单 |

## 关键约束（plan 执行纪律）

- 严格 TDD（plan 每个 Task 已给出失败测试 → 实现的完整步骤与代码骨架，**Dev 必须以 plan 对应 Task 文本为施工图**）
- 所有新增 HTTP 调用可注入 baseURL（httptest 模式，沿用 sprint-001 的 lixinger_httptest 风格）
- 信号链路（router/notifier/backtest）零改动
- 两个百分位策略**不抽公共基类**（plan T10 明示）
- typed-nil 接口陷阱（plan T14 明示 serve.go 注入写法）

## 现状事实

- internal/core 覆盖率 80.0%（踩线）；新增纯类型无语句，安全但留余量
- yahoo/lixinger/eastmoney 已具备 NewWithBaseURL 注入（sprint-001 产物），plan 直接复用
- HEAD = 80c43df（sprint-001 全部 16 commits 在 master 本地）

## 范围边界（plan 原文）

- ❌ 金融股（银行/券商/保险）理杏仁 non_financial 端点失败 → 按不可用降级（一期边界）
- ❌ 回测引擎不接 pe_percentile（依赖在线估值数据）
- ❌ 表外 ^ 指数仅 warning + 默认 US，不扩表
- ⚠️ 理杏仁 us/hk 指数代码（SPX/COMP/DJI/HSI）为候选值，**实现首日核对项**（plan T7/T15，需 LIXINGER_API_KEY；无 key 跳过并注明）

## 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| Yahoo fundamentals-timeseries 非官方接口 schema 漂移 | 中 | plan §5 降级设计；httptest 固化 fixture |
| 理杏仁成功码/metrics 键名约定与真实 API 不符 | 中 | plan 标注实现首日核对项；无 key 时按既有代码约定实现 |
| app 包三个 plan Task 串行（同 package） | 低 | 任务依赖链 010→011 强制串行 |
| core 包 80.0% 覆盖率踩线 | 低 | 任务级 coverage_minimum=78 |
