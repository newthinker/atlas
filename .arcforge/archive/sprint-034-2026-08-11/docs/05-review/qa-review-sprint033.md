# QA Code Review — Sprint-033 / `internal/hestia`（M1b-1 Store 层）

- **审查者**：qa-agent-10
- **审查对象**：`internal/hestia` 包 4 个生产文件 + 4 个测试文件
- **git ref**：`feat/hestia-store` @ `823ca15`（复核时 HEAD 已推进到 `6fd9107` 归档 commit，`git diff 823ca15 6fd9107 -- internal/hestia/` **为空**，审查对象逐字节一致）
- **基线**：`go vet` 干净；**86 PASS**（`go test ./internal/hestia/ -count=1 -v | grep -c '^    --- PASS\|^--- PASS'`）

## 判定：**REJECT**

2 条 CRITICAL + 7 条 WARNING，**全部经我本体独立实测复现**（非采信子代理回显）。
达到 `severity_threshold: warning`。

**但机制上不能走 `review_fix`**：7 个任务已全部 `accepted`，Sprint 已归档到
`.arcforge/archive/sprint-033-2026-08-09/`。**审查在归档之后才完成**——归档时
`docs/05-review/` 是空的，即本 Sprint 是**在没有任何 code review 报告的情况下关闭的**。
⇒ 建议 Leader 按「新任务」而非「返工」处置，见文末清单。

---

## 审查对象的三处校正（派单信息已过时，记录以免后来者按派单追溯）

| 派单所述 | 实际 |
|---|---|
| HEAD `ed776a0`，9 commit | `823ca15`，11 commit（多 `572f2ce` G10 白名单、`823ca15` C3/C4 守护） |
| T1–T7 `verified` | 全部 `accepted`（leader @ `2026-08-09T07:17:08Z`） |
| 已登记缺口 H1–H7 | 实际 **H1–H11**（H8–H10 在 `handoff-additions-h8-h10.md`、H11 在 `handoff-h11-cross-package.md`） |

派单点名要我切入的 **O-1（`DB()` 逃生口）** 与线索 **⑤/⑥**：⑤=H9、⑥=H10，在我开始前
已由我自己上一轮登记完毕。故本轮不重复报，**只报新发现**。

---

## 第一轮：常规 Code Review

**总体质量高于本仓库均值。** 值得点名的三处正面事实（均经核实，非客套）：

1. **唯一真相源贯彻到了机制层**：`fieldOrder` → DDL → INSERT 列 → 白名单全部派生，
   且 `TestFieldNamesAppearOnlyInFieldsGo` 用 AST 把「字面量只准出现在 fields.go」变成
   可执行断言，连自己的捕获上限（`fmt.Sprintf`/变量拼接）都写在测试注释里。
2. **三处同序契约三端齐全**（H2），且第三端 `TestSaveMetaValuesLandInMatchingColumns`
   的 fixture 取值互不相同——列互换会红，不是摆设。
3. **注释承载的是「为什么不做 X」而非「做了什么」**，`verifyObservationsSchema` 对
   「多列放行、少列失败」的不对称论证是本次读到的最好的一段设计记录。

**第一轮未发现新的功能性缺陷**——常规视角能看到的问题，前一轮 QA 的 C1–C4 已经清完。
本轮全部新发现来自第二轮，这本身说明常规 review 在这个包上已经触底。

---

## 第二轮：跨视角对抗

**用的是哪一种（如实说明）**：**纯 Claude 三视角**（Skeptic / Architect / Minimalist），
各自独立 context、只读、禁写。**未做跨模型复核**——`codex` CLI 实测可用
（`/Users/zuowei/.nvm/.../bin/codex`）且 `cross_model: "auto"`，但按 QA 角色定义，
该调用归 **Leader 主 session**，且它会把代码送到外部服务。**建议 Leader 补跑一轮**，
本报告的 2 条 CRITICAL 是合适的复核靶子。

