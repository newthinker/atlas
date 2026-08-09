# TASK-007 验证报告 —— 收尾验证（公开面、依赖、全量测试）

- **验证者**：test-agent-21 ｜ **epoch**：1 ｜ **日期**：2026-08-09
- **验证对象**：`ed776a0e68d9b52c3f79af69f3d94bc864fceb3e`（分支 `feat/hestia-store`）
- **基准**：`origin/master = d7c9c694f88e4f44497a4c08e38a30aaae985413`
- **隔离环境**：`git worktree add --detach ../wt-verify-TASK-007 ed776a0e…`，全程 `GOTOOLCHAIN=local`
- **判定**：**VERIFIED —— 六条 done_criteria 全部 PASS**

## 0. 交付物形态与引用关系

本任务的产出物是 `.arcforge/discoveries/TASK-007.json`（Leader 裁定 C，原 DoD 要求的
`docs/04-test/TASK-007-final-verification.md` 在当前 write-matrix 下 **dev-\* 结构上写不了**）。

**本报告引用该 discovery，不复述其内容**——复述会产生第二份必然不同步的副本。
读者需要 dev 侧的原始命令与输出摘要时，读 `.arcforge/discoveries/TASK-007.json` 原文。

漂移核验：discovery 实测 sha256 = `b325632cebf5b61d8de3db094878b01f935a4acf14b66c2ca3f0f303eebccd1d`，
与任务文件 `verify_baseline.discovery_sha256` **逐字相同**；`git rev-parse HEAD` = `ed776a0e…`
与 `verify_baseline.head` 相同。**无漂移**。

**本报告的全部结论出自我在隔离 worktree 里自己跑的命令，未采信 discovery 的任何输出摘要。**
下文每条都给出我这一侧的实测数字；它们与 dev 的数字一致，但一致性是结论不是前提。

## 1. 完成标准覆盖矩阵

六条 DoD 全部是 `verify_by: manual|review`，**无 test 级条目**——本任务不产生可执行断言，
「逐条有对应测试」的要求按角色定义在此不适用；判据是我独立复现命令并核对输出。

| # | 维度 | 完成标准（摘要） | 我的独立复现（命令 → 实测） | 判定 |
|---|---|---|---|---|
| 1 | functional[0] | 导出面恰好是 54 个 `Field*` + 2 个表名常量 + `CheckStatus` 三常量 + 7 个 type + 4 个 func/方法；**出现 Insert/Upsert/SaveUnchecked 或任何不要求 ValidationReport 的导出写方法即 FAIL 并指名** | 见 §2 | **PASS** |
| 2 | functional[1] | `go build ./...` + `go test ./internal/hestia/... -race` + `go vet` 全绿；**G9 并发契约二选一并落到包注释** | 见 §3 | **PASS** |
| 3 | boundary[0] | `git diff <基准> --stat go.mod go.sum` 无输出 | 见 §4 | **PASS** |
| 4 | error_handling[0] | `go test ./...` 全仓无 FAIL，既有包不受影响 | 见 §5 | **PASS** |
| 5 | non_functional[0] | 产出物为 discovery，逐条记录命令与输出摘要，**每项写明所在 commit sha** | 见 §6 | **PASS** |
| 6 | non_functional[1] | 向后续子迭代交接三件事 | 见 §6 | **PASS** |

## 2. functional[0] —— 导出面（PASS）

`go doc -all ./internal/hestia` 分节枚举：

| 类别 | 实测 | 期望 | |
|---|---|---|---|
| 导出 func/方法 | `NewStore`、`(*Store).Close`、`(*Store).DB`、`(*Store).Save` —— **恰 4，无第五个** | 4 | ✓ |
| 导出 type | `Check`、`CheckStatus`、`Meta`、`Observation`、`Outcome`、`Store`、`ValidationReport` —— **恰 7** | 7 | ✓ |
| CONSTANTS 段 | 56 条 = 54 个 `Field*` + `TableObservations` + `TablePending` | 56 | ✓ |
| `CheckStatus` 常量 | `CheckPassed` / `CheckFailed` / `CheckSkipped`（归在 TYPES 段下） | 3 | ✓ |
| VARIABLES 段 | **不存在**（无任何导出包级变量） | — | ✓ |

54 这个数字用**两条互不依赖的路径**交叉核对，避免只信 `go doc` 一家：
`grep -cE '^\tField[A-Za-z0-9]+ += +"' internal/hestia/fields.go` = **54**；
`fieldOrder` 字面量内唯一 `Field*` 引用数 = **54**。

负向检查（DoD 点名的形态）：
`go doc -all ./internal/hestia | grep -nE '^func .*(Insert|Upsert|Unchecked|Write|Put|Delete|Truncate)'`
→ **rc=1、输出为空**。无需指名任何符号。

### 一次性核验 vs 常驻防线

