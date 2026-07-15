# 独立 DoD 反审报告 — sprint-021 crisis-notify v1.1

> **反审方式**：独立 reviewer，严格两步。第一步仅读需求 `docs/plans/2026-07-15-crisis-notify-v1.1-design.md`
> 与实施方案 `docs/plans/2026-07-15-crisis-notify-v1.1-impl.md`，独立推导每任务应覆盖的
> functional/boundary/error_handling/non_functional 要点；第二步再读 TASK-001~003.json 逐条比对。
> **审阅日期**：2026-07-15

## 总体结论：PASS_WITH_NOTES

三个任务的 DoD 忠实覆盖了设计 v1.1 全部 R1–R6、4 条新增原则与 §4 七条测试要点，无孤儿需求、无
明显凭空 DoD；依赖图（T1→T2→T3）、wave 序（1<2<3）、同文件串行（T1/T2 共 notify_render.go 靠依赖串行）
与 T3 跨包原子性（internal/crisis 生产 + cmd/atlas test-only）均标注正确。所有 functional 判据均为
可断言的字面值，具备可测性。发现 5 条 note（1 中、4 低），无阻断性缺陷。

## 一、独立推导 → 任务覆盖比对表

| 设计条目 | 独立推导应含要点 | 覆盖任务 · DoD 定位 | 判定 |
|---|---|---|---|
| R1a renderTransition 降级溯源 | 警示行文案逐字；NewStale∧PrevDay≥AMBER；多指标 AllIndicators 序、颜色同序；置于尾注前；三独立否定（空/绿/升级）；AMBER 落界 | T1 fn3 + bd1/bd2 | ✅ 全覆盖 |
| R1b renderOpsAlert 条件警示 | 断更前 RED/AMBER 追加警示；AMBER 落界出现、绿不出现、缺行不出现；色词匹配 | T2 fn3 + bd | ✅ 全覆盖 |
| R2 条件符号 | 异常区空→✅状态解除；非空→🔽状态回落；⚪不计异常区；升级不变；恰一 AMBER 落界 | T1 fn2 + bd1 | ✅ 全覆盖 |
| R2 CRISIS→WATCH 语义句 | 逐字含「危机状态退出，转入观察期…可能仍异常，见下」；不含「危机状态解除」 | T1 fn1 | ✅ 全覆盖 |
| R3 WATCH→BREWING 去预测 | 含「此为状态描述而非预测，不构成操作依据」；删「3–12 个月」 | T1 fn1 | ✅ 全覆盖 |
| R4 双非色彩迁移 | 两侧非色彩用 nonColorNote 具体文案（STALE→季末抑制、NO_DATA→STALE）；混合维持 v1.0；不含「转白（原白）」 | T2 fn1 | ✅ 全覆盖 |
| R5 盘中去归因 | 含「成因未核实，非交易信号」；不含「carry trade」；格式逐字 | T3 fn1 | ✅ 全覆盖 |
| R5 页脚断言连锁 | 全家族改 HasSuffix(notifyFooter)；5 结构化真 / 2 速报假；计数 5+2 | T3 fn2 + bd | ✅ 全覆盖 |
| R6 术语外化 | 「不再计入触发判定」「数据恢复后自动重新计入」；灭「退出共振计数」「恢复后自动回归」 | T2 fn2 | ✅ 全覆盖 |
| 原则 1 非预测从句 | 由 R2/R3 落地 | T1 | ✅ 间接覆盖 |
| 原则 2 ✅仅全清 | 由 R2 条件符号落地 | T1 | ✅ 覆盖 |
| 原则 3 判定输入变化必溯源 | R1a（转移消息）+ R1b（P2），P2 非唯一载体 | T1 fn3 + T2 fn3 | ⚠ 见 N3（分解覆盖，无联合断言） |
| 原则 4 术语外化 | 由 R6 落地 | T2 | ✅ 覆盖 |
| §4.1~4.7 测试要点 | 逐条见上（1→T1bd,2→T2bd,3→T1bd,4→T1fn,5→T2fn,6→T3fn,7→T3nf） | T1/T2/T3 | ⚠ 4096 见 N1 |

