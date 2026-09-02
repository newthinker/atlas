# TASK-004 验证报告

**任务**：22 个 `_mom` 字段原子扩容（常量 + `fieldOrder` 54→76 + 模板表两列 + yaml 占位）
**验证者**：test-m1c4-b　**日期**：2026-09-02　**assignment_epoch**：1

## 结论：**VERIFIED**

8 条 `done_criteria` 全部 PASS，无未覆盖项，无失败用例。所有数字均由本报告作者在本次验证中亲自跑出，未采信 discovery 的自证数字。

---

## 0. 采样锚与基线核对

| 项 | `verify_baseline` 记录值 | 判定时实测值 | 结论 |
|---|---|---|---|
| `head` | `8d02371ca2bc9513dac1a625bf3a8c73d70a68c2` | `8d02371ca2bc9513dac1a625bf3a8c73d70a68c2` | 一致 |
| `discovery_sha256` | `584a0609c8ed…7955e` | `584a0609c8ed…7955e` | 一致 |

判定前后各核一次，**零漂移**，无需 `--ack-drift`。

**被验提交**：`f56ec0541970ab10cc2e01fe8bb913c30ff8c4cb`（父 = `4670ccbe0abd703f86b1e0c53aef8d3c86cc512d`），经 merge `1eb8cc8` 进 master。

### 🔴 隔离说明：为什么 A/B 不能在 `verify_baseline.head` 上做

`8d02371` 之上叠着 **TASK-001 的 `4c9ca20`**，它改的是 `internal/hestia/strip.go`（`stripHTML` 删行首空白）——**那是抽取行为本身**。若拿 `4670ccb` 对 `8d02371` 做背对背，TASK-001 的行为变化会被算到 TASK-004 头上。

故 A/B 对钉在**隔离对**上，两端都是全 sha、无任何符号引用：

```
base = 4670ccbe0abd703f86b1e0c53aef8d3c86cc512d   （f56ec05 的父，即 TASK-004 之前）
post = f56ec0541970ab10cc2e01fe8bb913c30ff8c4cb   （TASK-004 本身，不含 TASK-001）
```

套件 / 覆盖率则在**两棵树上各跑一次**（隔离树 + 交付态 master），两处同值，故结论不依赖于选了哪棵。

---

## 1. 完成标准覆盖矩阵

| # | 维度 | 完成标准（摘要） | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| F1 | functional | 22 个导出常量、`fieldOrder` 54→76、各族 `_mom` 紧跟对应 `_ytd`、`FieldTSFFlowMoM` 在增量分项之后 | 临时探针实测 `len(fieldOrder)=76`；`TestFieldOrderGoldenList` / `TestFieldOrderHasNoDuplicates` PASS；calibrate 报表逐行核序（见 §3） | **PASS** |
| F2 | functional | `nameField{name,ytdField,momField}`、`loanScope.totalMoM`、三张模板表填齐两列、`templateFields()` 收两列 | `TestTemplateTablesCoverAllFields` PASS（76 vs 76）；源码逐行读（见 §4） | **PASS** |
| F3 | functional | 三条新测试存在且通过（第三条当前应 `t.Skip`） | `-v` 实跑：`TestMomFieldsExistForEveryFlowFamily` PASS、`TestEveryMomFieldHasAYTDTwin` PASS、`TestMagnitudeRangesCoverEveryFieldWhenCalibrated` SKIP | **PASS** |
| B1 | boundary | 四条既有计数断言同步且**保持分族求和形态**；`golden_test.go` 更新期望键集而非改断言；DDL 自动扩表 | 探针**独立测得** `tsf_=36 / deposit_=12 / loan_=18 / other=10`；`require.Len(t, fieldOrder, 36+6+12+18+4)`（非裸 76）；`TestFieldGroupCounts` / `TestGoldenTablesAreWellFormed` / `TestObservationsColumnsDeriveFromFieldOrder` / `TestCurrentViewStructureFromLiveDB` 全 PASS | **PASS** |
| B2 | boundary | `configs/hestia.yaml` 补 22 项（`min`/`max`/`unit`）、`config_version` 递增、注释写明占位与重标点 | 脚本逐项核对（见 §5）：76 项 / 22 个 `_mom` / **每项与其 `_ytd` 孪生逐字节相等** / `unit` 全为「亿元」/ 全部 `min<max`；`TestShippedConfigLoadsAndIsCalibrated` PASS | **PASS** |
| E1 | error_handling | 背对背 diff **恰好 22 行纯新增、0 删除、0 修改**，每行 n=0，既有 54 行逐字节不变 | 隔离 A/B 交替各 2 轮（见 §2） | **PASS** |
| N1 | non_functional | gofmt 无新增项、vet 零输出、suite 全绿、覆盖率 ≥96.1%、无新增依赖、注释带 milestone 前缀 | 见 §6 | **PASS** |
| N2 | non_functional | 交付流程：独立 worktree、显式 pathspec、merge 先于 `dev_done`、声明范围与实际改动一致 | 见 §7 | **PASS** |

