# 架构决策记录 — Sprint 022（crisis replay report）

## AD-1 暖机语义统一（设计 v1.1，需求内决策，沿用）
既有 replay 冷启动改为从库内最早 vix 观测日暖机推进；全期窗口逐字节不变（黄金对照），
非全期窗口期初态变化属预期内行为变化。

## AD-2 Sender 接口不动
`SendDocument` 为 telegram 追加方法，crisis 包不感知；cmd 层 `documentSender` 类型断言，
断言失败降级为「总结尾附文件路径」。

## AD-3 Task 1 跨 2 个 package（Realistic Scope 例外）
引擎（internal/crisis）与既有 replay 重构（cmd/atlas）必须同任务落地：黄金对照回归
（`TestExecuteCrisisReplay*` 逐字节）要求引擎语义与 cmd 输出一次性对齐，拆开会造成
中间态不可验证。用户确认的计划即按此边界拆分。scope 互斥由 DAG 保证
（TASK-006 依赖 001，永不并行同写 cmd/atlas）。

## AD-4 机制降级（沿用项目 memory）
- Go validator 缺失 → Leader 手工做 DAG 无环/wave 序/scope 互斥/单 owner 校验。
- `arcforge-write.sh` 缺失 → 状态写入用 `bash .claude/hooks/with-task-lock.sh <TASK-ID> <cmd>`。
- dev/test 用 Agent 工具按 package-链协作，**dev 不写 .arcforge**，状态由 Leader 单写者维护；
  SendMessage + agentId 续接做返工。
- ECC 不可用且输入已是用户确认的实施计划 → 跳过 brainstorming（无增量价值）。

## AD-5 dev 协议 v2（Sprint 019 沉淀，沿用）
TDD 全绿**先 git commit**，再跑 code-simplifier（简化改动经 Leader 复核后二次提交），
防止 simplifier 占据 dev slot 身份导致协议断裂。

## AD-6 覆盖率口径（Sprint 019 沉淀，沿用）
`cmd/atlas` 历史包 master 基线 <80%，门禁作用于本任务新增/修改文件（profile 按文件聚合语句）。

## AD-7 测试 helper 共享策略
`mkReplayDay` 按计划留在 TASK-002 的 `replay_report_test.go`，TASK-003/004 同包复用，
依赖经 DAG 表达（003/004 依赖 002 verified）而非提前抽共享文件——尊重计划原文，减少偏差。

## AD-8 工具链约束
所有 go 命令 `GOTOOLCHAIN=local` 前缀；`modernc.org/sqlite` 固定 v1.38.2；零新第三方依赖。
