# TASK-003 验证报告 —— 由字段清单生成 DDL (schema.go)

**判定：VERIFIED**

| 项 | 值 |
|---|---|
| 验证对象 | `fb1526071bce20d4929258a24748f04431d4be84`（全 sha，与 `verify_baseline.head` 逐字相同） |
| 分支 | `feat/hestia-store` |
| 验证者 | test-agent-21（承接时 `assignment_epoch` = 1） |
| 隔离方式 | `git worktree add --detach ../wt-verify-TASK-003 fb15260…`，收尾 `git worktree remove` |
| 被验文件 sha256 | schema.go `77d64363…7571` / schema_test.go `04d8238f…707d` |
| discovery 漂移 | 无。实测 sha256 `1705a18b…19d0` == `verify_baseline.discovery_sha256` |
| 基线 | 开工 32 PASS / 0 FAIL / 0 SKIP（26 顶层函数）；收尾同（变异全部还原，`git status --porcelain` 空） |
| go build / vet / gofmt | 全部 exit 0 / `gofmt -l` 输出为空 |
| 回归 | `./internal/hestia/` 与 `./internal/macro/bitemporal/` 均 ok |

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | 业务列逐列由 fieldOrder 派生、按其序排列；列数恰 61 | `TestObservationsColumnsDeriveFromFieldOrder` | **PASS** |
| functional[1] | metaColumns 与 reflect 取自 `Meta` 的字段名（snake_case）逐一相等 | `TestMetaColumnsMatchMetaStructByReflect` | **PASS** |
| functional[2] | 视图基于 bitemporal 当前行语义（published_at 派生，非 is_current 列） | `TestCurrentViewDDLUsesBitemporal` | **PASS** |
| boundary[0] | 三段 DDL 在真实 SQLite 连续执行两次不报错且结果一致 | `TestDDLIsIdempotent` | **PASS** |
| error_handling[0] | DDL 不得出现 `is_current` / `absent_fields` | `TestDDLHasNoDerivableStateColumns` | **PASS**（字符串 + 真实库双查） |
| non_functional[0] | 字段名不在 schema.go 出现字面量（只约束非 `_test.go`） | `TestFieldNamesAppearOnlyInFieldsGo`（fields_test.go） | **PASS** |
| non_functional[1] | DDL 在真实 SQLite 实际执行，不只字符串比对 | 全部结构测试经 `openWithSchema` | **PASS** |
| non_functional[2]① | `PRIMARY KEY (period, period_type, published_at)` 存在且次序固定 | `TestObservationsStructureFromLiveDB` | **PASS** |
| non_functional[2]② | 业务列类型亲和性为 REAL | 同上 | **PASS** |
| non_functional[2]③ | 元数据列 NOT NULL（且为 TEXT） | 同上 | **PASS** |
| non_functional[2]④ | `hestia_pending` 的 PK（带 ingested_at） | `TestPendingStructureFromLiveDB` | **PASS** |
| non_functional[2]⑤ | 视图确按 MAX(published_at) 且关联 period+period_type | `TestCurrentViewStructureFromLiveDB` | **PASS**，但见 §4 O-1 |
| non_functional[2] 附款 | 「NewStore 实际部署的那个视图」 | — | **本任务不可覆盖，已如实声明**，见 §5 |

`metaColumns` 是元数据列名、不属于 `fieldOrder` 业务字段集合，故其字面量不违反 non_functional[0]；已核实该测试用 AST 遍历（含**字面量拼接折叠**）扫描目录下所有非 `_test.go`、非 `fields.go` 的 .go 文件，schema.go 在扫描范围内。

---

## 2. 变异实证（本人独立执行，未复用 dev 数据）

harness 内建三条自证：`git diff --numstat` 非空 / `go vet` exit 0 / PASS 计数对比基线 32。

