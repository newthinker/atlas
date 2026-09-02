# TASK-008 验证报告 —— `PrecedingAll` + `deposit_sum` drift 基线修复

- **验证者**：test-m1c4-a
- **判定对象**：`master @ 743c50730128be0a648d8d5d2c99cac321b1a9e0`（= `verify_baseline.head`）
- **dev commit**：`ddb325f8e3df978d3f8528b3a8bdbeb41efb8f2c`（parent `07d1113…`）
- **assignment_epoch**：1
- **结论：VERIFIED**（附两处数字修正，见 §4）

---

## 0. 基线核对与范围

| 项 | verify_baseline | 实测 | 一致 |
|---|---|---|---|
| `head` | `743c507301 28…` | 同值 | ✅ |
| `discovery_sha256` | `30d6967d817b…` | 同值 | ✅ |

`ddb325f` 的 parent 实测确为 `07d1113`；两树间 diff 恰为本任务 4 个文件，
numstat 与 discovery 报的**逐条一致**（`store.go 100/0`、`store_test.go 198/2`、
`validate.go 67/6`、`validate_test.go 271/13`）。`go.mod`/`go.sum` 未碰，**无越界**。

⚠️ 判定对象 `743c507` 与 dev commit 之间还合入了 TASK-009，故 **A/B 钉隔离对
`07d1113`/`ddb325f`**，门禁**两棵树都采**（这条是我在 TASK-007 验证时订正的纪律）。

---

## 1. done_criteria 覆盖矩阵（8 条）

| # | 维度 | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | `PrecedingAll` + `History` 接线 + `noHistory` 实现 | `store.go:410`；`Validate:103` 取；接口已加 | **PASS** |
| F1 | functional | drift 基线改 `priorAll` + **相邻性约束**（不按跨度放宽） | `validate.go:371` 判空 / `:383` 相邻 / `:409` hist；§6 | **PASS** |
| F2 | functional | 四条测试 + 截断分支用例 | 全在且绿，含 `TestPrecedingAllTruncatesAfterMerge` | **PASS** |
| B0 | boundary | 同族约束 + 导出面守卫 + **接线断言** | §5、§6（M9） | **PASS** |
| B1 | boundary | 沿用既有符号（`TablePending` 等） | 无越界；`scanPendingObservation` 已新建 | **PASS** |
| E0 | error_handling | 导出面守卫要更新期望清单 | `want` 已含 `Store.PrecedingAll`，且**项数由 `len(want)` 生成**（未退回手写）| **PASS** |
| N0 | non_functional | 门禁 + 覆盖率（**两棵树都采**） | §7 | **PASS** |
| N1 | non_functional | 交付流程 | numstat 逐条一致 | **PASS** |

---

## 2. 🔴 核心判据：独立复算「同族基线可得性」两列

**按 dev 声明的口径复算**：两列算自**同一份 post DB**（97 组合并观测），
差别只在基线取 `Preceding` 还是 `PrecedingAll`——**不是**「pre 那次运行 vs post 那次运行」。

我用**生产代码路径**写探针（`Store.PrecedingAll` / `Store.Preceding` + `pickCaliberBand`
+ `periodGapMonths`/`expectedPeriodGapMonths` + `depositResidualOf` + `minDriftHistory`），
**不是**单篇 Parse——正是 dev 自己撞出并更正的那个错口径。

```
入权威表 64 / 落 pending 33 / 合并观测 97
枚举到观测 97 条
族分布：ytd 52 / mom 21 / 两族都取不到 24（合计 97）

🔴 真被判过漂移（n>=3 且首基线相邻）:
   族     仅权威表(Preceding)    priorAll(PrecedingAll)
   ytd    21                     28
   mom    0                      15
```

⇒ **与 dev / Leader 报的逐个吻合**：ytd `21→28`、**mom `0→15`**。

**这证否了 DoD 的一条前提**——DoD `functional[2]` 写「mom 族取不到同族基线是设计的必然后果…
那批新救回的观测 drift 闸可能恒 skip 而完全没有保护」。实测：**mom 族的 drift 保护从 0 期变成 15 期**
（21 个 mom 观测里 15 个真被判过）。成因正如 dev 所述：mom 期次集中在 2020–2022，那批大多落 pending
⇒ 在旧实现里彼此互相看不见；`PrecedingAll` 纳入 pending 后**它们互相就是同族基线**。

mom 同族基线直方图 `{0:1, 1:2, 2:3, 3:5, 4:10}` 与 dev 报的**完全一致**。
⚠️ ytd 直方图我得 `{1:5,2:5,3:5,4:7,5:12,6:4}`（合计 38）而 dev 报 `{0:9,…}`（合计 52）——
**口径差异**：我在「priorAll 非空 ∧ 相邻」两道前置检查**之后**才记直方图，dev 统计的是全部 52 个 ytd 观测。
两列关键数字（21/28、0/15）不受影响。

