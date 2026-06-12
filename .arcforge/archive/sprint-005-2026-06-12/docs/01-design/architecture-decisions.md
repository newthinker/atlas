# 架构决策记录 — percentile_step Sprint

## ADR-1 门控落点：Router 内置（设计方案一）
策略保持无状态；通知去重职责（cooldown 与步进门控）同居 router。Rationale：与现有 cooldown 同构，状态生命周期一致（内存态、重启清零）。

## ADR-2 步长粒度：策略级经 Metadata 传递（rev4，用户确认）
`strategies.{name}.params.percentile_step` → 策略写入 `Signal.Metadata["percentile_step"]` → router `effectiveStep` 优先采用，回退全局。Rationale：watchlist 资产经绑定策略获得不同步长（价格分位 5、PE 分位 3），不引入 router 对策略注册表的反向依赖。

## ADR-3 与时间冷却完全替代（非叠加）
分位信号不查、不更新冷却戳。Rationale：避免分位通知压制同标的其它策略（如 ma_crossover）；范围边界明确排除"最小冷却叠加"。

## ADR-4 顺带修复 cfg.Router 死配置预存 bug
app.New() 硬编码（1h/0.5）改为 cfg 映射。Rationale：不修则 §7 的 cooldown_hours 配置同样落空。存量行为变更（1h→4h、0.5→0.6）在提交信息注明。

## ADR-5 两策略包独立实现，不抽公共基类
与既有结构一致；重复成本（一个字段 + 两行）远低于抽象成本。

## ADR-6 任务拆分偏离原计划处（Leader 决定）
原计划 5 Task → 7 任务：Task 3 按策略包拆为 TASK-003/004（Realistic Scope ≤1 package；两包可并行）；Task 4 按 config/app 包拆为 TASK-005/006（app 接线依赖 config 字段与 router GetStats，拆开后依赖显式化）。原计划的两段式 RED（先编译错误后断言失败）语义保留在 TASK-006 内。

## ADR-7 流程降级记录
- ECC 不可用 → 设计已获用户批准（4 轮评审），跳过重复 brainstorming，直接沉淀产出。
- Go validator（validator/cmd/arcforge-validate）在本仓库与插件目录均不存在 → 降级为 Leader 手工逐条核对校验规则（DAG 无环、wave 序、scope 互斥非空、context_from 闭合等），核对结果记入 02-plan。
- `.claude/hooks/arcforge-write.sh` 不存在 → teammate 状态写入降级为 `with-task-lock.sh` 临界区 + 原子写纪律（spawn prompt 中相应调整边界声明）。
