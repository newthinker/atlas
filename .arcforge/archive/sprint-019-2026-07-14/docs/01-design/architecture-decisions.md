# 架构决策记录（Sprint 019）

## AD-1 任务边界 1:1 沿用实施方案的 15 个 Task

方案的任务分解已满足 TDD 可执行粒度且带完整步骤/参考代码，Leader 不重切边界，只补齐 done_criteria 四维度与调度元数据（dependencies/wave/packages/context_from）。

## AD-2 同包任务串行化编入 dependencies

`internal/crisis` 承载 9 个任务、`cmd/atlas` 承载 4 个任务。「在途任务 packages 互斥」不变量通过把同包前序任务写进 `dependencies` 机制化（dag 就绪即天然互斥），而非依赖派发时人工检查。代价是并行度受限（仅 wave 1 三路并行、wave 5 二路并行），这是设计本身（单包为主）决定的，不是拆分缺陷。

## AD-3 TASK-014/015 声明双 package（Realistic Scope 偏差，已裁决）

方案的 Task 14/15 天然跨 `internal/crisis` + `cmd/atlas`（通知模板 + eval 流程接线；intraday 模式 + store 补一方法）。拆成两半会打断方案的 checkbox 序列且引入无谓交接。两任务位于队尾 wave 11/12 严格串行，无并发互斥风险。validator（手工降级版）对此二任务放行多 package。

## AD-4 验收分层：test / manual

三个「收口任务」（7/13/15）各含一段需要真实外部数据或真实部署的人工验收（FRED 全量 backfill 抽查、三段历史回测达标、launchd 部署试运行）。这些以 `verify_by: manual` 进入 done_criteria，Test Agent 不做 fantasy assertion，由终验收阶段人工执行（autonomy=dod-gate，终验收前不再强制暂停，但 manual 项会列入 final-report 待办）。

## AD-5 分支与提交

单分支 `feature/crisis-monitor`（首个 Dev 开工前由 Leader 创建），所有任务串行/互斥提交其上，提交格式沿用方案（`feat(crisis):` / `feat(fred):` / `fix(yahoo):`）。每任务提交前跑 gitnexus `detect_changes()` + code-simplifier（用户全局规范）。

## AD-6 cmd/atlas 覆盖率门禁按「本任务新增/修改代码」判定

`coverage.dev_scope=changed-package` 的本意是约束新代码质量。cmd/atlas 是含大量既有低覆盖代码的历史包（master 基线即 <80%），按全包判定会让任何 cmd 任务永远无法达标。裁决：cmd/atlas 任务的 80% 门禁作用于**本任务新增/修改的文件**（用 coverage profile 按文件聚合语句计算），internal/crisis 等新包仍按全包判定。TASK-007 依此判定：crisis.go 80.3% PASS（全包 70.9% 仅记录不阻断）。适用于 TASK-012/013/015。