**一处子代理回显不可信的实证（记录以佐证核实纪律的必要性）**：Skeptic 报告称结束时
`git status --porcelain internal/hestia/` 为空，Minimalist 同期实测报告该目录存在
Skeptic 遗留的 `zz_skeptic_probe_test.go`。**两份回显直接冲突**；我本体直读时该文件
已不在（Skeptic 事后清理了），即两方在**各自时点上都没说谎**——但若我采信任一方
的单次回显，都会得出错误的工作区结论。

---

## ① 真缺陷（新发现，需处置）

> 下列每条的「实测」均为我本体在 `823ca15` 工作区跑的探针/变异，非转述。
> 变异类结论一律附四条自证：diff 非空 / `go vet` 通过 / `--- PASS` 计数 / 首行完整。

### CRITICAL-1 · `rep.Checks[].Value` 的非有限值让该期数据**两张表同时消失**
`store.go:300`（`checkValues`）/ `store.go:461`（`savePending`）

`checkValues` 遍历的是 `Observation.Values`，**从不看 `rep.Checks[].Value`**；
而 `savePending` 第一件事是 `json.Marshal(rep)`。

**实测**：
```
Check{ID:"deposit_sum", Status:CheckFailed, Value:&NaN}, Passed:false
→ err = hestia: marshaling validation report: json: unsupported value: NaN
→ 观测表 0 行、pending 0 行
```

为什么这是缺陷而不是遗漏：`store.go:294-299` 花四行专门论证 NaN **必须**在入口拦下，
理由**逐字**就是「NaN 若流到 pending 路径会让 savePending 失败 → Save 返回 error →
两张表都没有这条数据，而 pending 存在的理由正是『不让那期数据彻底消失』」。
同一条推理、同一个包、隔壁一个字段，没被覆盖。

而它**必然会被触发**：该段注释自己点名最常见来源是「比率型字段的 0/0」，
`Check.Value` 恰恰就是比率型闸门的实测值——现有 fixture
`TestSaveFailedValidationGoesToPending` 里它就是 `deposit_sum = 0.0857`。
M1b-3 实现七道闸门时，第一个 0/0 就会让那一期彻底消失。

**建议**：`Save` 里加一轮 `checkReportValues(rep)`，与 `checkValues` 并列排在写库前。
约 6 行 + 1 条测试。

### CRITICAL-2 · `caliber_version` / `extractor` 用与 `period_type` 完全相同的文体枚举了合法值，却只有非空检查
`types.go:31-32` / `types.go:38` / `types.go:79-81`

```go
PeriodType     string // 必填，monthly | h1 | annual        ← 有 validPeriodTypes 白名单 + 专测
CaliberVersion string // 必填，2015-01 | 2023-01 | 2025-01  ← 只有 f.val == "" 检查
Extractor      string // 必填，rule@v1 | rule@v2 | llm-fallback@v1  ← 同上
```

**实测**：`"2025-1"` / `"garbage"` / `"2099-99"` 三个 `CaliberVersion`、
`"totally-made-up"` 的 `Extractor`，`validate()` **全部返回 nil** 并原样落盘。

危害不对称在于 `caliber_version` 不是普通元数据：`fields.go:71-72` 写着
「M1 口径在 2025-01 修订过（纳入个人活期存款），跨该点的同比无效。
**这件事校验闸门拦不住，只能靠 caliber_version 标注**」——即它是那个数据断裂点的
**唯一**防线，而这条防线接受任意字符串，且失败**完全静默**。一个 `"2025-1"` 的笔误
会让下游按 caliber 分组的逻辑静默漏掉那一期。

这与本 Sprint 刚做过的 C2 返工（`checkReportConsistency` 从单值黑名单改白名单，
理由逐字是「同一个包、同一条 Save 路径、同一类外部输入，把关强度必须一致」）
是**同一条推理**，只是没走到 `Meta` 上。

