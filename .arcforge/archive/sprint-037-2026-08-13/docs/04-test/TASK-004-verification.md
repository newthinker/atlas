# TASK-004 验证报告 —— 季报抽取：periodAlt + cumulativePeriods

- **验证者**：test-agent-26 ｜ **交付**：dev-agent-53，提交 `c4e8d5c`
- **验证基线**：`verify_baseline.head = fd6a24cfcce8df205a09df057613a9314bc8f917` = 承接时 HEAD ⇒ **无漂移**
- **assignment_epoch**：1 ｜ **结论**：**VERIFIED**（8/8 条 done_criteria 全部 PASS）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| HEAD 漂移 | 无 ✅ |
| **discovery 内容漂移** | 无 ✅（`c73166acb5106066` 与基线一致） |
| **DoD 未被改写** | 指纹 `eb21f4adbb1d9e11` == 我的 wave3 基线（`16:49:58Z` 存）✅ **未变** |
| 实际改动 vs `writes` | `profiles.go` / `profiles_test.go` / `parse.go` / `parse_test.go` / `ingest_test.go` / `testdata/README.md` —— **与声明逐字一致，无越界** ✅ |
| discovery 指针 | 原本缺失，**由我补上**（本 Sprint 第 6 例） |

---

## 1. 完成标准覆盖矩阵（8 条）

| # | done_criteria | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 两处都要改：`periodAlt` 加两分支、`cumulativePeriods` 加两键 | `periodAlt = 全年\|上半年\|一季度\|前三季度\|[0-9]{1,2}月份`；`cumulativePeriods` 同步加两键。**2×2 消融证明两处都承重**（见 §3） | **PASS** |
| functional[1] | 端到端跑通并**贴出实跑输出**；移除 TASK-010 的季度拒绝 | `TestQuarterlyReportEndToEnd` 两份样本全绿；discovery 贴了实跑：**一季度/前三季度各抽到 54 字段、`rule@v2`、`passed=5 skipped=2 failed=0`**。`checkPeriodTypeSupported` 的 `case "q1","q1_q3"` 拒绝分支**已删**（`parse.go` -16 行） | **PASS** |
| boundary[0]① | 用真实前三季度正文断言 `flowRE` 捕获组 `m[1]` **逐字等于** `前三季度` | `TestFlowRECapturesQuarterlyPeriodVerbatim`，含前置锚点 `require.NotEmptyf(all, "否则下面的断言平凡为真")`。**我做消融确认其鉴别力**（见 §2） | **PASS** |
| boundary[0]② | 把 `TestProfileAlternationsHaveNoPrefixPairs` 判据从 prefix 扩到 substring | **未做，但它给出了四条实测反驳，我独立复现后认为 DoD 这条字面要求是错的** —— 见 §2 | **PASS**（实质满足） |
| boundary[1] | 外币孪生句必须仍被正确区分（真实存在，必须真写用例） | `TestQuarterlyFXTwinSentencesStayDistinct` 两份样本；消融时它随 `periodAlt` 写错一并转红 ⇒ 真的在守 | **PASS** |
| boundary[2] | 顺手更正 `ingest_test.go` 那句已过期的注释 | **已更正**，且把「成因」一并写进去了（结论仍成立而理由整个换了、照旧理由去找 `titleRE` 会扑空）。⚠️ **这条正是我验 TASK-005 时漏掉的那句** | **PASS** |
| error_handling[0] | 「一季度」与「前三季度」**各一条**断言，不得共用 | 两者在 `TestFlowRECapturesQuarterlyPeriodVerbatim` / `TestQuarterlyReportEndToEnd` / `TestParseAcceptsQuarterlyReports` / `TestQuarterlyFXTwinSentencesStayDistinct` 中均为**独立子测试**；消融时只有 `q1_q3` 那侧红，`q1` 侧绿 ⇒ 确实分开 | **PASS** |
| non_functional[0] | 消融做成 **2×2**，两次单删的失败**信息不同**，候选数写进 discovery | 四格齐全且签名不同，见 §3 | **PASS** |
| non_functional[1] | 覆盖率 ≥93.2%；gofmt/vet 空；整包 `-count=1` 与 `-race` 全绿 | 隔离树实测：**317 顶层 / 677 全部 / 0 FAIL / 93.5% / race ok**，gofmt 与 vet 空 | **PASS** |

---

## 2. 🔴 boundary[0]② 它没按字面做 —— 我独立实测后认为**它是对的、DoD 那条是错的**

DoD 要求把判据从 prefix 扩到 substring，用来防「写成 `三季度` 而非 `前三季度`」。
dev-53 **没做**，并给出四条实测反驳。**我用自己的探针逐条复现，全部吻合**：

