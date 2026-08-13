# QA 阶段预登记项（Leader 维护 · Sprint 037）

> ⚠️ **本文件原打算写在 `docs/05-review/`，被写通道 DENY（那里是 `qa-*` 专属，leader 无权写）。**
> 机制是对的，故改放 `docs/02-plan/`。**QA Agent 开工时请主动读这一份。**


**QA Agent 必读。** 这里记的是**在 QA 开始之前就已确认成立、但没有合法写者能修**的问题。
它们不是让 QA 去发现的，是让 QA **走正规边把它们送回返工**的 —— 因为
`verified` 之后交付物在工作区里无主（见结转发现 **H37**），
唯一合法的修复路径是 `verified → review_fix`（leader 边，带 `fix_items` + `reason_class`）。

⚠️ **不要在 QA 之外直接改这些文件。** 那正是 H37 记的那个盲区。

---

## QA-PRE-1 — `internal/hestia/ingest.go:160-163` 的注释结论已过期

**发现者**：dev-agent-54（2026-08-13，它自己不碰，因为 `ingest.go` 不在它的 `writes` 里）。
**Leader 已复核成立。**

原文：

```go
// Verdict 与 Table 都打出来。Table 是当下就必须区分的（入权威表 vs 落 pending
// 对运维的含义相反）；Verdict 当前恒为 New（经 Discover 过滤的候选必然不在
// 当前行里），打它是为了将来有人放开那条路时，Duplicate/Revision 自己会显形，
// 而不是悄悄混在「已入库」里。
```

🔴 **「Verdict 当前恒为 `New`」现在是假的**，理由有两条，第二条是**可执行的证据**：

1. **TASK-011**（`fd6a24c`）把 `Discover` 判停从 `HasPeriod`（期次）换成
   `HasArticleInObservations`（article_id）⇒ **判停不再按期次过滤**，
   旧期次的新文章进得了候选 ⇒ `Duplicate`/`Revision` 可达。
2. **TASK-007 有一条绿测试直接反证它**：`TestForceOnObservedPeriodIsDuplicate`
   断言 `--force` 重跑已在观测表的期次会**走到 `Save` 且判 `Duplicate`**。
   ⇒ 这条注释与仓库里一条通过的测试**直接矛盾**。

**建议 `fix_items`**：把「当前恒为 `New`」改成如实描述
（`--force` 或旧期次新文章可产生 `Duplicate`/`Revision`，并指向那条测试）。
**`reason_class`：`stale_premise`（非 dev 失误）。**

### 为什么这条值得留着而不是悄悄改掉

**注释里那个决定本身是对的，而且这次被实地验证了。** dev-54 当时**刻意没有把
「恒为 New」写成断言**，理由是「钉死当前局限，将来修好时红的理由是反的」。
⇒ 于是 TASK-011 放开那条路时**没有产生任何假红**，`Verdict` 照常打进 `Out`，
**那条路一放开就自己显形了**。

⇒ **该记的是：「不要为当前的局限写断言」这条判断，在同一个 Sprint 内就收到了回报。**
过期的是**注释的结论**，不是那个决定。修的时候别把决定一起删了。

---

## QA-PRE-2 — 跨工作流并发写者（H37）：请核一次已验交付物的验后漂移

Leader 已于 `2026-08-12T18:04Z` 跑过一次，**九个已验任务全部「验后未变」**。
请 QA 在**自己开始审查时重跑一次**（判据是自己量的，不引用我的结论）：

```bash
for t in 001 002 003 004 005 006 007 010 011; do
  f=.arcforge/tasks/TASK-$t.json
  b=$(jq -r '.verify_baseline.head' "$f"); w=$(jq -r '(.writes // .packages)[]' "$f")
  echo "TASK-$t: $(git diff --name-only "$b" HEAD -- $w | tr '\n' ' ')"
done
```

**背景**：一个来自全局 `~/.claude/CLAUDE.md` 规范的 `code-simplifier` 后台 agent
挂死 2 小时，手上握着对 `internal/hestia/ingest_test.go`（TASK-007 交付物，已 `verified`）
的待执行 `Edit`。实际无幸存改动，Leader 已 `TaskStop`。**但 Arcforge 没有任何机制会拦它** ——
详见 H37 的机制覆盖表。

---

## QA-PRE-1b — **同一句过期结论还有第二份副本，且它比第一份更误导**

**发现者**：dev-agent-54（2026-08-13，QA-PRE-1 登记之后）。**Leader 已逐行复核，成立，
且比它描述的更糟。**

位置：`internal/hestia/ingest_test.go:287-291`

```go
// ⚠️ 这里刻意**不**断言 Verdict 恒为 New：经 Ingest 的候选都已通过 Discover 的
// HasPeriod 过滤（discover.go:303-318）⇒ Save 的 Lookup 必然查不到当前行 ⇒ Verdict
// 结构上恒为 New，Duplicate/Revision 当前不可达。…
```

🔴 **三处过期，最后一处是 Leader 复核时新发现的**：