**可预见的反驳与我的回应**：合法集会随年份增长（2027-01…），加白名单意味着每次
口径修订要改代码。但 `period_type` 面对完全相同的问题仍然选了白名单——两者不该
不一致。**若裁定不加，必须把「刻意不校验」写进注释**，否则现在的注释在宣称一个
不存在的约束，那比没有注释更坏。

### WARNING-1 · 未知字段名让**未过闸**的数据两张表都进不去
`store.go:204`（校验顺序）/ `store.go:300`

**实测**：`Values: {"tsf_stock_govt_bonds": 1}`（正确名少个 s）+ `Passed=false`
→ `err = hestia: unknown field(s) tsf_stock_govt_bonds`，**观测表 0 行、pending 0 行**。

`checkValues` 对键把关的理由（`store.go:292`）是「键会拼进 INSERT 的列名」——
这个理由在 `savePending` 路径上**不成立**：pending 把 `Values` 序列化成 JSON，
键从不进 SQL。但校验排在 `rep.Passed` 分支之前，于是解析器打错一个键名时，
发生的正是 `Save` 文档注释（`store.go:195-197`）声明要避免的
「若只是拒绝……那期数据就彻底消失了。一个入口、两个目的地」。

与 CRITICAL-1 是同一个失效形状：**过不了闸的数据比过了闸的更容易彻底丢失。**

**建议**：键校验只在 `rep.Passed` 的写库路径上硬失败；未过闸时把未知键连同报告
一并存进 pending（打错的键名本身就是最有价值的诊断信息）。

### WARNING-2 · 漂移检查只比对**列名**，类型漂移静默放行——而 schema.go 自己论证过类型是承重的
`store.go:144`（`verifyObservationsSchema`）

**实测**（把 `m2` 建成 `TEXT` 的老库）：
```
NewStore err = <nil>          ← 未被拦下
typeof(m2) = text
MAX(m2) over {356.71, 9.5} = 9.5     ← 字典序，静默算错
```

`schema.go:44-45` 逐字写着「业务列是 REAL 而非 TEXT：**亲和性错了不会报错**，
Scan 进 float64 照样成功，但 SQLite 会按字典序比较，**MAX() 与范围查询全部静默算错**」。
而漂移检查的立身之本（`store.go:79-82`）是「不检查的话老库能一路开到第一次 Save 才炸」
——**类型漂移根本不炸**，它比缺列更符合该函数要拦的危害画像，却完全不在拦截面内。

**附带的测试缺口（变异实证）**：把 `slices.Concat(metaColumns, fieldOrder)` 改成
`fieldOrder`（并删 `slices` import）——
> diff `1 insertion(+), 2 deletions(-)`（非空）／`go vet` exit 0／`--- PASS` = **86**（=基线）／首行 `package hestia`

**存活**。原因：`TestNewStoreRejectsSchemaDriftOnLegacyDB` 的 fixture 把七个
`metaColumns` 全建齐了、只砍业务列，**「元数据列缺失」这半边从未被验证过**。

**建议**：`pragma_table_info` 已经取到 `type` 列，一并比对即可（业务列 REAL、元数据列
TEXT），代价是同一次查询多 Scan 一列；并给漂移测试补一条「缺 `caliber_version`」用例。

### WARNING-3 · 两条写口守卫对**导出 `var` 形态**完全失明，M10 的洞换个语法重新打开
`store_test.go:308`（reflect）/ `store_test.go:327`（AST）

`TestPackageExposesNoWriteFunctions` 的注释明写它是为了补 M10（包级
`func InsertRow(db *sql.DB, ...) error` 绕过 `ValidationReport`）。但它只扫
`*ast.FuncDecl`。

**实测变异**：在包内加入
```go
var InsertRow = func(db *sql.DB, period string) error { /* 直接 INSERT，无 ValidationReport */ }
```
> 新增文件 `zz_m3.go`（diff 非空）／`go vet` exit 0／`--- PASS` = **86**（=基线）／被测包首行完好

