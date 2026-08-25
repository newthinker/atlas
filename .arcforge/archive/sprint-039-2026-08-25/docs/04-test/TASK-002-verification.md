# TASK-002 验证报告 — 批量解析快照与失败清单（collectSamples）

- **验证者**：test-m1c2-v2　**判定：VERIFIED（8/8 done_criteria 通过）**
- **验证对象**：`master @ 6bb60b4689eaecc8cd6979d17cd3c2b4f2164ae7`（分支提交 `380b9f46a4dd95e2240745b3c7616dcec17ab9d0`）
- **base 对照**：`fba0feb1e5ca8ae65277b9957e76b8acc1d7f1bf`
- **verify_baseline 核对**：记录 head 与 discovery_sha256 与当前值**逐字相同**，声明范围内两文件工作树无改动 ⇒ **无漂移，未使用 `--ack-drift`**
- **验证环境**：独立 worktree `../wt-verify-TASK-002 @ 6bb60b4689…`（对照 `../wt-verify-T002-base @ fba0feb1e5…`），
  主工作区两文件 sha256 在每个变异窗口内与收尾均核对未变（`32ceb8bc…` / `9df09f19…`）

---

## 1. Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 我做的独立证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 签名/类型、按期次归组、只解析《金融统计数据报告》、汇总 Samples | `TestCollectSamplesCountsDifferPerField` | 真跑：`Fields=54`，非社融字段 n=22 / 社融字段 n=4，两档差异如 DoD 所述 | **PASS** |
| functional[1] | 社融存量/增量两篇不计入失败表 | `TestCollectSamplesIgnoresNonFinanceKinds` | 真跑：`Unsupported` 中 69+69 条社融，`Failures` 中 0 条社融 | **PASS** |
| functional[2] | 三类各有一格、计数分别可见、不得 continue 丢弃 | `TestCollectSamplesSeparatesThreeCategories`＋`…RendersEveryCategoryToOut` | 复现 M2（红 2 条，L188/L224）＋自设 **Mv2-B**（红 L196/L224） | **PASS** |
| boundary[0] | CompletedAt 成对；三条错误路径 want 互不相同；「空」判据为可用样本数 | `RejectsIncompleteManifest` / `AllowsIncompleteWhenAsked` / `ErrorPathsAreDistinguishable` / `RejectsZeroUsableSamples` | 自设 **Mv2-E** 人为让两条错误串相撞 ⇒ 红 L317（DoD 的字面验收步骤已机制化） | **PASS** |
| boundary[1] | Dir 不存在 / manifest 不可读 ⇒ 报错 | `ErrorPathsAreDistinguishable/目录或_manifest_不存在`、`RejectsUnreadableManifest`、`RejectsUnstatableDir` | ENOTDIR 与 NotExist 分列，负向断言 `NotContains(err,"不存在")` 有实义 | **PASS** |
| error_handling[0] | 记 Failures 并继续；四字段都填；断言 Err **内容** | `RecordsMissingFileAndContinues` / `RecordsParseFailure` / `FailuresCarryDistinctReasons` | 自设 **Mv2-A**（保留交叉校验、只把理由换成通用话）⇒ 红 L607/L608 | **PASS** |
| error_handling[1] | manifest 四个可信度字段被消费 + 期次交叉校验 | `SurfacesSearchSkippedReason` / `SurfacesFetchFailed` / `SurfacesMissingPeriods` / `VerifiesArticleSHA256` / `CrossChecksPeriodAgainstTitle` | 自设 **Mv2-D**（去掉「无 reconcile 摘要」warning）⇒ 只红 `…/没有对账摘要也要出声`（L530） | **PASS** |
| non_functional | gofmt / vet / 全绿 / 覆盖率 ≥94.810% / G2 无新增依赖 | — | 见 §3，全部由我在 `6bb60b4` 上重采 | **PASS** |

---

## 2. Leader 点名的三件事

### 2.1 端到端真跑对账（dev 自陈「我没验」的第一条）—— **已做，四格全部对上**

`collectSamples` 对 `data/hestia-backfill-2026-08-14/`（`AllowIncomplete=true`，该 manifest 无 `completed_at`）实跑：

| 格 | 实现跑出来的 | dev 的 jq/awk | 我的独立分类器 | 一致？ |
|---|---|---|---|---|
| 社融存量 | 69 | 69 | 69 | ✓ |
| 社融增量 | 69 | 69 | 69 | ✓ |
| 金融统计 monthly（不解析） | 55 | 55 | 55 | ✓ |
| 金融统计 受支持（`Periods`） | **25** | 25 | 25 | ✓ |
| `Unclassified` | 0 | 0 | 0 | ✓ |

