# TASK-002 验证报告（verifier: test-m3-b）

> 结论：**VERIFIED**。6 条 `done_criteria` 全部 PASS，**未发现交付缺陷**。
> 另记 2 条 DoD 侧偏差（⑤.1 文件数笔误、⑤.2「已订正」的行号锚仍不准）与 2 条我自己的仪器失误（⑥）。
>
> 本报告每条结论都来自我在**独立 worktree** 上的实跑与**变异测试**，不采信 discovery 写着「绿」。

## ⓪ 验证元信息

| 项 | 值 |
|---|---|
| 任务 | TASK-002（≡ 需求文档 TASK-001 的**下半**，`cmd/atlas`），`verifying` → `verified`，`assignment_epoch=1` |
| `verify_baseline.head` | `5ba7c3ceca25f5c2495faf32e4ce1c38e92f90ca` |
| **裁决落盘当刻 HEAD** | `5ba7c3ceca25f5c2495faf32e4ce1c38e92f90ca` —— **与 baseline 逐位相同，零漂移**，未用 `--ack-drift` |
| `verify_baseline.discovery_sha256` | `f8bed34d2991128637c9a6ecd0a56e7662347d73fae88b5ca97d12ddafade399`（判定前后两次**实算**均一致） |
| 交付 commit | `7b38ef91b48ed3afebfc29671d70792be416ca4f`（分支 `task/TASK-002-m3` 保留） |
| merge commit | `d62ff27b53e01d40fca1431ebbba7a9d03a0f7a0` |
| 基线锚 | `1991c49ec5541fd4d4c0d0cd23117b389e7d00d1` |
| 验证方式 | scratchpad 内两棵 `--detach` 对照 worktree（`1991c49` / `5ba7c3c`），验毕拆除；主仓库 `cmd/` `internal/` `go.mod` `go.sum` 全程 `git status --porcelain` **0 条** |

**归因前提已核实**：`1991c49` → `5ba7c3c` 之间的 `.go` 改动**恰为本任务这 2 个文件**
（其余是 `PENDING-MECHANISMS.md` 与 `docs/hestia-m3/*.md`）⇒ 覆盖率差异可唯一归因于本任务。

---

## ① done_criteria 覆盖矩阵（6 条逐条）

| # | 完成标准（摘要） | 我跑的验证 | 判定 |
|---|---|---|---|
| **functional[0]** | `!hestiaEmitStdout` 时 `BuildHistory`→`WriteHistory`、两处 err 直接 return、`generated_by` 取 `c.GeneratedBy` | 读 `hestia.go` 全 15 行改动；**变异 M-GENBY** 独家杀 `TestHestiaContractEmitWritesHistory`（③.2） | **PASS** |
| **functional[1]** | 新测试 PASS：非 `--stdout` 时侧车存在；`--stdout` 时 stdout **不含** `"same_type"`；复位包级变量 | 10 条 emit 用例全 PASS，两把尺同为 10（②.3）；用既有 `restoreEmitGlobals` + `newCapturingCmd`（已在 `decisions` 申报，理由成立） | **PASS** |
| **boundary[0]** | `--stdout` **不写任何文件、不建 `pending/`**；queue 目录靠 `WriteHistory` 的 `MkdirAll` 建 | **区分性变异**精确刻画了这条断言的守卫力（③.1）；`find` 独立复查 `pending`/`queue` 目录 = **0** | **PASS** |
| **error_handling[0]** | TDD 红痕迹；`BuildHistory`/`WriteHistory` 任一失败 ⇒ 非 nil error **且不写契约** | RED 重放复现 dev 所记的**同一条**失败（含同一句 message），退出码 1（②.4）；**变异 M-ORDER2** 让**两条** SkipsContract 测试同时转红（③.3） | **PASS** |
| **non_functional[0]** | 两包门禁 + 两个覆盖率门槛 + gofmt/vet/无新依赖 + `filepath` 守卫 | 精确语句数复算（②.1）；新增语句**零未覆盖**（②.2）；**变异 M-IMPORT** 证实 AST 级守卫真在守（③.4） | **PASS** |
| **non_functional[1]** | 交付流程 / commit subject / merge / 数字统一重采 | 交付 commit 恰 **2** 文件与 `writes` 逐项一致、**零越界**；2 文件在交付 commit 与 baseline.head 上 sha256 逐一相同；subject `feat(TASK-002): …` 匹配门禁 `^[a-z]+\(TASK-002\):` | **PASS** |

---

## ② 实跑证据

### 2.1 覆盖率 —— 背对背 × 精确语句数复算

**第一层：背对背两轮**（同一时刻并排跑，同等负载）

