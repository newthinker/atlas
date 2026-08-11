# TASK-004 验证报告 — 数值原语 parseAmount

- **验证者**：test-agent-23（Reality Checker，默认 NEEDS WORK）
- **被验对象**：commit `65a4db00c65920e49ade129a9c0afaf638fc6837`（`internal/hestia/amount.go` 213 行 + `amount_test.go` 522 行，2 files / 735 insertions）
- **验证环境**：隔离 worktree `../wt-verify-TASK-004 @ 65a4db0`（detached，验毕移除）
- **assignment_epoch**：1（裁决携带 `--expect-epoch 1`）
- **判定：PASS → `verified`**
- **⚠ 附带一条 DoD 缺陷告警**：`boundary[0]` 的前提是**事实错误**，其字面判据不可实现。详见 §3。

---

## 0. 判定依据摘要

| 判据 | 实测值 | 门槛 | 结论 |
|---|---|---|---|
| 任务子集测试 | **76 `--- PASS`** / 0 FAIL | 全绿 | PASS |
| 全包 `go test -count=1` | **222 `--- PASS`** / 0 FAIL | 无回归 | PASS |
| `go vet` | exit **0** | 绿 | PASS |
| `go test -race -count=2` | ok | 绿 | PASS |
| amount.go 函数覆盖率 | **10 个函数全部 100.0%** | — | PASS |
| 包语句覆盖率 | **92.8%** | dev_minimum 80 | PASS |
| DoD 覆盖 | 8 条中 7 条直接 PASS；`boundary[0]` 实质 PASS（见 §3） | 逐条 | PASS |
| 变异测试（**我自设 16 体**） | **15 KILLED / 1 SURVIVED** | 存活体须可解释 | PASS |

**本报告所有数字均由我在隔离 worktree 实跑得出。对 dev 最关键的那条主张（推翻 DoD），我用自写探针独立复核，未采信它的任何一个测试。**

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | `newAmount` 方向词读符号、`toYi`/`toWanYi` 换算；**多增/少增/少减 必须报错** | `TestNewAmountSignDimensionOnly`、`TestNewAmountScaleDimensionOnly`、`TestNewAmountCombinesSignAndScale`、`TestNewAmountRejectsComparativeDirections`、`TestComparativeSentenceDoesNotMatchDirectionTemplate` | **PASS** |
| functional[1] | `parseRatio(dir,num)` 百分数、`parsePlainNumber(num)` 裸数字 | `TestParseRatio`、`TestParseRatioRejects`、`TestParsePlainNumber` | **PASS** |
| functional[2] | `directionAlt`/`unitAlt` 为可嵌入片段常量，T5 复用、不得第二份真相源 | `TestDirectionAltAndUnitAltAreEmbeddable`（用 T5 真实拼法 `QuoteMeta(名称)+方向+数字+单位` 跑 4 条真实样本句）、`TestDirectionSignCoversDirectionAlt`、`TestUnitScaleCoversUnitAlt` | **PASS** |
| boundary[0] | 「净融资」须排「融资」前；判据=写反顺序则「企业债券净」漏字段 | **前提经我独立实测证伪**；实质后果由 `TestGreedyNameCaptureSwallowsNetPrefix` 钉住，不变量由 `TestNoAlternativeIsPrefixOfAnother` 守 | **实质 PASS，DoD 缺陷（§3）** |
| error_handling[0] | 未知方向词报错，**错误信息含该词原文** | `TestNewAmountRejectsUnknownDirection`（6 个用例含空串，用 `strconv.Quote` 断言） | **PASS** |
| error_handling[1] | 单位为空串或表外值即报错 | `TestNewAmountRejectsUnknownUnit`（6 个用例含 `""`、`亿`、`万亿`） | **PASS** |
| error_handling[2]（G6） | 独立恒正入口；**禁止字面量方向词**；静态扫描 `newAmount("` 零命中 | `parsePlainAmount(num, unit)` + `TestNoLiteralDirectionWordAtProductionCallSites`（自带 `scanned != 0` 自证） | **PASS** |
| non_functional[0] | 符号与量纲**各自独立可测**，互不掩盖 | `TestNewAmountSignDimensionOnly`（断言未换算的 `a.value`）/ `TestNewAmountScaleDimensionOnly`（断言 `math.Abs(...)`，不经符号表） | **PASS** |

无空洞断言。零 mock——测试直接吃真实样本句与真实词表。

---

## 2. 独立变异测试（我自设 16 体）

不复用 dev 的脚本；判定变异是否生效用 Python `difflib` **直接比对源文件**，不经 git。

