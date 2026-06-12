# Changelog — percentile_step Sprint（2026-06-12）

## Added
- **router 百分位步进门控**：带分位元数据（`percentile`/`pe_percentile`）且有效步长 >0 的信号改按 `|当前分位−上次通知分位| ≥ step` 放行，完全替代时间冷却（不查、不更新冷却戳）；单规则天然覆盖买跌/卖涨/行情恢复重算三场景（055d062）。
- **策略级步长**：`strategies.{name}.params.percentile_step` 经 `Signal.Metadata["percentile_step"]` 传递，优先于全局 `router.percentile_step`（fa9ee68 / 10984ba）；两个分位策略可配不同步长（如价格分位 5、PE 分位 3）。
- **config**：`router.percentile_step`（float64，0=禁用，负值校验拒绝）（55668d2）。
- **router 状态管理**：`GetStats` 新增 `percentile_gates_active`/`percentile_step`；`ClearCooldown(symbol)`/`ClearAllCooldowns` 同步清理步进状态；RouteBatch 同一门控判定并补 nil-registry 守卫（7dd171a）。

## Fixed
- **cfg.Router 死配置预存 bug**：`app.New()` 此前硬编码 cooldown 1h / min_confidence 0.5，`cooldown_hours`/`min_confidence` 配置从未生效；现从 cfg.Router 映射接线，并含实证回归测试（eb9c12b）。

## Changed
- ⚠️ **存量行为变更**：未显式配置 router 节的部署，冷却 1h→4h、置信阈值 0.5→0.6（config 默认值开始生效）。
- `configs/percentile-watchlist.yaml`：启用 `router.percentile_step: 5`（全局回退）；两个分位策略 params 加 `percentile_step: 5`；`cooldown_hours` 由过渡值 24 回归 4（仅约束不带分位元数据的策略，如 ma_crossover）；删除过渡行为说明段落（fa60303）。
- `configs/config.example.yaml`：router 节补 `min_confidence`/`cooldown_hours`/`percentile_step` 及注释；分位策略补 `percentile_step` 示例（fa60303）。
- `internal/router/router.go`：code-simplifier 精简重构（分流逻辑收敛为 `passesDispatchGate`，行为零变化，全量测试回归通过）（fa60303）。
