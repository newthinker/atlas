# TASK-002 验证报告 —— 社融存量/增量独立报告的整篇抽取包装

- **验证者**：test-m1c3a-v1（承接时 `assignment_epoch` = 1）
- **判定**：✅ **VERIFIED** —— 8 条 done_criteria 全部 PASS
- **证据强度**：本次**全量语料在本地**（`data/hestia-backfill-2026-08-14/`，218 篇，未被 git 跟踪），
  故 dev 与 Leader 的每一条语料级论断我都**独立重跑复现**，不是采信自陈。

## 0. 判定对象、漂移与内容完整性

| 项 | 值 |
|---|---|
| `verify_baseline.head` | `cea6b3cb4c172c17adea7cf8a3a224b605f75d93` |
| `discovery_sha256` 基线/现值 | `6c160d6b1a…` / `6c160d6b1a…`（**一致**） |
| dev 分支 tip `55a5fef` 与 `8256ccb` | **同一棵 tree** `ad2062debbb66b366e6266a8c56b390e09fdd57e` |
| 我采样期间 HEAD | 漂到 `20c05ea46af33ac95ed3a0f7d952cd0feb036dfe`（TASK-001 返工合入） |
| 漂移是否触及本任务 `writes` | **否**（只动 `profiles_test.go`）；四个被验文件 sha256 与我开工时逐字相同 |

**内容判据（不用 `merge-base`）**：
`git diff 55a5fef master --name-only -- <本任务 writes 四路径>` **输出为空** ⇒ 交付内容完整合入。

⚠️ 值得记一笔：Leader 给的判据是「`git diff 55a5fef master --name-only -- internal/hestia/` 输出为空」，
**实跑不为空**——多出 `required.go` / `required_test.go` / `types.go` 三个文件。
但那三个是 **TASK-003（`5372f8e`）** 的，master 有而 dev-b 分支没有，与本任务无关。
**把判据收窄到本任务声明的四个路径后才为空。** 判据的作用域比它写下时以为的宽，结论不变。

