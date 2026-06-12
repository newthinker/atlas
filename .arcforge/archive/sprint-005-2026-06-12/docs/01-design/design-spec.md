# 设计规格 — Router percentile_step

> 本文为指针文档：权威设计规格见 `docs/plans/2026-06-12-percentile-step-design.md`（rev4，用户已批准），
> 实施细节（含测试/实现代码骨架、文件行号）见 `docs/plans/2026-06-12-percentile-step-implementation.md`（rev4.1 final）。
> Dev/Test/QA 一律以上述两份文档为准，本文仅摘录关键决定防漂移。

## 关键设计决定摘录

1. **判定规则（设计 §3）**：首次放行并记录；`|当前分位−上次分位| ≥ step` 放行并更新；否则抑制。无独立"重置"分支（防死锁靠对称距离规则天然覆盖）。
2. **分流（设计 §4）**：`passesFilters` 拆为 `passesStaticFilters`（confidence+action）+ `passesCooldown`。分位信号（带分位元数据且有效步长>0）走步进门控，**不查不更新冷却戳**；其余走原冷却路径，冷却戳更新移入冷却分支内。
3. **有效步长（rev4）**：`Metadata["percentile_step"]`（float64 且 >0）→ 全局 `router.percentile_step` → 均无效走冷却。"全局 0 + 策略 step>0" 是合法的按策略启用场景。
4. **分位提取**：依序 `Metadata["percentile"]` / `Metadata["pe_percentile"]`，仅接受 float64（信号全程内存传递，无反序列化边界，注释注明假设）。
5. **原子性**：判定+写入单 `r.mu.Lock()` 临界区。
6. **状态管理**：`ClearCooldown(symbol)` 按 `symbol+"|"` 前缀删步进 key（假设 symbol 不含 `|`）；`ClearAllCooldowns` 重建两 map；`GetStats` 增加 `percentile_gates_active` 和 `percentile_step`（仅回显全局回退值）。
7. **接线修复（设计 §2 装配点）**：app.New() 的 routerCfg 从 cfg.Router 映射（CooldownHours→Duration、MinConfidence、PercentileStep）；EnabledActions 维持硬编码（YAGNI）；`cooldown_hours: 0` = 禁用冷却（`time.Since(last) < 0` 恒 false，天然满足，注释注明无需特判）。
8. **配置校验**：`PercentileStep < 0` 拒绝（core.WrapError(core.ErrConfigInvalid, ...) 风格）；默认值块不加该字段（零值 0 = 禁用）。
