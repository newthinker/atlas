# TASK-007（Arcforge）/ TASK-005 后半（需求文档）交付记录：`verify.py` + 统一开 PR

> 承载形态见 `.arcforge/docs/01-design/design-spec.md` §3：本文件**不是摘要，是证据载体**。
> 验证者不以本文件为准——按 ③节的全 sha 锚点去 nanoclaw 实跑。
>
> **编号映射**：Arcforge TASK-007 = 需求文档「## TASK-005」节的**后半**（`prepare.py` 归 TASK-006）。
> atlas commit subject 用 Arcforge 编号（`docs(TASK-007):`），脚本注释里的里程碑编号用需求编号（`M3 的 TASK-005`）。
> 两处编号不同是**预期的**。
>
> **采样纪律**：本文件所有计数、sha、行数均采于**最后一次改动（nanoclaw commit
> `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`）之后**，与 ③节锚点同一时刻。
>
> 🔴 **worktree 已在本任务收尾时拆除**（TASK-004 建 / 本任务拆）⇒ ②节的命令**不能在原路径重跑**。
> 复现方式：`git -C /Users/zuowei/workspace/ai/nanoclaw worktree add <新临时目录> 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`
> 后在 `container/skills/warp-hestia/scripts/` 下执行。分支与提交都还在（④节 E 有证明）。

---

## ① 改动清单

本任务两个 commit，**均只动 `scripts/` 下四个文件**：

```
$ git show --numstat --format='' 3690015a496d1fae2d219269572b1e507586467c
6	1	container/skills/warp-hestia/scripts/prepare.py
17	1	container/skills/warp-hestia/scripts/test_prepare.py
303	0	container/skills/warp-hestia/scripts/test_verify.py
147	0	container/skills/warp-hestia/scripts/verify.py

$ git show --numstat --format='' 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8
1	1	container/skills/warp-hestia/scripts/test_prepare.py
```

**本任务累计**（相对 TASK-006 交出时的 `e8d0decaab2a9bcfd29c952ebff11356f63dd3ba`）：

```
$ git diff --numstat e8d0decaab2a9bcfd29c952ebff11356f63dd3ba 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8
6	1	container/skills/warp-hestia/scripts/prepare.py
17	1	container/skills/warp-hestia/scripts/test_prepare.py
303	0	container/skills/warp-hestia/scripts/test_verify.py
147	0	container/skills/warp-hestia/scripts/verify.py
```

| 文件 | 改动 | 对应 DoD |
|---|---|---|
| `scripts/verify.py` | **新建 147 行** —— 两级校验的消费者 | functional[0][1][2]、boundary、error_handling |
| `scripts/test_verify.py` | **新建 303 行 / 29 个测试** | 同上 |
| `scripts/prepare.py` | `+6/−1` —— `--now` 缺省取当天（含 4 行说明为什么） | functional[2] 的裁决 |
| `scripts/test_prepare.py` | `+17/−1` 与 `+1/−1` —— 补 2 条断言、修 2 处编号前缀 | functional[2]、non_functional[0] |

### 🔴 `__pycache__` / `*.pyc` 自查（DoD non_functional[2]）

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw show --numstat --format='' \
    3690015a496d1fae2d219269572b1e507586467c 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8 \
    | grep -c '__pycache__\|\.pyc'
0
```

两次提交前都在**最后一次跑测试之后**执行 `find … -name '__pycache__' -type d -exec rm -rf {} +`。
**没有改 `.gitignore`**（仓库级配置，超出本任务 `writes` 声明），沿用 TASK-006 的克制。

### `warp-research` 一行未动

```
$ git diff --numstat d791101e1defcfd3d3d3fcf7ae32a85ec420547b 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8 -- container/skills/warp-research | grep -c .
0
```

---

## ② 实测输出

### 2.1 TDD 红阶段留痕（error_handling 要求）

`verify.py` 尚不存在时：

```
$ cd <worktree>/container/skills/warp-hestia/scripts && python3 -m unittest test_verify -v 2>&1 | tail -5
Ran 26 tests in 0.385s

