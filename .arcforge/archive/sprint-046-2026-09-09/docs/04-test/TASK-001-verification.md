# TASK-001 验证报告

> 本文件含 **两轮** 验证，按时间倒序：
> **第二轮（review_fix 返工复验，verifier `test-m3-a`）在前**，**第一轮（首次验证，verifier `test-m3-b`）原文完整保留在后**。
>
> ⚠️ **为什么不直接覆盖**：Leader 指示「覆盖写」，但首轮报告是 357 行独立实证（M-ORDER 变异、AST 守卫
> 34→38 的双来源核实、TDD 红阶段、真语料回归……）。写通道**只能创建/覆盖、不能删除**，write-guard 又禁止
> 直接 `rm` —— **覆盖即不可逆销毁**。且首轮报告正是本轮返工的**问题来源**（它的 ⑤.1 就是这次 `fix_items`），
> 销毁它会让「返工为什么发生」失去唯一的一手记录。故取「新内容置顶 + 首轮原文逐字保留」，
> 既满足单文件（不新建重名文件），又不丢证据。首轮原文备份指纹 `ffb082eb7e1e592cc86c2184ae59aacb97d7b6ae1d4cab05a15c43dfbc192b71`。

---

# 第二轮：review_fix 返工复验（verifier: test-m3-a）

- **验证者**：`test-m3-a`（`07:35:46Z` 由 leader 经逃生边 `verifying → verifying` 从 `test-m3-b` 改派）
- **被验 owner**：`dev-m3-a`　**判定日期**：2026-09-08
- **判定**：✅ **VERIFIED**
- `rework_count = 1`　`reason_class = dod_defect`（问题在 DoD 不在交付）　`assignment_epoch = 1`

## ⓪ 基线与元信息

| 项 | 值 |
| --- | --- |
| `verify_baseline.head` | `1c7af81846a4fba3fbb76a84e42eb232b8178f81` |
| `verify_baseline.discovery_sha256` | `90c30bd45028985d3ef27b35a629cebfbaced7e048a40094ccc4baa9aa90cb6a` |
| 返工 commit | `71a985d0e2617686fe4c29bca9458047f4b805ba`（分支 `task/TASK-001-fix`） |
| 返工基线（其父） | `5ba7c3ceca25f5c2495faf32e4ce1c38e92f90ca` |
| 首轮交付 commit | `ac1acc5e77b9ac34b65d683461cded3b4d0240ad` |

🔴 **逃生边不刷新基线** ⇒ 我接的是与前一位 verifier **同一个判定对象**。
**判定前后各比对一次，两次都零漂移**：`git rev-parse HEAD` 与 discovery 的 sha256 均与基线**逐位相同**
⇒ 转 `verified` **不需要** `--ack-drift` / `--ack-discovery-drift`。

> 前一位 verifier 本轮零产物，我**从头做**，未捡任何中间态。
> 全部实跑在我自建的隔离 worktree（`git worktree add --detach ../wt-verify-TASK-001-a 1c7af818…`，**钉全 sha**）里完成，
> **主工作区非 `.arcforge/` 改动全程 0 行**；收尾已 `git worktree remove`（谁建谁拆）。

## ① 本轮判据矩阵

本轮验的是 `fix_items` 那一条 + 不动面 / 门禁（八条 `done_criteria` 的首轮结论见下半部分，未受本轮改动影响）。

| # | 判据 | 我实跑的核实 | 判定 |
| --- | --- | --- | --- |
| **fix_items 主项** | 补「侧车**写**失败 ⇒ 契约不被写出」 | `TestIngestHistoryWriteFailureSkipsContract` 新增；**2×2 变异证明它闸住了缺口**（§③） | **PASS** |
| **fix_items 可选项** | 「`BuildHistory` 失败那条若难以自然触发，可只补一条并说明理由」 | dev **两条都补了**：`TestIngestHistoryBuildFailureSkipsContract`；M1 变异同样 KILLED | **PASS（超出要求）** |
| **覆盖率（语句数口径）** | 未覆盖块应降到 89 或 90 | 见 §② | **PASS** |
| **套件** | `go test ./internal/hestia/... -count=1` 全绿 | 两包（并跑 `./cmd/atlas/...`）EXIT=0 | **PASS** |
| **不动面** | `ingest.go`/`parse.go`/`validate.go`/`store.go`/`go.mod`/`go.sum` 各 0 行 | 逐个 `git show --numstat 71a985d0 -- <file>` 全 0 | **PASS** |
| **范围** | 只改 `internal/hestia/ingest_test.go` | `git diff --numstat 5ba7c3c..1c7af81 -- internal/ cmd/` → 恰 1 文件 `70/0`，在 `writes` 内，**零越界** | **PASS** |
| **门禁附项** | vet / gofmt / 守卫 / 目录污染 / merge | 见 §④ | **PASS** |

**结论：本轮判据全部 PASS。** 两条低严重度观察见 §⑤，**均不构成 rejected**。

## ② 覆盖率 —— 按语句数精确复算（不看四舍五入显示值）

两棵树各自跑一遍 `-covermode=count`，用同一个脚本数语句：

