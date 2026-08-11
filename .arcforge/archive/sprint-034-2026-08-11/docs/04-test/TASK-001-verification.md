# TASK-001 验证报告 —— golden 期望值表（手工抄录）

- **验证者**: test-agent-22
- **验证对象**: `internal/hestia/golden_test.go` @ commit `d6a1f59e63ddcc98722f8f0eebbbbbf33db09bf4`
  （blob sha `eaa431fdce04c88ef0467635663ddda6dab8b137`，374 行）
- **承接时 assignment_epoch**: 1
- **验证环境**: 隔离 detached worktree，锚 `d6a1f59e63ddcc98722f8f0eebbbbbf33db09bf4`
  （主工作区当时 HEAD 已前移至 `acc5a96`，含 T2 的 strip.go 与 T4 在途的 amount.go；
  隔离是为保证「golden 在 T2–T6 全未完成时可独立编译」这条 DoD 的证据不被污染）
- **判定**: **PASS → verified**

---

## 一、基线计数（自证）

在隔离 worktree @ `d6a1f59`：

| 命令 | 结果 |
|---|---|
| `go vet ./internal/hestia/` | exit 0，无输出 |
| `go test ./internal/hestia/ -v -count=1` | **123 PASS / 0 FAIL / exit 0** |
| `go test ./internal/hestia/ -run TestGolden -v -count=1` | **11 PASS / 0 FAIL**（3 顶层 + 8 子测试） |

> 注：discovery 自述「含 9 个子测试」，实测为 8 个（4+2+2）。不影响结论，但属自述与事实的偏差，记录在案。

---

## 二、完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 变异实证 | 判定 |
|---|---|---|---|---|
| functional[0]a | `golden2025` 恰 54 项、`golden2020` 恰 27 项 | `TestGoldenTablesAreWellFormed`（`require.Len`） | M1 删 `FieldM0YoY` → KILLED（`should have 54 item(s), but has 53`） | **PASS** |
| functional[0]b | `goldenMeta*` 按 M1b-1 `Meta` 填 5 项，`ArticleID`/`IngestedAt` **留空** | `TestGoldenMetaIsValid` | M4 填入 ArticleID → KILLED（`Should be empty, but was 2026011509294440745`） | **PASS** |
| functional[1] | 每个键在 `allFields` 内，且**逐键**断言 | `TestGoldenTablesAreWellFormed` 的 4 个子测试 | M3 `FieldRateIBO`→`FieldTSFStock`（项数仍 27）→ KILLED，精确报 `does not contain "rate_ibo"` | **PASS** |
| boundary[0] | 2020 只有 27 项**是预期的**，须注释写明非抄漏并指出对应 6 板块无社融 | 子测试「2020/恰好是全部非社融字段」双向断言（该有的不少 + 不该有的不多）；注释见 `golden_test.go` `golden2020` 声明前 | 同 M3 | **PASS** |
| error_handling[0] | 单位须与 M1b-1 约定一致；原文单位不同须换算并注释原文值 | `TestGoldenUnitsMatchFieldClass` | M2 `tsf_flow_ytd` 356000→35.6 → KILLED（`"35.6" is not greater than or equal to "100"`） | **PASS** |
| non_functional[0]·人工 | **期望值手工抄自原文，严禁由解析器生成**；DoD 明示「无法用测试守住」，靠验证者抽查 | 无（按 DoD 设计即无） | 见第三节：验证者独立**全量**核对 81/81 项 | **PASS** |
| non_functional[0]·机械 | golden 须先于全部实现落盘且**此后不再被修改** | `git log --diff-filter=M` + blob sha | 见第四节 | **PASS** |
| non_functional[1] | 不依赖任何本 Sprint 新增函数（`stripHTML`/`parseAmount` 等） | 「本文件能编译」即证据 | 在无 strip.go/amount.go 的 `d6a1f59` worktree 中编译并全绿；grep 确认无相关引用 | **PASS** |

---

## 三、non_functional[0] 的人工抽查——**全量核对，非抽样**

任务要求「随机抽查至少 8 项」。因 81 项规模可控，**改为全量逐条核对**，且核对的是**三层**而非一层：

1. **源码字面量** ← 读 `golden_test.go` 全文
2. **运行时实际值** ← 借变异 M1/M3 失败信息中 testify 打印的完整 map dump，拿到两表的**运行时**键值全集
3. **原文 HTML** ← `awk` 带真实行号读取，逐句比对

