# TASK-014 验证报告

**任务**：QA 两轮返工——一行接线 + 八处失真/悬空注释 + CONTRACTS 三条订正
**验证者**：test-m1c4-b　**日期**：2026-09-03　`assignment_epoch=1`

## 结论：**VERIFIED**

8 条 `done_criteria` **全部 PASS**（`test`×2 / `review`×5 / `manual`×1）。

🔴 **但有一条必须立刻处理的操作性发现**（不影响判定，见 §2）：dev 的 worktree `wt-TASK-014b` 未拆除，且其中有 **174 行未提交的 CONTRACTS 内容** —— 正是 Leader 计划在 `accepted` 之后自己补的那三条，**外加一条切库判据恒真的订正**。**一个 `git worktree remove --force` 就会全部丢失。**

---

## 0. 采样锚与基线核对

| 项 | 记录值 | 实测 | |
|---|---|---|---|
| `verify_baseline.head` | `4735b817a441d61055c793a34dfca65b1309c521` | 相同 | **零漂移** |
| `discovery_sha256` | `c40e4376ae3144ab22f7b381bfc42be60ecad602a14c64abd037fbc6d74c583d` | 相同 | 一致 |

我自核，未采信 Leader 给的值。隔离对：base `c6f33b71d9d123a8110c668a75b451a8fd206a49` / post = 判定对象。

**声明范围**：`writes` 7 项、实改 7 项，两个差集均空。

---

## 1. 完成标准覆盖矩阵

| # | verify_by | 判据（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F0 | **test** | 唯一代码改动：`gateCorpLoanReconcile` 经 `bandLimitRatio` | 非注释非空行改动**恰 2 行**；🔴 **改前红实测**（§3） | **PASS** |
| F1 | review | 五处失真注释订正，保留原文 + 标注失真时点 | 删除的 8 行里 3 行为 2→1（信息无损，§5）；其余为含死符号的失真表述被订正文本替换 | **PASS** |
| F2 | review | 两处悬空旧函数文档 | godoc **双向对照**，我在两棵树现采（§4） | **PASS** |
| B0 | review | CONTRACTS 两条被 QA 证否的描述更正 | 🔴 **我自己复现了 M5**（§6） | **PASS** |
| B1 | review | 记「同名不同义」的两个 `median` | `CONTRACTS.md:2765` 专节 + 最小复现 `[1,2,3,4]` ⇒ `2.5` vs `2` + 「两边都不要改」 | **PASS** |
| E0 | review | 订正措辞须写「TASK-005 接上写入方后失真」，**不得**写「引入时就假」 | `profiles.go:589` 原文：「🔴 **它写下时是对的，不是「引入时就假」**（时序已实测，本任务复核过）」 | **PASS** |
| N0 | **test** | 门禁 | §8 | **PASS** |
| N1 | manual | `git log -S` 扫描写进 discovery；例行含「自己拆 worktree」 | 扫描在 discovery 且**另扫出 4 处同类残留**；⚠️ worktree 未拆 —— 判定理由见 §2.3 | **PASS** |

**未覆盖的完成标准：无。**

---

## 2. 🔴 操作性发现：174 行未交付内容困在残留 worktree 里

### 2.1 事实

```
/Users/zuowei/workspace/go/src/github.com/newthinker/wt-TASK-014b
  分支 task/TASK-014-m1c4-r2，tip == master（0 commit 领先）
  git status --porcelain →  M internal/hestia/CONTRACTS.md
  git diff --numstat     →  174    7    internal/hestia/CONTRACTS.md
```

### 2.2 内容（我只读查看，未改动）