```
base 5ba7c3ceca25f5c2495faf32e4ce1c38e92f90ca : 2783/2880 = 96.6319%   未覆盖块 91
head 1c7af81846a4fba3fbb76a84e42eb232b8178f81 : 2785/2880 = 96.7014%   未覆盖块 89
增量                                          : 语句 +2   块 −2   +0.0694pp
```

**目标两行的执行次数 0 → 1**（这是 `fix_items` 点名的两个 `count==0` 分支）：

```
base: ingest.go:453.17,455.4  1 0        head: ingest.go:453.17,455.4  1 1   ← BuildHistory 失败
base: ingest.go:456.64,458.4  1 0        head: ingest.go:456.64,458.4  1 1   ← WriteHistory  失败
```

`fix_items` 期望「降到 89 或 90」，实际达成**较好的 89**。

**关于 DoD 警示的「同一棵树两把尺差 0.1pp」**：本次两把尺**同值**（`go test -cover` 与
`go tool cover -func | grep total:` 都报 `96.7%`），**不构成该现象的反例**——但我全程按**语句数**比对，
本就绕开了这个口径问题。

## ③ 🔴 决定性证据：2×2 区分性变异

`fix_items` 描述的危害原文是「**将来有人把这两个 `return fail` 改成『记日志继续』，消费者会拿到没有侧车的
契约，而整个套件仍全绿**」。我把这句话**直接做成变异**，并加上「有/无返工测试」这一维：

**变异体**（隔离副本，逐字 diff 核对 + `gofmt -e` 语法闸）：

```diff
  hist, err := BuildHistory(ctx, d.Store, obs, contractGenerator)
  if err != nil {
-     return fail("contract", contractError{err: err})            ← M1
+     fmt.Fprintf(d.Out, "warn: build history: %v\n", err)
  }
  if _, err := WriteHistory(d.Cfg.Queue.Dir, hist); err != nil {
-     return fail("contract", contractError{err: err})            ← M2
+     fmt.Fprintf(d.Out, "warn: write history: %v\n", err)
  }
```

| | head（含返工测试） | base `5ba7c3c`（无返工测试） |
|---|---|---|
| **M1** `BuildHistory` 失败 ⇒ 继续 | **KILLED**（恰 1 条 FAIL：`…BuildFailureSkipsContract`） | 🔴 **SURVIVED（整包全绿）** |
| **M2** `WriteHistory` 失败 ⇒ 继续 | **KILLED**（恰 1 条 FAIL：`…WriteFailureSkipsContract`） | 🔴 **SURVIVED（整包全绿）** |
| **对照**（未变异） | 绿 | 绿 |

四格结论：

1. **缺口是真的** —— base 那两格把 `fix_items` 的预言**逐字复现**了：改成「记日志继续」，整个套件仍全绿。
   这不是推理，是观察。
2. **返工确实闸住了它** —— 同一变异在 head 上必死。
3. **零外溢** —— 每个变异**恰死 1 条**，且正是对应的那条 ⇒ 断言精准，不是「什么都红」的钝器。
4. **对照两臂皆绿** ⇒ harness 有效（否则「全红」会把 KILLED 记成假的）。

## ④ Leader 点名要核的那条防回归断言 —— 裁决：**有效但冗余，不是 no-op**

被核对象：

```go
assert.Contains(t, err.Error(), "preceding 2025-12/monthly",
    "失败的是 monthly_recent 那次查询 —— 证明校验已通过、确实走到了 BuildHistory")
```

### 4.1 先排掉一个会误判的变异

直觉做法是把夹具的 `WHERE period_type = 'monthly'` 直接改成 `'annual'`。**这个变异测不到该断言**：
那一刻库里**根本没有 annual 行**（annual 观测是 `Ingest` 期间才存进去的），UPDATE 命中 0 行 ⇒ **零污染**
⇒ `Ingest` 返回 `nil` ⇒ 红在 `require.Error`（第 1549 行）而非该断言。

> 这正是 Leader 转述的那条经验的另一面：**设计变异时不仅要问「想淘汰的那一版会不会也恰好红」，
> 还要问「我的变异真的造成了我以为的那个状态吗」。** 我第一次跑的就是这个无效变异。

### 4.2 真实回归夹具下的两臂

改成**另存一条 `2024-12/annual` 前序行并污染 annual**（这才是「把夹具改回污染 annual 行」的可实现形态）：

| 臂 | 断言集 | 结果 |
|---|---|---|
| **A′** | 全部保留 | **RED** —— 三条 `Contains` **全部**失败（含被核的那条） |
| **B′** | 只删被核的那条 | **仍 RED** —— `Contains "contract"` 与 `Contains "history 2025-12-annual"` 照样失败 |

实际错误串（两臂相同）：

```
hestia ingest: 1/1 期失败 (2025-12): hestia ingest 2025-12/annual (…):
validate: hestia: validate 2025-12/annual: hestia store preceding 2025-12/annual:
sql: Scan error on column index 43, name "m2": converting driver.Value type string ("not-a-number") to a float64
```

### 4.3 裁决

- **它不是 no-op** ——A′ 证明它**确实会执行、确实会红**。与 test-m3-b 在 TASK-002 揪出的
  `if derr == nil { assert… }` **形状不同**：那个永不执行，这个执行且报错。