**两条守卫双双 PASS，86 全绿。** 判据说的是「任何导出写口」，守卫仍窄于判据——
只是这次窄在 `var` 而不是包级函数上。

**建议**：同一条测试里补一轮 `*ast.GenDecl` 扫描，把导出的 `var`/`const`/`type`
名并进那个精确集合断言。约 15 行，**只改测试不动生产代码**——本报告里性价比最高的一条。

### WARNING-4 · insert 路径的主键碰撞行为**完全无测试**，`INSERT OR IGNORE` 存活
`store.go:420`（`insertSQL`）

**实测变异**：`INSERT INTO` → `INSERT OR IGNORE INTO`
> diff `1 insertion(+), 1 deletion(-)`（非空）／`go vet` exit 0／`--- PASS` = **86**（=基线）／首行 `package hestia`

**存活**。危害在于它与 H9（OutOfOrder 重放硬失败）是一对：H9 那条噪音**最省事的
「修法」正是加 `OR IGNORE`**，而加上之后，G9 契约登记的那次并发写入丢失会从
「响亮丢失」退化为「静默丢失」，整套测试零阻力。

`TestSaveDuplicateIsDefinedNotSilent` 的注释声称防住了 `INSERT OR IGNORE`，
但它防的是 `Duplicate` 路径，而 **`Duplicate` 根本不经过 `insert`**——
该守卫在 insert 路径上是空的。

### WARNING-5 · `ValidationReport{Passed: true}` 空 `Checks` 旁路：注释亲口点名，既未关也未登记
`store.go:330-332` / `types.go:124-127`

**实测**：`ValidationReport{Passed: true}`（无任何 Check）→ `err=<nil>`，
`Verdict=New`，**直接进权威表**。

`checkReportConsistency` 的注释自己写了「`ValidationReport{Passed: true}` 是
**22 个字符的旁路**」，然后只关了「自相矛盾」这一类。而它既不在 H1–H11，
也没有测试——属于**「注释里承认、交接文件里没有」**，正是
`handoff-to-later-iterations.md` 开篇批评的那条「已知缺口变成已遗忘缺口」的路径。
观测表不存任何校验痕迹，那种行落库后事后无从审计。

**建议**：`if rep.Passed && len(rep.Checks) == 0 { return error }`，3 行，今天就能做。
M1b-3 落地后再升级为「必须覆盖七道闸门的 ID 集」。

### WARNING-6 · G9 并发契约**登记的后果与实测不符**，且整段可被无声删除
`fields.go:18-31`

契约原文：「都判为 New、都执行 INSERT，主键约束让**其中一方拿到 UNIQUE 错误**
——那一次的数据随之丢失」。这描述的是一个**响亮**的失败。

**实测**（3 轮 × 8 goroutine，同业务键同 `published_at`，各带不同 `m2`）：
```
run0 tally=map[Duplicate:7 New:1] | obs_rows=1 m2=107 pending=0
run1 tally=map[Duplicate:7 New:1] | obs_rows=1 m2=107 pending=0
run2 tally=map[Duplicate:7 New:1] | obs_rows=1 m2=107 pending=0
```
**24 个 goroutine 全部 `err=nil`，零 UNIQUE 错误。** 落败者不是撞主键，而是 Lookup
已看到先写入的那行 ⇒ 判 `Duplicate` ⇒ 走 `refreshArticleID` ⇒ **`Values` 被静默丢弃**
（H3 的机制）。调用方 `if err != nil` 什么都看不到。

⇒ G9 的**实际危害等级高于登记**：不是「偶尔丢一次并报错」，而是
**并发窗口内几乎所有写入都静默退化成 H3**。机制早已登记，但**登记的后果描述会误导
后续「该不该修」的判断**。

**附带（变异实证）**：把整段「# 并发契约」从包注释删除——
> diff `0 insertions(+), 14 deletions(-)`（非空）／`go vet` exit 0／`--- PASS` = **86**（=基线）／`fields.go` 首行仍是 `// Package hestia ...` 包注释起始行

