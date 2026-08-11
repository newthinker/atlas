# 需求分析 — Hestia M1b-3 validate（Sprint 035）

**需求文档**：`~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-11-hestia-validate.md`（2531 行）
**目标包**：`internal/hestia`（在本仓库 atlas，不在 hestia 仓库——需求文档只是存放在那边）
**Sprint**：035

## 1. 需求性质：这是一份已完成的实施计划，不是待设计的需求

上游文档不是「需求」而是**逐步骤的实施计划**：7 个任务、每个 Step 给出了完整的目标代码与测试代码、
预期的 RED 失败信息、以及提交信息。它已经过 superpowers `writing-plans` 流程产出，并附带自审章节
（Spec 覆盖矩阵、计划外新增三条、类型一致性、零分母三处保护）。

**这决定了本 Sprint 的性质**：Arcforge 的价值不在「重新设计」，而在
①把计划拆成受状态机管理的任务、②用 DoD 把计划的「预期」变成可验证断言、
③用独立 reviewer 与 test-agent 检查计划自身的漏洞、④机制化地防住并发与门禁的已知坑。

⇒ **不重写设计**。设计规格以上游计划为准（见 `design-spec.md` 的指针）。
Leader 的增量产出是任务图、DoD 与风险处置。

## 2. 交付内容

为 `internal/hestia` 实现七道校验闸门，产出 `Store.Save` 强制要求的 `ValidationReport`：

| 闸门 ID | 判据 | 依赖历史 |
|---|---|---|
| `monetary_hierarchy` | M2 > M1 > M0 | 否 |
| `deposit_sum` | 绝对残差 <=12% **且** 漂移 <=3pct（两判据合成一态） | 是（漂移需 >=3 期） |
| `corp_loan_reconcile` | 短期+中长期+票据 vs 企业合计，残差 <=5% | 否 |
| `stock_continuity` | 社融存量环比 <=2% | 是（n=1） |
| `yoy_sanity` | 同比字段绝对值 <=50 | 否 |
| `completeness` | 必填集齐全（从模板表派生，非手写） | 否 |
| `magnitude_sanity` | 字段落在合理区间（区间表**有意为空**至 M1c） | 否 |

架构：表驱动的 `gates` 集合，每道闸是 `func(gateInput) Check` 纯函数；历史经 `History` 单方法接口注入，
`Validate` 一次取满 6 期供两道闸共用。

## 3. 前提核验（Leader 实测，非照抄计划声明）

计划声称依赖的符号全部实测存在，行号与计划所述一致：

| 前提 | 实测结果 |
|---|---|
| `checkEnum`（types.go:110）、`validCaliberVersions`（:92）、`validExtractors`（:101） | OK 存在 |
| `extractorV1`/`extractorV2`（profiles.go:184-185） | OK `rule@v1` / `rule@v2` |
| `tsfStockItems`（profiles.go:89）、`tsfFlowItems`（:123） | OK 存在 |
| `fieldOrder`（fields.go:115）、`metaColumns`（schema.go:31）、`viewCurrent`（schema.go:15） | OK 存在 |
| `ValidationReport`/`Check`/`CheckStatus`/三个状态常量（types.go:174-194） | OK 存在 |
| 测试 helper `newTestStore`(:46) / `passing`(:468) / `validMeta` / `golden2025`(:53) / `golden2020`(:160) | OK 存在 |
| `modernc.org/sqlite` 版本约束 | OK 已固定 `v1.38.2`（go 1.24.4 要求，见项目记忆） |

**覆盖率基线**：`go test ./internal/hestia -coverpkg=./internal/hestia` = **89.4%**，`ok` 全绿。
远高于 `dev_minimum=80`，⇒ 本 Sprint **不存在** AD-6 那类「基线结构性低于门槛」的问题，无需 `coverage_floor` 豁免。

**`checkEnum` 影响分析**（唯一被修改的既有函数，gitnexus `impact --direction upstream`）：
风险 **LOW**，`impactedCount=2` —— d1 `Meta.validate`、d2 `Store.Save`，影响 1 个执行流（Save）、1 个模块（Hestia）。
计划的改法是把 `meta.` 前缀从函数体挪到两处调用点，**错误信息输出逐字不变**，故 d1/d2 的行为不变。
⇒ 无需 HIGH/CRITICAL 警告。DoD 里以「既有 Meta 校验测试不得变红」钉住这一点。

## 4. 复杂度与依赖

| 任务 | 复杂度 | 说明 |
|---|---|---|
| T1 Thresholds | 简单 | 纯结构体 + 自校验，无历史依赖 |
| T2 必填集派生 | 简单 | 两个纯函数，与 golden 键集双向比对 |
| T3 History + Preceding | 中等 | 触真库，SQL + NullFloat64 的 absent 语义还原 |
| T4 骨架 + 五道闸门 | 中等 | 本 Sprint 最大的一块（约 250 行 + 测试） |
| T5 deposit_sum | 中等 | 两判据合成三态，5 行映射表逐行验证 |
| T6 stock_continuity | 简单 | 三种跳过理由的优先级 |
| T7 豁免 + 接线 + 契约 | 中等 | 跨 T1-T6 的收尾，含 Save 接线真库测试 |

**依赖图**：T1、T2、T3 相互独立（wave 1 并行）；T4 起严格串行——T4->T5->T6->T7 都往同一张 `gates` 表加行、
改同一个 `validate.go`。

⇒ **实际并行度只有 3，且只在 wave 1**。这是任务本身的性质，不是拆分不当。

## 5. 识别出的风险（处置见 architecture-decisions.md）

1. **AD-035-1 worktree 隔离 x dev_done 门禁的结构冲突** —— 已跨 Sprint 复发两次，失败模式是「静默成功」
2. **AD-035-2 commit 与 transition 的顺序** —— 决定 scope 漂移判定是否可见
3. **AD-035-3 计划的 Step 顺序内含 RED 阶段** —— TDD 的 RED 必须真的跑出预期失败信息，不能跳
4. **AD-035-4 `validator` 的 `scope-writes-outside-packages` 告警是本仓库已知假阳** —— 不得照它改声明