- **它也不是唯一的闸** ——B′ 证明另两条断言同样拦得住这个回归。
- **dev 的机制论断经实测完全成立**：真实错误串与它注释里写的
  `validate: … store preceding 2025-12/annual: Scan error` **逐字吻合**，校验阶段确实先失败、
  `BuildHistory` 那两行确实走不到。**污染 monthly 而非 annual 这个造法选择是对的**，
  代码注释里的成因解释准确。

⇒ **不构成缺陷，不判红。** 该断言的真实价值是**把诊断从「错了」压缩到「错在哪个阶段」**。

## ⑤ 本轮发现（2 条，均不构成 rejected）

### ⑤.1 🟢【措辞】那条断言 rationale 里「缺口悄悄回来」偏强

原文理由是「将来有人把夹具改回污染 annual 行，测试会**仍然报错**但错在别的阶段，**缺口悄悄回来**」。
前半句实测成立，**后半句偏强**——另两条断言会让测试**大声报红**（§4.2 的 B′），缺口藏不住。
建议后续顺手改成「另两条会红但指向模糊，这条把失败定位到 `monthly_recent` 那次查询」。**不要求返工。**

### ⑤.2 🟡【我自己的失误，已自纠，记在此供后来者避坑】

我一度报「discovery 里没有 `rework_round_1` / `anchor_note`」——**错在我查了顶层**，它们在
`.verification` **内部**。Leader 的描述是对的。

⇒ 教训：**判断嵌套 JSON 的字段缺失前必须先 `jq -r 'keys[]'` 看层级**。
`jq -r '.x // "<无>"'` 在**层级找错**时的输出与**字段真缺失**时**完全同形**，
而「缺失」会被读成对方的缺陷——这是个**只产假阳、且假阳指向被验方**的仪器故障。

## ⑥ 门禁附项与工作区卫生（逐条实跑）

```
go test ./internal/hestia/... ./cmd/atlas/... -count=1   → 两包 ok，EXIT=0
go vet ./internal/hestia/...                             → 输出 0 行，EXIT=0
gofmt -l internal/hestia cmd/atlas                       → 恰 2 项：cmd/atlas/backtest_test.go、
                                                            cmd/atlas/crisis_test.go（均为既有欠账）
TestPackageExposesNoWriteFunctions / TestStoreExposesNoWriteMethods → 双 PASS
find internal/hestia cmd/atlas -type d \( -name pending -o -name queue \)  → 0 个
```

> 目录污染用 `find` 而不是 `git status`——git 不跟踪空目录，`git status` 对它**恒空**（首轮报告已立此规矩）。

**merge 双判据**：拓扑 `git merge-base --is-ancestor 71a985d0… master` → IS-ANCESTOR；
**内容判据** `internal/hestia/ingest_test.go` 在 `71a985d0` 与 `master` 上 sha256 **逐字节同**
（`f6f3d512eaa4778d520f4f1de41e294990b5bdcb0c8da563afceef7467c16a38`）。
拓扑判据单独不可信（rebase/cherry-pick/squash/三方 merge 都会换 sha），故补内容判据。

**discovery 的锚分工核对通过**：`verification.anchor` 仍是首轮的
`7e24b116faff2174771d4551b54aa582a874d209` **一个字未改**；返工数字全在
`verification.rework_round_1`（锚 `1c7af81846a4fba3fbb76a84e42eb232b8178f81`）+ `anchor_note` 说明分工。
**「数字的锚必须是它们实际来源」这条做对了。** 我逐条复算了该段的每个数字，**无一不符**。

**门禁那两大段 WARN**：跨 sprint 同号 `TASK-001` 复用造成的历史噪声，门禁自己标注「不参与本次范围
漂移判定」——我**未**据其文件列表要求改声明。

## ⑦ 第二轮结论

**✅ VERIFIED。** `fix_items` 的主项与可选项**都做到了**（可选项本可省，dev 两条都补）；
覆盖率按语句数 **2783→2785 / 块 91→89 / 目标两行 0→1**；不动面全 0；范围零越界；门禁全绿；
判定前后基线**零漂移**。

**最有力的一条**：`fix_items` 描述的危害在 base 上被**逐字复现**（M1/M2 双双 SURVIVED、整包全绿），
在 head 上**双双 KILLED 且零外溢** —— 这个缺口是真的，也确实被闸住了。

---
---

# 第一轮：首次验证（verifier: test-m3-b）—— 原文逐字保留

> 以下为首轮验证报告全文，**未作任何改动**。它的 ⑤.1 正是本轮 `review_fix` 的来源。
> 其中的锚（`7e24b11` 等）与数字属于**首轮那棵树**，不要与第二轮的数字混用。

# TASK-001 验证报告（verifier: test-m3-b）

> 结论：**VERIFIED**。8 条 `done_criteria` 全部 PASS。
> 另报 **1 条真实缺口**（⑤.1：ingest.go 新增了 2 个无测试覆盖的错误分支）——它**不构成 rejected**，
> 理由与建议见该节；以及 3 条已裁决/已订正的已知偏差。
>
> 本报告每一条结论都来自我自己在**独立 worktree** 上的实跑与**变异测试**，不采信 discovery 写着「绿」。
> 凡引用交付方数字处，均标注我是否独立复算及复算结果。

## ⓪ 验证元信息

