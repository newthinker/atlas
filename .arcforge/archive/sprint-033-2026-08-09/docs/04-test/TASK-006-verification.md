# TASK-006 验证报告 —— `Duplicate` UPDATE 与 pending 分流

> **本文件覆盖两轮验证，第 2 轮（C3/C4 返工轮）为当前有效判定。** 覆盖同名文件的理由同 T4 报告。
> 第 1 轮结论完整保留在 §R1。

---

# 第 2 轮（C3/C4 返工轮）—— 当前有效判定

- **验证者**：test-agent-20 ｜ **判定：VERIFIED（通过）**
- **verify_baseline**：`head` = `823ca15fbea53c626e986419d8f91ad200f03859`、
  `discovery_sha256` = `94fc146c…8789`，出裁决前实算**均一致，无漂移**
- **epoch**：1 ｜ **rework_count**：1 ｜ **reason_class**：`task_defect`
- **验证环境**：隔离 worktree `.worktrees/wt-rw-46`（`--detach 823ca15…`），验毕已 remove
- **产出所在 commit**：`823ca15`（**纯测试 109 行，只改 `store_test.go`**）——
  我核了 `git show --stat`，确认**无生产代码混入**，与 T4 的 C1 归属混叠情况不同

## 1. 这两条的性质：零行生产代码改动

`savePending` 的八列顺序本来就对、`refreshArticleID` 的 WHERE 本来就带 `published_at`。
**缺的是判据，不是实现。**

⇒ 「RED」的形态不是「测试先红后绿」，而是「**变异在补判据之前存活、之后被杀**」。
按常规去找「先红」的痕迹会找不到——**那是任务性质决定的，不是它跳过了 RED**。
我按后一种形态验证，下面每条都给出「红的是哪条断言」。

## 2. 基线（@ `823ca15`，隔离检出）

**86 含子测试 / 66 顶层 / 0 FAIL / 0 SKIP**，覆盖率 89.3%（`-cover`）／89.0%（`-func`），
`go vet` exit 0、`gofmt -l` 空。

## 3. C3：pending 八列位置参数

四条自证：diff 非空 + **两文件首行完整** + `go vet` exit 0 + 计数对基线 86。

| 变异 | 内容 | 结果 | 红的测试 |
|---|---|---|---|
| C3 | **双交换**：`Period↔PeriodType` 且 `ArticleID↔Extractor` | KILLED 85/86 | **仅** `TestSavePendingColumnsMatchTheirValues` |
| C3a | **单侧**：只换 `ArticleID↔Extractor` | KILLED 85/86 | **仅** 同上 |
| **C3b（我加的）** | **另一侧**：只换 `Period↔PeriodType` | **KILLED 85/86** | **仅** 同上 |

**C3a 是 dev 自己加的，用来排除「靠双错互相抵消才红」**——这个顾虑是对的，且
**我补了它没做的另一侧 C3b**：两个单侧交换**各自独立被杀**，说明判据对每一对
位置参数都成立，不依赖两处错误的组合。

新测试自身也带 fixture 强度自证：断言六个取值**互不相同**
（`require.Falsef(seen[w.val], "fixture 太弱：值 %q 重复出现，列互换将无法被发现")`）——
没有这一步，「互换必红」不成立。这一点做得对。

## 4. C4：`refreshArticleID` 的 `published_at` 约束

| 变异 | 内容 | 结果 | 红的测试 |
|---|---|---|---|
| C4 | 删掉 `AND published_at = ?`（及对应参数） | KILLED 85/86 | **仅** `TestSaveDuplicateOnlyTouchesTheMatchingRevision` |

QA 的判词「现有两条 Duplicate 测试都只有一行库，结构上打不中这个缺陷」**成立**：
两行库是必要条件——单行库下删掉 `published_at` 约束行为完全不变。
新测试预置 `2026-07-15/art-v1` 与 `2026-08-20/art-v2` 两行修订，做 Duplicate 后
**分别断言目标行已刷新为 `art-v2-NEWURL`、历史行仍为 `art-v1`**，并带前提自证
`require.Equal(2, countRows(...), "前提自证：库里必须有两行修订")`。

