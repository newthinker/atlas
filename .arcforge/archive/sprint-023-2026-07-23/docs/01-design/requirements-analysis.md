# 需求分析 — Sprint: crisis NORMAL 态周报

> 日期：2026-07-23 | Leader 分析 | 需求入口：`docs/plans/atlas-macro-crisis-monitor-design.md`

## 1. 差距分析（需求文档 vs 现状）

用户以宏观危机监控总设计（v0.2）作为 /arcforge 需求入口。逐节核对代码现状：

| 设计章节 | 现状 | 证据 |
|---|---|---|
| §2 指标采集（collector/fred + yahoo 复用） | ✅ 已实现 | `internal/collector/fred/`、git log |
| §2.2 派生计算 | ✅ 已实现 | `internal/crisis/derive.go` |
| §3.1/3.2 三色规则 + 抑制 | ✅ 已实现 | `rules.go` / `suppress.go` |
| §3.3 状态机 | ✅ 已实现 | `statemachine.go` |
| §4.2 sqlite 存储 | ✅ 已实现 | `store.go` |
| §4.3 launchd 3 plist | ✅ 已部署 | `deploy/launchd/com.newthinker.atlas.crisis-*.plist` |
| §4.4 通知集成（七类消息矩阵 v1.0/v1.1/v1.2） | ✅ 已实现 | `notify*.go`、2026-07-14/15 系列提交 |
| 回测/replay 报告 | ✅ 已实现 | `replay*.go` |
| **NORMAL 态周报（2026-07-23 增量修订）** | ❌ **未实现** | 代码仍为 `SummaryDue bool`，无 `SummaryKind` |

**结论：本 Sprint 唯一待实现增量 = NORMAL 态周报**。其产品决策、方案选型、行级实施计划已定稿并已提交：
- 设计：`docs/superpowers/specs/2026-07-23-crisis-normal-weekly-report-design.md`
- 实施计划：`docs/superpowers/plans/2026-07-23-crisis-normal-weekly-report.md`（4 任务，TDD 步骤到行号）

## 2. 需求要点（摘自定稿设计）

- R1 触发：NORMAL 态评估日为周一 → 发周报；当月首个交易日 → 发月报；撞日只发月报。
- R2 路由：`SummaryDue bool` → 三值 `SummaryKind` 枚举；cmd 层一处判日，渲染层纯函数。
- R3 渲染：NORMAL 周报复用 WATCH 周报骨架，去「退出进度 n/20」行，尾行「下次周报：下周一 · 状态变更即时通知」。
- R4 成本不变量：NORMAL 周报不组装 Trends（仅月报拉 21 观测日窗口）；ClearStreak 仅 WATCH∧weekly 计算。
- R5 回归不变量：WATCH 周报输出逐字节不变；「结构化消息每日至多 1 条」互斥 switch 不变。
- R6 边界：坏日期/周末/BREWING/CRISIS → SummaryNone；枚举零值安全静默；周一假日顺延走既有「数据未齐」机制（零新增逻辑）。
- R7 文档：`docs/ops/crisis-monitor-notifications.md` §2 频率表与 §5 排障速查同步更新。

## 3. 复杂度评估

整体**简单~中等**：3 个代码文件 + 4 个测试文件 + 1 个运维文档；无新依赖、无 schema 变更、无调度变更。主要风险是 WATCH 周报逐字节回归与测试文件中 `SummaryDue` 引用点的完整替换（计划已 grep 枚举全部引用点）。

## 4. 降级路径记录

- ECC 不可用（config capabilities.ecc=false）→ 按流程应走 brainstorming 精炼；因需求已是**经评审定稿的 v0.2 设计 + 已定稿增量设计**，精炼环节判定为已完成，直接采纳定稿文档（记录于 architecture-decisions.md AD-1）。
- gitnexus 索引落后且 FTS 损坏（计划 Global Constraints 已声明）→ 引用点靠计划中的 grep 枚举；`gitnexus_detect_changes` 按降级判定标准执行。