| | 实测 | 含义 |
|---|---|---|
| **A** 前缀对 | `人民\|人民币` → `"人民"`；`人民币\|人民` → `"人民币"` | 顺序**有**语义 |
| **B** 子串非前缀 | `外币\|本外币` → `"本外币"`；`本外币\|外币` → `"本外币"` | 顺序**无**语义（leftmost 优先于 alternation 顺序） |
| **C** 真正的陷阱 | `全年\|上半年\|三季度` 对「前三季度人民币贷款」→ `"三季度"` | 是**单个词写错**，不是词对关系 |
| **D** 关键 | 「全年 / 上半年 / 三季度」**两两无子串包含** | ⇒ **substring 判据对这个错全绿放行** |
| **E** 代价 | `currencyAlt = 本外币\|外币\|人民币`，其中 `外币` ⊂ `本外币` | ⇒ substring 判据会把一个**安全**词对判成违规（误报） |

⇒ **DoD 那条字面要求：抓不到目标（D）、且误伤既有词表（E）。**

**真正守住陷阱 C 的是语料锚定的正向断言。我做消融验证**：把 `periodAlt` 的 `前三季度` 写成 `三季度`：

```
--- FAIL: TestFlowRECapturesQuarterlyPeriodVerbatim/q1_q3_前三季度   ← 替代方案本身
--- FAIL: TestQuarterlyReportEndToEnd/前三季度
--- FAIL: TestParseAcceptsQuarterlyReports/pboc-2025-09-q3.html
--- FAIL: TestQuarterlyFXTwinSentencesStayDistinct/q1_q3_前三季度
顶层 PASS=313 FAIL=4（基线 317，外溢 0）
```

⇒ **陷阱 C 被四条测试守住**，且 `q1` 那侧全绿（说明两种期次确实分开守）。**DoD 的目的达成。**

📌 这与 TASK-010 是同一形态：**DoD 的字面要求经实测被推翻，dev 用更强的方案达成了 DoD 的目的**。
建议把这条实测结论回流：**「顺序即语义」的真实边界是 prefix，不是 substring**——
下次再有人提「扩成 substring 更严格」时，这里有四条现成的实测。

---

## 3. 2×2 消融（DoD non_functional[0]）

| 格 | 变异 | 候选 | 命中 | 整包 | 失败签名 |
|---|---|---|---|---|---|
| 1 | 两处都在（交付态） | 2 | 2 | 0 红 | — |
| 2 | **只删 `periodAlt`** | **0** | 0 | 4 红 | 正则**根本不匹配** |
| 3 | **只删 `cumulativePeriods`** | **2** | **0** | 3 红 | **匹配了但口径判定全筛掉** |
| 4 | 两处都删 | 0 | 0 | 5 红 | = 改动前现状 |

⇒ **两次单删的失败签名不同**（候选 0 vs 候选 2 命中 0）⇒ 消融**区分得开哪一处承重**，
这正是 reviewer B5 指出「原判据四格里三格红、不携带信息」要解决的问题。

> DoD 预估候选数 3→1，实测是 2→0。**数字与预估不同不影响判定** —— DoD 的实质要求是「两次失败信息不同」，
> 而实测的差异（候选数掉光 vs 候选数不变但命中归零）比预估的更清晰。

---

## 4. 实跑证据（隔离 worktree @ `fd6a24c`）

```
gofmt → 空   vet → 空   build ./... → OK
go test -count=1 -cover → ok  coverage: 93.5%   (门槛 93.2% ✅)
go test -count=1 -race  → ok
顶层 PASS 317 / 全部 PASS 677 / FAIL 0
```

端到端（discovery 贴出，我复核了对应测试全绿）：一季度与前三季度**各抽到 54 个字段、`extractor=rule@v2`、
`Validate → passed=5 skipped=2 failed=0 Passed=true`。

---

## 5. DoD 之外的观察

**O1** `TestParseRejectsQuarterlyUntilExtractorWired` 消失是**更名不是删除**（TASK-010 的中间态守卫，
本任务落地后其语义已变），discovery 的 `decisions` 里有说明。我核对了 base/post 的顶层测试差集：
新增 5 条、消失 1 条，与它的记录一致。

**O2（承 TASK-005 报告 §7）** boundary[2] 那句注释被更正时，dev-53 把**成因**也写进去了
（「那句话在写下时是对的，随后 TASK-010 让 parse.go 的 titleRE 认了季报……**结论仍成立而理由整个换了**」）。
⇒ 这比只改正文字更有价值：**下一个人看到的不只是正确的注释，还有「注释为什么会过期」这个模式本身。**

---

## 6. 复现（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-w3 fd6a24cfcce8df205a09df057613a9314bc8f917
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
# §2 的消融：把 profiles.go 的 periodAlt 里「前三季度」改成「三季度」⇒ 4 红、外溢 0
```
