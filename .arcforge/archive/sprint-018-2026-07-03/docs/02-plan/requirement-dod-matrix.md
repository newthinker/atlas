# 需求 ↔ DoD 双向追溯矩阵 — Sprint 018

> 需求来源: 实施计划 `docs/superpowers/plans/2026-07-03-watchlist-metrics-command.md`（Task 1-4 + Global Constraints）
> 上游 spec: `docs/superpowers/specs/2026-07-03-watchlist-metrics-command-design.md`

## 正向：需求 → 任务/DoD

| 需求条目 | 任务 | 覆盖 DoD |
|---|---|---|
| 计划 T1: text 包迁移（导出名/逐字函数体/telegram 改引用/删旧文件） | TASK-001 | functional#1-2, boundary#1 |
| 计划 T2: FundamentalSource/SetFundamentalSource/SymbolMetrics/SnapshotMetrics 签名 | TASK-002 | non_functional#1 |
| 计划 T2: 全路径/降级/crypto 门控/过滤/panic 隔离/全失败 六用例 | TASK-002 | functional#1-4 |
| 计划 T2: 只读语义（不产信号不通知） | TASK-002 | boundary#1 |
| 计划 T3: 装配段逐字迁移 + serve 行为零变化 | TASK-003 | boundary#1, non_functional#1 |
| 计划 T3: 新增 FundamentalSource 注入（typed-nil 防护）+ cleanup 关 qlib 句柄 | TASK-003 | functional#2, non_functional#2 |
| 计划 T3: EmptyConfig/Defaults 两用例 | TASK-003 | functional#1 |
| 计划 T4: 表格 CJK 对齐/—/gaps 摘要/JSON null/--symbols 校验/空表/全失败退出码 七用例 | TASK-004 | functional#1-3, boundary#1, error_handling#1 |
| 计划 T4: stdout/stderr 分离、rootCmd 注册 | TASK-004 | non_functional#1 |
| Global: 离线测试全绿、零新增依赖、不改依赖版本 | 全部任务 | 各 non_functional |
| Global: gitnexus_impact/detect_changes、code-simplifier、提交格式 | Leader 流程 + Dev prompt | 波次门禁与工作方式 |
| spec §6 YAGNI 边界 | 全部任务 | description 边界声明 |
| spec JSON 歧义 | 计划已裁定（AD-5），TASK-004 functional#2 | — |

## 反向：DoD → 需求

逐条核对 4 任务全部 DoD 均回溯至计划 Task 1-4 步骤或 Global Constraints；
引申型 DoD 标注：TASK-002 non_functional#2 的 -race（计划未显式要求，errgroup 并发组装的健壮性引申）；
TASK-003 functional#2 的 typed-nil 用例（计划新增①的注记转测试）。

## 机器检查结论

- 孤儿需求: 0（计划四 Task 全部步骤与 Global Constraints 均有落点）
- 凭空 DoD: 0（2 条引申已标注来源）
- DoD 条数: T1=4 / T2=7 / T3=5 / T4=7，全部 ≤8 ✓