---

## 3. 真语料 A/B（隔离对，交替两轮）

```
pre  = 07d111309737de1aaa08bbe05dd22b31a1e07ffb
post = ddb325f8e3df978d3f8528b3a8bdbeb41efb8f2c
交替：pre(r1) → post(r1) → pre(r2) → post(r2)
```

- **确定性自检**：pre 两轮逐字节一致 ✅；post 两轮逐字节一致 ✅。
- 四次退出码均 0（pre 407 行 / post 404 行）。

| 指标 | pre | post | 与 dev 报的 |
|---|---|---|---|
| 入权威表 | **61** | **64** | ✅ 一致 |
| 落 pending | **36** | **33** | ✅ 一致 |
| `drift_exceeded[mom]` | 0 | **3** | ✅ 一致 |
| `drift_exceeded[ytd]` | **10** | **4** | ⚠️ dev 报 9 → 3（见 §4） |

### dev 举的那条例子逐字吻合

pre 第 400 行：
```
2024-11/monthly@2024-12-13 deposit_sum: drift_exceeded[ytd]: residual 0.1135 drifted 0.0351 from 3-period mean 0.0784
```
post 第 402 行（2024-10 那条，基线已变）：
```
2024-10/monthly@2024-11-11 deposit_sum: drift_exceeded[ytd]: residual 0.1092 drifted 0.0375 from 4-period mean 0.1467
```
⇒ 「`0.0784` 是停在 2024-03 的旧均值、改后变成 `4-period mean 0.1467`」这条机制描述**属实**。

### ✅ 「新增 3 条是新保护而非回归」——已验证成立

我查了那三期在 **pre** 报告里的**全部**出现：

| 期次 | pre 里的出现 | post |
|---|---|---|
| 2020-11 | 只有第 47 行「合并组明细」（哪几篇合成的），**无任何 `deposit_sum` 判定行** | `drift_exceeded[mom]` |
| 2021-07 | 只有第 67 行合并组明细 | `drift_exceeded[mom]` |
| 2022-11 | 只有第 107 行合并组明细 | `drift_exceeded[mom]` |

⇒ 那三期在改动前**压根没被 `deposit_sum` 判过**（基线取不到同族样本 ⇒ skip），
改动后才进入判定范围 ⇒ **确是新增的保护，不是回归** ✅

---

## 4. 🔴 两处数字修正（不影响判定，但会进 CONTRACTS / TASK-012 基线）

### ① `drift_exceeded[ytd]` 的基数：dev 报 9→3，我实测 **10→4**

我把两端逐条列出并做集合运算：

```
pre  (10 条): 2023-12/annual, 2024-10/monthly, 2024-11/monthly, 2024-12/annual,
              2025-04, 2025-07, 2025-08, 2025-10, 2025-11, 2026-02  (均 monthly)
post ( 4 条): 2023-12/annual, 2024-10/monthly, 2025-02/monthly, 2026-02/monthly
```

**差值一致**（10−4 = 9−3 = 6），机制描述也正确；只是**两端基数各少 1**。
若从两端各去掉 `2023-12/annual`（它在 pre/post 都在、未变化）恰好得到 dev 的 9→3，
推测是它的统计口径漏了那一条——**但成因我定不了，只报观察**。

### ② 🔴 dev 只报了「消失 6 条」，实际是「**消失 7 条 + 新增 1 条**」

```
消失（pre 有 post 无，7 条）: 2024-11/monthly  2024-12/annual  2025-04  2025-07
                              2025-08  2025-10  2025-11
🔴 新增（post 有 pre 无，1 条）: 2025-02/monthly
```

那条新增：`2025-02 residual 0.0458 drifted 0.0628 from 5-period mean 0.1086`。
我同样查了它在 pre 里的全部出现——**只有合并组明细，无任何 `deposit_sum` 判定行**
⇒ 与 mom 侧那三条**同性质**，也是**新增的保护**，不是回归。

⇒ 结论不变（VERIFIED），但**报告应写「ytd 侧净减 6 = 消失 7 + 新增 1」**，
而不是「消失 6 条」——否则 TASK-012 拿这份基线做对照时，`2025-02` 会显得来路不明。

---

## 5. 接线口径（用函数体行号精确核，不用 `grep -A20`）

⚠️ Leader 提示过：`grep -A20` 从函数头往下找会**命中 0**（函数在 508 行而 `in.prior` 在 531 行，超出窗口）。
我改用脚本按花括号配平定位函数体，再在体内检索：