| 项 | 值 |
|---|---|
| 任务 | TASK-001（≡ 需求文档 TASK-001 的上半，`internal/hestia` 包内），`verifying` → `verified`，`assignment_epoch=1` |
| verifier | `test-m3-b` |
| `verify_baseline.head` | `7e24b116faff2174771d4551b54aa582a874d209` |
| **裁决落盘当刻 HEAD** | `7e24b116faff2174771d4551b54aa582a874d209` —— **与 baseline 逐位相同，全程零漂移**，未使用 `--ack-drift` |
| `verify_baseline.discovery_sha256` | `72acb953e39873a1f0bbd1efb7a84c1bc642ba4657eaad1a7e463a881e4103c2`（判定前后两次**实算**均一致，判定原料未被改写） |
| 交付 commit | `ac1acc5e77b9ac34b65d683461cded3b4d0240ad`（分支 `task/TASK-001-m3`，保留未删） |
| 基线锚 | `fe95ac707ea46f9939bb6554261830a95f1b93e1` |
| 验证方式 | scratchpad 内两棵 `--detach` 对照 worktree（`fe95ac7` / `7e24b11`），验毕拆除；主仓库 `internal/` `cmd/` `go.mod` `go.sum` `data/` 全程 `git status --porcelain` **0 条** |

---

## ① done_criteria 覆盖矩阵（8 条逐条）

| # | 完成标准（摘要） | 我跑的验证 | 判定 |
|---|---|---|---|
| **functional[0]** | `history.go` 形态：`ContractHistory` 五字段 / `HistoryEntry` / `historyMeta` 七 snake_case 字段 / `historyWindow=12` / `BuildHistory`·`toEntries`·`JSON`·`FileName`·`WriteHistory` | 逐行读全 133 行核对；`JSON()` 为 `MarshalIndent(h,"","  ")` + `append(b,'\n')`；`WriteHistory` 落 `<dir>/pending/<for>.history.json`，`MkdirAll` + `writeAtomic` | **PASS** |
| **functional[1]** | `history_test.go` 四条原文测试全 PASS + **「同名覆盖」追加段** | 10 条全 PASS（两把尺同值，②.4）；同名覆盖段实读逐条核对（②.5） | **PASS** |
| **functional[2]** | `ingest.go` **先侧车后契约** + 新测试 + 3 条 M2a 既有调整仍绿 | **变异 M-ORDER 直接证实顺序断言**（③.1），全包 813 条零红 | **PASS** |
| **functional[3]** | AST 守卫 `want` **38 项** + 字典序插入位置 | 按**内容锚**独立解析：base **34** → head **38**，+4、**零删除**、已排序；位置逐项吻合；变异 M-GUARD-A/B **双向**证实是精确相等断言（③.2） | **PASS** |
| **boundary[0]** | monthly 省略 / 非 monthly 为 `[]` 不是 null / 不足 12 期按实有 / `Data` 按 `fieldOrder` 无 null / `history.go` 无业务字段名字面量 | 四个变异逐条证实断言在守（③.3–③.5）；Go 探针自报 `len(fieldOrder)==76` | **PASS** |
| **error_handling[0]** | 五条错误分支各有测试走到 + TDD 红痕迹 | `history.go` 逐函数 **100%**、未覆盖块 **0**；五条测试逐条实读，手法不依赖权限位；RED 重放复现 `undefined: BuildHistory`（②.6） | **PASS** |
| **non_functional[0]** | 门禁 + 覆盖率 ≥96.6 + 真语料回归 + 目录污染 | **背对背两轮**覆盖率对照 + **精确语句数复算**（②.2）；gofmt/vet/依赖（②.3）；真语料回归四组数字与 `CONTRACTS.md` 基线**逐字相同**（②.7） | **PASS** |
| **non_functional[1]** | 交付流程 / commit subject / merge / 数字统一重采 | 交付 commit 恰 5 文件与 `writes` 一致、**零越界**；5 文件在交付 commit 与 master 上 sha256 逐一相同；不动面全 0 行（②.1） | **PASS** |

---

## ② 实跑证据

### 2.1 范围、合入与不动面

```
$ git show --numstat --format='' ac1acc5e77b9ac34b65d683461cded3b4d0240ad
133  0  internal/hestia/history.go
318  0  internal/hestia/history_test.go
 11  0  internal/hestia/ingest.go
 52  7  internal/hestia/ingest_test.go
  1  1  internal/hestia/store_test.go        ← 恰 5 个，与 writes 声明逐项一致
```

**零越界。** 合入用**内容判据**核实（不用拓扑判据——rebase/amend/squash 都会换 sha）：5 个文件在
`ac1acc5e…` 与 master `7e24b11…` 上的 sha256 **逐一相同**。commit subject 为
`feat(TASK-001): M3 history 侧车……`，匹配门禁的 `^[a-z]+\(TASK-001\):`。

**不动面**（`fe95ac7…` → `7e24b11…`）：`parse.go` / `validate.go` / `store.go` / `go.mod` / `go.sum`
**diff 均 0 行**。另核：基线到 master 之间**没有本任务之外的 `.go` 改动**（其余是 TASK-003/004 的
`.md` 与 `PENDING-MECHANISMS.md`）⇒ 覆盖率差异可**唯一归因**于本任务。

### 2.2 覆盖率 —— 背对背 × 精确语句数复算

「零余量」判据下不能只看四舍五入后的百分比，故做两层：

