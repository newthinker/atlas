# 需求分析 — Router percentile_step（百分位步进提醒）

> 日期：2026-06-12
> 需求来源：`docs/plans/2026-06-12-percentile-step-implementation.md`（实施计划 rev4.1 final）
> 设计依据：`docs/plans/2026-06-12-percentile-step-design.md`（rev4，状态：设计已确认/用户批准）
> ECC 降级说明：ECC 不可用（capabilities.ecc=false）。常规降级路径为 superpowers brainstorming，
> 但本需求的设计已经过 4 轮评审并获用户批准（git 历史 0d224a3→4c1555a），再行头脑风暴无增量价值，
> 直接沉淀既有批准设计为本阶段产出。

## 核心功能列表

| # | 功能 | 来源 |
|---|---|---|
| F1 | Router 百分位步进门控：`|当前分位−上次通知分位| ≥ step` 才放行，单规则覆盖买跌/卖涨/恢复重算三场景 | 设计 §3 |
| F2 | 门控状态 key = `symbol|strategy|side`（strong 档与普通档同侧共享） | 设计 §2 |
| F3 | 步进门控对分位信号**完全替代**时间冷却（不查、不更新冷却戳） | 设计 §1 |
| F4 | 策略级步长：`strategies.{name}.params.percentile_step` 经 `Signal.Metadata` 传递，优先于全局 `router.percentile_step` | 设计 §2 rev4 |
| F5 | RouteBatch 复用同一判定与状态更新，防旁路；补 nil-registry 守卫 | 设计 §4 / 计划 Task 2 |
| F6 | ClearCooldown/ClearAllCooldowns 同步清理步进状态；GetStats 回显门控状态 | 设计 §4 |
| F7 | 修复 cfg.Router 死配置预存 bug：app.New() 从硬编码改为 cfg 映射接线 | 设计 §2 装配点 |
| F8 | 配置文件交付更新（percentile-watchlist.yaml / config.example.yaml） | 设计 §7 |

## 非功能性需求

- **并发安全**：步进判定与状态写入在同一 `r.mu.Lock()` 临界区内完成（无 check-then-act 竞态）。
- **向后兼容**：两级 step 均未配置/≤0 时所有信号走原冷却路径，既有用例零回归。
- **覆盖率**：变更 package 覆盖率 ≥ 80%（config coverage.dev_minimum）。
- **存量行为变更（有意）**：接线修复后未显式配置的部署冷却 1h→4h、置信阈值 0.5→0.6，提交信息须注明。

## 模糊/缺失需求点

无。计划文档已给出全部测试代码骨架、实现代码骨架与文件行号定位；边界（坏元数据、step=0、类型异常回退）均有明确行为定义。

## 模块划分与依赖

```
internal/router        （F1-F3,F5,F6 核心门控）
internal/strategy/price_percentile  （F4 步长参数携带）
internal/strategy/pe_percentile     （F4 步长参数携带）
internal/config        （F7 配置字段+校验）
internal/app           （F7 接线，依赖 router 字段 + config 字段）
configs/*.yaml         （F8 交付配置，依赖全部）
```

策略包不依赖 router 改动（仅写 Metadata），可与 router 并行开发。
