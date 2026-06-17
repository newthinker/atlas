# 架构决策记录 — Telegram 信号汇总表格

## ADR-1：显示宽度自实现，不引第三方 runewidth
- **决策**：手写 `isWide` 覆盖 atlas 实际涉及的东亚宽字符区段。
- **理由**：保持「无新依赖」约束；watchlist 符号/中文名落在有限区段，全 Unicode EAW 表属过度工程。
- **代价**：极少见字符可能误判宽度 → 仅影响对齐美观，不影响正确性。可接受。

## ADR-2：缓冲在 router 层，而非 notifier 层
- **决策**：`Route` 在 `batch_notify` 下缓冲到 `Router.pending`，cycle 末 `FlushNotifications` 统一批发。
- **理由**：路由决策/冷却/执行/信号存储仍需逐信号语义；只有「通知」需要聚合。在 router 缓冲让聚合边界与「一轮分析」对齐（`runAnalysisCycle` 的 `g.Wait()` 之后）。
- **替代方案**：在 telegram notifier 内攒批——被否，无法感知「一轮」边界，且会污染其他 notifier。

## ADR-3：默认 `batch_notify=true`
- **决策**：新默认开启汇总。
- **理由**：汇总是本需求的目标体验；逐条单发是回退选项。
- **影响**：改变现网默认行为；通过 `batch_notify:false` 可即时回退（R5 有测试保障）。

## ADR-4：Task 4 跨 config+app 两包不再细分
- **决策**：保留计划原 Task 4（config + app + yaml）为单任务，尽管触及 2 个 package（轻微超出「≤1 package」指引）。
- **理由**：均为极小接线改动（各 1-2 行），且为终端任务（依赖 T2+T3，不与任何任务并发），无 scope 互斥风险；强行拆分会制造零工作量的人为依赖。
- **scope 互斥**：Task 4 单独成 wave 3，与并发任务无包重叠。

## 降级声明（capabilities）
- ECC 不可用 → 不调 `/multi-plan`，直接据已定稿 spec 拆分（spec 本身即多轮精炼产物）。
- codex/gemini CLI 不可用 → QA 跨视角对抗退化为纯 Claude 多视角。
- Go validator (`validator/cmd/arcforge-validate`) **缺失** → 跳过自动校验，改用人工 DAG/wave/scope 互斥核验（见 plan.md）。
