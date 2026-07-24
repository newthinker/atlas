# 独立 reviewer 反审处置记录(Prism M1)

> reviewer 判定: NEEDS_CHANGES(3 必改 + 3 建议);处置后 validator 复跑通过。

## 必改项处置(全部已改,jq 直读核实)

1. TASK-009 未知 symbol→404 不可测 → boundary 口径改为「测试 fake 的 Series 须对未知 symbol 返回 error 使该分支可测」。
2. Context Checkpoint 注释(全局约束 line 14)未传导 → TASK-003/004/005 各补一条 verify_by:review;TASK-002 并入合并审查项(保持总条数 ≤8)。TASK-006~009 的测试骨架已内嵌该注释,由 Test Agent 按全局约束核对(spawn prompt 注入),不再增列。
3. TASK-002 error_handling → 改「Open 自动创建多级不存在的父目录(嵌套路径用例)」(test);schema 初始化失败分支降为 review(合并审查项 b)。

## 建议项决策(Leader 定夺,不改任务)

- **T8 跨 4 包 / T9 6 文件 2 包**: 显式接受。接线任务硬拆会产生琐碎子任务,依赖边 T7→T8→T9 已保证同包不并发在途,validator scope 互斥通过。
- **T1 LowPct==0 判空语义**: 保持计划行为。Go/mapstructure 零值即「未配置」是本仓库既有惯例;LowPct=0(永不低估)非设计 §5.3 要求的语义,不为其破坏惯例。
- **T8 status 等值边界(p==15/85→neutral)**: 不入 DoD;Dev 写表驱动测试时可顺手覆盖。
