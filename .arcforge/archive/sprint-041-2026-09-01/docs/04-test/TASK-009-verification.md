# TASK-009 验证报告

- **验证者**：test-m1c3b-a
- **被验交付**：commit `3ce09696a1fa4214aee6e2e1744e692af8914a97`（父 `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`），经 merge `7eda136` 进 master
- **verify_baseline**：head `e02056079baaa0c33bf25254e7cfe18b971547a7` / discovery_sha256 `4850c26e1db7145471ac5c50d8015ce0ca175aa107c67d7f1ca6835bd726ee41`
- **我测量时的 HEAD**：`053d1a96240819f4ca3394512226c85ff74ebea9`（下文每个数字都注明测自哪棵树）
- **assignment_epoch**：1
- **结论：PASS（verified）**

---

## 0. 漂移核对（AD-29）

测量期间 master 由 `e020560` 前移至 `053d1a9`（TASK-001 / TASK-002 两次 merge）。

| 检查 | 结果 |
|---|---|
| discovery sha256 | `4850c26e1db7145471ac5c50d8015ce0ca175aa107c67d7f1ca6835bd726ee41`，与基线**逐字相同**，未漂移 |
| 声明范围内文件（`writes` = `sections.go` / `sections_test.go`） | `git diff --numstat e020560 053d1a9 -- internal/hestia/sections.go internal/hestia/sections_test.go` → **0 行输出** |
| 内容判据（不只看拓扑） | `sections.go` 在 `e020560` 与 `053d1a9` 两版 sha256 均 `8116f0a0d0a76420451cce900b2808e3dfcec6d7853c5bec91e2a38cee479349` |

⇒ HEAD 前进落在**声明范围之外**，判定对象未变。

## 1. 越界申报核对

`3ce0969` 的 `git show --name-only` 只有 `internal/hestia/sections.go` 一个文件，在声明 `writes` 内；`go.mod` / `go.sum` **命中 0 条**（Global Constraint「无新增依赖」满足）。无越界。

---

## 2. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 「共 74 篇」订正为实测 55；注释写明取证方式与「随语料变须重采」；**订正前自己重采** | test | §3、§4 | **PASS** |
| functional[1] | 两个 55 的歧义写进注释（53 交集 / 各 2 篇 / 非包含关系） | test | §5 | **PASS** |
| functional[2] | R11 核实型：正确交付是 `parse.go` **零改动**，核实命令与输出入 discovery | review | §6 | **PASS** |
| boundary[0] | 只改注释不改行为；改动前后 PASS 集合逐条相同（**背对背**） | test | §7 | **PASS** |
| boundary[1] | 不顺手删死代码（`checkPeriodTypeSupported`） | review | §6 | **PASS** |
| error_handling[0] | 重采若不是 55，先怀疑判据、报 Leader、不静默改数 | test | §4（前件未触发，留证要求已满足） | **PASS** |
| non_functional[0] | gofmt / vet / test / 覆盖率 ≥95.9% / 无新依赖 / 编号带 milestone 前缀 | test | §8 | **PASS** |
| non_functional[1] | AD-4 交付流程（worktree / pathspec / merge 先于 dev_done / 自拆 worktree） | review | §9 | **PASS** |

---

## 3. 「改动全是注释」——两把机制不同的尺

**尺 A（文本，`git diff -U0` @ `32bc1e5`→`3ce0969`）**

```
新增行总数            = 22
新增行里非注释非空行  = 0
删除行总数            = 3
删除行里非注释非空行  = 0
```

尺 A 是行首 `//` 启发式，对 `/* */` 块与原始字符串字面量内的行会误判，故补一把不依赖文本形态的：

**尺 B（AST，`go/parser` 不带 `ParseComments` + `printer.Fprint` 后取 sha256）**

| 树 | 原文件 sha256 | 剥注释后 AST 打印 sha256 |
|---|---|---|
| `32bc1e5` | `c71cc593d3ec1e46…` | `ceefbf648ecb62a63ecf81086772c1d9ae24988e9fe59393ad0bb1e7481d586f`（5680 字节） |
| `3ce0969` | `8116f0a0d0a76420…` | `ceefbf648ecb62a63ecf81086772c1d9ae24988e9fe59393ad0bb1e7481d586f`（5680 字节） |

原文件不同、剥掉注释后逐字节相同 ⇒ **可执行代码一字未动**。dev 报的 `+22/-3 全部注释、非注释增删各 0` 属实。

---

## 4. 核心验收点：那个 55（我独立重采）