第 2 层是关键：它排除了「注释说 A、字面量写 B」以及「字面量正确但常量绑错」的可能——运行时 dump 给出的是最终生效的 `字段名: 值`。

### 3.1 结果

- **2025 样本 54/54 项全部与原文一致**，0 分歧
- **2020 样本 27/27 项全部与原文一致**，0 分歧
- **注释标注的原文行号 100% 正确**（2025: L320/L323/L325/L327/L328/L331/L332/L334/L336/L338；2020: L320/L322/L323/L326/L327/L331/L333）

### 3.2 单位换算项逐条核对（DoD 点名的高风险类）

万亿元 → 亿元，换算因子 ×10000：

| 字段 | 原文 | golden | 核算 |
|---|---|---|---|
| `tsf_flow_ytd` | 35.6 万亿元 | 356000 | ✓ |
| `tsf_flow_rmb_loan_ytd` | 15.91 万亿元 | 159100 | ✓ |
| `tsf_flow_corp_bond_ytd` | 2.39 万亿元 | 23900 | ✓ |
| `tsf_flow_govt_bond_ytd` | 13.84 万亿元 | 138400 | ✓ |
| `deposit_flow_ytd` (2025) | 26.41 万亿元 | 264100 | ✓ |
| `deposit_household_ytd` (2025) | 14.64 万亿元 | 146400 | ✓ |
| `deposit_corp_ytd` (2025) | 2.31 万亿元 | 23100 | ✓ |
| `deposit_nbfi_ytd` (2025) | 6.41 万亿元 | 64100 | ✓ |
| `loan_flow_ytd` (2025) | 16.27 万亿元 | 162700 | ✓ |
| `loan_hh_mlt_ytd` (2025) | 1.28 万亿元 | 12800 | ✓ |
| `loan_corp_total_ytd` (2025) | 15.47 万亿元 | 154700 | ✓ |
| `loan_corp_short_ytd` (2025) | 4.81 万亿元 | 48100 | ✓ |
| `loan_corp_mlt_ytd` (2025) | 8.82 万亿元 | 88200 | ✓ |
| `loan_bill_ytd` (2025) | 1.66 万亿元 | 16600 | ✓ |
| `deposit_flow_ytd` (2020) | 14.55 万亿元 | 145500 | ✓ |
| `deposit_household_ytd` (2020) | 8.33 万亿元 | 83300 | ✓ |
| `deposit_corp_ytd` (2020) | 5.28 万亿元 | 52800 | ✓ |
| `loan_flow_ytd` (2020) | 12.09 万亿元 | 120900 | ✓ |
| `loan_hh_mlt_ytd` (2020) | 2.8 万亿元 | 28000 | ✓ |
| `loan_corp_total_ytd` (2020) | 8.77 万亿元 | 87700 | ✓ |
| `loan_corp_short_ytd` (2020) | 2.82 万亿元 | 28200 | ✓ |
| `loan_corp_mlt_ytd` (2020) | 4.86 万亿元 | 48600 | ✓ |

**同句中单位本就是亿元、不得换算**的混排项也核对无误：
`deposit_fiscal_ytd` 6579（2025）/ 4384（2020）、`tsf_flow_fx_loan_ytd` -2043、
`tsf_flow_entrust_ytd` 1203、`tsf_flow_trust_ytd` 3682、`tsf_flow_bankaccept_ytd` 112、
`tsf_flow_equity_ytd` 4763、`loan_hh_short_ytd` -8351（2025）/ 7552（2020）、
`loan_nbfi_ytd` -1103（2025）/ -2775（2020）、`loan_bill_ytd` 9697（2020）、
`deposit_nbfi_ytd` -7446（2020）。

`deposit_fiscal_ytd` 尤其值得点名：它夹在四个万亿元分项中间，原文单位却是亿元。抄录正确。

### 3.3 符号（增/减）核对

原文用「下降/减少」的 8 项，golden 全部取负：
`tsf_stock_fx_loan_yoy` -18、`tsf_stock_bankaccept_yoy` -0.3、`tsf_flow_fx_loan_ytd` -2043、
`loan_hh_short_ytd` -8351（2025）、`loan_nbfi_ytd` -1103（2025）/ -2775（2020）、
`deposit_nbfi_ytd` -7446（2020）。✓

