# TASK-009 验证报告

**任务**：`calibrate` 输出 `deposit_sum` 与 `corp_loan_reconcile` 的残差分布（按口径族与 `period_type` 分档）
**验证者**：test-m1c4-b　**日期**：2026-09-02　`assignment_epoch=1`

## 结论：**VERIFIED**

8 条 `done_criteria` **全部 PASS**。5 个变异体全部 KILLED，dev 自称的两颗钉子经我独立复算确实有牙。

**本次最有价值的一步是 dev 没做也无法做的**：它的自证钉在 `3b47113` / `02c116c` 这一对上，而**那两棵树本就等价**，比对恒真；真正的判定对象 `743c507` 比它晚两个 commit、中间叠了 TASK-008。我在判定对象上重采，结论成立（§2）。

**另有一条要 Leader 路由**：`thresholds.go` 有**两处**已成假话的注释，该文件不在本任务 `writes`、在 TASK-010 的 `writes` 里（§8）。

---

## 0. 采样锚与基线核对

| 项 | `verify_baseline` 记录值 | 判定时实测值 | 结论 |
|---|---|---|---|
| `head` | `743c50730128be0a648d8d5d2c99cac321b1a9e0` | `743c50730128be0a648d8d5d2c99cac321b1a9e0` | **一致，零漂移** |
| `discovery_sha256` | `02da1489760d…3d0e42` | `02da1489760d…3d0e42` | 一致 |

不需要 `--ack-drift`。

**三个锚**（全 sha，无任何符号引用）：

```
base  = 07d111309737de1aaa08bbe05dd22b31a1e07ffb   TASK-009 之前（= dev commit 的父）
dev   = 3b47113e6e52d58f5a1dfc533fba591c4bbe172c   dev 的采样锚
judge = 743c50730128be0a648d8d5d2c99cac321b1a9e0   🔴 判定对象（= verify_baseline.head）
```

---

## 1. 完成标准覆盖矩阵

`done_criteria` 共 **8 条**，全部字符串形态 ⇒ 视同 `verify_by: test`。其中 functional[0] 与 error_handling[0] 各带一处 **dev 在 `dev_done` 前落盘的实测订正**，我按订正后的判据验。

| # | 维度 | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | **订正后**：`collectGateResiduals(recs) residualCollection` 纯派生，**不在 `CalibrateResult` 上加任何字段**；不塞进 `res.Samples` | `calibrate.go` **0 行改动**（`git show --numstat` 只列两个文件）；`residualCollection` 两半（`samples`/`skips`）就位 | **PASS** |
| F1 | functional | 报告新增一节，两族 × `period_type`，列 `n/min/p5/median/p95/max`，**不加建议区间列**，`n` 出声；**射程扩大到 `corp_loan`**；复用 `depositResidualOf`/`bandDiagnosis`，🔴 **不得用 `pickCaliberBand`** | 12 行分档**每行恰 7 列**；AST 层核实（§4） | **PASS** |
| F2 | functional | 两条点名测试存在且通过；断言须与**真实输出形态**一致（需求文档那条正则永不命中） | `calibrate_report_test.go:794/832`，实跑 PASS；M1/M4 变异能杀掉它们 ⇒ 非空洞（§6） | **PASS** |
| B0 | boundary | 两族必须分开、`period_type` 也要分；字段不全的期次 `continue`，**不当成残差 0** | M4（去 `period_type` 维）KILLED；M3（当成残差 0）KILLED（§6） | **PASS** |
| B1 | boundary | 用**真实**入口名 `renderCalibrateReport(w io.Writer, res *CalibrateResult) error` | 测试编译通过且全绿 | **PASS** |
| E0 | error_handling | **订正后**：`deposit_sum/ytd/monthly` 的 `max = 0.2501`（2020-01）、`p95 = 0.1663`（2024-04）；反向自检「max < 0.16 就停下来查」 | 我在**判定对象**上自采，**逐位相符**（§3） | **PASS** |
| N0 | non_functional | gofmt / vet / suite / 覆盖率 ≥96.1% / 无新增依赖 | §7 | **PASS** |
| N1 | non_functional | 交付流程；声明范围一致 | `3b47113` → merge `02c116c` → `dev_done`；`writes` 无越界（§7） | **PASS** |