小计 `69+69+55+25+0 = 218 = len(articles)`，无一篇遗失。
**我的第三口径不是 dev 的 awk**：我照 `backfill_scan.go:51`（`backfillTitleRE`，不锚定起点）、
`backfill_reconcile.go:154`（`backfillPeriodOf` 的期次段坍缩）、`parse.go:41/225`（`titleRE` 锚定 + `checkPeriodTypeSupported` 只拒 monthly）
在 Python 里重写了一份分类器（`scratchpad/v2-independent-classify.py`），与实现输出、与 dev 的 awk 三方对上。

**⚠ 但第五格对不上 discovery 给下游的预测**（见 §4 F1）：真跑 `Failures=3`，而 discovery 的
`notes_for_downstream` 写「TASK-005 …Failures 目标为 0」。

其余真跑事实：`FetchFailed=0`、`Warnings=1`（只有「无 reconcile 对账摘要」那条，与预测一致）、
无 sha256 缺失告警（218 篇全有 sha256）。序列确有洞：受支持期次里 **没有 2024-03**。

### 2.2 复现 dev 的 M2 —— **确认夹具已改名，M2 确实红 2 条，并做了反事实对照**

- 夹具 `calibrate_test.go:165` 现为 `File: "articles/nope-m.html"`（已改名）。
- **M2（去掉 monthly 预判）**：红 2 条 —— `SeparatesThreeCategories`（断言行 **L188**，`require.Len(Unsupported,3)`）
  与 `RendersEveryCategoryToOut`（断言行 **L224**，`Contains(out,"monthly")`）。
- **反事实对照（我加的）**：把那一行文件名改回 `articles/nope-monthly.html` 再施 M2 ⇒ **只红 1 条**（仅 L188），
  L224 平凡为真。**dev 的因果说法由实验证实，不是采信。**
- ⚠ 附带发现一处 `require` 遮挡：M2 在 L188 的 `require.Len` 处 `FailNow`，**L196「monthly 计数」那条断言在 M2 下根本没执行**。
  故我另设 **Mv2-B**（分类正确、只把理由文案里的 `period_type=monthly` 拿掉）⇒ 红 **L196 + L224**。两条内容断言这才真被证。

### 2.3 同族自查：还有没有别的断言被夹具字面量冒名满足 —— **未发现第二例**

两条互补手法：

1. **渲染归属探针**：把 `threeCategoryFixture` 的 `Out` 逐行打出，对 `RendersEveryCategoryToOut` 的 4 条期望串
   统计命中行 —— 4 条**各命中且仅命中 1 行**，且都落在它本该守的那一段：
   `2024年三季度…`→L09（标题解析不出期次段）、`articles/bad.html`→L07（解析失败段）、
   `社会融资规模存量统计数据报告`→L03（本迭代不解析段）、`monthly`→L05（同段的 monthly 那行）。
   四段文案的共有词只有「篇/条」和期次串，断言用的都是各段独有的部分。
2. **静态扫描**（脚本遍历 654 行里全部 `Contains`/`NotContains`）：仅标出 1 条
   —— `CrossChecksPeriodAgainstTitle` 的 want `"2025-12"` 也出现在字面量 `pboc-2025-12-annual.html` 里。
   **经查是假阳**：那是 `writeCalibrateFixture` 的 **testdata 源文件名**，落盘后 `Article.File` 是 `articles/m.html`，
   源名进不了 `Err`；且 Mv2-A 证明 L607/L608 确实由错配两侧的期次串守着。

---

## 3. 自证数字（**全部由我在 `6bb60b4689eaecc8cd6979d17cd3c2b4f2164ae7` 这棵树上重采**）

| 项 | 值 | 口径 |
|---|---|---|
| 定向套件 | **27 PASS / 0 FAIL**（19 顶层 + 8 子测试） | `go test ./internal/hestia/ -run '^TestCollectSamples' -count=1 -v` |
| 整包 | 绿 | `go test ./internal/hestia/ -count=1` |
| 全仓回归 | 无非 `ok`/`?` 的包 | `go test ./...` |
| 覆盖率 base | **1370/1445 = 94.810%** @ `fba0feb1e5…` | NumStmt 加权，直接从 coverprofile 求和 |
| 覆盖率 交付 | **1501/1576 = 95.241%** @ `6bb60b4689…` | 同上；**背对背同轮交替跑 2 轮，两轮 profile 逐字节一致** |
| `calibrate.go` | 六函数 **全 100.0%**，profile 内无 `count==0` 块（74 个块） | profile 在**各自源码树内**渲染 |
| gofmt (a) | 两文件输出为空 | `gofmt -l internal/hestia/calibrate.go internal/hestia/calibrate_test.go` |
| gofmt (b) | base 28 个 vs 交付 28 个，`diff` **逐字节一致** | `git ls-files '*.go' \| xargs gofmt -l \| sort` |
| `go vet` | 退出码 0，无输出 | `go vet ./internal/hestia/` |
| G2 无新增依赖 | `fba0feb..380b9f4` 改动文件仅 `calibrate.go` + `calibrate_test.go`，**无 go.mod/go.sum** | `git diff --name-only` |
| 声明范围 | 实际改动 = `writes` 声明，**无越界** | 同上 |

