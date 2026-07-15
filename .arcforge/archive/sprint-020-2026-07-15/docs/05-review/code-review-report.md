# Code Review 报告 — crisis 通知模板重写（两轮）

**审查人**：qa-agent-1（第一轮常规 + 第二轮纯 Claude 三视角对抗降级：Skeptic / Architect / Ops-Trader）
**范围**：feature/crisis-notify-templates（TASK-001..009，15+2 提交）
**日期**：2026-07-15
**落盘说明**：qa-agent-1 会话受写钩子降级约束，报告经消息交付、由 Leader 按单写者规则持久化（内容原样，Leader 补注两处时效性状态）。

## VERDICT: PASS（有条件）

唯一真 CRITICAL 已修复（带回归测试，QA 验证通过），无遗留 CRITICAL；其余 WARNING（Leader 裁量）/ INFO。
问题计数：CRITICAL 1（已修复）· WARNING 4 · INFO 5。

### 放行条件（Leader 补注：已满足）

1. 提交工作区的 ClearStreakDays 修复 —— **已满足**：提交 `2955906 fix(crisis): scope ClearStreakDays to in-state rows`（QA 审查快照略早于提交时刻），工作区已干净。复验由 test-agent-1 进行中。

### CRITICAL（1，已修复）

- **[C1 已修复] `ClearStreakDays` 未按 SystemState 门控**（internal/crisis/statemachine.go）。修复前统计 any_trigger=false 连续行不看状态，WATCH 周报"退出进度"会把 CRISIS/BREWING 康复尾段免触发日计入（反例：CRISIS×18(false)+WATCH×1 → 显示 19、真实 1），严重误导"快回 NORMAL"。修复：签名加 `state SystemState` + 态内门控 + cmd 调用点 + mixed 夹具回归测试。Skeptic 独立发现、QA 与 Leader 分别独立验证成立。

### WARNING（4，Leader 裁量归类）

- **[W1] STALE 触发的被动降级无溯源**（notify_render.go renderTransition + notify.go Messages 顺序）。RED 触发指标转 STALE 可致当日被动降级，"✅ 状态解除"消息无数据断更线索，P2 速报优先级更低且后发。→ **设计反馈**（STALE 退出共振为基础方案既有行为；renderTransition 无差异行为设计 §5.1/5.2 骨架）。
- **[W2] CRISIS→WATCH"✅ + 危机状态解除"推送预览误读风险**（notify_render.go:206,168）。→ **设计反馈**（措辞为设计 §4.1 原文）。
- **[W3] WATCH→BREWING 语义句预测形对冲偏弱**（notify_render.go:164）。→ **设计反馈**（设计 §4.1 原文；建议镜像 crisisSentence 加"非预测"）。
- **[W4] diffLine 非色彩间迁移渲染"转白（原白）"自相矛盾**（notify_render.go:218-248）。→ **设计反馈**（设计 §6.5 未考虑该场景；罕见、纯文案）。

### INFO（5，记录）

- I1：7 指标元数据分散 ~6 个 switch 静默 default，加第 8 指标不编译报错（全包既有惯例）。→ backlog：建议指标元数据表 + 穷尽性断言。
- I2：buildNotifyContext 领域逻辑在 cmd 层；stateStreakDays(cmd)/ClearStreakDays(internal) 近同构；窗口 21 魔法字面量（设计 §8 既定架构 + impl 给定实现）。→ backlog：下沉纯函数 + 命名常量。
- I3：FormatIntradayAlert"疑似 carry trade"成因叙事（设计 §5.7 原文指定，"疑似"对冲，速报家族正确无页脚；ops 视角曾评 CRITICAL，QA 依设计基线有据降级）。→ 设计反馈。
- I4：P2"退出共振计数"内部术语。→ 设计反馈（与 W1 同根）。
- I5：semanticSentences map/switch 的 %d 对应无穷尽性保护（当前 3 句与 switch 精确对齐，验证无 %!d(MISSING)）。→ backlog 留意。

### 两轮小结

- **第一轮（常规）**：build/vet/gofmt/覆盖率全净；装配矩阵优先级、语义句表、StateDays 双语义、diffLine 精度裁决、页脚归属、渲染纯函数纪律逐条核对扎实，0 新增 CRITICAL。六项已裁决事项执行质量确认无误。
- **第二轮（对抗）**：Skeptic 命中 C1 + W4 + I5，另验证 sparkline 无除零/越界、stateRank 相等分支不可达、装配矩阵、StateDays+1 语义无问题；Architect 确认纯函数纪律与 8 条可达转移穷尽覆盖，产出 I1/I2；Ops-Trader 产出 W1/W2/W3 + I3/I4，并逐行核对页脚归属无误挂/漏挂、禁词零出现。

### verdict 定性说明

非 CONTESTED：ops 视角曾将 I3/W1 评 CRITICAL，QA 依设计基线 + 可达性做出有据降级（设计指定措辞的忠实实现 / 既有行为上有 P2 协同的罕见文案缺口），属有理由的 severity 裁定。设计反馈类条目（W1–W4、I3、I4）由 Leader 汇入 final-report 提交用户决策是否立设计 v1.1。
