# TASK-005 验证报告 —— Save 的输入校验与 INSERT 路径

**判定：VERIFIED**

| 项 | 值 |
|---|---|
| 验证对象 | `39aa8af0bff0d9c46b8a1a77e633aa1a1eb0c7fb`（全 sha，与 `verify_baseline.head` 逐字相同） |
| 分支 | `feat/hestia-store` |
| 验证者 | test-agent-21（承接时 `assignment_epoch` = 1） |
| 隔离 | `git worktree add --detach ../wt-verify-TASK-005 39aa8af…`，收尾 `git worktree remove` |
| 被验文件 sha256 | store.go `abb87f08…5e95` / store_test.go `0dd5ee45…caa0a` |
| discovery 漂移 | 无。实测 `1f564fe3…be31` == `verify_baseline.discovery_sha256` |
| 基线 | 开工 **53 顶层 PASS / 0 FAIL / 0 SKIP**（65 含子测试）；收尾同；变异全还原后 `git status --porcelain` 空 |
| build / vet / gofmt | 全 exit 0；`gofmt -l` 输出为空 |
| 回归 | `./internal/hestia/`、`./internal/macro/bitemporal/` 均 ok |
| 覆盖率 | 91.2%（**不作充分性证据**，判定锚变异；且见 §3——未覆盖块正是关键证据） |

---

## 1. 完成标准覆盖矩阵（12 条全覆盖）

| # | 标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | Lookup→Classify 三路径；Revision **两行都保留** | `TestSaveNewObservation` / `TestSaveRevisionKeepsBothRows` / `TestSaveOutOfOrder` | **PASS**（W11 删旧行 → KILLED） |
| functional[1] | 三处同序契约**第三端**（insert 取值顺序） | `TestSaveMetaValuesLandInMatchingColumns` | **PASS**（W1 → KILLED，见 §4） |
| functional[2] | 按 fieldOrder 拼列而非 map；**判据须可机械判定** | `TestInsertSQLColumnOrderIsDeterministic` | **PASS**（已抽出 `insertSQL(obs) (string, []any)`，**无须降级为 `verify_by: review`**；W10 → KILLED） |
| boundary[0] | 部分字段：其余列 NULL 而非 0 | `TestSavePartialFieldsLeavesNulls` / `TestInsertSQLOmitsAbsentFields` | **PASS**（W7 → KILLED） |
| boundary[1] | 空 Values 与全 54 字段两极端 | `TestSaveEmptyValues` / `TestSaveAllFields` | **PASS** |
| error_handling[0] | 未知键报错且**零行** | `TestSaveRejectsUnknownField` | **PASS**（W5 → KILLED，见 §5） |
| error_handling[1] | Meta 非法报错且零行 | `TestSaveRejectsBadMeta` | **PASS** |
| error_handling[2] | `IngestedAt` 由 `s.now()` 覆写 | `TestSaveOverwritesIngestedAt` | **PASS**（W6 → KILLED） |
| error_handling[3] | G10 拒绝自相矛盾报告 | `TestSaveRejectsSelfContradictoryReport` | **PASS**（W9 → KILLED） |
| error_handling[4] | G6 非有限值报错且零行 | `TestSaveRejectsNonFiniteValues` | **PASS**（W8 → 精准只红 ±Inf 子测试） |
| non_functional[0] | `savePending` 是有意的桩，**不得据此 REJECT** | — | **已遵守**，见 §3 |
| non_functional[1] | 写口守卫扩到**包导出面**，精确集合相等 | `TestStoreExposesNoWriteMethods` + `TestPackageExposesNoWriteFunctions` | **PASS**，互补性**双向实证**见 §6 |

`assertNoRowsAnywhere` 同时查观测表与 pending 表，四条错误路径共用——DoD 要求的「不只断言 error 非 nil」已落实。

---

## 2. 变异实证（本人独立执行，11 个有效变异 + 2 个 harness 自证）

**10 KILLED / 1 SURVIVED。** 标【新】者为 dev 未覆盖的方向。

