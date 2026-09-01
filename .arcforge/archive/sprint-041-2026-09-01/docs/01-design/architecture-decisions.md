# M1c-3b 架构与流程决策（AD）

本文件只记**需求文档没有决定、而 Arcforge 流程必须决定**的事。
代码层的设计决策全部在需求文档与 spec 里，不在此复述。

---

## AD-1 任务编号与需求文档保持一致，只拆 Task 8

**决定**：TASK-001…007 ≡ 需求文档 Task 1…7；Task 8 拆为 TASK-008 / 009 / 010。

**理由**：需求文档每个 Task 的 Step 里都写死了 commit 命令（`refactor(TASK-001): …`）。
`dev_done` 门禁按 `<type>(TASK-ID):` 匹配提交。重编号后 dev 照抄文档 ⇒ 提交锚定错编号 ⇒
门禁匹配不到 ⇒ **两个集合双双为空 ⇒ 报绿**。这是静默失效，不是响的。

**代价**：TASK-008/009/010 三个任务的 commit message 需要 dev 自己改编号，
不能照抄文档 Task 8 的那一条。**已写进这三个任务的 DoD**。

---

## AD-2 `writes` 逐文件声明，`packages` 保持包级

**决定**：见 `design-spec.md` §2.1。

**理由**：十个任务同在 `internal/hestia` 一个包。包级 `writes` ⇒ 全互斥 ⇒ 完全串行。

**已知代价**：每个任务命中告警级 `scope-writes-outside-packages`。
该规则经两次受控实验判定**零信息量**（命中条数恒等于 `writes` 长度）。
⚠️ **不能用「0 条」反推没问题**，也不要为消它改 `packages`。

---

## AD-3 🔴 语料一律用主仓库绝对路径

**决定**：所有需要 `data/hestia-backfill-2026-08-14` 的命令，`--dir` 一律写
`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/hestia-backfill-2026-08-14`，
**不用 `$PWD/data/...`，不建软链**。

**理由**：`.gitignore:64` 是 `data/`，`git ls-files data/` = **0 个文件**
⇒ linked worktree 里不存在 `data/` 目录。需求文档全篇写的 `$PWD/data/...`
在 worktree 里必然落空。

**为什么不建软链**：42 / 107 / 96 这三个数是靠语料自证的，软链多一层间接会让
「我测的是哪份语料」变得需要推理。绝对路径是观察，软链是推理。

**受影响任务**：001（采基线 / 背对背比对）、005（产建议区间）、010（真跑验收）。

---

## AD-4 🔴 merge 必须在 `dev_done` **之前**，Leader 侧加周期扫描

**机制根据**（读代码查实，非流程偏好）：`task-completed.sh` 的 `git log --grep` 全文件
**不带 `--all`** ⇒ 只走 HEAD 祖先链 ⇒ 未合并分支上的 commit 对门禁**结构性不可见**；
「本任务已提交」与「未提交改动」两个集合双双为空，门禁于是报绿。
本仓库根有 `go.mod` ⇒ 门禁是**生产级触发**，不走 skip 分支。

**固定流程**：worktree 提交 → 通知 Leader → **Leader merge 进 master** →
**dev 回主仓库** → `transition dev_done`。

**Leader 的补偿扫描**（这一环没有任何机制活性保障——它在文件层与「还在写代码」完全同形，
`stale-dispatch` 刻意不含 `in_progress`，而 idle hook 在此状态的解锁文案恒为
「推进 dev_done」，**方向恰好相反**）：

```bash
for b in $(git branch --list 'task/TASK-*-m1c3b' --format='%(refname:short)'); do
  git merge-base --is-ancestor "$b" master || echo "待 merge: $b"
done
```

每轮进度扫描跑一次。**「分支有 commit 而 master 没有」= 有人在等我 merge。**

⚠️ 拓扑判据（`--is-ancestor`）**只在报 0 时可信**：它对空分支空真（假阳），
对 rebase/amend 换过 sha 的内容假阴。报非 0 时补内容判据——
`git diff <旧> <新>` 挑字串，或直接比目标文件 sha256。

**给 dev 的对应纪律**：**绝不能因 idle hook 催就转 `dev_done`**。
实证形态是「主仓库无我任何文件而 `go test` 是绿的」——门禁量的根本不是你的交付。

---

## AD-5 TASK-005 的 54 项 `magnitude_ranges` 由 dev 按三条原则填，Leader 复核

**问题**：需求文档明写「本步是人工判断，不写代码、不自动生成」。

**决定**：dev **先跑 calibrate 产出「建议区间」列**（那是工具给的依据），再按文档的三条
原则逐项调整，**每一项的取值理由写进 discovery 的 `decisions`**。
Leader 在 `verified → accepted` 前抽查，不通过则走 `review_fix`。

**理由**：文档说的「人工」是相对于「写脚本自动生成」而言——它要防的是「工具替人做判断」。
由 agent 逐项判断并**留下逐项理由**，满足这个约束；而完全停下来等人填 54 个数会让任务
卡在 `blocked_human`，且人手上并没有比 calibrate 报告更多的信息。

**兜底**：TASK-004 的校验（未知字段名 / 区间倒置 / 缺 unit）会在 `LoadConfig` 当场报错。
这正是 004 被排在 005 前面的原因。

---

## AD-6 TASK-010 的「数字仍有效」判据要按路径收窄

**问题**：需求文档 Task 8 Step 5 的判据是 `git diff --numstat <采样锚> HEAD` **为空**。
而 TASK-010 自己要改 `CONTRACTS.md`（把数字填进去）⇒ numstat 必然非空 ⇒ 判据自我否定。

**决定**：判据收窄为「**产生这些数字的文件**没变」：

```bash
git diff --numstat <采样锚> HEAD -- internal/hestia cmd/atlas configs \
  ':!internal/hestia/CONTRACTS.md'
```

**为什么不能直接放宽成「我这轮只改了文档」**：那个推断对 `go test` 成立、
**对自证数字不成立**——数字不来自被改动的文档，而可能来自**别人**在这期间改的代码。
收窄的是路径，不是「谁改的」。

---

## AD-7 `final-report.md` 的数字由 Leader **现采**，不引用任务中期数字

**问题**：`06-acceptance/` 的验收报告不在任何任务的 `writes` 里 ⇒ 没有 dev owner ⇒
「依赖变化后谁重采」这个角色不存在。归档里实测过它连续四轮数字过期无人发现。

**决定**：Leader 写 final-report 时**当场重采**每个数字，并在每个数字旁标注**采样锚的全 sha**。
不引用任务 discovery 里的数字（那些采于各自的树）。

---

## AD-8 分支命名带 `-m1c3b` 后缀

**理由**：仓库里已有 19 个 `task/TASK-00x-m1c3a` 遗留分支跨 sprint 累积。
同名分支会挡住 `git worktree add`。

**分支名**：`task/TASK-00X-m1c3b`。**worktree 路径**：`../wt-TASK-00X-m1c3b`。

---

## AD-9 待同步 hooks 清单（本 sprint 发现，不修）

| 文件 | 现象 | 处置 |
|---|---|---|
| `.claude/hooks/arcforge-write.sh:1007` | `--as leader --help` 报 `CMD<乱码>: unbound variable`——macOS bash 3.2 把紧跟 `$CMD` 的 CJK 字节并进变量名，应写 `${CMD}` | 运行时资产只读，登记进 final-report 待同步清单 |

对本 sprint 无实质影响：各子命令本身工作正常，只 `--help` 路径受影响。
