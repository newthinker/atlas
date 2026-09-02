# Sprint M1c-4 · 第一轮 Code Review（常规）


> **provenance**：本报告内容由 **`qa-m1c4`** 产出（两轮审查、全部实验均由该实例执行）。
> 落盘实例是 **`qa-m1c4-r4`**，原因是 `qa-m1c4` 的 `ARCFORGE_TOKEN` 与矩阵登记值不符
> （持有 `3d162516…` / 登记 `60951188…`，三次试写全 DENY），由 Leader 首次登记
> `qa-m1c4-r4` 后改名落盘。**不是另一个人审的，`transitions`/`by` 字段里的 `qa-m1c4-r4`
> 与 `qa-m1c4` 是同一个实例。** 这是身份轮换，不是返工。

**审查者** qa-m1c4 · **日期** 2026-09-02

## 锚与范围（两端都是全 sha，不是 `HEAD`/分支名）

```
base 4670ccbe0abd703f86b1e0c53aef8d3c86cc512d
head c6f33b71d9d123a8110c668a75b451a8fd206a49
git diff base..head -- internal/hestia cmd/atlas configs   → 33 files, 6825 insertions(+), 452 deletions(-)
```

⚠️ **审查时 `TASK-014` 处于 `in_progress`，其 `writes` 含本报告审过的 6 个文件**
（`validate.go` / `backfill_load_report.go` / `profiles.go` / `fields.go` / `extract.go` / `CONTRACTS.md`）。
⇒ **本报告的全部结论只对 `c6f33b71` 这棵树成立**；TASK-014 合入后须在新锚上重采。

## 结论

**PASS** —— 0 CRITICAL、0 代码级活缺陷。

## 方法与自证

| 项 | 结果 | 被比较集合的大小 |
|---|---|---|
| `go build ./...` + `go vet ./internal/hestia/... ./cmd/atlas/...` | 零输出 | 全仓库 |
| `go test ./internal/hestia/... ./cmd/atlas/...` | 全绿，**FAIL 行数 = 0 行** | 2 个包 |
| 真语料 `backfill load`（218 篇） | 入权威表 **76 个观测** / 落 pending **21 个观测**，四道恒等式全部成立 | 与 CONTRACTS §B 逐字相同 |
| 基线确定性自检 | 两轮**逐字节一致** ✓ | 2 轮 × 394 行 |
| pending 归属分布 | `tolerance_exceeded[ytd]` **1 条** + `drift_exceeded[mom]` **12 条** + `drift_exceeded[ytd]` **8 条** = **21 条** | 与 CONTRACTS §C 逐字相同 |
| `magnitude_ranges` 组规则核对 | **0 项不符** | **被检查集合 = 22 个字段**（带实测注释的全部条目） |
| 变异测试 | **7 KILLED / 1 SURVIVED** | **8 个有效变异体**（另有 1 个被有效性闸拦下重做） |

⚠️ **主工作区指纹**：全部变异在隔离 worktree（`/tmp/qa-m1c4-mut`，detached @ `c6f33b71`）内进行，
收尾核对 `git status --porcelain -- internal/hestia cmd/atlas configs` 为空、被变异文件在主工作区
的 sha256 **漂移 0 个（被检查文件数 = 4 个）**。worktree 已 `git worktree remove` 拆除。

### `configs/hestia.yaml` 的 22 项 `magnitude_ranges`（Leader 指定项 4）

组规则 `[min<0 ? min×4 : −max×0.5, max×4]`，圆整向外，**22/22 逐项相符**。
脚本 `<scratchpad>/qa-m1c4-check-ranges.py` 从 yaml 正则抽取「配置值 + 行尾注释里的实测 min/max」，
两者独立求值后比对。抽样两行：

```
OK  tsf_flow_mom            cfg=[  -37000,  290000]  rule_raw=[ -36100.0, 288800.0]
OK  deposit_fiscal_mom      cfg=[  -34000,   55000]  rule_raw=[ -33916.0,  54800.0]
不符组规则的条目数 = 0 个字段（被检查集合大小 = 22 个字段）
```

标定算式亦逐条手算复核通过：`K_abs = 2272×1.40 ≈ 3200`、`K_rel = 0.3931×1.40 ≈ 0.55`、
`corp_loan mom = 0.0788×1.40 ≈ 0.11`、`1.40 = 0.05/0.0357`、`0.17×147300 = 25041 < 28000`、
交叉点 `3200/0.55 ≈ 5818 亿元`、中位分母门限 `0.55×12700 = 6985 < 历史最大绝对差额 8681`。
**取值公式全部可从分位数复算，无一处「拍的」。**

