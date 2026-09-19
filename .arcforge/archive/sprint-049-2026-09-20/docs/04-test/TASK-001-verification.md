# TASK-001 验证报告 — Spec 装配与 v_macro_current 视图 DDL

- **验证者**：test-m4c-a
- **裁决**：**VERIFIED**（8/8 done_criteria 通过）
- **验证树**：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` @ `39d6565fb82c476aa4d6a5c8e05d38d9f26de03f`（master）
- **verify_baseline 核对**：`head` 记录值 `39d6565fb82c…` == 当前 `git rev-parse HEAD`；`discovery_sha256` 记录值 `346211e73179…` == 当前 `shasum -a 256 .arcforge/discoveries/TASK-001.json`。**判定对象与基线零漂移**，无需 `--ack-drift`。
- 本报告全部数字采于上述单一 commit，无跨时间点拼接。

---

## 1. 亲跑结果（不复用 dev 的输出）

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `GOTOOLCHAIN=local go build ./...` | 0 | 通过 |
| `GOTOOLCHAIN=local go test -count=1 ./internal/crisis ./cmd/atlas` | 0 | `ok internal/crisis 0.701s` / `ok cmd/atlas 1.812s`，**零 FAIL 行** |
| `go test -count=1 -v ./internal/crisis` | 0 | RUN 135 / PASS 135 / FAIL 0 / SKIP 0 |
| `go vet ./internal/crisis` | 0 | 通过 |
| `gofmt -l` 本任务三个文件 | 0 | 无输出 |
| `go test -cover -coverpkg=./internal/crisis ./internal/crisis` | 0 | **94.5% of statements** |

判定按**退出码 + `--- FAIL:` 计数**，不依赖管道后输出。

**分层计数用两把独立的尺互验**（防「数斜杠」类仪器失效）：
- 尺 1（名字不含 `/`）：顶层 102
- 尺 2（祖先存在性——任一 `/` 切分前缀在 RUN 名字集合中即为子测试）：顶层 102
- 两尺一致；守恒 102 + 33 = 135 = RUN 总数 = PASS 总数。

⇒ dev discovery 自报的 `顶层 102 / 子测试 33 / 135` **属实**（独立复算得同值）。

## 2. 越界申报核对

`git diff --stat 2917c67..39d6565`（基线→交付）：

```
 internal/crisis/schema.go      |  73 +++++++++++++++++
 internal/crisis/schema_test.go | 157 +++++++++++++++++++++++++
 internal/crisis/store.go       |  33 ++-------
 3 files changed, 236 insertions(+), 27 deletions(-)