| 变异 | 结果 | 红的是 | 顶层 PASS |
|---|---|---|---|
| W1 insert 取值顺序互换（CaliberVersion↔Extractor，metaColumns/Meta 不动） | KILLED | `TestSaveMetaValuesLandInMatchingColumns` | 52/53 |
| W2 包级导出写口 `InsertRow` | KILLED | `TestPackageExposesNoWriteFunctions` | 52/53 |
| **W3【新】** 嵌入不导出类型 `storeHelper` 的导出方法 `Bulk` | KILLED | `TestStoreExposesNoWriteMethods` | 52/53 |
| **W4** pending 路径 `Verdict` 置零值（= M7' 等效） | **SURVIVED** | — | 53/53 |
| **W5【新】** `checkValues` 移到 `insert` 之后（先写再报错） | KILLED | RejectsUnknownField + RejectsNonFiniteValues | 51/53 |
| **W6【新】** `IngestedAt` 仅在为空时覆写 | KILLED | MetaValuesLandInMatchingColumns + OverwritesIngestedAt | 51/53 |
| **W7【新】** 缺失字段写 0 而非省略 | KILLED | InsertSQLOmitsAbsentFields + SavePartialFieldsLeavesNulls | 51/53 |
| **W8【新】** 非有限值只查 NaN 不查 Inf | KILLED | 只红 `±Inf` 两个子测试，**NaN 子测试保持绿** | 52/53 |
| **W9【新】** G10 只查第一个 Check | KILLED | `failed_check_among_passed` 子测试 | 52/53 |
| W10 `insertSQL` 按 map 迭代 | KILLED | `TestInsertSQLColumnOrderIsDeterministic` | 52/53 |
| **W11【新】** 修订时先 DELETE 旧行 | KILLED | RevisionKeepsBothRows + SaveEmptyValues | 51/53 |

**逐条查过因果（含 KILLED）**：W5/W6/W7/W11 各红 2 条，均为直接因果，属合法多杀。最值得说明的是 W11 的第二条——`TestSaveEmptyValues` 第二次 `Save` 用同一期、更晚的 `published_at`（2026-08-20），因此也被分类为 Revision，删旧行同样破坏它。不是连带伤害。

W8 是精度最高的一个：去掉 `IsInf` 检查后，**只有 ±Inf 两个子测试红，NaN 子测试保持绿**，说明子测试划分与被测条件一一对应。

---

## 3. 【leader 指定复核】M7' 的「结构性不可测」论证——**成立**，且我有比口头论证更硬的证据

我独立复现了该存活（W4），**四条自证齐全**：diff 非空 / 文件首行完好 / `go vet` exit 0 / 顶层 PASS = 53 = 基线，0 FAIL。

在此之上，`go test -coverprofile` 直读给出机制化解释：

```
未覆盖块: store.go 行 180 → 180      (count = 0)
```

行 180 正是 `return Outcome{Verdict: verdict, Table: TablePending}, nil` —— **M7' 唯一能影响的那一行**。它不可达的原因在 L302–304：`savePending` 是永远返回 error 的桩，故 L177–179 的 `if err != nil { return Outcome{}, err }` 恒成立，L180 是死代码。

⇒ **「pending 路径的 Verdict 是否如实反映分类结果」在本任务范围内不存在观测点。** 这不是「测试写得不够」，是结构性的。dev 的论证成立。

它不强行闭合的两条理由我也认同：给桩注入假实现等于把 TASK-006 的设计提前定死；给 `Save` 加「出错也返回部分 Outcome」的语义是为测试改生产行为。两者都比一条明确交接更差。

`non_functional[0]` 明写「不得据此判 REJECT」，且规格已进 TASK-006 `functional[0]`（**构造 Revision 场景断言 `Verdict == bitemporal.Revision`——只测 New 等于没测**）。**一个 DoD 被部分满足而声明清楚，与被声称完全满足，是两回事**；此处属前者。

---

## 4. 【leader 指定复核】三处同序契约第三端——**已守住**

W1 只改 `insertSQL` 的取值顺序（`metaColumns` 与 `Meta` 都不动）→ **只红 `TestSaveMetaValuesLandInMatchingColumns`**。

关键在于**没红的那两条**：TASK-002 的 `TestMetaFieldOrderIsCrossTaskContract` 与 TASK-003 的 `TestMetaColumnsMatchMetaStructByReflect` 全绿——正是 DoD 预言的「两端都对、第三端单方面错位」那一类。三端至此全部有机制化保护。

**fixture 强度自证已核实**（store_test.go L466–472）：断言七个 Meta 值互不相同，否则「任意两列互换必红」不成立。期望值只由 `want`（Meta 侧）经 reflect 按声明顺序构造，不从 `got` 侧构造——避开了「期望值跟着实现变」的恒真陷阱。

---

## 5. 「零行落盘」判据确实有牙

