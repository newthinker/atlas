# TASK-017 验证报告 — `FiscalPeriod` 治本（标签冲突，发布阻断级）

- **验证者**: test-agent-7 (Reality Checker)
- **被验对象**: commit `89b1fd5` / package `./internal/collector/edgar`
- **assignment_epoch**: 1
- **判定**: ✅ **PASS (verified)** —— 附 2 项测试健壮性发现（§6），均**不影响功能正确性**
- **纪律**: 结论只锚定我本人实跑输出；未采信 dev-agent-15 自述。变异在隔离 worktree 内进行，
  **每个变异体先过 `go vet` 构建守卫**（编译失败不计为"被杀"），验完 `git worktree remove`。

---

## 1. 实跑证据

| 命令 | 结果 |
|---|---|
| `go test ./internal/collector/edgar/ ./internal/prism/ ./internal/prism/sankey/ -count=1 -cover` | 三包全 `ok`：**93.6%** / 95.5% / 97.5% |
| edgar 覆盖率 vs 基线 93.1% | **93.6%**，未回退 ✅ |
| `-count=20` | `ok`，无 flaky |
| `go vet` / `go build ./...` | 均通过 |

---

## 2. golden 变化：程序化核实（不靠肉眼看 diff）

DoD 与 leader 都给了「只许第一列变」的判据。我用脚本逐文件逐行逐列比对
`89b1fd5^` → `89b1fd5`：

```
golden 总数=9   本次变动=2  → [companyfacts_q4guard.txt, companyfacts_split.txt]
未变动 7 份: double_split / mini / nonsplit_jump / partial_fy / rev_fallback / rev_fpfy / shares_noq4
共 5 行发生变化；违反「只第 0 列变」的行数 = 0   ✓ 全部只动第一列
```
行数未变、列数未变、其余 7 字段逐字节不变。**无真漂移。**

### 2.1 ⚠ 但「只许第一列变」是必要非充分 —— 我另做了独立核对

leader 已指出该判据不检查第一列**内部**是否自洽。故我另行统计每份 golden 的
「标签重复」与「一个标签对应多个 period_end」：

```
新版本（89b1fd5）：9 份全部 —— 重复标签=无，一标签对多期末=无   ✓
旧版本（89b1fd5^）：9 份全部 —— 同样无冲突
```

**这里有一个必须点明的事实**：**旧 golden 本来就没有冲突**。
所以 golden 的标签唯一性 **不能作为「缺陷已修」的证据** ——
它只证明「没修坏」。这与 dev 的解释一致：既有 fixture 全是干净的单财年样本，
**根本不含「较晚报告重述较早期间」的形态**，这正是单测长期抓不到该缺陷的原因。
真正的判别力来自新增的 `companyfacts_fy_june.json` 与 `fiscalperiod_test.go`。

---

## 3. 推导逻辑：我手算复现，逐条对照 DoD 期望值

独立用 Python 复刻 `fiscalLabel` + `anchorYearMonth`，对照 DoD 给的 MSFT 对照表：

| DoD 冲突组 | 推导结果 | 期望 | |
|---|---|---|---|
| 2018Q4×4 | `[2016Q4, 2017Q1, 2017Q2, 2017Q3]` | 同左 | ✓ |
| 2019Q4×4 | `[2017Q4, 2018Q1, 2018Q2, 2018Q3]` | 同左 | ✓ |
| 2020Q4×4 | `[2018Q4, 2019Q1, 2019Q2, 2019Q3]` | 同左 | ✓ |
| 2025Q4×3 | `[2023Q4, 2024Q4, 2025Q4]` | 同左 | ✓ |
| 2026Q3×2 | `[2025Q3, 2026Q3]` | 同左 | ✓ |
| 2026Q2×2 | `[2025Q2, 2026Q2]` | 同左 | ✓ |
| 2026Q1×2 | `[2025Q1, 2026Q1]` | 同左 | ✓ |

**7/7 全部解开且值语义正确。** 另手算验证 DoD 点名的四个边界：
`2025-09-30→2026Q1`、`2026-06-30→2026Q4`、`2016-06-30→2016Q4`（含端点）、`2016-09-30→2017Q1`，全部相符。

---

## 4. 变异验证（全部带 `go vet` 构建守卫）

### 4.1 核心护栏：全部 load-bearing ✅

