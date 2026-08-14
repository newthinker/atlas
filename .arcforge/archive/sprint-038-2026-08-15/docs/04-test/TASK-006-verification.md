# TASK-006 验证报告 · 限速抓取、断点续抓与两类错误的不同处置

- **验证者**：test-agent-28（Reality Checker）
- **判定**：**VERIFIED**
- **判定锚**：`ffb1d60e970295b536d10052dde3587ab591ecd7`（= `verify_baseline.head`，**零漂移**：HEAD 逐字一致、`discovery_sha256` 逐字一致 `7cb895a5…`、声明范围内 diff 为空）
- **消融**：**20 个有效变异，KILLED 15 / SURVIVED 5**
- 🔴 **本报告与 Leader 的 reject 请求结论相反。** Leader 让我判 `rejected / dod_defect`，理由是「DoD 修订在派验后才到、第二条测试未落地」。**我逐条核实后认为那四条前提没有一条成立**，见 §2。**实质关切我同意**，但它的正确工具是 `verified → review_fix`（Leader 自己的边，本 sprint 在 TASK-004 上用过），不是 `rejected`。

---

## 1. 完成标准覆盖矩阵（9 条）

| # | 完成标准 | 对应测试 | 我自己的消融 | 判定 |
|---|---|---|---|---|
| functional[0] | 断点续抓：已抓的**不重发请求** | `SkipsAlreadyFetchedArticles` | A1 去掉断点续抓 → **KILLED** | **PASS** |
| functional[1] | index 页与搜索页**也存盘** | `SavesIndexAndSearchSnapshots` + `BackfillSearchSlugsCoverAllKeywords` | A2 不存 index / A3′ 不存搜索 / A4 index 改用页码命名 → **3 KILLED** | **PASS** |
| functional[2] | `Manifest.SearchSkippedReason` + fail-open 落盘 + 正常时 `omitempty` | `RecordsSearchSkippedReason` + `OmitsSearchSkippedReasonWhenSearchWorks` | A5 不写进 manifest / A6 去掉 omitempty → **2 KILLED** | **PASS** |
| error_handling[0] | 单篇失败 ⇒ `failed[]` + **继续** + 跑完返回非零 | `RecordsFailedArticleAndContinues` | A7 改成直接中止 / A8 有失败也返回 nil → **2 KILLED** | **PASS**（缺口见 §4.1） |
| error_handling[1] | 落盘失败 ⇒ **立刻中止，不继续抓** | `AbortsOnDiskFailure` | A9 改成记 failed 继续 → **KILLED** | **PASS**（缺口见 §4.1） |
| non_functional[0] | **sleep 次数 == 请求数 − 1**，每次 `backfillInterval` | `SleepsBetweenEveryRequest` | A10 完全不 sleep / A11 每次都 sleep / A12 时长改 2 秒 → **3 KILLED** | **PASS** |
| non_functional[1] | gofmt/vet/test 干净 + **不降低既有覆盖率** | 见 §3 | — | **PASS**（口径歧义见 §4.2） |
| boundary[0] | manifest 内容为字面量 `null` ⇒ 报错 | `RejectsNullManifest` + `LoadManifestRejectsJSONNull`（3 子测试） | A13 去掉 null 守卫 → **KILLED**（5 格同时红） | **PASS** |
| boundary[1] | 交集取 index 侧 `URL` 与 `Published` | `CrossCheckBackfillIntersectionTakesIndexSideFields` | A14 取搜索侧 URL / A15 取搜索侧 Published → **2 KILLED** | **PASS** |

**`boundary[1]` 是我在 TASK-005 找到的那两个 SURVIVED，确认闭合**：该用例让两侧 URL 的 **scheme 与末尾斜杠都不同**、`Published` 也不同，逐字段断言取 index 侧；A14/A15 两个方向各自 KILLED，红都落在这条用例上。

---

## 2. 关于 Leader 要我判 `rejected` 的四条前提 —— 逐条核实

Leader 的请求基于四条前提。**我逐条查了盘上的事实，没有一条成立。**

### 2.1 ❌「DoD 修订在我派验之后才到」—— 审计日志显示：**修订从未落盘，且 DoD 在 dev 开工前就定稿了**

`.arcforge/tasks/transitions.jsonl` 里 TASK-006 的写入序列：

```
11:21:58  update by=leader keys=["done_criteria"]    ← 最后一次 DoD 修改
11:21:59  transition by=leader pending->assigned      ← 1 秒后派发
11:24:04  transition by=dev-agent-56 assigned->in_progress
11:43:57  transition by=dev-agent-56 in_progress->dev_done
11:45:25  transition by=leader dev_done->verifying
```

