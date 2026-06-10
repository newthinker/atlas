# Learnings — dev-agent-2

## 2026-06-10 收尾：commit 时机 & 共享工作区门禁
- 教训：`dev_done` 前若不 commit，已验证任务的改动滞留共享工作区，会让 task-completed.sh
  对**后一个任务**误报 scope 漂移（门禁排除集只含 in_progress/dev_done/verifying，漏 verified）。
  解法：每个任务 dev_done 前严格 commit 本任务 scope 文件，保持工作区干净。
- SA1012：测试里 `Start(nil)` 传 nil context 会被 vet 拦；用 `context.Background()`。
- app 包覆盖率门禁是「整包」口径：只测自己接线点不够，需补已存在的未覆盖 getter/setter
  (SetSignalStore/SetArbitrator/Stop/Get*/Add*/Remove*/Detect*) 才过 80%。

## 2026-06-10 TASK-005 worker pool 并行化 + 仲裁超时
- **-race 揪出测试桩缺陷**：strategy.Engine.Analyze 会原地写 signals[i].Strategy。
  Engine 本身并发安全（RLock 读 map），但要求每个 Strategy 返回独立 slice。
  mockStrategy 返回共享底层数组 → 并行分析下多 goroutine 写同一内存 → race。
  教训：并发测试里 mock 返回的 slice/map 必须每次 copy，否则把"测试桩 bug"误报为"生产 race"。
- **并行路由 → notifier 桩需自带锁**：workers>1 时多 goroutine 并发 notifier.Send，
  共享 received slice 的 append 是 race。给 mock 加 mutex + received() 拷贝读取。
- **超时降级测试要避开 router cooldown 干扰**：同一 symbol 的第二个原始信号会被
  passesFilters 的 cooldown 过滤，故"超时返回 2 个原始信号"断言会失败（只到 1 个）。
  正确断言：路由到的是原始信号（Strategy != "meta_arbitrator"），而非比较数量。
- **typed-nil 接口陷阱**：把 *meta.Arbitrator 存进接口字段时，SetArbitrator(nil) 传入
  typed-nil 指针会让接口非 nil → arb!=nil 误判。守卫：if arb==nil { 存 nil interface }。
- **errgroup 依赖**：golang.org/x/sync 原是 indirect，直接 import 后 go build 报
  "missing go.sum entry"；`go get pkg@ver` + `go mod tidy` 提升为 direct。
- **gopls 假错**：编辑期 diagnostics 对 cfg.Analysis/Timeout（TASK-004 新增字段）报
  "undefined"，实为 gopls 缓存陈旧；以 `go build`/`go test` 结果为唯一判据，勿被带偏。

## 2026-06-10 TASK-005 W2 返工：执行不应受 cooldown 旁路
- **缺陷模式**：Route 与 SubmitSignal 两步独立时，router 过滤(cooldown/confidence)只挡了
  通知，没挡下单 → 被去重的信号仍下单。修复让"是否已路由"成为可判定结果：
  Route 返回 (routed bool, err error)，调用方据 routed 决定后续副作用。
- **改返回值的 blast radius 评估**：先 grep 所有调用点。Go 里 `f()` 作为表达式语句可丢弃
  全部多返回值，所以只有"赋值点"(`x := f()`)需要改——本例只有 app.go 1 处 + 1 个 router 测试。
- **改 scope 要先查冲突**：扩 packages 前用 jq 扫所有 in_progress/dev_done/verifying/verified/accepted
  任务的 packages，确认目标包无人占用，再锁内防护性写入 packages 字段。
- **返工常暴露旧测试的 fantasy 性**：旧 TestApp_Executor_ErrorDoesNotStop 断言 count==4 实际依赖
  "同标的多信号都下单"这一 bug；修 bug 后必须重写为 distinct symbol(count==3) 才真实。
  改实现连带改测试时，确认测试断言落在"期望行为"而非"恰好通过的旧路径"。
- **组合覆盖率门禁**：task-completed.sh 用 -coverpkg=<所有声明包> 跑一次取 total。某包单独偏低
  (router 77.8%) 不一定卡门禁——app 测试经 Route 间接覆盖 router，组合 total 89.7% 即过。
