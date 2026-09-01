# TASK-007 验证报告

- **验证者**：test-m1c3b-a
- **被验交付**：commit `3c34716ad1e0f93b6afc1e91ca9e88c693191556`，经 merge `6ff4b456983a0b149e4922d517e3689c4123ff9d` 进 master（`6ff4b45` 的第二父即 `3c34716`）
- **verify_baseline**：head `6ff4b456983a0b149e4922d517e3689c4123ff9d` / discovery_sha256 `62f3067741146501909e5aac0b47322c0afc135d857d02268a44a53e987480cf`
- **我测量时的 HEAD**：`6ff4b456983a0b149e4922d517e3689c4123ff9d`（= 基线，**零漂移**）
- **assignment_epoch**：1
- **结论：PASS（verified）**

---

## 0. 漂移、越界、依赖

| 检查 | 结果 |
|---|---|
| `verify_baseline.head` vs 我测量时 HEAD | 均为 `6ff4b45`，**零漂移** |
| `discovery_sha256` | `62f30677…`，与基线逐字相同 |
| 合入的是否逐字节是 dev 那版 | `git diff 3c34716 HEAD -- <两个文件>` = **0 行** |
| 声明 `writes`（2 项）vs 实际改动 | `cmd/atlas/hestia.go`、`cmd/atlas/hestia_test.go`，**逐一对应，无越界** |
| `go.mod` / `go.sum` | 命中 **0** 条 |
| 注释里任务编号 | 新增行里 5 处 `TASK-` 引用，**全部带 milestone 前缀**（`M1c-3b 的 TASK-007` ×2、`M1c-3b 的 TASK-006` ×2、`M1c-1 TASK-010` ×1），**无裸编号** |

### 0.1 `hestia.go` 的 `75/3` —— 那 3 处删除**没有真删掉任何东西**

| 被删的行 | 性质 | 我的内容判据 |
|---|---|---|
| `hestiaBackfillExpectPeriods    int` | gofmt 重新对齐 | 该标识符改前命中 **4 行** / 改后命中 **4 行** |
| `hestiaBackfillExpectArticles   int` | gofmt 重新对齐 | 该标识符改前命中 **4 行** / 改后命中 **4 行** |
| `hestiaBackfillCmd.AddCommand(hestiaBackfillFetchCmd, hestiaBackfillCalibrateCmd)` | 改写成含新子命令的两行 | 改后为 `AddCommand(hestiaBackfillFetchCmd, hestiaBackfillCalibrateCmd,` + `hestiaBackfillLoadCmd)` —— **原有两个子命令都还在** |

对齐变化的成因也确认了：同一 `var` 块里新增了更长的标识符 `hestiaLoadAllowIncomplete`，gofmt 因此重排整块。

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | `Use`/`Short`/`Long` **逐字照抄**需求文档 Task 7 Step 3；挂到 `hestiaBackfillCmd` | §2、§3 | **PASS** |
| functional[1] | 三个包级 flag；`--db` required 且**无默认值** | §3 | **PASS** |
| functional[2] | 缺 `--db` ⇒ 报错点名该 flag | §4 | **PASS**（有 Leader 已裁决的字面偏离） |
| functional[3] | 🔴 断言报告**真的到了 stdout**（阻断级缺口 A-2） | §5 | **PASS** |
| boundary[0] | `hestia.go` 不得 import `path/filepath`；`--db` 存在性检查留在 `BackfillLoad` | §6 | **PASS** |
| error_handling[0] | `go test ./... -count=1` 全绿 | §7 | **PASS** |
| non_functional[0] | gofmt / vet / 覆盖率 ≥ `coverage_floor 75` / 无新依赖 / 编号带 milestone 前缀 | §7 | **PASS** |
| non_functional[1] | AD-4 交付流程 | §7 | **PASS** |

---

## 2. functional[0]：逐字照抄独立复现

需求文档有**两份**，我先确认它们是同一份：

| 位置 | 行数 | sha256 |
|---|---|---|
| `~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-31-hestia-backfill-load.md` | 1408 | `667ed7d0341230a94e5bac42b15b257a87acca2fed9c859b48f722cdeee1cd53` |
| scratchpad 同级 `tasks/bhs7nbz6m.output` | 1408 | `667ed7d0341230a94e5bac42b15b257a87acca2fed9c859b48f722cdeee1cd53` |
| `diff` | — | **0 行** |

