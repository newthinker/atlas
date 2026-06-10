# QA 终审 Verdict（Round 3 · 修复复审）

- 范围：W1/W2/W3 修复复审 + 全量回归
- 修复 commit：W1 `784ed71`、W2 `16d52a8`、W3 `cc2f0ff`
- 审查人：qa-agent-1　日期：2026-06-10

## VERDICT: **PASS**

三个 WARNING 全部真实解决，反例场景被针对性测试覆盖（非 fantasy），router 接口改动无连带破坏，全量回归 + -race 全绿。I1/I2 维持记录、不阻塞。

## 客观证据（实跑）
| 检查 | 结果 |
|---|---|
| `go build ./...` | rc=0 ✅ |
| `go vet ./...` | rc=0 ✅ |
| `go test ./...` | 全包 ok（无 FAIL）✅ |
| `go test -race ./internal/app/... ./internal/router/... ./internal/broker/... ./internal/strategy/...` | rc=0 ✅ |

## 逐项复核

### W1 → 已解决（commit 784ed71）
- 修复：`strategy.go` 取 `lastClose := prices[len(prices)-1]`（prices[i]=bar.Close），金叉/死叉两条信号均 `Price: lastClose`。
- 反例闭合：真实 serve 路径（仅注册 ma_crossover）信号现携带价格 → ExecutionManager 不再以 "price must be positive" 拒单 → 能成交。
- 测试真实性：
  - `ma_crossover/strategy_test.go:79` 断言 `signals[0].Price == lastClose`。
  - `executor_test.go:241+` 改用**真实 ma_crossover 生成的信号**驱动 BUY e2e（替换硬编码 Price:100），断言 `sig.Price>0` 且成交后现金下降 + 建仓。
  - `executor_test.go:276 TestSignalExecutor_UnpricedSignalNotTraded`：Price=0 信号断言**不下单**（现金不变 + `ErrPositionNotFound`），显式锁死 W1 失败模式，防回归。
- 结论：fantasy-pass 已消除，正反两路径均覆盖。✅

### W2 → 已解决（commit 16d52a8）
- 修复：`router.Route` 改签名 `(routed bool, err error)`，被 filter（cooldown/confidence/action）抑制时返回 `false`；`app.go:382` 仅在 `routed && executor != nil` 时 SubmitSignal。
- 反例闭合：cooldown 抑制的重复信号不再下单。
- 连带影响核查：`.Route(` 唯一生产调用方为 `app.go:369`（已更新）；RouteBatch 为独立方法未受影响；build/vet/test/race 全绿佐证无破坏。
- 测试真实性：
  - `app_test.go:438 TestApp_Executor_CooldownSuppressedNotSubmitted`：同符号两信号，断言 `exec.count()==1`（第二条被 cooldown 抑制未提交）+ notifier 仅收 1。
  - `router_test.go:201/221`：Route 在 cooldown/confidence 抑制时报告 `routed=false`。
  - 正向：`app_test.go:346 TestApp_Executor_SubmitsRoutedSignals`。
- 结论：执行受 router 抑制结果约束。✅

### W3 → 已解决（commit cc2f0ff）
- 修复：`config.Load` 增 `v.SetDefault("broker.execution.mode","confirm")`，与 Validate/Execute 对齐。
- 反例闭合：broker 启用但漏写 execution.mode → 现默认 "confirm"，Execute 不再返回 ErrInvalidExecutionMode。
- 测试真实性：`config_test.go:314 DefaultsToConfirm` + `:337 ExplicitPreserved` 两场景。
- 结论：✅（边角：用户显式写空串 "" 仍会被运行期拒——pathological，可忽略）

## 非阻塞记录（INFO，carryover）
- **I1**（维持）confirm 模式仅入队却日志 "signal executed"；无自动 confirm。
- **I2**（维持）`paper.go:188` CancelOrder 非终态分支不可达死代码。
- **I3**（新增·latent）`app.go` arbitrate 合成的 `meta_arbitrator` 信号未设 `Price` → 若启用 arbitration 且 ≥2 策略产生冲突信号，其决策 Price=0 将被执行拒（fail-safe，不产生错误订单，但与 W1 同类 inert）。当前 serve 仅注册 ma_crossover、单策略下 arbitration 不触发，故不可达、不阻塞；多策略 + arbitration 场景前应补价。
- **I4**（新增·trivial）`router.Route` 现恒返回 nil err，`app.go:370` 的 `if err != nil` 为死分支，无害。

## 结论
W1/W2/W3 全部 PASS，sprint 质量门禁通过。建议 Leader 将相关任务推进至 `accepted`；I3 建议登记 issues.md 留作下一 sprint。