危害的表述也核实无误：少了该约束，一次站点迁移的新 `article_id` 会被**盖到全部历史行**，
静默毁掉逐行溯源，并让 M1b-4 的 `article_id` 幂等检查命中错误的修订行。

## 5. done_criteria 与 fix_items 覆盖

第 1 轮已验的 10 条 done_criteria 在本轮基线下**仍全绿**（86/0/0，无回归）。本轮两条 fix_items：

| fix_item | 要求 | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| **C3**（WARNING） | 七个元数据字段取可辨识值后逐列读回断言 | `TestSavePendingColumnsMatchTheirValues` | C3 / C3a / **C3b** 各只红这一条 | **PASS** |
| **C4**（WARNING） | 补一条两行库的 Duplicate 测试 | `TestSaveDuplicateOnlyTouchesTheMatchingRevision` | C4 只红这一条 | **PASS** |

两条 fix_item 的 `verify` 字段要求「重跑该变异必须变红且红的是新加的断言」——
**均满足，且红的确实只有新增的那一条**。

## 6. 结论

**VERIFIED。** C3/C4 均已闭合，四个变异（含我补的 C3b）各自**只红对应的新测试**。
「零行生产代码改动」的性质已核实属实，不构成跳过 RED。第 1 轮的 10 条 done_criteria 无回归。

**本轮无新增问题。**

## 7. 仍然悬而未决的一件（承第 1 轮，非本轮引入）

**G3 的数据静默丢失面**：上线 `rule@v2` 回填重跑历史时，每期 `published_at` 未变 ⇒
全判 Duplicate ⇒ v2 新抽的字段一个都不写入、`extractor` 列仍写旧值，而 `Save` 返回 nil。
DoD 允许该支且后果已被 `TestSaveDuplicateDiscardsRicherValues` 逐条钉死
（**从意外变成了看得见的决定**），故两轮均不判 REJECT。

**但它至今没有被解决，只是被登记。** 我在第 1 轮已附议 dev 的建议：单列一个任务处理
「重跑抽取如何入库」（方向：业务键加 extractor 维度，或引入第二根修订轴）。
字段刚从 ~20 扩到 54，回填重跑不是假设而是计划内动作。**本轮再次提请。**

---

# §R1 第 1 轮（历史记录，判定已被第 2 轮取代）

- 被验 commit `c1050d76e46c8e4075c42f33ecfece4d043a56b8`，判定 **VERIFIED**
- 计数：75 含子测试 / 61 顶层 / 0 FAIL / 0 SKIP，覆盖率 88.7%（`-cover`）／88.4%（`-func`）
- 10 条 done_criteria 全 PASS；12 个变异窗口
- **消融实验**（因果唯一性）：ABL0 控制组（只停用 `TestSavePendingVerdictSurvivesClassify`、
  实现不变）→ 72/72 确立正确基线；M7′ 单独 → KILLED 72/75，3 条红全属该测试；
  ABL1（M7′ + 停用）→ **72/72 完全存活** ⇒ 因果唯一。**没有 ABL0 就无法把 ABL1 的 72
  与「全绿」区分开**，控制组是消融实验的必需件。
- **删钉子等价性经实测**：我加 M2b（Duplicate 改走 `insert`，造多一行）→ KILLED，
  且 `TestSaveDuplicateIsDefinedNotSilent` 正是靠 `countRows == 1` 抓住它 ⇒ 旧钉子
  `TestSaveDuplicateIsLoudNotSilent` 的核心职责已完整迁移
- **pending 论证支点经探针实测**：pending 漂移下三种 `Values` 形态（空／单字段／全 54）
  报**同一个错**（数据无关）；观测表缺列时不含该字段**静默成功**、含才炸（数据相关）。
  选②所依赖的不对称是真的
- TASK-004 遗留的 P1（包级导出写口无守卫）已在 T5（`39aa8af`）闭环并经复验