**判据来源独立于 dev**：dev 的旧判据取自归档 sprint-040 的 description 散文；我改从 git 历史取**代码原文**——`git show 23aa880c7488e0374e85eb62baa89b77c9a6f6c1^:internal/hestia/sections.go` 的 `detectExtractor`：

```go
if len(secs) > 0 { if err := checkSectionOrdinals(secs); err != nil { return "", err } }
_, hasTSF := findSection(secs, tsfSectionKeyword)
switch {
case hasTSF  && len(secs) == sectionsV2 /*8*/: return "rule@v2", nil
case !hasTSF && len(secs) == sectionsV1 /*6*/: return "rule@v1", nil
default: return "", error
}
```

**我自己的实现**（临时 package 内测试，读 `data/hestia-backfill-2026-08-14/manifest.json`，跑完即删；测量树 `e02056079baaa0c33bf25254e7cfe18b971547a7`）：

```
[M0] manifest.articles 总数 = 218
[M1] parseTitle 失败 = 0 ; manifest.title 与 meta ArticleTitle 不一致 = 0
[M2] kind 社会融资规模增量统计数据报告 = 69
[M2] kind 社会融资规模存量统计数据报告 = 69
[M2] kind 金融统计数据报告 = 80
[M3] 分母（kindFinance）= 80
[R1] 尺1 正向累加被拒 = 55 （拒因 default=54 ordinals=1）
[R2] 尺2 通过 rule@v1=21 rule@v2=4 合计=25 ; 分母-通过 = 55
[R3] 尺3 布尔补集重算被拒 = 55
[R4] 尺4 switch-only 分母=80 被拒=55 通过=25
```

四把尺（正向累加 / 通过作差 / 布尔表达式重算 / 只套注释里逐字写出的那条 switch）**全部得 55**，分母 80、通过 25、55+25=80。**与 dev 报的数字逐条一致，与注释里写的数字逐条一致。**

> ⚠️ 特意**不**采用「分层桶求和 = 80」作证据——该校验总数守恒，对再分配错误零鉴别力。dev 在 discovery 里也明确排除了它。

### 关于 Leader 点名要核实的那句话

dev 声称「尺 2 独立重算、不复用尺 1 中间结果，且没拿分层求和当证据」。

- **可经检视核实的部分**：discovery 明文写着不以分层求和为证据，且它在 `key_findings` 里**另外**列出了分层表并标注了差异 —— 两者分列，未混用。属实。
- **不可经检视核实的部分**：dev 的重采代码 `zz_repro_task009_m1c3b_test.go` **未提交、已删除**（`git log --all --diff-filter=A -- '*zz_repro_task009*'` 无输出；磁盘上无残留）。因此「尺 2 是否复用了尺 1 的中间结果」**我无法通过检视源码求值**，我不能声称核实了它。
- **为什么这不影响判定**：那句话是 55 的**支撑证据**，而 55 本身已被我用**完全独立的实现 + 独立来源的判据 + 四把机制不同的尺**直接证实。dev 的尺 2（`80 − 25`）确实与尺 1 共用同一次判据求值，只能挡计数器错误、挡不住判据错误；我的尺 4（另起循环、另写布尔式、不带 ordinals 闸）补上了这一层。**结论不依赖那句无法核实的自陈。**

---

## 5. 两个 55 的集合关系（实测逐条对上）

我的测量（同一树 `e020560`）：

```
[S1] |A 旧判据挡掉|=55 |B 全部月报|=55 交集=53 仅A=2 仅B=2
[S2] 仅A: 2020年前三季度金融统计数据报告
[S2] 仅A: 2022年前三季度金融统计数据报告
[S3] 仅B: 2024年3月金融统计数据报告
[S3] 仅B: 2025年3月金融统计数据报告
[S4] 月报 55 篇中 hasFX=true 2 篇 / hasFX=false 53 篇
```

对照注释原文，逐项命中：

| DoD 要求写明的 | 注释里的原话 | 我的实测 |
|---|---|---|
| 旧判据挡掉的 55 = 53 月报 + 2 前三季度报 | 「旧判据挡掉的 55 = 53 篇月报 + 2 篇**前三季度报**（2020-09 / 2022-09，5 节、有外汇节）」 | 仅A = 2020/2022 前三季度报，实测 `5节 hasTSF=false hasFX=true q1_q3` ✔ |
| 月报的 55 = 那 53 篇 + 2 篇 3 月报 | 「月报的 55 = 那 53 篇 + 2 篇**3 月报**（2024-03 / 2025-03，6 节、有外汇节）」 | 仅B = 2024-03/2025-03，实测 `6节 hasTSF=false hasFX=true monthly` ✔ |
| **各有 2 篇是对方没有的，不是包含关系** | 「交集是那 53 篇，**各有 2 篇是对方没有的，不是包含关系**」 | 交集 53 / 仅A 2 / 仅B 2 ✔ |