**比对结果**（用 hestia 仓那份作参照）：

| 段 | 文档行号 | 交付行号 | `diff` 行数 |
|---|---|---|---|
| `Use` + `Short` | 1230-1231 | 117-118 | **0** |
| **`Long`（10 行）** | 1232-1241 | 119-128 | **0**（两侧 sha256 均 `f99989a81e5a80a8b87262719f28233ac0f46856043e93ed033e3a8b1db29310`） |
| **整个命令块（15 行）** | 1229-1243 | 116-130 | **0** |

DoD 点名 `Long` 必须写清的三件事，逐条命中：

```
must NOT already exist                   命中 1
one-off                                  命中 1
never touches the production database    命中 1
never edits configs/hestia.yaml          命中 1
```

> ⚠️ 取证过程记一笔：我第一次用 `Long: \`Parse every supported report` 作锚串定位，**匹配到两行**（103 与 119）—— `backfill calibrate` 的 `Long` 也以同样字样开头。改用精确行号后才对。这正是「针串 grep 只该做定位、不该做判定」的又一例。

---

## 3. functional[0]（注册）与 functional[1]（flag）

`init()` 里（`hestia.go:221-236`）：

```go
load := hestiaBackfillLoadCmd.Flags()
load.StringVar(&hestiaLoadDir, "dir", "", …)
load.StringVar(&hestiaLoadDB,  "db",  "", …)   // ← 默认值传空串
load.BoolVar(&hestiaLoadAllowIncomplete, "allow-incomplete", false, …)
for _, name := range []string{"dir", "db"} { hestiaBackfillLoadCmd.MarkFlagRequired(name) }
hestiaBackfillCmd.AddCommand(hestiaBackfillFetchCmd, hestiaBackfillCalibrateCmd,
    hestiaBackfillLoadCmd)
```

- 三个 flag 均为**包级变量**，沿用 `hestiaCalibrateDir` 的做法 ✓
- `--dir` 与 `--db` 均 `MarkFlagRequired` ✓
- **`--db` 无默认值**（注册处传 `""`）✓
- 子命令挂到 `hestiaBackfillCmd` ✓

**`TestHestiaBackfillLoadIsRegistered` 的判据是对的**，两处值得点出：

1. 判「注册了没有」用 `rootCmd.Find([]string{"hestia","backfill","load"})` 而**不是**「那个变量非 nil」—— 后者对一个建了命令却忘了 `AddCommand` 的实现同样为真，而那种实现在 CLI 上根本调不出来。
2. 判「无默认值」断的是 **`cmd.Flags().Lookup("db").DefValue`** 而不是读包级变量 —— **这是 Leader 让我确认的那一点，我确认它确实钉住了目标性质**：包级变量会被测试间的重置逻辑（`calExec` 的 `t.Cleanup` 里就有）抹成空串，断变量的用例对「注册时给了默认值」这个真实缺陷**免疫**；而 `DefValue` 记的是 `StringVar` 注册那一刻传进去的值，重置不会动它。

---

## 4. functional[2]：`--db` 错误串 —— 字面不可满足，我自己实测过

DoD 与需求文档都写「错误串含 `--db`」。**我直接把 cobra 的实际错误打了出来**（临时探针，跑完即删）：

```
[E] cobra 实际错误串 = "required flag(s) \"db\" not set"
[E] 含 "--db" = false ; 含 "db" = true ; 含 "required" = true
```

⇒ **`--db` 这个字面在 cobra 的产出里根本不存在**。要让它出现，只能放弃 `MarkFlagRequired` 自己手写检查。

**同文件既有两条 required-flag 用例也都断裸名**（我核过）：`hestia_test.go:733` 断 `"out"`、`:895` 断 `"dir"`。

dev 改断 `"db"` + `"required"`，并在注释里写明了偏离与理由。**加断 `"required"` 使断言更强**：只断 `"db"` 会被任何含 db 字样的别的错误冒名满足，加上它才钉住「是必填未给这条路径拦下的」。

Leader 的裁决已落盘（`.arcforge/docs/03-progress/plan.md` 第 50 行的裁决记录，四条理由），**本报告不判违反**。这是需求文档「未经实测的措辞」的本 sprint 第三例。

---

## 5. functional[3]：核心验收点 —— 我用**三个变异**验那条断言守的是不是目标性质