### 3.4 独立反向审计：「干扰值未混入」

dev 做的一次性审计方向是「golden 值能否在原文中找到」。验证者补了**反方向**的独立检查：
原文中紧邻正确值的 21 个陷阱值**不得**出现在 golden 中。

- 检查项：2025 的 336.14 / 275.74 / 4417 / 11300 / 33400 / 13100 / 1873 / 1780 / 3405 / 60.7 / 2180.31；
  2020 的 212.99 / 171.32 / 35600 / 24200 / 18100 / 45000 / 29000 / 2270 / 669.11
- **结果：命中 0**
- **harness 自证**（两道，防「静默失效式的假绿」）：
  1. 提取器首版正则 `Field[A-Za-z]+` **漏掉了 12 条**含数字的字段名（`FieldM2`/`FieldM1`/`FieldM0` 及 YoY），
     只抓到 69/81。改为 `Field[A-Za-z0-9]+` 后为 **81/81**。若不做这道自证，「0 命中」将是假绿。
  2. 阳性对照：用同一 `check` 函数查 `442.12`（确在表中）→ 正确报 `HIT`，证明检查函数会触发。

### 3.5 三条 dev 报告的原文事实——**全部复核属实**

1. **2025 L336 正文写「质押式回购加权`均`利率为1.4%」，缺「平」字**；同篇小标题 L334 写的是
   「质押式债券回购月加权`平均`利率为1.4%」；2020 L331 正文有「平」（「质押式回购加权平均利率为1.89%」）。
   → **T5 若按「加权平均利率」死锚正文，会在 2025 篇静默漏掉 `rate_repo`。属实且重要。**
2. **2020 混用半角标点属实**：L320 有两处半角逗号（`213.49万亿元,同比` 与 `7.95万亿元,同比`，
   即 M2 与 M0，M1 那处是全角）；L323 `票据融资增加9697亿元;非银行业金融机构贷款减少2775亿元`
   用**半角分号**，而分号正是住户/企业的作用域边界。→ T5 分句正则须同时容忍全角与半角。
3. **人民币/本外币口径属实**：2025 人民币 328.64/271.91 vs 本外币干扰 336.14/275.74；
   2020 人民币 207.48/165.2 vs 本外币干扰 212.99/171.32。golden 全部取人民币口径。
   **作用域措辞差异属实**：2020 是「住户`部门`贷款增加3.56万亿元」，2025 是「住户贷款增加4417亿元」。

### 3.6 Meta 核对

`<meta name="PubDate">` 均在两份 HTML 的 **L18**：2025 为 `2026-01-15`、2020 为 `2020-07-10`，与 golden 一致。
`Period`/`PeriodType` 与报告标题一致（`2025年金融统计数据报告`/annual、`2020年上半年金融统计数据报告`/h1），
且满足 `types.go` 的 `periodEndMonth` 约束（annual→12、h1→06）。
`CaliberVersion`：2020 取 `2015-01` 有原文 L339 注2「自2015年起…」直接支撑；2025 取 `2025-01` 对应 L345 注5。
`ArticleID` 与 `IngestedAt` 均为空，符合 functional[0]。

---

## 四、non_functional[0] 机械化那一半

```
git log --diff-filter=M --oneline -- internal/hestia/golden_test.go   → 空
git log --diff-filter=A --oneline -- internal/hestia/golden_test.go   → d6a1f59（唯一）
git rev-parse d6a1f59:internal/hestia/golden_test.go  → eaa431fdce04c88ef0467635663ddda6dab8b137
git hash-object internal/hestia/golden_test.go        → eaa431fdce04c88ef0467635663ddda6dab8b137
```

d6a1f59 之后已有一次实现落地（`acc5a96 feat(hestia): add HTML stripping…`），
golden_test.go 在该 commit 后仍**逐字节未变**。

**commit 范围核对（越界申报）**：`git show --stat d6a1f59` = `1 file changed, 374 insertions(+)`，
恰为申报的 `writes: ["./internal/hestia/golden_test.go"]`，`packages: ["./internal/hestia"]`。
**无越界改动，无他人产物混入。** testdata 由更早的 `87acf57` 引入，非本任务。

### T7 复核基线（请下游沿用）

