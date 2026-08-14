# TASK-005 验证报告 · 两侧交叉校验：并集、两个差集与搜索侧 fail-open

- **验证者**：test-agent-28（Reality Checker）
- **判定**：**VERIFIED**
- **判定锚**：`a9d75720b0616a2b836a04e3a311f41c2da122b2`（= `verify_baseline.head`，**零漂移**：HEAD 逐字一致、`discovery_sha256` 逐字一致 `d4acd028…`）
- **验证工作区**：`/Users/zuowei/workspace/go/src/github.com/newthinker/wt-verify-T005`（detached @ `a9d7572`，收尾已 remove）
- **消融**：**20 个有效变异，KILLED 18 / SURVIVED 2**
- **2 个 SURVIVED**：交集条目的 `URL` 与 `Published` 取哪一侧**不可观测**（§4.1）。**不据此 rejected** —— DoD 未要求该行为，实现也是对的；但它是活的缺口，见 §4.1 的处置建议。

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 我自己的消融 | 判定 |
|---|---|---|---|---|
| functional[0] | 抓取集 = A ∪ B，交集只出现一次 | UnionsBothSides（逐 id 断言**定序**并集）+ PrefersIndexSideOnOverlap | U3 去 `\|\|` 的 `inIndex` 半 / U7 搜索侧独有不进抓取集 —— **2 KILLED** | **PASS** |
| functional[1] | `only_in_index = A \ B`，断言**具体 id 集合** | UnionsBothSides（`assert.Equal` 具体 id 切片，非条数） | U5 条件反了 —— **KILLED** | **PASS** |
| functional[2] | `only_in_search = B \ A`，断言具体 id 集合 | UnionsBothSides | U7 —— **KILLED** | **PASS** |
| boundary[0] | 两侧完全相同 ⇒ 两差集空、并集 = 任一侧 | IdenticalSides | U5/U6 —— **KILLED** | **PASS** |
| boundary[1] | 搜索侧**空集**（非错误）⇒ `only_in_index` = 全部 A、并集 = A、reason **为空** | EmptySearchIsNotFailure | X1′（把失效当空集处置）—— **KILLED**，且红**只在** SkipLeavesDiffsEmpty ⇒ 两种情形确被分开判 | **PASS** |
| **boundary[2]** | **补 TASK-004 日期守卫「上界」那半的测试背书** | RejectsAboveUpperBound + GuardsBothEnds（5 子测试表驱动） | **D1（就是我找到的那个 SURVIVED）—— 现已 KILLED**，详见 §2 | **PASS** |
| error_handling[0] | 搜索侧失效 ⇒ nil error + 抓取集 = 完整 A + `search_skipped_reason` 非空 | FailsOpenOnSearchError + SkipLeavesDiffsEmpty | X2 抓取集清空 / X3′ reason 留空 / X4 来源标 both / X5′ reason 不带原始错误 —— **4 KILLED** | **PASS**（口径见 §4.3） |
| non_functional[0] | gofmt/vet/test + 不降覆盖率 | 见 §3 | — | **PASS**（94.0% → **94.2%**） |

**dev 超出 DoD 的三处加固，逐条确认有承重消融**：

| 加固 | 承重证据 |
|---|---|
| `SkipLeavesDiffsEmpty`（失效时差集必须空，不得谎报） | **X1′ 是决定性的**：把「搜索侧为空」的处置复用到失效上（reason 照常给），**只有这条用例转红**，`FailsOpenOnSearchError` **仍然绿** ⇒ 它独立承重，不是冗余 |
| dedup 的 `\|\|` 两半分开钉 | U2 删 `seenSearch` 半 → 红在 `搜索侧重复` 子测试；U3 删 `inIndex` 半 → 红在 `UnionsBothSides` 等。**两半各自承重** |
| `Source` 字段（index/search/both） | U6 交集标成 index → KILLED；U8 搜索侧独有标成 index → KILLED；X4 失效时标成 both → KILLED |

---

## 2. boundary[2] —— 我在 TASK-004 找到的那个 SURVIVED，确认已闭合

**判据不是「有没有新用例」，是「删掉上界那半，红会不会落在正确的格子」。** 我用 `-v` 抓具体子测试：

| 变异 | 结果 | **红在哪一格** |
|---|---|---|
| **D1** `< lo \|\| > hi` → `< lo`（**原样复现我那个 SURVIVED**） | **KILLED** | `RejectsAboveUpperBound` + `GuardsBothEnds/`**`高于上界`** |
| **D2** 镜像：→ `> hi`（只留上界） | KILLED | `GuardsBothEnds/`**`低于下界`** + TASK-004 的 `RejectsOutOfRangePublished` / `ChecksAllColumnsDates` |
| D3 上界 `>` → `>=`（开区间） | KILLED | `GuardsBothEnds/`**`恰好等于上界`** + TASK-004 的 `AcceptsBoundaryDates` |
| D4 下界 `<` → `<=` | KILLED | `GuardsBothEnds/`**`恰好等于下界`** + `AcceptsBoundaryDates` |

