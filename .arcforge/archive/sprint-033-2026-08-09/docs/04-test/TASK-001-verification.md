# TASK-001 验证报告 —— 字段常量与唯一真相源 (fields.go)

> **本文件覆盖两轮验证，第 2 轮（返工轮）为当前有效判定。**
> 刻意**覆盖同名文件而非另起新文件**：CLAUDE.md 规定验证报告文件名以
> `TASK-{id}-verification.md` 为准，且实测过一次「同内容两份文件」的代价——写通道只能
> 创建/覆盖不能删除、write-guard 又禁止 `rm`，最后靠人类在会话外清理。覆盖同时刷新 mtime，
> 满足「本轮产物不早于本轮进入该状态时刻」的活性判据。第 1 轮结论完整保留在 §R1。

---

# 第 2 轮（返工轮）—— 当前有效判定

- **验证者**：test-agent-20
- **判定**：**VERIFIED（通过）**
- **被验 commit**：`878660e4f6514da25ed3e787ec5666adb30b4265`（前身 `5dcc802`）
- **verify_baseline.head**：`878660e4f6514da25ed3e787ec5666adb30b4265` —— **一致，无漂移**
- **verify_baseline.discovery_sha256**：`e544b9c2…4039`，出裁决前实算一致
- **assignment_epoch**：1（出裁决携带 `--expect-epoch 1`）；`rework_count`：1
- **验证环境**：隔离 worktree `.worktrees/wt-verify-TASK-001r`（`--detach 878660e…`），验毕已 remove
- **返工范围**：`git show --stat 878660e` = **仅 `internal/hestia/fields_test.go`**（+123 / -67）。
  经 `git diff --stat 5dcc802..878660e -- internal/hestia/fields.go` 复核，**`fields.go` 零改动**，
  与 dev 自述一致。

## 2.1 基线计数 —— 顺带澄清 discovery 里的「18 vs 24」

dev 的 discovery 里两处数字看似矛盾：`verification.result` 写「整包 18 PASS」，
`mutation_evidence.baseline` 写「24 --- PASS」。**实测两者都对，只是单位不同**：

| 口径 | 值 |
|---|---|
| 顶层测试数（`^--- PASS`） | **18**（本任务 8 + TASK-002 的 10） |
| `--- PASS` 行总数（含 6 个子测试） | **24** |
| `--- FAIL` / `--- SKIP` | **0 / 0** |
| coverage | 100.0% of statements |
| `go vet` / `gofmt -l` | exit 0 / 输出为空 |

本报告的变异基线统一取 **24 `--- PASS` 行**。

⚠ 覆盖率仍不构成充分性证据（`fields.go` 除 `allFields` 匿名函数外无可执行语句），dev 已在
discovery 的 `coverage_caveat` 里如实记下这点。判定锚变异证据。

## 2.2 Leader 点名要独立核实的三件事

### ① M11 必须被杀死，且红的应是绑定断言 —— **成立，且 dev 的推理成立**

| 变异 | 计数 | 变红的具体测试 |
|---|---|---|
| **M11**（交换两常量的值 + 交换其在 `fieldOrder` 的位置） | 23P / 1F | **仅 `TestFieldConstantBindings`** |

`TestFieldOrderGoldenList` **仍 PASS**。dev 对这一点的解读是「值序列仍 PASS 本身是证据」，
**该推理成立**：值序列断言把 `wantOrder`（由规格侧 `want` 构造）与 `fieldOrder` 逐元素比对，
它 PASS 即证明 `fieldOrder` 的值序列在该变异下**逐字未变**；既然未变，任何只看值序列的断言
都不可能观察到这条变异 ⇒ 第 1 轮 M11 存活是**结构性不可见**，不是当初谁跑漏了。

我另用**同一条变异跑在第 1 轮的旧测试上**（第 1 轮记录：7 个测试全绿）交叉印证，结论一致。

### ② 「值序列断言必须只由 want 侧构造」的陷阱 —— **代码写对了，且陷阱经实证是真的**

实际代码（`fields_test.go:121-124`）：

```go
wantOrder := make([]string, len(goldenFields))
for i, g := range goldenFields {
    wantOrder[i] = g.want          // ← 用的是 want，不是 got
}
```

**不是只在注释里说说**。我做了一次元变异实证这条纪律不是空话：

