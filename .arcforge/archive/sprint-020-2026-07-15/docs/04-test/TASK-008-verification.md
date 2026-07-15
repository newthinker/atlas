# TASK-008 验证报告 — cmd 层 buildNotifyContext

- **验证者**: test-agent-1
- **提交**: 28a7cca（cmd/atlas/crisis.go +77 纯插入 buildNotifyContext + crisis_test.go +234：助手 + 5 测试）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: VERIFIED（PASS）— 第二个一次通过零返工的任务
- **覆盖率口径**: coverage_minimum=35（Leader 裁决，归档 8 处 cmd/atlas 先例；package main 含任务外 glue）
- **一句话**: 8 条 DoD 全 PASS，6 个关键功能变异全被拦截；裁决(A) 的 3 个不可达 err 块行号(358/364/392)经我独立核实准确，3 个可触发错误路径(348/382/404)确认已覆盖（非仅采信 dev 自报）。

## 亲跑证据
- `GOTOOLCHAIN=local go build ./...` → exit 0
- `go test ./cmd/atlas/` → ok，整包 coverage 74.7%（≥35 达标；基线 73.9%，+0.8pp）
- buildNotifyContext 函数级 93.2%（未覆盖=裁决A 3 块）
- 5 新测试全过（含 TestBuildNotifyContextStoreErrors 3 子测试）
- 关键功能变异矩阵（全部应 FAIL）：
  | 变异 | 目标测试 | 结果 |
  |---|---|---|
  | functional[1] 非变更日去 +1 | TestBuildNotifyContext | FAIL ✓ |
  | functional[1] 变更日 PrevState→State | TestBuildNotifyContextTransitionAndTrends | FAIL ✓ |
  | functional[3] ClearStreak 去 !AnyTrigger | TestBuildNotifyContextClearStreakConditions | FAIL ✓ |
  | functional[4] Trends 去 NORMAL 守卫 | TestBuildNotifyContext | FAIL ✓ |
  | functional[4] Delta 首末反转 | TestBuildNotifyContextTransitionAndTrends | FAIL ✓ |
  | functional[2] NewStale 去重条件反转 | TestBuildNotifyContext | FAIL ✓ |

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | PrevDay 每指标最近1条(昨日行非当日) | TestBuildNotifyContext：PrevDay[vix]==GREEN，而今日 res vix=STALE → 取的是昨日行 | PASS |
| functional[1] | StateDays 变更日=前状态streak / 非变更日=当前+1(决策6) | 非变更(=2：1 历史 WATCH+今日) + 变更(=2：前状态 WATCH 2 历史行)；两分支变异(去+1/误用State)均 FAIL | PASS |
| functional[2] | NewStale 昨日非STALE今日STALE去重；StaleLastObs 组装/缺省 | TestBuildNotifyContext(move 昨STALE去重→[vix]、vix 无观测→缺省)+StaleLastObs(vix 有观测→2026-07-14)；去重条件反转变异 FAIL | PASS |
| functional[3] | ClearStreak 仅 WATCH∧SummaryDue∧!AnyTrigger=历史+1(决策8) | TestBuildNotifyContext(三真=2)+ClearStreakConditions(逐条否定：非WATCH/非SummaryDue/AnyTrigger 各=0)；去 !AnyTrigger 变异 FAIL | PASS |
| functional[4] | Trends 仅 SummaryDue∧NORMAL，21观测升序 Delta=末-首，无观测不进map | TransitionAndTrends(NORMAL月首 Len=21、Delta=末-首、move 无观测不进map、WATCH 分支 nil)；去NORMAL守卫+Delta反转变异均 FAIL | PASS |
| error_handling[0] | store 查询错误原样上抛 | 3 可独立触发路径已覆盖(count≥1)：RecentIndicatorEvals(348 关库)/LatestObservation(382 删表)/SeriesWindow(404 删表)，TestBuildNotifyContextStoreErrors 3 子测试 require.Error。**裁决(A)**：stateStreakDays×2(358/364)+ClearStreakDays(392)结构性不可达（同查 crisis_evaluations，前置 RecentIndicatorEvals 同表必先败），接受记录在案缺口，不判 FAIL | PASS（裁决A） |
| non_functional[0] (test) | 不改 executeCrisisEvalDaily + build ./... + 旧 Messages 输出不变 | executeCrisisEvalDaily/Intraday 签名 diff 无 +/-（纯插入其后）；build ./... exit0；既有 cmd/atlas 全套绿零回归 | PASS |
| non_functional[1] (review) | 时序约束(须在 AppendEvaluations 之前)在函数注释声明 | crisis.go:341 函数头注释"必须在 AppendEvaluations 之前调用：PrevDay/StateDays/ClearStreak 都取截至昨日的库内历史，当日增量在此函数内补足" | PASS |

## 裁决(A) 独立核实
- buildNotifyContext(341-413) 内 count=0 块经 coverprofile 核实**恰为 3 个**：358.17（stateStreakDays 变更分支 err）、364.17（非变更分支 err）、392.17（ClearStreakDays err）——与裁决(A) 一致。
- 覆盖工具另报 419.16 未覆盖块，核实属**既有函数 buildCrisisSender**（loadConfigOrDefaults 错误分支，非 T8 范围，本次纯插入未触及）。
- 3 个可触发 err 返回(348/382/404)正向确认 count≥1，即 dev 声称"已 exercise"属实，非仅采信自报。
- 结构性不可达论据成立：三块均查 crisis_evaluations，PrevDay 循环首个 RecentIndicatorEvals 查同表且在前，具体类型 *crisis.Store 无法构造"首查成功后查失败"。抽接口注入 fake 会波及 Task 9 生产改动，收益 3 块不成比例——同 TASK-004 SliceStable 不可锁先例。

## detect_changes（Leader 代跑）
low、affected_processes 空；6 个既有 cmd 符号 touched 为纯插入位移伪影（已核实 diff 零改动既有函数）。crisis.go 单一 +77 纯插入 hunk。

## 结论
8 条 DoD 全 PASS（error_handling[0] 按裁决A），6 功能变异确认关键逻辑均已锁定，裁决(A) 3 块不可达经独立核实准确、可触发路径确认覆盖。TASK-008 verified。
