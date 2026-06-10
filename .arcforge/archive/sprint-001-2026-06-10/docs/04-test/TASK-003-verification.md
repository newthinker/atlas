# TASK-003 复验报告（review_fix / W1[high] 修复）— serve.go paper 模式 M4 执行链

- **Verifier**: test-agent-1 (Reality Checker)
- **验证时间**: 2026-06-10（review_fix 复验，commit 784ed71；rework=1, epoch=3）
- **判定**: ✅ **VERIFIED**（QA 最高严重度 W1 已真实修复）
- **被验包**: `./cmd/atlas` + `./internal/broker/...` + `./internal/strategy/ma_crossover`
- **任务 coverage_minimum=45**（Leader 机制化裁决，hook 门禁已于 dev_done 通过）

## W1 修复复验（对照 qa-verdict.md / qa-review-round2.md W1）
原 W1[high]：ma_crossover(strategy.go:86,102) 生成 Signal **不设 Price** → executor 收 Price=0 →
`ExecutionManager.Execute(...,0)` 必拒（"price must be positive"）→ SubmitSignal 吞掉 →
**真实 serve 永不下单**；单测/e2e 全靠硬编码 `Price:100` 掩盖（fantasy-pass）。

| 复验要点 | 证据 | 判定 |
|----------|------|------|
| 1. 信号携带真实 Price(bar.Close) | strategy.go:84 `lastClose := prices[len-1]`；BUY(93)/SELL(110) 均 `Price: lastClose`。strategy_test.go 新增断言 `signals[0].Price == lastClose` | ✅ |
| 2. 反 fantasy 显式断言 | `TestSignalExecutor_UnpricedSignalNotTraded`：提交 Price=0 BUY → 无 error(fail-safe) + cash **不变** + `GetPosition` 返回 `ErrPositionNotFound`（不下单/无持仓）；executor.go:60 `s.log.Warn("signal execution failed")` 记 warning | ✅ |
| 3. ≥1 条端到端走真实策略信号路径 | `TestSignalExecutor_EndToEnd_BuyChangesBalanceAndPosition`：`goldenCrossSignal` 跑**真实** `ma_crossover.New(2,4).Analyze(...)` 产出 BUY 信号（非硬编码 Price:100），经**真实链** buildChain(paper.New+RiskChecker+PositionTracker+ExecutionManager) 提交；断言 `sig.Price>0` + cash↓ + position.Quantity>0 | ✅ |
| 4. 三包 -race 全通过 | `go test ./cmd/atlas/ ./internal/broker/... ./internal/strategy/ma_crossover/ -race` → 全 ok（cmd/atlas, broker, broker/mock, broker/mocks, broker/paper, ma_crossover） | ✅ |

## 集成缺陷修复连带验证
- W1 真实可下单还依赖 `ExecutionManager.Execute` 把 price 写入 OrderRequest（否则市价单 Price=0 仍被 PaperBroker 拒）。
  核实 execution.go:167/220 `Price: price` 已落地。e2e BUY 真实链下 cash↓+建仓成功 ⟹ 该修复端到端被验证。

## 反 fantasy 核验（Reality Checker 重点）
- e2e 不再用 `core.Signal{...Price:100}` 硬编码（git 784ed71 显示该行被 `goldenCrossSignal(t,"AAPL")` 替换）。
- `goldenCrossSignal` 驱动**生产策略代码**（ma_crossover.Analyze），并 `t.Fatal` 守卫「必产 BUY 信号」，
  e2e 再 `t.Fatal` 守卫「sig.Price>0」——双重保证走的是真实定价信号路径。
- buildChain 全为真实 broker 组件（无 stub），状态变更（余额/持仓）为真实成交结果。

## Done Criteria 回归（8 条，复验后仍全 PASS；W1 影响 functional[2] 的真实性已修复）
| # | 标准 | 测试 | 判定 |
|---|------|------|------|
| functional[0] | 构造 ExecutionManager 并双注入(SetExecutor+Deps) | TestWireExecution_PaperMode_InjectsBoth / TestBuildExecution_PaperMode_Constructs | PASS |
| functional[1] | BUY/SELL→方向 OrderRequest 按 SizePct×余额定量提交 | EndToEnd_Buy（真实链定量成交） | PASS |
| functional[2] | e2e BUY 改余额/持仓；风险拒绝余额持仓不变 | EndToEnd_BuyChangesBalanceAndPosition（真实信号） + EndToEnd_RiskRejectionNoChange | PASS |
| functional[3] | futu 非 paper 保持 warning，进程正常启动 | TestBuildExecution_FutuNonPaper_NilNoError | PASS |
| boundary[0] | Enabled=false 不构造，Deps.ExecutionManager=nil | TestBuildExecution_Disabled_Nil + TestWireExecution_Disabled_NoInject | PASS |
| boundary[1] | 非 BUY/SELL(HOLD) 跳过不生成订单 | TestSignalExecutor_HoldSignalSkipped | PASS |
| boundary[2] | 余额/数量为 0 跳过下单不报错 | TestSignalExecutor_ZeroQuantitySkipped(+_Unit) | PASS |
| error_handling[0] | 提交错误记日志返回 nil，不打断分析循环 | TestSignalExecutor_ExecuteErrorReturnsNil | PASS |

## 覆盖率
- 改动函数（executor.go）：newSignalExecutor/SubmitSignal/isExecutableAction **100%**，
  buildExecution 92.3%，wireExecution 85.7%；ma_crossover Analyze 91.3%（定价分支被覆盖）。
- 任务 coverage_minimum=45（Leader 裁决；cmd/atlas+broker 合并 50.8%），hook 门禁 dev_done 已过。
  本任务 DoD 无 non_functional 覆盖项，验收以 DoD 矩阵 + 真实 e2e 为准。

## 结论
QA W1[high]「生产链路惰性」已被真实修复：策略层填充 Price(lastClose)、execution.go 传递 price、
新增 Price=0 失败模式守卫测试、e2e 改走真实 ma_crossover 信号 + 真实 broker 链。四项复验要点全部满足，
三包 -race 全绿，8/8 DoD 回归通过，反 fantasy 核验确认非占位。**VERIFIED。**