| 变异 | 结果 | 变红的测试 |
|---|---|---|
| 标签改回取自 EDGAR `fy/fp`（保留引用） | **被杀 10/10** | Golden, FiscalYearEndIsQ4, IsPureFunctionOfPeriodEnd, JuneFY, MonthBoundaryDrift, ResolvesRealConflicts（6 个） |
| 含端点漏等号（`m > ` → `m >= `） | **被杀 10/10** | Golden, Quarterization, SharesNoQ, DecemberFY, FiscalYearEndIsQ4, JuneFY, ResolvesRealConflicts（7 个） |
| 取消月界锚定（直接 `end.Month()`） | **被杀 10/10** | Golden, MonthBoundaryDrift |
| 放宽 `isSingleQuarter` 的 fp 检查 | **被杀 10/10** | Golden, RevenueTagSkipsUnusable, **RejectsAnnualTaggedQuarterlyEntry**（新增的负向锚点确实在防守） |

### 4.2 ⚠ 我自己撞到了 dev 描述的同一个陷阱 —— 构建守卫救了这次验证

我第一次构造「标签改回 fy/fp」时，替换后 `fyEndMonth` 变成未使用变量 → **编译失败**。
我的 `run_mut` 先跑 `go vet`，因此把它标为 **⛔ 无效变异**而**不是**"被杀"。
若没有这道守卫，编译失败的输出里同样没有 `--- FAIL:` 行，会被计成 **存活 0/10** ——
**与"变异存活"的输出形态完全相同**。

改用 `!hasFYEnd || fyEndMonth >= 0` 保留引用后，得到真实的 **10/10 被杀**。
这独立印证了 dev 报告的那次方法论事故，也再次验证「编译错误不是有效变异」这条判据。

### 4.3 ③ 12 月 fixture 确为回归护栏，无缺陷探测力 —— 已实证

`TestFiscalPeriodDecemberFY` **不在**「标签改回 fy/fp」的变红清单里
（该清单含 JuneFY、MonthBoundaryDrift、ResolvesRealConflicts 等 6 个，独缺 DecemberFY）。
即：把治本还原成缺陷版本，12 月那份**照样全绿** → **对本缺陷零探测力**。

**dev 的自述准确**。自然年公司的 EDGAR 标签本就与推导值一致，故它只能防"将来改坏"。
（12 月的**含端点**边界另由 `TestFiscalPeriodFiscalYearEndIsQ4` 覆盖，那个确实会红。）

---

## 5. ⑤ 降级路径：治本**未**覆盖 —— TASK-016 防御必需，已实证

dev 在 discovery 里要求保留 TASK-016 防御，理由之一是「降级路径下标签仍可能冲突」。
**我不满足于读代码推断，构造了实证**：

降级分支源码（`client.go:441-446`）返回 `fmt.Sprintf("%d%s", rawFY, rawFP)` ——
与 `period_end` 无关，正是产生冲突的原口径。构造无年度条目、两个不同 period_end
携带同一 `(fy,fp)` 的样本：

```
降级路径（无年度条目）  → map[2024-04-30:2026Q1  2025-04-30:2026Q1]
                          ★ 冲突复现：标签 2026Q1 同时对应两个 period_end
同一数据 + 一条年度条目 → map[2024-04-30:2025Q1  2025-04-30:2026Q1]   ✓ 分开
```

**结论：dev 的论断成立。** 治本只覆盖「有年度条目」的路径；
无年度条目时回退原 `fy/fp`，冲突风险原样保留（代码注释与 log 文案均如实写明
"which may collide across quarters"）。
**故 TASK-016 的 sankey 侧防御不是冗余，而是降级路径的唯一防线。**

已确认防御仍在：`periods.go:233` 的 `if len(qs) > 4` 拒绝聚合并记 `PeriodConflict`，
注释保留了 "measured on MSFT: FY2025 productivity at 3.25×" 的实测依据。

---

## 6. 两项测试健壮性发现（功能正确，断言不具判别力）

### 6.1 ① 阈值 15 是**单向锚定**（leader 点名核查项）

对 `anchorYearMonth` 的 `end.Day() > 15` 逐值变异：

| 阈值 | 结果 |
|---|---|
| 0（等于取消锚定） | **被杀 10/10** ✅ |
| 5 | ❌ 存活 10/10 |
| 10 | ❌ 存活 10/10 |
| **15（生产值）** | — |
| 20 | ❌ 存活 10/10 |
| 24 | ❌ 存活 10/10 |
| 25 | **被杀 10/10** ✅ |