**86 全绿。** 而同文件的 `TestPackageDocDeclaresUnits` 存在的理由逐字是
「单位不入库、也没有 units_version 列，唯一记录它的地方就是这段注释」——
**同一条推理对 G9 完全适用**（并发契约不入库、无机制、唯一记录就是注释），
但它的 want 列表只有四个词，不含并发。

**建议**：（i）改正后果描述为实测形态；（ii）want 列表加 `"串行化"`，1 行，
让纯契约项至少有存在性守卫。

### WARNING-7 · `verifyCurrentView` 的全等比对会对**纯书写差异**假阳性，且错误文案是误诊
`store.go:113`

**实测**：把视图定义的 `" AS "` 改成 `" AS\n  "`（SQL 语义完全相同）→
```
NewStore 失败
错误: hestia: view v_hestia_current was created by a different schema version and CREATE VIEW IF ...
```

**先说被推翻的假设**：跨 SQLite 实现**不是**风险来源（Skeptic 用系统 `sqlite3 3.51.0`
与 modernc 跑同一份 DDL，`sqlite_master.sql` 逐字节相同）。真正的假阳性来源是
**语义相同但书写不同**——sqlite_master 只做四件规范化（CREATE/VIEW 关键字、
去 IF NOT EXISTS、去库名限定、去结尾分号），其余原样保留。

⇒ 注释里「视图没有『多／少』的中间态，**定义不符即语义不符**」这句在假阳性方向上
是错的；错误文案断言 "was created by a different schema version" 在这种情形下是
**误诊**（视图完全正确，只是不是这段 Go 代码亲手建的）。

二阶成本与 **H11 相扣**：H11 只登记了「`bitemporal` 侧格式改动会让既有库打不开」，
但同一个全等比对还隐含依赖 **SQLite 驱动逐字保存 `CREATE VIEW` 文本**——驱动升级
若规范化空白，**全部既有库同时打不开**，而 H11 没覆盖驱动侧。

**建议**：比对前对两侧做空白折叠（`strings.Join(strings.Fields(s), " ")`）并对关键字
大小写不敏感——保留「业务键不同」这个真信号，去掉「书写不同」这个假信号；
或至少把错误文案改成「definition text differs」，不替读者下因果结论。

---

## ② 对已登记缺口的新证据（不另开条目，建议补进原文）

- **H4（pending 不做列校验）的成本论证被推翻。** H4 说加校验「需要一份 pending 期望
  列清单，而 `pendingDDL` 已有一份，那是本包明确反对的第二份 schema 定义」。
  这个代价**可以规避**：不写清单，而是从 `pendingDDL()` 的**产物**取——建一张临时表
  跑一次 `pendingDDL`、读它的 `pragma_table_info`、与实库比对，**零第二份定义**，约 20 行。
  ⇒ 「必须写第二份 schema」这个理由不再成立；是否值得做仍由 Leader 定，但**理由要换**。
  另：H4 残余的严重度值得写得更明确——漂移只在**第一次写 pending** 时暴露，
  而那一刻恰是一期数据过闸失败正要被保全的时刻，结果两张表都没有落点，
  与 CRITICAL-1 / WARNING-1 是同一个失效形状（本 Sprint 该形状已出现三次）。
- **H11 的孪生项**：见 WARNING-7 末段（驱动侧未覆盖）。
- **H9 的触发场景与 `refreshArticleID` 的设计目的完全重合**：站点迁移后全量重扫
  （`refreshArticleID` 注释指名的那个场景）在一个**有过修订**的期次上，重扫最新
  `published_at` 判 Duplicate 正常刷 id，重扫历史那条则 `UNIQUE constraint (1555)`
  硬失败 ⇒ 历史修订行的 `article_id` **永远刷不上** ⇒ H6-3 的一级幂等检查永远 miss
  ⇒ 每轮重扫固定产生一条告警。即 H9 让 `refreshArticleID` 的收益在最需要它的行上落空。

---

## ③ 观察级（记录，不阻断）

