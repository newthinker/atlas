# M1c-3b 需求分析 · 历史回填批量入库

**需求文档:** `/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-31-hestia-backfill-load.md`（1408 行）
**Spec:** `superpowers/specs/2026-08-31-hestia-backfill-load-design.md`
**分析时间:** 2026-08-31
**分析者:** leader
**起始锚:** `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`（master，与需求文档声明的锚一致）

---

## 1. 目标一句话

把 M1c-1 抓到的 218 篇语料解析、按业务键合并、过闸、写进一个**全新**的权威库，
并产出可供人工核对与切换的报告。

## 2. 需求文档的性质（决定了本次分析的增值点在哪）

需求文档**已经拆到 step 级**：8 个 Task，每个带 Files / Interfaces / 逐步 checkbox /
期望输出 / 完整实现代码 / commit 命令。这不是常见的「散文式需求」，而是一份可直接执行的
实施计划。

⇒ **本次需求分析不复述它。** Leader 的增值只在三处：

1. **把 Task 1–8 转成 Arcforge 任务图**（writes 互斥 / DAG / wave），见 `design-spec.md`
2. **核实需求文档声明的每一个环境前提**（下面第 3 节，全部带证据）
3. **补上需求文档未覆盖的流程风险**（第 4 节）——它写的是「代码怎么改」，
   没写「在 Arcforge 的 worktree 隔离 + dev_done 门禁下这些命令会怎么失败」

## 3. 环境前提核实（每条带证据，2026-08-31 采于 `32bc1e5`）

| 需求文档的声明 | 核实结果 | 证据 |
|---|---|---|
| 语料 218 篇 | ✅ 218 | `ls data/hestia-backfill-2026-08-14/articles/ \| wc -l` = 218 |
| manifest 无 `completed_at` | 待 TASK-001 首次跑时确认 | 文档说 `--allow-incomplete` 是**必需**，不传直接报错 |
| Go 1.24.4 | ✅ | `go version` = go1.24.4 darwin/arm64；`go.mod` = go 1.24.4 |
| 无新增依赖 | ✅ 基线 `modernc.org/sqlite v1.38.2` | `grep sqlite go.mod` |
| `gofmt` 既有欠账仅两文件 | ✅ 精确相等 | `gofmt -l internal/hestia cmd/atlas` → `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` |
| `go vet` 干净 | ✅ 零输出 | `go vet ./internal/hestia/... ./cmd/...` |
| 覆盖率基线 95.9% | ✅ 精确相等 | `go test ./internal/hestia/... -cover` → `coverage: 95.9% of statements` |
| 生产库不得被改 | ✅ 已采基线指纹 | `shasum -a 256 /Users/zuowei/workspace/runtime/atlas/data/hestia.db` = `478d40c079c8b0eab7d089bb6f1926725b361a6dc6c850f4c4a651406f3ec28c` |
| `sqlite3` CLI（TASK-010 核对用） | ✅ 3.51.0 | `/usr/bin/sqlite3` |

**基线指纹的用途**：交付前检查清单最后一条「生产库未被本迭代改动」以此为判据。
⚠️ 判据是 **sha256 相等**，不是「我没写过它」——后者是关于意图的声称，前者是关于载体的观察。

## 4. 需求文档未覆盖的流程风险（Leader 增补）

### 4.1 🔴 语料在 linked worktree 里**不存在**

需求文档里所有真跑命令都写 `--dir "$PWD/data/hestia-backfill-2026-08-14"`。

而实测：`.gitignore:64` 是 `data/`，`git ls-files data/` 返回 **0 个文件**。
⇒ **linked worktree 里根本没有 `data/` 目录**，`$PWD/data/...` 在 worktree 里必然指向不存在的路径。

受影响：TASK-001（Step 1 采基线 / Step 6 背对背比对）、TASK-005（Step 5 产出建议区间）、
TASK-010（真跑验收）。

**处置**（已写进相关任务的 done_criteria）：一律用**主仓库绝对路径**
`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/hestia-backfill-2026-08-14`
作 `--dir`，二进制在自己的 worktree 里 build。**不要建软链**——软链会让「我测的是哪份语料」
再多一层间接，而这批数字（42/107/96）正是要靠语料自证的。

⚠️ 这是**响的**失效（路径不存在会报错退出），不是静默的，但它会在 TASK-001 的第一步就
撞上，且撞上时的表象是「工具坏了」而不是「路径错了」。

### 4.2 🔴 merge 必须发生在 `dev_done` **之前**

`task-completed.sh` 的 `git log --grep` **全文件不带 `--all`** ⇒ 只走 HEAD 祖先链 ⇒
未合并分支上的 commit 对门禁**结构性不可见**，两个集合双双为空，门禁于是「报绿」。

本仓库根**有** `go.mod` ⇒ dev_done 门禁是**生产级触发**，不走 skip 分支。

⇒ 流程固定为：worktree 提交 → **通知 Leader merge** → Leader merge 进 master →
**dev 回主仓库**跑 `transition dev_done`。

⚠️ 「dev 交付完等 Leader merge」是流水线上唯一没有活性保障的环节：它在文件层与「还在写代码」
完全同形，`stale-dispatch` 刻意不含 `in_progress`，而 idle hook 的解锁文案在此状态恒为
「推进 dev_done」——**方向恰好相反**。Leader 侧的补偿扫描见 `architecture-decisions.md` AD-4。

### 4.3 TASK-005 的 `magnitude_ranges` 54 项是「人工判断」

需求文档明写「本步是人工判断，不写代码、不自动生成」。在 Arcforge 里这需要一个落点，
否则会卡成 `blocked_human`。处置见 AD-5。

### 4.4 写通道 `--help` 自身有中文变量插值缺陷

`bash .claude/hooks/arcforge-write.sh --as leader --help` 报
`line 1007: CMD<乱码>: unbound variable`——macOS bash 3.2 把紧跟 `$CMD` 的 CJK 字节并进了变量名。

**不修**（运行时资产对全体 agent 只读），登记进 final-report 的「待同步 hooks 清单」。
对本 sprint 无实质影响：各子命令本身工作正常，只有 `--help` 这条路径受影响。

## 5. 复杂度评估

| 维度 | 判断 |
|---|---|
| 总体 | **中等偏复杂** |
| 最难的一步 | TASK-006（`BackfillLoad` 编排）——它同时承接三个上游接缝，且有 9 条实现要点 |
| 最容易被做错而不自知 | TASK-001（纯重构）——`res.Periods++` 的位置错了会让 199 变 161，而测试全绿 |
| 唯一不可单测的 | TASK-010 的 42/107/96——语料不进 git，真跑是它们**唯一**的验收点 |
| 需人工介入 | TASK-005 的 54 项填值 |