**第一层：背对背对照**（同一时刻并排跑，逐轮同等负载）

| 轮 | base `fe95ac7` | head `7e24b11` |
|---|---|---|
| 1 | 96.6% | 96.6% |
| 2 | 96.6% | 96.6% |

**第二层：从 coverprofile 精确复算语句数**（`96.6%` 是显示值，微跌也可能仍显示 96.6）

| 树 | 覆盖 / 总计 | 精确值 |
|---|---|---|
| base `fe95ac7` | 2743 / 2841 | **96.5505%** |
| head `7e24b11` | 2783 / 2880 | **96.6319%** |

⇒ **不是「持平」，是实际上升 +0.08pp**；未覆盖语句块 **92 → 91**。DoD 下限 `≥96.6` 满足（96.6319 ≥ 96.6）。

`history.go` 逐函数覆盖率：`BuildHistory` / `toEntries` / `JSON` / `FileName` / `WriteHistory` **各 100.0%**，
`history.go` 未覆盖块 **0**——与 dev 所报一致，我独立复算确认。

> 📌 顺带订正一个口径：DoD 把基线记作「实测 96.6%」，而**精确基线是 96.5505%**（显示值恰好也四舍五入成 96.6）。
> 这不影响判定（head 96.6319% 严格高于两者），但「零余量」这个说法建立在显示值上；真实余量是 +0.08pp。

### 2.3 门禁

```
$ GOTOOLCHAIN=local go test ./internal/hestia/... -count=1   → ok，退出码 0
$ go vet ./internal/hestia/...                               → 零输出，退出码 0
$ gofmt -l internal/hestia cmd/atlas
cmd/atlas/backtest_test.go
cmd/atlas/crisis_test.go                                     → 恰 2 项
```

gofmt 的判据是「这两个之外无新增项」。我不止照判据看，还**在基线树上背对背跑了同一条命令**，
输出**同为这 2 项** ⇒ 它们确系既有欠账、不是本任务引入，判据成立。

`go.mod` / `go.sum` 在基线到锚点间 diff **0 行** ⇒ 无新增依赖。

### 2.4 测试条数 —— 两把尺 ×（包级 / 文件级）两个层次

| 口径 | base | head | 净增 |
|---|---|---|---|
| 全包 动态顶层 `--- PASS` | 801 | **813** | **+12** |
| 全包 静态 `git diff … '*_test.go' \| grep -c '^+func Test'` | — | — | **+12**（`^-func Test` = **0**，无删除） |
| `history_test.go` 静态 `grep -c '^func Test'` | — | **10** | — |
| `history_test.go` 动态顶层 `--- PASS` | — | **10** | — |

两把尺在两个层次上都同值：净增 12 = `history_test.go` 10 + `ingest_test.go` 2。任意层 `--- FAIL` = **0**。

### 2.5 functional[1] 的「同名覆盖」追加段（DoD 初稿四条测试无一验它，Leader 补的）

`TestWriteHistoryLandsInPending` 尾部实读，断言逐条到位：

- `require.Equal(t, path, path2)` —— 同名覆盖路径不变
- `assert.Equal(t, want, got)` —— 第二份**完整替换**（读回比对整份 JSON，不是子串）
- `assert.NotContains(t, string(got), "contract@v1\"")` —— 旧内容不残留
- `require.Len(t, entries, 1)` + 文件名断言 —— **不留 `.tmp` 中间文件**

### 2.6 TDD 红阶段（error_handling[0]）

discovery 记录的 RED 输出是写实现之前跑的，那棵树已不存在。我做了**可复现的重放**：在锚点树上把
`internal/hestia/history.go` 移走（只留测试与已接线的 `ingest.go`）后跑同一条命令：

```
退出码 1
internal/hestia/ingest.go:452:16: undefined: BuildHistory
internal/hestia/ingest.go:456:16: undefined: WriteHistory
internal/hestia/history_test.go:61:12: undefined: BuildHistory
... （`undefined: BuildHistory` 共 6 行）
```

复现了同一类失败（`undefined: BuildHistory`）。行号与 discovery 所记不同是**预期的**——dev 的 RED 采于
`ingest.go` 尚未接线、测试尚未定稿的那一刻（故其输出**只有** `history_test.go` 行、没有 `ingest.go` 行），
我的重放采自最终树倒推。两者互不矛盾，且 dev 的版本内部自洽。

### 2.7 真语料回归（non_functional[0]，本任务改了 `ingest.go` 故必须证明回填路径未受影响）

```
$ GOTOOLCHAIN=local go run ./cmd/atlas hestia backfill load \
    --dir <主仓库绝对路径>/data/hestia-backfill-2026-08-14 --db <临时> --allow-incomplete
退出码 0

  语料总篇数: 218   待解析: 217   本迭代不解析: 1        →  218 = 217 + 1  ✔
  解析成功: 213     解析失败: 4                          →  217 = 213 + 4  ✔
  合并后观测: 97（单篇 28 + 合并组 69）
  入权威表: 76      落 pending: 21                       →   97 = 76 + 21  ✔
  四道恒等式: 全部成立 ✓
  字段冲突（预期 0，共 0）                               →  0  ✔
  口径路由核对（预期违反 0，共 0 违反）                   →  0  ✔
```

与 `internal/hestia/CONTRACTS.md` 的既有基线**逐字相同**。