**自证数字的采样时点**：dev 的 `sampled_at_head` 是 `8256ccb`，而分支 tip 是 `55a5fef`——
看起来像「采样早于最后一次改动」。实跑 `git diff 8256ccb 55a5fef` **为空**（同 tree），
故 dev 的数字确实采自最终产物，**不构成 CLAUDE.md「自证数字须在最后一次改动之后重采」的违反**。

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 我跑的证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 抓两份快照，manifest 原名记进 discovery | — | `manifest.json` 实读：`5837468`→`articles/5837468.html`（存量）、`5837454`→`articles/5837454.html`（增量），title 与快照 `<title>` 逐字一致 | ✅ |
| functional[1] | 两个包装函数，整篇当一节，复用既有 section 抽取 | `TestExtractTSF{Stock,Flow}ArticleOnSnapshot` | 存量确为一行包装；增量按 Leader 答复加作用域切分 | ✅（偏离已授权，见 §5） |
| functional[2] | 存量 18 / 增量 9，**逐字段数值**断言 | 同上 + `assertMatchesGolden` | `require.Len` 18/9；我把 27 个 golden 值**逐条对快照原文核对，27/27 命中**，且单位归一算术自洽 | ✅ |
| boundary[0] | 不抽到「从结构看」段占比值 | `TestTSFStockArticleTakesBalanceNotStructureShare` | 结构段 7 个占比值实测**都真实存在**于快照；N3 故障注入证明信号路径正确 | ✅ |
| boundary[1] | 四种方向词 / 同句混用单位 / 负值带符号 | `TestTSFFlowArticleKeepsDirectionSign` | N4 消融 KILLED（`-816` vs `816`） | ✅ |
| boundary[2] | 总量句四类 + 口径混装响亮失败 | `TestTSFFlowArticleTotalSentenceForms`、`TestTSFFlowArticleRefusesCaliberMix` | 全量语料独立复现 19/6/19/25；A1 消融**直接观察到** `287000`/`4431` | ✅ |
| error_handling[0] | 缺分项报错且信息含分项名 | `TestTSFArticleNamesTheMissingItem` | 合成用例 3 条 + **真实语料** 7 篇失败的错误信息均含具体分项名 | ✅ |
| non_functional[0] | gofmt/vet/全绿/覆盖率/无新依赖/**milestone 前缀** | — | 全部满足，**裸编号 0 处** | ✅ |

## 2. 我自己重采的数字

三棵树同批次交替、锚全部钉全 sha：

| 锚 | 含义 | RUN | 覆盖率（两轮取值全同） |
|---|---|---|---|
| `4f469e96…` | pre（TASK-001 后、本任务前） | 975 | 1618/1694 = **95.5136%** |
| `bca01bd5…` | post（本任务完整、TASK-003 前） | **999 = 498 顶层 + 489 第1层 + 12 第2层** | 1665/1743 = **95.5250%** |
| `cea6b3cb…` | baseline head（含 TASK-003） | 1013 | 1678/1756 = **95.5581%** |

- 新增 **24 条 RUN = 9 顶层 + 15 子测试**，**0 条消失**。
- 三个锚点**都 ≥95.5%**，门槛无歧义。
- `gofmt -l internal/hestia/` 0 行、`go vet` 0 行（exit=0）。
- numstat `190/0` + `418/0` + `421/0` + `422/0`，**纯新增 0 删除**，四个文件全在 `writes` 内，无越界。
- 本任务改动不含 `go.mod` / `go.sum` ⇒ 无新增依赖。
- **与 dev 报数逐项一致**（999、498+489+12、95.5250%、1665/1743、四个 numstat）。

### ⚠️ 一处方法学更正（影响我上一份报告，不影响本任务判定）

dev 报的层级分解 `498 + 489 + 12` 我起初测成 `498 + 460 + 37 + 4`，一度以为 dev 错了。
**是我的方法错**：我按「名字里 `/` 的个数」算层级，而 `"N/A"`、`137_条_/_12_页`、
`<a>/</a>` 这类**名字自带斜杠**会被误记成更深层级。改用「祖先是否存在于 RUN 名集合」重算后，
**dev 的分解逐字正确**。详见 §7。

## 3. 全量语料独立复现（本报告最强的一段证据）

语料在 `data/hestia-backfill-2026-08-14/`（218 篇 = 69 存量 + 69 增量 + 80 月报，**未被 git 跟踪**，
故隔离副本里没有；探针用可覆写的 `ZZ_CORPUS` 指向主仓库只读读取）。

| 论断 | 出处 | 我的实测 | 结论 |
|---|---|---|---|
| 存量 64/69、增量 40/69、合计 **104/138**，失败 34 | dev | **完全相同** | ✅ |
| 总量句四类 **19 / 6 / 19 / 25 = 69** | dev + Leader | **完全相同** | ✅ |
| 口径混装 **8 篇**（不是 4 篇） | dev 实测、Leader 复验 | **8 篇，且 16 个数字逐字相符** | ✅ |

判别式我用的是「**旧路径 `err=nil` ∧ 新路径报错**」，正好切出这 8 篇：

```
2022-07 ytd=217700 rmb_loan=4088    2022-08 241700/13300
2022-10 287000/4431                 2022-11 304900/11400
2023-07 220800/364                  2023-08 252100/13400
2023-10 311900/4837                 2023-11 336500/11100
```

⚠️ **量级比的实测区间是 18.2×–606.6×**（2023-07 达 606.6×），
dev 与 DoD 都写「约 30 倍」——**偏保守**，不影响结论，但引用时以实测区间为准。

**34 篇失败归属核算**（我独立重算，与 dev 的分类一致）：

```
19  类3 只有当月值        刻意报错（人类裁决 A）
 8  口径混装              刻意报错（本任务新加的作用域切分）
 5  存量：3 空格 + 2 同比持平   移交 M1c-3a 的 TASK-005
 2  增量：数字前空格            移交 M1c-3a 的 TASK-005
--
34 = 138 − 104 ✓
```

**零回归**：上述 5 篇存量 + 2 篇增量，我逐篇跑了**旧路径**（`extractTSF*Section` 直接喂全文），
**旧路径同样失败、且错误逐字相同** ⇒ 不是本任务引入的。「归本任务的缺口 = 0」成立。

## 4. 消融

### A1 —— DoD boundary[2] 指定（判据是**看到那两个值**，不是「测试红了」）

去掉作用域切分后，我用探针直接观察 2022-10 体例的产出：

```
probe: err=<nil>
probe: len=9
probe: tsf_flow_ytd=287000
probe: tsf_flow_rmb_loan_ytd=4431
```

**正是 DoD 点名的那一对具体值。** 随后套件 KILLED：红在
`TestTSFFlowArticleRefusesCaliberMix`（"An error is expected but got nil"）与
`TotalSentenceForms/类4_两者都有_累计段在前`。⇒ boundary[2] 的消融要求**逐字满足**。

### 我设计的 5 个变异（**全部在 dev 的生成集 {A1,A2,A3,A4,A4b,A4c,A4d,A5,A6} 之外**）

| ID | 变异 | 结果 | 红在哪一条断言（因果） |
|---|---|---|---|
| **N1** | `isCumulative` 的 `h.groups[2] == "1"` → `strings.HasPrefix(h.groups[2], "1")` | **KILLED** | `TotalSentenceForms/类3_仅为_非1月`（"An error is expected but got nil"）。这正是代码注释点名的陷阱（「判据落在『1 月』而不是『一位数月份』，靠的是捕获组」）——**注释声称的性质确实被钉住了** |
| **N2** | 两条累计句时不报错、取第一条 | **KILLED** | **单点**红在 `RefusesTwoCumulativeSentences`，无旁落 |
| **N4** | `setFlow` 抹掉方向词符号（恒取绝对值） | **KILLED** | `KeepsDirectionSign/tsf_flow_fx_loan_ytd`（`-816` vs `816`）+ 快照 golden 三个负值字段 |
| **N3** | **故障注入**：强制 `委托贷款 = 2.6`（结构段占比值） | **KILLED** | 🔑 否定式断言以**其自称的文案**转红：`tsf_stock_entrust 取到了「从结构看」段的占比值 2.6` |
| **N5** | **故障注入**：强制 `信托贷款 = 1`（结构段占比值） | KILLED，**但** | 🟡 `TakesBalanceNotStructureShare` **保持绿**，只有 golden 测试红 |

**隔离纪律**：`git archive <全 sha> | tar -x` 到 `mktemp -d`；对照组先跑一轮 exit=0；
每个变异过替换计数闸 → diff 逐字核对闸 → `gofmt -e` 闸 → `go build` 闸。
主工作区四个文件 sha256 + `git status --porcelain` 指纹在开始、每个变异窗口内、收尾共 **8 次**校验，
**逐次相同，全程一个字节未碰**。harness 锚点做成可覆写 `ABLATE_ANCHOR`，默认钉全 sha。

### N3 / N5 为什么值得单列

dev 已证明「**没有任何单点变异能让抽取器返回 2.6**」（四道独立机制）。那证明的是**危害不可达**。
它没有回答**互补的另一半**：如果哪天它变得可达了，这条断言会不会以它自称的信号说话？

- **N3 回答了**：会。断言原样打出「取到了「从结构看」段的占比值 2.6」。⇒ boundary[0] 的守卫**名副其实**。
- **N5 暴露一处非阻断的覆盖缺口**：该专用测试的否定式集合有 7 项，覆盖 **6/8 分项 + 总量**，
  缺 `RMBLoan`（结构段值 61.2，现被记在总量那格）与 `Trust`（结构段占比 1%）。
  信托贷款取错时**这条专用测试不响**，靠 golden 测试兜住。
  ⇒ 性质本身被覆盖，但**不是被 DoD 指定的那条测试覆盖**。补两行即可闭合，建议留给 TASK-005/007。

## 5. functional[1]「不新增模板」的偏离——已授权，不判违规

DoD functional[1] 写「**不新增模板**——若探针显示必须放宽某条正则，只放宽那一条」。
dev 新增了 `tsfFlowArticleTotalRE`。判 PASS 的理由：

1. DoD 那句预设「放宽既有模板」是可行路径。**探针证明它不可行**：放宽成
   `增量(?:累计)?为` 会让第 4 类那 25 篇同时命中两句、`mustMatch` 报 `matched 2 sentences`
   ——**原本成功的 25 篇被打坏**。这条我在 §3 的四类分布里独立复现（类4 = 25 篇）。
2. **不是静默偏离**：dev 在 `questions[0]`/`questions[1]` 就此提问，Leader 答复
   「算 TASK-002 范围，你来修」并明确接受「改动会超出『两个包装函数各一行』」。
   代码注释里也写明了「为什么不是把 `tsfFlowTotalRE` 放宽」。
3. dev **自己发现并补上了偏离造成的缺口**：新模板落在 `profiles_test.go` 的
   `TestNoGreedyCaptureInTemplates` 覆盖之外，故按同一判据补了
   `TestExtractGoArticleTemplatesHaveNoGreedyCapture`，并在注释里注明 profiles.go 空出后应挪回去。

## 6. 我核实过的其它几项

1. **无生产调用方**：`extractTSFStockArticle` / `extractTSFFlowArticle` 除定义处外只被 `_test.go` 引用
   —— 与 discovery 的 `not_verified_by_me` 自陈一致，是刻意分层（TASK-007 接上），不是漏接。
2. **否定式断言非空**：结构段 7 个占比值（2.6 / 0.3 / 0.5 / 7.7 / 21.1 / 2.8 / 61.2）
   实测**都真实存在**于快照的「从结构看」段 ⇒ 不是在空集上平凡为真。
3. **milestone 前缀**：新增行 8 处任务引用，**裸编号 0 处**。
   其中 1 处为并列承接（`M1c-3a 的 TASK-001 与 TASK-005`），共享前缀、指向不歧义，我按满足计。
4. **快照溯源**：两份快照 `<title>` 分别是《2025年8月社会融资规模存量/增量统计数据报告》，
   与 manifest 的 title 与 id 三方一致。

## 7. 🔴 对我自己上一份报告（TASK-001）的更正

TASK-001 验证报告里我给的 RUN **层级分解**用的是「按名字中 `/` 个数做直方图」，
**该方法被本次证伪**——名字自带斜杠（`"N/A"`、`137_条_/_12_页`、`<a>/</a>`）会被误记成更深层级。

| 锚 | 我当时写的（❌） | 正确值（按祖先存在性） |
|---|---|---|
| `4a12794a…` | 483 + 420 + 37 + 4 = 944 | **483 + 449(第1层) + 12(第2层) = 944** |
| `4f469e96…` | 489 + 445 + 37 + 4 = 975 | **489 + 474(第1层) + 12(第2层) = 975** |

**载重数字全部不受影响**：总数 944 / 975、顶层 483 / 489、子测试合计 461 / 486、
增量 `+31 = 6 顶层 + 25 子测试`、`0 条消失` —— 这些都与层级切分无关，结论不变。
dev-m1c3a-a 的 TASK-001 discovery 写的 `420 / 37 / 4` 是同一个错误，我当时还「确认逐字相符」。

⚠️ 教训是**求和自洽校验挡不住这类错**：`483+420+37+4` 与 `483+449+12` 都等于 944，
因为错的是**同一总数在各档之间的再分配**。自洽校验只能发现漏项，不能发现错分。

## 8. 复现方式

```bash
# 锚全部钉全 sha
git worktree add --detach ../wt-v002-pre  4f469e961e0c56c7730c250a8cc14d3c2efb653d
git worktree add --detach ../wt-v002-post bca01bd5b5a0b113497e7360941364cfbcbd8803
ABLATE_ANCHOR=cea6b3cb4c172c17adea7cf8a3a224b605f75d93 bash <scratchpad>/ablate-TASK-002-test-m1c3a-v1.sh
ZZ_CORPUS=<repo>/data/hestia-backfill-2026-08-14 go test ./internal/hestia/ -run TestZZ -v   # 语料探针
```

## 9. 结论

✅ **VERIFIED**。8 条 done_criteria 全部满足，且本次证据强度高于常规：

- 27 个 golden 值**逐条对快照原文**核对，不采信 discovery；
- 全量 138 篇语料**独立重跑**，抽满率、四类分布、8 篇混装的 16 个数字全部复现；
- DoD 指定的 A1 消融**直接观察到** `287000`/`4431` 这一对具体值；
- 我另设计 5 个生成集之外的变异，全部 KILLED 且红在预期断言；
- 其中 N3 补上了 dev 证明链缺的那一半（危害不可达 **∧** 守卫名副其实）。

**非阻断建议**（转 TASK-005 / TASK-007）：
1. `TakesBalanceNotStructureShare` 的否定式集合补 `RMBLoan`(61.2) 与 `Trust`(1)，凑满 8 分项（N5）。
2. `profiles.go` 空出后把 `tsfFlowArticleTotalRE` 挪回去并登记进 `allTemplateRegexps()`（dev 自己已注明）。
3. 引用量级差时用实测区间 **18.2×–606.6×**，不用「约 30 倍」。
