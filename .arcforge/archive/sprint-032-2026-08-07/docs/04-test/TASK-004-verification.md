# TASK-004 验证报告 —— CurrentQuery 与 AsOfQuery

- **验证者**：test-agent-19（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：commit `89bc09c` @ `feat/macro-bitemporal`，`query.go`(51 行) + `query_test.go`(256 行)
- **承接时 assignment_epoch**：1
- **隔离**：`git worktree add --detach ../wt-verify-TASK-004 89bc09c`；全部变异在 worktree 内注入并逐条还原，收尾 4 个文件 md5 全部回到基线、`git status`/`git diff` 均空，worktree 已从主仓库拆除，主工作区零污染
- **判定**：**VERIFIED** —— 14 条 done_criteria 全部通过
- **附 4 项发现**（均非 criteria 失败）与**三处诚实记录的裁定**，见第 5–7 节

---

## 1. ⚠ 先厘清三个基线数字：派验单的「包级 120 PASS」不是交付物属性

这是本任务最容易出错的地方，也是我第一件核的事——**「0 红自证」的第三条要拿 PASS 计数当锚，锚错了整套自证就失效。**

| 数字 | 来源 | 我的复现 | 性质 |
|---|---|---|---|
| **91** | **交付 commit `89bc09c` 的包级 `--- PASS`** | 三个正则一致（`^\s*`/`^[[:space:]]*`/定串） | ✅ **本次验证的基线锚** |
| 120 | 主工作区，**含在途 TASK-005 的 `lookup.go`/`lookup_test.go`** | 精确复现（主工作区实跑 = 120） | ⚠ **不是交付物属性**，随 TASK-005 进展漂移 |
| 45 | dev 的隔离 file-list（`spec.go classify.go query.go fixture_test.go query_test.go`，**不含 `spec_test.go`**） | 精确复现 = 45 | ✅ dev 的变异基线，**隔离得当** |

**dev 在 discovery 里明确标注了「含 TASK-001/002/003/005 的测试」**，并且**变异基线用的是隔离的 45 而非 120**——方法论是对的，证据链干净。**标注是在 Leader 转述为「包级 120 PASS / 0 SKIP」时丢失的**，派验单因此把一个受在途任务污染的数字呈现为交付状态。

这与 test-agent 角色定义里 TASK-015 的教训同形：**在被别的在途任务污染的工作区里量出的数字，被当成了被验对象的属性。**

## 2. 覆盖矩阵（14 条，全部 PASS）

