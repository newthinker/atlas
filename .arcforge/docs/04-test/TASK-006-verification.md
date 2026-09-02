# TASK-006 验证报告 —— completeness 必填集口径感知，`momFields()` 退场

- **验证者**：test-m1c4-a
- **判定对象**：`master @ 3cbc0d9023abb81a2146d905ca6862cf72629d62`（= `verify_baseline.head`）
- **dev commit**：`d38d7619654feb03c1e8b4100e08ebe56af809f7`（parent `d50835b533cb…`）
- **assignment_epoch**：1
- **结论：VERIFIED**

---

## 0. 基线核对

| 项 | verify_baseline | 实测 | 一致 |
|---|---|---|---|
| `head` | `3cbc0d9023ab…` | 同值 | ✅ |
| `discovery_sha256` | `d07d03ae838d…` | 同值 | ✅ |

本任务只改校验判定、不改抽取，DoD 明确不做全语料背对背 ⇒ **只有一类数字，全部测自 merged master `3cbc0d9`**，
与 discovery 的 `anchors.说明` 一致。

`git show --numstat d38d761` 与 dev 报的逐个相同：`required.go 95/29`、`required_test.go 217/20`、
`validate.go 9/6`、`extract_test.go 18/4`、`backfill_load.go 11/7`（合计 350 增 / 66 删，5 文件）。

---

## 1. done_criteria 覆盖矩阵（8 条）

| # | 维度 | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | `caliberFamilies()` **从 `fieldOrder` 派生**、`missingCaliberAware` 按 `fieldOrder` 排序 | §2、§5 M6 | **PASS** |
| F1 | functional | 四条测试存在且通过；🔴 最后一条守**放松的边界** | §3、§5 M6 | **PASS** |
| F2 | functional | `gateCompleteness` 换成一次 `missingCaliberAware` 调用 | §4 | **PASS** |
| B0 | boundary | `want` 来源不动 + **`momFields()` 退场** + mom→ytd 方向可达 + 27 个测试仍绿 | §4、§5 M2 | **PASS** |
| B1 | boundary | `Reason` 格式与三样保留物原样 | §4 | **PASS** |
| E0 | error_handling | 不做背对背；全包全绿；completeness 未被放松波及 | §6 | **PASS** |
| N0 | non_functional | 门禁 + 覆盖率 ≥96.1%，测自 merge 后 master | §6 | **PASS** |
| N1 | non_functional | 交付流程 | commit 锚定 `feat(TASK-006):`；merge 先于 `dev_done` | **PASS** |

---

## 2. F0：探针复算（与 Leader 的数逐个比对）

在隔离树用临时探针（跑完即删，改动复校 0 行）直读包内私有符号：

```
tsfSectionFields = 36            tsfFlowFields = 18
requiredFields(rule@v1        ) =  40   其中 _mom=13
requiredFields(rule@v2        ) =  76   其中 _mom=22
requiredFields(rule-monthly@v1) =  38   其中 _mom=13
requiredFields(rule-monthly@v2) =  74   其中 _mom=22
caliberFamilies 对数 = 22        fieldOrder 总长 = 76
既非 _mom 也非 _ytd 的字段数 = 32（应原样逐个要求）
```

⇒ 与 Leader 独立复算的 `36 / 18 / 40 / 76 / 38 / 74` **逐个一致**，与 dev 报的 `27→36 / 9→18` 等亦吻合。

`caliberFamilies()` 确实**从 `fieldOrder` 派生**（`strings.CutSuffix(f, "_mom")` + `inOrder[stem+"_ytd"]`），
无第二份手写名单。`twin` **双向建**（`twin[p[0]], twin[p[1]] = p[1], p[0]`）。

---

## 3. F1：四条测试

全部存在且通过（`TestCaliberFamiliesPairsEveryMomWithItsYTD` / `…AcceptsEitherSide`（2 子例）/
`…StillReportsRealGaps` / `…LeavesUnpairedFieldsAlone`）。

**排序守卫做成了 `require` 而不是注释**（DoD 指出需求文档把期望数组顺序写反）：