| 轮 | `cmd/atlas` base | `cmd/atlas` head | `internal/hestia` base | head |
|---|---|---|---|---|
| 1 | 76.6% | 76.7% | 96.6% | 96.6% |
| 2 | 76.6% | 76.7% | 96.6% | 96.6% |

**第二层：从 coverprofile 精确复算**（`76.6%` 是显示值）

| 树 | 覆盖 / 总计 | 精确值 |
|---|---|---|
| base `1991c49` | 1112 / 1452 | **76.5840%** |
| head `5ba7c3c` | 1118 / 1458 | **76.6804%** |

⇒ **上升 +0.096pp**。DoD 下限 `≥76.6` 满足。`internal/hestia` 96.6% 未因本任务回退。

> 📌 与我在 TASK-001 上报的同一个口径问题：**精确基线 76.5840% 其实低于 76.6**，只是显示成 76.6。
> 判定不受影响（head 76.6804% 严格高于门槛），但「基线恰 76.6 = 门槛」这个说法建立在四舍五入上。

### 2.2 新增语句零未覆盖 —— 与 TASK-001 的关键差别

| 指标 | base | head | 增量 |
|---|---|---|---|
| `cmd/atlas` 总语句 | 1452 | 1458 | **+6** |
| `cmd/atlas` 已覆盖语句 | 1112 | 1118 | **+6** |
| `hestia.go` 未覆盖块 | **13** | **13** | **0** |
| `runHestiaContractEmit` 函数覆盖率 | 83.3% | **86.1%** | +2.8pp |

**新增的 6 条语句全部被覆盖，零新增未覆盖块。** 这正是我在 TASK-001 上发现缺口的那一类位置
（调用点的错误分支），而**本任务 dev 主动写了两条错误路径测试把它们全覆盖了**。

### 2.3 测试条数 —— 两把尺

| 尺 | 值 |
|---|---|
| 静态 `grep -c '^func TestHestiaContractEmit'` | **10** |
| 动态 `go test -run '^TestHestiaContractEmit' -v` 顶层 `--- PASS` | **10** |

任意层 `--- FAIL` = **0**。基线 7 条 + 新增 3 条 = 10。

### 2.4 TDD 红阶段重放

在锚点树上把 `cmd/atlas/hestia.go` 单独 checkout 回基线（去掉侧车块）后跑新测试：

```
--- FAIL: TestHestiaContractEmitWritesHistory
    Received unexpected error:
      open …/q/pending/2025-12-annual.history.json: no such file or directory
    Messages: 回放也要产出侧车，消费者不分实时与回放
退出码 1
```

与 discovery `red_phase_evidence` 所记**是同一条失败、同一句 message**，且失败原因确是「侧车不存在」
而非编译错或夹具问题。

### 2.5 门禁

```
$ GOTOOLCHAIN=local go test ./cmd/atlas/... ./internal/hestia/... -count=1   → 两包 ok，退出码 0
$ go vet ./cmd/...                                                          → 零输出，退出码 0
$ gofmt -l internal/hestia cmd/atlas   → 恰 2 项（backtest_test.go / crisis_test.go）
        基线树同一命令背对背跑                → 同为 2 项 ⇒ 确系既有欠账，判据成立
$ git diff --numstat 1991c49… 5ba7c3c… -- go.mod go.sum   → 0 行（无新增依赖）
$ git diff --numstat 1991c49… 5ba7c3c… -- internal/       → 0 行（本任务不碰 internal）
$ find cmd/atlas internal/hestia -type d \( -name pending -o -name queue \)  → 0 个
```

`internal/hestia` 的 AST 守卫仍 **38 项**且 PASS（本任务不新增任何导出符号）。

---

## ③ 变异测试

全部在 scratchpad 独立 worktree 上进行，每体过**语义闸**（打印 diff 逐字核对）与**编译闸**，
跑完还原并核对 **sha256**；主仓库 `cmd/` `internal/` 全程零改动。

| 变异 | 改动 | 转红 | 结论 |
|---|---|---|---|
| **M-IMPORT** | `hestia.go` 加 `import "path/filepath"` | `TestHestiaCmdDoesNotResolveDBPath` | KILLED ✔ |
| **M-GENBY** | `c.GeneratedBy` → 硬写 `"contract@v1"` | `TestHestiaContractEmitWritesHistory`（**独家 1 红**） | KILLED ✔ |
| **M-ORDER2** | 侧车块挪到契约写出**之后** | `…HistoryBuildFailureSkipsContract` + `…HistoryWriteFailureSkipsContract`（**2 红**） | KILLED ✔ |
| **M-STDOUT** | 去掉 `!hestiaEmitStdout` 守卫（`--stdout` 也写侧车） | `TestHestiaContractEmitStdoutDoesNotWrite` | KILLED ✔ |
| **M-DIR** | `--stdout` 建出 `pending/` 但不写文件 | 见 ③.1 | KILLED ✔（且**具区分力**） |

