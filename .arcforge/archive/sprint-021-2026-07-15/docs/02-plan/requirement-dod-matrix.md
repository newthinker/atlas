# 需求 ↔ DoD 双向追溯矩阵 — sprint-021（通知 v1.1）

**需求基线**：`docs/plans/2026-07-15-crisis-notify-v1.1-impl.md`（设计 v1.1 R1–R6）
**生成**：2026-07-15 Leader

## 正向：需求 → DoD

| 设计条目 | 任务 | DoD 锚点 |
|---|---|---|
| R1a 降级警示行（条件/多指标序/落界） | TASK-001 | functional[2]、boundary[0][1] |
| R1b P2 条件警示（落界/否定/缺行） | TASK-002 | functional[2]、boundary[0] |
| R2 条件符号 ✅/🔽 + CRISIS→WATCH 语义句 | TASK-001 | functional[0][1]、boundary[0] |
| R3 WATCH→BREWING 去预测感 | TASK-001 | functional[0] |
| R4 双非色彩迁移（混合维持 v1.0） | TASK-002 | functional[0] |
| R5 盘中去归因 + 内联限定语 + HasSuffix 连锁 | TASK-003 | functional[0][1][2]、boundary[0] |
| R6 术语外化（两分支） | TASK-002 | functional[1] |
| 原则 1（非预测从句） | TASK-001 functional[0]（措辞落地） |
| 原则 2（✅ 仅限全清） | TASK-001 functional[1] |
| 原则 3（判定输入变化必溯源） | TASK-001 functional[2] + TASK-002 functional[2] |
| 原则 4（术语外化） | TASK-002 functional[1] |
| 设计 §4 测试要点 1–7 | 1→T1、2→T2、3→T1、4→T1、5→T2、6→T3、7→T3（禁词回归+4096） |
| Global：impact/detect_changes/code-simplifier/禁词/变异纪律 | 各任务 non_functional |

**孤儿需求检查**：无——R1–R6、4 原则、7 测试要点全部有 DoD 锚点。

## 反向：DoD → 需求

22 条 DoD 全部可回溯至 R1–R6/原则/Global Constraints；无凭空 DoD。verify_by=review 共 3 条（各任务门禁项），占比低。

## DoD 解释性裁决（Leader，2026-07-15）

- T3 non_functional[0] 的「grep 四旧词全仓零命中」按意图解释为**生产消息字面值零命中**；
  测试 NotContains 守卫（须字面引用旧词才能断言缺席）与工程域注释（rules.go:226、
  crisis.go:463 解释金融机制的 carry trade 术语）豁免。R5 对象是用户可见文案，非注释术语。

## 任务图人工核查（validator 降级，沿 sprint-020 路径）

- DAG 无环：001→002→003 线性链 ✅
- wave 序：002:2>1、003:3>2 ✅
- scope 非空/互斥：串行链无并发在途；T1/T2 同包同文件靠依赖串行化；T3 跨两包为 AD-2 例外（cmd 仅测试文件，原子提交要求）✅
- context_from 闭合、epoch/owner 初始不变量（全 pending/epoch 0）✅
- DoD 条数 8/7/7 ≤8 ✅

## 团队规模建议

串行链 → **dev×1 + test×1**（沿用 sprint-020 存活实例 dev-agent-1/test-agent-1，上下文与纪律已就位）。