```

三个文件全部在任务声明的 `writes` / `packages` 内（`./internal/crisis/{schema.go,schema_test.go,store.go}`），**无声明外文件**。既有 crisis 测试（`store_test.go` / `ingest_test.go` 等）**零改动**——它们一行都没出现在 diff 里，`non_functional[0]` 的「既有测试零改动且全绿」因此是**结构性成立**而非「跑绿了就算」。

## 3. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 | 变异证据 | 判定 |
| --- | --- | --- | --- | --- | --- |
| functional[0] | `PRIMARY KEY (ts, indicator, fetched_at)` **且** `fetched_at TEXT NOT NULL`，从 `sqlite_master.sql` 读、两条分别断言 | test | `TestSchemaCreatesBitemporalShape`（两条 `assert.Contains` 分列） | M1、M2 各自单独杀死 | **PASS** |
| functional[1] | `v_macro_current` 存在且 DDL 含 `MAX(fetched_at)`（AD-2） | test | `TestSchemaCreatesCurrentView` | M3、M7 杀死 | **PASS** |
| functional[2] | 视图取最新修订 ⇒ `[11, 20]` | test | `TestCurrentViewPicksLatestRevision` | M1、M3、**M6**、M7 杀死 | **PASS** |
| functional[3] | `bitemporal.NewSpec(` 包内非测试源码恰 1 次且在 schema.go（AD-3） | test | `TestObsSpecIsSingleInstance`（按文件名分组计数） | M5 杀死 | **PASS** |
| boundary[0] | 乱序写入（先大后小 fetched_at）仍取较大那行 | test | `TestCurrentViewIgnoresInsertOrder` | M1、M3、**M6**、M7 杀死 | **PASS** |
| boundary[1] | `schemaDDL()` 幂等，表/索引/视图各恰 1 份 | test | `TestSchemaDDLIsIdempotent`（5 个对象逐个 `COUNT(*)`） | M4 杀死 | **PASS** |
| error_handling[0] | `mustSpec()` 只在 NewSpec 出错时 panic，文案含包名与原始错误，不吞成零值 Spec | **review** | 无测试用例（N/A，见 §4） | 隔离树实测 panic 原文 | **PASS** |
| non_functional[0] | 既有测试零改动全绿 + `go build ./...` + 两包测试 | test | 见 §1、§2 | — | **PASS** |

DoD 的数值/文本要求逐字比对无出入：`functional[2]` 的三条插入（2026-01-02/vix/10/2026-07-14、2026-01-02/vix/11/2026-08-14、2026-01-03/vix/20）与期望 `[11, 20]`、`boundary[1]` 的 `sqlite_master` 计数口径，测试与 DoD 一一对应。

## 4. 变异测试（隔离 worktree，主工作区零触碰）

`git worktree add --detach ../wt-verify-TASK-001 39d6565fb82c476aa4d6a5c8e05d38d9f26de03f`，7 个变异全部作用于该副本；harness：`scratchpad/test-m4c-a-T001-mutate.py`，原始结果：`scratchpad/test-m4c-a-T001-mutation.json`。

- **对照组**：rc=0，PASS 135，FAIL 0。**收尾对照**：rc=0，PASS 135，FAIL 0（与基线同值 ⇒ 还原完整）。
- **有效性闸**：每个变异体落盘后先 `go vet`（7/7 rc=0，无语法/编译型崩溃变异）、再 `git diff --stat` / `git status --porcelain` 确认变异**确实落盘**（7/7 非空）。
- **主工作区指纹**：三个交付文件的 sha256 在收尾时与开工时**逐字节一致**，`git status --porcelain -- internal/` 为空。

| 变异 | 内容 | 实际变红 | 结论 |
| --- | --- | --- | --- |
| M1 | 三段主键 → 两段主键 | Shape、PicksLatest、IgnoresOrder | KILLED |
| M2 | `fetched_at` 去掉 `NOT NULL` | Shape | KILLED（**单独**杀死，证明 f0 的两条断言各自独立生效，不是一条顶两条） |
| M3 | 视图改为裸 `SELECT *`（不经 `CurrentQuery`） | CreatesCurrentView、PicksLatest、IgnoresOrder | KILLED |
| M4 | 视图去掉 `IF NOT EXISTS` | IsIdempotent | KILLED |
| M5 | 包内另一非测试文件再造一个 `NewSpec` | IsSingleInstance | KILLED |
| **M6** | 视图取 `MIN` 而非 `MAX`，**但 DDL 文本里仍留 `/* MAX(fetched_at) */`** | PicksLatest、IgnoresOrder（CreatesCurrentView **保持绿**） | KILLED — **关键隔离**：证明 f2/b0 断的是**行为**而非文本，f1 断的是文本，两者不重叠、不互相顶替 |
| M7 | `NewStore` 改 `db.Exec(tablesDDL)`（store.go 没接上 `schemaDDL()`） | CreatesCurrentView、PicksLatest、IgnoresOrder | KILLED（store.go 的接线有三条断言守着） |

**M7 的一处非缺陷观察**：`TestSchemaDDLIsIdempotent` 在 M7 下**存活**。成因是该测试自己会 `s.db.Exec(schemaDDL())`，视图因此由测试体建出、计数仍为 1——这是我的预期写错了，不是断言空洞：该测试断的是 `schemaDDL()` 的幂等性（由 M4 证明有效），store.go 的接线由另外三条断言覆盖（M7 已证）。不构成覆盖缺口。

## 5. error_handling[0]（verify_by: review）的判定

被审对象 `internal/crisis/schema.go:22-29`：

```go
func mustSpec() bitemporal.Spec {
	s, err := bitemporal.NewSpec("macro_observations", []string{"ts", "indicator"}, "fetched_at")
	if err != nil {
		panic(fmt.Sprintf("crisis: obsSpec: %v", err))
	}
	return s
}
```

逐条对照：

1. **只在 NewSpec 出错时 panic**：`panic` 在且仅在 `err != nil` 分支内，成功路径直接 `return s`。✓
2. **文案含包名与原始错误**：不靠读代码推断——在隔离树把表名改成非法的 `"macro-observations"` 后实测得到：
   `panic: crisis: obsSpec: bitemporal: invalid table name "macro-observations"`
   包名 `crisis:` 在，原始错误经 `%v` 原样透传（且自带上游包名 `bitemporal:`）。✓
3. **不吞成零值 Spec**：错误路径不 `return Spec{}`，也不返回 err 让调用方忽略；对照 `bitemporal.CurrentQuery` 的头注释——零值 Spec 会生成表名为空的 SQL，报一个离成因很远的语法错误。此处在包初始化即炸开，成因与现象同处一行。✓

⇒ **PASS**。

## 6. dev 自报的两处判断 — 独立复核

### ① 删掉 `require.NotPanics(t, func() { _ = obsSpec })` — **成立，删得对**

不按 dev 的说理采信，直接实测：在隔离树里把那条断言**加回来**、同时让 `mustSpec` 真的出错，得到

```
panic: crisis: obsSpec: bitemporal: invalid table name "macro-observations"
	github.com/newthinker/atlas/internal/crisis.mustSpec()
		.../internal/crisis/schema.go:26
	github.com/newthinker/atlas/internal/crisis.init()
		.../internal/crisis/schema.go:16
FAIL	github.com/newthinker/atlas/internal/crisis	0.686s
```

输出里**没有任何 `=== RUN TestObsSpecNotPanics` 行**——进程在 `init()`（schema.go:16 的包级 var）就崩了，测试体一行都没执行到。

⇒ 该断言在「能跑到它」的前提下**恒真**，留着只会让读者以为 panic 路径有测试覆盖（而它实际由 `error_handling[0]` 的 review 覆盖）。删除是**提高**信噪比，不是降低覆盖。DoD 的 `functional[3]` 实质（源码扫描）原样保留且经 M5 证明有效。**不越界**：测试是本任务新建文件的一部分。

### ② 更正 `UpsertObservations` 的注释 — **成立，属于清理自己的 orphan，不越界**

- 旧注释：「the `(ts, indicator)` primary key makes rewrites overwrite, so backfill and repeated daily wakeups are idempotent by construction」。
- 本任务把主键扩成三段后，这句话**变成假话**——而使它变假的正是 dev 本次改动，属于 CLAUDE.md §3「Remove … that YOUR changes made unused / 清理自己的烂摊子」，不是「改邻近代码」。
- 新注释与实现逐条核对：`store.go:55-57` 是 `INSERT OR REPLACE INTO macro_observations (ts, indicator, value, source, fetched_at)`，在三段主键下**同一 `fetched_at` 的重写覆盖同一行、新的 `fetched_at` 落成新行**，与新注释所述一致；「v_macro_current is what picks the latest one」与 M6 实测的视图行为一致。
- 范围：`store.go` 在 `writes` 声明内。**不构成越界申报。**

⇒ 若不改，留下的是一句被自己的改动证伪、且与新语义相反的注释——这类假注释比没注释更贵。判断成立。

## 7. 残留风险（不阻断本任务，供 Leader 派下游时参考）

以下三条**均不在本任务 DoD 内**，不影响裁决，但都已核实为真：

1. **读路径尚未走视图**（dev 已在 discovery 主动上报，我复核属实）：`store.go:70` 的 `obsSelect` 与 `store.go:123` 的 `EvalDates` 仍直查 `macro_observations`。三段主键后同一 `(ts, indicator)` 可有多行，`Observation` 的 `QueryRow` 会取到**不确定的一行**，窗口类查询会混入同日多个修订。现有测试没红只是因为它们的 `fetched_at` 全相同（单一修订）——这是覆盖盲区，不是正确性证据。**建议 Leader 确认它已进入 TASK-004~007 中某个任务的 `done_criteria`（而不只是写在 discovery 里）**：按载体强度，只写在 discovery 的义务不会被验证者当作验收项。
2. **`functional[3]` 的源码扫描匹配字面量 `bitemporal.NewSpec(`**：若下游用 import 别名（`bt "…/bitemporal"` 后 `bt.NewSpec(`）再造 Spec，该断言**不会**变红。DoD 明文要求「用源码扫描断言」，故这是**符合 DoD** 的实现，不是缺陷；但下游 TASK-002~007 须知：别名导入可绕过这道闸。
3. **`CREATE TABLE IF NOT EXISTS` 只对新建库生效**：老库的三段主键与 `NOT NULL` 要靠 TASK-002 迁移 / TASK-003 守卫。schema.go:38-40 与提交信息都已显著标注，本任务范围内无遗漏。

另注（非本任务）：`gofmt -l ./internal/crisis` 另列出 4 个**既存**未格式化的测试文件（`eval_test.go` / `replay_html_test.go` / `replay_report_test.go` / `rules_test.go`），与本任务无关，dev 未动——处理正确。

## 8. 结论

8 条 done_criteria **全部有对应证据**：7 条 `verify_by: test` 逐条有测试且**逐条被至少一个变异杀死**（无空洞断言、无「文本断言顶替行为断言」），1 条 `verify_by: review` 的三个子要求逐条经代码 + 实测 panic 原文确认。亲跑 build/test 全绿、无越界写入、既有测试零改动、判定对象与 `verify_baseline` 零漂移。dev 自报的两处判断经独立实测均成立。

**裁决：VERIFIED。**

---

### 复现命令（锚一律钉全 sha）

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
git rev-parse HEAD   # 须为 39d6565fb82c476aa4d6a5c8e05d38d9f26de03f
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis ./cmd/atlas
GOTOOLCHAIN=local go test -count=1 -cover -coverpkg=./internal/crisis ./internal/crisis

# 变异（隔离副本，主工作区一个字节不碰）
git worktree add --detach ../wt-verify-TASK-001 39d6565fb82c476aa4d6a5c8e05d38d9f26de03f
python3 <scratchpad>/test-m4c-a-T001-mutate.py
git worktree remove --force ../wt-verify-TASK-001
```

隔离 worktree 已于报告落笔前 `git worktree remove --force` + `git worktree prune` 拆除（`git worktree list` 复核：仅剩 4 个**既存的、与本 sprint 无关的** `.worktrees/*`，无 `wt-verify-*` 残留）。