### 3.1 code-simplifier 那处改动 —— 精确刻画，而不是「能红就完事」

**改动确实在提交里**（`hestia_test.go` diff 可见），形态是：

```go
// 旧（dev 原写，code-simplifier 指出）
entries, derr := os.ReadDir(filepath.Join(queueDir, "pending"))
if derr == nil { assert.Empty(t, entries, "…") }

// 新（已采纳）
_, derr := os.Stat(filepath.Join(queueDir, "pending"))
assert.True(t, os.IsNotExist(derr), "--stdout 单独跑 ⇒ pending/ 一个文件都不该有，目录本身也不该建")
```

我做了三步，结论比「新写法能红」更精确：

**实验 1（插桩，直接观察而非推断）**：在**原实现**下把断言换成旧写法并加执行标记：

```
PROBE blockRan=false derr=open …/q/pending: no such file or directory
```

⇒ **旧写法的 `if derr == nil` 块确实从不执行**。「永不执行因而永不报错的断言」这个定性**成立**，
是我观察到的，不是接受的说法。

**实验 2（M-STDOUT）**：施加「`--stdout` 也写侧车」的变异后，**新旧写法都转红**。
旧写法之所以也能红，是因为**这个缺陷本身把目录建了出来**，`derr` 翻成 `nil`，那个 `if` 块于是执行了。
⇒ 单看这个变异，**分不出新旧写法的优劣**。

**实验 3（M-DIR，区分性变异）**：构造一个「`--stdout` 建出 `pending/` 但不写任何文件」的缺陷
（只读命令产生目录副作用）：

| 断言写法 | 结果 |
|---|---|
| 新写法（已采纳） | **FAIL（抓住）** |
| 旧写法 | **PASS（缺陷逃逸）** |

⇒ **改动有实质收益，收益点是「断言不再是绿路径上的 no-op」**——它现在能抓住那些**不会顺带建出目录**
的缺陷。但要说清楚：它**不是**「唯一能抓住 `--stdout` 写侧车」的东西（旁边 dev 自己那条无条件的
侧车文件断言 `os.IsNotExist(herr)` 也抓得住）。**dev 保留 code-simplifier 这处修改是对的。**

### 3.2 M-GENBY —— 「钉死具体值而非非空」被实测证实

测试写的是 `assert.Equal(t, "contract@v1/replay", h["generated_by"])`，注释里给的理由是
「钉死具体值而不是『非空』——传错成 `contractGenerator` 时『非空』照样绿」。
把实现改成硬写 `"contract@v1"` 后，该测试**独家转红**（其余 9 条 emit 用例全绿）⇒ 理由成立，
`generated_by` 随回放走这条语义有唯一闸守着。

### 3.3 M-ORDER2 —— 「侧车在契约之前」有闸

把侧车块挪到契约写出之后，`…HistoryBuildFailureSkipsContract` 与 `…HistoryWriteFailureSkipsContract`
**同时转红**。这两条测试的核心断言是「侧车失败时**契约不该被写出**」。

> 📌 **与 TASK-001 的对照值得记一笔**：我在 TASK-001 报的缺口正是「侧车写失败 ⇒ 契约不写」**没有断言**
> （`ingest.go` 两个新错误分支 `count==0`）。**同一片代码的另一端，本任务 dev 主动补上了**——
> 两条 SkipsContract 测试 + 新增语句零未覆盖。两个任务同一位 dev，本任务这半边更严。

### 3.4 M-IMPORT —— `filepath` 守卫是 AST 级的，不是 grep

`TestHestiaCmdDoesNotResolveDBPath` 用 `parser.ParseFile(fset, "hestia.go", nil, parser.ImportsOnly)`
遍历 `f.Imports` 逐个断言，**不是**文本 grep。加入 `import "path/filepath"` 后它转红 ⇒ 真在守。

---

## ④ 越界申报

**声明范围**：`writes` = `["./cmd/atlas/hestia.go", "./cmd/atlas/hestia_test.go"]`。

```
$ git show --numstat --format='' 7b38ef91b48ed3afebfc29671d70792be416ca4f
 15	0	cmd/atlas/hestia.go
131	0	cmd/atlas/hestia_test.go        ← 恰 2 个，与 writes 逐项一致
```