```go
iM2, iDep := slices.Index(fieldOrder, FieldM2), slices.Index(fieldOrder, FieldDepositHouseholdYTD)
require.Lessf(t, iM2, iDep, "用例前提：fieldOrder 里 %s 必须在 %s 之前", ...)
assert.Equal(t, []string{FieldM2, FieldDepositHouseholdYTD}, got, ...)
```

⇒ 期望值是 `[M2, DepositHouseholdYTD]`（M2 在前），与 DoD 的订正一致；且**顺序依据本身是断言**，
不是让读者相信注释——照需求文档抄会红在排序逻辑上，而那里没有错。

---

## 4. F2/B0/B1：改动面核实

**`gateCompleteness` 只换了中间那个 `for` 循环**（`validate.go` diff 全文核对）：

```diff
-	var missing []string
-	for _, f := range req { if _, ok := in.obs.Values[f]; !ok { missing = append(missing, f) } }
+	missing := missingCaliberAware(req, in.obs.Values)
```

**三样保留物原样未动** ✅：`len(req) == 0 → skipped{unknown_extractor}`、`Value: &n`、`firstN(missing, 3)`。
`Reason` 格式 `"missing N: f1,f2,f3"` 未变。变量名沿用 `req`（不是需求文档写的 `want`）。

### 🔴 `momFields()` 退场核实（用定义数，不用裸 grep）

```
func momFields 定义数 = 0   ✅ 已退场
```

⚠️ **我的裸 grep 计数与 Leader 的不同**：Leader 报 3、我得到 **8**（`required_test.go` 5 处 + `required.go` 3 处）。
我把 8 行全部打印核对——**全部是 `//` 注释**，无一处实际调用。两人结论一致，只是计数范围不同。
（附带一提：我自己在打印时把标签硬编码成了「3 处」而数据是 8 行，幸好把行都打了出来——
这正是「硬编码结论盖住数据」那个坑。）

### 🔴 `missingFields()` 复用同一函数（不是又写一份）

```
missingCaliberAware 的定义： required.go:139         ← 唯一一处
missingCaliberAware 的调用： validate.go:515
                             backfill_load.go:640
```

⇒ **一处定义、两处调用**，`backfill_load.go` 确实复用而非照抄第二份判定 ✅。
分叉的表现（「completeness 说齐了、部分覆盖清单说缺 22 个」）不会发生。

### 声明范围

- **越界申报**：`writes` 4→6（`extract_test.go` + `backfill_load.go`），均在 `dev_done` 之前经 `update` 补入 ✅。
- **`validate_test.go` 声明了但一行未改**：`git show --numstat d38d761` 对该文件**命中 0** ✅；
  它在 `writes` 里 ✅；`scope_note` 显式声明「一行未改」✅（我对 discovery 求值确认该声明存在）。
  gateCompleteness 的既有断言实跑全绿（`TestGatesRejectMalformedData`、`TestGatesSkipOnAbsentFields`、
  `TestGateBoundariesAreInclusive`、`TestGatesMatchContractedCheckIDs`）
  ⇒ **「只换算法、不换契约」这个结论成立**，而且它是由「既有测试一行未改而全绿」支撑的，不是自述。

---

## 5. 变异矩阵（我自己在隔离副本上跑）

隔离 worktree `../wt-t006`；每格 `gofmt -e` 语法闸 + 变异 diff 逐字核对 + `setup failed|build failed` 有效性闸；
`cp` 自备份还原。**主工作区 `required.go` sha256 前后一致**（`0a88468d…`），代码目录改动 **0 行**。

**对照组：641 PASS / 0 FAIL**（这同时覆盖 B0 判据 4「TASK-004 期间转红的 27 个测试仍绿」）。

### 🔴 M1：复现「改之前」—— 这是 dev 那条发现的核心证据

把 `tsfSectionFields()` 与 `tsfFlowFields()` 退回**只列 `_ytd`**：