| 变异 | 判定 | diff | vet | PASS/76 | 杀手 |
|---|---|---|---|---|---|
| M1 符号表翻转（增加=−1,减少=+1） | KILLED | 4 | 0 | 65 | TestNewAmountSignDimensionOnly |
| M2 单位倍率改错（万亿元=1000） | KILLED | 2 | 0 | 66 | TestNewAmountScaleDimensionOnly |
| M3 多增/少增/少减 加进白名单（两处同改） | KILLED | 2 | 0 | 70 | TestNewAmountRejectsComparativeDirections |
| M4 未知方向词默认为正 | KILLED | 3 | 0 | 65 | 同上 |
| M5 未知/空单位默认亿元 | KILLED | 2 | 0 | 67 | TestNewAmountRejectsUnknownUnit（`""` 子测试） |
| M6′ 允许数字自带正负号 | KILLED | 2 | 0 | 67 | TestNewAmountRejectsSignedNumber |
| M7′ 允许 NaN/Inf | KILLED | 2 | 0 | 65 | TestNewAmountRejectsNonFiniteNumber |
| M8 零值 amount 返回 0 而非 panic | KILLED | 3 | 0 | 75 | TestAmountZeroValueRefusesToScale |
| M9 parsePlainAmount 委托给 `newAmount("增加",…)` | KILLED | 9 | 0 | 75 | **TestNoLiteralDirectionWordAtProductionCallSites** |
| M10′ 生产文件插入字面量 `newAmount("` | KILLED | 2 | 0 | 75 | 同上 |
| **M11 交替顺序整个颠倒** | **SURVIVED** | 2 | 0 | 76 | —（**设计上不可杀，见 §3**） |
| M12 `toWanYi` 除以 1000 | KILLED | 2 | 0 | 67 | TestNewAmountScaleDimensionOnly |
| M13 「净融资」符号改 −1 | KILLED | 2 | 0 | 72 | TestNewAmountSignDimensionOnly（净融资 子测试） |
| M14 directionAlt 删掉「净融资」 | KILLED | 2 | 0 | 72 | TestDirectionAltAndUnitAltAreEmbeddable（企业债券 子测试） |
| M15 生产文件插入字面量 `parseRatio("` | KILLED | 2 | 0 | 75 | TestNoLiteralDirectionWordAtProductionCallSites |
| M16 directionSign 多登记一词（与 directionAlt 脱节） | KILLED | 1 | 0 | 71 | TestNewAmountRejectsComparativeDirections |

**合计 15 KILLED / 1 SURVIVED / 16 体。有效体 `go vet` 全部 exit=0——无一靠编译错误假杀。**

### 我自己的 harness 踩了一次假结果，被四条自证挡住

M6/M7 第一版用 `if false {` 做变异，导致 `strings` / `math` 的 import 被孤立 ⇒ **`go vet` 红 ⇒ 编译失败**。按第二条自证判为 **INVALID 而非 KILLED**（若只看「有测试失败」会误记两个假杀）。改成保持 import 被使用的恒假条件（`strings.HasPrefix(num, "\x00")` 与 `math.IsNaN(v) && math.IsInf(v,0)`，二者不可能同真）后重跑，两体均 KILLED。

**这与 dev 在 TASK-002 自报的 harness 静默失效同源**：变异 harness 本身就是会出错的软件，「无 SURVIVED」永远不能直接当满分。

---

## 3. 核心发现：`boundary[0]` 是 DoD 缺陷，dev 的推翻成立

### DoD 的断言

> 机制是 **RE2 取首个可匹配而非最长匹配**，故 `净融资` 必须排在 `融资` 之前。
> 判据：构造一个交替顺序写反的实现（`融资` 在前），断言「企业债券净」匹配不上任何清单项 ⇒ **静默漏一个字段**。

### 我的独立复核结论：**前提错误，该判据恒绿，是一条假防线**

我**没有运行 dev 的 `TestAlternationOrderHasNoEffect`**，而是自写探针，对 **4 种模板形态 × 5 条真实句 = 20 组**做正序/逆序对照：

| 模板形态 | 正序 vs 逆序 |
|---|---|
| A 无锚点 `FindString` `(alt)` | 逐字节相同 |
| B 非贪婪名称捕获 `^(.+?)(alt)…$` | 逐字节相同 |
| C 贪婪名称捕获 `^(.+)(alt)…$` | 逐字节相同 |
| D 固定名称 `QuoteMeta("企业债券")(alt)…` | 逐字节相同 |

**输出不同的用例数 = 0。** 我的 M11（顺序整个颠倒）也如实 SURVIVED。