注释另给了机制解释「3 月报多一个外汇节恰好凑成 6 节，从旧判据的缝里漏了过去」——与实测（那 2 篇 6 节、`hasFX=true`、判为 `rule@v1` 通过）一致。

「55 篇月报中 53 篇无外汇节」也与文件里既有的 `fxSectionKeyword` / `detectExtractor` 处的 53/55 对得上（`[S4]`）。

---

## 6. R11：`parse.go` 零改动（正确交付看起来像没干活）

| 检查 | 结果 |
|---|---|
| `parse.go` @ `32bc1e5` 的 sha256 | `9f991c99d4ed97daff53c2f9fdbeff3ee01f08b50bf32aa29f2ce78aa8ec905b` |
| `parse.go` @ `e020560` 的 sha256 | `9f991c99d4ed97daff53c2f9fdbeff3ee01f08b50bf32aa29f2ce78aa8ec905b` |
| 三条关键语句是否在 dev 开工前就已在位 | 是。`git show 32bc1e5:internal/hestia/parse.go \| sed -n '284,296p'` 逐字含「不要因此删掉这个函数」「新增第六种 `period_type` 必须明确表态」「`TestEveryPeriodTypeHasAnExplicitSupportDecision` 遍历 `validPeriodTypes` 逐个来问」 |
| 是否写了重复注释（本条点名的最可能错误产出） | 否。整个 `3ce0969` 只改 `sections.go` |
| discovery 是否留了核实命令与输出 | 是。`key_findings[0]` 含 `verification_command` + `output_verbatim` + `checked_at_head` |
| boundary[1]「不顺手删死代码」 | `checkPeriodTypeSupported` 完整保留（`parse.go` 逐字节未动即已证明） |

**⇒ 「零改动」是本条的正确交付，dev 做对了。**

---

## 7. 行为不变：背对背 PASS 集合比对

隔离到 `32bc1e5`（PRE）与 `3ce0969`（POST）两棵独立 worktree —— 刻意**不**用当前 master，因为 master 上还有 TASK-001/002/004 的改动，拿它比会把别人的测试算到本任务头上。

交错顺序 **POST / PRE / PRE / POST**（背对背 × 同等负载）：

| 轮次 | 顶层 `^--- PASS` | 含子测试的全部 `--- PASS` |
|---|---|---|
| POST r1 | 556 | 1220 |
| PRE  r1 | 556 | 1220 |
| PRE  r2 | 556 | 1220 |
| POST r2 | 556 | 1220 |

四次退出码均 0，`ok github.com/newthinker/atlas/internal/hestia`。

**不止比计数，还比集合**：取全部 PASS 测试名（含子测试）、`LC_ALL=C sort` 后取 sha256——

```
pre_r1  lines=1220 sha=e7b32bbcf85d6faa…
pre_r2  lines=1220 sha=e7b32bbcf85d6faa…
post_r1 lines=1220 sha=e7b32bbcf85d6faa…
post_r2 lines=1220 sha=e7b32bbcf85d6faa…
两两 diff 行数矩阵：全 0
```

> 取证过程中的一处仪器问题，一并记下以免后人重蹈：起初用默认 locale 的 `sort`，`pre` 与 `post` 的子测试名单出现 4 行 diff。查证发现**同一棵树两次运行之间也出现同样的 4 行 diff**（`TestFindSectionResolvesAllT5Keywords/2025/国家外汇储备` 只是位置移了一格），是 CJK 串在默认 locale 下排序不稳定所致，与改动无关。换 `LC_ALL=C` 后四份名单 sha256 完全一致。**这是工具的性质，不是被测对象的性质。**

---

## 8. 非功能门禁

| 项 | 结果 | 测量树 |
|---|---|---|
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**2 项，PRE 与 POST 完全相同，无新增项**） | `32bc1e5` / `3ce0969` / `053d1a9` 三棵树均如此 |
| `go vet ./internal/hestia/... ./cmd/...` | **输出 0 行** | `3ce0969`、`053d1a9` |
| `go test ./internal/hestia/... -count=1` | 全绿 | `3ce0969`、`053d1a9` |
| 覆盖率（`go test -cover`） | **95.9%** @ `3ce09696a1fa4214aee6e2e1744e692af8914a97`；**95.9%** @ `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`（背对背对照）；**95.9%** @ `053d1a96240819f4ca3394512226c85ff74ebea9`（当前 master，干净树复测） | 三树 |
| 不低于基线 95.9% | 满足（等于） | — |
| 无新增依赖 | `go.mod`/`go.sum` 在 `3ce0969` 命中 **0** 条 | — |
| 注释里任务编号带 milestone 前缀 | 新增行里唯一的编号引用是「（**M1c-3b 的 TASK-009** 重采」，带前缀 | — |

