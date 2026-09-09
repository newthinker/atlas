# TASK-008 验证报告

- **任务**：`internal/hestia/CONTRACTS.md` 的 `## Sprint M3` §A–§D + 全 sprint 数字对账
- **owner**：dev-m3-a（记录员模式）｜**验证者**：test-m3-d｜**assignment_epoch**：1
- **验证时间**：2026-09-09T02:53Z ～ 03:30Z（UTC）
- **本 sprint 最后一个任务**（TASK-001–007 已全 `verified`，其中 TASK-007 由我判定）

## 🔴 结论：**VERIFIED**

7 条 done_criteria 全部 PASS。3 个 finding 均**不构成缺陷**，2 处口径差异经查实后确认**被验方是对的**。

---

## 1. 锚（纯 atlas 任务，`verify_baseline` 够得着）

| 锚 | 现读值 | 对照 |
|---|---|---|
| atlas HEAD | `41a19d8739092b72f17ee390284f6c33129fd27c` | == `verify_baseline.head` **逐字节相同** |
| discovery sha256 | `bb8fee2798b3779f4369e98c44823802e22703f56feb9375e1ccb53abd7d7281` | == `verify_baseline.discovery_sha256` **逐字节相同** |

⇒ **判定对象未漂移。** 与 TASK-007 不同，本任务无跨仓库锚，不需人工补闸。

### §B 表头采样锚 ≠ HEAD —— 已核实为**有意且成立**

§B 表头锚是 `fd933f493d15bfe7dea6bdbd5c4c740d34084e9c`（本任务开工时的 master），不是 merge 后的
`41a19d87…`。按派验指示，我核的是**「两锚之间 `.go` 确实 0 变动」**，而非「锚为何不等于 HEAD」：

```
git diff --name-only fd933f49… 41a19d87…   →  internal/hestia/CONTRACTS.md   （唯一一个文件）
git diff --name-only fd933f49… 41a19d87… -- '*.go'   →  0 个文件
```

⇒ 两锚之间只有本任务自己的 CONTRACTS.md 追加，**`.go` 零变动**，§B 六行数字在两个锚上等价。
dev 的处理原则（「数字的锚必须是它们实际来源，不是事后好看的那个」）**成立**。

---

## 2. done_criteria 覆盖矩阵（7 条）

