# TASK-006 验证报告 — extractFields 按 extractor 决定板块适用性

- **验证者**：test-m1c3a-v2
- **判定**：**VERIFIED**（8/8 条 done_criteria 通过）
- **判定对象**：`verify_baseline.head` = `b4d0c9df250ed15104e0fcb3d0b3b3a8ffedb7ee`
  （dev 分支交付 commit `b04c40ce439481d46fc92db8cd76c3f5f55c8851`）
- **HEAD 收尾时未漂移**；`discoveries/TASK-006.json` sha256 `a76165c8…` 与
  `verify_baseline.discovery_sha256` **逐字相同** ⇒ dev 在 `verifying` 窗口内未改 discovery
  （它来信说明了为什么不改：改了会让我的判定原料漂移。这个判断是对的）。
- **交付物指纹**：隔离副本三文件 sha256 与主工作区逐字相同
  （`extract.go e3eb29dd…` / `extract_test.go 15bdd159…` / `sections.go 4944bf78…`）。

---

## 0. 我自己重采的数字

| 指标 | 我实测 @ `b4d0c9d`（判定对象） | dev 报（@ 其分支 `b04c40c`） |
|---|---|---|
| `go test -count=1` | rc=0，**0 FAIL** | ok，0 FAIL |
| 覆盖率 | **95.7007%（1714/1791）** | 95.6400%（1711/1789） |
| RUN 计数 | **1086 = 523 顶层 + 551 一级子 + 12 二级子** | 1059 = 516 + 531 + 12 |
| `gofmt -l` / `go vet` | 空 / 空 | 空 / 空 |
| numstat | +369/−20（3 文件） | 同 |

**覆盖率的三个值都对**，dev 在 discovery 的 `coverage_on_merged_master` 里**预先**写明了这一点
与成因（TASK-005 在它 merge 前 9 分钟合入，+2 语句，`1789+2=1791`）。
我实测 master 上正是 **1714/1791**，与它预告的第三个值**逐字一致**。
⇒ 它主动把「验证者会用的锚」和「我报数的锚」的差异连同成因一起写下来，是对的做法：
否则这个差异最省事的解释会是「dev 报错了」。

### ⚠️ RUN 计数的口径：**分层数不能用「名字里几个 `/`」去数**

我第一次用斜杠数分桶得 `523 + 521 + 38 + 4`，**自洽校验通过**（求和 = 1086）却是**错的**：
那 4 条「三级子测试」实为测试名自带斜杠
（`TestScanBackfillPageDetectsTagCaseChange/第_0_行的_<a>/</a>_变大写_⇒_报错`）。

改用**缩进口径**（Go 的 `--- PASS:` 行用缩进表示层级）复算：`523 + 551 + 12 = 1086`，与 RUN 总数一致。
两口径的**顶层数都是 523**（顶层测试名是 Go 标识符，不含 `/`），可靠；**子测试的分层数只有缩进口径可靠**。

⇒ 记在这里是因为 dev 报的 `531 + 12` 与我的 `551 + 12` 差异里，**可能同时混着「树不同」与「口径不同」两个成因**。
**总数与顶层数是可比的**，子测试分层数不建议跨报告直接相减。
（自洽校验守的是「有没有漏桶」，**不守「桶的定义对不对」**——它这次通过了，而分层语义是错的。）

---

## 1. done_criteria 覆盖矩阵（8 条）