- **pending 表独独丢掉 `caliber_version`**（`schema.go:67-79`）。六个必填 Meta 里
  五个进了 pending，`values_json` 也不含它 ⇒ 从 pending 行上不可恢复。而
  `fields.go:71-72` 恰好说 M1 口径断裂「只能靠 caliber_version 标注」——对一个
  人工排查 pending 的人，这是最该看到却唯一看不到的字段。不算数据丢失（凭
  `article_id` 可回源），但与 pending「保留诊断信息」的定位相悖。
- **包内已存在一处被自己契约禁止的 `ORDER BY ingested_at`**：`store_test.go:917`
  的 pending 读取 helper。`store.go:223` 的契约（H5）是「不要对 `ingested_at` 做
  ORDER BY 或 MAX()」。当前无害（fixture 要么全整秒要么全带小数），但**它是后来者
  会照抄的那一处**，抄走之后失效是静默的。改 `ORDER BY rowid` 即可，1 行。
- **G8（RFC3339Nano）的覆盖是单点，不是看上去的三重**：把 `store.go:224` 改回
  `RFC3339`，只有 `TestPendingDistinguishesSameSecondAttempts` 一条转红——另两条
  断言 `ingested_at` 落盘值的测试都用整秒 fixture，对格式不敏感。**不是缺陷**
  （那条测试构造得好），记下来是为了**避免将来有人以为可以删掉它**。
- **`schema_test.go:223` 是一条永不可能失败的断言**：`assert.Contains(t, ddl, "period")`
  被下一行的 `"period_type"` 严格蕴含。而这恰是本包在 `store_test.go:121` 自己写下
  「`period` 是 `period_type` 的前缀，子串断言会被另一者蕴含（本包已因此漏过两次缺陷）」
  的那个陷阱。删掉该行。
- **`store_test.go:182-200` 两段文档注释被融合**：`TestNewStoreToleratesUnknownExtraColumns`
  的 6 行注释与 `TestNewStoreRejectsDriftedCurrentView` 的注释之间缺一个空行，
  结果整块挂在了后者头上，而前者（在 254 行）没有任何文档注释。纯位置问题，移动即可。
- **Minimalist 结论：本包没有过度设计。** `insertSQL` 抽出、`now` 注入、`Outcome`、
  两个 verify 函数四项抽象**全部是挣来的**（`insertSQL` 若不抽出，「遍历 fieldOrder
  而非 map」就只能靠读代码确认；`now` 若不注入，G8 的同纳秒边界测试根本写不出来）。
  它差点砍掉 `checkValues` 的两轮遍历（合并成一轮看起来是净胜），最后判定保留——
  第一轮 range 的是 `fieldOrder` 而非 map，这决定了**两个字段都是 NaN 时错误信息
  指名哪一个是确定的**；改成 range map，同一份坏输入每次给出不同的错误串。
  这个判断我复核后同意。

---

## ④ 主交付：「靠契约不靠机制」清单

Leader 点名要的包级事实。类别：**(a)** 有机制 / **(b)** 有契约无机制 / **(c)** 连契约都没有。