**零越界。** 合入用**内容判据**核实（不用拓扑判据）：2 个文件在 `7b38ef91…` 与 `5ba7c3c…` 上
sha256 **逐一相同**。commit subject 匹配门禁正则。

---

## ⑤ DoD 侧偏差（留痕用，均不构成 rejected）

### ⑤.1 🟡 `non_functional[1]` 写「本任务 **3** 个文件」，实为 **2**

| 来源 | 值 |
|---|---|
| DoD `non_functional[1]` 第 2 步 | **3** |
| `writes` | **2** |
| `estimated_files` | **2** |
| 交付 commit 实际文件数 | **2** |

Leader 已认领为笔误（TASK-002 初稿含 `internal/hestia/CONTRACTS.md`，后因 reviewer O9 把整节移给
TASK-008，文件数没跟着改）。dev 也在 `key_findings` 里独立发现并记录了。
**⇒ 数出 2 个是正确的，不是缺陷。**

### ⑤.2 🟡 DoD 里「已订正」的守卫行号锚**仍然不准**

DoD `non_functional[0]` 说 `TestHestiaCmdDoesNotResolveDBPath` 在 `cmd/atlas/hestia_test.go:135-145`
（断言 141-142），并注明初稿的 `116-124` 是错锚。**我现读文件，实际位置是第 147 行**：

```
147:func TestHestiaCmdDoesNotResolveDBPath(t *testing.T) {
```

135-137 是 `TestHestiaCommandsSilenceUsage` 的尾部，139-146 是那段注释块。

**不影响判定**（守卫存在且 PASS，M-IMPORT 证实它真在守），但值得留痕：**这是同一个锚第二次被订正
而仍然不准**。行号是会随任何编辑漂移的锚，和 `HEAD`/分支名同类。建议后续一律用**符号名**
（`TestHestiaCmdDoesNotResolveDBPath`）作锚，让读者自己 `grep -n` 定位——符号名不会因为
上方插入了几行注释就失效。我全程未照任何文字里的行号。

### ⑤.3 🟢 `deviates_from_requirement` 结构化求值：**true 3 / false 1**

```bash
jq '[.decisions[] | select(.deviates_from_requirement == true)] | length' .arcforge/discoveries/TASK-002.json   # 3
jq '[.decisions[] | select(.deviates_from_requirement == false)] | length' .arcforge/discoveries/TASK-002.json  # 1
```

三条偏离（`restoreEmitGlobals`+`newCapturingCmd` 而非原文全局 `SetOut`、新增两条错误路径测试、
commit subject 编号）逐条复核成立。**这次我向产物求值，不向摘要求值**——上一份报告我在这里栽过。

### ⑤.5 🔴 覆盖率有两把尺，而**门禁用的那把是系统性偏高的**（我追到了成因）

Leader 转来 dev 的发现：同一棵树 `go test -cover` 报 **76.7%**、门禁（`task-completed.sh:477`）用的
`go tool cover -func | grep total:` 报 **76.8%**。我在**同一份 coverprofile** 上跑了三把尺：

| 读法 | 值 |
|---|---|
| `go tool cover -func` 的 `total:` 行（**门禁用的**） | **76.8%** |
| `go test ./cmd/atlas/ -cover` 行内报告 | **76.7%** |
| 从 coverprofile 直接数语句（**本报告 ②.1 用的**） | **76.6804%**（1118 / 1458） |

**成因我追到了，且是可精确复核的**——用 `go/ast` 解析 `cmd/atlas` 全部 483 个**具名函数声明**的行区间，
再把 profile 的 926 个块逐个归属，得到**恰好 1 个**「不属于任何具名函数」的孤儿块：

```
cmd/atlas/version.go:18.47,22.3   语句 3   覆盖计数 0
```

它是**包级 `var` 声明里的匿名函数字面量**——cobra 的经典写法：

```go
var versionCmd = &cobra.Command{
	Use: "version", Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {   // ← 第 18 行，3 条语句，一条都没被覆盖
		fmt.Printf(...); fmt.Printf(...); fmt.Printf(...)
	},
}
```

`go tool cover -func` **只聚合具名函数声明**（该文件它只列出了 `init`），这 3 条语句于是被**静默丢弃**。
剔除它复算：`1118 / 1455 = 76.8385%` → 显示 **76.8%**，与 `-func` 的输出**精确吻合**。

**这不只是「两把尺不一样」，它有方向**：被丢弃的 3 条语句**全部是未覆盖的**（covered 0 / uncovered 3）
⇒ **`-func` 的 `total:` 系统性偏高**。也就是说：

- `go test -cover` 与语句数口径是**诚实**的；**dev 在 discovery 里写的 76.7% 是对的**，
  偏高的是门禁那把尺，不是 dev 的数字。