3 条 `verify_by: review`、4 条 `manual`。按角色定义，`review` 类不强求断言，核内容是否写实。

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| **functional[0]**<br>§A 契约更正四条 `[review]` | A1 落点 `Wiki/Macro/PBOC/` + Spool **无条件覆写** `reviewed:false`（6.4/6.5 已批注）；A2 history 侧车 + **先侧车后契约**（6.2 已批注）+ **两个方向都有闸**（正向 `TestIngestHistoryLandsBeforeContractOnWriteFailure`、反向两条，反向系 QA 复审后补）+ 类型名 `ContractHistory` 冲突说明；A3 `source` 白名单 + `reviewed` 仍无条件覆写；A4 四信号权威实现 `signals.go` `Evaluate`，三期 golden **2/0/1** 两侧钉住 + 两条易错点（楼市/消费无黄灯、`corp_mlt_short_expand` 不参与四信号） | **PASS** |
| **functional[1]**<br>§B 六行逐格 + 每格来源 + O1/O2/O3 `[manual]` | 六行齐、**每行来源逐个回溯到来源载体且逐字一致**（§3）；全 sha 锚未缩短（`fd933f49…` 40 位）；「需求 line 44/1034 的 +2 是其自身笔误」注在；O1/O2/O3 三条齐全且各自结论完整 | **PASS** |
| **functional[2]**<br>§C 冒烟结转 + §D 结转 `[review]` | §C 写明「为什么不是留空」（前置排在 09-09~09-15 验收之后，本 sprint 09-08 定稿，**结构上不可能完成**）+ **冒烟五条判据原文**（实数 5 条 + 额外一条重放）+ 关键字订正 `Additional mount REJECTED` 且**标注「仅代码阅读，运行时未验证」**；§D 结转四条 + 第五条人执行前置**七项**（每项均注明为什么人做）+ **第六条三段式**（§4） | **PASS** |
| **boundary[0]**<br>采锚并核空 `[manual]` | discovery `boundary_anchor_and_empty_check` 留痕：写 §B **之前** `rev-parse` → `fd933f49…`、`git status --porcelain internal/hestia cmd/atlas` → **0 行**；§B 正文亦复述该纪律；我在当前树复核同为 **0 行** | **PASS** |
| **error_handling[0]**<br>对账不一致转 `blocked_clarification`，不自行圆数字 `[manual]` | **真被行使过，非空条款**（§5）：`transitions.jsonl` 实证 `02:13:28Z in_progress→blocked_clarification` → 写 `questions` → `02:16:39Z` 由 **dev 自己**转回 `in_progress`；四问一次问清；Leader 答复含「你搜错了空间，不是数不存在」 | **PASS** |
| **non_functional[0]**<br>只改一文件 / 纯追加零删除 / 两包测试仍绿 `[manual]` | 两锚间**唯一**变更文件 = `internal/hestia/CONTRACTS.md`，`.go` **0**；numstat **136 0**；删除行 `^-[^-]` = **0**；以提交本身 `git show --numstat 19ba2ed` 复核同为 136/0；`GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1` → **EXIT=0，2 ok / 0 FAIL** | **PASS** |
| **non_functional[1]**<br>交付流程 `[manual]` | 提交 `19ba2ed` 锚定 `docs(TASK-008):` 命中 1；merge **三判据**：拓扑 `IS-ANCESTOR` + `master^2 == 19ba2ed` + 内容 sha256 `df0bad28…` 在 master / 19ba2ed / 工作区磁盘**三处全同**；discovery 指针 `02:48:22Z` 落定、`dev_done` `02:48:23Z`（**晚 1 秒，顺序正确**）；worktree `wt-TASK-008-m3` 已拆（list 残留 0 且目录不存在） | **PASS** |

---

## 3. §B 六行来源逐个回溯（**比对来源只证转录忠实，故另加独立求值**）

派验指示要求「逐个回到来源载体核」。我先比对来源，再对可独立求值的项**不依赖 discovery 重算一遍**——
因为「与 discovery 一致」只能证明**转录忠实**，不能证明**数字为真**。

| § B 行 | 声明来源 | 回溯结果 |
|---|---|---|
| ① 覆盖率 `2785 / 2880 = 96.7014`（未覆盖块 89） | TASK-001 `verification.rework_round_1` | ✅ 原文含 `2783/2880 = 96.6319% → 2785/2880 = 96.7014%`、块 `91 → 89`，**逐字一致**；附 `cmd/atlas` **76.7%**（背对背基线 76.6%）← TASK-002 `verification.coverage_cmd_atlas` **逐字一致** |
| ② 导出面 **+4**、AST 守卫 `want 34 → 38` | TASK-001 `verification.ast_guard` | ✅ 逐字一致；**另独立求值**（见下） |
| ③ 真语料 `218=217+1` · `217=213+4` · `97=76+21`（单篇 28 + 合并组 69）· 冲突 **0** · 路由违反 **0** | TASK-001 `verification.real_corpus_regression` | ✅ **逐字一致** |
| ④ loom spool `35 + 4 = 39` | TASK-003 `verification.test_count_two_rulers` | ✅ 原文两把尺（静态 21+6+12=39 / 动态 `--- PASS` 39）同值、基线 35=18/6/11，**逐字一致** |
| ⑤ `unittest` **84**（`55 + 29`）；三期 golden **2/0/1** | TASK-007 `verification.tests`；**TASK-006 验证报告** | ✅ 84/55/29 逐字一致；三期 golden 用**内容串定位**（未照抄 DoD 给的行号，符合 non_functional[1] 纪律）落在 TASK-006 报告 128–130 行：`temp_score: 2 / 0 / 1`，`temp_known` 三期均 **4**；覆盖矩阵 `functional[4]` 亦记「三期温度 2/0/1」 |
| ⑥ 变异 prepare **23/23**；verify **13/13** | TASK-006 / TASK-007 `verification.mutation` | ✅ 两处**逐字一致**（含隔离副本、`ast.parse` 语法闸、锚点命中≠1 即 `sys.exit(3)`、每轮主工作区指纹校验） |