### 变异测试（Leader 指定项 2：确认断言真在守）

| # | 变异 | 位置 | 结果 | 杀死它的测试 |
|---|---|---|---|---|
| M1 | `sectorCaliberOf` 取**最早**期次前缀而非最近 | `extract.go:655` | **KILLED**（5 红） | `TestSectorCaliberGuardsBothSides` 等 |
| M2 | `bandLimitRatio` 丢掉 `K_abs`，退化为纯比值 | `validate.go:317` | **KILLED**（1 红） | `TestDepositSumMoMAbsoluteFloorCoversSmallDenominator` |
| M3 | `selectRMBFlowByCaliber` 当月族谓词丢掉 `m[1] != ""` | `extract.go:359` | 🔴 **SURVIVED** | —— 见 R1-2 |
| M4 | `nameField.pick` 在当月口径下返回 `ytdField` | `profiles.go:325` | **KILLED**（12 红） | golden + 路由测试共 12 条 |
| M7 | `gateDepositSum` 两族顺序反转（两族都齐时取 mom） | `validate.go:379` | **KILLED**（1 红） | `TestDepositSumPrefersYTDWhenBothPresent` |
| M8 | `residualGates` 的 mom 族 `total` 列漂移（副本与闸门分叉） | `calibrate_report.go:403` | **KILLED**（3 红） | `TestResidualGatesAgreeWithValidationGates` 等 |
| M9 | `missingCaliberAware` 只建 ytd→mom 单向孪生 | `required.go:142` | **KILLED**（21 红） | 端到端 21 条 |
| M10b | `stripHTML` 把行首删除**换序**到 `spaceRE` 折叠之前 | `strip.go:62` | **KILLED**（2 红） | `TestStripHTMLRemovesLeadingWhitespace`、`TestStripRealSampleTitlesSurviveLeadingWhitespace` |

🔴 **M10b 值得单记**：`strip.go:71-76` 的注释**预先声明**「换序变异实测转红的是」这两条测试，
实测转红的**正是这两条、逐字相同**。⇒ 该处注释的自证是可核查且成立的。

⚠️ **有效性闸真的拦下了一次**：M10 第一版我把行首删除那行**复制**到 `spaceRE` 之前而没有移走
（= 做了两遍，不是换序），SURVIVED。若不看 diff 会记成「注释的声明不成立」这个**方向完全相反**
的结论。按纪律早退重做为 M10b 后 KILLED。**这是「语义闸」而非「语法闸」拦下的 —— 变异体
`bash -n`/`go build` 都过。**

---

## 发现清单

### [MAJOR] R1-1 · CONTRACTS §F / 结转项 7-d 的「没有任何测试守着」是假的

**位置** `internal/hestia/CONTRACTS.md:2203`（及 `:2200`、`:2349`）
**观察**（这是观察不是推理 —— 用**真的 61 列老库**跑出来的）：

```
$ cp data/hestia.db.bak-20260902-204417 <scratchpad>/qa-m1c4-legacy/hestia.db
$ sqlite3 .../hestia.db 'pragma table_info(hestia_observations);' | wc -l   → 61
$ atlas hestia status --hestia-config <指向该库的配置>
Error: hestia: hestia_observations is missing 22 column(s) (e.g. tsf_flow_mom,
  tsf_flow_rmb_loan_mom, tsf_flow_govt_bond_mom, tsf_flow_corp_bond_mom, tsf_flow_fx_loan_mom):
  this database was created by an older schema and CREATE TABLE IF NOT EXISTS does not add columns.
  Automatic migration is an explicit non-goal; migrate manually with ALTER TABLE or rebuild ...
```

守卫是 `verifyObservationsSchema`（`store.go:152`，由 `NewStore:78` 调用），
测试是 `TestNewStoreRejectsSchemaDriftOnLegacyDB`（`store_test.go:195`），
同族还有 `TestNewStoreRejectsDriftedCurrentView`（`:241`）与
`TestNewStoreToleratesUnknownExtraColumns`（`:294`）。

**为什么要紧** §F 同时写着「`INSERT` 会因列不存在而失败，或（若走的是 JSON 路径）静默丢字段」
—— 观测表上两者都不会发生，它在**开库那一刻**就响亮失败了。这条被登记成**结转项 7-d**，
会占用下一轮预算去补一个已经存在的守卫。方向是「过度悲观」，故不威胁正确性，但
**它正是本文件反复记的那类「留一句已变假的自证比没有自证更糟」**。