**未覆盖的完成标准：无。**

---

## 2. 🔴 dev 的自证是**恒真的**，真正的问题它无从知道——我补上了

### 2.1 它的自证为什么不构成证据

discovery 的 `anchors` 写：

> 真语料残差分布首采于 `my_commit` 那棵树，**merge 后在 `02c116c` 上复采过一次，两份输出逐字节一致**

拓扑事实：

```
02c116c 的两个父 = 07d1113 与 3b47113
3b47113 的父     = 07d1113
git diff --stat 3b47113 02c116c  →  空
```

⇒ **`02c116c` 与 `3b47113` 树本就逐字节等价**，那次复采**在结构上不可能不一致**。它证明的是「程序确定性」（有价值），**不是**「merge 带进来的他人改动没影响我」。

⚠️ 这不是 dev 的过失：它的 `anchors` 里如实写了「merge 区间只有我自己的两个 commit，无他人改动混入」——**它知道自己那段是干净的**。

### 2.2 真正的缺口：基线在 `dev_done` **之后**又前进了

```
02c116c  merge(TASK-009)              ← dev 的 merged_master
ddb325f  feat(TASK-008): 漂移基线改用 priorAll…
743c507  merge(TASK-008)              ← 🔴 verify_baseline.head，判定对象
```

TASK-009 在 `05:58:48Z` 转 `dev_done`，基线在 `06:00:24Z` 捕获时 master 已是 `743c507` —— **TASK-008 在那 96 秒内合入，dev 无从知道**。而 TASK-008 改了 `validate.go`（67/6），正是 `deposit_sum` 漂移基线所在的文件。

⇒ **「重采于 merge 后 master」这条纪律本身有一个它盖不住的窗口**：master 可以在 `dev_done` 与派验之间再动。**只有验证者能在 `verify_baseline.head` 上量。**

### 2.3 我的实测：结论成立

三棵树**交替** `base → dev → judge` 各跑 2 轮，同一语料同一 flag，`rc` 全 0、stderr 全 0 行。

| 树 | 两轮自比 | 输出 sha256 | 行数 |
|---|---|---|---|
| `base` `07d1113` | 逐字节一致 | `363e9f98b77a…` | 112 |
| `dev` `3b47113` | 逐字节一致 | `62f9493e6a7c…` | 132 |
| **`judge` `743c507`** | 逐字节一致 | **`62f9493e6a7c…`** | 132 |

🔴 **`dev` 与 `judge` 输出 sha256 完全相同** ⇒ TASK-008 的改动不触及残差分布，dev 的全部数字在判定对象上原样成立。

`base → judge` 的 diff：**20 行纯新增、0 删除、1 个 hunk** —— 恰是新增的两节，无任何既有输出被改动。

⚠️ 另核：TASK-008 的漂移（`store.go`/`store_test.go`/`validate.go`/`validate_test.go`）与 TASK-009 声明范围（`calibrate*.go`）**交集为空**。

---

## 3. E0 —— 真语料分布（我在判定对象上自采）