**目录污染**（`git status` 对空目录恒空，必须用 `find`）：回归跑完后复查，
`find internal/hestia cmd/atlas -type d \( -name pending -o -name queue \)` = **0 个**（worktree 与主仓库各查一次），
仓库根 `queue/` 两处均不存在。

⚠️ 复现提示：`data/` 是 gitignored、只在主仓库工作区，linked worktree 里**没有** ⇒ `--dir` 必须写绝对路径。

---

## ③ 变异测试 —— 「PASS」不等于「断言在守卫」

全部在 scratchpad 内的独立 worktree 上进行，每体先过**语义闸**（打印 diff 逐字核对）与**有效性闸**
（`go build` 必须通过），跑完立即 `git checkout --` 还原并**核对 sha256**。主仓库 `internal/` `cmd/`
全程零改动（已核实）。

| 变异 | 改动 | 转红的测试 | 结论 |
|---|---|---|---|
| **M-ORDER** | 把 `BuildHistory`+`WriteHistory` 整块挪到 `WriteContract` **之后** | **只有 `TestIngestHistoryLandsBeforeContractOnWriteFailure`**（813 条里独家 1 红）；`TestIngestWritesHistoryBesideContract` **照样绿** | KILLED ✔ |
| **M-GUARD-A** | 从 `want` 删掉 `"WriteHistory"`（模拟漏登记） | `TestPackageExposesNoWriteFunctions` | KILLED ✔ |
| **M-GUARD-B** | 往 `want` 塞入不存在的 `"ZZBogusSymbol"`（模拟凑数） | `TestPackageExposesNoWriteFunctions` | KILLED ✔ |
| **M-OMIT** | `omitzero` → `omitempty` | `TestBuildHistoryNonMonthlyKeepsEmptyMonthlyRecent` **与需求原文自己的** `TestIngestWritesHistoryBesideContract` | KILLED ✔ |
| **M-WINDOW** | `historyWindow` 12 → 24 | `TestBuildHistoryMonthlyOmitsRecent`（独家 1 红） | KILLED ✔ |
| **M-FIELDORDER** | `toEntries` 改遍历 `o.Values`（丢掉 `fieldOrder` 键序） | `TestHistoryEntryShapeAndDeterminism`（独家 1 红） | KILLED ✔ |
| **M-LIT** | 往 `history.go` 塞 `var zzProbe = "tsf_stock"` | `TestHistorySourceHasNoFieldLiterals` | KILLED ✔ |

### 3.1 M-ORDER —— Leader 点名要核的那条，我取到的是观察不是推理

dev 声称：「原文那条 `TestIngestWritesHistoryBesideContract` 只看成功路径，把 `WriteHistory` 挪到
`WriteContract` 之后它照样绿，断言照不出顺序」。这句话可以**直接实测**，我做了：

```
对照组（未变异）：两条都 PASS
变异后（顺序交换，编译通过）：
  --- PASS: TestIngestWritesHistoryBesideContract        ← 原文那条，照样绿
  --- FAIL: TestIngestHistoryLandsBeforeContractOnWriteFailure
        侧车在契约之前写：契约写失败时它必须已经在
        stat …/pending/2025-12-annual.history.json: no such file or directory
全包：顶层 --- FAIL = 1 / --- PASS = 812   ⇒ 813 条里独家 1 红
```

⇒ **dev 的理由完全成立**，而且结论比它自陈的更强：**「先侧车后契约」这个性质，在整个 813 条套件里
只有 `TestIngestHistoryLandsBeforeContractOnWriteFailure` 一条守着，它是唯一闸。** 这条「需求原文没有的
新增测试」不是多余，删掉它顺序就再也不会变红。

### 3.2 M-GUARD —— AST 守卫是**双向**精确相等

漏登记（少一项）与凑数（多一项无中生有的符号）**都**让守卫转红 ⇒ DoD description 里那句
「这是条精确相等断言，照 37 去凑会删掉一个已登记符号」的担心是真的，而守卫确实挡得住。

### 3.3 M-OMIT —— `omitzero` 那条裁决被实测证实

Leader 与 dev 的理由是「`omitempty` 会让需求原文自己的接线断言 `assert.Contains(t, h, "monthly_recent")` 必红」。
把标签改回 `omitempty` 后，转红的**恰好就是**那条接线断言（`TestIngestWritesHistoryBesideContract`）
加上专门钉这条语义的 `TestBuildHistoryNonMonthlyKeepsEmptyMonthlyRecent`。
⇒ **这不是一句可信的推理，是一个可复现的观察**：需求原文的 `omitempty` 与它自己的断言确实互斥。

### 3.4 M-WINDOW / M-FIELDORDER

两条 boundary 语义各自被一条测试**独家**守着：改窗口大小只有 `TestBuildHistoryMonthlyOmitsRecent` 红，
丢掉 `fieldOrder` 键序只有 `TestHistoryEntryShapeAndDeterminism` 红（该测试用**流式 `Decoder` 取原始键序**
而不经 `map`——经 map 会被排成字典序，那就测不出 `fieldOrder` 序了，这个手法是对的）。

### 3.5 M-LIT 与 `fieldOrder` 项数

