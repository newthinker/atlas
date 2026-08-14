
## dev-agent-56 · Sprint M1c-1 / TASK-003（2026-08-14）

### 1. 开工前工作区不干净 ⇒ 挡住**全体** dev 的 dev_done，且 dev 侧无合法出路

本轮实撞：8 个与本 sprint 无关的既有脏文件（GitNexus 重建索引再生的
`.claude/skills/gitnexus/*/SKILL.md` × 6、`AGENTS.md`、`CLAUDE.md`，session 起始快照里
就是 ` M`，`git diff --name-only | grep -c hestia` = 0）让我的 `transition dev_done`
被 scope 漂移检查 BLOCKED。

**为什么 dev 自己解不了**（读 `task-completed.sh:246-286` 逐行确认，不是推测）：
漂移集 = 未暂存 + 已暂存 + untracked + 本任务自己的提交 − 他人在途 scope − 本任务 `writes`，
唯一豁免是 `.arcforge/` 整棵树。这类文件既未提交、又不在任何任务 `writes` 下 ⇒
门禁给的两条出路对 dev 都不成立：补进 `writes` 是**假声明**（`.claude/` 还对全体 agent 只读），
`git checkout --` 撤回**别人的未提交改动**不可逆且越出任务范围。

**处方（给 Leader，零成本且必须前置）**：**spawn dev team 之前**把工作区清干净——
把无关的既有改动提交或撤回。这不是洁癖：它是**全 sprint 级**的阻塞，且发现得越晚，
撞上它的 dev 越多（每个 dev 都要在自己交付的最后一刻才撞到，然后各花一轮去查清它不是自己的）。

**判据形状值得记住**：门禁的阻断文案默认「越界 = 你干的」，而它其实无法区分
「你改的」与「你开工前就脏的」。**看到越界路径的第一步是查它是否早于自己的认领时刻**
（session 起始 git status 快照 / `git diff` 内容是否触及本任务的包），别条件反射去补声明。

### 2. 同包并行时，「导出面白名单」守卫会让所有人撞同一行

`internal/hestia/store_test.go:399` 的 `TestPackageExposesNoWriteFunctions` 用 AST 扫全部
非测试 `.go` 文件，断言导出函数/方法集合**恰好等于**一份写死的 18 项名单。本 sprint 有
6 个任务往同一个包加代码，任何一个新增导出函数都会让它红——而 `store_test.go`
**不在任何任务的 `writes` 里**，且所有人要改的是**同一行** want 列表（并行 ⇒ 逐个 merge 冲突）。

处置：**优先用非导出名**。判据很简单——**调用方在不在包内**。本任务的下游
（TASK-006 fetch / TASK-008 reconcile）都在 `package hestia` 内，本就不需要导出；
该守卫只看 `FuncDecl`，所以导出**类型**不受影响（DoD 要求的 `Manifest`/`Article`/`Failed`
照常导出，JSON 契约不变）。只有确实要跨包用时才值得动那份名单，且必须由**唯一一个**
任务持有 `store_test.go` 的 writes。

### 3. 变异 harness 打出了一行假的 `SURVIVED`

M7（把原子写改成 `os.WriteFile` 直写）的 python 变异脚本因引号转义报 SyntaxError，
**变异根本没落盘**，而 harness 照样往下跑测试、打印「(无 FAIL —— 该变异存活 SURVIVED)」。
若不是我顺手看了那条空 diff，这个结论会以「该断言守不住」的形式进交付报告——**方向恰好
与真相相反**（重跑后 M7 是被预期的那条测试干净杀死的）。

⇒ 与 CLAUDE.md「外溢闸命中即早退」同族，但成因不同：那条防的是**闸命中后不早退**，
这条防的是**变异未生效却继续判定**。可照抄的两道前置闸：
`diff 为空 ⇒ 立即 return`（变异未落盘）、`go vet / bash -n 不过 ⇒ 立即 return`（变异非法）。
两道都要在**打印任何 KILLED/SURVIVED 之前**。

## TASK-006 的一条 DoD 修订：窗口在消息到达前就关了（2026-08-14）

**这条记在这里，是因为它没有别的持久载体了。** Leader 请我把修订写进 discovery，但消息到达时
任务已是 `verifying`，我对任务文件与 discovery 都没有写权（实测 DENY 见下）。wisdom 是我仍
拥有、且**不是判定对象**的唯一载体。