**根因**（我独立推导并实测确认）：Go 的 `regexp` 是 **leftmost-first**。交替顺序只在两个分支能在**同一起始位置**都匹配时才有语义——也就是**一个是另一个的前缀**时。而「融资」是「净融资」的**后缀**，不是前缀：在「净」那个位置上，「融资」压根匹配不上，引擎只能选「净融资」。DoD 把「RE2 取首个可匹配」这条正确的机制，错误地套到了一对**后缀**关系的词上。

⇒ 按 DoD 字面去写测试，**无论实现对错都会绿**。它不是「没写」，而是**不可能写成有意义的测试**。

### 我第一版探针也设计错了，必须记下

我最初用**全锚定** `^(.+?)(alt)([0-9.]+)(万亿元|亿元)$` 去验证「前缀理论」，得到「即使加入前缀词『增』，顺序仍无影响」——这与 dev 的理论矛盾。复查发现是**我的探针错了**：`$` 锚点加上必须匹配的数字段会**强制回溯**，短词「增」匹配后无法收尾，引擎回退再试「增加」，从而掩盖了顺序敏感性。

改用**无锚点**形态后差异立刻显现：

| 形态 | `增加\|增长\|增`（长词在前） | `增\|增加\|增长`（短词在前） | 顺序有语义 |
|---|---|---|---|
| 无锚点 `(alt)` | `增加` | **`增`** | **是** |
| `^(.+?)(alt)` | `委托贷款 / 增加` | **`委托贷款 / 增`** | **是** |
| `^(.+?)(alt)([0-9.]+)`（无 `$`） | `增加` | `增加` | 否（回溯救回） |
| 全锚定 `^…$` | `增加` | `增加` | 否（回溯救回） |

⇒ **前缀理论成立**，`TestNoAlternativeIsPrefixOfAnother` 是**真守卫**而非摆设：一旦有人往 `directionAlt` 加「增」或往 `unitAlt` 加「亿」「万亿」，顺序立刻开始决定语义，该测试会转红。当前词表两两前缀扫描：`directionAlt` **0 命中**、`unitAlt` **0 命中**，不变量当前成立。

### 真正会漏掉「企业债券」的失效模式：贪婪名称捕获

我独立复现（`企业债券净融资2.39万亿元`）：

| 名称捕获 | 正序 | 逆序 | name |
|---|---|---|---|
| 贪婪 `(.+)` | 漏 | 漏 | **`企业债券净`** ⇒ 匹配不上任何清单项，**静默漏字段** |
| 非贪婪 `(.+?)` | 正常 | 正常 | `企业债券` |

**与交替顺序无关，两种顺序下都发生。** 这正是 DoD 想防的后果，只是把原因归错了。dev 的 `TestGreedyNameCaptureSwallowsNetPrefix` 断言的就是 DoD 字面要求的那句「『企业债券净』匹配不上任何清单项」——**DoD 的后果判据被满足了，被归错的只是成因**。

### 为什么判 PASS 而不是 `rejected --reason_class=dod_defect`

- 实质风险（企业债券字段静默丢失）**已被钉住**，且是被**真正会导致它的机制**钉住的；
- dev 交付的三条测试（顺序无效的实测记录 + 前缀不变量 + 贪婪失效演示）**严格强于** DoD 字面要求；
- 退回无 rework 可做（DoD 字面判据不可实现），只会白涨 `rework_count`。

**⇒ 建议 Leader 修订 `boundary[0]` 措辞**，改为两条可测判据：
1. 名称捕获**禁用贪婪 `(.+)`**，必须用 `QuoteMeta(固定名称)` 或非贪婪 `(.+?)`——**这是给 T5 的硬约束**；
2. 交替片段内**任意两项互不为前缀**（前缀出现时才需长词在前）。

---

## 4. G6（`error_handling[2]`）复核：防线真的合上了

DoD 指出需求文档在约四分之一的调用点上把方向词校验**结构性关闭**（余额/存量句硬编码字面量「增加」）。dev 的处置与我的复核：

- 提供独立命名恒正入口 `parsePlainAmount(num, unit)`，与 `newAmount` 共用 `checkUnit` + `parseUnsignedNumber`；
- `TestNoLiteralDirectionWordAtProductionCallSites` 扫描包内全部**非** `_test.go` 文件，`newAmount("` 与 `parseRatio("` 必须零命中；
- **自证到位**：`require.NotZero(t, scanned)`——扫到 0 个文件时「零命中」毫无意义，这条防止了空绿。

**我用三个变异独立确认它不是摆设**：M9（把 `parsePlainAmount` 改成委托 `newAmount("增加",…)`）、M10′（生产文件插入字面量 `newAmount("`）、M15（插入字面量 `parseRatio("`）**全部被这条测试杀死**。

---

## 5. `non_functional[0]` 复核：两维度确实互不掩盖

