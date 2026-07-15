# 需求 ↔ DoD 双向追溯矩阵

**需求基线**：`docs/plans/2026-07-14-crisis-notification-templates-impl.md`（设计 v1.0 条目经其"自查记录"表映射）
**生成**：2026-07-14 Leader

## 正向：需求 → DoD

| 需求条目（设计章节） | 任务 | DoD 锚点 |
|---|---|---|
| §2 消息类型矩阵（7 类、前缀、页脚归属） | TASK-009 | functional[1]（装配矩阵）、non_functional[1]（页脚归属全家族测试） |
| §3 首行规范（MM-DD、前缀、持续日数） | TASK-005/006/007/009 | 各 functional 首行断言 |
| §4.1 语义句查表 + %d 配置注入 | TASK-005 | functional[1][2] |
| §4.2 页脚常量 | TASK-005 | non_functional[1]；TASK-009 non_functional[1] |
| §5.1/5.2 状态升级/降级 | TASK-005 | functional[3][4] |
| §5.3 日报 + §6.5 较昨日差异行 | TASK-006 | functional[1][2]、boundary[1][2] |
| §5.4 月报 + §6.4 sparkline/箭头 | TASK-003 functional[4][5]、TASK-007 functional[1][2][3] |
| §5.5 周报 + §6.6 退出进度 | TASK-002 全部、TASK-006 functional[3]、TASK-008 functional[4] |
| §5.6 P2 速报（滞后/通道/去重/降级） | TASK-007 functional[4]、boundary[2]；TASK-008 functional[3] |
| §5.7 盘中速报 | TASK-009 functional[2] |
| §6.1 emoji/非色彩说明 | TASK-003 functional[2] |
| §6.2 层名/冰山层序/分区 | TASK-003 functional[1]、TASK-004 functional[3][4] |
| §6.3 数值格式/tag/持续/周跌 | TASK-003 functional[3]、TASK-004 functional[2]、TASK-001 functional[1][2] |
| §7 禁词/4096/纯文本/发送失败语义 | TASK-009 non_functional[1]、functional[3] |
| §8 NotifyContext/新字段/cmd 组装职责/detail JSON | TASK-001 functional[3]、TASK-004 functional[1]、TASK-008 全部、TASK-009 functional[3] |
| §9 测试要点 1–6 | 1→T9、2→T4、3→T3/T7、4→T6、5→T8、6→T5（各任务对应 DoD 已覆盖） |
| 补充决策 1（StaleLastObs） | TASK-004 functional[1]、TASK-007 boundary[2]、TASK-008 functional[3] |
| 补充决策 2（sparkline 分桶） | TASK-003 functional[5]、boundary[1] |
| 补充决策 3（persistLookbackObs=30） | TASK-001 functional[1]、boundary[1] |
| 补充决策 4（showPct5y） | TASK-003 boundary[2] |
| 补充决策 5（全绿标题条件） | TASK-004 boundary[2] |
| 补充决策 6（StateDays 语义） | TASK-008 functional[2] |
| 补充决策 7（周跌触发判定） | TASK-004 boundary[2] |
| 补充决策 8（ClearStreak 含当日） | TASK-008 functional[4] |
| Global：gitnexus_impact 前置 | TASK-001/009 non_functional（verify_by: review） |
| Global：任一提交点全仓可编译 | TASK-008 non_functional[1]、TASK-009 non_functional[2] |

**孤儿需求检查**：无。设计条目经"自查记录"表逐条映射到任务，每个映射任务的 DoD 含对应锚点。

## 反向：DoD → 需求

抽查全部 9 任务 DoD 共 41 条，均可回溯到上表需求条目或实施方案 Global Constraints；
无凭空 DoD。verify_by 标注：`review` 4 条（gitnexus 前置 ×2、纯函数约束 ×1、时序注释 ×1），
`test` 其余全部——`review`/`manual` 占比低，适合全自动流程。

## 任务图人工核查（validator 降级）

`validator/` 不存在，按 CLAUDE.md 降级路径由 Leader 人工核查：

- **DAG 无环**：001→004→005→006→007→009；002→008→009；003→004；无回边 ✅
- **wave 序**（本任务.wave > max(依赖.wave)）：004:2>1、005:3>2、006:4>3、007:5>4、008:3>max(1,2)、009:6>max(3,4,5,3) ✅
- **scope 非空**：9/9 任务 packages 非空 ✅
- **scope 互斥（在途）**：internal/crisis 任务（T1–7、T9）依赖链天然串行，dag 调度下同包同时就绪的只有 wave1 的 T1/T2/T3——**派发时 Leader 必须一次只放行一个 internal/crisis 任务**；T8（./cmd/atlas）可与 T5/6/7 并行 ✅（调度纪律，见 plan.md）
- **context_from 闭合**：引用的 task id 全部存在 ✅
- **epoch/owner 初始不变量**：全部 status=pending、assignment_epoch=0、assigned_to=null ✅
- **例外记录**：TASK-009 跨 2 包（AD-1，原子切换要求）；TASK-001 预计 6 文件（AD-3，实际 diff 小）

## 团队规模建议

Dev × 2（dev-agent-1 主链 T1→T3→T4→T5→T6→T7→T9；dev-agent-2 承接 T2 后转 T8）+ Test × 1。
注：T1/T2/T3 同包不可并行在途，dev-agent-2 在 T2 完成后需等 T4 verified 才能开 T8。