| 变异 | 结果 | 红的是 | PASS |
|---|---|---|---|
| V1 无 PK 但语法合法（`PRIMARY KEY` 行 → `CHECK (1)`，仍 61 列） | KILLED | `TestObservationsStructureFromLiveDB` | 31/32 |
| V2 metaColumns 单方面换序（caliber_version ↔ extractor，`Meta` 不动） | KILLED | `TestMetaColumnsMatchMetaStructByReflect` | 31/32 |
| V3 业务列 REAL → TEXT | KILLED | `TestObservationsStructureFromLiveDB` | 31/32 |
| **V4【新】** pending 主键去掉 published_at | KILLED | `TestPendingStructureFromLiveDB` | 31/32 |
| **V5【新】** 手写视图主体（丢弃 CurrentQuery，无 MAX、无键关联） | KILLED | `TestCurrentViewStructureFromLiveDB` + `TestCurrentViewDDLUsesBitemporal` | 30/32 |
| **V6【新】** 元数据列 TEXT → REAL | KILLED | `TestObservationsStructureFromLiveDB` | 31/32 |
| **V7【新】** 业务列加 NOT NULL（应可空） | KILLED | `TestObservationsStructureFromLiveDB` | 31/32 |
| **V8【新】** 视图漏 period 键，保留 MAX + period_type | KILLED | `TestCurrentViewDDLUsesBitemporal`**（结构断言那条没红——见 O-1）** | 31/32 |
| V9 业务列倒序生成 | KILLED | `TestObservationsColumnsDeriveFromFieldOrder` | 31/32 |
| V10 去掉 `IF NOT EXISTS` | KILLED | `TestDDLIsIdempotent` | 31/32 |

**10 个有效变异，0 存活。** 其中 5 个（V4–V8）是 dev 未覆盖的方向，测试套件同样抓住。

---

## 3. dev 的方法论声称：独立复现成立

dev 声称「第一版 M1（直接删 PRIMARY KEY 行）作废，因为它造成语法错，6 条红证明的是『语法错会被抓』而非『无 PK 会被抓』」。**我并排跑了两版**：

| 版本 | DDL 是否合法 | 红的条数 | PASS | 红的测试 |
|---|---|---|---|---|
| **V1-bad**（对照，删 PK 留右括号 → 尾逗号语法错） | 否 | **6** | 26/32 | ObservationsColumns / ObservationsStructure / **PendingStructure** / CurrentViewStructure / DDLIsIdempotent / DDLHasNoDerivableStateColumns |
| **V1**（`CHECK (1)` 占位，语法合法、仍 61 列、仅缺 PK） | 是 | **1** | 31/32 | ObservationsStructure |

**结论：dev 作废第一版是对的。** 那 6 条红里 5 条与 PK 无因果——最明显的是 `TestPendingStructureFromLiveDB`：pending 表跟观测表的 PRIMARY KEY 毫无关系，它红只是因为 `openWithSchema` 在执行**第一段** DDL 时 `require.NoError` 就失败了，后面根本没跑到。若只跑 V1-bad 就宣称「PK 有测试保护」，那是被语法错伪造的全红。

dev 给的一般化判据（「一个变异杀死多个测试时，先怀疑它顺带破坏了别的东西」）成立。我据实测把它收紧成一条可操作判据：

> **连带伤害的标志是「红的测试与变异点无因果关系」，不是「红的条数多」。**

V5 就是反例：它红了 2 条，但两条都在视图上、都由该变异直接引起（一条查 `sqlite_master` 里的实际视图定义、一条查返回字符串是否原样含 `CurrentQuery`），属**合法多杀**，不需作废。只看条数会把 V5 也误判成需要重做。

---

## 4. 发现

### O-1（观察，不阻断）：`schema_test.go:223` 是被子串蕴含的恒真断言

```go
assert.Contains(t, ddl, "period")        // ← L223：被下一行蕴含，零保护
assert.Contains(t, ddl, "period_type")   // ← L224
```

任何含 `"period_type"` 的字符串必然含 `"period"`，故 L223 在 L224 通过时恒真。

**实证（V8）**：把视图改成只按 `period_type` 关联、漏掉 `period`（即 DoD ⑤ 明写要防的「少一个键会让当前行跨期混算」），`TestCurrentViewStructureFromLiveDB` 的四条 `Contains` **全部 PASS**、该测试全绿；红的是另一条 `TestCurrentViewDDLUsesBitemporal`（因主体不再原样等于 `CurrentQuery(spec)`）。