### 独立求值（不依赖 discovery，直接量代码）

**AST 守卫 `store_test.go:415`（用符号名定位，未照抄行号）**：

```
尺1 正则 "([^"]+)" 计数 : 38
尺2 逗号分割计数        : 38        两把尺同值
已排序: True ｜ 四个新增名全在 ｜ 38 - 4 = 34
```

⇒ §B 的「**+4**」与「`want 34 → 38`」**被独立证实为真**，不只是转录一致。

**表头两个 PR 现读**：loom #13 `OPEN` / `mergedAt=null` / `headRefOid=6d6c38f96901dc60f516ef2154283a213782ad5e`
（与 §B 逐行来源所记 loom 锚**逐字节一致**）；nanoclaw #5 `OPEN` / `mergedAt=null`。
两者均未合并，与 §D 把它们列为**人执行前置** ①④ **自洽**。

**dev 自证数字复算（派验指示：坦白不构成采信理由）**——全部属实：

| dev 自报 | 我的复算 |
|---|---|
| `scope` 136/0 单文件 | ✅ |
| `append_only` 删除 0、3519 → 3655 | ✅（3655 − 136 = 3519） |
| `suite` 两包 ok | ✅ EXIT=0，2 ok / 0 FAIL |
| M3 节内四节**绝对行 3525/3543/3580/3603** | ✅ 与我实测（M3 起 3523 + 相对 3/21/58/81）**逐个吻合** |
| `anchor_drift_check`：`1c7af818…`→`fd933f49…` 区间 10 提交、`internal/hestia`+`cmd/atlas` 变动 0 | ✅ 区间 **10**、变动 **0**、全区间 `.go` **0** |
| 自曝判据错误：`^### [A-D]\.` 过宽得 34 | ✅ 我实测全文确为 **34**，**坦白属实** |

---

## 4. 派验指示点名的三处 + 一处

### 4.1 §D 第六条：三段式，**未写成待办** ✅

三段标题逐字对应且完整：

- **实测行为**：改第 17 行 `tsf_stock_yoy: 7.40` 这类派生数据字段后 `verify.py` 仍 exit **0**
- **为何不判红**：规格明定封条自 `begin` 的下一行起，DoD 逐字要求，`verify.py` 忠实实现，
  三任验证者均按此判过 PASS ——**明写「这不是缺陷」**
- **待决问题**：不被覆盖的数据面；是否上移封条作用域是**规格层取舍**，**明写「必须由人决定」**

⇒ 忠实反映我 TASK-007 报的 F3，**没有任何一句会诱导后人去「修」一个按规格正确的实现**。

### 4.2 §B 的 `55 + 29 = 84`，且**性质**写出来了 ✅

§B 原文写明「该错**总数守恒**（两组都等于 84）⇒ **求和自洽校验恒过、永远不会被『加起来对不对』发现**。
这类『再分配型』错误只能靠**两把独立的尺各算一遍分层**抓到，验总数无效。判别式：**仪器出错时总数会
跟着变吗？**」——性质、失效机理、判别式三者齐全，比单纯记录数字有用得多。

并且它额外区分了两句话：「**『非缺陷』与『数字是对的』是两句话，不可合并**」——这个区分是准确的。

### 4.3 §B 六行每格都有来源 ✅（附口径说明）