> 覆盖率是在移除我自己的临时重采测试之后重测的——临时测试会执行 `splitSections`/`parseTitle` 等，留着会抬高覆盖率，那就成了我的仪器污染被测对象。

---

## 9. AD-4 交付流程

| 要求 | 核实 |
|---|---|
| 自己的 worktree、分支带 `-m1c3b` 后缀 | discovery 记 `task/TASK-009-m1c3b`；分支名符合 |
| 提交显式 pathspec | `3ce0969` 只含 `internal/hestia/sections.go`，无夹带 |
| commit message 锚定 `<type>(TASK-009):` | `docs(TASK-009): 订正 sections.go 的「共 74 篇」为实测 55，并写明两个 55 的歧义` ✔ |
| **merge 必须在 `dev_done` 之前** | merge `7eda136` committer 时间 `2026-08-31T23:49:21Z`；`dev_done` 迁移 `2026-08-31T23:51:25Z`（`tasks/transitions.jsonl`）⇒ **早 2 分 4 秒** ✔ |
| 自拆 worktree | `git worktree list` 中**无** `wt-TASK-009-m1c3b` ✔ |

---

## 10. INFO（不构成缺陷，不影响判定）

**INFO-1｜拒因归属的一处细节，两条路径总数都是 55。**
套**完整**的旧 `detectExtractor`（含 `checkSectionOrdinals` 前置闸）时，55 篇里 54 篇落 `default`、另 1 篇（4 节月报）先被 ordinals 闸拒；套注释里**逐字写出的那条 switch**（不带前置闸）时，55 篇**全部**落 `default`（尺 4 实测 `被拒=55`）。注释写的是「套上面那条旧判据，落进 default 的即被挡下」——按它字面复刻得 55，**自洽**。此处仅提示后人：若改照完整旧函数复刻，`default` 那一格会是 54，总数仍是 55。

**INFO-2｜dev 的重采代码未提交，「尺 2 独立性」这句自陈不可经检视核实。**
详见 §4 末段。DoD 只要求「取证方式写进 discovery」，未要求提交复刻代码；而**我按 discovery + 注释里写下的取证方式，独立重建并复现出了 55** —— 这恰恰是本任务真正要保护的性质（「74」当年之所以成为坏数字，就是因为取证方式没留下）。该性质**已被兑现**。

**INFO-3｜与归档 sprint-040 布局表的差异，已由第二方证实站在 dev 这边。**
归档记「5 节月报 43 / 4 节月报 3」，dev 实测 42/4，**我的独立实现同样得 42/4**（`[L] 5节…monthly 被拒(default) -> 42`、`4节…被拒(default) -> 3` + `4节…被拒(ordinals) -> 1`）。合计两边同为 46，被拒总数两边同为 55，对本任务要订正的数字无影响。Leader 已裁决「不追查、留证即可」。留一条线索：多出的那 1 篇正是被 `checkSectionOrdinals` 拒掉的 4 节月报，归档很可能把它归进了 5 节那一格。

**INFO-4｜未跑 code-simplifier，是有据偏离而非缺陷。**
dev 已在 discovery `decisions` 主动申报，Leader 已裁决接受（本次 diff 可执行代码 0 行——§3 的 AST 判据独立证实了这一点；而 simplifier 对注释的「简化」会删掉 DoD 明确要求写明的取证细节）。**不重复判罚。**

**INFO-5｜validator 的 `scope-writes-outside-packages` 告警不作判罚依据。**
本轮 validator 报 11 条该告警，其中 TASK-009 占 2 条——但它对本 sprint **全部 5 个在途任务的每一条 `writes` 都命中**（11 条恰等于各任务 `writes` 条数之和）。对「`packages` 写包路径、`writes` 写文件路径」这一标准形状它恒真，零鉴别力。validator 退出码 0，`✓ 任务图校验通过（12 个任务）`。

---

## 11. 结论

八条 done_criteria **逐条 PASS**，无未覆盖项、无失败用例、无覆盖率不足。

核心数字 55 由**独立来源的判据 + 独立实现 + 四把机制不同的尺**复现；「行为不变」由**文本尺 + AST 尺**两个机制与**背对背四轮 PASS 集合 sha256 全等**共同证实；R11 的「零改动」经 `parse.go` 两版 sha256 相同 + 三条关键语句在 dev 开工前即在位而确认为**正确交付**。

**判定：VERIFIED。**
