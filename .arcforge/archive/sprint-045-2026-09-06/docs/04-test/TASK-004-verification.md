# TASK-004 验证报告 · M2a 契约队列写盘 `EnsureQueueDirs` / `WriteContract` + 守卫 34 + 002/003 残留子例

- **验证者**：test-m2a-b
- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 057a91c5123c8440a2c0bcc49943b1c381805df0`（merge commit；dev 提交 `418f23eab28c18f4f2452c4fca7e0376f6c0f87d`，dev-m2a-b），discovery sha256 `39c6be51f6d7cbdea617d734a1cec42dab6226997f516eb2a0de3eb8297b12ce`；承接时 `assignment_epoch=1`
- **验证树**：`../wt-verify-TASK-004-b`（detached 于 `057a91c5…`），全部数字自采于此树，不取 discovery 的数
- **上游**：TASK-003（verified）；开工锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 1. 完成标准覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据（自采） | 判定 |
|---|---|---|---|
| functional[0] | `queue.go` 按原文（`queueStates` 四态、`EnsureQueueDirs` `MkdirAll` 0o755 错误含 `contract queue dir`、`WriteContract` 先 `JSON()` → `MkdirAll(pending)` → `writeAtomic`，错误含 `contract <FileName>`）；五条测试按原文 PASS | 与需求原文代码块（L1163-1204）去注释 diff 仅两处：`sub`/`pending` 局部变量提取（discovery `key_findings[5]`/`decisions[2]`/`provenance` 申报为 code-simplifier 改动，语义等价，我逐行核对）；`snapshot.go:97` 的 `writeAtomic` 确在 `Rename` 失败时 `os.Remove(tmp)`；五条测试 `--- PASS`；`queue.go` 引号字面量只有四个状态名、两个错误格式串与 import 路径 | PASS |
| functional[1] | AST 守卫 34：`EnsureQueueDirs`（`Discover` 后 `Evaluate` 前）、`WriteContract`（`Validate` 后）；两守卫 PASS | `store_test.go:453` want 计数 **34**，序列片段 `"Discover", "EnsureQueueDirs", "Evaluate"` 与 `"Validate", "WriteContract"}`，`sort` 比对字母序 OK；reflect want 仍 14；两守卫 PASS。本 Sprint 终值与 AD-10 一致 | PASS |
| boundary[0] | `TestWriteContractFailsWhenTargetIsDir`：目标预建为目录 ⇒ 错误含 `contract`、`pending/` 只剩那个目录、无 `.tmp` | 测试 PASS（断言 `ReadDir` 恰 1 项、名为 `c.FileName()`、`IsDir`）；变异 Q6（rename 层错误前缀改字）**独家**由它转红 | PASS |
| boundary[1] | `EnsureQueueDirs` 对普通文件 ⇒ 含 `contract queue dir`；`WriteContract` 对不存在的 `dir` 自建 `pending/` | `TestEnsureQueueDirsRejectsFile`、`TestWriteContractCreatesPendingWithoutEnsure`（并断言 `processing/done/failed` 不被建）PASS；变异 Q2/Q9 由前者、Q5 由后者转红 | PASS |
| boundary[2] | 002 残留六子例（五个相等边 + 混口径反向，want 不变，`signals_test.go` 除此不改）；003 残留 (a) 文件末字节 `\n`、(b) 全字段在场 `absent_fields` 为 `[]` | `signals_test.go` diff **纯新增 34 行**（一行映射注释 + `TestEvaluateThresholdEdges` 表驱动六子例），既有用例一行未改；`queue_test.go` 的 `TestWriteContractEndsWithNewline`（另断言倒数第二字节非 `\n`）、`TestWriteContractAbsentFieldsEmptyArray`（`for _, f := range fieldOrder { vals[f] = 1 }` 夹具）PASS。**8 个对应变异体各被其子例/用例独家转红**（§3 S1–S6、C1、C2）；AST/reflect want 不变 | PASS |
| error_handling[0] | 红阶段含 `undefined: EnsureQueueDirs` | 验证树移走 `queue.go` 复现：`queue_test.go:35:21`、`:41:21`、`:46:21: undefined: EnsureQueueDirs`、`:49:15: undefined: WriteContract`。discovery 记 `:30/:36/:41/:44`，四条一致偏移 **+5**——code-simplifier 在红阶段捕获后提取的 `passingContract` 块（2 行注释 + 3 行函数）恰 5 行，dev 已申报该改动；同形同序，非伪造 | PASS |
| non_functional[0] | 门禁 | 见 §2 | PASS |
| non_functional[1] | 交付流程（AD-9） | 提交 `418f23ea` 锚 `feat(TASK-004): M2a …`；merge `057a91c5`；discovery 记双 sha 与 `provenance`；code-simplifier「回复无动作但实改两处」以 `git diff` 核实属实（`queue.go` 两个局部变量、`queue_test.go` `passingContract()` 8 处调用 + 无 `strings` import）；`git worktree list` 无 `wt-TASK-004-m2a`/`pre-TASK-004` 残留 | PASS（review） |

## 2. 门禁实测（验证树 `057a91c5123c8440a2c0bcc49943b1c381805df0`）

| 项 | 结果 | 门槛 |
|---|---|---|
| `GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1` | rc=0，两包 ok | 全绿 |
| 覆盖率 | `internal/hestia` **96.6%**；`cmd/atlas` **76.4%** | ≥96.6 / ≥76.4 |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，rc=0 | 零输出 |
| `gofmt -l internal/hestia cmd/atlas` | 恰 `backtest_test.go`、`crisis_test.go` | 只允许这两处 |
| 不动文件 + go.mod/go.sum（`d27791c..057a91c5`） | 0 行 | 空 |
| `Save` 函数体 grep | 0 | 0 |
| `hestia.go` import 块 `filepath` | 0 | 不 import |
| 越界申报 `git diff --stat 61403387 057a91c5` | 恰 4 文件 = `writes`（`queue.go` 44/0、`queue_test.go` 181/0、`signals_test.go` 34/0、`store_test.go` 1/1，与 discovery numstat 逐项相同） | 无声明外文件 |
| 注释前缀 | 新增行里不带 `M2a 的` 的 `TASK-004` 引用 0 条 | 0 |
| `find … pending/queue` | 命中 0（测试全部写 `t.TempDir()`） | 空 |
| 目标测试 `-v` | `TestEnsureQueueDirs\|TestWriteContract\|ExposesNoWrite\|TestEvaluateThresholdEdges\|TestFieldNamesAppearOnlyInFieldsGo` 顶层 **14** 条 `--- PASS`（`ThresholdEdges` 六子例全 PASS），与 discovery 一致 | — |

## 3. 变异测试（验证树内变异 + `git checkout` 还原；每个先打 diff、目标串不匹配即早退不计；主仓库六文件 sha256 前后一致）

| # | 变异 | 结果 | 致红测试 |
|---|---|---|---|
| S1 | `signals.go` `d >= ScissorsActive` → `>` | KILLED | 独家 `ThresholdEdges/剪刀差 d == ScissorsActive` |
| S2 | `d <= ScissorsSink` → `<` | KILLED | 独家 `…/剪刀差 d == ScissorsSink` |
| S3 | `v >= warm` → `>` | KILLED | 独家 `…/楼市月均 v == HHMltMonthlyWarm` |
| S4 | `r < BillRatioHealthy` → `<=` | KILLED | 独家 `…/票据比 r == BillRatioHealthy` |
| S5 | `r >= BillRatioSevere` → `>` | KILLED | 独家 `…/票据比 r == BillRatioSevere` |
| S6 | `sameCaliberPair` 允许 (`_mom`, `_ytd`) | KILLED | 独家 `…/混口径反向` |
| C1 | `contract.go` `AbsentFields` 初值 nil | KILLED | 独家 `AbsentFieldsEmptyArray` |
| C2 | `contract.go` `JSON()` 去末尾 `\n` | KILLED | 独家 `EndsWithNewline` |
| Q1 | `queueStates` 去 `failed` | KILLED | `CreatesStateMachine` |
| Q2 | `EnsureQueueDirs` 错误前缀去 `contract` | KILLED | `RejectsFile` |
| Q3 | 写到 `done/` | KILLED | 5 条 |
| Q4 | `writeAtomic` → `os.WriteFile` | **SURVIVED** | — |
| Q5 | 跳过 `MkdirAll(pending)` | KILLED | `CreatesPendingWithoutEnsure` 等 3 条 |
| Q6 | rename 层错误前缀改字 | KILLED | `FailsWhenTargetIsDir` |
| Q7 | 落盘前多 append 一个 `\n` | KILLED | `LandsInPending`、`EndsWithNewline` |
| Q8 | `MkdirAll` 权限 0o755 → 0o700 | **SURVIVED** | — |
| Q9 | `EnsureQueueDirs` 吞错 | KILLED | `RejectsFile` |
| Q10 | 返回 `.tmp` 路径 | KILLED | 5 条 |
| Q11 | mkdir 层错误前缀去 `contract` | KILLED | `FailsLoudly` |

**17/19 KILLED**。8 条残留子例的对应变异体全部由**自己那条子例独家**转红，dev discovery 里「7/7 KILLED、各独家转红」的自证与我重采一致（我多加了 S6 混口径反向）。存活两个都是**实现按 DoD 写对了（我逐行核对）、但该性质在现有测试下不可观测**：

- **Q4**：`writeAtomic` 与直接 `os.WriteFile` 在「目标是目录」「目标是文件」两种失败形态下的可观测结果相同（都报错、都不留半个文件）。原子性的价值在并发读者视角，单进程测试构造不出「读到半个文件」。不算测试缺口，记作结构性不可测；实现确用 `writeAtomic`。
- **Q8**：目录权限无断言。DoD 写的 0o755 已由代码核对；真实权限还受 umask 影响，断言 `Perm()` 会在不同机器上漂，不建议补。

两者不构成 `task_defect`。**本任务无需向后续任务转移残留。**

## 4. 结论

- **VERIFIED**：全部 `verify_by: test` 条目有对应测试且断言真在守卫；002/003 转来的 8 条残留子例逐条落地且各自独家守卫；`queue.go` 与需求原文逐字一致（两处等价重构已申报且经 diff 核实）；门禁逐项达标；改动文件集与 `writes` 逐一相同；红阶段可复现（偏移有确定成因）；AST 守卫达本 Sprint 终值 34。
- 无阻断残留；Q4/Q8 记为结构性不可测，不转移。