- **门禁 `task-completed.sh:477` 用的正是偏高的那把。** 今天的缺口只有 0.16pp，但它**随 cobra 惯用法
  规模化**：每多一个 `var xCmd = &cobra.Command{Run: func(...){…}}` 且函数体未被覆盖，
  门禁看到的数就比真实值更高一点，而且**这个偏差不会在任何输出里显形**。
- 极端情形下门禁可以在真实覆盖率**低于**门槛时放行。

**对本次判定无影响**：本报告 ②.1 用的是三把尺里**最保守**的那把（76.6804%），
它已经 ≥ 门槛 76.6，故 PASS 在三把尺下都成立。

**建议**（给 Leader，属机制层不属本任务）：若要让门禁的数与「真实覆盖率」一致，把
`task-completed.sh` 那行换成从 coverprofile 直接求和（`awk` 两列相加），或改用 `go test -cover` 的行内值。
⚠️ `.claude/hooks/` 是运行时资产、对我只读，我不改，只报。

### ⑤.4 🟡 `questions[0].answer` 仍为 `null`

`tasks/TASK-002.json` 的 `questions[0]` 是 dev 在等 merge 时转 `blocked_clarification` 留下的活性信号。
merge 已由 Leader 完成（`d62ff27b…`）、dev 也已自行转回 `in_progress` 并走完交付，但**那条 `answer`
从未被回填**。不影响本次判定（任务早已离开 `blocked_clarification`），但它是文件级真相源里的一处
悬空记录——dev 当初特意写「把障碍写进本条 `answer` 即可，写进文件比发消息可靠」，而回复走了消息。
提请 Leader 决定是否补一句归档。

---

## ⑥ 我自己的两处仪器失误（当场发现并纠正，一并留痕）

1. **`grep -c 'path/filepath' cmd/atlas/hestia.go` 得 2**——我一度以为与 discovery 的「0 处」矛盾。
   实为两处都在**注释**里（157、276 行提到「不 import path/filepath」），import 块中没有；
   而守卫走的是 **AST**（`parser.ImportsOnly`），dev 的判据 `grep -c '"path/filepath"'`（带引号形态）得 0，
   **dev 是对的，我的仪器量错了性质**（「文件里提到这个串」≠「import 了这个包」）。
2. **RED 重放后工作树没还原干净**：`git checkout <commit> -- <path>` 会把该版本**写进暂存区**，
   随后的 `git checkout -- <path>` 是从**暂存区**恢复的 ⇒ 还原成了基线版而非锚点版，
   `git status` 显示 `M cmd/atlas/hestia.go`。改用 `git restore --source=HEAD --staged --worktree`
   并用**内容判据**（与 master 上那份比 sha256）确认还原完整后，复跑门禁确认数字仍成立。
   ⇒ 记给后来人：**RED 重放用 `git checkout <sha> -- <path>` 之后，必须用 `git restore --source=HEAD
   --staged --worktree` 还原，且用 sha256 比对而不是只看 `git status`。**

---

## ⑦ 结论

**VERIFIED。未发现交付缺陷。**

6 条 `done_criteria` 逐条 PASS。5 个定向变异逐条证实关键断言真在守卫；覆盖率经**背对背 + 精确语句数
复算**为**上升**；**新增 6 条语句零未覆盖**；范围恰 2 文件、零越界；RED 重放复现 dev 所记的同一条失败。

**给 Leader 的三句话**：

1. **你点名的 code-simplifier 那处改动，我给的不是「能红」而是一个刻画**：旧写法在绿路径上**确系
   no-op**（插桩观察到 `blockRan=false`）；但「`--stdout` 也写侧车」这个变异**新旧都能抓**（旧的能抓
   是因为缺陷自己建出了目录）；**只有「建出目录但不写文件」这个区分性变异**才把两者分开——新写法抓住、
   旧写法逃逸。**保留这处修改是对的**，收益点在「断言不再是 no-op」。
2. **我在 TASK-001 报的那个缺口，本任务的同一位 dev 在这一端主动补上了**（两条 SkipsContract 测试，
   且新增语句零未覆盖）。同一片代码的两端，这半边更严。
3. **⑤.2 值得改个做法**：`TestHestiaCmdDoesNotResolveDBPath` 的行号锚**第二次订正后仍然不准**
   （实际在 147 行）。行号和 `HEAD`/分支名一样是会漂的锚，建议以后一律写**符号名**让读者自己
   `grep -n`。另 ⑤.4 有一条 `questions[0].answer` 悬空为 `null`，你看要不要补一句归档。