| 判据 | 值 |
|---|---|
| golden_test.go blob sha | `eaa431fdce04c88ef0467635663ddda6dab8b137` |
| 引入 commit | `d6a1f59e63ddcc98722f8f0eebbbbbf33db09bf4` |
| 修改记录 | 无 |

> **建议 T7 用 blob sha 而非仅用 `--diff-filter=M`**：后者在 rebase / `commit --amend`
> 重写历史后会连同修改记录一起消失，从而给出假绿；blob sha 比对不受历史重写影响。

---

## 五、变异实证汇总

每条变异均附四条自证：diff 非空 / 首行完整（`package hestia`）/ `go vet` 红绿都查 / `--- PASS` 计数与基线比较。

| 编号 | 变异 | diff | 首行 | vet | golden 子集 PASS（基线 11） | 全包 PASS（基线 123） | 判定 |
|---|---|---|---|---|---|---|---|
| M1 | 删 `FieldM0YoY: 10.2` | 1 deletion | 完整 | exit 0 | **6** | **118** | KILLED |
| M2 | `tsf_flow_ytd` 356000→35.6 | 1+/1- | 完整 | exit 0 | **9** | **121** | KILLED |
| M3 | `FieldRateIBO`→`FieldTSFStock`（2020，项数不变） | 1+/1- | 完整 | exit 0 | **9** | **121** | KILLED |
| M4 | `goldenMeta2025` 填 `ArticleID` | 1 insertion | 完整 | exit 0 | **9** | **121** | KILLED |
| M5 | `deposit_flow_ytd` 264100→264200 | 1+/1- | 完整 | exit 0 | 11（=基线） | 123（=基线） | **SURVIVED** |
| M6 | `tsf_stock` 442.12→442.21 | 1+/1- | 完整 | exit 0 | 11（=基线） | 123（=基线） | **SURVIVED** |

**KILLED 的因果性均已核对**：失败测试与变异点直接对应，非连带伤害——
M1 报 `should have 54 item(s), but has 53`（`golden_test.go:260`）；
M2 报 `"35.6" is not greater than or equal to "100"`（`:326`，单位守护）；
M3 报 `does not contain "rate_ibo"`（`:290`，逐键断言）；
M4 报 `Should be empty, but was …`（`:367`）。
M1 的 PASS 从 11 掉到 6 是因为 `require.Len` 失败即 `FailNow`，后续子测试不再运行——
这是 testify 的正常行为，非额外破坏。

**一次假 SURVIVED 已被自证拦下**：M3 首次施加时 sed 行号范围写错，**diff 为空**，
脚本仍会打出「SURVIVED」。因强制打印 diff 而当场识破，修正后重跑得 KILLED。
这正是「diff 非空」自证的价值所在。

**M5/M6 的 SURVIVED 是设计内的、且 dev 已如实声明**：`TestGoldenUnitsMatchFieldClass`
只拦量级，拦不住同量级抄错数字。M5 变异后连注释 `// 26.41 万亿` 都与值 264200 自相矛盾，
仍无任何测试转红 —— **这从反面证明了 non_functional[0] 的人工抽查是唯一防线，而不是冗余。**

---

## 六、对 dev 三处取舍的判断

### 6.1 抄录审计脚本一次性执行、不做成常驻测试 —— **取舍成立**

dev 的理由：对小整数会平凡命中（`yoy=6`、`flow=112` 在 HTML 中必然出现），
常驻会给出高于实际的信心。**认同，且理由比 dev 自己说的更强**：

- 该审计只做「值 → 原文中存在」的单向存在性检查，**不检查值绑在哪个字段上**。
  M3 那类「值对、键错」的变异它一条都抓不到，而那正是 DoD functional[1] 点名的失败模式。
- 常驻后它会长期显示绿色，诱导后续验证者把「抄录正确性」记作已被机器覆盖，
  从而跳过人工抽查 —— 而人工抽查是 DoD 明示的唯一手段。这与 dev 在文件头警告的
  「不要把 `TestFieldNamesAppearOnlyInFieldsGo` 记作本文件的守护」是同一类失效。

结论：**不常驻是正确的**。dev 把它作为一次性审计执行、把结论写进 discovery 供抽查，处置得当。

### 6.2 `TestGoldenUnitsMatchFieldClass`（DoD 未点名）—— **建议保留**