| # | 完成标准 | 对应测试 | 我独立跑的变异 | 判定 |
|---|---|---|---|---|
| functional[0] | Current 只返最新；AsOf 修订前后不同 | `TestCurrentQueryReturnsLatestRevision`、`TestAsOfQueryReturnsHistoricalRow` | **M1**(MAX→MIN)、**M5**(去时点) KILLED | PASS |
| functional[1] | 两套形状（hestia+crisis）并行 | 7 处 `bothShapes` + `t.Run` | 变异输出中子测试名带 `/hestia` `/crisis`，断言另带「形状 %s」（F2 双保险） | PASS |
| functional[2] | **契约 T1（本任务最重要）** | `TestCurrentQueryReturnsLatestRevision` | **T1a**(correlate `AND`→`OR`) **KILLED，17 个子测试红** | PASS |
| boundary[0] | 空表 → 零行 | `TestCurrentQueryOnEmptyTable` | **M13 不杀它**（证实诚实记录①） | PASS（见 §7①） |
| boundary[1] | 乱序插入仍取最大 revision | `TestCurrentQueryIgnoresInsertOrder` | M1 / M13 KILLED | PASS |
| boundary[2] | 单列业务键 | `TestQueriesWorkWithSingleColumnKey` | **M10** KILLED（全包红 **2** 个，见 §5.2） | PASS |
| boundary[3] | 零值 Spec 引用**既有决定** | `TestZeroSpecProducesSyntaxErrorAtExecution` | **M6**(加防御) KILLED 且**唯一红** | PASS |
| boundary[4]① | as-of 恰等于某 revision（`<=` 包含性） | `TestAsOfQueryAtExactRevision` | **M3**(`<=`→`<`) KILLED、**M4**(`<=`→`>=`) KILLED | PASS |
| boundary[4]② | as-of 早于全部 revision → 零行 | `TestAsOfQueryBeforeAllRevisions` | M4 / M8 KILLED | PASS |
| boundary[4]③ | 空表上的 AsOfQuery | `TestAsOfQueryOnEmptyTable` | 反向变异 R1 红 | PASS |
| error_handling[0] | C7：SQL 字符串断言仅一处 | `TestQueriesContainNoLiteralValues` | grep 证实仅 L211-214，全在该测试内 | PASS |
| error_handling[1] | 标识符只来自 Spec；alias 过 identRE | `TestQueryAliasPassesIdentifierGate` | **M7** KILLED；**P1：90 PASS / 1 红 ⇒ 闸门是唯一守护** | PASS（限度见 §5.4） |
| non_functional[0] | 0 SKIP / 头部 Checkpoint / 中文注释 / `?` 占位符 | 全项 | **M8**(`?`→字面量) KILLED；实测 **0 SKIP** | PASS |
| non_functional[1] | map 以**完整业务键**为键 | `currentRows` | **M9**(改回第一列) KILLED，**12 个子测试红** | PASS |
| non_functional[2] | 「0 红」须自证三条 | — | **已实证三条的相对强度**，见 §4 | PASS |
| non_functional[3] | 子代理报「无改动」后须 `diff` 核实 | — | 三处 simplifier 改动**已核语义等价**，见 §6 | PASS |

覆盖率：`CurrentQuery` 100.0% / `AsOfQuery` 100.0% / package total 100.0%。`go vet` exit 0。
越界申报：`git diff --name-only 89bc09c~1..89bc09c` = `query.go` + `query_test.go`，**与 `writes` 声明完全一致，无漂移**。

## 3. 变异明细（全部由 test-agent-19 独立注入实跑；harness 内建自证三条）

我的 harness 每条都输出 `PASS=n/基线91  FAIL=n  vet_exit=n  命中文件`，并在**所有文件零变化时直接判「靶不存在，结论作废」**。

| ID | 判据 | 变异 | 结果（PASS/基线91） |
|---|---|---|---|
| **T1a** | functional[2] 契约 T1 | `correlate` 的 `AND`→`OR` | **KILLED** 74/91，17 红；含 `TestCurrentQueryReturnsLatestRevision/{hestia,crisis}` |
| **M1** | functional[0] | `CurrentQuery` 的 `MAX`→`MIN` | **KILLED** 84/91 |
| **M2** | functional[2] | 子查询关联换 `1=1` | **KILLED** 81/91 |
| **M3** | boundary[4]① | `AsOfQuery` 的 `<=`→`<` | **KILLED** 84/91 |
| **M4** | boundary[4]① | `<=`→`>=` | **KILLED** 81/91 |
| **M5** | functional[0] | 去掉时点条件 | **KILLED** 80/91 |
| **M6** | boundary[3] | 给零值 Spec 加防御返空串 | **KILLED** 90/91，**唯一红** = `TestZeroSpecProducesSyntaxErrorAtExecution` |
| **M7** | error_handling[1] | `queryAlias`→`"1o"` | **KILLED** 67/91（含闸门测试本身红） |
| **M8** | non_functional[0] | `?` 换字面量 | **KILLED** 84/91 |
| **M9** | non_functional[1] | map 键改回第一列 | **KILLED** 79/91，12 红 |
| **M10** | boundary[2] | 单列键时 `correlate` 返 `1=1` | **KILLED** 89/91，红 **2** 个（见 §5.2） |
| **M11b** | error_handling[0] | `CurrentQuery` 混入 `?` 占位符 | **KILLED** 80/91 |
| **M13** | boundary[0] | 去掉整个 `WHERE` | **KILLED** 83/91；**空表测试未红** |
| **P1** | error_handling[1] 深探 | alias 换成 sqlite 合法但 identRE 拒收的带引号标识符 | **KILLED** 90/91，**唯一红 = 闸门测试** |
| **P2** | error_handling[1] 深探 | 加不过 Spec 的 `ORDER BY rowid` | **存活** 91/91（自证三条齐备，见 §5.4） |
| **M11-dev** | ② | 变异期望值为 **0** 的那条断言 | **存活** 91/91（自证三条齐备） |
| **M11-mine** | ② | 变异期望值为 **1** 的那条断言 | **KILLED** 90/91 |
| **M12** | ② | `assert.Empty`→`assert.NotNil` | **存活** 91/91（自证三条齐备） |
| **R1** | §6 | `SELECT *`→`SELECT nosuchcol`（查询报错） | 两个空表测试**均红** |
| **R2** | §6 | 改成无 GROUP BY 的聚合（空表返非零行） | 空表测试**红** |