`TestHestiaBackfillLoadWritesReport` 用 `calExec` 取 stdout，断非空 + 含「四道恒等式」「合并组明细」。

**光看它绿不够**——我要回答的是「杀死目标失效的是不是这条断言本身」。三个变异（隔离 worktree，主仓库一字节不碰，对照组 FAIL 0）：

| 变异 | 内容 | FAIL 条数 | 结果 | 杀死它的断言 |
|---|---|---|---|---|
| **A** | 删掉 `Out: cmd.OutOrStdout(),` 整行 | **1** | **KILLED** | `hestia_test.go:991` 的 `require.NoError` |
| **B** | `Out: cmd.OutOrStdout()` → `cmd.ErrOrStderr()` | **0** | **SURVIVED** | —（等价变异，见下） |
| **C** | `Out: cmd.OutOrStdout()` → `io.Discard` | **1** | **KILLED** | **`hestia_test.go:993` 的 `require.NotEmpty`** —— 报 `Should NOT be empty, but was`，Messages 正是「命令必须真的打印报告——零字节 + 退出码 0 正是本条要防的形态」 |

三个变异都验过有效性（sha256 确实改变、逐行 diff 打印、`go build` 通过），收尾 sha256 复位、worktree 干净。

**结论分三层，逐层说清**：

1. **变异 C 是 DoD 点名的那个失效形态**（「命令把观测正确写进库、返回 exit 0、而 stdout 一片空白」），它**被杀死了，而且杀死它的正是为它写的那条断言**，不是别的用例连坐 —— **外溢度 1**。这是 `functional[3]` 成立的直接证据。
2. **变异 A（漏填 `Out`）也被杀死，但杀死它的是 `require.NoError`**，因为 TASK-006 的 `BackfillLoad` 对 nil `Out` 直接报错。⇒ 严格说，「漏填 `Out`」这条路径**本来就不静默**（TASK-006 已堵上）；真正需要这条断言守的是变异 C 那种「填了、但填成了不打印的东西」。**DoD 的措辞与实际的分工略有出入，但这不影响判定** —— 该守的形态确实被守住了。
3. **变异 B 存活，成因是等价变异，不是断言弱**：`calExec`（`hestia_test.go:798`）同时 `rootCmd.SetOut(&buf)` 与 `rootCmd.SetErr(&buf)`，**两条流指向同一个 buffer**（804-805 行）⇒ 在测试装置下 `OutOrStdout()` 与 `ErrOrStderr()` 返回同一对象，变异 B 不产生任何可观测差异。这是共用 harness 的既有形状（`bfExec` 同样合并），**早于本任务，不是 dev 引入的**。

> ⇒ 残余缺口只有一条、且危害小：这条测试无法区分「报告到 stdout」与「报告到 stderr」。而运维在终端上两者都看得见，与 DoD 要防的「一片空白」不是一回事。**不构成缺陷，记在此备查。**

---

## 6. boundary[0]：`hestia.go` 不 import `path/filepath`

| 口径 | 结果 |
|---|---|
| 全文 `grep -c 'path/filepath'` | **2**（第 142、249 行） |
| 那两行是什么 | **都是注释**：`// …这一层若自己 os.Stat 就要 import path/filepath，` / `// …TestHestiaCmdDoesNotResolveDBPath 用「不 import path/filepath」钉住这一点。` |
| **import 块口径** | **0** |
| **权威判据** `TestHestiaCmdDoesNotResolveDBPath` | **PASS** |

dev 在 discovery 里记了这个陷阱（先用全文 grep 得 2、与守卫 PASS 矛盾），我复现了，结论一致：**`grep` 数的是「文件里提到过这个字符串」，守卫数的是「import 声明里有没有它」，两个不同的性质**。dev 还指出了反事实的危险——若那条 grep 恰好返回 0，它会被当成独立佐证，而它对「注释写了但 import 没有」这个真实情况**恰好也给 0**。这个自我披露质量很高。

`--db` 的存在性检查确实留在 `BackfillLoad`：`runHestiaBackfillLoad` 只把 `DBPath: hestiaLoadDB` 透传，cmd 层不碰路径。

---

## 7. 门禁与交付流程

### 7.1 覆盖率：**两把尺各自复算，两次取样**