`grep -c '来源 TASK-'` 得 **6**（**行数**口径，与 dev 自报一致）；`grep -o` 的**出现次数**是 **8**，
因覆盖率行与变异行**各带两个来源**（TASK-001+TASK-002、TASK-006+TASK-007），另有 1 处指向
TASK-006 **验证报告**。DoD 要求的是「逐行注明」——六个 bullet **全部有来源**，**性质满足**。

### 4.4 §B 那格对我 TASK-007 §0 的转述：**基本忠实，一处超出**（我是唯一能判这条的人）

先核「留痕 4 处」：我报告里 `严格度` 出现 **4** 次、`独立复核` 出现 **4** 次 ⇒ **属实**。

**忠实的部分**（对照我 §0 原文）：

| CONTRACTS §B 转述 | 我 §0 原文 | 判定 |
|---|---|---|
| 「dev 自证 13/13 KILLED，verifier 未独立复核」 | 「本报告接受该自报数字，未做任何独立复算，也未运行变异 harness」 | ✅ 忠实 |
| 「前三任在此阶段连续卡死（87/94/62 分钟），人类决定缩小第四任范围」 | 同义 | ✅ 忠实 |
| 「这是本 sprint 唯一一处证据强度低于其他任务的登记，**不要把它当作与其他数字同等硬**」 | 「仅有 dev 单方声明支持，不具备本报告其余结论所依据的独立证据强度」 | ✅ 忠实，**且比我原话更强调** |

**超出的半句**（见 §5-F2）：「**dev 侧的 harness 记录本身是充分的**」——这句我的 §0 从未说、
也**不可能说**：我明写「未做任何独立复算」，因此**不具备判断其充分性的依据**。

⇒ 前半「证据强度低的是复核环节，不是 dev 的工作」是对原措辞的**正确订正**（我确实只说「我没复核」，
未否定 dev 做过工作）；后半是一句**关于 dev 工作质量的正面断言**，而它恰好落在**唯一未经复核的那一格**上。

---

## 5. Findings（均**不构成缺陷**，不影响 VERIFIED）

### F1【低】§B 导出面那行的「位置」注是 **0-based** 索引，但未声明基准

discovery 与 §B 均记：`BuildContract=2 → BuildHistory=3`；`Contract.JSON=6 → ContractHistory.FileName=7
→ ContractHistory.JSON=8 → DefaultSignals=9`；`WriteContract=36 → WriteHistory=37`。

我实测（1-based 序号）分别是 **3/4**、**7/8/9/10**、**37/38** —— **每一个都差 1**。

- **0-based 假设完美解释全部 5 处差异，一个不差** ⇒ 非巧合。
- **不是我的仪器错**：38 项全为合法 Go 符号名、无误捕，总数与两把尺及 discovery 三方一致。
- **不影响 §B 任何数字**：`38` / `+4` / `34` 三个我都独立证实为真；位置注属逐行来源的补充说明。
- 但读者照「`ContractHistory.FileName`=7」去 `want` 数组第 7 项找，会找到 `Contract.JSON`。

**建议**：注明「（0-based）」或改用 1-based。**判非缺陷，留痕。**

### F2【低】§B 变异那格有半句超出我 TASK-007 §0 能背书的范围

原文：「证据强度低的是复核环节，不是 dev 的工作——**dev 侧的 harness 记录本身是充分的**」。

- 前半**正确**（见 §4.4）。
- 后半「记录本身是充分的」——我 §0 明写「未做任何独立复算」，**因此不具备判断其充分性的依据**。
  这句话在同一格里与后文「不要把它当作与其他数字同等硬」**彼此削弱**：若记录已经充分，何须提醒不要
  当作同等硬。
- ⚠️ 方向是「为 dev 辩护」而非淡化严格度下降，**未损害留痕的主目的** ⇒ 判**非缺陷**。

**建议措辞**：「dev 侧的 harness **记录详实**（隔离副本 / `ast.parse` 语法闸 / 锚点命中≠1 即
`sys.exit(3)` / 每轮主工作区指纹校验均有记载），但其**充分性未经独立复核**。」
——把「记录的详实程度」（可核查）与「证据的充分性」（未核查）分开说。