| 块 | 是否已交付到 master |
|---|---|
| **开口 e**：族内量级核对是**刻意的告警级**判据，`famViol` 到不了 `checkLoadIdentities` | 🔴 **0 次** |
| **开口 f**：`renderLoadReport:90` **无条件**打印「四道恒等式: 全部成立 ✓」 | 🔴 **0 次** |
| **开口 g**：合计句是**二选一**不是双写（`selectRMBFlowByCaliber` 的 `hasCum` 分支） | 🔴 **0 次** |
| **开口 h**：四条只登记不修的发现 + **孤儿引用位置清单** | 🔴 **0 次** |
| 🔴 **§H-3 切库判据恒真的订正**（`grep -m1 '四道恒等式'` 恒通过 ⇒ 改成看退出码） | 🔴 **0 次** |
| `deposit_sum_drift_max` 量化：`0.03→0.30` 时入权威表 **76→93**（它挡着 17 个观测） | 🔴 **0 次** |
| M5/M6/M11 与 `magnitude_sanity` 的「性质不同、不要并列」订正 | 🔴 **0 次** |

**核实方法**：在 `4735b81` 的 `CONTRACTS.md` 里 grep `开口 e` / `开口 f` / `开口 g` / `开口 h` / `原判据恒真` / `M11` / `入权威表 93` —— **全部 0 次**。交付版 CONTRACTS 仅 `+57/-4`，未提交版是 `+174/-7`。

**其中 §H-3 那条最要紧**：它订正的是**给人照抄的切库清单**（唯一不可逆的一步）里一道**恒真**的判据 —— 原判据 `grep -m1 '四道恒等式' | 必须「全部成立 ✓」`，而 `renderLoadReport` 无条件打印那句话 ⇒ 恒等式不成立时正文照样写「✓」。**那次切库安全不是靠这道判据兜住的，是靠退出码碰巧为 0。**

### 2.3 为什么不因此判 REJECT

- **DoD 里没有这三条**（Leader 已确认消息到达晚于 `dev_done` 15:54:17）；
- N1 的「自己拆 worktree」是**例行项**，而 dev 保留它**正是因为**在写 Leader 交棒后要求的内容 —— **拆掉会毁掉这些内容**。此处不拆是更优选择；
- 交付物本身（`4735b81`）8 条判据逐条满足。

⇒ 记为**发现并紧急上报**，请求 Leader：**先抢救再清理**（`git -C ../wt-TASK-014b diff -- internal/hestia/CONTRACTS.md > 补丁` 或直接在那棵树里提交），**不要直接 `git worktree remove --force`**。

⚠️ CLAUDE.md 的「孤儿 worktree 由 Leader 用 `--force` 收」这条纪律，在这里恰好会造成损失。

---

## 3. F0 —— 改前红（本次修复唯一的守卫价值所在）

**射程**：`git diff -U0 c6f33b7 4735b81` 取 5 个实现文件，去掉注释行与空行后**恰剩 2 行**：

```
-	if r <= b.tol {
+	if r <= bandLimitRatio(b, math.Abs(total)) {
```

**改前红实测**：把 post 的 `validate_test.go`（纯新增 101 行）拷进 base 树，在 `c6f33b7` 上跑：

```
--- PASS: TestCorpLoanBandRespectsAbsTol            ← band 层，改前改后都绿
--- FAIL: TestCorpLoanReconcileGoesThroughBandLimitRatio
    🔴 gateCorpLoanReconcile 没走 bandLimitRatio …… 实际调用=[pickCaliberBand]
```

⇒ **dev 的自陈逐字属实**：band 层那条自己说「单独没有守卫价值」，接线层那条**改前确实红**。

### 3.1 AST 是否真的不被注释骗到 —— 我做了决定性实验

它的实现：`parser.ParseFile(token.NewFileSet(), "validate.go", nil, 0)`（**mode 0 ⇒ 注释根本不进 AST**）+ `ast.Inspect(fn.Body, …)`（**只遍历函数体**）。两重理由。

**但我没有停在读实现**：往 base 树的 `gateCorpLoanReconcile` **函数体第一行**注入

```go
// 变异探针：本行提到 bandLimitRatio，但函数体里并没有调用它。
```

重跑 ⇒ **仍然 FAIL** ✅。它确实骗不到。

