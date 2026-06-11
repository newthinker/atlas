# 架构决策记录 — sprint-002

> 产品/技术架构决策已在 design rev6 与 plan rev3 定稿（扩展既有采集器不新建、valuation 纯函数包、
> 双策略独立不抽基类、窄接口注入估值源、兜底链语义等），此处不复述。本文件仅记录 **arcforge 任务图重组决策**。

## ADR-S2-1: 以 plan 原文为施工图，DoD 引用 plan Task 而非复写

plan rev3 每个 Task 已含失败测试代码与实现骨架（经 3 轮评审）。arcforge DoD 提炼为可验收断言并标注「以 plan Task N 为准」，避免转写引入偏差。Dev prompt 强制要求先读 plan 对应章节。

## ADR-S2-2: 15 个 plan Task 重组为 12 个 arcforge 任务

重组依据是 package 互斥而非 plan 章节边界：
- 合并 T2+T3（同 yahoo 包）、T4 前半+T5（同 collector 根包）、T6+T12（同 app 包）、T14+T15 部分（同 cmd 范围）
- 拆分 T4（indexes.go 在 collector 根包、parseSymbol 在 eastmoney 包，跨包必拆）
- app 包两任务 010→011 依赖串行（scope 互斥）

## ADR-S2-3: T15 收尾项分流

- 理杏仁指数代码核对：依赖 LIXINGER_API_KEY，无 key 则跳过并在 discovery 注明（plan 原文允许）→ 并入 TASK-012
- code-simplifier：Dev 工作循环 commit 前已强制 → 不单设任务
- gitnexus_detect_changes + 全量回归：QA 阶段承接
- 最终集成 commit：Leader 终验收阶段确认（各任务已独立 commit，是否再做聚合 commit 待交付时定）

## ADR-S2-4: 覆盖率基线沿用 sprint-001 机制

任务级 coverage_minimum：core 纯类型 78（现状踩线）、cmd/atlas 35（sprint-001 裁决先例）、其余默认 80。
