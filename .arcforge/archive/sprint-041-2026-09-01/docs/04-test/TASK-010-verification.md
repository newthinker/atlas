# TASK-010 验证报告

- **验证者**：test-m1c3b-a
- **被验交付**：commit `21815beda194398b1ee9b0777e969f4f102b8ac3`（`docs(TASK-010): …`，只动 `internal/hestia/CONTRACTS.md`）
- **verify_baseline**：head `21815beda194398b1ee9b0777e969f4f102b8ac3` / discovery_sha256 `72d21a6bdd01b0dfe5628125e832ad8c0a91e2ca3a98314dd63c70856613e3e7`
- **我测量时的 HEAD**：`21815beda194398b1ee9b0777e969f4f102b8ac3`（= 基线，**零漂移**）
- **assignment_epoch**：1
- **结论：PASS（verified）** —— 含一条**判据零信息量**的重要发现，见 §3

> ⚠️ **本次派验通知丢失**：TASK-010 于 `03:39:16Z` 派给我，我是被 idle hook 唤醒后重扫磁盘发现的，不是收到消息。本 sprint 第二次（test-m1c3b-b 的 TASK-006 是第一次）。

---

## 0. 漂移、越界、两条 error_handling 判据

| 检查 | 我的实测 |
|---|---|
| `verify_baseline.head` vs 我测量时 HEAD | 均为 `21815bed`，**零漂移** |
| `discovery_sha256` | `72d21a6b…`，与基线逐字相同 |
| 实际改动 vs 声明 `writes` | **仅** `internal/hestia/CONTRACTS.md`，与 `writes` 逐一对应，无越界 |
| **error_handling[0]**（AD-6 路径收窄） | `git diff --numstat 6ff4b45 HEAD -- internal/hestia cmd/atlas configs ':!internal/hestia/CONTRACTS.md'` ⇒ **0 行** ⇒ 自证数字仍有效 |
| **error_handling[1]**（生产库未被改动） | `shasum -a 256 …/runtime/atlas/data/hestia.db` = `478d40c079c8b0eab7d089bb6f1926725b361a6dc6c850f4c4a651406f3ec28c`，与期望值**逐字符相等**。⚠️ 我在**真跑之后又验了一次**，仍相等 |
| `non_functional[1]` 交付流程 | 提交 `21815bed` @ `03:35:06Z` **早于** `dev_done` @ `03:37:19Z` ✔ |

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 我的证据 | 判定 |
|---|---|---|---|
| functional[0] | 真跑并记采样锚 | §2（**我自己跑了一遍**） | **PASS** |
| functional[1] | 四道恒等式 + `merged@v1`=42 + 冲突 0 | §2、§4 | **PASS** |
| functional[2] | Step 2b 同构 + `tsf_stock`=79 | §5 | **PASS** |
| functional[3] | CONTRACTS A–E 五节写全 | §6 | **PASS** |
| functional[4] | ① `unknown_extractor:merged@v1`=0 ② PartialCoverage 非空且 33/96；17 事件逐条进 §B | §3、§4、§6 | **PASS**（判据本身有问题，见 §3） |
| functional[5] | ① PartialCoverage 只收入权威表的组 ② `Unclassified`=0 | §4 | **PASS** |
| boundary[0] | 结转项 6/7/8 + 结转项 7 实证 + 矛盾标签两栏 | §6 | **PASS** |
| boundary[1] | CONTRACTS 结构先锚 `## Sprint` 再切 | §6 | **PASS** |
| boundary[2] | 明写移交 M1c-4 的 1b 与 3 | §6 | **PASS** |
| boundary[3] | 绊线失效登记 | §6 | **PASS** |
| error_handling[0][1] | 见 §0 | §0 | **PASS** |
| non_functional[0] | gofmt/vet/`go test ./...`/两个包覆盖率分别判 | §7 | **PASS**（边界情形见 §7.1） |
| non_functional[1] | 主仓库直接干、提交后才转 `dev_done` | §0 | **PASS** |

---

## 2. 我自己跑了一遍真跑（不采信报告）

`42 / 107 / 96` 只能在真语料上核，**这是它们唯一的验收点**，所以我没有读 dev 的报告了事。