| 变异 | `TestFieldOrderGoldenList` 的表现 |
|---|---|
| M1（常量拼写错 `deposit_corp_ytd`→`deposit_corp_yt`）+ **现状 want 侧构造** | **FAIL** ✅ |
| M1 + **把 `wantOrder[i]` 改成 `g.got`**（模拟图省事的写法） | **PASS** ⚠️ 退化为恒真 |

同一条 M1，仅改构造侧，值序列断言就从 FAIL 翻成 PASS —— **A/B 直接证明** got 侧构造会把这条
断言变成自指恒真（拿 `fieldOrder` 和它自己比）。dev 避开了这个陷阱。

（注：got 侧构造的那次整体仍被 `TestFieldConstantBindings` 判红，所以缺陷不会完全逃逸；
但**值序列这一层判据确实被架空**，而它正是 `functional[2]` 的原判据。）

### ③ P2 的捕获上限声明 —— **准确，未夸大也未缩水**

dev 声称 `foldStringLiteral` 折叠纯字面量拼接，并声明「变量拼接 / `fmt.Sprintf` /
`[]byte` 往返都抓不到」。逐条实测：

| 形态 | 结果 | dev 的声明 |
|---|---|---|
| `"deposit_" + "corp_ytd"` | **KILLED** | 声称能抓 ✅ |
| `("deposit_" + ("corp" + "_ytd"))` 嵌套带括号 | **KILLED** | 未声称，**实际比声称的更强**（`ParenExpr` 递归） |
| `pfx + "corp_ytd"`（`pfx` 是变量） | **SURVIVED** | 声称抓不到 ✅ |
| `fmt.Sprintf("deposit_%s", "corp_ytd")` | **SURVIVED** | 声称抓不到 ✅ |
| `string([]byte{0x6d, 0x30})` | **SURVIVED** | 声称抓不到 ✅ |

三条存活均带三条自证（diff 非空 + `go vet` exit 0 + PASS 计数 24 == 基线 24）。
**声明与实际逐条吻合。** dev 那句「把它当完备拦截会是下一个恒真断言」是对的自我限定。

**负向对照（必须不误报）**：非测试文件里写 `return FieldM0`（即我们**要求**的正确写法）
→ 全绿 24P/0F。判据没有把正确用法误判成违规。

## 2.3 done_criteria 覆盖矩阵（返工后重跑全部 7 条，不只是改动项）

`fields_test.go` 被重写了 123/67 行，故**全部判据重新验证**，不假设未受影响。

| # | 完成标准 | 对应测试 | 变异证据（红的是哪一条） | 判定 |
|---|---|---|---|---|
| functional[0] | 分组计数逐组断言 27/6/7/10/4 | `TestFieldGroupCounts` | M9 **仅红这一条**；M10 红这一条+绑定表 | **PASS** |
| functional[1] | `allFields` ≡ `fieldOrder` | `TestAllFieldsMatchesFieldOrder` | M7 **仅红这一条** | **PASS** |
| functional[2] 值序列侧 | golden list 逐字 + 顺序敏感 | `TestFieldOrderGoldenList` | M2 **仅红这一条**；M1 红这一条+绑定表 | **PASS** |
| functional[2] 绑定侧（返工新增） | 常量名↔值绑定 | `TestFieldConstantBindings` | **M11 仅红这一条** | **PASS** |
| boundary[0] | `fieldOrder` 无重复 | `TestFieldOrderHasNoDuplicates` | M5 红这一条 + `allFields` | **PASS** |
| error_handling[0] | `^[a-z][a-z0-9_]*$` | `TestFieldNamesAreValidIdentifiers` | M6 **仅红这一条** | **PASS** |
| non_functional[0] | `fields.go` 外非 `_test.go` 无字段名字面量 | `TestFieldNamesAppearOnlyInFieldsGo` | M3a/b/c/d/p **均仅红这一条**；M3e 豁免正确；M3i 不误报 | **PASS** |
| non_functional[1] | 包注释单位 + breaking change | `TestPackageDocDeclaresUnits` | M8a/b/c **均仅红这一条** | **PASS** |

## 2.4 变异总表（21 个窗口，全部三条自证）

harness 锚点写死全 sha `878660e4f6514da25ed3e787ec5666adb30b4265`，首行打印 worktree HEAD。

**KILLED（14）**：M11、TRAP（元变异）、M1、M2、M5、M6、M7、M9、M10、M3a、M3b、M3c、M3d、M3p、M8a、M8b、M8c
**SURVIVED（3，均为 dev 已声明的捕获上限）**：M3g（变量拼接）、M3h（`fmt.Sprintf`）、M3f（`[]byte` 往返）
**负向对照（必须绿，实际绿）**：M3e（`_test.go` 豁免，基线 25）、M3i（引用 `FieldM0` 常量）

