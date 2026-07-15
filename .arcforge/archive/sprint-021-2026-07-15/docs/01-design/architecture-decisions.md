# 架构决策记录 — sprint-021（通知 v1.1）

## 上游已冻结决策（设计 v1.1，用户在 brainstorming 中逐项确认）

1. 修订范围 = 全部 6 条（W1/W2/W3/W4/I3/I4 → R1–R6）。
2. W1 双侧增强：降级消息条件警示行 + P2 断更前状态措辞（数据全部来自既有 PrevDay/NewStale，零新管道）。
3. I3 去归因 + 短免责（内联限定语，速报家族仍不挂页脚常量）。
4. W2 条件符号：✅ 仅异常区为空；非空 🔽「状态回落」。
5. 4 条新增设计原则（非预测从句显式化 / ✅ 仅限全清 / 判定输入变化必溯源 / 术语外化）。

## 本次拆分决策（Leader）

### AD-1: 3 任务全串行（T1→T2→T3）

T1/T2 追加写同一对文件（notify_render.go/_test.go）；T3 的全家族页脚/禁词测试断言 T1/T2 的
最终文案。dev×1 + test×1 即可，无并行收益。

### AD-2: TASK-003 允许跨 2 个 package（测试连锁原子性）

FormatIntradayAlert 格式串变更会立即使 cmd/atlas/crisis_test.go 的 "carry trade" 断言失败——
生产变更与 cmd 测试断言适配必须同一提交（同 sprint-020 AD-1 理由），packages 声明两包。
cmd 侧仅测试文件、无生产代码。

### AD-3: 继续在 feature/crisis-notify-templates 分支上实施

v1.0 实现尚未合并，v1.1 是其文案修订——同分支连续提交，合并时一体交付，避免堆叠分支。

### AD-4: 降级路径沿 sprint-020

validator 人工核查、with-task-lock.sh 原子写、gitnexus 门禁必要时 Leader 代跑、
QA 对抗轮纯 Claude 跨视角。coverage_minimum：本 Sprint 任务不动 cmd 生产代码，
T3 涉 cmd 仅测试——仍写 coverage_minimum=35 以防 hook 按包门禁（先例）。