⚠️ **INFO —— dev 给的理由不够精确**：它说「那个文件的注释里恰好多处提到 `bandLimitRatio` ⇒ 子串版会平凡为真」。实测分类：

| 树 | 注释 | 函数声明 | 代码调用 |
|---|---|---|---|
| base `c6f33b7` | **1** | 1 | 1（兄弟闸 `gateDepositSum`） |
| post `4735b81` | 4 | 1 | 2 |

⇒ 「多处注释」描述的是 **post 树**；而决定「子串判据是否平凡为真」的是**必须变红的那棵树（base）**，那里的平凡为真主要来自**函数声明 + 兄弟闸调用点**，注释只有 1 处。**结论（用 AST）对，理由不够精确。**

---

## 4. F2 —— 悬空文档（双向对照，我在两棵树现采）

⚠️ 我先用「提及该符号的行数」去数，得 `checkSectorCaliber` 2→1、`selectRMBCumulativeFlow` 6→3，与 dev 的 1→0 对不上。**去看内容**才知道口径不同：dev 数的是**悬空的文档段**，我数的是**提及行**（含刻意保留的历史标注与折行续行）。

换成正确签名（**文档段以已删符号名开头**）：

| 项 | base | post | |
|---|---|---|---|
| `checkSectorCaliber` 悬空段 | **1**（错挂在 `type sectorCaliber int` 上） | **0** | ✅ |
| `selectRMBCumulativeFlow` 悬空段 | **1**（另一处是折行续行，不是段） | **0** | ✅ |
| 对照组 `func selectRMBFlowByCaliber` | 1 | **1** | ✅ 未误删 |
| 对照组 `type sectorCaliber` | 1 | **1** | ✅ |
| 对照组 `func sectorCaliberOf` | 1 | **1** | ✅ |

**dev 的方法（在 base 树现采对照组）是对的**：没有对照组，「改后 0」也可能是命令写错。我按同法独立采了一遍。

---

## 5. F1 —— 5 行删除的信息无损

dev 称「两句话在紧随其后的正确文档里逐字重复，删除后各仍存在 1 次」。逐条核（`base 次数 → post 次数`）：

```
2 → 1   捕获组：1=期次 2=口径 3=方向词 4=数值 5=单位。
2 → 1   两个维度都要判：外币孪生句的期次前缀同样是「全年/上半年」，只判期次会取到
2 → 1   「全年外币存款增加2135亿美元」。
```

**3 条物理行 = 2 个句子**（第二句跨两行）⇒ 「两句各仍存在 1 次」**精确成立**，信息损失为 0。其余删除行（1→0）都是引用死符号 `checkSectorCaliber` / `selectRMBCumulativeFlow` 的失真表述，已被订正文本替换 —— 那正是本任务要修的东西。

---

## 6. B0 —— M5 我自己复现了

**基线**（锚 `c6f33b7`，未变异）：入权威表 **76**、族内量级核对 **0 违反**、样本不足未判 **84**。

**M5 变异**（`profiles.go` 住户 scope 两项 `ytd↔mom` 互换，二进制 sha 确认与基线不同）：

| 项 | 基线 | M5 后 | CONTRACTS 记 |
|---|---|---|---|
| 入权威表 | 76 | **66** | 76 → **66** ✅ |
| 族内量级核对违反 | 0 | **2** | **2 违反** ✅ |

违反明细：

```
monthly loan_hh_short: median|ytd|=1503 (n=26) < median|mom|=3238 (n=27)  ← 整族可能写错列
monthly loan_hh_mlt:   median|ytd|=3316 (n=26) < median|mom|=8084 (n=27)  ← 整族可能写错列
```

**与 CONTRACTS 的「2 违反：`monthly loan_hh_short` / `monthly loan_hh_mlt`，全中」逐字相符。**
主工作区 `profiles.go` 指纹变异前后一致，变异树已 `git checkout` 还原。

### 6.1 「独立复现」声明属实 —— 且有时序证据，不是采信自陈

