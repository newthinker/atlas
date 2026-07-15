# DoD 独立反审报告（Crisis 通知模板实施方案）

- **审阅对象**：`.arcforge/tasks/TASK-001.json` ~ `TASK-009.json`
- **需求来源**：`docs/plans/2026-07-14-crisis-notification-templates-impl.md`（v1.0，含 Global Constraints、8 条补充决策、§9 测试要点、自查记录）
- **审阅方式**：先只读需求文档独立推导每任务应覆盖的 functional/boundary/error_handling/non_functional 要点，再与 Leader 生成的 done_criteria 比对（第一步完成前未读任务 JSON）。

## 总体结论：**PASS_WITH_NOTES**

DoD 与需求文档贴合度高：8 条补充决策全部落到具体任务、§9 测试要点 1–6 全部有对应断言、Global Constraints 大部分进了 DoD。**无孤儿功能需求、无凭空 DoD、无不可测 DoD、无任务图硬错误。** 发现 6 条问题，均为 NOTE 级（覆盖缺失/边界缺失/图说明），不阻塞派发。最突出的是 Global Constraint「每任务提交前跑 detect_changes + code-simplifier」完全缺席所有 DoD。

## 逐任务判定

| 任务 | 判定 | 发现的问题 |
|---|---|---|
| TASK-001 | PASS_WITH_NOTES | omitempty 负向路径（GREEN/零值行省略 persist_days/wow/wow_ok）未断言（P2）；DoD 只要求 `go test ./internal/crisis/` 而非 Global Constraint 的 `go build ./...` 全仓（P5） |
| TASK-002 | PASS | 覆盖完整（func/空历史/max 上限/坏 JSON 保守中断/RecentSystem 上抛），与独立清单一致 |
| TASK-003 | PASS | layerName/emoji/tag/formatReading/trendArrow/sparkline 及边界（全平▄/≤7逐点/空串/showPct5y）全覆盖 |
| TASK-004 | PASS_WITH_NOTES | monthDay 非 10 长度降级路径未断言（P3，minor）；其余（indicatorLine 各分支、决策5/7、splitZones 排序）全覆盖 |
| TASK-005 | PASS | 8 转移语义句 + %d 注入 + YAML 调参跟随 + 升/降级前缀 + 未知转移空串全覆盖；禁词就地检查缺席但设计明确归 Task 9 兜底，可接受 |
| TASK-006 | PASS | diffLine 状态迁移优先/读数仅异常区/无变化文案、renderDaily/renderWeekly 首行尾注全覆盖 |
| TASK-007 | PASS_WITH_NOTES | nextMonthlyDue 日期解析失败降级路径未断言（P4，minor）；月报不分区/空窗口省略行/通道名/无页脚全覆盖 |
| TASK-008 | PASS | PrevDay 昨日行/StateDays 双语义/NewStale 去重/StaleLastObs/ClearStreak 含当日/Trends 按需、store 错误上抛全覆盖 |
| TASK-009 | PASS | 装配优先级矩阵/FormatIntradayAlert/cmd 接线时序/删除旧符号/禁词全 7 类/页脚归属/4096 上限/impact 全覆盖 |

## 问题清单

### P1 — detect_changes + code-simplifier 全任务缺席【类型：覆盖缺失 / moderate】
- **任务**：全部（尤其 TASK-001、TASK-009）
- **描述**：Global Constraints（impl 文档第 21 行）明确「每个任务提交前：跑 `gitnexus_detect_changes()` 核对影响面；按用户全局规范运行 code-simplifier」。`gitnexus_impact` 前置约束进了 TASK-001/009 的 non_functional，但它的姊妹约束 detect_changes + code-simplifier 一个任务都没进。用户全局 CLAUDE.md 亦强制 commit 前跑 code-simplifier。impact 进 DoD 而 detect_changes 不进，是内部不一致。
- **建议修正**：至少给 TASK-001/009（改已有 symbol 的两个任务）的 non_functional 补一条 `verify_by: review`：「提交前已跑 gitnexus_detect_changes 核对影响面且已跑 code-simplifier」；其余任务可在 plan.md 头部统一声明为全局门禁，避免逐条冗余。

