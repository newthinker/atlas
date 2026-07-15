# 架构决策记录 — crisis 通知模板 Sprint

## 上游已冻结决策（实施方案"补充决策"1–8，执行者不得重新决策）

1. `NotifyContext` 增加 `StaleLastObs map[string]string`；cmd 用 `store.LatestObservation` 组装；
   通道名写死 move/usdjpy→Yahoo、其余→FRED。
2. sparkline：>7 观测分 7 连续桶取均值后 min-max 八阶归一；≤7 逐点；全平全 `▄`；空窗口省略整行。
3. `persistLookbackObs=30` 为显示统计窗口（非规则阈值），留在代码。
4. 5y 分位片段仅 vix/move/hy_oas/t10y2y/nfci 显示（`showPct5y` 写死）。
5. "7 指标全绿："标题仅当异常区为空且 7 指标全 GREEN；存在 ⚪ 时用"其余指标："。
6. `StateDays`：变更消息 = 前状态持续评估日数；日报/周报/月报 = 当前状态含当日。
7. usdjpy 周跌片段触发：`WowOK && Wow <= amber_wow_pct`（渲染层读 cfg）。
8. `ClearStreak` 含当日：`ClearStreakDays` 只统计历史行，当日由 cmd 层补。

## 本次拆分新增决策（Leader）

### AD-1: TASK-009 允许跨 2 个 package（Realistic Scope 例外）

Task 9 是唯一签名切换点：`internal/crisis`（notify.go 重写）与 `cmd/atlas`（调用切换）
必须一次提交，否则存在全仓不可编译的中间提交点（违反实施方案 Global Constraints）。
拆成两个任务会破坏原子性 → 保留单任务，`packages` 声明两个包。
风险缓解：T9 位于 DAG 末端、独占在途，scope 互斥不受影响。

### AD-2: 任务链大幅串行（scope 互斥的必然结果）

T1–T7、T9 全在 `internal/crisis`，同包任务不可同时在途；且 T4→T5→T6→T7 追加写
同一对文件（notify_render.go/_test.go），存在真实文件依赖（T6/T7 消费 T5 的 notifyFooter）。
唯一并行机会：T8（cmd/atlas）与 T5/T6/T7 并行（T8 依赖 = T2+T4）。
→ Dev Agent 数量 2（1 主链 + 1 承接 T8），Test Agent 1。

### AD-3: 任务粒度沿用实施方案的 9 任务划分

实施方案已按"每任务一次提交、全仓可编译"切好边界，且每任务自带失败测试→实现→验证步骤。
重新切分只会引入与文档的对齐成本。T1 预计改动 6 个文件（4 源 + 2 测试）轻微超出
"≤5 文件"标度，但实际 diff 很小（加 3 字段 + 2 助手），不再拆分。

### AD-4: 降级路径

- validator 缺失 → Leader 人工核查任务图（结果记录于 02-plan/requirement-dod-matrix.md）。
- arcforge-write.sh 缺失 → owner 直接原子写 + `with-task-lock.sh` 临界区（认领协议不变）。
- ECC/codex/gemini CLI 不可用 → QA 对抗轮退回纯 Claude 跨视角。