- DoD `error_handling[0]` 点名了「照搬原文单位 → T7 才红、根因在这里」这个失败模式，
  该测试正对着它，**不属于范围外的自作主张**。
- M2 变异证明它有效；M5/M6 变异证明它的边界正如 dev 注释所写。
- **边界在注释里写明了，没有假装是完整守护** —— 这是关键。
  一条自称「拦不住 X」的测试不会诱导下游误判覆盖度。

唯一顾虑：阈值 `_ytd ≥ 100` 依赖「全样本最小增量是 112 亿元」这一**当前样本事实**。
若将来某期次出现 <100 亿元的真实增量，它会误报。注释已写明该常数来源，可接受；
**建议在注释里补一句「此阈值随样本扩充需复核」**（非阻塞，留给后续任务）。

### 6.3 `golden2020` 用 `nonTSFFields()` 而非写死 27 —— **认同**

写死 27 会在 schema 增删字段时变成一条说谎的断言。
同时 `require.Len(golden2020, 27)` 仍保留了字面 27 —— 两者互为交叉校验（一处漏改会红），
不是重复。

---

## 七、发现的问题（不构成退回）

### P1（中）`abs` 与 T4 的 `amount.go` 有编译冲突风险

`golden_test.go:342` 声明**包级** `func abs(f float64) float64`。Go 在构建测试二进制时
把 `_test.go` 与非 test 文件并入同一 package —— 因此 T4 若在 `amount.go` 里加同名
`func abs`（对一个金额解析文件而言是极自然的辅助函数），**整个 package 编译失败**。

- 验证时点（主工作区 HEAD `acc5a96` + 未跟踪的 `amount.go`）**尚未发生冲突**，仅为潜在风险。
- 值得告知的原因：冲突一旦发生，报错现场在 T4，**根因却在 TASK-001 已冻结的文件里**，
  而该文件按 DoD 不应再被修改 —— 届时只能由 T4 改名，需提前知会。
- 同理适用于 `nonTSFFields()`（冲突概率低得多）。

### P2（中）`CaliberVersion` 的选取规则无 spec 明文

2025 年报同时含注4（2023-01 三类机构纳入统计）与注5（2025-01 M1 口径修订）。
dev 取**较新**的 `2025-01`，理由写在注释里（`period ≥ 2025-01`）。
判断：**这个选择是对的**（2025-12 的数据确实处于 2025-01 口径下，它蕴含 2023-01 的调整），
但「多条口径注同时适用时取最新」这条规则在 M1b-1 的 `types.go` 与 CONTRACTS.md 中都**没有写**。
T5/T6 若实现成「取首个匹配」或「按注释出现顺序」，T7 会红而根因是规则未定义。
**建议 Leader 在 T5/T6 的 DoD 里明确这条规则。**

### P3（低）`fields.go` 单位约定未覆盖两个 FX 字段

`fields.go:6-12` 的单位约定只有三类（余额=万亿元 / 增量=亿元 / 比率=百分数），
而 `fx_reserve` 是**万亿美元**、`fx_rate` 是**元/美元**，两者都不属于任何一类。
golden 忠实抄了原文（3.36 / 7.0288、3.11 / 7.0795），是正确的；
`TestGoldenUnitsMatchFieldClass` 也为 `fx_rate` 开了特例分支。
**这是 M1b-1 的文档缺口，由本任务暴露，不是本任务的缺陷。** 建议补进 `fields.go` 的约定注释。

### P4（极低）discovery 自述子测试数不准

discovery 的 `verification.tests` 写「含 9 个子测试」，实测 8 个。不影响任何结论。

---

## 八、结论

八条完成标准**逐条有证据支撑**，其中 5 条有变异实证的机器守护、
1 条（non_functional[0] 人工部分）经验证者**全量 81/81 独立核对**、
1 条（non_functional[0] 机械部分）经 blob sha 与 git 历史双重确认、
1 条（non_functional[1]）经隔离 worktree 编译证明。

自报与实测的偏差只有一处（子测试 8 vs 9），且 dev 对**三处**测试边界主动做了
如实且准确的声明（`TestFieldNamesAppearOnlyInFieldsGo` 不构成守护、
单位守护拦不住同量级抄错、审计脚本对小整数平凡命中）—— 三条经变异实证**全部属实**。
没有发现夸大覆盖度的表述。

**判定：PASS → `verified`**
