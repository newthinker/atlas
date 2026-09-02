# TASK-007 验证报告

**任务**：`deposit_sum` 与 `corp_loan_reconcile` 按口径族分别校验
**验证者**：test-m1c4-b　**日期**：2026-09-02　`assignment_epoch=1`

## 结论：**VERIFIED**

8 条 `done_criteria` **全部 PASS**。核心的端到端要求我在真语料上复现成功，且比 dev 演示的更强。

**另有一条不阻断但需要 Leader 路由的发现**：变异实测显示实现里两处关键行为**无测试守卫**（§6），其中一处是 dev 自己在 discovery 里写明的失效模式。二者都会在 **TASK-010 标定容差之后**变成活的回归口，建议补进那个任务的 DoD。

---

## 0. 采样锚与基线核对

| 项 | `verify_baseline` 记录值 | 判定时实测值 | 结论 |
|---|---|---|---|
| `head` | `5045c65974f8907596c5e28ec16c4ac34185e069` | 主仓库 HEAD 已是 `07d111309737de1aaa08bbe05dd22b31a1e07ffb` | ⚠️ **有漂移，见下** |
| `discovery_sha256` | `ec2e870e05db…c23fa` | `ec2e870e05db…c23fa` | 一致 |

### HEAD 漂移的归因（不静默放行）

派验后 master 前进了 **4 个 commit，全部属于 TASK-011**：

```
07d1113 merge(TASK-011)  ff49e79/28ea907 …   涉及文件仅两个：
  internal/hestia/backfill_load_report.go        341/0
  internal/hestia/backfill_load_report_test.go   452/1
```

TASK-007 的 `writes` 是 `validate.go` / `validate_test.go` / `thresholds.go` / `thresholds_test.go` ——
**漂移不触及本任务声明范围内的任何文件**，按机制只应出 INFO 而非要求 `--ack-drift`。

⇒ **我把判定对象钉在 `5045c65974f8907596c5e28ec16c4ac34185e069`**（= `verify_baseline.head`，与 dev 的 commit `0e36ff8` 树等价，`git diff --stat` 为空），门禁在那棵树上采；同时在当前 master 上也采了一份作对照（§7）。

---

## 1. 完成标准覆盖矩阵

`done_criteria` 共 **8 条**，全部字符串形态 ⇒ 视同 `verify_by: test`。

| # | 维度 | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F1 | functional | `depositPartFieldsMoM` 与 `depositPartFields` 逐项对应；`depositResidualOf` 泛化；算不出返回 `false` | 符号在 `validate.go:215/373`；旧 `depositResidual(` 调用点已清零；**M4 变异（清单顺序错位）KILLED** | **PASS** |
| F2 | functional | 两道闸先选族（两族齐取 ytd）；比较用选出的 `tol`；`Reason` 带族名与 `total` 列名；两族都不齐时说清哪族缺 | 真语料 Reason 逐字为 `tolerance_exceeded[mom]: … (total=deposit_flow_mom)`；**M1 变异（选族顺序调换）KILLED**；真语料上确认 skipped 的 Reason 同时带两族诊断（§4） | **PASS** |
| F3 | functional | 五条新测试 + `corp_loan` 两条 + 既有 `TestCorpLoanSkipsOnZeroDenominator` 不得被破坏 | 8 条全部到位（§3）；`-run 'TestDepositSum\|TestCorpLoan' -v` → **21 PASS / 0 FAIL / 0 SKIP** | **PASS** |
| B1 | boundary | 两个 MoM 容差；**「值写下来」+「注释写明未标定」两半都要做** | `thresholds.go` 58 行注释：含四族分布表、「照抄不行」的结论、以及 `CorpLoanToleranceMoM` **无从标定**的开口（§5） | **PASS** |
| B2 | boundary | 沿用既有测试辅助，不新写重复份；**先读文件确认实际名字** | `testThresholds`/`checkByID` 在本仓库**根本不存在**（DoD 已警告别照需求文档命名），dev 正确地没有新造；沿用 `NoHistory`×4；仅新增无重复的 `momOnly` | **PASS** |
| E1 | error_handling | 逐条读 `TestDepositSum\|TestCorpLoan` 输出，检查有无 `failed↔skipped` 翻转 | 既有 `TestDepositSumCombinesTwoCriteria`(6 子)、`TestDepositSumBoundaryIsInclusive`(4 子)、两条 `SkipsOnZeroDenominator`、`IgnoresUncomputablePriors`、`DistinguishesNoHistoryFromShortHistory` **全部原样通过，无一翻转** | **PASS** |
| N1 | non_functional | gofmt / vet / suite / 覆盖率 ≥96.1% / 无新增依赖 | 见 §7 | **PASS** |
| N2 | non_functional | 交付流程；数字重采自 merge 后 master；声明范围一致 | `0e36ff8` → merge `5045c65` → `dev_done`；`writes` 无越界（§8） | **PASS** |