| | 改后（master） | **改之前（M1）** |
|---|---|---|
| `requiredFields(rule@v1)` | 40，含 `tsf_flow_*_mom` **0** | **49**，含 **9** |
| `requiredFields(rule-monthly@v1)` | 38，含 **0** | **47**，含 **9** |
| v1 样本 `pboc-2019-annual.html` 的口径感知缺失 | **0 个** | **9 个** |

那 9 个具体是：
```
tsf_flow_mom  tsf_flow_rmb_loan_mom  tsf_flow_govt_bond_mom  tsf_flow_corp_bond_mom
tsf_flow_fx_loan_mom  tsf_flow_entrust_mom  tsf_flow_trust_mom  tsf_flow_bankaccept_mom
tsf_flow_equity_mom
```

⇒ **dev 的发现完全属实**，且我看到了具体是哪 9 个字段。这一篇（2019 年报）**根本没有社融节**，
而口径感知救不了它——两侧都不在场。M1 下另有 11 条测试转红。

### M2：恢复 `momFields()` 的 `without` —— 验证 dev 关于测试设计的说法

DoD 与 dev 都声称「手工构造 `want` 的那条即使在 `without` 还留着时也全绿，必须用 `requiredFields()` 的真实返回值」。
我把 `requiredFields` 的 v2 分支改成剔除全部 `_mom`（等价于 `without(momFields())`）：

```
--- FAIL: TestMissingCaliberAwareMomToYTDIsReachableOnRealWant   ← 用真实 want 的那条，转红 ✅
--- PASS: TestMissingCaliberAwareAcceptsEitherSide               ← 手工构造的那条，仍绿 ✅
```

⇒ **dev 的说法成立**：手工构造版对这个变异免疫，只有用真实返回值的那条抓得住。

### M3：twin 单向 —— 🔴 我第一次的变异方向选错了

| 变异 | 结果 |
|---|---|
| **M3a**：`twin[p[0]] = p[1]`（只建 **ytd→mom**） | `TestRequiredFieldsMatchGoldenKeySets` 红，但红的是 **②「口径感知之下不该缺任何必填字段」**，**不是交叉** |
| **M3b**：`twin[p[1]] = p[0]`（只建 **mom→ytd**，即 dev 声称的那个方向） | **交叉转红**：「把 golden 的 `_ytd` 全换成 `_mom` 之后，rule@v2 同样不该缺任何必填字段」；而 `TestExtractFieldsScopeMatchesRequiredFields` 的 **①② 四个子例全绿** ✅ |

⇒ M3a 测的不是 dev 声称的那件事。**以 M3b 为准：dev 的说法成立**——只建 mom→ytd 的实现让 ①② 全绿，
是那条交叉把它抓住的。（`…AcceptsEitherSide/want要ytd_values只有mom` 也红，说明交叉不是唯一的守卫，但它**独立地**抓住了。）

### 🔴 M4b/M5/M6：「放松边界」守卫 —— 两个无效变异之后才打中

`TestMissingCaliberAwareLeavesUnpairedFieldsAlone` 我一开始**杀不动**，跑了三个变异才定位到原因：

| 变异 | 结果 | 为什么 |
|---|---|---|
| **M4**：配不上的 `_mom` 硬配给字面量 `"m2"` | 只有 `TestFieldNamesAppearOnlyInFieldsGo` 红 | 我写的字段名字面量被**另一个无关守卫**先拦住，本格作废 |
| **M4b**：改配给 `fieldOrder[0]`（避开字面量守卫） | **全绿** | `fieldOrder[0]` 恰是 `tsf_stock`，而交叉格的 `want` 就是它 ⇒ **自己配自己等于没配** |
| **M5**：给所有不成对字段都配 `fieldOrder[0]` | **全绿** | 第一格的 `values` 是**空 map** ⇒ 任何 twin 都查不到 ⇒ 结构上免疫 |
| **M6**：让 `x` 与 `x_yoy` 也算一族 | **转红** ✅ | 打中了第二格：`want=[tsf_stock]`、`values` 含 `tsf_stock_yoy` ⇒ 被顶替 |

M6 的失败断言正是那条交叉：