⇒ **派验之后没有任何 `done_criteria` 写入**。那条「修订」只以消息形式存在，**从未成为 DoD**。
按 CLAUDE.md 的第一原则（文件系统是唯一真相源）与第二原则（DoD 是一切测试的唯一依据），
**我只能按盘上的 DoD 判**，而盘上的 DoD 里没有那条要求。

### 2.2 ❌「第二条测试（`base` 使两侧不分叉）尚未落地」—— **盘上的 DoD 根本没要求它**

`boundary[1]` 的**断言**子句原文只有一句：

> **断言**：喂一组两侧 `URL` 与 `Published` **都不同**的交集数据，确认结果取的是 index 侧那份。

关于 `base` 的那段是**理由**（解释为什么把测试放在 TASK-006 而不是回炉 TASK-005），不是第二条要求。
**一条 DoD 的义务是它的「断言」子句，不是它的理由段。**

### 2.3 ❌「M10 只闭合了一半」—— 盘上 DoD 要求的那一条**完整闭合了**

`TestCrossCheckBackfillIntersectionTakesIndexSideFields` 在 `backfill_fetch_test.go`。
Leader 转述「按裁决它应在 `backfill_crosscheck_test.go`」——但：
- 盘上 DoD **没有指定文件**；
- `backfill_crosscheck_test.go` **不在 TASK-006 的 `writes` 里** ⇒ dev **不可能**合法把它写在那儿。

⇒ 放在 `backfill_fetch_test.go` 是**唯一合法且满足 DoD 的位置**。这一步 dev 做对了。

### 2.4 ❌「`rejected → assigned` 是唯一能让新代码经过门禁的路」

`write-matrix.json` 直读：

```
verified->review_fix    → 合法写者: ["leader"]
review_fix->in_progress → 合法写者: ["dev-*"]
in_progress->dev_done   → dev，且门禁在此前置执行
```

⇒ **`verified → review_fix → in_progress → dev_done` 同样恢复写权、同样重跑门禁、同样 `rework_count+1`。**
本 sprint 的 TASK-004 走的正是这条路。

**⇒ 判 `rejected` 会把一份「完全符合盘上 DoD」的交付记成「验证不通过」，而 `reason_class=dod_defect`
的定义是「done_criteria 自相矛盾、不可测试或无法实现」——盘上这份 DoD 三者皆非。**
**Leader 的实质关切（`base` 一致性无人守）我完全同意**，但它是一条**新要求**，正确工具是
`verified → review_fix` 或写进 TASK-007 的 DoD。

---

## 3. 我自己跑出来的证据

### 3.1 套件
```
go vet ./internal/hestia/           → 空（exit 0）
gofmt -l internal/hestia/           → 空
go test ./internal/hestia/ -count=1 → ok
```
全包 **顶层 PASS 417 / 子测试 PASS 401 / 真 FAIL 0**。

### 3.2 覆盖率（提交前后两棵树**背对背**，各采两轮）
移出新文件的做法在本任务**不可用**（TASK-006 还改了 `backfill_manifest.go` 与 `store_test.go`，移不干净、编译失败），
改用 `993e956^`（= `a9d7572`）与 `993e956` 两个临时 worktree 并排跑：

| 树 | 第 1 轮 | 第 2 轮 |
|---|---|---|
| before `a9d7572` | 94.2% | 94.2% |
| after `993e956` | **93.6%** | **93.6%** |

⇒ 包级覆盖率**降了 0.6pp**，可复现。口径讨论见 §4.2。

**逐函数比对（各自树内渲染）**：135 个既有函数**百分比零变化**、无既有函数消失 ⇒
dev 的 `coverage_stronger_claim`（「既有代码覆盖率逐函数一致」）**属实，我独立复核确认**。
新增函数覆盖率：`BackfillFetch` 100% / `Get` 100% / `runBackfill` 86.0% / `fetchBackfillSearchAll` 85.0% /
`saveBackfillIndexPages` 80.0% / `writeBackfillFile` 80.0% / `backfillSearchSlug` 66.7%。

### 3.3 消融（20 个有效变异）
纪律：独占 detached worktree；有效性闸用 **`go vet`**（比 `go build` 更严，本轮拦下 2 个无效变异）；
`-v` 抓 `--- FAIL` 行确认**是哪一格红**；跑完还原并校验**三个**被改文件的 sha256（全部一致）。

**KILLED 15 / SURVIVED 5。** 9 条 DoD 的字面要求**全部有承重消融**；5 个 SURVIVED 见 §4.1。