| # | 完成标准 | 对应测试 | 我实际跑的证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 适用性收敛成**一个**函数，`extractFields` 调用它，**不在循环体堆并列 if** | `TestSectionAppliesToIsTheSingleSourceOfScope` | 读代码：`sectionRule.appliesTo` 是唯一定义，循环体只剩 `if !rule.appliesTo(extractor) { continue }`。我的探针打出完整 **appliesTo 矩阵**（7 板块 × 4 extractor） | PASS |
| functional[1] | 适用规则按 description 的表：社融两节仅 v2 版式；外汇节仅非月报族；核心四节全适用 | 同上 | 探针矩阵逐格核对：社融两节 `rule@v1=false rule@v2=true monthly@v1=false monthly@v2=true`；外汇节 `v1=true v2=true monthly@v1=false monthly@v2=false`；核心四节全 `true` ⇒ **与 DoD 的表逐格一致** | PASS |
| functional[2] | 接受 4 种走板块路径的值，拒绝 `tsf-stock@v1`/`tsf-flow@v1` 与未知值；错误信息由白名单拼出 | `TestExtractFieldsRejectsNonSectionPathExtractors` | 探针实打：4 种接受；`tsf-stock@v1`/`tsf-flow@v1`/`llm-fallback@v1` ⇒「合法但走错路」；`rule@v3`/`""` ⇒「值本身认不出」。消融 **R-M5**（白名单失效）⇒ KILLED，锚定单跑 RUN=5、失败行 `:1197` | PASS |
| boundary[0] | 🔴 板块归属与 `requiredFields` **双向相等**，对四种 extractor 各断言一次 | `TestExtractFieldsScopeMatchesRequiredFields` | **我的独立探针自己算差集**（不复用 dev 的 `ElementsMatch`）：四个 extractor 的 `产出\必填` 与 `必填\产出` **全部为空**，数量 27/54/25/52 ⇒ 双向相等成立 | PASS |
| boundary[1] | 🔴 `rule-monthly@v1` 下外汇节是**声明式跳过**；**必须用含外汇节的输入** | 同上 + `sectionPathSample` 前置断言 | 探针用的是 **8 节 v2 年报（含外汇节）**：月报族产出里 `fx_reserve=false fx_rate=false` ⇒ 确是声明式跳过。**我的新变异 N1**（从样本删掉外汇节）⇒ KILLED，失败行 `:1008` 正是 `sectionPathSample` 的前置断言 ⇒ **DoD 点名的假绿陷阱确实被堵住** | PASS |
| boundary[2] | `rule-monthly@v2` 下社融两节适用、外汇节仍跳过 ⇒ 两维独立 | 同上 | 探针：`rule-monthly@v2` 产出含 `tsf_stock=true` **且** `fx_reserve=false` ⇒ 两个维度确实独立，不是耦合成一个开关 | PASS |
| error_handling[0] | 适用板块缺失仍报错；与「不适用板块缺失放行」**成对** | `TestExtractFieldsSkipsVsMissesSections`（三个子测试） | 消融 **R-M6**（缺失一律放行）⇒ KILLED，**锚定单跑 PASS 列表里有 `不适用板块缺失_放行`** ⇒ **定点失败**；失败行 `:1125`/`:1135` 正是那两条 `require.Error` | PASS |
| non_functional[0] | gofmt/vet/test 绿、覆盖率 ≥95.5%；无新增依赖；milestone 前缀；🟡 补两行否定式断言 | — | 95.7007% ≥ 95.5%；`go.mod`/`go.sum` 未出现；改动 3 文件与 `writes` `diff` 逐条一致；本次 diff 新增的 6 处 `TASK-006` 全带 `M1c-3a` 前缀；`sections.go` 经逐行过滤确认**全部是注释行、无一行代码改动**；🟡 那两行（`RMBLoan`/`Trust`）**已落地** | PASS |

---

## 2. 🔴 dev 来信更正的独立裁决：M5/M6 的 `VOID` 标签

dev-m1c3a-b 在我判定前来信，说 discovery 里 `M5 → 🔴 VOID` / `M6 → 🔴 VOID` 两个标签是错的，
正确记法是「10 格全部有效」。⚠️ **该更正方向对它有利（低估自己的覆盖），我一律自己跑，不采信。**

### 我的复跑结果

| | 全套 | 锚定单跑 | FAIL | **PASS** | 失败行 |
|---|---|---|---|---|---|
| **R-M6** | KILLED | RUN=4（期望 4） | `适用板块缺失_报错`、`外汇节缺失_在累计期仍报错` | **`不适用板块缺失_放行`** | `:1125` `:1135` |
| **R-M5** | KILLED | RUN=5（期望 5） | 全部 4 个子测试 | 空 | `:1197` |

### 裁决：**dev 的更正成立，两个 `VOID` 标签确实是错的**

判据是「**本格声称要证的性质，是否落在失败断言的下游**」：

- **M6**：要证的性质是 error_handling[0]「适用板块缺失仍报错，『跳过』不能退化成『所有缺失都放行』」。
  M6 变异**正是**「所有缺失都放行」，而红的 `:1125`/`:1135` 两条 `require.Error`
  **就是那个性质本身**，不是它的下游。⇒ 有效击杀。
  被遮蔽的是 `require` 后面的 `assert.Contains(err, "人民币贷款")` 等——那是**另一条**性质
  （错误信息指名板块），由 M6b 覆盖。