**我的自证数字有没有「不由我产生」的锚？**

- **有**：① base 覆盖率我独立测出 **1370/1445=94.810%**，与 DoD 写死的锚点逐位相同 —— 该锚点不是我算的；
  ② 真跑四格与 **dev 的 jq/awk** 和 **我另写的分类器**三方对上；
  ③ 那 3 条解析失败我**绕开 calibrate** 直接对文件调 `Parse` 复现，错误串逐字相同，且 manifest 记的 sha256 与磁盘实测前 12 位相同、标题与期次自洽 ⇒ 排除「calibrate 读错了文件」。
- **没有的地方**：`27 PASS`、`95.241%`、消融的红/绿只有「我跑过」这一个来源（dev 也跑过同样的数并且对上了，但那不是独立口径 —— 同一套件同一棵树）。

**validator**：退出码 0；2 条 `scope-writes-outside-packages` 告警。
按判别式**重新求值**（不照抄结论）：`packages=["./internal/hestia"]`，`writes` 两条 `.go` 均在该目录下 ⇒ **假阳**。
（并记：该规则条数恒等于 `writes` 长度，**「0 告警」不能反推没问题**。）

---

## 4. 发现（均**不阻断**本任务验收，但下游要用）

### F1（中）真跑 `Failures=3`，而 discovery 给 TASK-005 的预期写的是 0

三条都是**真实的上游 Parse 失败**，不是 calibrate 的缺陷（已绕开 calibrate 直调 `Parse` 复现）：

| 期次 | 文件 | 标题 | Parse 错误 |
|---|---|---|---|
| 2019-12 | `articles/2025092212551494470.html` | 2019年金融统计数据报告 | `loan scope anchor 企（事）业单位贷款 not found` |
| 2020-09 | `articles/2025092212551025045.html` | 2020年前三季度金融统计数据报告 | `unrecognized layout: 5 sections, tsf_section=false`（已知 6/8） |
| 2022-09 | `articles/2025092212552829983.html` | 2022年前三季度金融统计数据报告 | 同上 |

后两条都是 **`前三季度`（q1_q3）** 且是「5 板块」这个既有 extractor 认不出的形态 —— q1_q3 的抽取支持是
M1b-4b 的 TASK-004 接上的，那次用的两份快照显然不含这个形态。**这正是 `Failures` 这一格存在的意义**：
它就是 M1c-4 的兜底工作量（3 篇）。
⇒ **请 Leader 更正 discovery/计划里给 TASK-005 的预期**：`Failures` 的实测值是 **3**，不是 0；
对不上才该查的是「≠3」而不是「≠0」。

### F2（中）discovery 的 `note_for_T3` 写「非社融字段 n=25」，实测 **n=22**

`25` 是**尝试解析的期数**（`Periods`），不是**产出样本的期数**；25 − 3（F1 的失败）= 22。
社融字段 n=4 是对的（cutover 后 4 期全部解析成功）。
这是「数字为真、但被搬到了它测不出的那个位置」——T3 的 `n < 3 不给建议` 两档都仍 ≥3，行为不变，
但 `n` 列的表头数字若照抄 25 就会与报告实际打印的 22 对不上。**建议把该 note 改成 `4 vs 22（实测）`。**

### F3（低）`only_in_index` / `only_in_search` 既未消费也未注释

DoD 的表列了 4 个字段，这两个不在表内 ⇒ **不构成 DoD 违反**。且实际影响很小：
`crossCheckBackfill` 的 `Fetch` 取的是**并集**（`backfill_crosscheck.go:112`），两侧独有的篇目都会被抓，
所以它们是「差异记录」而非「缺失清单」；真语料里两者**均为 0**。
建议 T3/T4 顺手补一行注释说明本迭代为何忽略（DoD 那句「『没想到』和『想过并决定忽略』在代码里长得一样」同样适用）。

### F4（低-记录）sha256 守卫在结构上够不着「抓取当时就截断」

