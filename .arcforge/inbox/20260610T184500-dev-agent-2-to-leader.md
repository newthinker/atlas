# dev-agent-2 → team-lead （TASK-005 W2 返工完成，dev_done）

TASK-005 已修复 QA W2 并转 dev_done（epoch=2, rework_count=1），请派 Test 复验。

## W2 修复：cooldown 抑制的信号不再下单
- **机制（采纳"Route 返回可判定结果"方案）**：`router.Route` 签名由 `(core.Signal) error`
  改为 `(core.Signal) (routed bool, err error)`——filter（cooldown/confidence/action）命中返回
  `(false,nil)`，成功路由返回 `(true,nil)`。
- **app.go**：analyzeSymbol 路由循环 `routed, err := a.router.Route(sig)`，仅当 `routed && executor != nil`
  才 `SubmitSignal`。cooldown/去重抑制的信号不再触发下单。
- **scope**：锁内将 packages 由 `[./internal/app]` 扩为 `[./internal/app, ./internal/router]`
  （核对无其他在途任务占用 router，validator/gate 无冲突）。Route 调用方仅 app.go + router 自身测试，
  唯一赋值点 router_test.go:50 已同步改 `routed, err :=`。

## 新增/调整测试（反 fantasy，断言落在状态而非碰巧路径）
- app 层 `TestApp_Executor_CooldownSuppressedNotSubmitted`：同标的 2 信号，第 1 个路由+下单，
  第 2 个被 cooldown 抑制 → SubmitSignal 只触发 1 次（旧 bug 下是 2 次）。
- router 层 `TestRouter_RouteReportsCooldownSuppression` / `TestRouter_RouteReportsConfidenceSuppression`：
  锁定 Route 的 routed 返回契约。
- `TestApp_Executor_ErrorDoesNotStop` 原断言依赖旧 bug（同标的 2 信号都下单=4 次），改用 distinct symbol
  断言 count==3，保留"executor 出错不中断后续标的"的原意。

## 验证
- `go test -race ./internal/app/ ./internal/router/` → ok
- 组合门禁 `-coverpkg=app,router` 总覆盖率 **89.7%**（≥80%）；app 单包 94.5%
- gofmt 干净、go vet 干净、go build ./... 干净
- commit **16d52a8**（fix(TASK-005)，scope：app + router + discovery）

## dev-agent-2 现状
- TASK-002 verified、TASK-010 dev_done(返工待复验)、TASK-005 dev_done(W2 返工待复验)
- 无 assigned 任务，转入待命。如再被退回按反馈修复。

注：code-simplifier 本轮返回"Blocked on human decision / No further action"（未改动任何文件）；
代码在我交付时已是简化状态，pre-commit 简化步骤已按规范触发。
