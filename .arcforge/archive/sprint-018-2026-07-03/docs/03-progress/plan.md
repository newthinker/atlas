# Sprint 018 进度 — atlas watchlist 指标命令

> 需求: `docs/superpowers/plans/2026-07-03-watchlist-metrics-command.md`（+ 上游 spec）
> 调度: dag + 单 PR；validator 缺失 → Leader 手工校验（沿用 017 降级路径）
> 更新: 2026-07-03（初始化）

## 当前阶段

**Step 5 — wave1 并行开发中**（dod-gate 2026-07-03 人工放行，dev×2 + test×1）。
分支: `feature/watchlist-metrics-command`。反审闭环见 AD-8（C3 误报澄清、B3/B4/B5/B10 补入 DoD）。
（注：teammate 可寻址名带 -2 后缀：dev-agent-1-2 / dev-agent-2-2 / test-agent-1-2，自我身份不带。）

## 任务看板

| 任务 | 标题 | wave | 依赖 | 状态 | owner | rework |
|---|---|---|---|---|---|---|
| TASK-001 | internal/text 共享宽度包 | 1 | — | **verified** ✓（2e824a5，text 100%） | dev-agent-1 | 0 |
| TASK-002 | App.SnapshotMetrics + FundamentalSource | 1 | — | **verified** ✓（9e9a431，app 95.8%，AD-8 三增补实证） | dev-agent-2 | 0 |
| TASK-003 | buildCollectors 提取（serve 重构） | 2 | 002 | **verified** ✓（203cf8a，逐字迁移实证，helper 偏离判定等价接受） | dev-agent-1 | 0 |
| TASK-004 | watchlist 命令与渲染 | 3 | 001,002,003 | **verified** ✓（e3cdb0a，8/8 + 收口门禁全绿） | dev-agent-2 | 0 |

> **Step 6 QA verdict: PASS 带 3 WARNING（0 CRITICAL / 4 SUGGESTION），review_fix 第 1 轮进行中**：
> - W1 eastmoney ChangePercent 未除 100（既有缺陷，裁决本轮修）→ **TASK-005**（新建，dev-agent-1）
> - W2 positivePtr 掩盖合法 0/负值 → TASK-002 review_fix（rework=1，dev-agent-2）
> - W3 窗口口径分叉 + docstring 过度承诺 → 方案(b) 文案软化 → TASK-004 review_fix（rework=1，dev-agent-2，在 002 后做）
>   ——**AD-8a 重开修正 C3**（原"误报"裁决前提有误：SinceInceptionBars 仅 lookback==0 时使用）
> - S1~S4 → backlog（gctx 语义注释、gap 摘要 stdout 取舍、SilenceUsage 全仓约定、并发调用需锁提示）
> QA 报告: 05-review/qa-review.md。

| 修复任务 | 内容 | 状态 |
|---|---|---|
| TASK-005 | eastmoney ChangePercent /100（W1） | dev_done→复验中（ddc543c） |
| TASK-002-fix | 合法 0/负值如实显示（W2） | dev_done→复验中（b6f59a4） |
| TASK-004-fix | help 文案软化（W3b）+ allFailed 补判据（W4） | in_progress dev-agent-2 |

> 异常记录：dev-agent-1 报告一次 code-simplifier 子代理响应夹带疑似注入文本（"managed policy/_reminders"），
> 已正确忽略并重跑正常。交付总结向用户披露。

> QA 计数修订：CRITICAL 0 / WARNING 4（+W4 allFailed 漏检 PB/DYR）/ SUGGESTION 5（+S5 负 lookback 校验，入 backlog）。
> QA 另排除对抗轮"--symbols 大小写"疑点（非 bug）。

## 交付组织

单 PR，分支 `feature/watchlist-metrics-command`；提交格式 `<type>(watchlist-cmd): <描述>`。
wave1 两任务并行（包不相交）→ wave2 → wave3 → 门禁（detect_changes + 全量离线测试）→ PR → QA → 交付。

## 里程碑

- [x] Step 1 环境检查（沿用 017 已核实降级路径；运行时目录已由归档重置）
- [x] Step 2 需求分析（计划已 self-review，关键代码事实合并后复核无漂移）
- [x] Step 3 任务拆分 + DoD + 追溯矩阵（reviewer 反审中）
- [ ] Step 4 dod-gate 人工确认
- [ ] Step 5 开发（wave1 T1∥T2 → T3 → T4）
- [ ] Step 6 QA 两轮审查
- [ ] Step 7 交付归档