**未覆盖的完成标准：无。**

---

## 2. 🔴 真语料端到端——DoD 点名的那条，我复现成功且做得更强

DoD 的独立反审明确要求：单测手工构造 `Values` 证明不了端到端，**必须跑一次真语料**确认 `deposit_sum` 在至少一篇 mom 族观测上真的判了。

### 2.1 复现 dev 的单篇口径（逐字命中）

临时探针：`Parse(articles/2025092212552757258.html)` + `Validate(…, NoHistory, DefaultThresholds())`

```
period=2022-07  type=monthly  extractor=rule-monthly@v1  字段数=25
  deposit_flow_ytd    = <不在>      deposit_flow_mom    = 447
  loan_corp_total_ytd = <不在>      loan_corp_total_mom = 2877
  loan_corp_short_mom = -3546  loan_corp_mlt_mom = 3459  loan_bill_mom = 3136

deposit_sum          status=failed  value=2.950783
  reason="tolerance_exceeded[mom]: residual 2.9508 exceeds 0.1200 (total=deposit_flow_mom)"
corp_loan_reconcile  status=failed  value=172.000000
  reason="residual 0.0598 exceeds 0.0500 [mom, total=loan_corp_total_mom]"
```

**与 discovery 逐字相同**（期次、extractor、25 字段、四个字段取值、两道闸的 status/value/reason 全部一致）。**两道闸都真的判了，不是 `skipped{absent}`。**

### 2.2 我另跑了真实管线（比 dev 的单篇口径更强）

`atlas hestia backfill load --dir <语料> --allow-incomplete --db <新库>`，`rc=0`、stderr 0 行。管线校验的是**合并后**的观测（2022-07 由 3 篇文章合并而成），比单篇更接近生产路径。

直读库里 `hestia_pending.report` 的 JSON（**不是** stdout 报告，理由见 §9）：

| 闸门 | 判定分布（36 期 pending） |
|---|---|
| `deposit_sum` | `failed/mom` **17**、`failed/ytd` 18、`skipped/mom` 1 |
| `corp_loan_reconcile` | `failed/mom` **5**、`passed` 31 |

自洽校验：两个闸各自求和均 = 36 = pending 期次数；36 pending + 61 权威表观测 = 97 = 报告自报的合并组数。

🔴 **这 22 条 `failed/[mom]` 判定，在本任务之前全部会是 `skipped{absent}`** —— 即「这一期没查过」而报告上看不出来。这是本任务价值的直接、可数的证据，比单篇样本强得多。

合并后的 2022-07 两道闸判定与 §2.1 单篇口径**逐字相同**（`deposit_sum` value=2.9507829977628637、`corp_loan_reconcile` value=172、reason 一字不差）。

---

## 3. F3 —— 八条测试逐条定位

| 测试 | 位置 | 来源 |
|---|---|---|
| `TestDepositPartFieldsAgreeAcrossCalibers` | `validate_test.go:1551` | 新增 |
| `TestCorpLoanPartFieldsAgreeAcrossCalibers` | `validate_test.go:1563` | 新增（补 DoD 缺口一） |
| `TestDepositSumChecksMoMFamily` | `validate_test.go:1577` | 新增 |
| `TestCorpLoanReconcileChecksMoMFamily` | `validate_test.go:1600` | 新增（补 DoD 缺口一） |
| `TestDepositSumPrefersYTDWhenBothPresent` | `validate_test.go:1627` | 新增 |
| `TestDepositSumSkipsWhenNeitherFamilyComplete` | `validate_test.go:1654` | 新增 |
| `TestMoMTolerancesAreDeclaredAndNonZero` | `validate_test.go:1671` | 新增 |
| `TestCorpLoanSkipsOnZeroDenominator` | `validate_test.go:452` | **既有，未被破坏**（实跑 PASS） |

⚠️ 七条新测试全部落在 `validate_test.go`，**这正是 `thresholds_test.go` 被声明却未改动的原因**（`writes` 声明范围大于实际使用是允许的，非越界）。

---

## 4. F2 的一条真语料证据（不只是单测）

DoD 要求「两族都不齐才 `skipped`，且 `Reason` 要说清是哪一族缺」。真语料上恰好有一例：

```
2023-07  deposit_sum  skipped
  Reason = ytd:absent_field:deposit_household_ytd mom:absent_field:deposit_flow_mom
```