dev 的 discovery 写「未采信未落盘的 QA 报告（`05-review/` 目录**实为空**）」。时序核实（全 UTC）：

```
15:29:52  dev 认领 in_progress
15:46:00  QA 落盘 round1-review.md / round2-adversarial.md
15:52:47  QA 落盘 round2-lens-findings.md
15:54:08  dev 写 discovery
15:54:17  dev 转 dev_done
```

⇒ dev 工作窗口的**前 16 分钟目录确实是空的**，报告在它交棒前 8 分钟才落盘。**它的独立复现是有条件基础的**，而我的复现与它、与 QA 三方收敛。

⚠️ INFO：「目录实为空」这句在它交棒时**已过期**（它没有回头再看一眼），属时间索引陈述未复检，不影响复现结论。

### 6.2 84 分桶（两把独立的尺）

| 尺 | 结果 |
|---|---|
| awk | `annual 21 / h1 21 / q1 21 / q1_q3 21`，无 `monthly` |
| python（独立解析） | 同上；求和 **84**，与报告抬头自报的 84 **一致**；`monthly` 出现 **0** 次 |

与 CONTRACTS 的「84 = 四档各 21、monthly 0 次」相符。

---

## 7. 三条已知缺口（Leader 承接，未据此判红）

Leader 在 `dev_done`（15:54:17）**之后**才发给 dev，DoD 里没有这三条。我核实的是**它该做对的那部分**：

**那 5 处 `writes` 之外的孤儿引用，dev 一个都没碰** —— `calibrate.go` / `extract_test.go` / `profiles_test.go` / `parse_test.go` / `testdata/README.md` 在 `c6f33b7..4735b81` 的改动条目**均为 0**，且逐条确认**都在 `writes` 之外**（碰了反而是越界）。**这是正确处置。**

⚠️ 但见 §2：**这三条的内容 dev 其实已经写好了 174 行，只是没提交。**

---

## 8. N0 —— 门禁（我在判定对象 `4735b81` 上重跑）

| 项 | 判据 | 实测 |
|---|---|---|
| `go test ./... -count=1` | 全绿 | **全绿**（无 FAIL 行） |
| 覆盖率 | ≥96.1% | **96.3%** |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | **零输出** |
| `gofmt -l internal/hestia cmd/atlas` | 只有两处既有欠账 | **恰为** `backtest_test.go`、`crisis_test.go` |
| `go.mod` / `go.sum` | 0 | **0** |
| 待 merge 的 `task/*` 分支 | 0 | **0**（全部为 master 祖先） |

⚠️ 我第一次数「待 merge」时误报了 1 条 —— `git branch --list` 的输出带 `+ ` 前缀（表示该分支被别的 worktree 检出），我把 `+ ` 当成了 ref 名的一部分。换 `git for-each-ref --format='%(refname:short)'` 后正确。

---

## 9. INFO

1. **AST 选型的理由不够精确**（§3.1）：结论对，但「注释里多处」描述的是 post 树；base 树上的平凡为真主要来自函数声明 + 兄弟闸调用点。
2. **「05-review 目录实为空」在交棒时已过期**（§6.1），不影响复现结论。
3. ⚠️ **我自己三次仪器错，都靠「去看内容 / 换仪器」纠正**：① godoc 数「行」而非「段」（§4）；② `git branch --list` 的 `+ ` 前缀（§8）；③ 用 `stat -t` 给本地时间加了字面 `Z`，差点按错时区推时序（§6.1 已用真 UTC 重取）。记在这里是因为**三次都是「口径/前缀/时区」这类边角**，而它们各自都会给出一个**看起来合理的错数**。

---

## 10. 判定小结

- 8 条 `done_criteria` 逐条 PASS，每条锚定我自己跑出的输出。
- 关键处做了**实验而非阅读**：改前红（拷测试到 base 树实跑）、AST 抗注释（注入注释探针）、M5（真变异 + 真语料）、godoc 双向对照（两棵树现采）、84 分桶（两把尺）。
- 唯一的重大发现是**操作性的**，不是交付缺陷：174 行已写好但未提交的 CONTRACTS 内容困在残留 worktree 里，**清理前必须抢救**。

