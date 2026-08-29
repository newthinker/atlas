# verify_baseline 盲区全量审计（TASK-001~006）

**执行者**：team-lead ｜ **执行时刻**：2026-08-26
**master = `e21be041104300ee984ff3767944e8e36a238bdd`**

> ⚠️ 本应落 `docs/04-test/`（原派 test-m1c3a-v1，它撞 session limit），
> 但该目录写者限 `test-*`，Leader 自执行后只能落 `06-acceptance/`。

---

## 🔴 本报告曾有一版结论是错的，先说这个

**初版结论「命中 1 例（TASK-002）」是假阳，已推翻。真值是 0 例。**

初版的判据是拓扑的 `git rev-list <baseline>..<branch>`：它报 `task/TASK-002-m1c3a`
有 1 个 commit（`55a5fef`）在 baseline 之外，我据此宣布发现了框架级盲区，
并把这条写进了清单、plan、checkpoint 和本报告。

**它假在哪**：

| commit | 时刻 (+0800) | 来源 |
|---|---|---|
| `55a5fef` | 08-26 07:54:16 | dev-b 在 `task/TASK-002-m1c3a` 分支上 |
| **`bca01bd`** | **08-26 08:05:08** | **team-lead 自己**在 master 上手工补的（`git checkout 8256ccb -- <file>`） |

**同一份内容，两个 sha。** 我 11 分钟后手工补的那个先进了 master。
于是拓扑判据看见「`55a5fef` 不是 master 的祖先」就报未合入，
而**内容一个字节都不缺**。

**决定性证据**（三条互相独立）：

1. `git show cea6b3c:internal/hestia/extract_test.go | grep -c 'A4d 8 个分项全部改指结构段'`
   → **1** ——`cea6b3c` 正是 TASK-002 的 `verify_baseline.head`，
   **验证者验的那棵树里就有那 22 行**。
2. `git merge-base --is-ancestor bca01bd cea6b3c` → **是**（内容早于 baseline 6 分 49 秒进入）。
3. `git diff --numstat <baseline> master -- internal/hestia/extract_test.go` → **空**，
   两版 sha256 均为 `7e0481856041cef9`。我那次 merge（`e21be04`）**内容层面是 no-op**。

⇒ **TASK-002 的 VERIFIED 判定对象是完整的，没有验错对象。**

**根因（这才是本报告唯一有价值的部分）**：
判「进去了没」我用了**拓扑**判据（`rev-list` / `merge-base --is-ancestor`），
而 `rebase` / `cherry-pick` / 三方 merge / **手工 `checkout -- <file>` 重放**都会换 sha。
**sha 是载体的名字，不是载体。** 这条我记过两次（2026-08-25 各一次，
一次假阴「名字换了」、一次假阳「空分支空真」），**今天是第三次，形态是新的第三种：
内容已在、sha 不在**。

---

## 订正后的结论

**8 个分支 ref / 6 个任务全部检查，盲区实例 0 例。**

| 任务 | 分支 | baseline 外 commit（拓扑） | 内容判据复核 | 判定 |
|---|---|---|---|---|
| TASK-001 | `task/TASK-001-m1c3a` | 0 | — | ✅ |
| TASK-001 | `task/TASK-001-m1c3a-fix` | 0 | — | ✅ |
| TASK-002 | `task/TASK-002-m1c3a` | 1 (`55a5feffb`) | **内容已在 baseline 内** | ✅ 假阳 |
| TASK-003 | `task/TASK-003-m1c3a` | 0 | — | ✅ |
| TASK-004 | `task/TASK-004-m1c3a` | 0 | — | ✅ |
| TASK-005 | `task/TASK-005-m1c3a` | 0 | — | ✅ |
| TASK-005 | `task/TASK-005-m1c3a-fix` | 0 | — | ✅ |
| TASK-006 | `task/TASK-006-m1c3a` | 0 | — | ✅ |

⚠️ **注意「0」这一列不需要内容复核，而「非 0」必须做**：
拓扑判据只在**报 0** 时可信（sha 在祖先里 ⇒ 内容必在）；
**报非 0 时它既可能是真盲区，也可能是同内容换了 sha**，两者输出完全一样。
本 Sprint 唯一一次非 0 就是假阳。

## 那个机制本身还成立吗

**推理上成立，但本 Sprint 没有任何实例支撑它。**

机制描述——`verify_baseline` 锚主仓库 `git rev-parse HEAD`，
而 dev 的交付物在 worktree 分支上，未 merge 的 commit 对它拓扑隐形——
这在 git 语义上是对的。AD-29 防的是「判定后交付物又变了」（baseline **落后于**交付物），
确实防不住「判定时交付物还没进来」（baseline **看不见**交付物）。

**但我原来举的唯一实例是假的**，而单例支持的因果强度本就等同于没有，
现在连那个单例都没了。⇒ **这条降级为「推理上的可能缺口，零实证」**，
不作为下游 sprint 的行动依据，除非将来真撞到一次。

## 完整性断言的依据（test-m1c3a-v1 要求）

本报告写了「6/6 任务」「8 个分支全部检查」，依据：

1. **枚举源独立于被检查对象**：分支列表取自 `git for-each-ref refs/heads/`（仓库全量 ref），
   不是从任何一次改动的 diff 里数出来的（清单第 53 条的要求）。
2. **逐条打印「查了没命中」而非只打印命中**：上表 7 行 `✅` 是检查记录，不是沉默。
3. **自洽校验（一把独立的尺）**：8 个 ref 对应 6 个任务，多出的 2 个是 `-fix` 分支，
   恰属 TASK-001 与 TASK-005 —— 而这两个正是本 Sprint 仅有的 `rework_count == 1` 的任务。
   分支数与返工数由两条互不相干的路径产生，二者对上。

⚠️ **但这三条依据一条都没能拦住上面那个假阳**——它们保证的是「**该查的都查了**」，
不保证「**判据测的是我以为的那个性质**」。**完整性与正确性是两个维度。**
本报告初版把完整性论证做得很仔细，反而更像已经把结论证完了。

## 🔴 已声明的边界

**只覆盖当前仍存在的 ref。** 若某任务的分支在 `git worktree remove` 后 ref 也被删，
本方法查不到且**不会报错**——它安静地表现为「该任务 0 个分支」。
本次 6 个任务每个都至少有一个 ref 存活（上表可验），故本次无此缺口；
但这是**观察到的事实，不是方法保证的性质**。