```
Messages: 别的字段在场不构成顶替——顶替关系只在同族两列之间
（同时 TestCaliberFamiliesPairsEveryMomWithItsYTD 也红：「每个 _mom 都该有 _ytd 孪生」）
```

⇒ **那条守卫确实守住了「顺手放松掉一大批无关字段」**，DoD functional[1] 的声称成立。

**但记下一个细节**：这条守卫的两格**射程不同**——第一格（`values` 为空 map）对「乱配 twin」这类变异
**结构上免疫**，它实际测的是集合相等与排序；真正守住放松边界的是**第二格交叉**。这不是缺陷，
但若后人删掉第二格而留下第一格，守卫会静默失效。

**方法论上的记录**：「变异全绿」有两种成因——**守卫无效**与**变异打不到**。这次连续两个无效变异
若不追查，就会被记成「守卫失效」这个错误结论。区分它们的办法是读被测测试的**实际输入**
（这里是 `values` 为空、`fieldOrder[0]` 撞名），而不是从变异的意图去推断它是否生效。

---

## 6. E0/N0：门禁与全包

| 项 | 实测（测自 `3cbc0d9`） |
|---|---|
| `gofmt -l internal/hestia cmd/atlas` | 恰为两个既有欠账 ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，退出码 0 ✅ |
| `go test ./internal/hestia/... -count=1` | 退出码 **0**，**641 PASS / 0 FAIL** ✅ |
| 覆盖率 | **96.1%**，未跌破（仍零余量）✅ |
| `go.mod` / `go.sum` | 命中 **0** ✅ |
| 注释任务编号 | 带 `M1c-4 的 TASK-006` ✅ |

**completeness 未被放松波及**（DoD 要求跑一次确认）：`-run TestComplete` 无同名测试，
扩大到 `Complete*` 得 6 条全通过；gate 族 4 条全绿。**没有任何既有断言从 failed 变成 passed**
——若有，全包会出现行为变化，而对照组是 641 PASS / 0 FAIL。

---

## 7. 测试质量评审

- **非空洞**：M1/M2/M3b/M6 四个变异各自杀掉对应守卫，四条新测试均非重言式。
- **用真实返回值而非手工构造**：`TestMissingCaliberAwareMomToYTDIsReachableOnRealWant` 先
  `require.NotEmpty(moms)` 再断言——它在「`without` 还留着」时因**前一条 require** 而红，
  **红的位置指向正确的原因**（want 恒为单侧），不是指向排序或别处。
- **迁移而非删除**：原「必填集 ≡ 键集」等式在 `_mom` 进必填集后结构上不成立，
  拆成「① 键集 ⊆ 必填集 ② 口径感知之下一个都不缺」+ 一条交叉。**② 比原断言更严**
  （漏配一对 `caliberFamilies` 那一对的 `_mom` 就会进 missing，M6 已实证）。
- **既有契约未动**：`validate_test.go` 一行未改而全绿，是「只换算法」最直接的证据。

---

## 8. 观察项

1. 覆盖率 **96.1% 仍是零余量**。
2. 「放松边界」守卫的第一格对乱配 twin 结构上免疫（见 §5），有区分力的是第二格交叉。
3. 裸 `grep momFields` 的计数我与 Leader 不同（8 vs 3），结论一致（全是注释、`func` 定义 0）。

---

## 9. 结论

**8 条 done_criteria 全部 PASS**。两处 DoD 未预见的连锁我都独立复现了：
**①** 把 `tsfSectionFields`/`tsfFlowFields` 退回只列 `_ytd`，v1 样本的口径感知缺失由 0 变成
**9 个具体的 `tsf_flow_*_mom`**（且那一篇根本没有社融节）；
**②** `missingCaliberAware` **一处定义、两处调用**，`backfill_load.go` 确实复用而非另写一份。

dev 关于测试设计的两条说法（真实 `want` 才抓得住 mom→ytd 方向、交叉断言堵单向 twin）
经 M2 与 M3b **均确证**；其中 M3b 是在我第一次选错变异方向之后重做的。

**判定：VERIFIED**
