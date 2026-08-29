# baseline 盲区全量复核 —— 「判定时交付物还没进来」

> **2026-08-28 修订**：原结论写作「**命中 1 例**（TASK-002 / `55a5fef`）」。
> Leader 指出并经我复核：**那是拓扑判据的假阳，内容判据下盲区不成立**。见 §3。
> 处置结论不变（无需重验），但**性质从「命中 1 例盲区」改为「拓扑命中 1 例、内容复核后不成立」**。

- **审计者**：test-m1c3a-v1
- **要查的失效**：`verify_baseline.head` 锚主仓库 HEAD，而 dev 的 commit 可能还在未 merge 的
  worktree 分支上 ⇒ **判定时已存在、却不在 baseline 祖先里**。与 AD-29 方向相反。
- **结论**：**8 个已 verified 任务 / 10 个 (任务, 分支) 对，实际盲区 0 例。** 无需重验任何任务。

## 1. 完整性依据

不用「列个清单逐个查」——那只证明我查完了**我列的**。用**两个互相独立的枚举源**，
再证明两个残余缺口都不存在：

- **源 A（任务侧）**：`find .arcforge/tasks` 筛 `status=="verified"` ⇒ `001–007, 009`（**8 个**）
  > ⚠️ Leader 派的是「001–006 六个」。审计期间 007/009 也转了 `verified`，
  > 我按**当时的真实全集**审了 8 个，**没照抄派单里的数字**。
- **源 B（git 侧）**：`for-each-ref refs/heads/task/` ⇒ 16 个分支（含 4 个 `-fix`）
- **缺口一（分支被删）**：`reflog --all` 出现过的 `task/*` 名集合 − 当前集合 = **空**
  ⇒ 本 sprint 无分支被删，源 B 是全集
- **缺口二（命名不匹配）**：8 个任务逐个数匹配分支 ⇒ **每个 ≥1 个**，无「取不到 ref」

## 2. 判据的方向性（为什么 ✅ 是结构性的）

分支只前进 ⇒ `tip_当时 ≤ tip_现在`。故 **`tip_现在` 是 baseline 祖先 ⇒ `tip_当时` 必然也是**
⇒ 该对**结构上不可能有盲区**，不需要再比时刻。只有 `False` 的对才进入 §3 的时刻 + 内容复核。

⚠️ **时区**：`baseline.at` 是 UTC（`Z`），`git log` 默认 `+0800`。用
`datetime.fromisoformat(cI).astimezone(utc)` 换算后比较，**不做字符串比较**。
（我第一次的显示命令写了 `--date=format-local` 又手贴一个 `Z`，打出的是本地时间贴假 UTC 标记，已更正。）

## 3. 结果：10 对中 9 对结构性无盲区，1 对拓扑命中但**内容复核后不成立**

| 任务 | baseline.head | baseline.at (UTC) | 分支 | tip 是 baseline 祖先 |
|---|---|---|---|---|
| TASK-001 | `20c05ea46` | 00:23:16Z | `…-m1c3a` / `…-m1c3a-fix` | ✅ / ✅ |
| **TASK-002** | `cea6b3cb4` | **00:11:57Z** | `task/TASK-002-m1c3a` | 🔶 拓扑否 |
| TASK-003 | `cea6b3cb4` | 00:12:15Z | `task/TASK-003-m1c3a` | ✅ |
| TASK-004 | `33f3ed7cb` | 01:01:02Z | `task/TASK-004-m1c3a` | ✅ |
| TASK-005 | `b87de0cfc` | 06:43:09Z | `…-m1c3a` / `…-m1c3a-fix` | ✅ / ✅ |
| TASK-006 | `b4d0c9df2` | 06:17:38Z | `task/TASK-006-m1c3a` | ✅ |
| TASK-007 | `2ec9811b6` | 12:04:24Z | `task/TASK-007-m1c3a` | ✅ |
| TASK-009 | `80976e417` | 06:52:25Z | `task/TASK-009-m1c3a` | ✅ |

### 🔶 TASK-002：拓扑命中，内容复核后盲区不成立

拓扑上 `55a5feffb…`（`2026-08-25T23:54:16Z`）确实不在 `cea6b3c` 的祖先里，早于 baseline 17.7 分钟。
**但那份内容当时已经在 baseline 树里了**，只是换了个 sha：

```
55a5fef  2026-08-25T23:54:16Z  dev-b 在 task/TASK-002-m1c3a 分支上
bca01bd  2026-08-26T00:05:08Z  team-lead 在 master 上 checkout 同一份内容重放   ← 早于 baseline 6 分 49 秒
```

我的复核（三条，都在 **baseline 树 `cea6b3c` 上**取证，不是在当前 master 上）：

```
git show cea6b3c:internal/hestia/extract_test.go | grep -c 'A4d 8 个分项全部改指结构段'   → 1
git show cea6b3c:internal/hestia/extract_test.go | grep -c 'SURVIVED'                      → 1
git merge-base --is-ancestor bca01bd cea6b3c                                               → 是
两个 commit 对该文件各新增 23 行 '+'                                                        → 23 / 23
```

⇒ **验证者当时验的那棵树里就有那 22 行**，「判定时还没进来」根本没有发生。
（另：该 commit 非注释、非空白改动行 = 0，所以即便真是盲区也无需重验——但**性质不同**。）

🔴 **这是我上一版报告的实质错误，成因值得记**：我确实跑了内容判据，
**但跑在 `master` 上（「内容现在在不在 master」），而该问的是「内容当时在不在 `cea6b3c`」。**
判据形式正确、**取证的树错了**。⇒ 与清单第 58 条一致：
**拓扑判据只在报 0 时可信；报非 0 时必须补内容判据，且内容判据要打在 baseline 那棵树上。**

## 4. 时效与范围声明

1. **只覆盖「最终一轮」的 baseline。** TASK-001 / TASK-005 各有一次返工，第 1 轮 baseline 已被覆盖、
   不可回溯；但那两次第 1 轮都判 **REJECTED**，没有产生「验错对象的 VERIFIED」。
2. **这是一次快照。** `TASK-007` 的分支在我两次读 ref 之间就前进过（`80976e4` → `1d71ce33`）。
   后续任务转 `verified` 后需重跑本审计。

## 5. 机制建议

- **派验前置检查**（Leader 已采用）：`git branch --list 'task/<ID>-*'` 逐个
  `merge-base --is-ancestor <tip> HEAD`，不成立先 merge 再派验。
  ⚠️ 按 §3 的教训：**该检查报非 0 时要再跑一次内容判据**（打在目标树上），
  否则会把「同内容换了 sha」当成「未合入」，多做一次 `git diff --stat` 为空的无谓 merge。
- 或在 `verify_baseline` 里**同时记该任务分支的 tip**，让验证者能自己发现不一致。

⇒ 现状下**验证者无法从任务文件察觉这件事**：baseline 看起来完全正常，
分支上那个 commit 在任何以 baseline 为起点的命令里都不存在。
