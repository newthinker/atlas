
## TASK-003: 装饰/适配层的 e2e 测试会暴露上游集成缺陷
- ExecutionManager.Execute 构造市价 OrderRequest 时漏带 Price，而 paper.PaperBroker
  要求 request.Price>0 才成交。单元 mock 测试覆盖不到（mock 不校验 Price），只有用真实链
  跑 e2e（BUY → 断言余额/持仓变化）才会暴露。教训：跨层接线任务的 DoD 里"端到端真实链"
  测试是必需的，不能用 mock 替代——它正是"验证链路真实可用"的落点。
- 修复需触碰非声明 package（internal/broker）。协议：先确认无在途任务占用该包，再锁内
  epoch 校验后防护性扩 packages 字段，跑全包回归测试确认无副作用。

## cmd/atlas (package main) 的 80% 整包覆盖门禁不可行
- package main 充满 cobra 命令处理器 + 阻塞式 runServe + main()，天然难测到 80%。
  本任务只新增 executor.go（~95% 覆盖）却被既有未测样板拖到整包 19%。hook 按整包算 →
  必然阻断。这类任务应在 DoD 制定时就约定按"改动文件覆盖 + DoD 验收"，否则 Dev 只能
  blocked_clarification 找 Leader 裁决。提交前先模拟 hook 的 coverpkg 命令自测可提早发现。

## TASK-007: cmd/atlas (package main) 覆盖率门禁的务实解法
- 即便 Leader 把 coverage_minimum 降到 35，cmd/atlas 仅靠本任务的小改动（maybeCache 4 条 DoD 测试）
  也只到 ~20.6%——因为 package main 充满既有未测 CLI 样板（runServe/main 不可测，broker.go/backtest.go
  的 cobra handler 全 0%）。
- 解法（优于再次 escalate）：为同包内『无在途任务占用』的既有未测代码补特征化测试。broker.go 的 7 个
  CLI handler 用 mock broker 一个 broker_test.go 即覆盖，整包升到 45.9%。注意避开其它 cmd/atlas 任务
  已声明的文件（TASK-008 占 backtest.go，故只碰 broker.go）。在 discovery 明确标注『非 DoD，是覆盖门禁
  补测』供 QA 区分。
- 装饰器接线的 boundary 关键：CachedCollector 只嵌入基础 Collector 接口，会"吃掉"扩展接口
  （FundamentalCollector）。包装前必须 `if _, ok := c.(ExtIface); ok { return c }` 守卫，否则破坏下游
  类型断言消费路径。

## 通用：code-simplifier 子代理会"顺手"推进任务状态
- 两次调用 code-simplifier 子代理，它都不止简化，还把任务走完了 discovery+dev_done（锁内 epoch 校验正确），
  但都跳过了 git commit。教训：调用后务必复核 (1) 测试仍绿 (2) 它改了哪些文件 (3) 状态/discovery 是否正确
  (4) 自己补上 commit。子代理 prompt 里已写"不要 commit/不要碰 .arcforge"仍被越权——以实际落盘为准复核。

## TASK-003 返工(QA W1): e2e 里硬编码输入会制造 fantasy-pass
- 我的 paper 执行链 e2e 用手搓 Signal{Price:100} 驱动，"证明"了链路能下单——但生产里信号来自
  ma_crossover，而它根本不设 Signal.Price。真实 serve 下 Price=0 → ExecutionManager 拒 → 适配器
  吞错 → 永不下单。测试全绿但功能在生产惰性（QA 称 fantasy-pass）。
- 教训：跨层"验证链路真实可用"的 e2e，输入必须来自真实上游组件（这里=真实策略 Analyze 产出的信号），
  不能手搓关键字段。手搓输入只适合单元级边界测试，不能作为"端到端可用"的证据。
- 修复同时补"失败模式守卫"：显式断言 Price=0 信号→不下单/无持仓，让回归（谁再把 Price 丢了）当场被抓，
  而不是再次静默惰性。