即：**缺陷可被检出，但检出者不是名义上负责 ⑤ 的那条断言。** 现有保护来自「视图主体必须原样来自 bitemporal」这条更强约束。风险在于——若日后有人放宽那条（例如允许拼接前缀/后缀），⑤ 的保护会静默消失，而覆盖矩阵上它看起来仍有测试。

不阻断的理由：所有 DoD 条目均有有效覆盖，该冗余断言**不产生假绿**（它只是不产生额外保护），且缺陷实际可被检出。

建议（留给 TASK-004/005 或后续修）：把 L223 换成对独立标识符的断言，例如断言 `ddl` 含 `"o.period ="` 或用词边界正则，使其不被 `period_type` 蕴含。

**这与 TASK-002 `error_handling[0]` 的 period/period_type 陷阱是同一形状的第二次出现**：`period` 是 `period_type` 的前缀，凡用 `Contains`/子串语义断言这两者之一，都会被另一者蕴含。建议在 wisdom 里作为该包的常驻陷阱记录。

---

## 5. 限制声明合规性（leader 指定复核项）

DoD `non_functional[2]` 末句要求覆盖「`NewStore` 实际部署的那个视图从未被验过」。**该条本任务不可覆盖**——`NewStore` 属于 TASK-004，此刻不存在；本任务验的是测试自建库（`openWithSchema`）的结构，用的仍是测试自己造的 spec（`testSpec`），正是那条 DoD 想消除的验法。

**已核实 dev 如实声明**，`discoveries/TASK-003.json` 的 `notes_for_downstream` 首条原文写明该条无法覆盖、原因、后果（"NewStore 用错 spec（少一个业务键）不会被任何测试发现"）并指派 TASK-004 补断言。

⇒ **一个 DoD 被部分满足而声明清楚，与被声称完全满足，是两回事。** 此处属前者，合规。

---

## 6. scope 一致性

```
$ git diff --stat fb15260~1..fb15260
 internal/hestia/schema.go      |  88 +++++++++
 internal/hestia/schema_test.go | 272 ++++++++++++++++++++++++++
```
与 `writes` 声明（`./internal/hestia/schema.go`、`./internal/hestia/schema_test.go`）逐项一致，**无越界申报**。`packages` 为 `./internal/hestia`，与实际测试包一致。

validator 对本任务有 2 条 `scope-writes-outside-packages` 告警——经核实那两条针对 **TASK-001** 的 fields.go/fields_test.go，与本任务无关。

---

## 7. 对 DoD 本身的反馈

三条都是正面的，无需修改：

1. **functional[0] 显式删去「+主键等固定列」并改判据为「列数恰好 61」**——这个修订是对的。`PRIMARY KEY (...)` 是表约束不是列，`pragma_table_info` 返回 61 行；若保留原表述，一个验证者会去找不存在的第 62 列。测试注释里也复述了这条（L140），来源可追。
2. **functional[1] 明写「必须用 reflect 机制化断言，不能只靠人眼比对」，并说明 TASK-002 那条只保护一端**——V2 实证了这个判断：`metaColumns` 单方面换序时，TASK-002 的 `TestMetaFieldOrderIsCrossTaskContract` **不红**，只有本任务新加的那条红。契约第二端确已补上，第三端（TASK-005 的 insert）仍裸奔，dev 也在 `key_findings` 与 `notes_for_downstream` 里点名交接了。
3. **non_functional[2] 把「为什么必须从真实库读」连同两条实测后果一起写进 DoD**——V1/V3/V6/V7 四个变异全部只被这一条测试抓住，说明这些断言不是装饰。

另：dev 给 `toSnakeCase` helper 加的四条自我断言（L116–119）是必要的——`ArticleID` 是 `Meta` 七字段里唯一有歧义的形态，helper 若转成 `article_i_d` 而 `metaColumns` 恰好也那么写，两个错误会互相印证、整条断言全绿。已核实四条覆盖了 `ArticleID`/`PeriodType`/`IngestedAt`/`Period`；未覆盖的 `PublishedAt`/`CaliberVersion`/`Extractor` 均为无连续大写的常规形态，风险低。
