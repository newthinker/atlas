# QA Code Review — 第二轮（跨视角对抗）

codex/gemini CLI 不可用 → 纯 Claude 三视角独立重审。日期 2026-06-10，qa-agent-1。

## 视角 A：攻击者（状态一致性 / 缓存污染 / 越权）
- **PaperBroker 并发**：所有读写经 `mu`（RWMutex）；`applyFill` 在写锁内完成现金扣减+持仓更新，无 TOCTOU。并发买入不会双花（notional>cash 检查与扣减原子）。✅
- **缓存污染**：命中与回填都 `cloneOHLCV` 返回独立副本（cache.go:59,74），调用方修改返回切片无法影响缓存条目。key 含 symbol|start|end|interval，截断到分钟——不同 symbol 不串槽。✅ 无投毒/越权路径。
- 结论：未发现可被并发信号打破的状态一致性缺陷。

## 视角 B：运维（配置组合 / 默认值 / 可观测性）
- **W3（high-ops）**：`broker.enabled=true` 但配置文件未写 `execution.mode` 时，`config.Load` 不补默认（仅 `config.Defaults()` 补），Validate 又把 `""` 当合法（config.go:372），而运行期 `ExecutionManager.Execute` 对 `""` 返回 `ErrInvalidExecutionMode`（execution.go:185-186）。→ 每次执行都 error（被 SubmitSignal 吞掉，fail-safe），但用户得不到任何下单且校验阶段无提示。建议 Load 补 `execution.mode` 默认或 Execute 把 `""` 视作 auto/confirm。
- **I1**：默认 `execution.mode=confirm`，Execute 仅入队返回 `Success=true`，适配器日志打 "signal executed"（executor.go:75）实则未成交，且无自动 confirm 路径——paper 模式下 pending 单只增不减。日志语义误导。
- 可观测性其余良好：worker 数、cache ttl、wiring 状态均有 Info 日志。

## 视角 C：资金安全（风控绕过 / 重复下单 / Price=0）
- **Price=0 真的堵死？是**：双层防护——Execute 入口 `price<=0` 守卫（execution.go:124）+ PaperBroker `request.Price<=0 → ErrInvalidPrice`（paper.go:84）。无法以 0 价成交。✅
- **W1（high）**：但堵死的副作用是**整条生产链路 inert**。serve.go:142 只注册 ma_crossover，其信号（strategy.go:86,102）**从不设 Price**→`sig.Price=0`→`Execute(...,0)` 必 error→SubmitSignal 吞掉。**真实运行永不下单**。单测/e2e 全靠硬编码 `Price:100`（executor_test.go:171,185,222...）掩盖——典型 fantasy-pass。
- **W2（high）**：`executor.SubmitSignal` 不依赖 `router.Route` 结果（app.go:369-377 两步独立）。router 的 cooldown/dedup **不约束执行**。若 Price 被正确填充，每周期重复的同向信号将每周期下单，仅受 RiskChecker（MaxOpenPositions/MaxPositionPct）兜底——潜在过度交易/重复下单。建议执行前判定 Route 成功，或在执行侧加 dedup/cooldown。

## 三视角共识
- 无 CRITICAL（所有缺陷 fail-safe：不崩溃、不产生错误订单）。
- W1 为真实 high-severity（交付功能在生产中不工作），三视角无分歧认其为真；**唯处置取决于"Signal.Price 填充是否属本 sprint 范围"——需人工/Leader 判断** → 故 verdict=CONTESTED。
