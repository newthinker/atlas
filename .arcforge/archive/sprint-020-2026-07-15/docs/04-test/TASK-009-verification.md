# TASK-009 验证报告 — 切换：notify.go 重写 + cmd 接线 + 全家族测试（终局）

- **验证者**: test-agent-1
- **提交**: 454083e（AD-1 唯一跨两包：notify.go/notify_test.go 重写 + crisis.go 接线 + crisis_test.go 适配，+147/-141）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: VERIFIED（PASS）— 第三个一次通过零返工的任务（终局切换）
- **覆盖率**: internal/crisis 93.8%、cmd/atlas 74.4%（≥ coverage_minimum 35）；Messages/FormatIntradayAlert 函数级 100%
- **一句话**: 全链切换干净——装配矩阵四分支+互斥经变异全锁、时序锁测试真能拦颠倒（第2日→第3日）、旧符号 4 类全仓无残留、7 类全家族禁词/页脚归属源码+测试双验、build ./... + 两包全绿。

## 亲跑证据
- `go build ./...` exit 0；`go test ./internal/crisis/ ./cmd/atlas/` 两包全绿
- 覆盖：internal 93.8%、cmd 74.4%（≥35）；Messages 100%、FormatIntradayAlert 100%
- 关键变异（全部应 FAIL）：
  | 变异 | 目标测试 | 结果 |
  |---|---|---|
  | 时序：buildNotifyContext 挪到 AppendEvaluations 之后 | TestExecuteCrisisEvalDailyContextBeforeAppend | FAIL ✓（报"第 3 日"） |
  | 装配：日报去 CRISIS 分支 | TestMessagesDispatch | FAIL ✓ |
  | 装配：月报守卫 NORMAL→WATCH（优先级错乱） | TestMessagesDispatch | FAIL ✓ |
  | 装配：NewStale 循环去除 | TestMessagesDispatch | FAIL ✓ |
  | 装配：去状态变更优先分支 | TestMessagesDispatch | FAIL ✓ |

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | Messages 装配矩阵：状态变更>BREWING/CRISIS 日报>NORMAL 月报>WATCH 周报，结构化至多一条；NewStale 每指标追加 [P2] | TestMessagesDispatch：四分支各 require.Len==1 + 互斥否定（变更日∧SummaryDue 只出变更、NotContains"周报"）+ NewStale 空/单/多（1结构化+N个P2，顺序即NewStale序）。4 装配变异全 FAIL | PASS |
| functional[1] | FormatIntradayAlert §5.7 格式 + 无页脚 | TestFormatIntradayAlert："[P0] 🚨 USD/JPY 盘中急跌 -3.4% · 07-18 14:30"+现价/基准/系统状态/去重+NotContains"非交易信号"；函数级 100% | PASS |
| functional[2] | executeCrisisEvalDaily：buildNotifyContext 在 AppendEvaluations 前；sender nil→打印 out；发送失败仅 errOut warning 不中断 | 时序：源码顺序正确 + TestExecuteCrisisEvalDailyContextBeforeAppend 变异颠倒→FAIL（第2→第3日，真区分性）；nil sender→TestExecuteCrisisEvalDailyNilSenderPrints；发送失败→TestExecuteCrisisEvalDailyNotifyFailureDoesNotAbort（源码仅 Fprintf errOut 无 return） | PASS |
| functional[3] | 旧 footer 常量/旧 Messages(res,days,due,stale)/indicatorLines/cmd staleIndicators 已删且无残留调用方 | 全仓 grep（含测试文件）：四者均无残留（footer 仅 notifyFooter、无四参 Messages、无 indicatorLines、无 staleIndicators）；TestStaleIndicators 测试同步删除 | PASS |
| boundary[0] | NORMAL 非到期 → Messages 返回空 | TestMessagesDispatch：assert.Empty(NORMAL,StateDays=30,非SummaryDue) | PASS |
| non_functional[0] (test) | 7 类禁词零出现；"非交易信号"仅结构化家族；单条≤4096 | TestMessagesForbiddenWordsAllFamilies：组装 7 类 require.Len(all,7)，禁词 NotContains、页脚归属（结构化 Contains/非结构化 NotContains）、len≤4096。另独立源码 grep notify.go+notify_render.go 无禁词；notifyFooter 仅 4 结构化渲染器引用（renderOpsAlert/FormatIntradayAlert 不引用） | PASS |
| non_functional[1] (test) | 两包全绿；cmd 既有断言只改措辞不放宽 | 两包 ok；cmd crisis_test.go 仅新增时序锁测试 + 删失效 TestStaleIndicators，既有断言（"NORMAL → WATCH"等）无删改、无放宽 | PASS |
| non_functional[2] (review) | gitnexus impact/detect_changes/code-simplifier | Leader 代跑：impact 全 LOW（收敛 runCrisisEval）、detect_changes risk medium（切换性质相称，受影响流仅 ExecuteCrisisEvalDaily→IsWeekend）、code-simplifier 无改动 | PASS |

## Leader 七点核查回复
1. 装配矩阵四分支+互斥否定：TestMessagesDispatch 四分支 require.Len==1 + 变更日∧SummaryDue 只出变更(NotContains周报) + NewStale 空/单/多；4 变异全 FAIL 锁死。
2. 旧符号负向 grep（含测试文件）：footer/四参Messages/indicatorLines/staleIndicators 全仓无残留。
3. 时序锁区分性：独立变异颠倒 buildNotifyContext↔AppendEvaluations 后测试 FAIL（第2日→第3日），真能拦。
4. 全家族禁词/页脚/4096：TestMessagesForbiddenWordsAllFamilies 覆盖全 7 类（含 FormatIntradayAlert），页脚归属正确；另源码级独立扫描无禁词。
5. cmd 既有断言未放宽：diff 仅 +1 时序测试 / −1 失效 staleIndicators 测试，既有 assert 无删改。
6. T5 遗留观察：T9 仅 dispatch 到既有渲染器，未引入新变级路径；renderTransition 空语义句守卫不变，无新增不可达路径。
7. 两包覆盖率(93.8%/74.4%≥35)+build ./...+全套亲跑绿。

## 结论
8 条 DoD 全 PASS，5 个关键变异（时序+4装配）全拦截，禁词/页脚双层核验、旧符号零残留。TASK-009 verified——**全链路交付，9/9 任务收官**。

## 提交 hash 更新（amend 454083e → 058765f，2026-07-15）
- HEAD = 058765f（未 push 的安全 amend）。`git diff -w 454083e 058765f` 忽略空白后语义等价——仅 crisis.go 删 1 处残留双空行、notify_test.go 新用例字段对齐，均 gofmt、语义零变化。上文全部验证结论（8 DoD、5 变异、覆盖率、旧符号零残留、禁词/页脚双验）对 058765f 完全适用；重跑 build ./... exit 0、两包测试绿确认。
- **gofmt 状态**：internal/crisis/notify.go、internal/crisis/notify_test.go、cmd/atlas/crisis.go 三文件 gofmt clean ✓。cmd/atlas/crisis_test.go 有 gofmt 漂移，但经独立核实**在 686/810/1169 行，全部落在 T9 改动区间(724-756 新增 / 772-798 删除)之外**——686 既有 summaryDue 测试、1169 系 T8(28a7cca) 引入的 dropMacroObs 助手、810 既有 deps 结构（T9 时序测试用 now: sat711 非此）。属既有漂移，非 T9 引入，按 surgical-changes 原则不在本任务修复范围，不判 FAIL。
- 非阻塞观察：crisis_test.go 既有 gofmt 漂移建议后续独立 cleanup（非本 Sprint 范围）。
