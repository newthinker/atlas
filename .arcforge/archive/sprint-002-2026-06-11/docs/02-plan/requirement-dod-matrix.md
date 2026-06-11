# 需求 ↔ DoD 追溯矩阵 — sprint-002

**生成**: 2026-06-11 | **需求源**: plan rev3（15 Task）→ 12 arcforge 任务

## 正向：需求 → 任务

| 需求 | plan Task | arcforge 任务 | 覆盖要点 |
|------|-----------|---------------|----------|
| R1 core 类型 | T1 | TASK-001 | 三类型存在 + 零回归 + 78 基线 |
| R2 yahoo 指数/期货 + EPS | T2,T3 | TASK-002 | 符号正则/URL 编码/EPS 解析/双端点注入/^ 拒绝 |
| R3 指数表+selector+eastmoney | T4,T5 | TASK-003, TASK-004 | 六指数表/路由/市场归属/secid 区分/零回归 |
| R4 lixinger 估值分位 | T7 | TASK-005 | endpointFor 七用例/cvpos×100/粒度映射/业务错误 |
| R5 valuation 纯函数 | T8 | TASK-006 | strictly-less/阶梯对齐/双哨兵错误/空序列防伪装 |
| R6 双策略 | T9,T10 | TASK-007, TASK-008 | 分档/置信度/门槛/AssetTypes/Source 解析/Price 非 0 |
| R7 既有策略声明 | T11 | TASK-009 | 三包 AssetTypes 断言 + 零回归 |
| R8 app 装配 | T6,T12,T13 | TASK-010, TASK-011 | 类型识别/绑定过滤/动态窗口/兜底链六路径/亏损不兜底 |
| R9 cmd+配置 | T14,T15 | TASK-012 | 注册/typed-nil 防护/配置示例/冒烟/README/指数代码核对 |

## 反向：任务 → 需求

12 任务全部回溯到 R1-R9，无凭空 DoD。plan T15 的 code-simplifier（Dev 流程内）与 gitnexus 回归（QA 阶段）由流程承接，不设任务。

## 验收对照（plan 文末清单 → 承接点）

| plan 验收项 | 承接 |
|-------------|------|
| ^GSPC/GC=F 入 watchlist 产生 price_percentile 信号 | TASK-012 冒烟 + TASK-007 单元 |
| 000300.SH 走 eastmoney secid + cn/index 估值 | TASK-004 + TASK-005 |
| 美股 pe_percentile 三态（重建/兜底/双失败） | TASK-011 六路径表驱动 |
| 亏损股不出 PE 信号且不触发兜底（stub 计数） | TASK-011 硬断言 |
| GC=F 绑定 pe_percentile → warning + 跳过不崩溃 | TASK-010 effectiveStrategies |
| 全量 go test 零回归 | TASK-012 non_functional + QA |

## 机器检查结论

- 孤儿需求: 无。凭空 DoD: 无。
- verify_by 分布: test 35 条 / review 1 条（README 行更新）/ manual 0。
- 注: 需求源为 3 轮评审定稿的实现计划，DoD 锚定 plan 原文（ADR-S2-1），转写偏差风险低；reviewer 反审重点放在「重组遗漏」而非标准充分性。
- **独立 reviewer 反审（2026-06-11）**: 初判 NEEDS_REVISION（轻量）。已采纳：①TASK-012 冒烟增加 ^GSPC 符号（承接 plan 验收对照第 1 条指数端到端）；②TASK-012 补 go vet 全量（plan T15 Step 3）；③TASK-002 description 补 UA/Accept 头提示（真实端点 403 风险）。code-simplifier 由 Dev 工作循环每任务 commit 前承接、gitnexus_detect_changes 由 QA 阶段显式承接（写入 QA prompt）。陷阱提示核对 16 项全部到位、数值零失真、依赖图无错误。修订后判定 PASS。