⇒ **四格各自独立承重**，没有一格是被兄弟断言先红带过去的。D1/D2 成对说明**两个半边现在各有自己的红**，这正是缺口闭合的定义。

**同时确认了那条教训的证据本身**：`checkBackfillSearchDateRange` 的语句覆盖率
**闭合前是 100%、闭合后还是 100%** —— 同一个数字，两种现实。整个 `if` 是一条语句，两个半边共用一格。
这不是推论，是我两轮各测一次的实测值。

---

## 3. 我自己跑出来的证据

### 3.1 套件
```
go vet ./internal/hestia/           → 空（exit 0）
gofmt -l internal/hestia/           → 空
go test ./internal/hestia/ -count=1 → ok
```
全包 **顶层 PASS 391 / 子测试 PASS 387 / 真 FAIL 0**。
本任务 **17 条 RUN = 10 顶层 + 7 子测试**，全 PASS。

### 3.2 覆盖率（背对背 A/B/A′）
| 采样 | 内容 | 覆盖率 |
|---|---|---|
| A | 两个新文件移出 package | 94.0% |
| B | 移回 | **94.2%** |
| A′ | 再移出（复采） | 94.0% |

A′==A ⇒ 可复现，**升 +0.2pp**。`crossCheckBackfill` / `taggedCandidates` 均 **100%**。

### 3.3 消融（20 个有效变异，我自己的 harness）
纪律：独占 detached worktree；每个变异体先过 `go build` 有效性闸；打印 diff 逐字核对；
用 `-v` 抓 `--- FAIL` 行以确认**是哪一格红**；跑完还原并校验两个源文件的 sha256（均一致）。

**KILLED 18 / SURVIVED 2。**

**harness 自查暴露并纠正了三个无效变异**（如实记录，因为它们本可以被误记成 KILLED）：
- X3 / X5 首版让 `fmt` 变成未用 import ⇒ **编译不过**，被有效性闸拦下，改写后重跑（X3′/X5′ 均 KILLED）。
- **X1 首版是「混合变异」**：我本想隔离「失效被当成空集处置」，却顺手把 reason 也弄丢了 ⇒ 它同时杀掉两条用例，**证明不了 `SkipLeavesDiffsEmpty` 独立承重**。改成「reason 照常给、只把处置换掉」（X1′）后，才得到那个决定性结果。**一个 KILLED 若来自变异体的副作用而非目标行为，它什么都没证明。**

### 3.4 越界申报
`git show --numstat a9d7572`：
```
133  0  internal/hestia/backfill_crosscheck.go
308  0  internal/hestia/backfill_crosscheck_test.go
```
恰好两个声明的 `writes`，**纯新增 0 删除，无越界**。
（同区间 `backfill_scan*` 的改动来自 `d4b4866`，属 **TASK-002**，不在本任务提交内。）

⚠️ 值得记一笔：`boundary[2]` 要补的是 **TASK-004** 那个文件的守卫，而 `backfill_search_test.go`
**不在本任务的 `writes` 内**。dev 把新用例写在 `backfill_crosscheck_test.go` 里（同 package，可直接调
`checkBackfillSearchDateRange`）—— **既满足了 DoD，又没有越界**。这一步做对了。

---

## 4. 发现

### 4.1 🟡 两个 SURVIVED：交集条目的 `URL` 与 `Published` 取哪一侧**不可观测**

```
[SURVIVED] 交集取【搜索侧的 URL】     （Title 仍取 index 侧）
[SURVIVED] 交集取【搜索侧的 Published】
[KILLED  ] 交集取【搜索侧的 Title】   ← 对照组，红在 PrefersIndexSideOnOverlap
```

有对照组 ⇒ 结论**隔离到 URL/Published 两个字段**，不是 harness 失灵。

**成因**：`PrefersIndexSideOnOverlap` 让两侧的 **Title** 不同（"index 侧的标题" vs "搜索侧的标题"），
但测试辅助 `xcIndexItem` 与 `xcSearchHit` **用同一个模板拼 URL**、`Published` 也传同一个值
⇒ 那两个字段两侧**逐字相同**，取哪一侧在测试里没有任何差别。

**这与它自己注释里写的原则是同一条**：

> 「这条用例刻意让两侧的 Title 不同，**否则「取哪一侧」在测试里根本不可观测**」

dev 把这条原则应用到了 Title，**没有应用到旁边的 URL 与 Published**。

**为什么这是活的缺口而不是假想**（我查了才敢说）：
- index 侧的 URL 是**相对路径**（实测 `pboc-index-p1.html` 里是 `/goutongjiaoliu/113456/113469/<id>/index.html`）经 `resolveURL(base, href)` 补全；
- 搜索侧的 URL **本就是绝对的**（TASK-004 实测 12/12 以 `https://www.pbc.gov.cn/` 开头）；
- 而 `base` 是 `scanBackfillPage(html, base)` 的**参数**，由**尚未实现的 TASK-006** 传入。

⇒ **两侧 URL 是否同形，在本 sprint 已提交的代码里还没有被确定**（`http` vs `https`、host 形式任一不同即分叉）。
一旦分叉，「交叉校验不该反过来改写主路径的数据」这条 dev 明确写下的设计意图会**零信号地失效**，
而 TASK-006 会照着那个 URL 去下载。