`TestHistorySourceHasNoFieldLiterals` 遍历**运行时解析后的** `fieldOrder` 常量值逐条断言，比 DoD 给的
前缀正则 `"(m0|m1|m2|afre|loan_|deposit_|bill_)` 强（后者靠人猜前缀）。塞入 `"tsf_stock"` 后它转红 ⇒ 真在守。
另用一次性 Go 探针让 Go 自己报数：`len(fieldOrder) == 76`，首 `"tsf_stock"` 末 `"fx_rate"`，
确认 dev 所称「全 76 项」属实。

---

## ④ AST 守卫 34 → 38 —— 用**两个独立来源**证实基线

DoD 初稿写 33（Leader 已订正为 34）。我没有只复核 Leader 的订正，而是取了两个互不依赖的来源：

1. **解析基线树 `fe95ac7` 的 `store_test.go`**：三把尺（正则 / 逗号分割 / JSON 解析器）同为 **34**，已排序，
   首 `BackfillFetch` 末 `WriteContract`。
2. **`internal/hestia/CONTRACTS.md`**（前一个 sprint 写下的独立记录）：
   「导出面（`TestPackageExposesNoWriteFunctions` 的 want）| **34 项**」。

两个来源**各自独立得出 34**（一个是代码解析，一个是往期文档），故不是「一个抄了另一个」。**33 确系笔误。**

head 侧：**38 项**，新增恰 `['BuildHistory','ContractHistory.FileName','ContractHistory.JSON','WriteHistory']`，
**删除项为空集**（这一条要紧：若照错误的 37 去凑，必然要删掉一个已登记符号）。位置：

| 符号 | index | 与 Leader 订正对照 |
|---|---|---|
| `BuildContract` | 2 | — |
| **`BuildHistory`** | **3** | 「`BuildContract` 之后」✔ |
| `Contract.JSON` | 6 | — |
| **`ContractHistory.FileName`** | **7** | 「`Contract.JSON` 之后」✔ |
| **`ContractHistory.JSON`** | **8** | — |
| `DefaultSignals` | 9 | 「`DefaultSignals` 之前」✔ |
| `HealthSummary` | 14 | ⇒ DoD 初稿的「`HealthSummary` 之后」会落在 15，**确系改名前的旧字典序**，订正正确 |
| `WriteContract` | 36 | — |
| **`WriteHistory`** | **37** | 「`WriteContract` 之后」✔ |

与 dev 报的 6→7→8→9 与 36→37 逐项吻合。

> ⚠️ **我自己在这里翻过一次车，记下来**：首次解析时我用 `re.search(r'want\s*:?=\s*\[\]string\{...')`，
> 抓到的是 `store_test.go` 里**更靠前的另一个 `want`**（reflect 守卫那个，14 项）。**三把尺同为 14——
> 三把全错**。三尺同值只防「同一对象上算错」，完全不防「量错了对象」。改为把锚钉在内容上
> （字面量以 `"BackfillFetch"` 开头）并断言该锚在文件中**唯一命中**之后才得到 34/38。
> 同类第二次：抓 `fieldOrder` 的引号内容得 0 项——因为它的元素是 Go **常量**不是字符串字面量，
> 「0」是我的仪器坏了而不是「没有字段」，改用 Go 探针才拿到 76。

---

## ⑤ 发现的问题

### ⑤.1 🟡 **真实缺口：`ingest.go` 新增了 2 个无任何测试覆盖的错误分支**

从 coverprofile 精确定位（`count == 0`）：

```
internal/hestia/ingest.go:453.17,455.4    ← BuildHistory 失败 ⇒ fail("contract", contractError{})
internal/hestia/ingest.go:456.64,458.4    ← WriteHistory  失败 ⇒ fail("contract", contractError{})
```

`ingest.go` 未覆盖块 base **5** → head **7**，新增的正是这两个。

**这意味着什么**：③.1 证明了「契约写失败时侧车已在」有唯一闸守着，但**反方向没有任何断言**——
「**侧车写失败时契约不被写出**」这半个契约，目前没有一条测试碰过。而这半边恰恰是
「先侧车」设计的另一面：若将来有人把这两个 `return fail(...)` 改成 `log + 继续`，
消费者就会拿到**没有侧车的契约**，而**整个套件仍然全绿**。

**为什么仍判 PASS 而不是 rejected**：

- DoD `error_handling[0]` 的义务是一个**枚举**：「`BuildHistory` 的两处 `Preceding` 失败、`WriteHistory` 的
  `JSON`/`MkdirAll`/`writeAtomic` 失败」——这五条**全部**有测试走到，`history.go` 逐函数 100%。
  这两个 ingest 调用点的错误分支**不在枚举内**。
- 数值门槛也满足：覆盖率不降反升（②.2）。
- 交付方对 DoD 的每一条都做到了；这是**枚举式 DoD 够不到的地方**，不是交付缺陷。

**建议（成本很低，给 Leader 决定是否开返工或留给下轮）**：补一条镜像测试即可同时闭合覆盖缺口与断言缺口。
dev 已经把所需手法造好了——`TestIngestHistoryLandsBeforeContractOnWriteFailure` 用「把某个文件名预建成目录」
让指定的那次写失败；镜像版只需把预建的目标从 `2025-12-annual.json` 换成 `2025-12-annual.history.json`，
断言 `Ingest` 报错**且 `2025-12-annual.json` 不存在**。这条会同时钉住「侧车失败 ⇒ 契约不写」。