- **M5**：要证的性质是 functional[2]「拒绝非板块路径的 extractor」。红的 `:1197 require.Error`
  就是该性质本身。被遮蔽的「错误信息含白名单」由 M5b/M5c 覆盖。

⇒ 正确记法是 **10 格全部有效**；discovery 里紧跟标签的散文（「只证明了『必须报错』这一半」）
**现在读仍然准确**——错的只是 `VOID` 这个词。M5b/M5c/M6b **不是** M5/M6 的替代品，
它们证的是另外几条断言，四组合起来才完整。

**我不改 discovery**：它的 sha256 是我的判定原料，改了会逼我走 `--ack-discovery-drift`。
订正落在本报告里是正确位置——这是「验证者对 dev 自述的核实结论」，本就该由我写。

### dev 提的「代理 1 有个边界」⇒ 我独立确认成立

M5 下锚定单跑 **PASS 列表为空、4 个子测试全红**，按代理 1 的字面读法该判「连锁失败、可疑」。
但那 4 格**不是 4 个性质，是同一性质的 4 个输入**（`tsf-stock@v1`/`tsf-flow@v1`/`rule@v3`/`""` 都该报错）。

**真正的定点证据在表外**，而且我不需要额外跑就能确认：
该测试末尾有一个反向对照循环（`extract_test.go:1209-1213`，四个白名单值 `assert.NoErrorf`，
**不是子测试**所以不出现在 `--- PASS:` 行里）。
🔴 **我的失败行清单是 `[1197]`，其中没有 1209-1213 任何一行** ⇒ 反向对照**仍然通过**
⇒ 失败精确落在该报错的 4 格，**没有外溢**。

⇒ 代理 1 需要一句限定：**「其它子测试仍 PASS」里的「其它」必须是**其它性质**的子测试**；
同一性质的多输入全红是预期，此时定点证据要到**表外的反向对照**去找
（而反向对照若不是子测试，就要靠**失败行清单里有没有它**来读，不是靠 PASS 列表）。

---

## 3. 🔴 我的新变异 N2：**SURVIVED —— 一条 dev 明确声称的设计价值零守卫**

**变异**：把 `case slices.Contains(validExtractors, extractor):` 改成恒 false
⇒「合法但走错路」这条分支永不进入，`tsf-stock@v1` / `tsf-flow@v1` / `llm-fallback@v1`
全部落进 `default`（「值本身认不出」）分支。

**结果：全套 rc=0，零条红。**

先问「这个变异真的破坏了 DoD 要的性质吗」——**没有**：

- functional[2] 要的三件事（接受 4 种 / 拒绝其余 / 错误信息由白名单拼出）**变异后全部仍然成立**，
  只是拒绝的理由文案换了一条。
- 所以 **N2 不构成 DoD 违反，这是我判 PASS 的依据。**

**它破坏的是 dev 自己的一个 decision**（discovery `key_findings[5]` / `decisions[5]`）：

> 「合法但走错路」与「值本身认不出」**排障方向完全不同**：前者该去查路由，后者该去查取值域。
> 合并成一条会把两边都指错方向。

**这条设计价值目前没有任何断言在守。** 现有的
`TestExtractFieldsRejectsNonSectionPathExtractors` 断言的是
① 错误信息回显收到的 extractor ② 含全部 4 个 `sectionPathExtractors`
——**两条分支的错误信息都满足这两条**，因此分不出来。

**具体后果**（不是理论风险）：`llm-fallback@v1` 是**已知的合法值**，变异后它被报成
`unknown extractor` —— 正是 dev 想避免的「把人指错方向」。

**建议（给 Leader，不阻断本次验收）**：若「两种拒绝分开报」是要保住的契约，
补一条**交叉断言**即可（我在 TASK-004 验证里用过同一手法）：
断言 `tsf-stock@v1` 的错误信息含 `valid but does not take the section path` 且**不含**
`unknown extractor`，反之亦然。
是否值得补由你定——**DoD 没有要求它**，所以这是「要不要把设计意图升级成契约」的决定，不是缺陷修复。

⇒ 归类：本文件式的「**意图写进了注释/decision，从未变成守卫**」。
与我在 TASK-004 报的「理由③射程窄」同族，但更彻底：那次断言存在且有鉴别力（只是射程窄），
这次是**完全没有断言**。