即现有测试只把阈值钉在 **(0, 24]** 这个很宽的区间里，**具体值 15 没有锚点**。
上界之所以在 25 被抓住，是因为 NVDA 系 fixture 有 `2026-01-25` 这类期末（day=25）。

**为什么这不只是洁癖**：若阈值被改到 20，一个**真正在月中结束**的财季
（如 11-18 结束）会被错误地归到上一个月，从而整体错位一个季度 ——
而现有 fixture 里没有任何 day ∈ 11..24 的期末，这类错误会**静默通过**。

**建议修法（2 行）**：在 6 月或 12 月 fixture 里加一对期末 `day=15` 与 `day=16`，
锚定「15 归上月、16 归本月」。改后阈值 10 与 20 都会被杀。
dev 给的依据（"锚定月末的漂移至多数天，不会到半月"）是充分的**理由**，但**理由不等于断言**。

### 6.2 财年结束月「取众数」相对「取首个」未被锚定

变异「`if n > bestN || (n == bestN && m < best)` → `if bestN == 0`」（即取首个而非众数）
**存活 10/10**。众数是为 52/53 周财年**年度条目跨月漂移**准备的稳健性措施，
而现有 fixture 的年度条目月份一致，故该措施未被任何用例区分。

属稳健性措施缺锚点，非当前数据的正确性问题。若要锚定，需造一份年度条目月份不一致的 fixture。

---

## 7. Done Criteria 逐条

| # | 完成标准 | 核实 | 判定 |
|---|---|---|---|
| **F0** | 标签是 `period_end` 的纯函数 | `TestFiscalPeriodIsPureFunctionOfPeriodEnd`；变异"改回 fy/fp" 10/10 被杀 | **PASS** |
| **F1** | 真实冲突全解开**且语义正确** | 我手算 7/7 对照表全中；测试锚定 9 个具体标签 + 唯一性 | **PASS** |
| **F2** | 非日历财年（6 月 / 12 月 / 1 月） | 三种结束月均有 fixture；6 月有探测力、12 月为回归护栏（§4.3 实证） | **PASS** |
| **F3** | 财年结束月由 isAnnual 推导；降级定义且有断言 | `fiscalYearEndMonth` 取众数；降级回退 fy/fp + log；`TestFiscalPeriodDegradesWithoutAnnualEntries` 断言值与日志 | **PASS** |
| **B0** | 含端点，且**锚定具体值**而非只断言唯一 | `TestFiscalPeriodFiscalYearEndIsQ4` 表驱动，`Equal(want)` + `NotEqual(wrong)` 对偶；变异漏等号 10/10 被杀 | **PASS** |
| **B1** | golden 预期变化、只第一列、逐份核对、点名降级份 | 程序化核实 9→2 份、5 行、全在第 0 列；discovery 逐份对照表；`rev_fallback` 明确点名为唯一降级份 | **PASS** |
| **N0**(review) | golden 交叉校验 + 变异验证 + TASK-016 防御保留 | 均独立复现（§2/§4/§5） | **PASS** |
| **N1**(test) | 三包全绿；覆盖率 ≥93.1% | 全 `ok`；**93.6%** | **PASS** |

**8 条 done_criteria 全部满足。**

---

## 8. 判定

**verified（PASS）。**

8 条 done_criteria 全部满足；标签推导经我**手算独立复现**，DoD 的 7 组冲突对照表 7/7 相符；
4 个核心护栏变异各 **10/10 被杀**（含新增的负向锚点）；golden 变化经**程序化**确认仅第一列；
降级路径仍会冲突这一论断经我**构造实证**复现，据此确认 TASK-016 防御为必需而非冗余；
三包全绿，edgar 覆盖率 93.6% 高于基线。达到「压倒性证据」标准。

§6 两项为测试健壮性问题（阈值 15 单向锚定、众数选择未锚定），
**生产行为均正确**，合计约 2~3 行 fixture 即可闭合，建议指派后续任务，不构成退回理由。

§2.1 需特别记入：**golden 的标签唯一性不能作为"缺陷已修"的证据**（旧版本同样无冲突），
它只证明"没修坏"。这一点若被误读，会高估 golden 的保护力。
