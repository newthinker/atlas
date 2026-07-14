# TASK-203 serve alert 装配 + 派生指标 — 验证报告

- 验证者: test-agent-1
- 结论: **VERIFIED（PASS）**
- commit: 6f23172（epoch=1, dev-agent-3）/ 分支 feature/audit-optimization-wave1-cleanup
- coverage_minimum=35（cmd/atlas package main 存量样板，Leader 裁决，整包实测 65.1%）

## 反向验收
- 改动 5 文件 = estimated_files（新增 alert_runner.go/.test，改 serve.go/serve_test.go/config.example.yaml）。
- internal/alert 本体零改动（git diff 确认未涉及 internal/alert/）。

## 新增代码逐函数覆盖（Leader 主要要求，go tool cover -func）
| 函数 | 覆盖 |
|---|---|
| httpErrorRate | 100.0% |
| mapRules | 100.0% |
| evaluateOnce | 100.0% |
| run | 100.0% |
| maybeStartAlertRunner | 93.3%（≥90% 达标；未覆盖为 reg==nil 快照分支的部分组合） |

## Done Criteria 覆盖矩阵（测试 fresh 非缓存，cmd/atlas 全 26+ 用例 PASS）

| # | 维度 | 标准 | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | Enabled 时循环 Snapshot→派生→SetMetrics→EvaluateAll，触发送适配器 | evaluateOnce 实现该管线；TestAlertRunner_EvaluateOnce_FiresNotifier（基线不触发→rate0.2 触发1次）、TestAlertRunner_Run_TicksAndEvaluates | **PASS** |
| F1 | functional | http_error_rate 相邻快照 _5xx/基名增量；首轮不产出；负增量 clamp0；不用累计比率 | httpErrorRate 用 http_requests_total_5xx 与 http_requests_total 增量；FirstSnapshotNoBaseline(produced=false)、NormalDelta(0.2)、NegativeDeltaClampsZero(0) | **PASS** |
| F2 | functional | signals_24h = SignalStore.Count(from=now-24h) | evaluateOnce 调 count(now-24h)；TestAlertRunner_Signals24h_FromCount 断言 lastFrom==now-24h 且触发 | **PASS** |
| F3 | functional | config.AlertRule→alert.Rule 映射（含 for/字符串解码） | mapRules 五字段映射；TestMapRules_FieldMapping + TestMapRules_DurationStringDecode（真实 config.Load 解 for:5m→5m、check_interval 30s） | **PASS** |
| B0 | boundary | Enabled=false 零行为：不起 goroutine、不构造 Evaluator | maybeStartAlertRunner 首行 return nil；TestMaybeStartAlertRunner_DisabledReturnsNil | **PASS** |
| B1 | boundary | 总请求增量 0 不除零 | dTotal<=0 return 0,true；TestDerivedMetrics_HTTPErrorRate_ZeroTotalDeltaNoDivZero | **PASS** |
| E0 | error_handling | ctx 取消 goroutine 有限时间返回无泄漏（须停止断言）；单条 Notify 失败不中断 | run 在 ctx.Done return；TestAlertRunner_Run_StopsOnCtxCancel（interval=1h，cancel 后 2s 超时判泄漏）；NotifierErrorDoesNotStop（Notify 报错两规则均送达）；Signals24h_CountErrorSkips（count 错→跳过+warn，不触发不 panic） | **PASS** |
| N0 | non_functional | config.example.yaml 示例规则(http_error_rate>0.1 + for:5m)及注释；alert 零改动 | example.yaml 规则改为 http_error_rate>0.1/for:5m 并注明可用键、裁剪不产出的 up/error_rate；alert 零改动 | **PASS** |

## Leader 九项关注点确认
1. 新码逐函数覆盖：见上表，全部达标。 2. 停止断言：StopsOnCtxCancel（2s 超时检泄漏）。 3. http_error_rate 三边界：均测。
4. signals_24h Count(now-24h)+出错跳过：均测。 5. for:"5m" 经真实 config.Load 解码：DurationStringDecode。
6. Enabled=false 不起 goroutine：DisabledReturnsNil。 7. alert 零改动：git 确认。 8. example 规则可用：引用已产出键 http_error_rate、for:5m 可解码。
9. registerConfiguredNotifiers int→[]notifier.Notifier：serve_test 9 处断言纯机械 `got!=N`→`len(got)!=N`，仍断言精确数量，次要断言(statsNotifierCount/日志过滤)全保留，无弱化。

## 非阻断观察
- maybeStartAlertRunner 93.3%：未覆盖为 snapshot 闭包 reg==nil 分支的组合，主路径已覆盖，达标不阻断。

全部条目 PASS，证据充分（fresh + 逐函数覆盖）→ VERIFIED。