FAILED (failures=21, errors=4)
$ echo "EXIT=$?"
EXIT=1
```

26 条里 25 条红。**剩下 1 条通过的是 `ReusesPrepare.test_no_second_definition`**——它断言的是
「`verify.py` 里没有第二份 `seal_digest`/`with_check` 定义」，文件不存在时该断言**恰好成立**。
⚠️ 如实记下：这是一条**在被测对象缺席时也会绿**的断言，红阶段抓不到它。
（它在 GREEN 阶段由 `test_imports_prepare` 与 `test_uses_prepare_functions` 补齐——后两条文件不存在时必红。）

### 2.2 最终测试（84 条全绿）

```
$ cd <worktree>/container/skills/warp-hestia/scripts && python3 -m unittest -v 2>&1 | tail -3
Ran 84 tests in 2.233s

OK
$ python3 -m unittest >/dev/null 2>&1; echo "EXIT=$?"
EXIT=0

$ grep -c 'def test_' test_prepare.py      → 55
$ grep -c 'def test_' test_verify.py       → 29
                                     合计 → 84
```

`Ran 84` 与 `grep -c 'def test_'` 之和 **两个口径一致**。
⚠️ 退出码**单独取**，不跨管道——`tail` 几乎总 exit 0，跨管道取会系统性报成功。

### 2.3 只用标准库（functional[0]）

```
$ grep -n '^import \|^from ' verify.py
23:import argparse
24:import os
25:import re
26:import sys
29:import prepare  # noqa: E402  —— 必须在 sys.path 插入之后
```

四个标准库 + 同目录的 `prepare`，无第三方依赖。

### 2.4 🔴 复用 `prepare`，不另写一份（functional[2]）

**自查①——本文件里的调用都来自 `prepare`：**

```
$ grep -n 'seal_digest\|with_check' verify.py
6:🔴 **作用域定义不在本文件里重新实现**——`seal_digest` 与 `with_check` 一律从 `prepare` 导入。
114:        # 作用域文本仍必须用带行尾的 lines 拼；期望值只经 prepare.with_check 得出，本文件不另算
116:        want_line = prepare.with_check(scope).rstrip("\n").splitlines()[-1]
124:    want_seal = prepare.seal_digest(md)
```

**自查②——本文件里没有第二份定义：**

```
$ grep -cE '^def (seal_digest|with_check)\b' verify.py
0
```

另有 `test_uses_prepare_functions` 用 `assertIs(verify.prepare.seal_digest, prepare.seal_digest)`
**在对象同一性层面**钉住——比 grep 更硬：即便有人写了个同名包装转发过去，`assertIs` 也会红。

### 2.5 四态实跑（functional[1]，原样粘贴）

```
$ python3 verify.py fixtures/golden/2026-06-h1.md
$ echo "EXIT=$?"
EXIT=0                                    ← ① 未改动

$ python3 verify.py <把表内 2212 改成 9999 的副本>
段「本期数据」的 check 不匹配（第 50 行）：
  期望 d6d955123e3943f454a01eca0e3ab04546116099aff48b4e3717ce7738658b5f
  实得 3b43497fb821b8246318ff8b2896badd67dc2136faa90cbf66d8e48e145096b2
EXIT=1                                    ← ② 表内数字被改：指出了**哪一段**、**期望与实得的 hex**

$ python3 verify.py <只在 narrative 里插入带 `## ` 小标题的叙述的副本>
EXIT=0                                    ← ③ 只改叙述

$ python3 verify.py <删掉封条行的副本>
封条 seal 行应为 1 条，实得 0 条 —— 判为被篡改
EXIT=1                                    ← ④ 删除类篡改
```

⚠️ **需求原文 `test_rejects_edited_table` 用的 `462.06` 在实际输出里不存在**（`grep -c` 命中 0，
那是需求作者臆想的值）⇒ 改用表内真实值 `2212`，并在测试里先 `assertIn("| 2212 |", self.md)`
**把这个前提本身也断言掉**——否则将来表格变了，这条测试会静默变成「替换了个不存在的字串然后验没改过的文件」。

**④ 的四种删除类篡改各有一条测试**（`test_4a`～`test_4d`）：删一条 check 行 / 删整段（标题+表+check）/
改无 check 保护的 `## 信号` 段 / 删封条。**前三种分段 check 各自仍自洽，只有封条抓得住**。

### 2.6 `--now` 缺省修复（functional[2] 的裁决）

```
$ python3 prepare.py fixtures/2026-06-h1.json fixtures/2026-06-h1.history.json | sed -n '4,5p'
created: 2026-09-08
updated: 2026-09-08
```

不传 `--now` 时两字段**非空且等于当天**。新增两条断言：`test_created_updated_nonempty_without_now_flag`
（不传时等于当天）与 `test_explicit_now_still_wins`（显式传时仍以它为准）。