**未覆盖的完成标准：无。**

---

## 2. E1 — 背对背行为比对（`verify_by: manual`，我自己跑的）

**纪律**：背对背（两个二进制同一时刻交替跑，非跨时间点）× 同等负载（同一语料、同一 flag、逐轮交替 base/post/base/post）。

```
CORPUS=/Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/hestia-backfill-2026-08-14
<bin> hestia backfill calibrate --dir "$CORPUS" --allow-incomplete
```
（主仓库绝对路径 + `--allow-incomplete`，AD-3；每个二进制在**自己的 worktree** 内运行，故各读自己那版 `configs/hestia.yaml`。）

**确定性对照**：`rc` 全为 0；base 两轮逐字节一致、post 两轮逐字节一致 ⇒ 输出确定，A/B 差异不来自运行间抖动。

**行数**：base 186 行 → post 208 行（+22）。

**diff 全文**（`diff cal-base-1.txt cal-post-1.txt`）：

```
92a93,101   > tsf_flow_mom / tsf_flow_rmb_loan_mom / tsf_flow_govt_bond_mom /
            > tsf_flow_corp_bond_mom / tsf_flow_fx_loan_mom / tsf_flow_entrust_mom /
            > tsf_flow_trust_mom / tsf_flow_bankaccept_mom / tsf_flow_equity_mom     (9)
105a115,119 > deposit_flow_mom / deposit_household_mom / deposit_corp_mom /
            > deposit_fiscal_mom / deposit_nbfi_mom                                  (5)
115a130,137 > loan_flow_mom / loan_hh_short_mom / loan_hh_mlt_mom / loan_corp_total_mom /
            > loan_corp_short_mom / loan_corp_mlt_mom / loan_bill_mom / loan_nbfi_mom (8)
```

| 判据 | 要求 | 实测 | |
|---|---|---|---|
| 新增行（`^> `） | 22 | **22** | PASS |
| 删除行（`^< `） | 0 | **0** | PASS |
| 修改（`^--- $` 分隔） | 0 | **0** | PASS |
| 每行形态 | n=0，其余列 em dash | 22/22 匹配 `<字段>  0  —  —  —  —  —  —` | PASS |
| 既有 54 行 | 逐字节不变 | 无任何 `<` 行、无其它 hunk ⇒ 报表其余部分（含各闸判定段）一字未动 | PASS |
| 第 23 行差异 | 不得存在 | 无 | PASS |

**我复现并确认了 Leader 与 dev 对原判据的订正**：`calibrate_report.go:163` 是 `for _, f := range fieldOrder` 全序遍历、不跳过 n=0 行（`TestRenderCalibrateReportFollowsFieldOrder` 钉着「一个不少」，本次实跑 PASS），⇒ **空 diff 在结构上不可能**。而 22 行 n=0 是比空 diff **更强**的证据：它同时证明这 22 列被识别、被渲染、且样本数为 0 —— 即「本任务确实没有接抽取路由」。

---

## 3. F1 — `len(fieldOrder)` 与排序（用正确的仪器）

⚠️ **没有用** `grep -oE 'Field[A-Za-z0-9]+' | wc -l`：注释行里的 `// FieldTSFFlowMoM 是总量…` 会被计入，Leader 用它得到过 78（假阳 +2）。

改用临时探针 `internal/hestia/zz_verify_probe_test.go`（跑完已删除，`git status` 复核工作树干净）：