**为什么仍判 PASS**（三条都成立才放行）：
1. **DoD 没有要求这个行为** —— functional[0] 只要求「并集 id 正确、交集只出现一次」，
   「交集取哪一侧的字段」是 dev **超出 DoD** 的设计决定。
2. **实现是对的** —— 现在取的确实是 index 侧。
3. **今天不会出错** —— 两侧 URL 在当前语料下预期同形。

**处置建议（供 Leader 选，我不代选）**：
- (a) **在本任务补**：把 `PrefersIndexSideOnOverlap` 里搜索侧的 URL/Published 改成不同值、加两条断言 —— **两行**。
  我倾向这条：**「取哪一侧」这个决定完全发生在 TASK-005，不在消费点**，与 boundary[2] 那次
  （`to` 是参数、消费者才提供真实值）**结构不同**，那次「在消费点补」的理由这里不成立。
- (b) 登记为 TASK-006 的一条 DoD（它是 `Fetch[].URL` 的消费者，也是 `base` 的提供者）。

### 4.2 🔵 一个值得单独说的观察：**教训被用在了被点名的地方，没有被用到旁边同形的地方**

同一个文件里，同一条原则，三处：

| 位置 | 是否应用了「两半/多字段必须分开钉」 |
|---|---|
| dedup 的 `if inIndex[id] \|\| seenSearch[id]` | ✅ **应用了**，且注释明确引用我在 TASK-004 的发现，拆成两条子测试、各做消融 |
| `checkBackfillSearchDateRange` 的两半（boundary[2]） | ✅ **应用了**，5 格表驱动 |
| **交集字段偏好的三个字段（Title/URL/Published）** | ❌ **只应用到 Title** |

三处相隔不到 200 行。⇒ **把教训用在「别人指给你的那个位置」，不会自动用到「结构相同的旁边那个位置」。**
这不是 dev 的疏忽指控 —— 前两处它做得比 DoD 要求的更细致；而是说明**「已经学到这条教训」不构成
对下一处同形位置的保护**，只有把它变成一个**可机械执行的检查**（「这条断言里，有几个字段/分支两侧取值相同？」）才行。

### 4.3 ℹ️ `error_handling[0]` 的「三个断言」实为「两个断言 + 一个由类型签名保证」

DoD 写「断言：搜索侧报错时**返回 nil error**、抓取集仍等于完整的 A、且 `search_skipped_reason` 非空
（**三个断言都要**）」。而实现的签名是

```go
func crossCheckBackfill(index []backfillItem, search []backfillSearchHit, searchErr error) backfillCrossCheck
```

—— **根本没有 error 返回值**。「返回 nil error」因此**不可能被违反**，也就无从断言。
这比一条断言**更强**（断言可以被删掉，签名不行）。**如实记录，免得后人照 DoD 字面数断言时以为少了一条。**

### 4.4 ✅ 跨任务缺口已确认落地（不是空头承诺）

discovery 的 `key_findings` 第 1 条报了一个真缺口：`Manifest` **没有** `search_skipped_reason` 字段
⇒ 本任务返回的 `SearchSkippedReason` 在 TASK-006 落盘之前是「**返回了但没人存**」，
而「一个有声跳过的机制，若那个声音无处可放，等于机制不存在」。

**我核实了裁决是否真的落盘**（而不是只停在 discovery 的叙述里）：

```
TASK-006.writes  → 已含 ./internal/hestia/backfill_manifest.go 与 _test.go
TASK-006.DoD     → 含「给 Manifest 补上 SearchSkippedReason string `json:"search_skipped_reason,omitempty"`，并在 fail-open 时落盘」
```

✅ 已闭合，不是悬空。

---

## 5. discovery 与指针字段

`discoveries/TASK-005.json` 内容齐备（`key_findings` / `decisions` / `interfaces_exposed` 均已登记，
含「本函数**不返回 error**」这一关键契约）。

任务 JSON 原本**缺 `discovery` 指针字段**（这是本 sprint 第 4 次，四次都由验证者补），
我已在转 `verified` **之前**补上。

---

## 6. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试并经我自己的消融证明有鉴别力；
`boundary[2]`（我在 TASK-004 找到的缺口）**确认闭合，且四格各自独立承重**；
覆盖率背对背可复现地上升；无越界申报；判定对象与 `verify_baseline` 零漂移。

两个 SURVIVED（§4.1）落在 **DoD 未要求、dev 超出 DoD 的一个设计决定**上，实现正确、今日不出错，
⇒ 不构成 rejected 理由；但它是**活的**（两侧 URL 是否同形取决于 TASK-006 才会传入的 `base`），
已给出两行的闭合方案与两条处置路径，请 Leader 选。

⚠️ **本报告的边界**：这 20 个变异是我设计的，`SURVIVED 2` 只说明**我想到的那 20 种**里有 2 种没被挡住。
它不是「已穷尽」的证明——同一句话我在 TASK-004 复验时对 dev 的 26 个变异说过，对我自己同样适用。