采样锚 **`21815beda194398b1ee9b0777e969f4f102b8ac3`**，跑前 `git status --porcelain -- internal/hestia cmd/atlas configs` 命中 **0**：

```
exit=0 ；报告 302 行
  语料总篇数:     218      解析成功:       161      入权威表:        75
  待解析:         199      解析失败:        38      落 pending:      21
  本迭代不解析:    19      合并后观测:      96（单篇 54 + 合并组 42）
四道恒等式: 全部成立 ✓
```

四道恒等式逐条：`218 = 199 + 19`、`199 = 161 + 38`、`96 = 54 + 42`、`96 = 75 + 21`。**与 dev 报的逐项相同**（它跑于 `6ff4b45`，我跑于 `21815bed`，两者代码路径 diff 为 0 行，故应当相同——这是一致性的预期，不是巧合）。

---

## 3. 🔴 最重要的发现：`functional[4]` 第 1 条的判据**零信息量**

DoD 把「报告里 `completeness: skipped{unknown_extractor:merged@v1}` 的条数为 0」当作 **「TASK-011 接线是否真的生效」的判据**，也是 A-1 闭合的证明。

**我在派验前就怀疑它可能恒真，实测证实了。**

### 3.1 变异实验：删掉整个 merged 接线，那个 0 不变

在隔离 worktree 里删掉 `gateCompleteness` 的三行 merged 分支：

```go
-	if in.obs.Meta.Extractor == extractorMerged {
-		req = mergedRequiredFields(in.obs.Parts)
-	}
```

重新构建、对同一语料重跑：

| | 对照组 | 变异组 |
|---|---|---|
| `unknown_extractor` 命中 | **0** | **0** |
| `skipped` 命中 | **0** | **0** |
| 报告 | — | **与对照组逐字节相同**（`cmp` 无差异，`diff` 0 行） |
| `hestia_observations` / `hestia_pending` | 75 / 21 | **75 / 21** |
| 其中 `merged@v1` | 28 / 14 | **28 / 14** |

⇒ **把 A-1 的修复整个删掉，可观测输出一个字节都不变。那个 0 不是「接线生效」的证据。**

### 3.2 成因（查清了才下结论，SURVIVED 不携带因果）

我读了代码而不是猜：

- `Validate` 确实被调用（`backfill_load.go:274`），`gateCompleteness` 确实在闸门表里（`validate.go:163`）——**分支不是没跑**。
- 但 `gateCompleteness` 的三种返回（`CheckPassed` / `CheckSkipped` / `CheckFailed`）**在本语料上都不可观测**：
  - 报告全文**根本不出现 `completeness` 这个词**（`grep -c` = 0）；
  - `pendingReason` 只渲染落 pending 那 21 条的判因，而它们全是 `deposit_sum` / `stock_continuity` 触发的（报告里 `missing` 命中 **0** ⇒ `gateCompleteness` 在本语料上从不返回 `Failed`）；
  - `CheckSkipped` 与 `CheckPassed` 对入库/落 pending 的决定**没有区别**（两组 DB 逐格相同即证）。

⇒ **判据坏在「它读的那个数在两种世界里都是 0」，不是坏在实现。**

### 3.3 那 A-1 到底闭合了没有？——我用**直接观测**回答

既然真跑输出不可分辨，我在 `gateCompleteness` 的 merged 分支里临时插了一个打点（写 stderr，不污染报告），重新构建并跑同一语料：

```
🔴 打点总条数（= merged 分支实际执行次数）= 42
🔴 parts=0 的条数 = 0 ；parts>0 的条数 = 42
🔴 req=0  的条数 = 0
   parts 长度分布： 19×parts=2 ， 23×parts=3        （合计 42）
   req   长度分布： 2×27, 15×36, 2×45, 17×52, 6×54  （合计 42）
   样例： ZZPROBE merged-branch parts=3 req=54 period=2019-12
```

这一次观测同时闭合了 `plan.md` 裁决记录第 6 条要求的三条路径：