```
勾稽残差分布：deposit_sum 与 corp_loan_reconcile（按口径族与 period_type 分档，不给建议值）
闸门·族·period_type                  n          min           p5       median          p95          max
deposit_sum/mom/monthly          21 0.008188976  0.016283186  0.257550007  0.819414317  2.950782998
deposit_sum/ytd/annual            7 0.016636132  0.016636132  0.077467448  0.093923854  0.093923854
deposit_sum/ytd/h1                7 0.076492117  0.076492117  0.089186176  0.146020942  0.146020942
deposit_sum/ytd/monthly          27 0.002645788  0.023417367  0.095395879  0.166311475  0.250104167
deposit_sum/ytd/q1                5 0.048610778  0.048610778  0.064049527  0.078875893  0.078875893
deposit_sum/ytd/q1_q3             6 0.088468320  0.088468320  0.099955967  0.123238434  0.123238434
corp_loan/mom/monthly            26 0.001437908  0.001608422  0.017567709  0.078372591  0.078813454
corp_loan/ytd/annual              7 0.002340550  0.002340550  0.011635423  0.028552413  0.028552413
corp_loan/ytd/h1                  7 0.007017544  0.007017544  0.013717218  0.035710872  0.035710872
corp_loan/ytd/monthly            27 0.003612717  0.008999264  0.013826531  0.024650350  0.029411765
corp_loan/ytd/q1                  5 0.011506623  0.011506623  0.013742938  0.029626168  0.029626168
corp_loan/ytd/q1_q3               6 0.003453039  0.003453039  0.010527489  0.028167939  0.028167939

算不出残差的期次（两族都不齐或零分母 ⇒ 不计入上表）
  deposit_sum    ×135  ytd:absent_field:deposit_flow_ytd mom:absent_field:deposit_flow_mom
  deposit_sum    ×5    ytd:absent_field:deposit_household_ytd mom:absent_field:deposit_flow_mom
  corp_loan      ×135  ytd:absent_field:loan_corp_total_ytd mom:absent_field:loan_corp_total_mom
```

（上表为便于阅读截断了小数位；判据比对用的是原始全精度值。）

**对 DoD 订正后判据的核对（机械比对，容差 1e-12）**：

| 判据 | 期望 | 实测 | |
|---|---|---|---|
| `deposit_sum/ytd/monthly` `max` | `0.2501041666666667` | `0.2501041666666667` | 符合 |
| `deposit_sum/ytd/monthly` `p95` | `0.16631147540983607` | `0.16631147540983607` | 符合 |
| 反向自检 `max ≥ 0.16` | — | `0.2501` | 通过 |

⇒ **dev 对原判据（「0.1663 是 max」）的订正是对的**：`0.1663` 是 `p95`，`max` 是 `0.2501`。

### 3.1 交叉验证：两条独立代码路径的四族 `n/min/max` 逐位一致

本任务走**回填管线**（`collectSamples → Records → collectGateResiduals`）；TASK-007 的 dev 走**逐篇 `Parse`**。两条路径互不调用。

| 族 | 我（本任务，跨桶聚合） | TASK-007（独立路径） | |
|---|---|---|---|
| `deposit_sum/ytd` | n=52 min=0.0026 max=0.2501 | n=52 min=0.0026 max=0.2501 | ✅ |
| `deposit_sum/mom` | n=21 min=0.0082 max=2.9508 | n=21 min=0.0082 max=2.9508 | ✅ |
| `corp_loan/ytd` | n=52 min=0.0023 max=0.0357 | n=52 min=0.0023 max=0.0357 | ✅ |
| `corp_loan/mom` | n=26 min=0.0014 max=0.0788 | n=26 min=0.0014 max=0.0788 | ✅ |

⚠️ **只比 `n` / `min` / `max`**：这三个跨桶可聚合。**`p50` / `p95` 跨桶不可聚合**——拿各桶的分位数去合成整族分位数是错的仪器，比出来的一致或不一致都没有意义，故不比。

### 3.2 自洽闸：两道闸各自的「已算 + 跳过」都等于 213

```
deposit_sum   已算 73 (52 ytd + 21 mom) + 跳过 140 (135+5) = 213
corp_loan     已算 78 (52 ytd + 26 mom) + 跳过 135          = 213
```

两道闸**独立地**把每条记录分进「已算」或「跳过」，若收集逻辑漏掉记录，两个总数会各自变小且不必相等。两者同为 213（报告自报「尝试解析 217 期」，4 篇未产出记录）。

---

## 4. F1 —— 实现约束（用 AST 核，不用 grep）

