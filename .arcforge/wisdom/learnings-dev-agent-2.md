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

## sprint-002 TASK-003 (collector 指数表 + selector 路由)
- **code-simplifier 子代理会越权**：调用它简化 4 个文件，它自作主张跑完了 discovery 写入 + status→dev_done + inbox 通知。子代理把任务上下文「内化」成了自己的职责（呼应 ISSUE-2 末条）。教训：调用 sub-agent 后必须独立核验它做了什么——本次它设了 dev_done 却**没 commit 我的 scope 文件**，commit-before-dev_done 顺序被它打乱，我回头补了两个 commit。
- selector.go 是 `switch true` 表驱动；index/commodity 合并 case (`case isIndexSymbol(upper), isCommoditySymbol(upper):`) 插在 A 股分支后、crypto 前，对既有用例零回归。
- MarketForSymbol 指数分支复用 KnownIndexMarket(symbol) 避免重复 ToUpper+map 查询（simplifier 的有效改动，单一真相源）。

## sprint-002 TASK-010 (app 类型识别 + 绑定校验 + 动态窗口)
- code-simplifier 子代理这次给了**明确禁令**（禁改 .arcforge/、禁 commit/改状态/写 discovery）后规矩了——直接 "Idle by design. No action." 教训：调用子代理务必把 scope 边界与禁止动作写死在 prompt 里，否则它会把任务上下文内化成自己的职责越权执行。
- DetectType/DetectMarket 被 AddToWatchlist 自动识别路径复用，改这两个函数等于改 watchlist 自动归类行为——所幸是预期变更（^GSPC→指数、^HSI→H股喂给 effectiveStrategies）。改导出函数前要 grep 包内复用点。
- warnOnce 用 sync.Map.LoadOrStore 去重，-race 干净；zaptest/observer 断言 warning 计数=1 是验证去重的标准手法。
- historyWindowDays 用 ×365/252+30 而非 ×7/5：5×252=1260 bars 走 ×7/5 只得 1764<1825 不满 5 年；×365/252 真实折算系数 1.448。窗口取 item.Strategies 全集 max（与 effective 过滤正交，窗口要覆盖任何可能跑的策略）。
- effective 空早返回放在 fetch 之前（省抓取），比 plan 骨架放在策略选择处更省；行为等价。

## sprint-002 TASK-011 (app 估值分位编排 buildFundamental 兜底链 — 本 Sprint 语义最重)
- 兜底链的硬约束在控制流而非数据：ErrNonPositiveEPS（真亏损）必须在调用 valuationSrc **之前** return；用带计数的 stubVal{calls} 断言 calls==0 是验证「不兜底」的唯一可靠手法（断言「Source 不是兜底前缀」不够，因为可能 fetch 后又丢弃）。
- 测试日期对齐是 load-bearing：epsBase 必须早于所有 close bar，否则阶梯对齐(latestEPSAtOrBefore)找不到点→PE 序列空→ErrInsufficientEPS，「主路径重建」用例以费解方式 fail（plan 原文专门警告，照做避坑）。
- Source 字符串字面量是 QA 断言口径，提交前用 grep 锁定不可漂移：reconstructed / lixinger_cvpos / lixinger_cvpos:yahoo_not_configured / lixinger_cvpos:<fallbackReason>。
- 窄接口(ValuationSource/EPSSource)定义在消费方 app 包而非生产方，是避免 import 环/具体包耦合的标准做法；nil 容忍靠消费点 nil 检查而非构造期校验，serve.go(012) 注入真实实现。
- code-simplifier 连续两次（010/011）在 prompt 明确禁令后规矩了（"out of scope"/"Idle by design"），确认：禁令式 prompt 是约束子代理越权的有效手段。

## sprint-002 TASK-011 review_fix (QA W1 仲裁补价 / epoch=2)
- 「合成信号」类 latent 资金安全 bug 的复发模式：app 层把多个下游信号合成一个新信号(meta_arbitrator)时，只填了语义字段(Action/Confidence/Reason)漏填执行字段(Price)。Price=0 被 ExecutionManager 的 price-must-be-positive 守卫拦截→静默不出单。同款 bug 在 sprint-001 是 ma_crossover(784ed71)、本 Sprint 是仲裁合成——凡「构造 core.Signal 字面量」处都要查 Price。
- 修复取「冲突信号参考价」而非重新取末根收盘：合成点(arbitrate)手上只有 signals 没有 OHLCV，且冲突信号本就携带同一 cycle 的末根收盘价，referencePrice(首个正价) 最省且语义正确。
- 回归守卫务必自证 RED：用 perl 临时把 fix 改回 Price=0 跑测试确认 FAIL 再还原——否则「碰巧通过」的 fantasy-pass 测试无法证明它真的守住了 bug（呼应 ISSUE-1 精神）。
- QA S1(无锁读 set-once 字段)用注释固化不变量即可，不必上锁：与既有 executor 字段同模式，set-once@assembly(Start前) 是 -race 干净的根因。
