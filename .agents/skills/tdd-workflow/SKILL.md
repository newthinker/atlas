---
name: tdd-workflow
description: 严格的 TDD 开发工作流（RED→GREEN→REFACTOR），测试由完成标准 done_criteria 驱动。Dev Agent 开发任务时使用。
---

# TDD 工作流

## 强制约束
- 禁止在没有对应测试的情况下编写功能代码。
- **测试必须由 Leader 定义的完成标准 (done_criteria) 驱动，不能凭空编写。**
- 每个功能必须经过 RED → GREEN → REFACTOR 三阶段。
- 测试文件和实现文件必须在同一次任务中完成。

> 若宿主装有 ECC 的 `tdd-workflow` skill / `tdd-guide` agent，优先复用其 Red-Green-Refactor 强制；
> 本 skill 提供等价的内置流程作为降级方案。两者都要叠加 Arcforge 的 DoD 驱动 + 自检。

## RED 阶段
1. 读取任务的 `done_criteria` 字段。
2. **逐条将完成标准转化为测试用例**：
   - `functional` → 核心功能测试
   - `boundary` → 边界条件测试
   - `error_handling` → 错误路径测试
   - `non_functional` → 性能/安全基准测试
3. 编写测试代码（含表驱动测试）。
4. 运行测试 → 必须失败。若通过了，说明测试没有正确覆盖新功能，需检查。
5. **自检点**：打印完成标准↔测试用例映射，确认无遗漏。

## GREEN 阶段
1. 编写最小实现让测试通过。
2. 运行全部测试（不仅新测试）。
3. 全部通过才进入下一阶段。

## REFACTOR 阶段
1. 优化实现代码（消除重复、改善命名、提取公共逻辑）。
2. 每次修改后运行测试，保持全绿。

## 覆盖率检查
任务完成前：读取 `arcforge.config.json` 的 `coverage.dev_minimum`，运行覆盖率命令，
不达标则补充测试后再报告完成。TaskCompleted hook 会对变更 package 做硬性校验。