🔴 **两份 golden 逐字节未变**（现有测试都经 helper 显式传 `--now 2026-09-12`）：

```
$ python3 prepare.py fixtures/2026-06-h1.json      … --now 2026-09-12 | shasum -a 256
13d6e47bd775bd3d3b4bfcd60ffc40975d648b8e33dcebb5598ef80ac6f6bfc7   ← 与 fixtures/golden/2026-06-h1.md 相同
$ python3 prepare.py fixtures/2023-08-monthly.json … --now 2026-09-12 | shasum -a 256
638cb1a52f585d99d101c741af92a0a9324ef5ca04ceba2d9ab687a9811d1586   ← 与 fixtures/golden/2023-08-monthly.md 相同
```

⚠️ 这是**独立复算**（重新生成后比 sha256），不是「文件没被我碰过所以没变」。

### 2.7 变异测试：13 个全部 KILLED、0 存活

⚠️ 变异只在 `mktemp -d` 隔离副本上跑；每轮校验主工作区 `verify.py`+`prepare.py` 的 sha256 与
`git status --porcelain` 指纹，**每轮均未变**。每个变异体落盘后过 `ast.parse` 语法闸。

🔴 **harness 对锚点命中数 ≠1 直接非零退出（`sys.exit(3)`），不静默跳过**——这一手照抄 test-m3-a
在 TASK-006 验证里的写法。TASK-006 我那版是 `print("锚点不唯一，跳过")` 然后 `continue`，
**打印了但不影响任何判定**，与「硬编码的 echo 永远不会错也永远不告诉我任何事」同族。

| 变异 | 内容 | 结果 |
|---|---|---|
| V1 | 封条整个不验（只验分段 check） | ✅ 红 1 |
| V2 | check 条数改从 `## ` 标题数推 | ✅ 红 10 |
| V3 | 格式损坏当成「没有校验行」放行 | ✅ 红 4 |
| V4 | 同段多条 check 不判篡改 | ✅ 红 1 |
| V5 | check 作用域少算标题行 | ✅ 红 8 |
| V6 | seal 条数不校验 | ✅ 红 1 |
| V7 | 机器区外的校验行也放行 | ✅ 红 1 |
| V8 | 分段 check 不匹配也放行 | ✅ 红 1 |
| V9 | 空文件分支删掉 | ✅ 红 2 |
| V10 | 非笔记格式分支删掉 | ✅ 红 2 |
| V11 | seal 不扣 narrative（改 `prepare.py`） | ✅ 红 10 |
| V12 | scope 用去尾换行的 `bare` 拼 | ✅ 红 8 |
| P1 | `--now` 缺省改回空串 | ✅ 红 1 |

🔴 **其中 3 个是先存活、补断言后才杀死的，而它们的共同形状是新的**——不是「夹具取值恒定」
（TASK-006 那次），而是**一条判据被更早的判据遮蔽，因而从未被行使**：

| 存活的 | 被谁遮蔽 | 补了什么 |
|---|---|---|
| **V4** 同段多条 check | 原测试只是**复制**一条 check ⇒ 总数变 3，被「恰好 2 条」那条**更早**的判据挡下 | `test_two_checks_both_in_one_section`：构造**总数仍为 2、但两条都在第一段** |
| **V7** 机器区外的校验行 | 没有任何测试构造过「总数仍 2/1、但有一条在机器区外」 | `test_check_line_outside_machine_zone`：把第二条 check **移到**批注区之后 |
| **V9** 空文件分支 | 空文件也不含 `begin`/`end` ⇒ 被「不是笔记格式」分支接住，**两条给出相同退出码**，而断言只查退出码 | `test_empty_file` 加断言消息含「空文件」；另加 `test_empty_and_not_a_note_give_different_messages` |

⚠️ **V9 那条尤其值得记**：覆盖率会显示空文件分支**被执行了**（它确实被执行），
所以覆盖率、断言数、DoD 义务清单**三样都看不见这个缺口**——只有变异看得见。

### 2.8 任务编号前缀（non_functional[0]）

```
引用总数 = 9   带 `M3 的 ` 前缀 = 9
```