### 3.4 越界申报
`git show --numstat 993e956`：
```
240 0  internal/hestia/backfill_fetch.go
570 0  internal/hestia/backfill_fetch_test.go
17  0  internal/hestia/backfill_manifest.go
3   0  internal/hestia/backfill_manifest_test.go
1   1  internal/hestia/store_test.go
```
恰好 **5 个声明的 `writes`**，**无越界**。对 `backfill_manifest.go` 的 17 行全部是
`SearchSkippedReason` 字段与 `null` 守卫，**未动 TASK-003 的既有行为**（既有函数覆盖率零变化亦佐证）。

---

## 4. 发现

### 4.1 🟡 `error_handling[1]`「落盘失败 ⇒ 立刻中止」有 **6 条写盘路径，只有 1 条被钉住**

实现在**全部 6 条路径上都正确中止**。但把任意一条改成「忽略错误继续」，只有 1 条会被测出来：

| 落盘路径 | 实现是否中止 | 消融 | 有测试钉住 |
|---|---|---|---|
| 文章正文 `articles/…` | ✅ | A9 | ✅ **KILLED**（`AbortsOnDiskFailure`） |
| index 快照 `saveBackfillIndexPages` | ✅ | A17 | ❌ **SURVIVED** |
| 搜索快照 `search/…` | ✅ | A18 | ❌ **SURVIVED** |
| `store.Save()`（manifest 本体） | ✅ | A19 | ❌ **SURVIVED** |
| `store.AppendFailed()` | ✅ | A20 | ❌ **SURVIVED** |
| 搜索路径的 `errBackfillDisk` 例外 | ✅ | A16 | ❌ **SURVIVED** |

**A16 那条后果最重，且不只是「少一道检测」**：去掉它之后，**本机磁盘故障会被 fail-open 吞掉**，
并在 manifest 的 `search_skipped_reason` 里写下 `search side failed, cross-check skipped: …`
——**把本机故障记成检索服务故障**。而这个字段存在的全部意义，就是让 M1c-2 分得清
「没做校验」与「校验通过」；写进一个**错误的成因**，比字段缺失更糟。
这与 TASK-005 的 `SkipLeavesDiffsEmpty` 防的是同一种「谎报」。

**为什么仍判 PASS**：DoD `error_handling[1]` 的字面要求是「**两条各写一个独立用例**」，dev 写了
（`AbortsOnDiskFailure`），且**实现在 6 条路径上全对**。缺的是**对正确行为的测试背书**，不是行为缺陷。

**建议**：把「6 条写盘路径逐条钉住」登记为 TASK-007 的一条 DoD（或走 `review_fix`）。
最低成本是把 `AbortsOnDiskFailure` 改成表驱动，用一个「第 N 次写盘失败」的 fake 文件系统跑 6 格。

### 4.2 🟠 `non_functional[1]` 的「不降低既有覆盖率」有**两种读法，给出相反结论** —— 请 Leader 定口径

| 读法 | 结果 |
|---|---|
| **A：包级覆盖率百分比不得下降** | ❌ **不满足**：94.2% → 93.6%（−0.6pp，可复现） |
| **B：既有代码的覆盖率不得下降** | ✅ **严格满足**：135 个既有函数百分比**零变化**（我独立复核） |

**dev 主动披露了这 0.6pp**，并写明「全部来自新文件未覆盖的错误分支（磁盘与网络故障的返回路径），
**未为凑数注入 hook 变量**」。我核对了：那些未覆盖行确实全是 `return err` 的故障分支。

### 🔴 补记：歧义已由**先例**消解，不由我的偏好消解

写完上面那段后我去查了这条 DoD（措辞在 TASK-001/003/004/005 里**逐字相同**）此前是怎么执行的：

| 任务 | 包级覆盖率 | 验证者的处置 | 任务状态 |
|---|---|---|---|
| TASK-001 | 93.6% → 93.7%（+0.1pp） | PASS，报告用「**128 个既有函数逐函数百分比完全相同，无一下降**」的口径 | verified |
| TASK-002 | — | PASS，报告写「**覆盖率逐函数未降**」 | verified |
| **TASK-003** | **−0.3pp（降了）** | **✅ PASS** —— 报告原文「覆盖率 −0.3pp，但『**既有覆盖率**』未降」 | **verified** |

⇒ **TASK-003 与本任务是同一形状**（包级下降、既有逐函数未降），已由 test-agent-27 判 PASS 并被 Leader 接受。
**读法 B 是本 sprint 已经确立的口径**，不是我现在临时选的。本任务照同一口径判 PASS，与先例一致。