---

## 4. 消融证据汇总（4 个，harness 独立实现）

**方法**：`git archive b4d0c9df250ed15104e0fcb3d0b3b3a8ffedb7ee | tar -x` 到 `mktemp -d`
（判定对象锚，**钉全 sha**，可覆写 `ARCFORGE_MUT_REF`）。harness 我独立实现，
**未复用** dev 的 `ablate-TASK-006-m1c3a-b.py`。每个变异：逐字替换（锚点出现次数必须恰为 1）
→ 打印 diff 逐字核对 → `go build` 语法闸 → 全套 `-v` → 锚定单跑并**分别列出 FAIL / PASS 子测试 + 核 RUN 行数**。

| ID | 变异 | 结果 | 关键证据 |
|---|---|---|---|
| **R-M5** | 白名单校验失效 | KILLED | 失败行 `:1197`；表外反向对照（`:1209-1213`）**不在失败行清单里** ⇒ 无外溢 |
| **R-M6** | 适用板块缺失一律放行 | KILLED | **PASS: `不适用板块缺失_放行`** ⇒ 定点；失败行 `:1125` `:1135` |
| **N1** | 从 `sectionPathSample` 删掉外汇节 | KILLED | 失败行 `:1008` = 前置断言 `require.Truef` ⇒ 假绿陷阱被堵住 |
| **N2** | 两种拒绝合并成一条 | **SURVIVED** | 见 §3——不违反 DoD，但一条 decision 零守卫 |

⚠️ **N1 的锚定单跑 RUN 行数是 1（期望 5）**，这**不是**「`-run` 没匹配到」的假绿：
`sectionPathSample` 是 `t.Helper()`，前置断言 `require` 失败即中止顶层测试，子测试根本没机会 RUN。
rc=1，确实红了。**RUN 行数偏少要看成因，不能只对数字。**

**卫生**：每个变异窗口内 + 收尾各校验主工作区 3 文件 sha256 与 `git status --porcelain`，**全程未变**。
副本已 `rm -rf` 拆除。
⚠️ 收尾我用 `diff` 比两份指纹文件时报了差异，实为**路径写法不同**（绝对 vs 相对），
三个 hash 逐字相同；改成只比 hash 列后确认。记下来是因为**那个假阳看起来完全像真的**。

---

## 5. dev 自述的核实

| 自述 | 我的核实 | 结论 |
|---|---|---|
| M5/M6 的 `VOID` 标签是错的，10 格全有效 | 我独立复跑两格，看 FAIL/PASS 子测试与失败行 | **成立**（见 §2） |
| 代理 1 需要限定（同一性质的多输入全红是预期） | 我从失败行清单确认表外反向对照未红 | **成立** |
| 覆盖率三个值都对，master 上会得 95.7007% | 我实测 master 得 **1714/1791 = 95.7007%**，逐字一致 | 属实 |
| `sections.go` 只改注释、守卫代码一行未动 | 逐行过滤非注释改动 ⇒ **零行** | 属实 |
| 🟡 那条处方（补 `RMBLoan`/`Trust` 两行）已落地 | diff 里可见，且 dev 的 M7 消融证明它是活的 | 属实 |
| `sectionPathSample` 前置断言堵住 DoD 点名的假绿 | 我的 N1 实测：删掉外汇节 ⇒ 前置断言红 | 属实 |
| `discovery` 未在 `verifying` 窗口内改动 | sha256 与 `verify_baseline` 逐字相同 | 属实 |

---

## 6. 结论

**VERIFIED。** 8 条 done_criteria 全部有对应测试、断言非空洞，且经消融证明在守
（`:1197`/`:1125`/`:1135`/`:1008` 各自被点亮，且我对每个 KILLED 都区分了「定点失败」与「连锁失败」）。
boundary[0] 的双向相等、boundary[1] 的声明式跳过、boundary[2] 的两维独立，
我都用**自己算的探针**而非 dev 的断言复核过一遍。

**dev 来信的 M5/M6 更正成立**，正确记法是 10 格全部有效；订正记在本报告，不改 discovery。

**一条观察（N2 SURVIVED）**：「两种拒绝分开报」这条 decision 目前零守卫。
不违反 DoD，是否升级成契约请 Leader 定。