### F3【信息·归档提醒】遗留分支 `task/TASK-008-m3` 仍在

worktree 本身**已正确拆除**（`worktree list` 残留 0、目录不存在），DoD 第 7 条只要求拆 worktree、
**未要求删分支** ⇒ **不是 DoD 违反**。但同名 `task/TASK-00x` 分支跨 sprint 累积会挡住后续
`git worktree add -b` 开工。建议 Leader 在 sprint 归档时一并清理。

---

## 6. 两处口径差异：查实后**被验方是对的**（我差点误报）

如实记录，因为两次都是「我按一个口径算出不同的数」，若不查实就会变成两个假缺陷。

### 6.1 `14 基础 + 9 派生 + 4 信号 = 27` vs 我实测 25 —— **CONTRACTS 对**

我先按**实例口径**数两份 golden 笔记的 frontmatter，得 **25**（基础 14 ✅、信号 4 ✅、派生仅 **7**）。
查规格后确认：`note-format.md` 的派生指标表**明列 9 个**（`m2_yoy`、`m1_yoy`、`scissors`、
`tsf_stock_yoy`、`hh_mlt_monthly`、`hh_short_monthly`、`bill_ratio`、`temp_score`、`temp_known`），
两份 golden 因 `m2_yoy`/`m1_yoy` 在夹具中 omitzero 而各缺 2 个。

⇒ CONTRACTS 用的是**规格口径**（字段全集），成立；且它是更保守的表述——暴露在闸外的字段面是 27 而非 25。

### 6.2 来源注「6 处」 —— **行数口径，DoD 要求的性质满足**

见 §4.3。`grep -c` 行数 6、`grep -o` 出现次数 8，两个都对，口径不同；DoD 要的「逐行注明」成立。

---

## 7. 未做的事

| 未做 | 理由 |
|---|---|
| 独立复算 §B 的覆盖率 / 真语料 / loom 测试条数 | 三者的**来源 discovery 均在**且逐字一致，且 dev 已在采样锚上重跑复现；`anchor_drift_check` 经我复算证实区间 `.go` 变动为 **0**，数字在两锚上等价。**AST 守卫那项我做了独立求值**作为抽样验证，结果为真。 |
| 复核 verify.py 的 13 个变异 | 沿用 TASK-007 的范围决定（人类缩小范围）。§B 已就此**独立成格留痕**，本报告 §4.4 复核其转述忠实性。 |
| 运行时冒烟 | §C 已写明结构上不可能完成，结转下个 sprint。 |

## 8. 可复现锚

```
判定对象  atlas master @ 41a19d8739092b72f17ee390284f6c33129fd27c
          == verify_baseline.head，discovery sha256 亦相同 ⇒ 未漂移
交付提交  19ba2ed065146e2606e52c6ab7df540a4298b555（docs(TASK-008):）
§B 采样锚 fd933f493d15bfe7dea6bdbd5c4c740d34084e9c（两锚间 .go 变动 0，已核）
被验产物  internal/hestia/CONTRACTS.md @ sha256 df0bad28b22482d36748d60415ebd9f9039a125b530a2ef2dc8564faa06fd794
          M3 节 = 行 3523..3655；四小节绝对行 3525 / 3543 / 3580 / 3603
复现命令（锚钉全 sha）:
  git diff --numstat fd933f493d15bfe7dea6bdbd5c4c740d34084e9c 41a19d8739092b72f17ee390284f6c33129fd27c
  GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1
```

---

**验证者**：test-m3-d ｜ **判定**：**VERIFIED** ｜ 7/7 done_criteria PASS ｜ 3 findings 均非缺陷
**说明**：本报告 §4.4 / §5-F2 涉及对我自己 TASK-007 声明的转述核实——该部分我既是原声明作者又是验证者，
判断依据是我 TASK-007 报告 §0 的**原文逐字比对**（已在 §4.4 列表对照），不依赖记忆。