⚠️ 但先例只覆盖 `internal/hestia` 这一个包。**TASK-007 要动 `cmd/atlas`（基线约 75%），
读法 A 下它结构上不可能通过**，而读法 B 下它取决于「既有函数是否被改动」——
那条边界先例没走过，仍建议 Leader 在 TASK-007 派发前把口径写进 CONTRACTS。

**我按读法 B 判 PASS**，理由三条（先例之外的独立理由）：
1. 「既有」二字修饰的是**既有代码**，读法 B 更贴字面；
2. 读法 A 是个**棘轮**——每个新文件都必须高于当前包均值（94.2%），越往后越难，最终只会逼出覆盖率表演；
   而 `dev_done` 门禁已有绝对下限（80%），DoD 若是棘轮就与门禁重复且更严；
3. dev 没有粉饰，主动报数并给出更强口径的证明。

🔴 **但这个口径必须在 TASK-007 之前定死**：TASK-007 要动 `cmd/atlas`，而该包历史基线约 **75%**
（远低于 `internal/hestia` 的 94%）⇒ 读法 A 下它**结构上不可能通过**。这不是假设，是下一个任务就会撞上。

### 4.3 ℹ️ 我自己差点犯的一个错，值得记下来：**跨树混渲 coverage profile 会产出假差异**

我第一次做逐函数比对时，在 **atlas 根目录**下渲染两棵树的 profile ⇒ `go tool cover` 用**当前树**的源码
归属行号，报出 **5 条「既有函数覆盖率变化」**，其中 `Save 73.7% → 66.7%` 看起来像真的下降。
**我据此差点写下「dev 的 coverage_stronger_claim 不成立」。**

改成**各自在自己的树里渲染**后：**0 条差异**。我又直接从 profile 数了 `Save` 的语句块——
两棵树都是 **14/21 = 66.7%**，那个 `73.7%` 根本不存在。

⚠️ **dev 在 discovery 里明确警告过这一点**（「两份 profile 各自在自己的源码树里渲染……混渲会产出假差异」），
**而我照样撞了**。⇒ 读了警告不等于避开了它；**警告要变成命令行里的那个 `cd`，才算生效**。

### 4.4 ℹ️ `boundary[1]` 的理由段里有一句**已被实验证伪**的话（不影响判定）

DoD 写：「那个缺口在 TASK-005 是**不可达**的（两侧同值）」。
这句我在 TASK-005 验完后已用实验证伪并告知 Leader：`crossCheckBackfill` 是纯函数，两侧输入完全由测试构造，
在 TASK-005 自己的测试文件里改 4 行即可让两个 SURVIVED 全部转 KILLED。

时序上：DoD 定稿于 `11:21:58`，我的证伪在那之后到达 ⇒ **这句话写下时是当时的认知**。
**放置决定本身没问题**（TASK-006 确实是引入分叉的一方，测试放这里有独立价值），
**错的只是那条理由**。⇒ 建议顺手订正，否则后人会据「TASK-005 不可达」认为那边不必再补。

---

## 5. discovery 与指针字段

`discovery` 指针字段**已在**（`.arcforge/discoveries/TASK-006.json`）——**dev 在转 `dev_done` 之前自己写的**，
是本 sprint 第一次不需要验证者补。discovery 内容详实，且**主动披露了覆盖率下降**与
「profile 混渲会产假差异」的坑（我恰好据此发现了自己的错误，见 §4.3）。

---

## 6. 结论

**VERIFIED。** 9 条 done_criteria 的字面要求逐条有测试并经我自己的消融证明有鉴别力（20 个有效变异 KILLED 15）；
判定对象零漂移；无越界；`discovery` 指针齐备。

5 个 SURVIVED 全部落在**「实现正确、但缺测试背书」**这一类（`error_handling[1]` 的 6 条写盘路径中的 5 条），
**不是行为缺陷**，且 DoD 的字面要求（两条各一个独立用例）已满足 ⇒ 不构成 rejected 理由。

（`non_functional[1]` 的覆盖率口径已由 TASK-003 的先例消解，见 §4.2 补记。）

**两件事移交 Leader**：
1. **§4.2 的覆盖率口径必须在 TASK-007 之前定死**——`cmd/atlas` 基线约 75%，读法 A 下它结构上过不了。
2. **§4.1 的 5 条未钉路径**建议进 TASK-007 的 DoD 或走 `review_fix`。

⚠️ **本报告的边界**：这 20 个变异是我设计的，`SURVIVED 5` 只说明**我想到的那 20 种**里有 5 种没被挡住，不是「已穷尽」。