⚠️ 我第一次用 `grep -n 'pickCaliberBand' calibrate_report.go` 得到 3 处命中（412/415/421），**全部是注释里解释「为什么不用它」**——与 TASK-004 那次 `grep -oE 'Field[A-Za-z0-9]+'` 被块注释污染同形。

换成 Go 语法层仪器（`go/parser` 以 `ParseFile(..., 0)` **不解析注释**，遍历 `*ast.CallExpr`）：

```
calibrate_report.go:430:8   bandDiagnosis
calibrate_report.go:481:14  pickBandFor
calibrate_report.go:486:15  depositResidualOf
validate.go:304:8           bandDiagnosis
validate.go:332:13          pickCaliberBand
validate.go:339:11          depositResidualOf
validate.go:395:16          depositResidualOf
validate.go:471:13          pickCaliberBand
```

⇒ `calibrate_report.go` 对 `pickCaliberBand` **零调用** ✓，且确实复用了 `bandDiagnosis` 与 `depositResidualOf` ✓；`validate.go` 的两处 `pickCaliberBand` 未被改动 ✓。

**「不给建议值」的判据落在列数上**：12 行分档**每行恰 7 列**（键 + `n` + 5 个统计量），零行有第 8 列。这比 `NotContains(out, "建议")` 强——后者被一个打出 `[0.05,0.13]` 却不写「建议」二字的实现绕过。

**射程扩大**：`corp_loan` 六档已产出（`ytd` n=52 / `mom` n=26），报告标题已改名为「勾稽残差分布：deposit_sum 与 corp_loan_reconcile」。

---

## 5. F0 —— 不加字段

`git show --numstat 3b47113` 只列两个文件：`calibrate_report.go` 195/0、`calibrate_report_test.go` 378/0。**`calibrate.go` 一行未改** ⇒ `CalibrateResult` 未新增任何字段（导出的与非导出的都没有），DoD 订正后判据满足。

⚠️ 顺带：DoD boundary[0] 提醒「导出面守卫 `TestPackageExposesNoWriteFunctions` 只遍历 `*ast.FuncDecl`、不覆盖 struct 字段」。本任务通过**压根不加字段**规避，不依赖那道守卫——这是比「加了但登记一下」更强的处理。

---

## 6. 变异 5/5 全 KILLED（隔离树 `wt-v9-mut` @ `743c507`）

五道闸：sha256 生效闸 / 变异体 diff 回显 / `go vet` 构建闸（**构建不过既不计 KILLED 也不计 SURVIVED**）/ 全包转红清单 / `git checkout --` 还原 + 主工作区指纹复核。

| 变异 | 改动 | 结果 |
|---|---|---|
| **M1** | `deposit_sum` 选族顺序 `ytd` 优先 → `mom` 优先 | KILLED：`ResidualCollectionPrefersYTD…` + 另 2 条 |
| **M2** | `corp_loan` 的 `mom` 分项列错配成 `ytd` 的（模拟**副本分叉**） | KILLED：🔴 **`TestResidualGatesAgreeWithValidationGates`**（dev 的钉子二） |
| **M3** | 算不出的期次记成残差 `0` 而非 `continue` | KILLED：`CalibrateReportsDepositResidualDistribution` + `ResidualCollectionRecordsWhySkipped` |
| **M4** | 复合键去掉 `period_type` 维 | KILLED：`SeparatesPeriodTypes` + 另 3 条 |
| **M5** | `pickBandFor` 恒取第一族（忽略诊断） | KILLED：🔴 **`TestPickBandForAgreesWithPickCaliberBand`**（dev 的钉子一） |

对照组（未变异）全包 `ok`。

⇒ **dev 自称「变异实测两颗钉子都有牙」，经我独立复算成立。** 这两颗钉子是它为「`residualGates` 是 `validate.go` 那两份 band 字面量的第二份副本」这个已知风险自愿补的——M2 证明副本分叉会当场变红，风险确实被堵住。

---

## 7. N0 / N1 —— 门禁与声明范围