DoD 要求「方向词错误不应因单位正确而被掩盖，反之亦然」。dev 的做法不是各写一组用例，而是让**断言对象本身不经过另一张表**。我直接实测两个方向：

| 变异 | `TestNewAmountSignDimensionOnly` | `TestNewAmountScaleDimensionOnly` |
|---|---|---|
| M1 符号表翻转 | **FAIL** | PASS |
| M2 单位倍率改错 | PASS | **FAIL** |

**完全隔离，判定成立。** 另有 `TestNewAmountCombinesSignAndScale` 接住「各自对、合起来乘反了」。

---

## 6. 其他值得记录的质量点（抽验属实）

- **`assert.Contains(err, "")` 恒真陷阱**：`error_handling[0]` 的用例集含空串方向词，若直接 `Contains(err.Error(), dir)` 则该用例**恒真等于没测**。dev 改用 `strconv.Quote(dir)` 断言 `%q` 形态（`""`），是真检查。这是 dev 自己发现并修掉的假绿断言。
- **NaN/Inf 拒收**：`strconv.ParseFloat` 接受 `"NaN"`/`"Inf"`/`"Infinity"` 且不报错；NaN 落进 `Values` 后恒等式与量级区间两类闸门**全部放行**（NaN 参与的比较一律 false）。M7′ 确认有守护。
- **零值 amount 拒绝换算**：`scaleOf` 对表外单位 panic 而非返回 map 零值 0.0——一个「看起来合理的 0」静默流进 Values 是下游接不住的。M8 确认有守护。
- **数字自带符号一律拒收**：把「符号只来自方向词」钉在**原语**上，而非寄望 T5 的 `numPat` 恰好捕不到负号。M6′ 确认有守护。

---

## 7. 声明范围核对

- `writes` 声明：`./internal/hestia/amount.go`、`./internal/hestia/amount_test.go`
- `git show --stat 65a4db0` 实际：**恰为这两个文件**，735 insertions
- **无越界申报。**
- **零漂移**：`verify_baseline.head = 65a4db0` 与验证时主仓库 HEAD **同值**；`discovery_sha256 = 7b9707c0…c1354` 与当前 discovery 文件一致。

---

## 8. 转告下游与 Leader 的事项（均不构成退回理由）

1. **给 T5 的硬约束（最重要）**：模板里的名称捕获**绝不能用贪婪 `(.+)`**，必须 `QuoteMeta(固定名称)` 或非贪婪 `(.+?)`；否则「企业债券净融资」会被切成 `企业债券净` 而**静默漏字段**。
2. **给 T5 的币种约束**：`amount` 保留原始 unit 字符串（`万亿美元` ≠ `万亿元`），但**换算后的浮点丢币种**——`1.07万亿美元.toWanYi()` 与 `1.07万亿元.toWanYi()` 完全相等。⇒ T5 的 DoD「断言捕获组而非最终值」**用现有接口今天就能满足**（断言 `m[unit组] == "万亿元"`），无需改接口。是否新增 `currency()` 访问器属**新任务**，由 Leader 定夺；不建议为此退回 T4。
3. **M1b-1 文档缺口**：`fields.go:6-12` 的单位约定只列三类，**不含** `fx_reserve` 的「万亿美元」与 `fx_rate` 的「元/美元」。建议补第四类。
4. **包级命名空间**：`golden_test.go` 已占 `abs`、`nonTSFFields`；`amount.go` 未定义 `abs`（用限定名 `math.Abs`），当前无冲突。T5/T6 写 `extract.go`/`parse.go` 时需避开。
5. **`numPat` 分居两处**：本任务未声明 `numPat`（在 T5 的 `profiles.go`），测试用私有 `numPatForTest`。「正则捕得到什么」与「原语接受什么」分居两文件，已由原语侧的拒绝规则兜住，但 T5 放宽 `numPat` 时需同步复查。

---

## 9. 结论

**PASS → `verified`。**

八条 DoD 中七条直接覆盖且有具体断言；`boundary[0]` 的实质风险被更强的测试钉住，其字面判据经我独立实测证明**不可实现**（属 DoD 缺陷，已建议修订措辞）。76/222 测试全绿、amount.go **10 个函数 100% 覆盖**、我自设 16 个变异体 **15 杀 1 存**且有效体 vet 全部 exit=0，唯一存活体经独立证明为**设计上不可杀**。

dev 对 DoD 的公开质疑经我用独立探针复核**完全成立**，且它没有停在「顺序无所谓」，而是把真正的失效模式（贪婪捕获）与真正的不变量（两两非前缀）都补上了测试——这比照字面实现一条恒绿的假防线要好得多。
