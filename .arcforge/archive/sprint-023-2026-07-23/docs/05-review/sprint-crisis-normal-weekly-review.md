# Sprint 终审 Code Review — crisis NORMAL 态周报

> 审查者：qa-agent-1（Reality Checker + Adversarial）
> 范围：git 7d14d4a..HEAD（8ae94fc / b717e9b / f5d7b82 / cbfbed9 / d59bc74）
> 依据：设计 docs/superpowers/specs/2026-07-23-crisis-normal-weekly-report-design.md、AD-1..AD-6
> 取证：GOTOOLCHAIN=local 实跑（非缓存）+ codex 跨模型独立复核

## 最终 Verdict：PASS

无 CRITICAL / WARNING。两轮审查 + 跨模型（codex/GPT）独立复核均无 high-severity 发现，reviewer 共识一致。

---

## 第一轮：常规 Code Review

### 正确性 — PASS
- 撞日归月报：`summaryKind`（cmd/atlas/crisis.go:434-453）NORMAL 分支先判 `isFirstTradingDayOfMonth` 返 SummaryMonthly，再判周一返 SummaryWeekly；分支顺序天然实现「周一撞月初只发月报」。测试 TestSummaryKind 覆盖 2026-06-01（周一∧首交易日→Monthly）、2026-08-03（8/1 周六顺延至周一首交易日→Monthly）。
- 路由互斥：Messages switch（notify.go:33-42）顺序 Transitioned → Brewing/Crisis → Monthly → Weekly，保证「结构化消息每日至多 1 条」不变量。周一撞状态变更日 → 变更消息优先、周报让位（TestMessagesDispatch 否定路径断言 NotContains「周报」）。
- NORMAL 周报去退出进度行：renderWeekly（notify_render.go:285-297）尾段 `if res.State == StateWatch` 才拼「退出进度」；NORMAL 仅「下次周报」行。

### 边界 — PASS
- 坏日期 / 周六非交易日 / NORMAL 周二 / WATCH 非周一 → SummaryNone（TestSummaryKind 逐条覆盖）。
- 枚举零值 SummaryNone = 不到期，未显式赋值的 NotifyContext 安全静默（设计 §4.3）。
- 周一美国假日：summaryKind 用 res.Date（被评估交易日）+ Monday 判定，与 WATCH 周报共用同一上游「数据未齐空跑/顺延」机制，无新增逻辑（设计 §4）——评估被跳过则该周无周报，评估照常则 res.Date 仍为周一照发，语义由上游覆盖，本变更未触碰。

### 回归风险 — PASS
- WATCH 周报字节级不变：旧 `"退出进度…\n下次周报…"` == 新 `"退出进度…\n" + "下次周报…"`，拼接结果逐字节相同；既有 TestRenderWeekly（含 WatchExitDays=25 异值注入锁）非缓存重跑通过。
- 「每日至多 1 条」互斥不变量：switch 结构未变，仅新增 Weekly 分支的 NORMAL 态放行，不破坏互斥。
- ClearStreak 收窄为 `WATCH ∧ SummaryWeekly ∧ !AnyTrigger`、Trends 收窄为 `SummaryMonthly ∧ NORMAL`——NORMAL 周报不拉 21 观测窗口，评估成本不变（设计 §3.4，TestBuildNotifyContextNormalWeekly 断言 Trends nil ∧ ClearStreak 0）。

### 测试质量 — PASS
- 断言真实性：TestSummaryKind 用 crisis.SummaryWeekly/Monthly/None 精确相等断言（非布尔），可判别三值漂移。
- 两半判别：TestBuildNotifyContextClearStreakConditions 逐条独立否定三条件（否 WATCH / 否 Weekly / 否 !AnyTrigger）均留 ClearStreak=0。
- 旧名彻底迁移：全仓 grep SummaryDue/summaryDue 零残留；go vet 通过。

### 文档一致性 — PASS
- 运维手册 §2 频率表新增 NORMAL 周报行（无退出进度）+ 撞日只发月报规则；§5 排障「NORMAL 非月初静默」已改为「非周一静默（NORMAL 非月初）」。
- 「退出进度 n/20」「WATCH→NORMAL 20 天」与生产 configs/crisis-monitor.yaml `watch_exit_days: 20` 一致（测试中的 25 系防硬编码异值注入锁，非矛盾）。

### 覆盖率（AD-4 文件级口径复核）— PASS
- cmd/atlas/crisis.go 修改函数：summaryKind 100%、isFirstTradingDayOfMonth 100%、buildNotifyContext 93.2%。整包欠账全来自本 Sprint 未触碰的 CLI 入口（AD-6 已机制处置，dev_minimum 已恢复 80）。

---

## 第二轮：跨视角对抗（含跨模型）

config capabilities：codex_cli=true（实测 `which codex` 命中）、gemini_cli=false。**已启用真跨模型对抗**——codex exec（read-only sandbox）独立复核核心 diff。

### 跨模型（codex / GPT）— 未发现真实缺陷
独立确认：撞日分支顺序正确、月报分支先于周报、退出进度仅 WATCH 拼接、NORMAL 周报不组装 Trends、WATCH 退出进度条件收窄为 SummaryWeekly 语义正确、测试覆盖四类关键场景。结论与 Claude 侧一致。

### Skeptic（逻辑/边界/隐含假设）— 无 high-severity
双层守卫无语义漂移空间：cmd `summaryKind` 保证 WATCH 永不返 Monthly、Brewing/Crisis 永返 None；Messages switch 的状态守卫为纯防御性冗余（即便未来调用方错配 Summary 与 state 也不会误渲染），非分歧。周一撞变更日已由 switch 首分支 Transitioned 兜底。

### Architect（设计/扩展/依赖）— 无 high-severity
忠实实现设计方案 A（cmd 一处判日、渲染纯函数），否决了渲染层重复日历逻辑（方案 B）与靠 Trends 空判区分月/周报（方案 C）。三值枚举定义于 crisis 包、cmd 单向依赖导出名，无循环依赖。

### Minimalist（过度设计/可删代码）— 仅 1 处 INFO
- [INFO] notify.go:38,40 两个 switch case 的 `res.State ==` 守卫，在 cmd `summaryKind` 已按状态收敛 Summary 的前提下属防御性冗余。建议保留（防御未来调用方错配，成本可忽略），不构成缺陷、无需改动。

---

## 问题清单
| 级别 | 描述 | 位置 |
|---|---|---|
| INFO | switch 状态守卫防御性冗余，建议保留 | internal/crisis/notify.go:38,40 |

CRITICAL: 0 · WARNING: 0 · INFO: 1（无需返工）

## 结论
所有设计产品决策（撞日只发月报、NORMAL 周报无退出进度行）被忠实实现；WATCH 周报零回归；双层守卫无漂移；跨模型复核一致。**建议进入最终验收。**