🔴 **自查时抓到 1 处漏网并修掉了**（`test_prepare.py:271`，我本轮新写的 docstring）。
它有**两个**问题：缺前缀，且引的是 **Arcforge 编号 `TASK-006`** 而非需求编号——
注释的读者是三仓库开发者，读的是需求文档不是 `.arcforge/` 归档。已改为
「验证者在 M3 的 TASK-005 的 prepare 部分验收时发现」，单独一个 commit（`2d2fbe80…`）。

⚠️ 如实记：**第一次自查我用的口径是「总数 vs 带前缀数」，得 9 vs 8 才发现有漏**。
若我只 grep `'M3 的 TASK'` 数得 8 条并觉得「都带了」，这处会溜过去——
**两个数必须都数，差值才是信号**。

### 2.9 code-simplifier（全局规范 + DoD）

跑了**两次**：

1. **第一次被 API 中断**（`ECONNRESET`，只读完文件、4 次工具调用）。核实：四个源文件 + 两份 golden
   与基线**逐字节相同**，84 测试仍全绿 ⇒ 未造成任何改动。
2. **第二次完成**，报告是「完成，无新动作」——**而 `diff` 显示它改了 `verify.py` 与 `test_verify.py`**。
   按 DoD「子代理回复不可采信，以 `git diff` 为准」核实。

它的实际改动（逐条读过后接受）：
- `verify.py`：预算 `bare = [line.rstrip("\n") …]` 复用；三元表达式改 `if/else`；比对从「整行」改为「hex」；
  **并加了一行注释**说明作用域仍须用带行尾的 `lines` 拼。
- `test_verify.py`：抽出 `first_marker_line()` / `verify_src()` 两个 helper。

约束逐条核实：**golden 两份 sha256 未变、`def test_` 84→84、`verify.py` 里 `seal_digest`/`with_check`
的 `def` 定义数仍为 0、import 仍只有标准库 + `prepare`、`Ran 84 / OK / EXIT=0`。**

🔴 **更强的一层**：它改了 `verify.py` 的**代码形状** ⇒ 我的变异锚点 V8 随之失效
（harness 当场 `FATAL: 锚点命中 0 次` 非零退出，**没有静默跳过**）⇒ 按新形状重写锚点，
**13 个变异全部重跑**，仍 KILLED 13 / SURVIVED 0。上表就是简化**之后**那一轮。
并顺势补了 **V12**（针对简化引入的新风险面：若 scope 改用去尾换行的 `bare` 拼会怎样）——**红 8，被杀死**。

---

## ③ 锚点

| 项 | 值 |
| --- | --- |
| nanoclaw 分支 | `feat/warp-hestia` |
| nanoclaw 本任务 commit ① **全 sha** | **`3690015a496d1fae2d219269572b1e507586467c`**（`verify.py` + `--now` 修复） |
| nanoclaw 本任务 commit ② **全 sha** | **`2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`**（编号前缀订正）= **分支 HEAD** |
| nanoclaw 上游 commit（TASK-006 交出时） | `e8d0decaab2a9bcfd29c952ebff11356f63dd3ba` |
| nanoclaw 分支起点（TASK-004 交出时） | `d791101e1defcfd3d3d3fcf7ae32a85ec420547b` |
| **PR 链接** | **https://github.com/newthinker/nanoclaw/pull/5** （`feat/warp-hestia` → `main`，**OPEN**，5 commits，23 files） |
| 远端分支 HEAD | `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`（`git ls-remote` 与本地一致） |
| nanoclaw 主 checkout | `feat/vendor-agent-reach-skill` / `aefea6ce5439cfcf15b3dc54ea9bd507c5846c95`（**未触碰**） |
| nanoclaw worktree | 🔴 **已拆**（见 ④节 E） |
| atlas 基线 HEAD | `b5873ad840019bbccf7c4d5a41f643db8408d4b3` |
| atlas worktree / 分支 | `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-TASK-007-m3` / `task/TASK-007-m3` |

**PR 的整体口径**（相对分支起点 `d791101…`，即 PR 的全部内容）：

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw diff --numstat \
    d791101e1defcfd3d3d3fcf7ae32a85ec420547b 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8 \
    | awk '{a+=$1;d+=$2;n++} END{printf "%d 个文件, +%d / -%d\n", n, a, d}'