`go doc` 只在有人跑时有效。包内两条常驻测试守同一条约束，**均为精确集合相等、均 PASS**：

| 测试 | 视野 | 期望集合 |
|---|---|---|
| `TestStoreExposesNoWriteMethods` | `reflect.TypeOf(&Store{})` 的方法集——**能看见嵌入类型提升上来的方法** | `[Close, DB, Save]` |
| `TestPackageExposesNoWriteFunctions` | `go/ast` 扫非 `_test.go` 文件的 **FuncDecl**——能看见包级函数 | `[NewStore, Store.Close, Store.DB, Store.Save]` |

**互补性我已在 TASK-005 双向实证**（本 Sprint 报告 `TASK-005-verification.md`）：
W2 新增包级 `func InsertRow(db *sql.DB, period string) error` → **只有 AST 那条红**；
W3 新增未导出接收者的提升方法（`type storeHelper struct{}` + `func (storeHelper) Bulk() error`，嵌入 `Store`）
→ **只有 reflect 那条红**，因为 AST 走查跳过未导出接收者、而 Go 的方法提升把 `Bulk` 放进了 `*Store` 的方法集。
**两条都不能删。**

本次新增核验（TASK-006 之后该防线是否仍准确）：AST 那条在 `store_test.go:278` 用
`!fn.Name.IsExported()` 过滤，故 TASK-006 引入的未导出 `savePending` 不进期望集——
期望集合未被 TASK-006 稀释。

## 3. functional[1] —— build / race / vet 与 G9 并发契约（PASS）

| 命令 | 实测 |
|---|---|
| `go build ./...` | rc=0 |
| `go vet ./internal/hestia/...` | rc=0 |
| `go test ./internal/hestia/... -race -count=1 -v` | rc=0，`ok … 2.019s` |
| 顶层 `--- PASS` | **61** |
| 含子测试 `--- PASS` | **75** |
| `--- FAIL` / `--- SKIP` | **0 / 0** |
| `DATA RACE` | **0** |
| 覆盖率 | **88.7% of statements** |
| `gofmt -l internal/hestia/` | 空 |

### G9 契约：前状态与后状态都独立核实

**前状态（dev 声称派发时缺失）**——我在 `c1050d7` 上自己 grep：

```
git grep -nE '并发|串行化|concurren|goroutine|事务|transaction' c1050d7 \
  -- 'internal/hestia/*.go' ':!internal/hestia/*_test.go'   →  rc=1，空输出
```

**契约在派发时确实不存在。** Leader 派单里那句「确认包注释里已有其中之一」预设了它存在，
dev 发现预设不成立并按 DoD 选项①补入，是正确处置。

**后状态**：`go doc ./internal/hestia` **第一屏**可见 `# 并发契约` 小节，内容满足 DoD 选项①的
要求（「同一业务键上 Save 不保证并发安全，由调用方串行化」逐字在列），并额外写明了失效形状、
为什么不加事务、将来改法（`BEGIN IMMEDIATE` 而非 Go 侧加锁）、以及 `-race` 为何看不到它。

### 「只改注释、无可执行语句改动」——机械证明

DoD 未要求这一点，Leader 点名要核实。我给三重证据，最后一条是充分的：

1. `git show ed776a0 --numstat` → **`15  0  internal/hestia/fields.go`**，单文件、零删除。
2. 新增行中「非注释非空」的行数：`git show ed776a0 -- …fields.go | grep '^+' | grep -v '^+++' | grep -vcE '^\+\s*(//.*)?$'` → **0**。
3. **去掉整行注释后逐字节比对**：
   ```
   git show c1050d7:internal/hestia/fields.go | grep -vE '^\s*//' > a
   git show ed776a0:internal/hestia/fields.go | grep -vE '^\s*//' > b
   diff a b   →  rc=0
   ```
   两侧行数均 96，**sha256 完全相同**（`6953d02404e959c0…`）。
   ⇒ 该 commit 对非注释内容的改动**为空集**，不是「看起来只改了注释」。

## 4. boundary[0] —— 无新增依赖（PASS）

| 基准 | `git diff <基准> --stat -- go.mod go.sum` |
|---|---|
| `origin/master` = `d7c9c69` | **空** |
| 本地 `master` = `a1c6ae4` | **空** |

双基准结论一致，与 dev 的交叉核验吻合。

## 5. error_handling[0] —— 全仓回归与影响范围（PASS）

```
GOTOOLCHAIN=local go test ./...        →  rc=0
```

| 指标 | 实测 |
|---|---|
| `^ok` 行 | **64** |
| `(cached)` 行 | **0**（本次是新鲜跑，不是缓存复读） |
| `[no test files]` 行 | **0** |
| 含 `FAIL` 的行 | **0** |
| 输出总行数 | 64 |
| `go list ./... \| wc -l` | **64** ← 与 ok 数吻合，**证明 64 覆盖了全部包**，没有包被静默跳过 |

