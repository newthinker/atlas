---
name: test-verification
description: 按完成标准逐条验证任务质量（Reality Checker 模式），产出 done_criteria 覆盖矩阵。Test Agent 验证任务时使用。
---

# 测试验证 Skill

## 心智模型：默认 NEEDS WORK
所有判定必须有**实际命令输出**作为证据。拒绝没有真实输出的 PASS。

## 验证步骤

### 1. done_criteria 逐条核对（最重要）
对被验证任务，逐条检查 `done_criteria`，为每条找出对应测试用例，填入覆盖矩阵：

| # | 维度 | 完成标准 | 对应测试 | 判定(PASS/FAIL/未覆盖) | 证据 |
|---|------|---------|---------|------------------------|------|

任一标准无测试覆盖 → 整体判定不通过。

### 2. 测试有效性审查
- 断言是否空洞（`assert true` 之类）。
- mock/stub 是否过度，是否掩盖真实问题。
- 边界/错误路径是否真的被触发。

### 3. 覆盖率（复用，不重跑全量）
优先读取 TaskCompleted hook 已生成的 `coverage.out`/报告；仅对可疑点补跑。

### 4. 回归与集成
运行 `go test ./...` 确认未破坏既有测试；检查跨任务接口兼容。
如可用，调用 ECC `e2e-runner` 做集成测试。

## 产出
- `.arcforge/docs/04-test/TASK-{id}-test-report.md`：含覆盖矩阵 + 证据 + 结论。
- 不通过：原子写 task status=`rejected` + `reject_reason`。
- 通过：原子写 task status=`verified`，并确保 `discovery` 文件已由 Dev 写好。
- 全部任务验证后汇总 `.arcforge/docs/04-test/coverage-report.md` 与 `dod-coverage-matrix.md`。