### P2 — TASK-001 omitempty 零值省略未断言【类型：边界缺失 / minor】
- **任务**：TASK-001
- **描述**：func#3 只断言 sofr RED 行含 `"persist_days":9`、usdjpy RED 行含 `"wow"/"wow_ok"`（正向存在），未断言 GREEN/零值行**省略**这三个 `omitempty` 字段（负向路径）。注意 `Wow float64 json:"wow,omitempty"` 在 wow 恰为 0 但 WowOK=true 时会丢 `"wow":0` 只留 `"wow_ok":true`——读回默认 0 语义一致，但值得一条断言锁死表示。
- **建议修正**：boundary 增一条「全绿指标行的 detail JSON 不含 persist_days/wow/wow_ok 键」。

### P3 — TASK-004 monthDay 降级路径未断言【类型：边界缺失 / minor】
- **任务**：TASK-004
- **描述**：impl 中 `monthDay` 对 `len(date)!=10` 原样返回（防御 fallback），DoD 只测 `"2026-07-14"→"07-14"`，未覆盖非法长度输入。
- **建议修正**：boundary 增一条「非 YYYY-MM-DD 输入 monthDay 原样返回」（低优先）。

### P4 — TASK-007 nextMonthlyDue 解析失败降级未断言【类型：边界缺失 / minor】
- **任务**：TASK-007
- **描述**：impl 中 `nextMonthlyDue` 日期 Parse 失败返回「下月首个交易日」，DoD 未覆盖该 fallback。
- **建议修正**：boundary 增一条「日期不可解析时月报尾注降级为『下月首个交易日』」（低优先）。

### P5 — TASK-001~007 未按 Global Constraint 要求全仓 `go build ./...`【类型：覆盖缺失 / minor】
- **任务**：TASK-001 ~ TASK-007
- **描述**：Global Constraints（第 23 行）要求「每个任务结束时 `go build ./...` 必须通过」。TASK-001~007 的 non_functional 只写 `go test ./internal/crisis/`（仅编译本包）。虽然这些任务不碰 cmd、旧 Messages 签名不变，全仓通常仍可编译，但与约束字面不符，无显式护栏。
- **建议修正**：把 TASK-001~007 的收尾断言从 `go test ./internal/crisis/` 提升为 `go build ./... && go test ./internal/crisis/`（与 TASK-008/009 一致）。

### P6 — wave 1 同包三任务并行标注误导【类型：图说明 / 非阻塞】
- **任务**：TASK-001/002/003
- **描述**：三者 `wave=1` 且 `packages=["./internal/crisis"]`。validator 的 scope 互斥（在途任务 package 非空且互斥）会强制它们串行派发，故「wave 1 并行」名不副实——但这与 team-lead「Task 1–7 同包串行」意图**一致**，且串行是保守安全侧，**非缺陷**。
- **依赖图正确性单独核实（均通过）**：DAG 无环；`本任务.wave > max(依赖.wave)` 逐条成立（T4:2>1, T5:3>2, T6:4>3, T7:5>4, T8:3>2, T9:6>5）；context_from 闭合（T6 的 [4,5]、T7 的 [4,5,6] 均为祖先）；**同文件 `notify_render.go` 的写入由 4→5→6→7 依赖链正确串行化**，不会并发写；TASK-008 在 `cmd/atlas` 声明可与 5/6/7 并行，与兼容性策略「Task 8 并行」一致；TASK-009 跨两包原子切换（AD-1 例外）声明清晰。
- **建议修正**：无需改图；如需消除误导，可在 plan.md 注明「wave 1 因同包 scope 互斥实际串行」。

## 覆盖性交叉核对（无遗漏项）

- **8 条补充决策**：1→T7(通道)+T8(StaleLastObs)、2→T3、3→T1、4→T3+T4、5→T4、6→T4/T5/T6/T8、7→T4、8→T2+T8。全部有归属。
- **§9 测试要点 1–6**：1→T9、2→T4、3→T3+T7、4→T6、5→T8、6→T5。全部覆盖。
- **Global Constraints**：GOTOOLCHAIN=local（各任务 test）、禁词（T9）、页脚归属（T5/6/7+T9）、YAML 阈值+%d 注入（T5）、persistLookbackObs=30（T1）、发送失败记 stderr（T9）、impact 前置（T1/T9）均已覆盖；**唯 detect_changes+code-simplifier 缺席（见 P1）**、全仓 build 对 T1–7 未强制（见 P5）。
- **凭空 DoD 扫描**：未发现不对应需求文档的 DoD 条目；DoD 条目基本逐条源自 impl 文档的测试代码与设计小节。