收尾：变异全部还原后 `git status --porcelain -- internal/hestia/` 为空，PASS 计数回到 24。

## 2.5 与包外真相源的四方比对（绑定表格式变了，故重做）

需求文档附录 A：hestia 仓 `docs/superpowers/plans/2026-08-08-hestia-store.md`。

| 比对 | 结果 |
|---|---|
| 绑定表 54 条 `{常量, 字面量}` vs 附录 A 按 `fieldOrder` 序展开的「常量→值」 | **逐条逐序相同** |
| 绑定表左侧常量互异性 / 右侧字面量互异性 | 54 条 / 去重 54 / 去重 54（**无常量被写两次或漏写**） |
| `fields.go` 的 `fieldOrder` 常量序 vs 绑定表左侧常量序 | **完全一致**（这是 `wantOrder` 与 `fieldOrder` 可比的前提） |
| `fields.go` 54 个常量定义 vs 附录 A | **逐条逐序相同**（复核 `fields.go` 确未被返工触碰） |

第 3 项值得单列：绑定表**同时**承担「值序列」与「名值绑定」两个判据，若它的常量序与
`fieldOrder` 不一致，值序列断言会以错误理由红/绿。此处已排除。

## 2.6 越界申报核对

`git show --name-only 878660e` = `internal/hestia/fields_test.go` 单文件，落在 `writes`
声明（`fields.go` / `fields_test.go`）之内。**无越界，无需申报。**

## 2.7 第 1 轮遗留问题的处置结论

| 编号 | 第 1 轮结论 | 本轮状态 |
|---|---|---|
| **P1** 常量名↔值绑定无守卫（M11 存活） | 判为 DoD 缺口，交 Leader 决策 | **已修复并实证**（§2.2①），M11 KILLED |
| **P2** DoD 的 grep 核验命令比其判据严（M3d 存活） | 记录供 QA 参考，非缺陷 | **已收窄**（§2.2③），M3d KILLED；剩余差距仅非字面量形态，已被 dev 显式声明为上限 |

## 2.8 结论

**VERIFIED。** 8 个测试对应 7 条 done_criteria（`functional[2]` 一分为二：值序列 + 名值绑定），
逐条有变异证据，其中 6 条存在「**只红该条对应测试**」的精确变异。Leader 点名的三件事逐条独立
复核：M11 被杀且红的正是绑定断言、`wantOrder` 确由 `want` 侧构造（并用元变异实证该陷阱真实存在）、
捕获上限声明逐条准确。绑定表已与包外附录 A 四方比对通过。dev 自报无夸大，
「18 vs 24」是单位差异而非矛盾。

**本轮无新增问题，无需 Leader 决策事项。**

---

# §R1 第 1 轮（历史记录，判定已被第 2 轮取代）

- 被验 commit：`5dcc8027bfd53cfd45cdef0a39ca1506da122ee5`，判定 **VERIFIED**
- 计数：7 PASS / 0 FAIL / 0 SKIP（当时包内只有 `fields.go` + `fields_test.go`），coverage 100%
- 7 条 done_criteria 全部通过，13 个变异，其中 5 条判据有「只红该条」的精确变异
- **发现 P1（中）**：变异 M11 存活 —— 常量名↔值绑定无任何断言守卫。同时交换
  `FieldDepositCorpYTD`/`FieldDepositFiscalYTD` 的值与其在 `fieldOrder` 的位置后，值序列逐字
  不变、7 测试全绿，而 `FieldDepositCorpYTD == "deposit_fiscal_ytd"`。下游拿它当 `Values` 键
  ⇒ 企业存款静默写进财政存款列。判为 **DoD 缺口而非实现偏离 DoD**，故未 reject。
  → **第 2 轮已修复**。
- **发现 P2（低）**：DoD `non_functional[0]` 给的 grep 核验命令比其判据措辞更严（grep 抓得住
  拼接形态，AST 判据按字面措辞放行）。→ **第 2 轮已收窄**。
- 另确认：`TestFieldNamesAppearOnlyInFieldsGo` 在 `5dcc802` 确为 vacuous（包内无可扫文件），
  但非空断言；实际从 **TASK-002**（`types.go` 入包）起生效，而非原先记的 TASK-003。
