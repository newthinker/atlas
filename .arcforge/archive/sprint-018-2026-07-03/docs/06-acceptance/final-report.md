# Sprint 018 交付报告 — atlas watchlist 指标命令

> 需求: `docs/superpowers/plans/2026-07-03-watchlist-metrics-command.md`（+ spec）
> 周期: 2026-07-03 单日 ｜ 团队: Leader + dev×2 + test×1 + qa×1 + 独立 DoD reviewer×1
> 结论: **5/5 任务 accepted（4 计划任务 + QA 修复任务 TASK-005），PR #44 已创建待合并**
> https://github.com/newthinker/atlas/pull/44（单 PR，8 提交）

## 1. 交付内容

`atlas watchlist [--json] [--symbols A,B]`——离线输出 watchlist 行情/估值/百分位（CJK 对齐表格 / JSON）。

| 任务 | 交付 | commits |
|---|---|---|
| TASK-001 | internal/text 共享 CJK 宽度包（telegram 逐字迁移） | 2e824a5 |
| TASK-002 | App.SnapshotMetrics + FundamentalSource（并发保序/panic 隔离/只读）+ W2 修复（合法 0/负值如实显示） | 9e9a431, b6f59a4 |
| TASK-003 | buildCollectors 装配共享（serve 逐字迁出 + fundamentalSourceOrNil + cleanup） | 203cf8a |
| TASK-004 | watchlist 命令（表格/JSON/退出码）+ W3b 文案软化 + W4 allFailed 补判据 | e3cdb0a, 8ef8835, df769a4 |
| TASK-005 | eastmoney ChangePercent /100（QA W1，既有缺陷，web UI 同受益） | ddc543c |

## 2. 质量数据

- 覆盖率: internal/text 100% ｜ internal/app 96.1% ｜ eastmoney 87% ｜ cmd/atlas 68.6%（整包，含存量样板）
- 24 个新测试；-race 干净；全量 go build/vet/test 离线全绿；零新增第三方依赖
- 返工: TASK-002 rework=1（W2）、TASK-004 rework=1（W3b+W4），其余 0
- QA 两轮（常规+四视角对抗）: 0 CRITICAL / 4 WARNING（全部本轮修复并聚焦复核 PASS）/ 5 SUGGESTION（backlog）
- 独立 reviewer 反审: C3 误报澄清后被 QA 以新证据重开（AD-8a）——两级审查互相纠错的良性案例

## 3. 流程要点

- AD-8/AD-8a：reviewer 与 QA 对"窗口口径同源"的两轮辩证——最终事实：分析循环 per-strategy lookback（5y/3y），
  snapshot 用全局 valuation.lookback_years，默认配置巧合一致；处置为文档如实措辞（方案 b），窗口对齐留 backlog。
- TASK-003 偏离（fundamentalSourceOrNil helper）经 test-agent 等价性判定接受——"按唯一可测解交付+记录提请"模式二次实践。
- QA 补报 W4 在 TASK-004 认领后追加 fix_items 并显式通知，无丢失。

## 4. 异常披露（安全相关）

两个 dev agent 独立报告：code-simplifier 子代理的响应中出现疑似提示注入样文本（"managed policy/_reminders"类，
非来自用户或权限系统）。两次均被 dev 按边界约束忽略、复核后正常完成，未影响任何产物。
建议：留意 code-simplifier 插件的上游内容来源；如再现可暂停该插件并审查。

## 5. Backlog

- S1 snapshot errgroup gctx 取消语义注释/简化 ｜ S2 gap 摘要 stdout 取舍已记录 ｜ S3 SilenceUsage 全仓约定
- S5 config.Validate 缺 LookbackYears>=0 校验 ｜ 百分位窗口对齐 per-strategy lookback（W3 方案 a）
- 回写 spec：JSON gaps 位置消歧（AD-5 已裁内嵌）｜ Sprint 017 遗留 backlog 见其 final-report

## 6. 验收对照（spec §7）

- ✅ 离线命令可用（联网冒烟 44 标的表格对齐、降级正确、exit 0；坏 config/全失败 exit 1）
- ✅ 与分析循环装配同源（buildCollectors 共享；窗口基准已如实文档化）
- ✅ `go test ./...` 离线全绿；YAGNI 边界未引入