改 `spec.go` 的变异（T1a / M10）逐条还原后 md5 = `59934cd2238daeeedb3ab9c8494cc437`，与 TASK-001 交付版一致。

## 4. dev 那两个判断：我独立实证，**均成立**

**判断原文**：「第三条（PASS 计数）确实比第二条强。这里 `vet` 也报了 1，**但如果 harness 只看 `--- FAIL` 计数（多数 harness 的默认写法），两条都不会触发**。」

| 场景 | 构造 | FAIL 计数 | PASS 计数 | vet_exit | 结论 |
|---|---|---|---|---|---|
| **A**（复现 dev 的原缺陷） | zsh 下 `go test $FILES` 未加引号 | **0** | **0** | 1 | 报错原文逐字复现：`malformed import path "spec.go classify.go …": invalid char ' '`。⇒ **只看 `--- FAIL` 的 harness 会报「12 个变异全存活」** |
| **B**（我构造，用于检验「第三条更强」） | `-run` 过滤打空 | 0 | **0** | **0** | **vet 绿放行，第三条拦下** ⇒ **PASS 计数严格强于 vet** |

场景 B 是我为了验证 dev 的**相对强度**主张而构造的——dev 只观测到「两条同时触发」的场景 A，**这不足以证明第三条更强**。场景 B 补上了那个区分：存在 vet 通过而 PASS 计数拦下的情形，反之不存在。dev 的判断成立，但它当时的证据不足以支撑，是我补的。

## 5. 四项发现（均非 criteria 失败）

### 5.1 派验单的「包级 120 PASS」不可从交付物复现 —— 严重度：低（记录准确性）
见 §1。dev 标注了「含 TASK-005」，转述时标注丢失。**建议：涉及并行任务的包级计数，一律注明测量时的工作区状态或直接用交付 commit 复测。**

### 5.2 M10「只红一个」是隔离 harness 的属性，不是包的属性 —— 严重度：低
dev/派验单称 M10「**只红了 `TestQueriesWorkWithSingleColumnKey` 一个**」。**我在全包下实测红 2 个**：
`TestQueriesWorkWithSingleColumnKey` + **`TestCorrelate`**。
根因：dev 的隔离 file-list **不含 `spec_test.go`**，`TestCorrelate` 在它的 harness 里根本不存在。

**实质结论不变**：`TestCorrelate` 是**字符串级**断言，按契约 T1 的定义**不构成行为守护**——所以「单列键只有一处**行为**守护」这个结论仍然成立，Leader 在派发前修正措辞的判断也仍然成立。但**表述应加限定词**：是「唯一的行为守护」，不是「唯一红的测试」。

这与 §5.1 是同一形状：**在缩小的 harness 里量出的数字，被表述为包的属性。**一个任务里出现两次。

