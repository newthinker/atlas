# TASK-001 验证报告 — router 步进门控核心（Route 分流 + 策略级步长）

- 验证者: test-agent-1（Reality Checker 心智模型）
- 任务: TASK-001 / commit 055d062 / 分支 feature/percentile-step
- 判定: **PASS -> verified**

## 1. Done Criteria 覆盖矩阵

| # | 维度 | 完成标准 | 对应测试 | 判定 | 证据 |
|---|------|----------|----------|------|------|
| functional[0] | functional | 买入侧步进 49放行/47抑制/44放行/46抑制/49放行(恢复重算) | TestRoute_PercentileStep_BuySide | PASS | 表驱动 5 步逐条 Fatalf 断言，序列与标准逐位一致 |
| functional[1] | functional | 卖出侧对称 81放行/83抑制/86放行 | TestRoute_PercentileStep_SellSideSymmetric | PASS | 表驱动 3 步断言 |
| functional[2] | functional | key 独立：buy/sell 独立、不同 strategy 独立、strong_buy 与 buy 共享同侧 key | TestRoute_PercentileStep_KeysIndependent | PASS | sell 首发放行 / pe_percentile 首发放行 / strong_buy 47 被 buy 侧 49 抑制 三断言 |
| functional[3] | functional | 策略级步长三态：step=3.0 覆盖全局5；全局0+信号3 仍启用；string 类型异常回退全局5 | TestRoute_PercentileStep_PerStrategyOverride | PASS | 三态各有断言：{49,47,46}按3；r0(全局0) 49放行/48抑制；string "3" 回退5(45抑制) |
| functional[4] | functional | 静态过滤前置：低 confidence 分位信号 routed=false 且不写门控状态，后续合格信号按首次放行 | TestRoute_PercentileStep_StaticFilterBeforeGate | PASS | conf=0.1 被拦截；同分位合格信号随后按首次放行，证明未写门控状态 |
| boundary[0] | boundary | 全局 step=0 且无 step 元数据 -> 走冷却路径；既有用例零回归 | TestRoute_StepDisabled_UsesCooldown + 既有 12 用例 | PASS | step=0 时 49放行/30被冷却抑制；20/20 全 PASS |
| boundary[1] | boundary | 分位元数据非 float64(string) -> 回退冷却不 panic | TestRoute_PercentileStep_BadMetadataFallsBackToCooldown | PASS | string 元数据首发经冷却放行、二发被冷却抑制，无 panic |
| non_functional[0] | non_functional (verify_by:review) | 单临界区 check+update；冷却戳只在冷却分支更新；percentileOf 类型不符输 debug 日志 | 代码评审 | PASS(review) | router.go:270-281 单 r.mu.Lock 内 check+update；:98-100 冷却戳仅冷却分支；:236-241 debug 日志 |

## 2. 实现一致性核查（设计 §4/§5）
- Route() 先 passesStaticFilters（:74），故低置信信号在触及门控前即被拦，不写 pctGates —— functional[4] 语义由结构保证。
- 分位分支（:87-90）不查不更新冷却戳；冷却戳更新（:98-100）仅在 else 冷却分支 —— 分位信号不压制同标的其它策略。
- passPercentileGate（:270-281）单 r.mu.Lock + defer Unlock，check（|pct-last|<step）与 update（pctGates[key]=pct）同临界区，无 check-then-act 竞态。
- effectiveStep（:249-256）：Metadata["percentile_step"] 为 float64 且 >0 才采用，否则回退全局 —— string "3" 自然回退，三态正确。
- 测试有效性抽查：若门控写在静态过滤之前，functional[4] 第二条会被抑制而失败；若冷却戳在分位分支也更新，则与 TASK-002 NotTouchCooldown 冲突 —— 非空转测试。

## 3. 覆盖率与范围
- 覆盖率 83.8%（>=80% 门禁）。新增 6 函数（percentileOf/effectiveStep/sideOf/passPercentileGate/passesStaticFilters/passesCooldown）均 100%；Route 86.4%。
- 未覆盖 ClearAllCooldowns(0%)/StartCleanupRoutine(0%) 为既有未测函数，ClearAllCooldowns 属 TASK-002 范围，非本任务。
- git show 055d062 --stat：仅 router.go + router_test.go（声明 package 内，无越界）。router 目录 git 干净，无 WIP 干扰。

## 4. 次要观察（非阻断）
- Route() :87-88 对 effectiveStep(signal) 调用两次（一次判定 >0、一次传参），轻微重复计算，无正确性影响。可选优化。

## 5. 结论
7 条功能/边界 done_criteria 全部有对应、有意义测试且实测 PASS（go test -count=1 全 20/20 PASS）；non_functional(review) 经代码评审确认单临界区/冷却戳隔离/debug 日志均到位；覆盖率 83.8%，范围未越界。**verified**。