| 项 | 判据 | 实测（判定对象 `743c507`） |
|---|---|---|
| `go test ./internal/hestia/... -count=1` | 全绿 | **ok** |
| 覆盖率 `-cover` | ≥96.1% | **96.3%** |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | **零输出** |
| `gofmt -l internal/hestia cmd/atlas` | 两项既有欠账之外无新增 | **恰为** `backtest_test.go`、`crisis_test.go` |
| `go test ./cmd/... -count=1`（我加测的） | 通过 | **ok** |
| `go.mod` / `go.sum` 在 `3b47113` 里 | 0 | **0** |

**声明范围**：

```
声明 writes = 3  [calibrate.go, calibrate_report.go, calibrate_report_test.go]
实际改动    = 2  [calibrate_report.go, calibrate_report_test.go]
改了但未声明 = []                ← 无越界
声明了但未改 = [calibrate.go]    ← 允许（且正是 F0 订正后判据要求的结果）
```

---

## 8. 🔴 要 Leader 路由：`thresholds.go` 有**两处**已成假话的注释

TASK-007 交付时在 `thresholds.go` 写下的标定开口，**在本任务射程扩大之后已经不成立**：

| 位置 | 原文 | 现状 |
|---|---|---|
| `thresholds.go:85-86` | 「🔴 **TASK-009 不产 corp_loan 的残差分布** ⇒ TASK-010 **无从标定**」 | **假** —— 本任务已产出 `corp_loan` 六档（`ytd` n=52 / `mom` n=26） |
| `thresholds.go:160` | 「corp_loan 那个 TASK-009 不产分布，需 Leader 裁决」 | **假**，同上 |

⚠️ **discovery 只报了一处，且行号写成 `84-85`**（实际 `85-86`），**漏了 `:160` 那处**。

**为什么必须路由而不是记在这里**：`thresholds.go` **不在** TASK-009 的 `writes`（dev 无权改，处理正确），**在 TASK-010 的 `writes` 里**。而 TASK-010 的 dev 会读 `thresholds.go`（那是它要改的文件），读到这两句会得出「corp_loan 的容差没有依据」——**而依据就在它同时要读的 calibrate 报告里**。

⇒ **请把「订正 `thresholds.go:85-86` 与 `:160` 两处」写进 TASK-010 的 `done_criteria`**。只写进 discovery 或本报告，TASK-010 的验证者不会照着它判。

---

## 9. INFO（不影响判定）

- **`calibrate_report_test.go:692`** 有一处 `TASK-007` 未带 `M1c-4 的` 前缀（本次新增，在 `bandValues` 的文档注释里）。纯排版，DoD 的该要求是 `non_functional[0]` 的一条 ⚠️ 附注；同两文件里其余 TASK 引用多数合规。
- ⚠️ **我自己制造过一次假警报**：变异 harness 里主工作区指纹用 glob `calibrate*.go`（**4** 个文件），而我开工时手工列了 **3** 个文件；收尾对比时多出的第 4 个哈希被我读成「主工作区被污染」。实为既有的 `calibrate_test.go`。核实：`git status --porcelain -- internal/hestia/` **为空**、HEAD 仍 `743c507`、我关心的三个文件哈希与开工前**逐一相同**。
  ⇒ **两组测量口径不同就不可比**——这正是我上一轮对 dev 的「14 个子测试」用过的判据，这次栽在自己身上。指纹前后必须用同一个口径。

---

## 10. 判定小结

- 8 条 `done_criteria` 逐条 PASS，每条锚定我自己跑出的输出。
- 关键处用了**比 dev 更强或不同的仪器**：三棵树（含判定对象）交替各 2 轮 / AST 而非 grep 核调用点 / 两条独立代码路径交叉验证 `n·min·max` / 两道闸「已算+跳过」自洽闸 / 5 个变异体。
- 唯一的实质发现（`thresholds.go` 两处假注释）**不属本任务射程**，已给出路由建议。

**⇒ VERIFIED**