**孤儿需求**：无。R1–R6、原则 1–4、§4.1–4.7 均有对应 done_criteria。
**凭空 DoD**：仅 T3 `coverage_minimum:35`（见 N2），其余 DoD 均可回溯到设计条目或 Global Constraints。

## 二、任务图与串行约束校验

| 检查项 | 结论 |
|---|---|
| DAG 无环 | ✅ T1(deps=[]) → T2(deps=[T1]) → T3(deps=[T1,T2]) |
| wave 序 `本任务.wave > max(依赖.wave)` | ✅ 1 / 2 / 3 严格递增 |
| T1/T2 同文件（notify_render.go）串行 | ✅ 靠 T2 依赖 T1 强制串行；DAG 调度下二者不同时在途，scope 互斥不被违反 |
| T3 跨包原子性（生产 + cmd test-only） | ✅ description 标 AD-2；packages=[./internal/crisis,./cmd/atlas]；nf3 要求两包全绿间接兜底（见 N4） |
| context_from 闭合 | ✅ T2←T1、T3←T1,T2 |

## 三、问题清单（含建议修正）

### N1 [中] §4.7 的 4096 最坏组合未锚定在引入加长的 T1
设计 §4.7 明确把 4096 上限与「R1a 警示行加长后的最坏组合」绑定，但唯一的 `len(m)<=4096`
断言在 T3 的 TestMessagesForbiddenWordsAllFamilies 里，遍历的是 `Messages` 常规产物，**不保证**
构造出「多指标 R1a 警示行 + 异常区非空」的最坏降级消息。引入加长的 T1 本身无任何长度边界判据。
- **建议**：在 T3 该测试内显式构造一条多指标 stale 被动降级消息纳入 `all` 后再断 4096；
  或在 T1 non_functional 增一条「多指标警示行下 renderTransition 输出 ≤4096」的落界判据。

### N2 [低-中] T3 `coverage_minimum:35` 疑似凭空阈值
该字段在设计与 impl 中均无出处；T3 以测试改动为主（生产仅 FormatIntradayAlert 一格式串），
35% 覆盖门槛缺乏依据，可能造成误判 NEEDS_WORK。
- **建议**：删除该字段，或注明其来源与对 test-only 任务的适用口径。

### N3 [低] 原则 3「P2 不得成为唯一载体」仅分解覆盖，无联合不变量断言
R1a（T1，转移消息带警示）与 R1b（T2，P2 带警示）各自独立断言，但无任一 DoD 在「被动降级当日」
同时校验转移消息与 P2 都携带警示。分解覆盖可接受，但缺少体现原则原意的集成级断言。
- **建议**：可选——在 T3 全家族测试补一条「被动降级场景下转移消息含 ⚠ 警示（不依赖 P2）」的断言，或明确接受分解覆盖。

### N4 [低] T3 跨包原子提交（AD-2）仅在 description，未成可核查判据
「生产变更与 cmd 测试断言须原子提交」写在 description，未落为 done_criteria；当前靠 nf3
「两包全绿」间接兜底（若 cmd 测试未同提交则 build/test 断裂）。
- **建议**：将「单次提交同时含 internal/crisis 生产改动与 cmd/atlas 测试适配」提为一条
  `verify_by: review` 判据，使原子性可被显式核查。

### N5 [低/nit] R2 首行 `· MM-DD` 日期后缀未进 DoD 字面值
T1 fn2 仅断到「[P1] 🔽 状态回落」前缀，未含设计 R2 首行的 `· MM-DD`。impl 测试用 HasPrefix
含日期，可测性无碍，仅记录 DoD 字面覆盖与设计示例的细微差。

## 四、可测性小结
所有 functional 判据 = 具体字面串断言（可 Contains/HasPrefix/HasSuffix 直测）；boundary 均给出
恰好落界取值（一个 AMBER / 断更前恰 AMBER）；non_functional 的变异自检项（len>0→>=0、severity <→<=、
&&→||、>=→>、误挂 notifyFooter）均可机械执行。除 N1 的 4096 最坏组合锚点偏弱外，无不可测条目。