```
gateStockContinuity 函数体：508 .. 583
  in.prior    出现于行: [531, 550, 553, 557]
  in.priorAll 出现于行: 无 ✅

gateDepositSum      函数体：328 .. 426
  in.priorAll 出现于行: [371, 383, 387, 391, 392, 409]
  in.prior    出现于行: 无 ✅
```

⇒ **双向都干净**：统计量类判据只读 `priorAll`，逐期比较类判据只读 `prior`。
双向接线断言 `TestStockContinuityStillReadsAcceptedPriorsOnly` 在 `validate_test.go:1873` ✅。

---

## 6. 变异（我自己跑，隔离 worktree）

主工作区 `validate.go` / `store.go` sha256 前后一致，代码目录改动 **0 行**。
**对照组 685 PASS / 0 FAIL、`=== RUN` 1461 条。**

### 🔴 M6：`len(in.priorAll)==0` 误写成 `len(in.prior)==0`（Leader 点名要验）

```
--- FAIL: TestDepositSumDriftWorksWhenAllPriorsArePending     ← 且只有它一条（684 PASS）
      TestDepositSumDriftBaselineIncludesPending 未红          ← 印证 Leader 的另一半
```

⇒ **Leader 的两半说法都成立**：新补的那条**杀得动**，而原有的 `…BaselineIncludesPending`
**确实分辨不出它**（那里权威表有三期、`prior` 非空，两种写法都往下走）。
这个缺口是真的，且已闭合。

### M9：`gateStockContinuity` 改读 `priorAll`

杀 `TestStockContinuityStillReadsAcceptedPriorsOnly` + `TestStockContinuityDoesNotUseRejectedPeriodAsBaseline`
⇒ 「只该喂给统计量」这条现在**有机制守着**，不只是注释。

### M12 → M12b：我第一次的变异是崩溃型，结论作废后重做

| | 手法 | 结果 |
|---|---|---|
| **M12（作废）** | 短路 `if err != nil` | `rows` 为 nil 仍被 `Next()` ⇒ **panic**，`=== RUN` 只有 1196（对照组 1461）⇒ **128 个顶层测试根本没跑**。「只有一条红」是假的 |
| **M12b** | `return accepted, nil`（正确的「退化成只读权威表」） | 套件**跑全**（`=== RUN` 1461 = 对照组），**只杀 `TestPrecedingAllWrapsPendingQueryError`**（684 PASS）✅ |

⇒ 新补的那条测试确实闭合了 `TestPrecedingAllWrapsQueryError` 的恒绿缺口
（那处正是 Leader 说「靠覆盖率画出 `store.go:427` 是白的」才发现的）。

⚠️ 记这一条是因为它是**崩溃型变异被误记成 KILLED** 的教科书形态：退出码非 0、有 FAIL 行、
甚至"只有一条红"看起来都对——**唯一能拆穿它的是 `=== RUN` 计数**。

---

## 7. 门禁（两棵树都采）

| | `ddb325f`（dev commit） | `743c507`（merge 后 master） |
|---|---|---|
| `go test ./internal/hestia/...` | 退出码 0 | 退出码 0 |
| 覆盖率 | **96.3%** | **96.3%** |
| `go vet` | 零输出 | 零输出 |
| `gofmt -l` | 仅两个既有欠账 | 仅两个既有欠账 |

⇒ **两棵树同为 96.3% ⇒ TASK-009 的合入没有掩盖任何东西**。

**导出面守卫**：`want` 名单已含 `Store.PrecedingAll`（字母序正确），
且期望项数由 `len(want)` 生成、未退回手写 ✅。

---

## 8. 结论

**8 条 done_criteria 全部 PASS**。核心证据由验证者独立产生：

- **两列复算完全吻合**（ytd 21→28、mom **0→15**），用生产代码路径而非单篇 Parse；
  这证否了 DoD「mom 族恒 skip、完全没有保护」的前提——**dev 的发现属实且是本任务价值的核心**。
- **A/B 主要数字吻合**（权威 61→64、pending 36→33、mom 新增 3 条），dev 举的例子逐字对上。
- **「新增是新保护而非回归」已验证**：那三期在 pre 里无任何 `deposit_sum` 判定行。
- **M6 缺口真实且已闭合**，新补测试杀得动、旧测试确实分辨不出。
- **两处数字修正**（§4）：`drift_exceeded[ytd]` 实为 10→4；ytd 侧还有 **1 条新增**（`2025-02`）未被报告。
  两者都不改变结论，但应在 CONTRACTS / TASK-012 基线里更正。

**判定：VERIFIED**