**⇒ VERIFIED**

---

## 11. 🔴 §2 与 §8 的订正（写于裁决之后，2026-09-02T16:1x Z）

**§2 与 §8 的原文在我测量的那一刻是对的，但在我写完报告之前已经过期。** 两处都必须订正。

### 11.1 事实变化

我在 §2 记录「`wt-TASK-014b` 有 174 行**未提交**改动，一个 `--force` 就没了」。之后 dev 自己：

```
705f9aecf795e31a1fc26ad43a0f551d386acf88   parent = 4735b81
  fix(TASK-014): CONTRACTS 追加 QA 两轮的四条开口与三处订正（本地暂存，待 verified 后再请合并）
  176  7  internal/hestia/CONTRACTS.md
  提交时刻 2026-09-03T00:08:55+08:00  =  2026-09-02T16:08:55Z
```

**并随后拆除了 worktree**（`git worktree list` 现已无 `wt-TASK-014b`，文件系统上也不存在）。

⇒ **内容已安全落在 commit 里**，不再有被 `--force` 抹掉的风险；DoD N1 的「自己拆 worktree」**也已完成**。

### 11.2 订正后的状态

| 项 | §2/§8 原文（测量于裁决前） | 订正后（现状） |
|---|---|---|
| 那 176 行 | 未提交，困在残留 worktree | **已提交为 `705f9ae`** |
| `wt-TASK-014b` | 残留未拆 | **已拆除** |
| 待 merge 的 `task/*` 分支 | **0** | 🔴 **1 条**：`task/TASK-014-m1c4-r2` 领先 master **1 个 commit** |
| 那些内容在 master | 0 次 | **仍是 0 次** —— 未合入 |

逐项核实（`705f9ae` vs `master`）：`开口 e` 1/0、`开口 f` 2/0、`开口 g` 2/0、`开口 h` 1/0、`原判据恒真` 2/0、`M11` 3/0、`入权威表 93` 1/0。

### 11.3 风险等级下调，但**没有归零**

- **原风险**（未提交 + 残留 worktree）：会被例行 `--force` 清理**永久销毁** ⇒ 已消除。
- **现风险**（已提交、**未合入**）：内容不在 master ⇒ **`go test` / 门禁 / 任何读 master 的人都看不到它**；而 `task-completed.sh` 的 `git log --grep` 不带 `--all`，**未合并分支对门禁结构性不可见**。
- dev 的提交信息明写「**待 verified 后再请合并**」⇒ 它在等 Leader merge。**本任务现已 `verified`，那个等待条件已满足。**

⚠️ 这正是本 sprint 记过的那条结构性缺口：**「dev 交付完等 Leader merge」是流水线上唯一没有活性保障的环节** —— 它在文件层与「还在写代码」同形，`stale-dispatch` 刻意不含 `in_progress`，idle hook 也覆盖不到。此处它以另一种形式复现：**任务已 `verified`，而一个 commit 停在分支上等人。**

⇒ **给 Leader 的动作项**：合并 `705f9ae`（或据它重写自己的版本，但那会重复 176 行已写好的工作）。其中 **§H-3 切库判据恒真的订正最要紧** —— 它修的是给人照抄的、唯一不可逆步骤的判据。

### 11.4 对判定的影响：无

`verify_baseline.head` = `4735b817a441…`，判定对象是 master 上的那棵树；`705f9ae` 是**判定之后**产生的、**判定对象之外**的内容。8 条 `done_criteria` 的结论不变，**仍为 VERIFIED**。

### 11.5 我为什么把这段写进报告而不是只发消息

§2 与 §8 的原文若原样留着，就是两句**在写下时为真、在被读时为假**的陈述 —— 本 sprint 反复记的正是这一族（「说谎的状态之所以看起来正常，是因为它曾经是真的」）。报告是持久载体，消息不是。