23 个文件, +7349 / -0
```

PR 里 `__pycache__`/`*.pyc` 文件 **0** 个、`warp-research` 文件 **0** 个（`gh pr view --json files` 实查）。

> 🔴 **给验证者**：`verify_baseline` **只锚 atlas，够不到 nanoclaw**。请在判定前后各记一次
> nanoclaw 分支 HEAD，与 `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` 比对。
> ⚠️ **worktree 已拆**，请用 `git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse feat/warp-hestia`
> 或 `gh pr view 5 --json headRefOid` 取，**不要**去原 worktree 路径（已不存在）。

---

## ④ 未做与理由

### A. 🔴 **只开 PR，不合并**

DoD non_functional[0] 明写。需求交付清单 line 1036「nanoclaw：PR 合并到 `main`；本机 checkout 在 `main`」
是**人执行**的前置（属需求 TASK-006 Step 0，本 sprint 结转）。

⇒ **没有跑 `gh pr merge`**（`fork/main` 是共享分支），也**没有**把本机 checkout 切回 `main`
（那会动到别人未提交的改动——主 checkout 现有 2 改 2 删 1 未跟踪）。
**PR 未合并不构成本任务未完成**：开完 PR、把号写进 ③节，交付即完整。

### B. 需求原文 `class Verify` 三条：**已实现且加强**

需求给的三条（`test_accepts_untouched` / `test_rejects_edited_table` / `test_narrative_edit_ok`）
全部实现，另按 DoD 补到 **29 条**。三处有意偏离：

1. **`462.06` → `2212`**：需求那个值在实际输出里不存在（见 2.5），且**把前提本身也断言了**。
2. **不用固定的 `/tmp/m3-*.md` 路径**：需求原文写死 `/tmp/m3-ok.md` 等。改用 `tempfile.mkstemp` +
   `finally: os.unlink`。⚠️ 固定路径会让**上一轮的残留或并发进程的文件**冒充本轮输入 —— 这是本
   sprint 记过的假 PASS 来源（`tests/hooks` 固定写 `/tmp/…` 致假 PASS）。
3. **`test_narrative_edit_ok` 加强为两条**：需求只插一句普通叙述；另加
   `test_3_narrative_with_markdown_headings_ok`，在 narrative 里写 `## （一）经济是在扩张…` 这样的
   小标题——**methodology 的八问框架天然诱导模型这么写**，这正是封条必须扣除 narrative 的理由。

### C. 不建 `examples/2026-06-h1.md`

spec §5.1 目录树里有它，但注明是「冒烟产物经人审后回填」，属需求 TASK-006（人执行）。

### D. 🔴 产物在冒烟时的可见性依赖两步人执行动作（结转）

`src/container-runner.ts` 的 `projectRoot = process.cwd()`，skills 从 `<projectRoot>/container/skills`
**只读 bind mount** 到 `/app/skills` ⇒ **容器只看得见主 checkout 工作树里的 skill**。
**不需要重建镜像**（Dockerfile 无 `COPY skills`）。冒烟前必须 ①本 PR 合并到 `main`
②本机 checkout 切回 `main`——**两步都是人执行**（TASK-008 §D）。

⚠️ 不影响本任务交付：开发与 `python3 -m unittest` 都在 worktree 里跑，不经容器。

### E. worktree 收尾证明（本任务拆）

拆前先确认 **PR 已建、提交已推送**（远端 HEAD == 本地 HEAD == `2d2fbe80…`，脏文件数 0）：

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw worktree remove /Users/zuowei/workspace/ai/wt-warp-hestia
$ echo "EXIT=$?"
EXIT=0
$ git -C /Users/zuowei/workspace/ai/nanoclaw worktree prune
$ echo "EXIT=$?"
EXIT=0

$ git -C /Users/zuowei/workspace/ai/nanoclaw worktree list
/Users/zuowei/workspace/ai/nanoclaw  aefea6c [feat/vendor-agent-reach-skill]
```

`wt-warp-hestia` 残留 **0**、目录**已删**。**分支与提交都还在**：

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse feat/warp-hestia
2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8
```

主 checkout 仍在 `feat/vendor-agent-reach-skill`，工作区状态与开工前逐字相同。

### F. 未改 `.gitignore`

`__pycache__` 每跑一次测试就重新生成，本仓库 `.gitignore` 没有对应规则。本任务**在最后一次跑测试
之后手工排掉**，两次提交里 `.pyc` 均为 0。**没有改 `.gitignore`**——那是仓库级配置，超出本任务
`writes` 声明，沿用 TASK-006 的处置。⇒ 若将来还有人在这个目录下加 Python 文件，这一步仍需手工做。
