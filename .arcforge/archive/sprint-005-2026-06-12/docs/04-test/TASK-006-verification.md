# TASK-006 验证报告 — app.New() 配置接线（修复 cfg.Router 死配置预存 bug）

- 验证者: test-agent-1（Reality Checker 心智模型）
- 任务: TASK-006 / commit eb9c12b (HEAD) / 分支 feature/percentile-step
- 判定: **PASS -> verified**

## 1. Done Criteria 覆盖矩阵

| # | 维度 | 完成标准 | 对应测试 | 判定 | 证据 |
|---|------|----------|----------|------|------|
| functional[0] | functional | CooldownHours=24 → GetStats cooldown_seconds==86400.0 | TestNew_RouterConfigFromCfg | PASS | 断言 stats[cooldown_seconds]==float64(24*3600)=86400（死配置 bug 修复实证） |
| functional[1] | functional | MinConfidence=0.7 接线生效 | TestNew_RouterConfigFromCfg | PASS | 断言 stats[min_confidence]==0.7 |
| functional[2] | functional | PercentileStep=5 接线生效 | TestNew_RouterConfigFromCfg | PASS | 断言 stats[percentile_step]==5.0 |
| functional[3] | functional | EnabledActions 仍硬编码四 action，不从 config 读 | TestNew_RouterConfigFromCfg | PASS | 断言 enabled_actions len==4；app.go:93-94 硬编码带 YAGNI 注释 |
| boundary[0] | boundary | cooldown_hours=0 → 冷却禁用恒放行（连续两条无元数据信号均 route） | TestNew_CooldownDisabledWhenZeroHours | PASS | CooldownHours=0，同标的 ma_crossover 连续两条 Route 均 routed=true |
| non_functional[0] | non_functional (review) | 提交信息注明存量行为变更 BREAKING-ish 段落（1h→4h、0.5→0.6） | git log -1 eb9c12b | PASS(review) | 提交信息含 "BREAKING-ish: ... documented defaults (cooldown 4h, min_confidence 0.6) instead of the hardcoded 1h/0.5" |
| non_functional[1] | non_functional (test) | 既有 app 用例零回归 | 全包 39 用例 | PASS | go test ./internal/app/ -count=1 全 39 PASS |

## 2. 实现一致性核查（设计 §2 装配点修复）
- app.go:90-95 routerCfg 改为 cfg.Router 映射：MinConfidence=cfg.Router.MinConfidence、CooldownDuration=Duration(CooldownHours)*Hour（带 0=禁用注释）、PercentileStep=cfg.Router.PercentileStep。
- EnabledActions 维持硬编码四 action（带 YAGNI 注释），未从 config 读 —— functional[3] 满足。
- 回归连带：TestApp_Executor_CooldownSuppressedNotSubmitted 原用 New(&config.Config{}) 隐式依赖被修复的硬编码 1h；接线后空 config CooldownHours=0=禁用，故显式设 CooldownHours=1 以保留 W2 原意。已核对其断言体未被削弱：仍断言 exec.count()==1（冷却抑制信号不提交执行）+ noti.received()==1（仅一条 routed）—— 非削弱式 fix，是死配置 bug 修复的预期连带。
- 测试有效性：若 New 仍硬编码忽略 cfg.Router，functional[0-2] 实测值恒 3600/0.5/0 立即失败 —— 非空转测试。

## 3. 覆盖率与范围
- 覆盖率 96.3%（>=80% 门禁）；New() 100%。
- git show eb9c12b --stat：仅 app.go(+8/-4) + app_test.go(+58)（声明 package 内，无越界）。working tree 干净，commit 为 HEAD。

## 4. 结论
4 条功能 + 1 条边界 done_criteria 全部有对应、有意义测试且实测 PASS（go test ./internal/app/ -count=1 全 39/39 PASS）；non_functional review（提交信息 BREAKING-ish 段落）与 test（零回归）均满足；覆盖率 96.3%，范围未越界。修复死配置预存 bug 实证到位。**verified**。
