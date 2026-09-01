# 需求 ↔ DoD 双向追溯矩阵 · Sprint M1c-4

**采样锚**：`4670ccbe0abd703f86b1e0c53aef8d3c86cc512d`
**需求源**：`2026-09-01-hestia-backfill-finalize.md`（13 个 Task + Global Constraints + 交付前检查清单）

## 1. 正向：需求 Task → TASK-00N

| 需求 Task | TASK | wave | 射程是否逐字一致 |
|---|---|---|---|
| Task 1 `stripHTML` 去行首空白 | TASK-001 | 1 | ✓ |
| Task 2 `periodAlt` 补「今年前N个月」 | TASK-002 | 2 | ✓ |
| Task 3 `moneyRE` 容忍短从句 | TASK-003 | 3 | ✓ |
| Task 4 22 个 `_mom` 字段常量 | TASK-004 | 1 | ⚠️ **射程扩大**（见 §3） |
| Task 5 模板表带 mom + 抽取路由 | TASK-005 | 4 | ⚠️ **射程缩小**（模板表并入 004） |
| Task 6 必填集口径感知 | TASK-006 | 5 | ✓ |
| Task 7 闸门按口径族校验 | TASK-007 | 6 | ✓ |
| Task 8 `PrecedingAll` + drift 修复 | TASK-008 | 7 | ✓ |
| Task 9 calibrate 输出残差分布 | TASK-009 | 7 | ✓ |
| Task 10 人工标定 yaml | TASK-010 | 8 | ✓（占位值已由 004 先填） |
| Task 11 口径路由断言 | TASK-011 | 7 | ✓ |
| Task 12 真跑验收 + CONTRACTS | TASK-012 | 9 | ✓ |
| Task 13 人工核对与切库 | TASK-013 | 10 | ⚠️ **产物载体改变**（见 §3） |

**孤儿需求检查**：13/13 全部有对应任务，**无孤儿**。

## 2. 反向：Global Constraints & 交付清单 → 落点

### 2.1 Global Constraints

| 约束 | 落在哪 | 载体 |
|---|---|---|
| Go 1.24.4，无新增依赖 | 全部 13 个任务 | `non_functional`（**每个任务都写**，不靠指针） |
| 字段名字面量只许在 `fields.go` 与 `_test.go` | TASK-004（新增 22 常量）、TASK-005（模板表消费） | ⚠️ **未显式写进 DoD** — 见 §4 缺口 1 |
| 包的导出面精确相等 | TASK-008（新增导出 `PrecedingAll`） | `error_handling`：`TestPackageExposesNoWriteFunctions` 会转红，**不要改断言** |
| `Meta` 七字段三处同序 | 本迭代**不动 `Meta`** | ⚠️ 未写进任何 DoD — 见 §4 缺口 2 |
| 注释里任务编号带 milestone 前缀 | 全部任务 | `non_functional` |
| 每 task 结束 `gofmt`/`vet`/`test` 干净 | 全部任务 | `non_functional`，判据是「那两个既有欠账文件之外没有新增项」 |
| 工具只产出依据，不做人的判断 | TASK-009（不加建议列）、TASK-010（人工填值）、TASK-013（人执行切库） | `functional` + `error_handling` 的澄清环 |
| 测试文件 `import` 按需增补 | 全部 | 不单列（属常识性实现细节） |

### 2.2 交付前检查清单（12 条）

| 清单条目 | 落点 | DoD 维度 |
|---|---|---|
| 四道恒等式成立 | TASK-012 | functional[1] |
| 解析残余 0 或每篇有解释 | TASK-012 | functional[1] |
| 口径路由违反 = 0 | TASK-011（立断言）+ TASK-012（真跑核） | functional |
| **异号跳过数 ≠ 总对数** | TASK-011 functional[1]+error_handling、TASK-012 functional[1] | ✓ **两处都写了** |
| 字段冲突 = 0 | TASK-012 | functional[1] |
| `tsf_stock` = 79 | TASK-012 | functional[1] |
| `deposit_sum` 落 pending = 0 或有裁决 | TASK-012 | functional[1] |
| yaml 22 项带 `unit`、`config_version` 递增 | TASK-004（占位）+ TASK-010（标定） | functional / boundary |
| 覆盖率 ≥96.1% | **全部 13 个任务** | non_functional |
| 自证数字采于最后一次改动之后 | TASK-012 | functional[0]（**前置**为第一步）+ error_handling |
| 结转项 6 闭合/销案、7 不做、8 未触发 | TASK-012 boundary[0]；结转项 8 另在 TASK-002 boundary[0] | ✓ |
| ADR-0003 补建立判据 | TASK-012 | boundary[1] |
| 生产库备份 + 切库后逐项相等 | TASK-013 | functional[2] |