| # | 条目 | 类 | 依据 | 违反后果 |
|---|---|---|---|---|
| 1 | `DB()` 写入绕过 | **b** | 契约仅 `store.go:186`；两条守卫看不见句柄之后 | **全静默**，五道防线一次全绕过 |
| 2 | 写口守卫对导出 `var`/`const`/`type` 失明 | **c** | 守卫只看 `*Store` 方法集与 `*ast.FuncDecl` | **全静默**（WARNING-3 实证） |
| 3 | G9 并发契约（同键 Save 串行化） | **b** | `fields.go:18-31`；`-race` 明确看不见 | **全静默**，且**登记的后果本身写错了**（WARNING-6） |
| 4 | G9 契约文本自身无存在性守卫 | **c** | `TestPackageDocDeclaresUnits` 只守单位四词 | 整段可删，零测试转红（WARNING-6） |
| 5 | `Check.Reason`「Skipped 必填」· Passed=true | **a** | `store.go:357-361` + `TestSaveRejectsSkippedWithoutReason` | 已闭合（C2 返工） |
| 6 | 同上 · Passed=false | **b** | `store.go:343-345` 明文声明刻意不查 | 仅进 pending JSON，消费者是人；论证成立 |
| 7 | `Passed=true` 空 `Checks`（22 字符旁路） | **b** | `store.go:330-332` 自己点名且不关 | **全静默**进权威表（WARNING-5） |
| 8 | `Check.Value` 非有限值 | **c** | `checkValues` 从不看 `rep.Checks[].Value` | 不静默，但该期**两张表都没有**（CRITICAL-1） |
| 9 | 单位约定 · 注释存在性 | **a** | `TestPackageDocDeclaresUnits` | 已闭合 |
| 10 | 单位约定 · 数值实际符合单位 | **c** | 无任何量级/区间检查 | **全静默**：亿元当万亿元写 = 10000× 误差，REAL 亲和性正确故无类型信号 |
| 11 | `Meta` 三处同序 | **a** | reflect ×2 + 落盘验证 ×1，三端齐全 | 已闭合（H2） |
| 12 | pending 八列（第四处同序） | **a** | `TestSavePendingColumnsMatchTheirValues` | 已闭合（C3 返工） |
| 13 | 字段名字面量只准在 fields.go | **a** | `TestFieldNamesAppearOnlyInFieldsGo` | 捕获上限测试自己写明 |
| 14 | `ingested_at` 不得 `ORDER BY`/`MAX()` | **b** | H5 契约；测试钉的是**理由**不是禁令 | **全静默**；且包内已有一处违反（③） |
| 15 | `ingested_at` 用 RFC3339Nano（G8） | **a** | 单点支点，非三重（③） | 已闭合 |
| 16 | pending 表列漂移 | **a** | `TestSavePendingFailsLoudlyOnDriftedPendingTable` | 确定性响亮失败（H4）；成本论证已被推翻（②） |
| 17 | `Values`「键不存在即缺失」 | **a** | NaN/Inf 闸门 + 白名单 + 落 NULL 测试 | 已闭合。空 `Values` 合法为刻意 |
| 18 | 观测表列**名**漂移 | **a** | `verifyObservationsSchema` | 已闭合（但只覆盖业务列半边，WARNING-2） |
| 19 | 观测表列**类型**/主键漂移 | **c** | 无任何检查 | **全静默**，`MAX()` 按字典序算错（WARNING-2） |
| 20 | `period`/`published_at` 形态 | **a** | 两个正则 + 五条测试（含尾随换行用例） | 已闭合；日历有效性=H1、组合合法性=H10 |
| 21 | `caliber_version` / `extractor` 取值 | **b→c** | 注释枚举了合法值，只有非空检查 | **全静默**（CRITICAL-2） |
| 22 | `caliber_version` 跨口径同比无效 | **c** | `fields.go:71-72` 声明，全包无消费者 | 属下游，但**未登记任何落点** |
| 23 | `bitemporal` spec 与视图同源 | **a** | 单一 spec 同喂两侧 + `verifyCurrentView` | 已闭合；跨包格式依赖=H11 |
| 24 | 全等比对依赖驱动**逐字保存**视图文本 | **c** | `store.go:109-110`「已实测」 | 驱动升级规范化空白 ⇒ 既有库全打不开（WARNING-7） |
| 25 | 观测表 PK 与 spec 同源 | **a**（非派生） | PK 是 `schema.go:55` 手写字面量，第四份拷贝 | 当前有测试兜住，无静默口子 |
| 26 | `Store.now` 注入点 | **a** | `now` 未导出 + Save 无条件覆写 + 三条测试 | 已闭合 |
| 27 | `insert` 失败不落 pending | **b** | 「一个入口、两个目的地」只对**过闸失败**成立 | H9 已就 OutOfOrder 登记 |
| 28 | 错误路径一律返回 `Outcome{}`（Verdict=New） | **b** | H8 观察级已登记 | — |