### 5.3 M11 我测 KILLED、dev 报存活 —— 根因已定位，dev 结论方向对但适用范围窄 —— 严重度：低
两条断言的**可变异性不对称**，我用两个变体分离出来了：

```go
assert.Equal(t, 0, strings.Count(CurrentQuery(s), "?"))  // 期望 0：换任何不出现的字符都恒真 ⇒ 存活
assert.Equal(t, 1, strings.Count(AsOfQuery(s),   "?"))  // 期望 1：换字符即 0≠1        ⇒ KILLED
```

dev 变异的是前者，我最初变异的是后者。**`assert.Equal(t, 0, Count(x, c))` 这种形态在字符变异下结构性近乎恒真**——这是个值得推广的观察：**断言「某物出现 0 次」天然比断言「出现 n 次」弱得多**，因为前者对「找错了东西」免疫。

### 5.4 ⚠ error_handling[1] 的机制化只覆盖 alias，**不覆盖 dev 自己举的那个场景** —— 严重度：中（架构声明）
**P2 实测**：给 `CurrentQuery` 末尾加一个不过 Spec 的字面量标识符 `ORDER BY rowid` ⇒ **91/91 全绿**，自证三条齐备（diff 非空 / vet=0 / PASS=91 与基线一致）。

**而 dev 机制化 alias 的理由原话正是**：「逐个核对是一次性人工动作，**下次有人加个 ORDER BY 列就失效了**」。
⇒ **它举的那个例子，恰恰是新机制不覆盖的。**

**这不构成 criteria 失败**：`error_handling[1]` 的判据是「**逐个核对当前** query.go 里每个标识符来源 + 确认 alias 是包内常量或过校验」，dev 两条都做了（discovery 里有完整的 `identifier_source_audit`），还额外机制化了 alias。判据没要求防未来改动。

**但包的架构声明「注入面为零」目前没有抗回归的机制守护**：新增一个不过 Spec 的标识符，91 个测试无一会红。**建议转给 TASK-005/006**——这正是我此前给 TASK-005 列的盯点「**封装是否真的无旁路**」在本包的第一个实例。可行的落点（供 Leader 裁量，非我决定）：一条静态检查测试，断言 `query.go`/`lookup.go` 中进入 SQL 格式串的标识符只能来自 `spec.*` 或已过 identRE 的包级常量。

## 6. code-simplifier 的三处改动：语义等价已核实

改动内容（dev 记录）：①② 两个空表测试改用 `currentRows` + `assert.Empty`；③ `countQuestionMarks` 换成 `strings.Count`。
git 只有单个 commit，取不到改前版本，故用**属性等价 + 反向变异**两条路核：

- **③ 等价性**：重建最合理的 `countQuestionMarks`（逐字符计数 `'?'`），与 `strings.Count(s,"?")` 在 **14 个用例**上逐一比对——含两条真实 SQL 输出与对抗串（空串、`"?"`、`"??"`、`"???"`、首尾 `?`、中文夹 `?`、100 个连续 `?`、`"a?"×50`）——**全部相等**。
- **①② 守护保全**（反向变异）：
  - **R1** 让查询报错（`SELECT *`→`SELECT nosuchcol`）⇒ `TestCurrentQueryOnEmptyTable` 与 `TestAsOfQueryOnEmptyTable` **均红**（`no such column: nosuchcol`）⇒ 「空表应返回零行**而非报错**」这一半仍守得住。
  - **R2** 让空表返回非零行（改成无 `GROUP BY` 的聚合）⇒ 空表测试**红** ⇒ 「返回零行」这一半仍守得住。
  - 附带：`currentRows` 内是 `require.NoError(db.Query)` + `require.NoError(rows.Err())`，**错误检测面不窄于手写版本**。

**结论：三处改动语义等价，未削弱守护。** dev 按纪律全量复跑变异矩阵并为新路径补 M11b（我复现 KILLED，80/91）——处置正确。