`backfill_fetch.go:210` 是 `SHA256: articleSHA256(body)` —— sha 算在**落盘的同一份 body** 上。
所以若截断发生在抓取当时，manifest 记的就是截断内容的 sha，calibrate 比对**必然通过**。
`Article.SHA256` 覆盖的是「抓取之后」的改动（磁盘损坏、拷贝不全、被人改过），**不是**「抓取本身不完整」。
dev 自陈的第二条盲区（「没构造真被截断但仍 Parse 成功的样本」）比它自己说的更重一点：
那种样本**不只是没测，而是这道守卫在结构上覆盖不到**。
真正兜住它的是 `Parse` 的板块数检查（F1 里那两条 `5 sections` 的错误文案自己就写着「or a truncated fetch」）。
⇒ 记为 TASK-005 的已知盲区；**本任务 DoD 只要求「被消费或注释声明忽略」，已消费且有两格测试，PASS。**

### F5（低-建议）monthly 委托 `checkPeriodTypeSupported` 的注释理由，今天**不可证伪**

注释写「自己维护第二份名单，漏同步的表现是静默少解析一批期次」。我把委托换成硬编码
`if periodType == "monthly"`（**Mv2-C**）跑全套 ⇒ **0 红**。
这是一次**等价变异**：两种实现在当前全部输入上逐点同值，差别只在「有人删掉 `parse.go` 的 monthly 分支」那一刻显形。
所以这个 SURVIVED **不说明断言弱**，说明**这条理由今天没有任何测试守着**。
**决定仍然是对的**（单一真相源），且可以今天就闭合：加一条耦合测试
—— 对 `validPeriodTypes` 里每个 `pt`，断言 `unsupportedPeriodType(某个该 pt 的标题) != ""` ⟺ `checkPeriodTypeSupported(pt, 标题) != nil`。
它在委托实现下是恒真的（今天全绿），而一旦有人既删了 `parse.go` 的分支又留着 calibrate 的名单，它会**自动变红**。
这就是「把未来才显形的差异写成今天的 fixture」。**DoD 未要求，故不阻断；建议由 T3 或后续任务顺手补。**

---

## 5. 消融清单（我自己跑的 6 例，全部在独立 worktree 内）

| ID | 变异 | 红的**具体断言行** | 说明 |
|---|---|---|---|
| 基线 | 无 | — | 19 顶层 + 8 子测试全 PASS |
| M2（复现 dev） | 去掉 monthly 预判 | L188、L224 | 红 2 条，与 dev 所报一致 |
| M2-反事实 | M2 + 夹具改回 `nope-monthly.html` | 仅 L188 | **只红 1 条**，L224 平凡为真 ⇒ dev 的因果说法成立 |
| Mv2-A | 保留交叉校验，理由换成通用话 | L607、L608 | 证明 `error_handling[0]` 的内容断言（M5 会被 L606 的 `require` 遮住） |
| Mv2-B | 分类正确，理由里不写 `period_type` | L196、L224 | 证明 M2 遮住的那条 monthly 计数断言 |
| Mv2-D | 去掉「无 reconcile 摘要」warning | L530（且只红那一个子测试） | 证明该 warning 不是空声明 |
| Mv2-E | 人为让两条错误串相撞 | L317 | DoD `boundary[0]` 的字面验收步骤已被机制化 |
| Mv2-C | 委托改硬编码名单 | **0 红（SURVIVED）** | **等价变异**，见 F5；不计入 KILLED |

卫生：每例落盘前打印 diff 逐字核对（语义闸）＋ `go vet` 有效性闸；每例复原后与收尾各校验一次
主工作区两文件 sha256 与 `git status`，全程未变（`32ceb8bc…` / `9df09f19…`）。

## 6. 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-002 6bb60b4689eaecc8cd6979d17cd3c2b4f2164ae7
git worktree add --detach ../wt-verify-T002-base fba0feb1e5ca8ae65277b9957e76b8acc1d7f1bf
cd ../wt-verify-TASK-002
GOTOOLCHAIN=local go test ./internal/hestia/ -run '^TestCollectSamples' -count=1 -v
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -coverprofile=cov.out   # NumStmt 加权自行求和
GOTOOLCHAIN=local gofmt -l internal/hestia/calibrate.go internal/hestia/calibrate_test.go
git diff --name-only fba0feb1e5ca8ae65277b9957e76b8acc1d7f1bf..380b9f46a4dd95e2240745b3c7616dcec17ab9d0
```
真跑对账与消融的脚本见 `scratchpad/v2-independent-classify.py`、`scratchpad/v2-ablate.sh`。

---

## 结论：**VERIFIED**

8 条 done_criteria 逐条有对应测试且断言有实义；四格真跑与两个独立口径对账一致；
覆盖率、gofmt、vet、G2、声明范围全部满足。F1–F5 为下游输入与改进建议，不构成退回理由。