**建议** 订正 §F 与 §G-7d，保留原文并标注实测证据（已由 Leader 写进 TASK-014 的 `boundary[0]`）。

---

### [MINOR] R1-2 · `selectRMBFlowByCaliber` 的「期次前缀非空」守卫零测试覆盖

**位置** `internal/hestia/extract.go:359`（注释在 `:348-350`）
**观察**

```go
// 注释（:348）：🔴 **当月族的谓词是 `m[1] != "" && !cumulativePeriods[m[1]]`，
//               不是 `!cumulativePeriods[m[1]]`**：… 写成后者会让**空前缀**落进当月族
//               ——那就是在猜，正是本迭代唯一保留的那条拒绝要防的东西。
keepCur := func(m []string) bool {
    return m[2] == currencyRMB && m[1] != "" && !cumulativePeriods[m[1]]
}
```

变异 M3 把 `m[1] != "" &&` 去掉：

- 单元测试 **FAIL 行数 = 0 行**（SURVIVED）
- 真语料 218 篇 A/B：**报告 diff = 0 行**（两份各 394 行；两个二进制 sha256 分别为
  `532e5a71…` / `f0d11499…`，**确认变异真的进了二进制**，不是「跑了同一个程序两次」）

**为什么要紧** 这是一条被注释明确指认为「本迭代唯一保留的那条拒绝」的守卫，而它
**既没有测试、在当前语料上也不触发** ⇒ 删掉它不会有任何东西变红。它是「靠写法而不是靠
守卫」的又一例，与 CONTRACTS §G「开口 b」同形，但**开口 b 已登记、这一条没有**。

⚠️ **不建议在本轮修** —— 它今天不产错数据。建议**登记进 CONTRACTS 的开口清单**，
并在下一轮补一条测试：构造一句期次捕获组为空、句尾完全正确的人民币合计句，
断言它**不**被当成当月族。

---

### [MINOR] R1-3 · 六个容差标量不校验 NaN/Inf，而同一文件 30 行外对同一失效模式有守卫

**位置** `internal/hestia/thresholds.go`（`Thresholds.validate()`，`:277` 起）
**观察** A/B 实测（两份配置差异 **2 行**，非空已核）：

```
K_rel = 0.001, K_abs = 0  → 入权威表 67 个观测 / pending 30 个观测 / tolerance_exceeded[mom] 21 条
K_rel = .nan,  K_abs = 0  → 入权威表 76 个观测 / pending 21 个观测 / tolerance_exceeded[mom]  0 条
```

⇒ `.nan` 让这道闸**静默关闭并报 passed**。`math.Max(x, NaN) == NaN`，而 `r > NaN` 恒假。

**对照**：同一张配置的 `magnitude_ranges` 对 `.nan` 是**响亮拒绝**的，理由写得一字不差：

```
Error: hestia: magnitude_ranges[m0] 的 min/max 必须是有限实数, 实得 min=NaN max=61:
  NaN 参与的比较恒假、Inf 区间永不越界，两者都会让这道闸对该字段完全不设防且报 passed
  ——YAML 里写 .nan / .inf 也会走到这里
```

**为什么要紧** 同一失效模式、同一个文件、判据只差 30 行，一个有守卫一个没有。
本轮把这类标量从 3 个（`deposit_sum_tolerance` / `_drift_max` / `corp_loan_tolerance`）
扩到 **6 个**，其中 `DepositSumToleranceMoMAbs` 还引入了 `absTol/|total|` 这一步除法
（NaN 传播路径变长）。触发需要有人手写 `.nan`/`.inf`，可能性低；但那条 `magnitude_ranges`
的错误文案本身就证明团队认为这个触发值得防。

**建议** 在 `Thresholds.validate()` 里对六个标量加一句 `math.IsNaN || math.IsInf || < 0` 的拒绝，
文案照抄 `magnitude_ranges` 那条。**属于下一轮，不建议为它返工。**

---

### [MINOR] R1-4 · `renderLoadReport` 无条件打印「四道恒等式: 全部成立 ✓」

**位置** `internal/hestia/backfill_load_report.go:90`
**观察**

```go
b.WriteString("\n四道恒等式: 全部成立 ✓\n")   // 无条件，先于 checkLoadIdentities 执行
```