### ⑤.2 🟢 DoD 初稿「AST 守卫 33 项」→ 实为 **34**（Leader 已订正）

见 ④ 节，两个独立来源证实。**按 34/38 验，数出 34 不是缺陷。**

### ⑤.3 🟢 四处偏离需求原文，均已裁决，逐条复核成立

| 偏离 | 我的复核 |
|---|---|
| 类型名 `ContractHistory` 而非 `History` | `validate.go:35` 确有 `type History interface`；本任务 `validate.go` diff **0 行**（不动面守住）；JSON 键与 `FileName()` 返回值未变 ⇒ 下游零影响 |
| `omitzero` 而非 `omitempty` | **M-OMIT 变异实测证实**（③.3）：改回 `omitempty` 后需求原文自己的接线断言确实转红 |
| helper 用 `passing()` 而非 `ValidationReport{Passed: true}` | `store.go:937` 确有「Passed 但零 checks」拒绝；套件全绿 |
| commit subject 用 Arcforge 编号 `feat(TASK-001):` 而非需求原文的 `feat(M3 TASK-001):` | 门禁 `task-completed.sh` 只认 `^[a-z]+\(TASK-001\):`，照抄需求原文会被判「改动不可见」。我实读交付 commit subject 为 `feat(TASK-001): M3 history 侧车……` ⇒ 匹配门禁。裁决出处是任务 JSON `description` 的编号映射表（不是 `questions[].answer`） |

> ⚠️ **本节标题原为「三处」，已订正为「四处」。** 错因值得记一笔：那个「三」我是从派验说明的散文里拿的
> （它列了类型名 / `omitzero` / `passing()` 三条），而不是去对产物求值。discovery 里这个性质是
> **结构化字段**，一条命令就能精确判定：
>
> ```bash
> jq '[.decisions[] | select(.deviates_from_requirement == true)] | length' .arcforge/discoveries/TASK-001.json   # 4
> jq '[.decisions[] | select(.deviates_from_requirement == false)] | length' .arcforge/discoveries/TASK-001.json  # 3
> ```
>
> 漏掉的第四条（commit subject 编号）**我实际上验过**（②.1 核对了 subject 与门禁正则），
> 失误在于没把它计入「偏离」计数——它的裁决载体与另三条不同（任务 JSON `description` 而非
> `questions[].answer`），而我的计数来自一份只记了后者的摘要。**计数要向产物求值，不向摘要求值**；
> 这与我在 ⑤.1 指出的、以及我在 TASK-003 报告里订正自己那次的，是同一类问题。

### ⑤.4 🟢 口径提示：DoD 记的基线 96.6% 是**显示值**，精确基线为 96.5505%

不影响判定（head 96.6319% 严格高于下限与基线），但「零余量」这个说法建立在四舍五入后的数上；
真实余量是 +0.08pp。后续若再以「96.6 持平」作判据，建议直接比 coverprofile 的语句数，
四舍五入会同时吞掉小幅上升与小幅下跌。

---

## ⑥ 越界申报与工作区卫生

- **零越界**：交付 commit 恰 5 个文件，与 `writes` 声明**逐项一致**，无需补声明。
- **跨 sprint 同号 `TASK-001` 的两条门禁 WARN** 属历史噪声（历史上有 `feat(TASK-001): PaperBroker` 等），
  门禁自己标注「不参与本次范围漂移判定」——我**未**据其文件列表要求改声明。
- 我建的 2 个 worktree（`wt-t1-base` / `wt-t1-head`）已 `worktree remove --force` + `prune`
  （**下述均为执行并核实之后落笔**）：`git worktree list` 中我的两个已消失，scratchpad 无 `wt-*` 残留；
  临时回归 db 已删。主仓库 `git status --porcelain -- internal/ cmd/ go.mod go.sum data/` = **0 条**。
  （列表中残余的 `.worktrees/*` 与 `wt-TASK-005-m3` 分别是本仓库既有的与 dev-m3-c 在途的，不属于我。）

---

## ⑦ 结论

**VERIFIED。**

8 条 `done_criteria` 逐条 PASS。关键性质除测试通过外，另有 **7 个定向变异**逐条证实断言真在守卫；
覆盖率不是照读而是**背对背 + 精确语句数复算**，结果是**上升**而非仅持平；真语料回归四组数字与
往期基线逐字相同；范围零越界、不动面全 0。

**给 Leader 的三句话**：

1. **⑤.1 是本次唯一的实质发现**，且它**不是 dev 的失误**——DoD 用枚举方式列错误分支，枚举到哪里护到哪里，
   两个新调用点落在枚举之外。若下轮要闭合，成本是一条镜像测试。
2. 你要我核的两条 dev 自查（顺序断言、`omitzero`）**都成立**，而且我拿到的是变异实测而不是推理；
   顺序那条的结论比 dev 自陈更强：它是全套 813 条里的**唯一闸**。
3. **DoD 初稿的 33 确系笔误，真值 34**，我用了两个独立来源（解析基线代码 + 往期 `CONTRACTS.md`）而非只复核你的订正。
   另外 ⑤.4 那个「96.6 是显示值」的口径问题值得记一笔——它会让小幅下跌和小幅上升长得一模一样。