```
PROBE len(fieldOrder)=76
PROBE mom=22 ytd=22 tsf_=36 deposit_=12 loan_=18 other=10
PROBE unique=76            ← 无重复
PROBE momFields()=22
PROBE requiredFields_v2=54 ← 76 − 22，确认 _mom 已被排除
```

**分族计数是我独立测出来的，不是照抄 DoD**：36 / 6 / 12 / 18 / 4（`other=10 = 6+4`），求和 = 76。与既有断言 `require.Len(t, fieldOrder, 36+6+12+18+4)` 一致，且该断言**保持分族求和形态**、未被改成裸 `76`（DoD 明令）。各族增量 36−27=9 / 12−7=5 / 18−10=8 ⇒ **社融 9 / 存款 5 / 贷款 8**，与 `requirements-analysis.md:24` 的「22 个 `_mom` 字段」相符。

**排序核对**（读 post 报表，报表即按 `fieldOrder` 打印）：

```
85–92   tsf_flow_{rmb_loan,govt_bond,corp_bond,fx_loan,entrust,trust,bankaccept,equity}_ytd
93      tsf_flow_mom              ← 总量，落在增量分项之后（DoD 明确要求）
94–101  同上 8 项的 _mom，顺序与 _ytd 逐项镜像
110–114 deposit_{flow,household,corp,fiscal,nbfi}_ytd
115–119 同上 5 项的 _mom，顺序镜像
122–129 loan_{flow,hh_short,hh_mlt,corp_total,corp_short,corp_mlt,bill,nbfi}_ytd
130–137 同上 8 项的 _mom，顺序镜像
```

每族 `_mom` 块紧跟本族 `_ytd` 块之后、块内顺序逐项镜像；`tsf_flow_ytd`（A.1 总量）在第 68 行，而 `tsf_flow_mom` 在第 93 行——**与 DoD 的例外要求逐字一致**。

---

## 4. F2 — 模板表两列

`nameField` 由 `{name, field}` 改为 `{name, ytdField, momField}`；`loanScope` 增 `totalMoM`；`tsfFlowItems`(8) / `depositItems`(4) / `loanScopes[].items`(2+3+0) 每项两列齐全；`templateFields()` 同时收 `ytdField`/`momField` 与 `totalMoM`（`totalMoM != ""` 时才收，与 `totalField` 同形）。

`extract.go` 三处 `setFlow(c, it.field, …)` → `it.ytdField`：**纯机械改名**，`git show` 逐行确认三个 hunk 各只改这一个标识符，捕获组下标与 keep 条件一字未动。

`TestTemplateTablesCoverAllFields` PASS ⇒ 模板覆盖数 == `len(fieldOrder)` == 76。

---

## 5. B2 — yaml 22 项占位（脚本逐项核对，非目测）

```
magnitude_ranges 条目总数 = 76
_mom 条目数 = 22
缺 _ytd 孪生的 = []
与 _ytd 区间不一致的 = []          ← 22/22 逐项等于对应 _ytd 的 {min,max,unit}
unit 非「亿元」的 _mom 项 = []
min>=max 的 _mom 项 = []
```

⇒ **注释里声明的取法（「逐项照抄对应 `_ytd` 区间」）经机器核对为真**，不是一句无从检验的自述。

`config_version` `2026-08-31` → `2026-09-01`，`config_test.go:377` 的断言同步改到新值。三段注释各自写明「本组区间**未经标定**，占位待 M1c-4 的 TASK-010 重标」+ 破环理由（循环依赖）+ 取法依据（累计量程 ⊃ 当月，偏宽只漏拦不误拦）+ 「不要当标定结果引用」的显式警告（并援引 `deposit_sum_tolerance` 0.12 的走样先例）。**DoD 要求的「值 + 标注」两半都做到了。**

---

## 6. N1 — 门禁数字（全部我自己跑，两棵树各一次）

| 项 | 判据 | 隔离树 `f56ec05` | 交付态 master `8d02371` |
|---|---|---|---|
| `go test ./internal/hestia/... -count=1` | 全绿 | **ok** | **ok** |
| 覆盖率 `-cover` | ≥96.1% | **96.1%** | **96.1%** |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | **零输出** | — |
| `gofmt -l internal/hestia cmd/atlas` | 两项既有欠账之外无新增 | **恰为** `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` | — |
| `go.mod` / `go.sum` 出现在改动里 | 0 | **0** | — |
| 集成回归 `go test ./cmd/... -count=1` | （我加测的）不得被 61→83 建表变更波及 | **ok（8.2s）** | — |

