# Sprint 036 进度 — Hestia M1b-4a discover + fetch + config

**需求**：`hestia/docs/superpowers/plans/2026-08-12-hestia-discover.md`（2143 行）
**目标包**：`internal/hestia` · **调度**：`dag` · **基线**：`f5a17d5`，覆盖率 **92.1%**

## 阶段

- [x] Step 1 环境检查（运行时目录已重置、基线 92.1%、git 干净）
- [x] Step 2 需求分析（`01-design/` 两份）
- [x] Step 3 任务拆分 + DoD（7 任务、各 ≤8 条）+ 双向追溯 + validator `exit=0`
- [x] Step 4 **人类确认门**（2026-08-11 放行 spawn；worktree 沿用方案 C）
- [ ] Step 5 组队与开发 <- **当前**：6/7 已 verified，T5 验证中，T7 待就绪
- [ ] Step 6 QA 两轮
- [ ] Step 7 交付与归档

## 当前状态（2026-08-12）

| ID | status | 备注 |
|---|---|---|
| TASK-001/002/003/004/006 | `verified` | — |
| TASK-005 | `verifying` | verifier=test-agent-25，基线 `7b49b13c11f2649017a7f2393da481354dc530e2`；实测 93.1% / 0 FAIL / `-race` 绿 |
| TASK-007 | `pending` | **未就绪**：dag 模式要求 deps 全 `verified`，T5 尚在验证中。且其 `writes` 含 `store_test.go` —— 与 T5 刚改的文件同一份，抢跑会撞 scope |

**覆盖率轨迹**：基线 92.1% → T5 交付后 **93.1%**（`discover.go` 六个函数全部 100%）。
**整包**：266 顶层测试（含子测试 562），0 FAIL。

**结转发现**：`02-plan/findings-carryover.md` 已累计 **G1–G31**。
最新 G31 是 Leader 自己写的假自证判据被验证者预演拦下 —— 详见该文件。

## 任务图（**按文件级而非逻辑级排 wave**，见 AD-036-1）

| ID | wave | 标题 | 依赖 | writes 关键项 |
|---|---|---|---|---|
| TASK-001 | 1 | Fetcher 与绕代理的 PBOC client | — | fetch.go, **store_test.go** |
| TASK-002 | 1 | index 快照与分页模板解析 | — | discover.go, testdata/* |
| TASK-006 | 1 | 口径豁免的键补 PeriodTypes | — | thresholds.go, validate.go |
| TASK-003 | 2 | 标题正则、期次映射与条目提取 | 002 | discover.go |
| TASK-004 | 3 | Store.HasPeriod 与 PeriodChecker | 003, 001 | discover.go, store.go, **store_test.go** |
| TASK-005 | 4 | Discover 主循环 | 001, 003, 004 | discover.go, **store_test.go** |
| TASK-007 | 5 | LoadConfig 与 CONTRACTS | 005, 006 | config.go, thresholds.go, **store_test.go** |

**并行度 3**（wave 1）。计划声称「T1–T4、T6 可并行」是**逻辑独立**，
但 **T2/T3/T4/T5 全部写 `discover.go`**，且 T3 要重构 T2 写的 `pageURL` ⇒ 必须串行。
**T1/T4/T5/T7 都要改 `store_test.go`**（导出面守卫登记），分处不同 wave 故不冲突。

## 前提核验（Leader 实测）

- viper `v1.21.0` 已在 go.mod；`validPeriodTypes`/`checkEnum`/`failing()`/`saveMonthly` 等符号全部存在
- **T6 改动清单逐字一致**：`CaliberExemption{` 构造点 **10 处**、`exemptionFor` 调用点 **4 处**（生产代码仅 1 处）
- **时效项已验**：index p1/p2 均 HTTP 200；**p1 无报告、p2 有报告、干扰项同在 p2**；
  分页控件两页都有（408 页）⇒ T2 用 p1+p2 成立，不必改用 p3。**但快照须由 T2 的 dev 当场抓**
- 计划的 `checkEnum` 行号已漂（写 110、实为 116）⇒ **计划的行号引用不可尽信，以符号名为准**

## Leader 独立追溯的两个发现

1. **计划的 `TestDiscoverFindsReportOnSecondPage` 推演必定失败**（fake 只备 2 页而 `MaxPages: 3`，
   空库下实现会翻到第 3 页）—— **这是推断不是实测**，已写进 TASK-005 要求 dev 实跑确认后自行选修法
2. **spec `functional[4]` 与 `functional[5]` 的断言对象相反**（`calls` 变短 vs 达到 MaxPages），
   共用同一个 fake 容易被合并成一个用例，而**只看返回值两者不可分**

## 沿用 Sprint 035 的四条派单硬要求

编译失败闸 + 计数自证 / `require.NotNil(errors.Unwrap(err))` / 写断言时的事前发问 / 别把理由写强。
完整实测依据见 `.arcforge/archive/sprint-035-2026-08-11/docs/02-plan/findings-carryover.md`（F1–F43）。
