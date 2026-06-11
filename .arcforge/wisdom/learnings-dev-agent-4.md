
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

## TASK-009: 多包合并覆盖门禁 + code-simplifier 再次顺带补测试
- hook 用 `-coverpkg=pkg1,pkg2,pkg3` 把多包合并成单一 total 算门禁（不是各包独立判）。
  跨包 coverpkg 会把每个包代码算进所有 test binary 的并集 → 合并 total 往往低于任一单包覆盖。
  本任务只加 AssetTypes 声明时合并 total=80.0% 恰好压线（`${TOTAL%.*}` -lt 80 = false 勉强过）。
- 自测合并门禁必须复刻 hook 的 coverpkg 写法。坑：本机 Bash tool 是 zsh，不对未加引号变量做
  word-splitting，`go test $PKGS`（PKGS 含换行）会被当单参数报 "directory not found"。
  hook 自身是 #!/bin/bash 没问题；自测时显式空格分隔包名即可。
- code-simplifier 子代理（第三次）又越权：没简化我的代码，反而**新增** 4 个特征化测试
  (Description/Init) 把覆盖从 80→93.3%。这次是净收益（纯测试、不动既有测试、消除压线脆性），
  予以保留并在 discovery 标注"非 DoD，是覆盖特征化测试"。但仍验证了它会自作主张——
  事后逐文件 git diff 复核是必须的，不能盲信其总结（它还返回了含糊的"等你决定"消息）。

## TASK-005: lixinger 嵌套 metric 不能复用平铺 postJSON
- 既有 postJSON decode 进平铺 lixingerResponse(pe_ttm float64)，承载不了 pe_ttm.y5.cvpos 嵌套。
  解法：加 postJSONRaw 返回原始 body（复用同一 POST+StatusCode 守卫），调用方自解析进
  []map[string]any + digFloat(path...) 下钻。避免改既有 postJSON 影响其它 5 个方法。
- ISSUE-1 实操：HTTP 错误测试必须『合法 JSON body + 非200』专门打 StatusCode 守卫，
  与『HTTP200+业务码非0』『metric 字段缺失』分成 3 个独立测试，否则三条错误路径挤在
  一个 decode 失败上 = fantasy-pass。本任务 _HTTPError/_BusinessError/_MissingMetric 分离。
- 「实现首日核对项」类 caveat（成功码、键名 metricsList vs metrics、第三方代码映射）无 API_KEY
  时按既有代码约定 + plan 候选值实现，必须在 discovery 显式列为冻结/核对项，让 QA/集成阶段知道
  这些是未经真实 API 验证的假设，而非已证事实。

## TASK-004: 「GREEN-on-arrival」任务要诚实标注，别假装 RED
- 接表前后输出等价（索引 secid 市场前缀与 .SH/.SZ 后缀对当前数据巧合一致），测试写完直接 PASS。
  plan Step 2 已预判。处理：照写测试+按 DoD 接权威表（真相源迁移，非死代码），discovery 里
  明说 RED 未自然失败的原因 + 实现的真实价值（解耦后缀、防未来分歧）+ 加 secid==表值 权威断言
  作回归守卫。不要为凑 RED 伪造发散数据（会污染他人拥有的 indexes.go）。

## code-simplifier 子代理第 3/4 次：返回含糊『等你决定/idle by design』
- 两次都返回模糊收尾语而非明确结论，但实际有动作（TASK-005 把港股补零 for 循环换成
  fmt.Sprintf("%05s",code)）。教训不变：无论它说什么，一律 git diff 复核它真实改了什么 +
  重跑 -race 测试。%05s 对字符串确实零填充（实测 [00700]），保留；但若它改的是行为敏感处
  必须实测验证（我 go run 验证了 %05s 非空格填充才敢留）。