**两族诊断同时在场**，格式沿用既有的 `absent_field:` 前缀。这条是在真实数据上撞到的，不是构造出来的。

---

## 5. B1 —— 未标定占位的标注（两半都做了）

`thresholds.go` 新增 58 行，其中注释部分明确写了：

- 🔴🔴「**这两个值都是未标定的占位数，不是标定结果**」
- 为什么必须是两个独立字段（ytd 分母是年初至今累计、mom 是单月增量，量级差一个数量级）
- 一张**指示性取样**分布表（deposit ytd/mom、corpLoan ytd/mom 的 n/min/p50/p95/max），并明写「这不是标定」「只回答『照抄 ytd 行不行』，答案是**不行**」
- 为什么保留照抄值而不就地拍宽：「**failed 会进 pending 被人看见，拍宽的容差会让所有 mom 期次静默通过**」
- 标定开口：`DepositSumToleranceMoM` 由 TASK-009→TASK-010 可标定；🔴 **`CorpLoanToleranceMoM` 因 TASK-009 不产 corp_loan 残差分布而无从标定**，需 Leader 裁决

⇒ Leader 点名要确认的那个标注**在，且写得比最低要求充分**：它把「它没标定」这句话变成了一张**可核对的数**，而不是一句免责声明。

⚠️ 我复算了那张表所依赖的关键数：真语料上 `deposit_sum` 的 mom 族 17 期 failed —— 与「p50 0.2576 > 占位 0.12 ⇒ 过半 failed」的预测方向一致。

---

## 6. 🔴 变异实测：两处关键行为**无守卫**（不阻断，需路由）

隔离树 `wt-v7-mut` @ `5045c65`。每个变异带五道闸：sha256 生效闸 / 变异体 diff 回显 / `go vet` 构建闸（**构建不过时既不计 KILLED 也不计 SURVIVED**）/ 全包转红清单 / `git checkout --` 还原 + 主工作区**四文件**指纹复核（前后一致）。

| 变异 | 改动 | 结果 |
|---|---|---|
| **M1** | 选族顺序 `ytd` 优先 → `mom` 优先 | **KILLED** —— `TestDepositSumPrefersYTDWhenBothPresent` |
| **M2** | `if r > b.tol` → `if r > in.cfg.DepositSumTolerance` | 🔴 **SURVIVED**，全包绿 |
| **M3** | 漂移历史 `depositResidualOf(p.Values, b.total, b.parts)` → 写死 ytd 族 | 🔴 **SURVIVED**，全包绿 |
| **M4** | `depositPartFieldsMoM` 顺序错位（Corp↔Fiscal） | **KILLED** —— `TestDepositPartFieldsAgreeAcrossCalibers` |

对照组（未变异）全包 `ok`。

### 6.1 为什么这不构成 REJECT

- **8 条 `done_criteria` 没有任何一条要求为这两处行为写测试**，五条点名的测试全部到位且都真的在守（M1/M4 证明）。
- **dev 没有做任何虚假声明**——恰恰相反，M3 那条**是 dev 自己在 discovery 里写明的**：「漂移历史必须用与本期同一族算……而它**不会让任何测试变红**，只是给出一个同样合理的错结论。」**我的变异实测证实了它的自述**。
- 这与 TASK-003 第 1 轮的情况**不同**：那次是「注释声称有守卫、实际没有」（假注释）；这次是「明说没有守卫，也确实没有」。

### 6.2 为什么仍然必须报给你

- **M2 现在杀不掉，是因为两个占位容差恰好相等**（`DepositSumToleranceMoM = DepositSumTolerance = 0.12`）⇒ 任何测试都区分不出「用了选出的族的 tol」和「写死 ytd 的 tol」。
  🔴 **TASK-010 一旦标定、两值分开，M2 立刻从「不可测」变成「活的回归口」**，而那时不会有任何测试拦它。
- **M3 今天就是活的**：mom 族期次一旦通过容差判据进入漂移判据，历史残差若按 ytd 族算，两个分母量级差一个数量级，`|r − mean|` 完全失去意义。DoD 的 `error_handling` 正是在防这类「不会变红、只是换了一个同样合理的结论」的错。

### 6.3 建议的闭合方式（都很便宜）

| 缺口 | 建议补的测试 | 放哪 |
|---|---|---|
| M2 | 构造 `Thresholds{DepositSumTolerance: 0.12, DepositSumToleranceMoM: 0.50}` + 一份 mom 族观测，残差取 0.3 ⇒ 必须 `passed`（写死 ytd 的实现会判 failed） | **TASK-010 的 DoD**（标定时两值必然分开，正是该守住的时刻） |
| M3 | 构造 mom 族本期 + 一批 ytd 族历史，验证 `hist` 为空（而非拿 ytd 残差充数）；或本期与历史同为 mom 族，验证漂移基于 mom 均值 | 同上 |