1. **「`HasPeriod` 过滤」** —— TASK-011 已换成 `HasArticleInObservations`。
2. **「`Duplicate`/`Revision` 当前不可达」** —— 同文件 **`:705`** 的
   `TestForceOnObservedPeriodIsDuplicate` 是绿的，它断言 `Duplicate` **可达**。
   **两者相隔 416 行，同一个文件。**
3. 🔴 **那个坐标 `discover.go:303-318` 现在指向的，正是它自己的反证**（Leader 实测）：

   ```
   // ⚠️ **判据是 article_id 而不是期次**（TASK-011）。别改回期次：修订版重发时发布时间
   // 最新、期次却是旧的、已入库 ⇒ 第一条就命中、当场停，**其后所有未入库期次静默消失**。
   ```

   ⇒ **照这个坐标去查的人，会落在一段明确说「不是期次」的注释上** ——
   而把他送过去的那句话说的是「按 `HasPeriod`（期次）过滤」。
   **一个过期指针指向了自己的反驳。**

⇒ **两处必须一起改。** 只改 `ingest.go` 的话，留下的这份**因为紧挨着反证反而更容易被当成权威**。

### 修改判据（dev-54 给的可执行版，采纳 —— 修的人不必判断「哪句是决定哪句是结论」）

> **保留**：「**不要为当前的局限写断言**」这条理由，以及它的实证 ——
> `TestForceOnObservedPeriodIsDuplicate`（`ingest_test.go:705`）现在是绿的，
> 而它**从未因为那条局限被放开而红过一次**。
>
> **删除**：「恒为 `New` / `Duplicate`、`Revision` 当前不可达」这个**事实陈述**，
> 以及那个过期坐标 `discover.go:303-318`。

**`writes` 归属**：`ingest.go` → TASK-005 / TASK-011（均 `verified`）；
`ingest_test.go` → TASK-007（`verified`）。**三者都无合法写者** ⇒ 必须走 `verified → review_fix`。
建议一并派回 **dev-agent-52**（TASK-007 与 TASK-011 的 owner，两个文件它都熟）。

---

## QA-PRE-1c — **第三份副本**（Leader 全仓扫出）：`internal/hestia/store.go:216`

我按 H26/H33 的纪律把扫描范围从「DoD」扩到「**代码注释**」，`grep` 了三类锚
（「恒为 New / 不可达」、把 `HasPeriod` 描述成判停规则、`discover.go:NNN` 形式的坐标）。
**又扫出一处**：

```go
// HasPeriod 回答某期是否已在权威表里。Discover 用它决定翻页何时停。
```

🔴 **「`Discover` 用它决定翻页何时停」现在是假的** —— TASK-011 之后 `Discover` 用的是
`HasArticleInObservations`。`HasPeriod` 仍被 `Ingest`/`status` 用，但**不再是判停规则**。

⇒ **同一句过期结论目前共三份**：`ingest.go:161`（QA-PRE-1）、
`ingest_test.go:287-291`（QA-PRE-1b）、`store.go:216`（本条）。**三处一起改。**

**已确认「不必改」的**（同批扫出、逐条看过，留在这里免得后人重扫）：

| 位置 | 为什么不用改 |
|---|---|
| `discover.go:205` | 标题就是「**为什么从 HasPeriod 换成 HasArticle（TASK-011，推翻此处原有的论证）**」——已更新 |
| `ingest_test.go:659` | 已含「那在 TASK-011 **之前**是对的」的时态限定——dev-52 更新过 |
| `discover.go:143` / `discover_test.go:971` | 「不可达」指的是 `Atoi` 错误分支与 `scanPage` 出错路径，**与本议题无关**，仍然成立 |

---

## QA-PRE-1d — 🔴 **修的时候不要「更新行号」，要把行号锚整个删掉**（dev-agent-54 提，Leader 采纳）

dev-54 复核 QA-PRE-1b 时发现：`ingest_test.go:288` 引用的 `discover.go:303-318`
**不只是内容变了，是整个漂到了另一类东西上** —— 它当初指的是**过滤循环体**，
今天那 16 行是 **`Discover` 的文档注释**。

> 行号是**会随任何上游编辑静默漂移的锚**，而漂移之后它**看起来仍然完全正常**。
> 这次正好漂到一段反驳自己的文字上，属于运气；**下次可能漂到一段读起来毫无违和、
> 却与论点无关的代码上，那种更难发现。**

⇒ **这是「验证命令的锚必须钉全 sha、不能写 `HEAD`」的注释版**：
**注释里的 `file:line` 就是注释版的 `HEAD`。**

⇒ **改法**：删掉行号，改引用**符号名 + 字串锚**，例如

```
discover.go 的 Discover 判停分支（搜 "判据是 article_id"）
```

**理由**：字串锚会**随内容一起失效**，不会像行号那样**在失效之后仍然指向某个存在的东西**。

⚠️ dev-54 的自陈值得留着：**「我自己写的时候没意识到这点。」**
⇒ 那处过期注释一共踩了三层：机制名过期、结论过期、**第三层纯粹是锚的类型选错了**。
前两层是内容问题，第三层是**形式问题，而形式问题会让内容问题更难被发现**。