| 路径 | 结论 | 证据 |
|---|---|---|
| **① 正向**：`Obs.Parts` 非空 | **成立** | `parts=0` **0 条**、`parts>0` **42 条** ⇒ TASK-006 确实把 `MergedObservation.Parts` 接到了 `Obs.Parts` |
| **② 反向**：闸门分支真被执行过 | **成立** | 分支内打点 **42 次**，恰等于 `merged@v1` 观测数 |
| **③ 同源校验** | **成立** | 这个 42 是从 `gateCompleteness` 的 merged 分支**内部**打出来的，**与闸门同源**；它不是 `res.MergedGroups++` 那个合并阶段的 42 |

另：`req=0` 的 **0 条** ⇒ 42 条合并观测**没有一条**落进 `CheckSkipped`，全部被真正校验（`req` 长度 27/36/45/52/54 与各自 `Parts` 长度对应）。

**⇒ A-1 闭合是真的，但证明它的不是 DoD 那个 0，是这次插桩观测。**

### 3.4 我自己欠的那个动作，补上了

我此前对 Leader 说「42 与 `unknown_extractor` 不同源是结构性的」——那是从 Leader 给的位置描述推出来的，**我没读过代码**。现在读了：

- `res.MergedGroups++` 在 `backfill_load.go:267`，位于 `for _, g := range groups` **合并循环**内，判据是 `len(g.SourceIDs) > 1`；
- `"unknown_extractor:"` 在 `validate.go:477`，位于 `gateCompleteness` 内。

**不同源确认**，现在是观察不是推断。

### 3.5 建议（不影响本次判定）

这条判据应当换成**能观测到差异**的东西。可选：让报告渲染一节 completeness 摘要（各状态条数），或在 `BackfillLoad` 里对 `CheckSkipped` 计数并纳入恒等式。**这属于 M1c-4，本任务的 DoD 已被满足**——dev 无过错，它做的正是 DoD 要求的事。

---

## 4. `functional[1]` / `[4]` / `[5]` 的其余数字（全部我自己复算）

### 4.1 dev 的核心发现独立复现：42 跨两张表

DoD 给的验收 SQL 只查 `hestia_observations`：

```
tsf-stock@v1|42   merged@v1|28   rule@v2|3   tsf-flow@v1|2      合计 75
```

⇒ **`merged@v1` = 28，不是 DoD 期望的 42。** 而 `hestia_pending` 里另有：

```
merged@v1|14   rule-monthly@v2|6   rule@v2|1                    合计 21
```

**两表合计**：`tsf-stock@v1 42 + merged@v1 **42** + rule-monthly@v2 6 + rule@v2 4 + tsf-flow@v1 2 = 96`。

⇒ **期望值 42 是对的，是 DoD 的命令射程不足。** 而 `75 = 42+28+3+2` **自洽**——不会有任何东西提示你少看了一张表。dev 发现并已写进 CONTRACTS §A-2，**判断正确**。

### 4.2 字段数分布（我用 sqlite + python 自算）

```
9→2   18→42   27→2   36→15   45→2   52→23   54→10     求和 = 96
完整（52 或 54）= 33/96 ；18 字段 = 42
```

**与 DoD 的预期表逐格一致。**

### 4.3 其余判据

| 判据 | 我的实测 |
|---|---|
| 字段冲突 | 报告第 278 行「字段冲突（预期 0，共 **0**）」 |
| `unknown_extractor` / `skipped` | **0 / 0**（但见 §3——这个 0 不构成证明） |
| `PartialCoverage` | 报告第 212 行「部分覆盖的期次（**64**）」，逐条列出缺哪一族（如 `2020-03/q1  缺: 社融存量`） |
| `PartialCoverage` 只收入权威表的组 | 抽检 3 条：`2020-01/monthly`、`2020-02/monthly`、`2020-03/monthly` **均在权威表、pending 为 0** ✔ |
| `Unclassified` | **0**（报告无该类条目），恒等式一 `218 = 199 + 19` 成立 |
| 17 个拆分事件 | **17**。第二把尺：发布事件 79 个、`period_type` 数分布 `{1: 62, 2: 17}`，`62×1 + 17×2 = 96` = 观测数 ✔ |
| 拆分的时间边界 | 最晚一个 **2025-06**；2025-09 起（`2025-09…2026-07` 共 10 个期次）**无一拆分** ⇒ dev 那条「证伪判据的后半段已经发生、不是『将来』」**独立复现** |

