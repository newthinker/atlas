# 架构决策（sprint 2026-09-27 银行指标监控）

| AD | 决策 | 理由 |
|---|---|---|
| AD-1 | 以 spec+plan 为需求源，不重跑 brainstorming | 设计已人类确认；ECC 不可用的降级路径其产物已存在 |
| AD-2 | 计划中的测试与实现草稿是 DoD 的**下限**，Leader 补的条目（孤儿需求、边界）是增量 | 计划测试已覆盖 Review Focus 五点；补的是 spec §7 未落到测试的两条 |
| AD-3 | `day()` 挪到 TASK-001 的 `internal/bank/helpers_test.go` | 解除 Task 2↔3 的测试辅助耦合，使二者并行（文件级 writes 互不相交） |
| AD-4 | 每 dev 在 `git worktree add -b task/<ID> ../wt-<ID> feature/bank-indicator-monitor` 开工；**基准是 feature 分支不是 main/master** | 本特性全程在 feature 分支，交付 PR 到 master 由人类决定 |
| AD-5 | dev 提交后通知 Leader，**Leader 串行 merge 进 feature 分支，merge 后 dev 才回主仓库转 dev_done** | 门禁在主仓库树上跑 `go test`；未合并分支上的提交对门禁不可见（atlas M1c-3a 实证） |
| AD-6 | 提交主题 `feat(TASK-00x): …`（项目约定），正文可写 bank；结尾附 Co-Authored-By | 门禁按 TASK-ID grep 提交主题 |
| AD-7 | TASK-006 的 sender 构造做成包级可注入变量（如 `bankSenderFactory = buildBankSender`），`bankExit` 同理 | 让「--dry-run 不构造/不调用 Sender」与「部分失败 exit 2」可被测试直接观测（spec §7） |
| AD-8 | TASK-006 `coverage_floor: 78` | 基线 78.44%（profile 求和）/ 78.6%（-func），历史同包 77–78 |
| AD-9 | 真实推送、部署到 runtime、`launchctl load` 由人类执行，不在任何 DoD 内 | 外向且不可逆；计划 Task 7 Step 6 已如此约定 |
| AD-10 | 只新建文件，不改现有函数 ⇒ 无需对现有符号做 gitnexus impact；提交前仍跑 `detect-changes` | 计划 Global Constraints；项目 CLAUDE.md 提交前要求 |