覆盖率恰好压线 96.1%，达标（判据是 ≥）。

**milestone 前缀约定**：机器扫描新增行里的 `TASK-0xx` 引用，除 5 处为**换行折断**（`M1c-4 的` 落在上一行行尾）外，其余首次提及均带前缀。唯一例外见 §8 的 INFO-1。

---

## 7. N2 — 交付流程与声明范围

**声明范围与实际改动逐项核对**（脚本比集合，非目测）：

```
声明 writes 项数 = 12   实际改动文件数 = 12
改了但未声明 = []
声明了但未改 = []
```

12/12 **逐项一致**。越界的 4 个文件（`extract.go` / `required.go` / `required_test.go` / `schema_test.go`）已由 dev 在 `transition dev_done` **之前**经写通道补进 `writes`，并在 commit message 与 discovery 里申报——**时序合规**（进 `dev_done` 后无任何角色能合法补此字段）。Leader 已批准。

流程：`f56ec05`（feat，锚定 `feat(TASK-004):`）→ merge `1eb8cc8` 进 master → 回主仓库转 `dev_done`。**merge 先于 `dev_done`**，符合 AD-4。

---

## 8. 观察项（不构成缺陷，不影响判定）

- **INFO-1（纯排版）**：`profiles.go` 的 `singletonFields()` 注释里 `TASK-007` 是该块内的首次提及且未带 `M1c-4 的` 前缀（同块其余任务号均带）。DoD 的该要求是 `non_functional[0]` 里的一条 ⚠️ 附注，其余 20+ 处引用合规。不值得为此返工。

- **已知开口 —— 四条全部经核实「被诚实记录且指派了退场点」**（这正是 Leader 点名要查的部分）：

  | 开口 | 记录位置 | 退场/归属 | 核实 |
  |---|---|---|---|
  | `momFields()` 是过渡代码（22 个 `_mom` 暂不进必填集） | `required.go` 函数头 15 行注释 | 🔴 **明写「退场点是 M1c-4 的 TASK-006」并列出该删哪三处** | 已读原文，PASS |
  | `deposit_flow_mom` / `loan_flow_mom` **无任何写入方** | `profiles.go` `singletonFields()` 注释 | 明写走 `selectRMBCumulativeFlow`、keep 条件只保累计句、**归 TASK-005**，并写出「不补的无声连锁」 | 已读原文，PASS |
  | `TestMagnitudeRangesCoverEveryFieldWhenCalibrated` **恒 skip、一次都不跑** | `thresholds_test.go` 测试头注释 | 明写它与 `TestDefaultThresholdsLeaveMagnitudeRangesUncalibrated` 条件互补 ⇒ 永不进循环、「现在守不住任何东西，留着是当文档」；点名真守卫是 `TestShippedConfigLoadsAndIsCalibrated`；并劝阻「别为了让它跑去填 `DefaultThresholds()`」 | 已读原文 + `-v` 实测确为 SKIP，PASS |
  | `LoadConfig` 侧缺全覆盖校验（半张表静默通过） | 同上注释末段 | 明写 `thresholds.go` 的 `if !ok { continue }`、归 **TASK-010**、并区分「要拒半张表、不是空表」 | 已读原文，PASS |

  **这一项按 Leader 的口径判的是「事实有没有被诚实记录」，不是「skip 本身对不对」。** 四条都不仅写了「是什么」，还写了「为什么」「谁来收」「不要顺手做的错事」——记录质量高于最低要求。

---

## 9. 判定依据小结

- 所有 PASS 均锚定**我自己运行的命令输出**，无一条来自 discovery 的转述。
- 关键数字用了**与 dev 不同的独立仪器**：`len(fieldOrder)` 用编译期探针（不是 grep）、分族计数用探针遍历（不是照抄 DoD 的 36/12/18）、yaml 占位取法用脚本逐项比对（不是相信注释的自述）、A/B 用隔离对（不是相信 `dev_done` 时的树）。
- 唯一发现的偏差是一处排版级 INFO，不触及任何 `done_criteria`。
- `verify_baseline` 判定前后零漂移。

**⇒ VERIFIED**