---

## 5. `functional[2]`：Step 2b 同构与 `tsf_stock` = 79

按 `(extractor, 字段集合)` 分组（**不是按 extractor 直接比**，否则 2-篇组会进分母造出一个假失败）：

```
merged@v1          不同字段集合数=5  大小=[27, 36, 45, 52, 54]
rule-monthly@v2    不同字段集合数=1  大小=[52]
rule@v2            不同字段集合数=1  大小=[54]

merged@v1(52) == rule-monthly@v2(52) ?  True    对称差 = []
merged@v1(54) == rule@v2(54)         ?  True
全字段集(54) − merged@v1(52) = ['fx_rate', 'fx_reserve']
```

⇒ **同构成立，且是逐字段相等而非只比基数**。缺的两个字段实测正是 `fx_rate` / `fx_reserve`。27/36/45 三种是不齐 3 篇的已知例外，未计入同构分母。

**`tsf_stock` 非空观测数**：权威表 **60** + pending **19** = **79**，与标定阶段的 79 相等 ⇒ spec §3.3「合并不影响标定」成立。

> ⚠️ 这里也踩着同一个坑：只查权威表会得 60。与 §4.1 是同一条。

---

## 6. CONTRACTS 内容与结构

**结构（`boundary[1]`）**：我先锚 `## Sprint` 再切。全局重名实测 `### A.` **4 次**、`### F.` **3 次**、`### G.` **3 次** ⇒ 全局 `grep '^### F\.'` 必然错，dev 的做法正确。M1c-3b 节在 **1690–1985**（295 行），节内小节 **A–G 七节**。

> DoD `functional[3]` 要求 A–E 五节；dev 写了 A–G，多出的 F/G 是流程/方法学登记，理由写在 `decisions` 里。**A–E 逐条写全，多写不构成偏离。**

**逐条核对 DoD 点名的内容**：

| DoD 要求 | 落点 | 我的核实 |
|---|---|---|
| `functional[4]`：需求文档 Task 3 说三篇「`period_type` 完全相同」是假的，要记下更正 | §A-1 | 在，且给出 17 事件、季末月、形态 `monthly + {q1\|h1\|q1_q3}`，并推出「合并键必须含 `period_type`」 |
| `functional[4]`：17 个发布事件**逐条**写进 §B | §B-1 | **逐条 17 行表**，我数得 17，且最后一行正是 `2025-06`——与我实测的最晚拆分**吻合** |
| `functional[4]`：人类 2026-09-01 裁决 + 证伪判据 | §B | 「人类裁决（2026-09-01）：不合并是刻意的」在；证伪判据在 |
| `functional[3]`：§C 「合并不影响标定」的证伪判据 | §C | 「若这两个数不再相等，§3.3 立刻失效，必须停下来查，不要调整期望值」 |
| `functional[3]`：§D 写明是「现在用不上」而不是「不需要」+ 重建判据 | §D | 标题即含「是『现在用不上』，不是『不需要』」；三条重建判据在 |
| `functional[3]`：§E 每条带 `done_criteria` 承接句 | §E | **6 条，逐条都有** ⚠️「必须进某个任务的 `done_criteria`」 |
| `boundary[0]`：结转项 7 的实证原样登记 | §F | `git log --grep` 的三个 dev 实证 + 「为什么没修」（commit 锚点格式是门禁强制、改它要动运行时资产） |
| `boundary[0]`：矛盾标签**两栏**登记 | §E-3 | **两栏表**（谁来修 / 何时必须修完），并写明「入库前必须清零」这条期限信息此前没有正式的家、本表就是它的家 |
| `boundary[2]`：明写移交 1b 与 3 | §E-1 / §E-2 | 在 |
| `boundary[3]`：绊线失效 | §G | 在，标题即「设计正确、前提失效、且**不会报告自己已失效**」 |
| `boundary[0]`：结转项 6 | §A-3 | **如实记「定位器已失效、未能定位故未处理」**，并给出证据（`profiles.go` 本 sprint numstat 零输出）⇒ **没有猜，这是正确处置** |
| `boundary[0]`：结转项 8 原样保留 | — | 条件是「本迭代若未动 `periodAlt` 就原样保留」，dev 实测未动；`periodAlt` 在节内命中 3 次 |
| §A-2：两张表的提醒 | §A-2 | 在，且写明「75 = 42+28+3+2 自洽、不会有任何东西提示你少看了一张表」 |