**包级事实**：**(b) 类 9 条、(c) 类 7 条**——本包有 **16 处靠自觉**，其中 **8 处违反后
完全静默**（#1 #2 #3 #7 #10 #14 #19 #21）。这份表本身就是 test-agent-21 说的
「目前没有任何一处能回答『这个包有哪些地方靠自觉』」的答案，**建议随代码入库**
（如 `internal/hestia/CONTRACTS.md`），否则它会随本报告一起被归档遗忘。

---

## ⑤ 建议的后续处置（任务已 accepted，走新任务而非 review_fix）

**按性价比排序**，前三条合计约 25 行生产代码 + 3 条测试：

| 优先级 | 内容 | 成本 | 对应 |
|---|---|---|---|
| P0 | `checkReportValues(rep)`：拦 `Check.Value` 的 NaN/Inf | ~6 行 + 1 测试 | CRITICAL-1 |
| P0 | 写口守卫补扫 `*ast.GenDecl`（导出 var/const/type） | ~15 行，**只改测试** | WARNING-3 |
| P0 | `Passed=true` 要求 `len(Checks) > 0` | 3 行 | WARNING-5 |
| P1 | `validCalibers` 白名单（或显式写「刻意不校验」） | 4 行 + 1 测试 | CRITICAL-2 |
| P1 | 漂移检查一并比对列类型 + 补「缺 metaColumn」用例 | ~10 行 | WARNING-2 |
| P1 | 键校验移到 `rep.Passed` 之后 | ~5 行 | WARNING-1 |
| P1 | 补一条「insert 主键碰撞不得被静默吞掉」的测试 | 1 测试 | WARNING-4 |
| P2 | 改正 G9 后果描述 + want 列表加「串行化」 | 2 行 | WARNING-6 |
| P2 | `verifyCurrentView` 比对前空白折叠 / 改错误文案 | ~3 行 | WARNING-7 |
| P2 | `DB()` 收窄为只读接口（**包外当前零调用方，实测 25 处命中全在包内**） | ~10 行 | 清单 #1 |
| P3 | 三条观察级（pending 补 caliber_version、`ORDER BY rowid`、删 `schema_test.go:223`） | 各 1 行 | ③ |

**关于 `DB()` 收窄与 M1b-4 是否冲突**：**不冲突**。M1b-4 需要的（H6-3）是
`SELECT 1 FROM hestia_observations WHERE article_id = ?`，一个只读接口原样满足；
把 `DB() *sql.DB` 改成 `DB() Reader`（`interface{ QueryContext; QueryRowContext }`，
与 `bitemporal.Querier` 同一手法）即可让 Go 侧写入绕过变成**编译期不可能**。
**现在是唯一零成本的时间窗**——包外零调用方；等 M1b-4 落地，它就成了第一个既成事实。
**残余要写清楚**：Grafana 是**进程外**直连，任何 Go 类型都管不到，那一侧只能靠
`mode=ro` DSN 或文件权限。收窄 `DB()` 关的是 Go 侧这一半，不是全解。

**另建议 Leader 补跑 `codex` 跨模型复核**（CLI 实测可用，`cross_model: "auto"`），
靶子选 CRITICAL-1 与 CRITICAL-2。

---

## 附：流程层面的两条观察（给 Leader，非代码问题）

1. **本 Sprint 在没有 code review 报告的情况下归档**：`docs/05-review/` 在归档时为空，
   而 `06-acceptance/final-report.md` 已写就。质量门禁的顺序被绕过了一次。
2. **`572f2ce` 的溯源陷阱已登记但值得重申**：`verifyCurrentView`（C1 的修复）随该
   commit 落盘，而 commit 标题只写了 G10 白名单。按 commit message 追溯 C1 会找错
   地方，须按符号名 `git log -S 'func verifyCurrentView'`。