范围检查（基准 `origin/master`）：

```
git diff d7c9c69 --name-only -- internal/ | grep -v '^internal/hestia/'   →  空
git diff d7c9c69 --stat -- internal/hestia/                              →  11 files changed, 3561 insertions(+)
```

**只新增 `internal/hestia/`，crisis / prism / collector / macro 一个文件都没碰。**

### 基准陷阱：我复现了它

Leader 与 dev-41 提示的陷阱**在我这里同样成立**，我照跑了错误基准取证：

| 基准 | `git diff <基准> --name-only -- internal/ \| grep -v hestia` |
|---|---|
| `origin/master` = `d7c9c69` | 空 ✓ |
| 本地 `master` = `a1c6ae4` | **10 个文件**：`internal/macro/bitemporal/{classify,classify_test,fixture_test,identifiers_test,lookup,lookup_test,query,query_test,spec,spec_test}.go` ✗ |

根因实测：`git ls-tree master internal/macro/` → **0 项**；`git ls-tree origin/master internal/macro/` → 1 项。
本地 `master` 停在 `a1c6ae4`，缺 PR#53/54/55。

**命令原文相同、基准不同、结果不同**——而错误的那个结果长得完全像一个真结果（10 个具体文件名、
可信的路径），不会以任何形式自我暴露。这与我在 TASK-003/005 交接纪律里那条同形：
**符号引用（`master`、`HEAD`）是会过期的锚，钉全 sha 才不会静默变义。**
本报告全部范围类命令的锚均为全 sha。

## 6. non_functional —— 产出物与交接（PASS）

**non_functional[0]**：产出物 `.arcforge/discoveries/TASK-007.json`（84 行）具备 DoD 要求的形态——
开头 `is_deliverable` 声明交付物身份，`verified_at_commit` = `ed776a0e…`、`baseline` = `origin/master d7c9c69`
写在文件级而非散落条目，`verification` 下六条各自带 `commands` 与输出摘要，
并有 `why_sha_on_every_item` 显式记录 sprint-032 那条「数字对得上 ≠ 测的是对的 commit」的教训。
**内容不在此复述。**

**non_functional[1]**：交接三件事对应
`.arcforge/docs/01-design/handoff-to-later-iterations.md` 的 **H6（L60–64）**，我逐条核到原文：
L62 = M1b-2 的 `Values` 键须取自 `Field*` 常量；L63 = M1b-3 的 `Check.Value` 单位自定义；
L64 = M1b-4 经 `Store.DB()` 做 `article_id` 一级幂等检查。
discovery 指向原文而非转述，与「不复述」纪律一致。

## 7. 声明范围核对（无越界）

| | 声明 | 实际 |
|---|---|---|
| `packages` | `[.arcforge/discoveries]` | ✓ |
| `writes` | `[.arcforge/discoveries/TASK-007.json, ./internal/hestia/fields.go]` | `git show ed776a0 --numstat` 全 commit 仅 `internal/hestia/fields.go` |

**无声明外文件改动。** 工作区收尾：`git status --porcelain` 空、`gofmt -l` 空、`HEAD` = `ed776a0e…`。

## 8. 观察（不阻断本次判定）

### O-1：`DB()` 是「单一写入口」约束上一个有文档、无机制的逃生口

`DB() *sql.DB` 把裸句柄交给调用方，`db.Exec("INSERT …")` 可完全绕过 `Save` 与 `ValidationReport`。
约束只写在 doc 注释里（「DB 暴露只读用途的句柄……写入仍然只能走 `Save`」），**没有任何机制阻止写**——
两条写口守卫看的是「导出符号集合」，它们对通过 `DB()` 拿到句柄后的行为完全不可见。

**不据此判 FAIL**：DoD 的期望导出面里明列 `DB`，H6 第 3 条还要求 M1b-4 经 `Store.DB()` 查幂等，
这是有意保留的口子，不是本任务的缺陷。

记下它是因为它与本任务刚补的 **G9 并发契约同形**：两者都是**契约而非机制**，
都无法被任何测试验证调用方是否遵守。本包目前有两条这样的约束，建议 final-report 归一登记，
让「哪些约束靠自觉」成为一份看得见的清单，而不是分散在两处注释里。

### O-2：DoD 的基准写法（已由 Leader 认领）

`boundary[0]` 与 `error_handling[0]` 都以 `master` 为基准而未写 `origin/master`。
依赖类结论不受影响（双基准一致），**范围类结论会失真**（§5 已实测）。
Leader 已认领此为 DoD 撰写错误并记入 final-report；此处仅登记，无需额外动作。

## 9. 结论

六条 done_criteria **全部 PASS**，全部由我在隔离 worktree @ `ed776a0e…` 独立复现命令得出。
声明范围与实际改动一致，无越界。工作区洁净，全仓 64 包零 FAIL，既有包零改动。

**判定：VERIFIED。**