### 修订内容（权威等级同 DoD，来源：leader inbox 消息）
原 DoD 那条「补 URL/Published 可观测性」基于一个被证伪的论证（我原以为该缺口在 TASK-005
「不可达」，test-agent-28 用实验证否：`crossCheckBackfill` 是纯函数，两侧输入完全由测试构造）。
⇒ 拆成**两条性质不同、缺一不可**的测试：

| 要钉的性质 | 放哪 | 为什么只能放那 |
|---|---|---|
| 两侧**分叉时**取 index 侧 | `backfill_crosscheck_test.go` | 这个决定只发生在 TASK-005，纯函数 ⇒ 测试上一直可达 |
| 本层的 `base` 使两侧**根本不分叉** | `backfill_fetch_test.go` | 只有抓取层提供 `base` |

**只做第二条的后果**：它成立时「取哪一侧」在生产上不产生差别 ⇒ 谁把 crosscheck 改成取搜索侧
都不会有东西变红。**只做第一条的后果**：`base` 哪天变了（`http` vs `https`、末尾斜杠）没有
任何东西出声。⇒ 两条各自都不充分。

### 我实际交付的是哪一条（说清楚，免得后人以为两条都在）
我交付的 `TestCrossCheckBackfillIntersectionTakesIndexSideFields` 位于
**`backfill_fetch_test.go`**，钉的是**第一条**（分叉时取 index 侧）。
⇒ **第二条（base 不分叉）尚未落地**，且第一条按 leader 的裁决应挪到 `backfill_crosscheck_test.go`。

### 成品代码（谁拿到写权谁照抄，两条都要）

`backfill_crosscheck_test.go` 新增（自足，不动 TASK-005 任何既有用例）：

    func TestCrossCheckBackfillPrefersIndexSideURLAndPublished(t *testing.T) {
        index := []backfillItem{xcIndexItem(xcIDBoth1, "index 侧的标题", "2025-09-12")}
        search := []backfillSearchHit{xcSearchHit(xcIDBoth1, "搜索侧的标题", "2025-09-13")}
        search[0].URL = "http://www.pbc.gov.cn/goutongjiaoliu/113456/113469/" + xcIDBoth1 + "/index.html/"
        got := crossCheckBackfill(index, search, nil)
        require.Len(t, got.Fetch, 1)
        assert.Equal(t, index[0].URL, got.Fetch[0].URL)       // https、无末尾斜杠
        assert.Equal(t, "2025-09-12", got.Fetch[0].Published) // index 侧日期
    }

`backfill_fetch_test.go` 新增：

    func TestBackfillFetchBaseKeepsBothSidesURLsIdentical(t *testing.T) {
        cfg := bfConfig(t, t.TempDir())
        const id = "9001"
        got, err := resolveURL(cfg.IndexURL, "/goutongjiaoliu/113456/113469/"+id+"/index.html")
        if err != nil { t.Fatalf("resolveURL: %v", err) }
        if got != bfArticleURL(id) {
            t.Errorf("index 侧补全后与搜索侧绝对 URL 分叉\n index: %q\nsearch: %q", got, bfArticleURL(id))
        }
    }

配套还需把 `./internal/hestia/backfill_crosscheck_test.go` 加进 TASK-006 的 `writes`
（leader 已显式取舍：第 6 个文件，超 Realistic Scope 的 ≤5 一个）。

### 🔴 机制教训：这是「交出写权」陷阱的**第五个实例**，而它咬的正是关于该陷阱的修订
- 我 `11:24:04Z` 认领 ⇒ leader 从那一刻起写不了该任务字段（他的第四次实撞）；
- 我交付后转 `dev_done`、报告 leader 可派验 ⇒ leader 派验转 `verifying` ⇒ **我也失去写权**；
- 修订消息到达时，**没有任何角色能落盘它**（实测：
  `DENY: 「dev-agent-56」无权执行 verifying 写入(合法写者: ["test-*"])`，`writes` 仍 5 项）。
- ⇒ 判别式要再扩一层：不只问「这一步之后我还写得了什么」，还要问
  **「我催对方做的那个动作，会不会顺手关掉我自己的窗口？」** —— 我报「可以派验了」就是自己
  关上了自己的窗口，而当时我并不知道有一条修订在路上。
- ⇒ 实践含义：**dev 报「可派验」之前，先问一句「还有没有在途的、给我的未决输入？」**
  这一条比事后补救便宜得多——事后没有补救路径。