`writeLoadReport`（`:44`）是「先 `renderLoadReport` 再 `checkLoadIdentities`」，**顺序本身是刻意的**
（`:22-40` 有完整论证，我不反对那个论证）。但被打印的这一行**不是表格，是结论**：
恒等式不成立时报告正文里仍写着「全部成立 ✓」，只有 `error` 与退出码说真话。
`backfill_load_test.go:733` 的注释已如实记下这个行为（「报告打印『单篇 0 + 合并组 96』这个假数，
而『四道恒等式全部成立 ✓』照常打印」），`TestLoadReportSurfacesUnclassified`（`:215`）也证实
失败路径上报告照样渲染。**无任何测试断言报告正文不得声称成立。**

**为什么要紧** 报告会被重定向到文件、贴进交接文档、事后被读。那时 `error` 与退出码都不在了，
只剩正文里那句「✓」。这与本 sprint 记的「硬编码的结论串盖住数据」是同一形状。
**pre-existing（M1c-3b 的 TASK-006 引入），不是本轮新增。**

**建议** 把那一行改成条件渲染（`checkLoadIdentities` 的结果为 nil 才打 ✓，否则打「不成立 ✗ 见下」），
或至少加一条测试钉住「失败时正文不含 ✓」。**属于下一轮。**

---

### [INFO] R1-5 · 组规则的圆整**粒度**未写明

**位置** `configs/hestia.yaml:213`（「圆整一律**向外**（下界向下、上界向上）」）
**观察** 22/22 项在「向外 + **2 位有效数字**」下逐项相符，但「2 位有效数字」这句
**yaml 里没有写**。举例：`deposit_household_mom` 上界 `32600×4 = 130400`，向外圆整到
2 位有效数字得 `140000`（配置值），到 3 位得 `131000`，两者都满足「向外」。
**建议** 在组规则那段补一句「圆整到 2 位有效数字」。

### [INFO] R1-6 · `missingCaliberAware` 的放松边界值得写明

**位置** `internal/hestia/required.go:129`
**观察** 「同族两列任一在场即不算缺」。CONTRACTS §H-6 记「单条观测里两族并存的有 **25 条观测**」
⇒ 对这 25 条，`completeness` 比改动前**净放松**（其中一列真的丢了也不算缺）。
当前无害（抽取侧任何缺失都是 `mustMatch` 硬失败，不会只丢一列），但这条「无害」依赖的是
抽取路径的性质，不是本函数的性质。**建议**在函数注释里补一句这个依赖。

### [INFO] R1-7 · `tsfFlowTotalRE` 的「无生产调用方」已独立核实

**位置** `internal/hestia/profiles.go:531`，声称见 `extract.go:457`
**观察** 用 AST（`go/parser` 遍历顶层声明 + `ast.Ident` 使用计数）扫 `internal/hestia`：
**解析文件 生产 28 个 / 测试 29 个，顶层 func/var/const 声明 436 个**。
`tsfFlowTotalRE` 在生产文件里共 6 处，逐处看过上下文：**4 处是注释、1 处是声明本身、
1 处是 `allTemplateRegexps()` 的登记**；而 `allTemplateRegexps` 的唯一调用方是
`profiles_test.go:123`。⇒ **CONTRACTS 那笔债的描述属实。**
⚠️ 顺带：`tsfFlowArticleTotalRE`（真正在生产路径上的那条）**不在** `allTemplateRegexps()` 里
⇒ 不受 `TestNoGreedyCaptureInTemplates` 覆盖；`extract_test.go:574` 已就地自补检查并写明。

### [INFO] R1-8 · 代码质量总评

注释密度极高且**绝大多数是可核查的裁决记录**（写明「为什么不那样做」「删掉会怎样」「哪条测试守着」）。
我抽查的每一处「本注释声称 X」都能被实验证实（M10b、`bandLimitRatio` 的浮点论证、
`pickBandFor` 与 `pickCaliberBand` 的绑定断言、`sectorSegmentStart` 锚点项的惰性声明）。
错误信息一律**点名成因 + 给下一步动作**，不止步于「not found」。
`gateDepositSum` 的 `drift_skipped:insufficient_same_caliber_history (%s n=%d<%d, prior=%d)`
把「这一期没被保护」做成了**可数可查**的一行，是本轮设计上最好的一处。

**发现 8 条（CRITICAL 0 / MAJOR 1 / MINOR 3 / INFO 4）。**
CRITICAL 为 0 —— 查过的范围：全部 9 个非测试源文件的逐行 diff、`configs/hestia.yaml` 全部 152 行改动、
8 个变异体、4 组真语料 A/B、1 次老库 schema 实验、1 次 AST 死代码扫描。
