# 架构决策记录 — atlas watchlist 指标命令（Sprint 018）

| # | 决策 | 结论 | 理由 |
|---|---|---|---|
| AD-1 | 指标组装位置 | App 导出只读 `SnapshotMetrics`，复用 orderedCollectors/buildFundamental | 与分析循环口径同源，避免第二套估值逻辑 |
| AD-2 | 估值三项来源 | 新增窄接口 `FundamentalSource`，lixinger 注入（仅 A 股） | app 不依赖具体采集器包（镜像 ValuationSource 模式） |
| AD-3 | serve 装配复用 | 提取 `buildCollectors` 供 serve 与新命令共用 | 原样迁移零行为变化；cleanup 托管 qlib 句柄 |
| AD-4 | CJK 对齐 | telegram 宽度函数迁移 `internal/text` 共享包 | 函数体逐字保留，防回归 |
| AD-5 | JSON 形态 | 数组 + gaps 内嵌每标的对象 | 计划已消解 spec 歧义，信息等价 |
| AD-6 | 交付组织 | 单 PR、4 Task、3 wave（T1∥T2 → T3 → T4） | 依赖链清晰，包 scope 互斥 |
| AD-7 | 降级语义 | 单指标缺失→nil+gap；单标的 panic→隔离；全失败→非零退出 | 离线命令可用性优先，与采集器降级哲学一致 |

## AD-8 独立 reviewer 反审裁决（2026-07-03）

| 反审发现 | 裁决 |
|---|---|
| C1/B1 JSON gaps 位置偏离 spec（顶层数组 vs 内嵌） | **接受偏离**（AD-5 既定，信息等价）；backlog：回写 spec 消歧 |
| C2/B2 "config 缺失→exit 1" 半满足 | **接受 plan 语义**（与既有 export 命令一致）：--config 指向坏文件→非零退出；无 --config→默认配置继续。TASK-004 补 DoD 钉死 |
| C3 价格百分位窗口"与策略不同源"（100年 vs 10年+90d） | ~~误报，维持 plan~~ **AD-8a 重开修正（2026-07-03 QA W3）**：原裁决前提错误——SinceInceptionBars 仅在 lookback_years==0 时使用，pe_percentile/price_percentile 策略默认用 lookback×252（5y/3y，QA 核验 strategy.go）。分叉真实存在但影响有界（A 股 lixinger cvpos 不用价格窗口；默认配置 PE 窗口 5y 巧合一致；仅美/港个股非默认配置下数值不同但仍有效）。处置：QA 方案 (b)——docstring 软化 + 注明 snapshot 窗口基准为 valuation.lookback_years（TASK-004 review_fix）；窗口对齐 historyWindowDays 留 backlog |
| B3 并发等价未测 | 采纳：TASK-002 补 Workers=1 vs 4 结果逐元素相等用例 + -race |
| B4 结果保序未直接断言 | 采纳：TASK-002 补 ≥3 标的保序断言 |
| B5 价格百分位极值边界含糊 | 采纳：TASK-002 补边界 DoD（严格小于语义：current>全部收盘→100、==最小收盘→0、内部值用例） |
| B9 空 watchlist exit 0 | 接受 plan 决定（提示 + exit 0） |
| B10 Task 3 零变化锚点 | 采纳：Defaults 用例断言注册 collector 名称集合（可机检等价锚点） |
| B7/B8 stdout 纯净与退出码为构造保证 | 接受（框架标准）；验证阶段以手工冒烟兜底，验证结论措辞不夸大 |

## 环境降级（沿用 Sprint 017 AD-9）

validator / arcforge-write.sh 缺失：Leader 手工校验任务图；task JSON 写入走 with-task-lock.sh 临界区 + epoch 认领协议。