⇒ **请把这两条写进 TASK-010 的 `done_criteria`**。写进 discovery 或本报告的载体强度不够——TASK-010 的 dev 读的是它自己的 DoD。

---

## 7. N1 —— 门禁（两棵树各采一次）

| 项 | 判据 | 判定对象 `5045c659` | 当前 master `07d1113` |
|---|---|---|---|
| `go test ./internal/hestia/... -count=1` | 全绿 | **ok** | **ok** |
| 覆盖率 `-cover` | ≥96.1% | **96.1%**（压线达标） | **96.3%** |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | **零输出** | — |
| `gofmt -l internal/hestia cmd/atlas` | 两项既有欠账之外无新增 | **恰为** `backtest_test.go`、`crisis_test.go` | 同左 |
| `go test ./cmd/... -count=1`（我加测的） | 通过 | **ok** | — |
| `go.mod` / `go.sum` 在 `0e36ff8` 里 | 0 | **0** | — |

⚠️ 值得你知道：**漂移把覆盖率抬高了（96.1 → 96.3），不是压低**，TASK-011 带进来的新代码测试覆盖比均值高。DoD 里担心的「合进来的改动把它压到 96.0 而报的 96.1 掩盖了它」这次没有发生——但我是两棵树各采一次才敢这么说的。

---

## 8. N2 —— 声明范围

```
声明 writes = 4  [thresholds.go, thresholds_test.go, validate.go, validate_test.go]
实际改动    = 3  [thresholds.go, validate.go, validate_test.go]
改了但未声明 = []      ← 无越界
声明了但未改 = [thresholds_test.go]   ← 允许（声明范围可大于实际使用）
```

`thresholds_test.go` 未改的原因见 §3：`TestMoMTolerancesAreDeclaredAndNonZero` 落在 `validate_test.go`。

---

## 9. ⚠️ 我自己踩的一次仪器错（记下来，因为它会再犯）

我最初用 `grep 'corp_loan_reconcile'` 读 `load` 的 **stdout 报告**，只得到 `2023-07` 一条，于是判断「dev 报的 2022-07 corp_loan 0.0598 与管线不符」，并且**差一点把成因归给「合并改变了观测」**——那是我编的。

实际成因（是观察，不是推理）：**报告的 pending 段每期只印一行**（自报 36 期，该段恰 36 行，库里也恰 36 行）。2022-07 两道闸都 failed，报告只印了 `deposit_sum` 那条。直读库里的 `report` JSON 后，2022-07 的 `corp_loan_reconcile` 与 dev 报的**逐字相同**。

⇒ **权威产物是 `hestia_pending.report` 的 JSON，不是给人看的 stdout 报告。** 另：那份 JSON 的键是 `Checks`/`ID`/`Status`/`Reason`（**首字母大写**）——我第一版统计脚本用小写键，跑出「36 期、0 条 check」的空结果，靠自洽校验（求和应等于期次数）才发现，没有当成「没有数据」。

---

## 10. INFO（不影响判定）

- **discovery 报「14 个子测试全 PASS」，我对不上这个数**。同一份输出我用五把尺算：全部 `--- PASS` 行 = **21**、顶层 = 11、子测试 = 10、叶子节点 = 19、`=== RUN` 顶层 = 11。差值 7 / −3 / −4 / −5 / −3，**都不构成「某一类 × 观测数差」**。⇒ **口径未能对上，成因不明，我不编一个。** 该条的实质结论（全 PASS、无 `failed↔skipped` 翻转）我已独立验证成立，不受此影响。
- **mom 族大面积 failed 是预期状态**（Leader 已裁决），我没有因此判红：17 条 `failed/mom` 与 discovery 的 `p50=0.2576 > 0.12` 预测方向一致，属占位容差的必然结果而非实现缺陷。

---

## 11. 判定小结

- 8 条 `done_criteria` 逐条 PASS，每条锚定我自己跑出的输出。
- 关键处用了**比 dev 更强或不同的仪器**：真语料走**完整 load 管线**（dev 是单篇 Parse）+ 直读库 JSON（不是 stdout）+ 四个变异体 + 两棵树各采一次门禁。
- 唯一的实质发现（M2/M3 无守卫）**不构成 DoD 不达标**，但会在 TASK-010 之后变成活的回归口，已给出便宜的闭合方式并建议写进那个任务的 DoD。

**⇒ VERIFIED**