---

## 7. 门禁

| 项 | 结果 | 测量树 |
|---|---|---|
| `go test ./... -count=1` | **FAIL 包数 0** | `21815bed` |
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**零新增项**） | 同 |
| `go vet ./internal/hestia/... ./cmd/...` | **0 行输出** | 同 |

### 7.1 覆盖率：两个包 × 两把尺 × 各两次

全部测于 HEAD **`21815beda194398b1ee9b0777e969f4f102b8ac3`**：

| 包 | 尺A（`go test -cover`） | 尺B（`-coverpkg` + `go tool cover -func` total） |
|---|---|---|
| `internal/hestia` | **96.2% / 96.2%** | **96.1% / 96.1%** |
| `cmd/atlas` | **75.7% / 75.7%** | **75.9% / 75.9%** |

四个数各自两次一致 ⇒ 差值是口径差不是抖动。

🔴 **一个必须写明的边界**：DoD `non_functional[0]` 指定 `go test ./internal/hestia/... -cover` **≥ 96.2%**，实测 **96.2%**——**恰好相等，按 `≥` 满足**。但换门禁口径（尺B）是 96.1% < 96.2%，**会得出相反结论**。

**dev 主动如实记明了这一点，没有挑对自己有利的那把尺** —— 这一点我认为值得单独指出：它本可以只报 96.2% 而完全合规。

任务级 `coverage_floor: 75` 是门禁的**实际**判据（`task-completed.sh` 读它），**四个数全部 ≥ 75**。

---

## 8. INFO（不影响判定）

**INFO-1｜一处自证数字与载体不符。**
discovery 的 `files_modified` 写 `CONTRACTS.md（+291 −0）`，而 `git show --numstat 21815bed` 是 **296 0**，差 **5 行**（M1c-3b 节实际 295 行）。改动性质（纯新增、单文件、在 `writes` 内）与全部结论均不受影响，但按本 sprint「自证数字必须在最后一次改动之后统一重采」的标准，这个数看起来采于最后一次改动之前。

**INFO-2｜`functional[4]` 第 1 条判据零信息量，见 §3。**
建议在 M1c-4 换成可观测的判据（渲染 completeness 摘要，或把 `CheckSkipped` 计数纳入恒等式）。**dev 无过错**——它做的正是 DoD 要求的事，且它自己在 discovery 里写下了「四道恒等式抓不住它（全由同一批计数器派生，内部自洽 ≠ 闸门真的执行了）」，方向是对的，只是那个替代判据同样不可观测。

**INFO-3｜结转项 6 的处置是对的。**
定位器（`profiles.go:198`）已失效，dev **如实记明「未能定位故未处理」而不是猜一个**，并在 CONTRACTS 写下「下次用内容定位而不是行号」。本 sprint 行号漂过三次，这条自我记录质量高。

---

## 9. 结论

十四条 done_criteria **逐条 PASS**，无越界、无新增依赖、生产库未被触碰（sha256 逐字符相等，真跑前后各验一次）。

**关键数字全部由我独立重采**：我自己构建二进制、自己跑真语料（exit=0、302 行报告），四道恒等式、42 跨两张表、字段数分布、`tsf_stock`=79、17 个拆分事件、Step 2b 逐字段同构，全部自算而非引用。

**最重要的一处**：`plan.md` 裁决记录第 6 条要求的三条路径，我用**变异 + 插桩**给出的是**观测**而不是推理——变异证明 DoD 那个 0 恒真（删掉整个接线，报告与 DB 逐字节不变），插桩证明 merged 分支**实际执行 42 次、`Parts` 全部非空、无一落进 skipped**。**A-1 闭合成立，但成立的理由与 DoD 写的不是同一个。**

**判定：VERIFIED。**