W5 把 `checkValues` 移到 `insert` 之后，模拟「先写脏数据再报错」——这种实现**在只断言 `error != nil` 的测试下完全隐形**。结果 `TestSaveRejectsUnknownField` 与 `TestSaveRejectsNonFiniteValues` 全部转红，红在 `assertNoRowsAnywhere` 上。DoD 那句「只断言 error 非 nil 不能排除『已写脏数据再报错』」得到实证。

---

## 6. 【leader 指定复核】两条写口守卫的视野互补——**双向实证**

dev 声称二者互补。我对**两个方向**各做一个变异：

| 变异 | `TestPackageExposesNoWriteFunctions`（AST） | `TestStoreExposesNoWriteMethods`（reflect） |
|---|---|---|
| W2 包级函数 `func InsertRow(db *sql.DB, period string) error` | **红** | 绿（盲） |
| **W3 嵌入不导出类型的导出方法**（`type storeHelper struct{}` + `func (storeHelper) Bulk() error`，嵌入 `Store`） | 绿（盲） | **红** |

W3 的盲点成因：AST 侧对有接收者的函数会检查接收者是否导出（`recvTypeName` + `ast.IsExported`），`storeHelper` 不导出故被跳过；而 Go 的方法提升让 `Bulk` 出现在 `*Store` 的方法集里，reflect 看得见。

⇒ **互补性声明成立，删任一条都会重开一个缺口。** dev 只实证了 W2 方向，W3 方向由本次验证补齐。

两条都是**精确集合相等**（`[Close, DB, Save]` 与 `[NewStore, Store.Close, Store.DB, Store.Save]`），未弱化成「包含」，符合 dev-agent-42 的交接要求。

---

## 7. 【leader 指定复核】两条 harness 教训——已复现，并已纳入我的流程

我把 dev 提的第四条自证加进了自己的 harness，并对**我自己的工具**做了两条证伪测试：

| 自证 | 构造 | 我的 harness 判定 |
|---|---|---|
| A（复现 dev 第一版 M3） | `if math.IsNaN(v) \|\| math.IsInf(v,0) {` → `if false {` ⇒ `math` 导入未使用 ⇒ 编译失败 | **INVALID**（`go vet` 非 0），**未误报 SURVIVED** |
| B（复现工具改坏文件） | 把 store.go 首行写成 `if` | **INVALID**（首行 ≠ `package hestia`） |

dev 的论证值得原样保留：

> 现有三条自证都挡不住第二种——diff 确实非空（而且很大）、vet 确实非 0 但**我原本只在存活时才看它**、PASS 计数确实是 0。「变异改对了地方」和「变异改坏了文件」在输出上长得很像。

我据此做了两处调整：①加入文件完整性检查；②**把 `go vet` 从「仅存活时检查」改为红绿都检查**。第二点尤其重要——只在存活时查 vet，等于默认「红了就是测试起作用了」，而编译失败恰恰会让所有测试都不跑、计数塌成 0，看起来像「全红」。这与 TASK-003 里「语法错伪造全红」是同一枚硬币：**任何让被测代码根本没运行的破坏，都会同时伪造全绿或全红两种极端。**

---

## 8. scope 一致性

```
$ git diff --stat 39aa8af~1..39aa8af
 internal/hestia/store.go      | 174 ++++++++++
 internal/hestia/store_test.go | 496 +++++++++++++++++++++++++-
```
与 `writes`（`./internal/hestia/store.go`、`./internal/hestia/store_test.go`）逐项一致，**无越界申报**。`packages` = `./internal/hestia`，与测试包一致。

---

## 9. 对 DoD 的反馈

1. **`functional[2]` 预先写明「若不抽出 SQL 构造就须标 `verify_by: review`」**——这条起了作用：dev 抽出了 `insertSQL`，于是该条保持 `test` 级且 W10 可机械判定。DoD 提前给出「降级路径」反而促成了不降级。
2. **`error_handling` 四条统一要求「零行」而非「error 非 nil」**——W5 实证了这条不是形式主义。
3. **`non_functional[0]` 预先声明桩的存在并写「不得据此判 REJECT」**——避免了一次可预见的误判。建议在后续任务保持这个做法：**把已知的不可测区域写进 DoD，比留给验证者临场判断更可靠。**
4. 唯一可考虑的补强：`error_handling[2]`（IngestedAt 覆写）目前由 W6 实证有效，但该断言与 `TestSaveMetaValuesLandInMatchingColumns` 存在耦合（W6 同时红两条）。这不是缺陷，只是说明两条测试都依赖 `s.now` 注入；若将来重构注入方式，需同时检查两处。
