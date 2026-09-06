# 需求 ↔ DoD 双向追溯矩阵 · Sprint M2a（契约队列与信号快照）

**采样锚**：`d27791c695e8ebd0fd5d54c9161782f52d9b12cb`
**需求源**：`hestia/docs/superpowers/plans/2026-09-05-hestia-m2a-contract-queue.md`（7 个 Task + Global Constraints + 8 条交付前清单）；spec `specs/2026-09-05-hestia-m2a-contract-queue-design.md`
**本 Sprint 射程**：需求 TASK-001 ～ 007 全部（AD-1）

## 1. 正向：需求 Task → Arcforge TASK

| 需求 Task | Arcforge | wave | 射程是否逐字一致 |
|---|---|---|---|
| 001 `queue`/`signals` 段 | TASK-001 | 1 | ✓ +json tag 移入（AD-6）+ `hestia_test.go` 两处内联 yaml 补 `queue.dir`（AD-5，B7 订正）+ AST 登记 `DefaultSignals`（B1）+ `:387` 版本号（B5）+ 2 条边界（仓库 yaml 与预填一致、`Signals` JSON 键集） |
| 002 `Evaluate` | TASK-002 | 2 | ✓ +边界（`monthsInPeriod` 非法输入、`total==0`、空 Values）+ golden 数值溯源（review） |
| 003 契约结构 + `Current`/`PriorPublishedAt` | TASK-003 | 3 | ✓ **不改 `config.go`**（AD-6）；`articleURL` 改名 `pbocArticleURL`（B2）、去 `context` import（B3）、登记 `Contract.JSON`/`FileName`（B4）；+顶层键序守卫、`period_type` 不串；⚠️ **依赖 002**（AD-3，需求说可并行） |
| 004 队列写盘 | TASK-004 | 4 | ✓ +边界（目标名是目录 ⇒ rename 失败无残留；`dir` 是文件） |
| 005 ingest 接线 + P2 | TASK-005 | 5 | ⚠️ **四处订正**：写失败夹具（AD-4a）、outcome/P1 判定（AD-4b）、空 `queue.dir` 守卫（AD-5）、OutOfOrder 不写契约（AD-14）；+边界（P2 失败契约仍在） |
| 006 `contract emit` | TASK-006 | 5 | ✓ cleanup 沿既有模式（AD-8）；用例改 `newCapturingCmd`（B6）；+边界（`--period-type` 五值校验、`SilenceUsage`/注册守卫 S3） |
| 007 收口 | TASK-007 | 6 | ⚠️ **形态改为 docs-only**（AD-1）；Step 1 code-simplifier 改为终检只审；Step 6 合并/归档是 Leader 动作；§A 加 A7/A8 |

**孤儿需求检查**：7/7 有对应任务，无孤儿。

## 2. 反向：Global Constraints → 落点

| 约束 | 落点 | 载体 |
|---|---|---|
| Go 1.24.4，无新增依赖 | 001–006 GATE 段；007 §B | `go.mod`/`go.sum` 无 diff |
| `Parse`/`Validate`/`Save` 不动；`store.go` 只新增只读方法 | 每任务 GATE（四文件 diff 空 + `Save` 0）；003 functional[1]（`-` 行 0） | 命令给全 |
| 写口守卫「登记而不是放宽」 | 001 functional[0]（26）；002 functional[1]（27）；003 functional[2]（32 / 14）；004 functional[1]（34） | 精确项数（AD-10 订正后） |
| 字段名字面量只在 `fields.go`/`_test.go` | 002 functional[0]、003 functional[0] | `TestFieldNamesAppearOnlyInFieldsGo` |
| 契约 JSON 固定键序、逐字节相同 | 003 functional[0]（`TestContractJSONIsDeterministic`）+ boundary[0]（顶层 17 键序） | 测试 |
| `cmd/atlas/hestia.go` 不 import `path/filepath` | 每任务 GATE；006 boundary[1] | 既有守卫 |
| 注释带 milestone 前缀 `M2a 的 TASK-00N` | 每任务 GATE | review |
| gofmt / vet / 两包测试干净 | 每任务 GATE | 命令 |
| 覆盖率 ≥ 96.6% | 每任务 GATE（+`cmd/atlas` ≥ 76.4）；007 §B | AD-7 |
| 提交前 code-simplifier | 每任务交付流程第 3 步；007 终检 | discovery 申报 |
| 测试 import 按需增补 | 006 functional[1] | — |

## 3. 交付前清单（需求末尾 8 条）→ 落点

| 清单 | 落点 |
|---|---|
| 两包测试绿、覆盖率 ≥ 96.6、vet 干净 | 每任务 GATE；007 §B |
| 四个不动文件 diff 为空；`Save` 不动；无新增写方法 | 每任务 GATE；003 functional[1]；007 §B |
| 三期 golden 通过 | 002 functional[0]；007 §B |
| 真语料回归数字一个不变；`queue/` 无文件 | 007 functional[3] |
| 守卫登记（六个新导出项） | 002/003/004；007 §B |
| yaml 有 `queue`/`signals`、`config_version` 递增 | 001 functional[1] |
| CONTRACTS §A–§C | 007 functional[1,2,4] |
| 投递排在首期验收之后 | 007 §C；不 deploy（AD-1） |

## 4. 凭空 DoD 检查（DoD 中不对应需求原文的条目，逐条给出依据）

| 条目 | 依据 |
|---|---|
| 001 b[0] `TestSignalsJSONKeys` | 003 契约 `thresholds.signals` 直接序列化 `Signals`；需求要求 `TempScale` `json:"-"`——键集是契约的一部分（spec §2.2） |
| 001 f[2] cmd 测试 yaml 补 `queue.dir` | AD-5（B7 订正）：`hestia_test.go:1225/:1342` 真入库用例 + 预填相对 cwd |
| 002 b[0] 非法输入 | 需求 `monthsInPeriod` 注释「解析不出 ⇒ 0」的边界枚举 |
| 002 b[1] golden 溯源 | 需求原文「数值来自回填库 v_hestia_current，不是编的」——把声明变成可核对 |
| 003 b[0] 顶层键序 | Global Constraints「契约 JSON 固定键序」 |
| 003 b[1] `period_type` 不串 | 需求 `FileName` 注释「12 月月报与年报不能撞名」的存储侧对应 |
| 004 b[0] 目标名是目录 | AD-4a：005 夹具的前置验证 |
| 005 f[0] 空 `queue.dir` 报错 | AD-5（机制：`MkdirAll("pending")` 相对 cwd） |
| 005 f[2] `contractError` | AD-4b（需求 §A6 与既有 `runRow`/循环冲突，必须择一） |
| 005 b[0] P2 失败契约仍在 | 需求「契约在通知之前，通知是投影」的另一半 |
| 006 b[0] `--period-type` 校验 | 需求「参数写错的人第一秒就知道」对第二个参数的自然延伸；**若 reviewer 认为越界可删** |
| 007 f[0] 采锚收窄为代码范围 | 需求「`git status --short` 必须干净」在 sprint 进行中恒不成立（`.arcforge/` 在途） |
| 005 f[1] OutOfOrder 不写契约 | AD-14（spec「最新的赢」；`Save` 对 OutOfOrder 入表） |
| 006 b[1] `SilenceUsage`/注册守卫 | 既有守卫的精确列表（S3） |

**reviewer 反审**：见 `02-plan/dod-review.md`；处置表 AD-15。