**关于「code-simplifier 第三次报告不实」**：本次我无法独立复现「它报无改动而实际改了三处」这一事件本身（子代理会话不在我可及范围），**这一条我采信 dev 的记录**，如实标注为**推断而非实测**。我实测的是**改动的结果**——三处语义等价、守护未削弱。

## 7. 三处诚实记录的裁定

### ① 空表测试的独立判别力有限 → **是该 criteria 本身的限度，不是缺陷**
**实测证实**：M13（去掉整个 `WHERE`）红 8 个子测试 / 4 个顶层函数（`TestCurrentQueryReturnsLatestRevision`、`TestCurrentQueryIgnoresInsertOrder`、`TestQueriesWorkWithSingleColumnKey`、`TestZeroSpecProducesSyntaxErrorAtExecution`），**`TestCurrentQueryOnEmptyTable` 确实没红**。

**裁定理由**：`boundary[0]` 的原文是「**空表 → CurrentQuery 返回零行**」。空表在任何单表查询语义下都返回零行——这不是测试写弱了，而是**该需求本身的信息量就这么大**。要它捕获语义错误等于要求它做另一件事。它守的是「不报错」，而 R1/R2 证明这一半**确实守得住**。
⇒ **PASS。** dev 主动标注「不应被计入语义守护」是对的，这种自我设限的记录**提高**而非降低可信度。

### ② oracle 变异 vs 被测代码变异的区分 → **正确**
**实测**：M12（`assert.Empty`→`assert.NotNil`）**存活**，91/91，自证三条齐备。`currentRows` 恒返回非 nil map，故 `NotNil` 恒真。

**裁定理由**：变异断言本身，检验的是「这条断言有没有冗余备份」，而**唯一断言没有备份是定义使然，不是守护缺口**。守护缺口的定义是「**被测代码**出错而无人发现」。dev 用变异被测代码的 **M11b** 取代（我复现 KILLED，11 红），**方法论修正是对的**。
⇒ **区分成立。** 补充一句限定（见 §5.3）：dev 那条具体的 M11 之所以存活，还有个更浅的原因——它打的是期望值为 0 的断言。

### ③ alias 提为常量 + 过 identRE 是否真把一次性核查变成持续约束 → **部分成立**
| 范围 | 探针 | 结果 | 裁定 |
|---|---|---|---|
| **alias 本身** | **P1**：alias 换成 sqlite 合法、identRE 拒收的带引号标识符 | **90 PASS / 1 红，唯一红 = `TestQueryAliasPassesIdentifierGate`** | ✅ **真持续约束**——行为测试全绿（SQL 仍合法），**只有这道闸门抓得住**，它不是冗余装饰 |
| **一般情形** | **P2**：新增不过 Spec 的 `ORDER BY rowid` | **91/91 全绿** | ❌ **不覆盖**——而这恰是 dev 举的例子 |

**P1 是关键的一步**：如果只跑 M7（alias→`"1o"`），SQL 整体语法失败、24 个测试连带红，**根本看不出闸门是不是多余的**。P1 构造了一个「SQL 仍然合法、只有 identRE 不认」的值，才把闸门的**独立判别力**分离出来。
⇒ **③ 部分成立**：对 alias 是真约束（有独立判别力的证据），对一般情形不是。dev 的落点有效，**理由的措辞越界了**。

## 8. 判定

**VERIFIED** —— 14 条 done_criteria 全部有意义覆盖；我独立注入 **20 条变异**（13 条 KILLED、3 条经自证三条判定存活、2 条反向变异、2 条用于分离 M11 差异根因），无一采信 dev 自述；覆盖率 100%、`go vet` exit 0、**0 SKIP**；声明范围与实际改动完全一致。

**须 Leader 后续处置（非本任务缺陷）**：
1. **§5.4 转 TASK-005/006** —— 架构声明「注入面为零」缺抗回归机制守护（**严重度：中**）。
2. §5.1 / §5.2 —— 两处「缩小 harness 里的数字被表述为包的属性」，建议在归档时订正表述。