**凭空 DoD 检查**：逐条回溯 13 个任务的全部 DoD 条目，**未发现不对应任何需求条目的判据**。
唯一「需求文档里没有、DoD 里有」的是 §3 的三处调整——它们是**对文档缺口的修补**，不是凭空发明，理由逐条记在 `01-design/architecture-decisions.md`。

## 3. 三处偏离需求文档的调整（逐条附理由）

| # | 调整 | 理由 | 记在 |
|---|---|---|---|
| 1 | **TASK-004 射程扩大**：加入模板表 `momField` 列、`templateFields()` 收两列、yaml 占位 22 项、三处计数断言更新 | `fieldOrder` 被三条全覆盖断言绑住（`config_test.go:377` / `fields_test.go:151` / `profiles_test.go:73`），**拆开必然留下跨 6 个 wave 的红测试**，而每个任务的 `dev_done` 门禁都跑 `go test` | AD-M1c4-3 |
| 2 | **yaml 先填占位值**（TASK-004）后重标（TASK-010） | 循环依赖：填表要实测分布 → 分布要抽取路由 → 路由要测试绿 → 绿要表填好。先例是 `stock_continuity_max` 三档 n=0 | AD-M1c4-4 |
| 3 | **TASK-013 产物落 `CONTRACTS.md`** 而非 `.arcforge/docs/07-deploy/` | write-matrix 里 `docs/07-deploy/*` **只有 `ops-*` 能写**，而 `ops-*` 不在 `assigned → in_progress` 的合法写者里 ⇒ 那条路走不通，且会在交付时才发现 | 本文件 + TASK-013 description |

## 4. 🔴 已知缺口（矩阵检查暴露，待 reviewer 与人类确认）

### 缺口 1：「字段名字面量只许在 `fields.go` 与 `_test.go`」未写进 TASK-004/005 的 DoD

Global Constraint 说这条由 `TestFieldNamesAppearOnlyInFieldsGo` 守着。TASK-004 新增 22 个常量、TASK-005 在 `extract.go` 里消费它们——**若实现时图省事写了裸字符串，那条测试会转红**。

**风险等级：低**（守卫会立刻变红，有反馈回路）⇒ 按「重复发生 ≠ 该加机制，先问有没有反馈回路」的判据，**不单独加 DoD 条目**，但已在本矩阵登记，交 reviewer 复核该测试是否真的存在且真的这么判。

### 缺口 2：「`Meta` 七字段三处同序」未写进任何 DoD

需求文档说「本迭代**不动 `Meta`**，三处同序约束不受影响」。⇒ 这是一条**不适用声明**，不是义务。

**但 TASK-004 会扩 `fieldOrder` 进而扩 `observationsDDL`**——需要确认 `metaColumns` 与业务字段列在 DDL 里是**分开生成**的。若不是，加字段会波及 `Meta` 的列序。

**处置**：已在 TASK-004 boundary[0] 要求跑 `go test -run 'TestSchema|TestObservationsDDL'` 确认 DDL 自动扩表。交 reviewer 核实这条是否足够。

### 缺口 3：TASK-005 的「解析失败大幅下降」不可自动裁决

需求文档 Task 5 Step 8 明说「**不要在这里写死期望数字**——TASK-012 才是采数的地方」。

⇒ 这是**有意为之**的不可测，理由充分（spec §5.1：「理论残余为 0」是判据不是期望值）。DoD 已原样保留该说明。**不作为缺口修补**，但验证者需知道这一条不能按「没有数字判据」判 NEEDS WORK。

## 5. 验证者须知（写给 test-m1c4-*）

1. **每个任务的验收锚点在 `error_handling` 维度**——背对背基线比对、语料路径、真跑核对都在那里，不在 `functional`。
2. **`verify_by: manual` 的条目要求你自己复现那条命令**，不能只看 dev 的 discovery 里写了什么数字。
3. **TASK-004 的验收判据是「背对背 diff 为空」**——它是唯一一个「改了很多但输出必须逐字节不变」的任务。
4. **凡是 dev 报的自证数字，核实它的采样锚**：数字必须采于该任务最后一次改动之后。跨时间点采的数字在报告里**长得和同时刻采的一模一样**。