| 尺 | 命令 | 第一次 | 第二次 |
|---|---|---|---|
| **门禁口径** | `go test ./cmd/atlas -coverpkg=./cmd/atlas -coverprofile=… ` 再 `go tool cover -func \| grep total:` | **75.9%** | **75.9%** |
| **per-package** | `go test ./cmd/atlas/ -cover` | **75.7%** | **75.7%** |
| 对照（未波及） | `go test ./internal/hestia/ -cover` | **96.2%** | — |

全部测于 HEAD **`6ff4b456983a0b149e4922d517e3689c4123ff9d`**。

- **两个数都稳定（各自两次一致）⇒ 0.2pp 的差是口径差，不是抖动。** 与 dev 报的两个数逐一相符。
- **判据是 `coverage_floor: 75`，两个口径都达标。**
- ⚠️ 我按自己的旧教训核了一遍「这个字段真的有消费者吗」（编错的字段名会静默走完全流程）：`task-completed.sh:401` 用 `jq -r '.coverage_floor // empty' .arcforge/tasks/${TASK_ID}.json` 读它、`:403` 打印覆盖、`:408` 用它判 BLOCKED。**确有消费者，不是零消费者的死字段。**
- DoD 原写「不低于 95.9%」是从模板结转的过期数字套到了另一个包上（`cmd/atlas` 实测 75.6%），按字面本任务永远不可能达标。该问题由 dev 预读时发现、Leader 已裁决并落盘 `plan.md` 第 48 行。

### 7.2 其余

| 项 | 结果 |
|---|---|
| `go test ./... -count=1` | **FAIL 包数 0** |
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**零新增项**） |
| `go vet ./internal/hestia/... ./cmd/...` | **0 行输出** |
| 三条新测试 + `TestHestiaCmdDoesNotResolveDBPath` | **4/4 PASS** |
| AD-4：merge 早于 `dev_done` | merge `6ff4b45` @ `2026-09-01T03:08:39Z` < `dev_done` @ `03:11:24Z`（**早 2 分 45 秒**） |
| AD-4：commit message 锚定 | `feat(TASK-007): atlas hestia backfill load 子命令` ✔ |
| AD-4：自拆 worktree | `git worktree list` 中无 `wt-TASK-007-m1c3b` |

---

## 8. INFO（不构成缺陷）

**INFO-1｜`functional[3]` 的措辞与实际分工略有出入。**
DoD 说「漏填 `Out` 是静默的」，而 TASK-006 的 `BackfillLoad` 对 nil `Out` 直接报错 ⇒ 漏填**是响的**。这条断言真正守住的是「填了、但填成了不打印的东西」（变异 C），它被目标断言精确杀死。**该守的形态守住了，只是理由那句需要修正。**

**INFO-2｜残余缺口一条：无法区分 stdout 与 stderr。**
成因是 `calExec` 把两条流合并到同一 buffer（既有 harness 形状，早于本任务）。危害小于 DoD 要防的「一片空白」。若将来要堵，改法是给 `calExec` 加一个分离 out/err 的变体，那属于 harness 层的改动、不属于本任务。

**INFO-3｜dev 记录的两个方法观察都成立，且质量高。**
① `grep` 与守卫是**两个不同的性质**，且反事实危险（grep 若返回 0 会被当独立佐证，而它对真实情况恰好也给 0）；② 「拓扑 + 内容」两条 merge 判据**不是原子的**，两条打架时先原地重跑，别立刻当成 sha 被改写。第二条我验证时未遇到（一次即一致），但记录本身正确。

---

## 9. 结论

八条 done_criteria **逐条 PASS**，无失败用例、无越界、无新增依赖。

`functional[0]` 的「逐字照抄」不是采信自陈：我先确认两份需求文档 sha256 相同，再把整个命令块与交付比到 **diff 0 行**。`functional[2]` 的字面偏离我自己把 cobra 的真实错误串打了出来（**确实不含 `--db`**），并核到既有两条同类用例都断裸名，Leader 裁决已落盘。

**最核心的 `functional[3]` 我用三个变异回答了「杀死它的是不是目标断言」**：DoD 点名的静默形态（变异 C）被 `require.NotEmpty` 这条为它而写的断言精确杀死、外溢度 1；唯一存活的变异 B 经查是 harness 合并 out/err 造成的**等价变异**，不是断言弱。

覆盖率**两把尺各自两次取样**（门禁口径 75.9% / per-package 75.7%，均稳定），并核实 `coverage_floor` 确有消费者。

**判定：VERIFIED。**
