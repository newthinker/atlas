# QA 终审 Verdict

- 范围：master `76a54b5..HEAD`（13 commits / 10 任务）
- 审查人：qa-agent-1　日期：2026-06-10

## VERDICT: **CONTESTED**

无 CRITICAL；存在 3 个 high/中 severity WARNING。其中 W1 为 high-severity 且**处置依赖范围判断（需人工/Leader 定夺）**，因此按规则判为 CONTESTED 而非 REJECT。所有缺陷均 fail-safe（不崩溃、不产生错误订单）。

## 客观证据
build ✅ / vet ✅ / `go test ./...` 全绿 ✅ / `-race`（app+broker+collector）✅ / gofmt 干净 ✅

## 问题清单

### WARNING
**W1 [high] 生产执行链路 inert（交付功能在真实运行中不下单）**
- 文件：`internal/strategy/ma_crossover/strategy.go:86,102`（不设 `Price`）；`cmd/atlas/serve.go:142`（仅注册 ma_crossover）；`cmd/atlas/executor.go:58`（`Execute(...,sig.Price)`）；`internal/broker/execution.go:124`。
- 描述：ma_crossover 生成的 Signal 不含 Price → `sig.Price=0` → `Execute` 返回 "price must be positive" → 被 `SubmitSignal` 吞掉。**真实 serve 运行下永不下单**。单测/e2e 全部硬编码 `Price:100`（`cmd/atlas/executor_test.go:171,185,222,248,269`）掩盖该事实——fantasy-pass。
- 建议修复：在策略层用最近收盘价填充 `Signal.Price`（strategy.go 已有 `bar.Close`），或在适配器 SubmitSignal 中先从 collector/quote 取现价再 Execute；并补一条「Price 未填→不下单」的显式断言测试。
- 需 Leader/人工裁断：填充 Signal.Price 是否属本 sprint 范围。

**W2 [中] 执行不受 router cooldown/dedup 约束 → 重复信号重复下单（潜在）**
- 文件：`internal/app/app.go:369-377`。
- 描述：`router.Route` 与 `executor.SubmitSignal` 为两步独立调用，Route 因 cooldown 抑制信号时 SubmitSignal 仍执行。当前因 W1 暂时不显现；W1 修复后每周期同向信号将每周期下单，仅靠 RiskChecker 兜底。
- 建议：执行前判定 Route 成功（或返回"已路由"标志），或在执行侧引入 dedup/cooldown。

**W3 [中] execution.mode="" 校验与运行不一致**
- 文件：`internal/config/config.go:372`（Validate 接受 `""`）vs `internal/broker/execution.go:185-186`（Execute 对 `""` 返回 ErrInvalidExecutionMode）；`config.Load` 不为 execution.mode 补默认。
- 描述：broker 启用但配置文件漏写 execution.mode 时，校验通过但每次执行 error（被吞，fail-safe），用户无下单且无提示。
- 建议：`Load` 补默认（如 "confirm"），或 Execute 把 `""` 视作默认模式。

### INFO
- **I1** `cmd/atlas/executor.go:75`：confirm/batch 模式仅入队却日志 "signal executed"，语义误导；paper 模式无自动 confirm，pending 单只增不减。
- **I2** `internal/broker/paper/paper.go:188-191`：CancelOrder 非终态分支 `return nil` 不删除订单；因 paper 单恒为 filled/terminal 而不可达，建议删死分支或注明。

## 已验证修复（PASS 项，附证据）
- **ISSUE-1（StatusCode）彻底解决**：eastmoney(4)+lixinger(6)+yahoo(2) 共 10 条 fetch 路径全部 `StatusCode!=200→error` 且 `defer Body.Close()` 位置正确（无非 200 泄漏）。
- **ISSUE-3（Execute Price 缺陷）已修**：`execution.go:167` 市价单携带 Price，paper BUY 不再永拒（`-race`/单测佐证）。
- **并发安全**：worker pool 有界、panic 隔离、共享态全锁、`-race` 全绿。
- **缓存正确**：clone-on-read/write 防污染、TTL+LRU 淘汰、错误不缓存。

## 给 Leader 的处置建议
- W1 需人工决定范围：若属本 sprint → 相关任务（TASK-002/003 或新增 strategy 任务）置 `review_fix` + fix_items={填充 Signal.Price + 反 fantasy 断言}；若判定越界 → 记 issues.md 留作下一 sprint，本 sprint 可放行。
- W2/W3 建议一并 `review_fix`（小改动，fail-safe 但属真实缺陷）。
- I1/I2 可选修。
