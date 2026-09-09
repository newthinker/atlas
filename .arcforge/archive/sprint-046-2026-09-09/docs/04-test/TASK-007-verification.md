# TASK-007 验证报告

- **任务**：nanoclaw `verify.py`（校验行四态，含删校验行）+ `test_verify.py` + 统一开 PR + worktree 收尾
- **验证者**：test-m3-d（**第四任**；前三任 test-m3-b / test-m3-a / test-m3-c 均在「建完隔离 worktree 之后」失去响应，分别 87 / 94 / 62 分钟，被 TaskStop）
- **验证时间**：2026-09-09T01:28Z ～ 01:50Z（UTC）
- **assignment_epoch**：1（承接时记下，裁决迁移携带）

## 🔴 结论：**VERIFIED**

8 条 done_criteria 全部 PASS。3 个 finding 均**不构成缺陷**（详见 §5），其中 2 个需 Leader 知悉。

---

## 🔴 0. 验证严格度下降声明（必须留痕，不得淡化）

**本次验证未独立复核变异测试。**

DoD 之外，dev-m3-c 自报「13 个变异全部 KILLED、0 存活（V1–V12 + P1），其中 V4/V7/V9 为先存活、
补断言后杀死」。**本报告接受该自报数字，未做任何独立复算，也未运行变异 harness。**

原因：**前三任验证者恰在这一阶段连续三次卡死**（同一形态：建完 worktree 后再无落盘、
不回应问询），人类因此拍板换人并**缩小验证范围**，明确指示不复算存活变异、不跑变异 harness。

⇒ **这是验证严格度相对原定标准的一次下降。** 「13 变异全 KILLED」这一条**仅有 dev 单方声明支持**，
不具备本报告其余结论所依据的独立证据强度。若后续对该模块的断言强度有疑，此处是唯一未经二次确认的环节。

---

## 1. 三锚现读（判定前 / 判定后各一次）

`verify_baseline` **够不到 nanoclaw**（它只锚 atlas 的 head + discovery_sha256），故 DoD
non_functional[1] 要求验证者人工补这道闸——判定前后各记一次目标仓库 HEAD。

| 锚 | 判定前 01:28:34Z | 判定后 01:49:34Z | 结论 |
|---|---|---|---|
| nanoclaw `feat/warp-hestia` | `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` | `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` | **无漂移** |
| 我的 worktree HEAD | `2d2fbe8094…`（同上） | `2d2fbe8094…`（同上） | 一致 |
| PR #5 `headRefOid` | — | `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` | **三方一致** |
| discovery sha256 | `cffeb50062e84b5cc99dca958407e5f8e6131e3595abe0b2b8b3f8cb35f729df` | 同左 | 与 baseline 逐字节相同 |

**判定对象 = `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`**，与交付文档 ③节锚点、PR head 三方一致。

### atlas 侧基线漂移（声明范围内零变更，只应 INFO）

- `verify_baseline.head` = `cf7758bafa2f7f37f608497d9f1027537c100ddf`（四任共用，**未刷新**）
- 判定时 atlas HEAD = `fd933f493d15bfe7dea6bdbd5c4c740d34084e9c`
- 中间两个 commit（`698f7fa` / `fd933f4`）**只改 `PENDING-MECHANISMS.md`**，在我声明范围之外。
- **声明范围（`writes` = `docs/hestia-m3/TASK-007-warp-hestia-verify.md`）双判据核实**：
  `git diff --numstat cf7758ba fd933f49 -- <该文件>` 命中 **0** 行；内容判据 sha256 两端均为
  `20cd0b872cad4620f561b4af714bdc8cfae7030202c61471a4d2981e9709a67b`（**逐字节相同**）。
  ⇒ 只用拓扑/numstat 不够，故补内容判据；两者一致。

---

## 2. 测试执行

在**自建隔离 worktree**（`scratchpad/wt-nc-t7-test-m3-d`，detached @ `2d2fbe8094…`）中执行。
未触碰前三任残留的 `wt-nc-t7-test-m3-b` / `wt-nc-t7-test-m3-c`（留待 Leader 在阶段边界收）。

```
$ python3 -m unittest -v      # 退出码单独取，不跨管道
Ran 84 tests in 2.384s
OK
EXIT=0
```

**分层用两把独立的尺互验**（避免「总数守恒的再分配错误」恒过求和自洽校验）：

| 尺 | test_prepare.py | test_verify.py | 和 |
|---|---|---|---|
| 静态 `grep -c '    def test_'` | 55 | 29 | 84 |
| unittest 各自实际收集 | `Ran 55` | `Ran 29` | 84 |
| 全量实跑 | — | — | `Ran 84` |

三个口径一致 ⇒ **55 + 29 = 84 确认**。

---

## 3. done_criteria 覆盖矩阵（8 条）

8 条全部 `verify_by: manual`。**证据一律来自我本轮的独立实测**——直接调用 `verify.py` 构造篡改样本，
**不经 `test_verify.py`**：被验方的测试与被验方的实现同源，用它验它构成循环论证。

| # | 完成标准（摘要） | 独立证据 | 判定 |
|---|---|---|---|
| **functional[0]** | 只用标准库；作用域与 `note-format.md` **逐字一致**；全匹配 exit 0，否则 exit 1 并打印差异（哪一段/是否封条/期望与实得 hex） | import 仅 `argparse/os/re/sys` + 同目录 `prepare`；**按 note-format.md 文字自行实现算法**复算两份 golden（见 §4.1）全 MATCH；态②报错原样含段名 + 行号 + 期望/实得 hex | **PASS** |
| **functional[1]** | 四态，含删除类四种一律 exit 1；判据条数写死不从结构推 | **7/7 PASS**（见 §4.2）；源码 `EXPECT_CHECKS=2` / `EXPECT_SEALS=1` 为字面常量，非从 `## ` 计数推导 | **PASS** |
| **functional[2]** | 必须 `import prepare` 复用 `seal_digest`/`with_check`，不得另写一份；同时修 `prepare.py --now` 缺省 | DoD 两条自查均过；**另加两个更强判据**（见 §4.3）；不传 `--now` → `created`/`updated` 均 `2026-09-09`（当天、非空）；golden 三路证明未变（见 §4.4） | **PASS** |
| **boundary** | ①格式损坏报格式错不得当「没有校验行」放行 ②同段多 check / 第二条 seal ⇒ exit 1 ③批注区不参与校验 | **6/6 PASS**（见 §4.5），含「同段两条 check 而总数仍为 2」这一绕过条数闸的关键情形 | **PASS** |
| **error_handling** | ①文件不存在 ②非笔记格式 ③空文件 ⇒ 均非零且不是 traceback；红阶段留痕 | **3/3 PASS**（见 §4.6）；红阶段留痕见交付文档 §2.1（`Ran 26` / `failures=21, errors=4` / EXIT=1，并如实记「其中 1 条否定式断言恰好通过」，未粉饰成全红） | **PASS** |
| **non_functional[0]** | 统一开 PR **不合并**；拆 TASK-004 建的 worktree；编号带 milestone 前缀；不动 warp-research | PR #5 现读 `OPEN` / `mergedAt=null` / base=`main` / 5 commits 覆盖三任务（见 §4.7）；`wt-warp-hestia` 已拆（`worktree list` 残留 **0** 且目录不存在）；`*.py` 编号引用 **9** 处、带前缀 **9** 处；`warp-research` 变更文件 **0** | **PASS** |
| **non_functional[1]** | 交付文档固定四节、锚一律全 sha、commit 锚定 `docs(TASK-007):`、merge 进 master 后才 dev_done | ①②③④ 四节齐全；③节含 nanoclaw 全 sha `2d2fbe8094…`，与判定锚一致；`docs(TASK-007): ` 命中 1；merge **双判据**：拓扑 `IS-ANCESTOR` + 内容 sha256 `20cd0b87…` 两端相同 | **PASS** |
| **non_functional[2]** | 提交前排掉 `__pycache__`/`*.pyc` 且**不改** `.gitignore`；顺带修 `test_prepare.py:130` 编号前缀 | 两个 commit 的 numstat 中 pycache/pyc 命中 **0**；`.gitignore` 未在变更文件内；第 130 行前缀由 commit `3690015a` 补上（见 §4.8） | **PASS** |

**变异测试**：接受自报，**未独立复核**（原因见 §0）。

---

## 4. 独立实测明细

### 4.1 作用域定义与 `note-format.md` 逐字一致（最强判据）

DoD 要求「**逐字一致**」，Leader 要求「去比实际字串，别接受『应该一致』」。
源码对读只能证明「看起来像」，故我采用更强的做法：**只照 `note-format.md` 的散文重新实现一遍算法**
（不 `import prepare`、不参考 `verify.py`），对两份 golden 笔记复算全部摘要：

```
2023-08-monthly.md: check@行49 段「本期数据」MATCH / check@行69 段「前 12 期」MATCH / seal@行82 MATCH
2026-06-h1.md    : check@行50 段「本期数据」MATCH / check@行63 段「前 12 期」MATCH / seal@行76 MATCH
```

⇒ 证明的是**行为等价**，不是文本相似。规格两处原文与实现的对应：

- `note-format.md:83`「从**上一个 `## ` 标题行**（含该行）起，到本 check 行之前（不含本行）」
  ↔ `verify.py:115` `scope = "".join(lines[heading_i:idx])`
- `note-format.md:97-98`「从 begin 的**下一行**起、到本 seal 行**之前**，**扣除** narrative 与
  /narrative 之间（**含这两行本身**）」↔ `prepare.seal_digest()` 的三步 split

### 4.2 四态（7/7）

| 态 | 构造 | 期望 | 实得 |
|---|---|---|---|
| ① 未改动 | 原样 golden | exit 0 | **0** |
| ② 表内数字被改 | 第 38 行 `7.40`→`999.99`（`## 本期数据` 段内） | exit 1 且含 `check` | **1**，输出「段「本期数据」的 check 不匹配（第 50 行）：期望 `119857de…` 实得 `3b43497f…`」 |
| ② 补充 | 第二张表（`## 前 12 期`）同样改一处 | exit 1 | **1** |
| ③ 只改 narrative | 插入叙述**并在其中写 `## ` 小标题** | exit 0 | **0** |
| ④-1 | 删一条 `<!-- check: -->` 行 | exit 1 | **1** |
| ④-2 | 删整段（`## ` 标题 + 表 + 该段 check 行） | exit 1 | **1** |
| ④-3 | 改 `## 信号` 段（该段无 check 保护） | exit 1 | **1** |
| ④-4 | 删封条 `<!-- seal: -->` 行 | exit 1 | **1** |

④ 的三种「分段 check 各自仍自洽」的删除类篡改**确实只有封条抓得住**——与 DoD 所述机制一致。

> **仪器自纠留痕**：态②初次实测报 FAIL（exit=0）。**追查后确认是我的仪器错，不是实现缺陷**：
> 我用「第一个 `\d+\.\d\d`」定位表内数字，实际命中的是**第 17 行 frontmatter** 的
> `tsf_stock_yoy: 7.40`，而机器区自第 33 行才开始 ⇒ 它不在任何 check/seal 作用域内，
> `exit=0` 本就是正确行为。改为精确定位第 38 行表内单元格后 exit=1。
> 记此一笔，是因为「假红被误记成真缺陷」是最难归因的一类错误。

### 4.3 复用 `prepare`，不存在第二份作用域实现

DoD 写下的两条自查（照文本判）：

- `grep -n 'seal_digest\|with_check' verify.py` → 4 处命中：2 处是注释/文档串，2 处是
  **调用** `prepare.with_check(scope)`（第 116 行）与 `prepare.seal_digest(md)`（第 124 行）
- `grep -c 'def seal_digest\|def with_check' verify.py` → **0**

Leader 已自陈这两条**只按函数名查、换名重写就全过**。我补两个不依赖名字的判据：

1. **结构性判据**：`verify.py` 的 import 是 `argparse/os/re/sys` + `prepare`，**没有 `hashlib`**；
   全文 `hashlib` 命中 **0**、`sha256|md5|hexdigest|digest` 命中 **0**
   ⇒ **它在结构上不可能算出任何摘要**，换名重写也做不到。
2. **AST 穷举**（完全不依赖命名）：解析出全部函数定义共 **4** 个——`fail` / `scan_markers` /
   `verify` / `main`，无一是摘要实现；AST 里的 `prepare.*` 调用恰为 `seal_digest` 与 `with_check`。

⇒ **Leader 自陈的那处 DoD 缺口，在本次交付上没有被利用**；性质本身满足（详见 §5-F4）。

### 4.4 `--now` 缺省修复后 golden 逐字节未变（三路独立证据）

Leader 指示：把这一条当**待验断言实测**，不得从「测试都显式传 `--now`」推断。三路：

1. **git 层面**：两个 commit 的真实 numstat 变更文件中**没有 golden**
   （`3690015a`: prepare.py 6/1、test_prepare.py 17/1、test_verify.py 303/0、verify.py 147/0；
   `2d2fbe80`: test_prepare.py 1/1 —— 与 discovery `files_modified` 逐字一致）
   ⚠️ 中途 `grep -c 'golden'` 曾报 3，追查后**三行全在 commit message 正文**，非变更文件。
2. **sha256 现读** == TASK-006 记录值：`13d6e47b…`（2026-06-h1）、`638cb1a5…`（2023-08-monthly）
3. **重新生成比对**：用当前 `prepare.py` 显式传 `--now 2026-09-12` 重跑，两份均 `diff` **IDENTICAL**

另验缺省行为本身：不传 `--now` → `created: 2026-09-09` / `updated: 2026-09-09`（== 当天、非空）；
源码 `prepare.py:399` 为 `default=datetime.date.today().isoformat()`。Leader 的裁决已落实。

### 4.5 边界（6/6）

| 情形 | 期望 | 实得 |
|---|---|---|
| `<!-- check: zzz -->` | exit 1 且报**格式**错 | **1**，含「格式」 |
| hex 长度 63 位 | exit 1 且报格式错 | **1**，含「格式」 |
| 缺 `-->` | exit 1 且报格式错 | **1**，含「格式」 |
| **同段两条 check，且总数仍为 2** | exit 1 | **1**「段「前 12 期」出现了多于一条 check 行 —— 判为被篡改（不取第一个也不取最后一个）」 |
| 出现第二条 seal | exit 1 | **1**「封条 seal 行应为 1 条，实得 2 条」 |
| 批注区（`## 我的批注` 后）加一行 | exit 0 | **0** |

第 4 行是关键情形：它**绕过了「恰好 2 条」这道条数闸**（删一处、另一段插一条，总数不变），
仍被 `seen_sections` 抓住。DoD 明写「不取第一个也不取最后一个」——实现与报错文案均符合。

### 4.6 错误处理（3/3）

| 情形 | 期望 | 实得 |
|---|---|---|
| 文件不存在 | 非零 + stderr 含路径 + **不是** traceback | exit **2**，含 `no.md`，无 `Traceback` |
| 非笔记格式（无 frontmatter/无机器区） | 非零且报清楚原因，不 panic | exit **2**「不是笔记格式：缺少机器区标记 …」 |
| 空文件 | 非零 | exit **2**「空文件，没有可校验的内容: …」 |

退出码分层（0 通过 / 1 被篡改或格式损坏 / 2 输入不可用）与 DoD 相容：
functional[0] 的「否则 exit 1」射程是**校验不通过**，error_handling 只要求**非零**。

### 4.7 PR 与 worktree 收尾

`gh pr view 5` 现读：`state=OPEN`、`mergedAt=null`、`baseRefName=main`、
`headRefOid=2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`、**5 commits**。

**PR `OPEN` 是符合 DoD，不是缺陷**——non_functional[0] 原文明写「你只开 PR，不合并」
「不要因为 PR 未合并就卡住等」。

5 个 commit 覆盖三个任务（注意**两套编号体系**，见 §5-F2）：

| commit | 需求编号 | Arcforge 编号 |
|---|---|---|
| `aff25214` + `7ca6b57e` | M3 的 TASK-004（四份文档） | TASK-005 |
| `e8d0deca` | M3 的 TASK-005 前半（prepare + 夹具） | TASK-006 |
| `3690015a` + `2d2fbe80` | M3 的 TASK-005 后半（verify） | **TASK-007（本任务）** |

worktree 收尾：`git worktree list` 中 `wt-warp-hestia` 残留 **0**，且
`/Users/zuowei/workspace/ai/wt-warp-hestia` 目录已不存在；分支与提交仍在
（`rev-parse feat/warp-hestia` → `2d2fbe8094…`）。主 checkout 仍在
`feat/vendor-agent-reach-skill` @ `aefea6c`——**未切回 `main`，符合** DoD（那是人执行的一步）。

### 4.8 编号前缀订正（DoD 点名的第 130 行）

DoD non_functional[2] 点名 `test_prepare.py:130`。**逐版本追溯**：

| 版本 | 第 130 行原文 |
|---|---|
| `e8d0deca`（TASK-006 交出） | `（TASK-005 的 G6b 裁决）` ← **缺前缀**，正是 test-m3-a 报的 F3 |
| `3690015a`（本任务 commit ①） | `（M3 的 TASK-005 的 G6b 裁决）` ← **已修** |
| `2d2fbe80`（本任务 commit ②） | 保持 |

`git log -S` 确认前缀由 `3690015a` 引入。另一处（第 271 行，dev 本轮新写的 docstring）
由 `2d2fbe80` 修，是 dev 自查额外发现的，与 DoD 点名的不是同一处——**两处都已修**。

**顺带核实 DoD 里的数字**：DoD 称「4 处中 3 处合规」。实测 `e8d0deca` 时点 `scripts/` 下
`prepare.py` 2 处 + `test_prepare.py` 2 处 = **4 处**，缺前缀 1 处 ⇒ **3 处合规。DoD 数字准确。**

---

## 5. Findings（均**不构成缺陷**，不影响 VERIFIED 判定）

### F1【低】commit message 里的分层数字错（证据载体是对的）

nanoclaw commit `3690015a` 的 message 写：「84 个测试全绿（test_prepare **57** + test_verify **27**）」。
**真值是 55 + 29。**

- 两者**总数都等于 84** ⇒ 求和自洽校验**恒过**，这是典型的「再分配」型错误，总数守恒故不会被发现。
- **前任 test-m3-a 的线索经独立复算确认属实，两半都对**：
  - 交付文档 §2.2（行 102-104）写 `55 / 29 / 84` —— **正确**
  - discovery `verification.tests` 写「test_prepare.py 55 + test_verify.py 29 = 84」—— **正确**
- ⇒ **错只在 commit message**。DoD 要求的证据载体（交付文档 ②节 + discovery）均正确，
  且 commit message 不是任何一条 done_criteria 的指定载体 ⇒ **不判红**。

建议：无需返工。若 Leader 认为值得留痕，写进 final-report 即可（commit message 已入历史，不宜 amend）。

### F2【信息】两套任务编号体系并存，容易误读

nanoclaw 的 commit message 与代码注释一律用**需求编号**（`M3 的 TASK-004/005`），
而 DoD、discovery、本报告用 **Arcforge 编号**（TASK-005/006/007）。二者相差一位且**范围不对齐**
（Arcforge 的 TASK-006 与 TASK-007 合起来才是需求的 TASK-005）。

这是 DoD non_functional[0] 有意规定的（「用**需求编号**」，理由是注释的读者是三仓库开发者），
**不是缺陷**。但读 PR 历史时极易误判「TASK-007 的提交怎么写着 TASK-005」，故在 §4.7 给出映射表。

### F3【信息·建议报需求侧】frontmatter 不在任何校验作用域内

实测：改第 17 行 `tsf_stock_yoy: 7.40`（frontmatter 内的**数据字段**）→ `verify.py` **exit 0**。

成因是规格本身：seal 作用域自 `<!-- machine-generated: begin -->`（第 33 行）的**下一行**起，
frontmatter 在其之前；check 作用域自 `## ` 标题起，亦不含 frontmatter。

- **`verify.py` 忠实实现了 `note-format.md` 的规格，TASK-007 的 DoD 也逐字要求实现该规格
  ⇒ 不判红。**
- 该设计有其道理：`created`/`updated` 每次写回都变，纳入校验会使笔记无法二次保存。
- 但 frontmatter 里确实**混有数据字段**（如 `tsf_stock_yoy`），存在一个不被这道闸覆盖的面。
  是否需要加固（例如只把数据类字段纳入封条）属**规格层面**的取舍，超出本任务范围，报 Leader 转需求侧判断。

### F4【信息】Leader 自陈的 DoD 缺口：本次未被利用，且有更强判据可复用

Leader 已自陈 `functional[2]` 的自查「只按函数名查、换名重写就全过」是其 DoD 缺口。
我按指示**照 done_criteria 原文判 PASS**，并额外确认**性质本身也确实满足**（§4.3）：
`verify.py` 不 import `hashlib`、全文无任何摘要调用 ⇒ 结构上算不出 sha256；AST 穷举的 4 个函数
无一是摘要实现。

⇒ 本次**无需按 `dod_defect` 处理**（自查过了，性质也满足，两者不冲突）。
建议未来同类 DoD 把自查从「函数名 grep」换成**能力判据**（如「不得 import hashlib」）——
后者不依赖命名，换名重写绕不过。

---

## 6. 未做的事（明确列出）

| 未做 | 理由 |
|---|---|
| **独立复核 13 个变异 / 运行变异 harness** | **人类决定缩小范围**（§0），因前三任验证者在此阶段连续卡死。**这是严格度下降，已在 §0 显式标注。** |
| `gh pr merge` | DoD 明写只开 PR 不合并；PR 合并是人执行的前置（需求 TASK-006 Step 0，本 sprint 结转） |
| 把本机 checkout 切回 `main` | 同上，且会动到主 checkout 未提交的改动 |
| 运行时冒烟（容器是否加载 skill） | 依赖上面两步人执行动作，属需求 TASK-006 |
| 拆前三任残留的 worktree | Leader 指示留待阶段边界统一收（`wt-nc-t7-test-m3-b` / `wt-nc-t7-test-m3-c`） |

## 7. 本次验证自身的可复现锚

```
判定对象   nanoclaw feat/warp-hestia @ 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8
atlas      判定时 HEAD fd933f493d15bfe7dea6bdbd5c4c740d34084e9c
           verify_baseline.head cf7758bafa2f7f37f608497d9f1027537c100ddf（未刷新，四任共用）
           声明范围内零变更（内容 sha256 20cd0b87… 两端相同）
复现命令（锚钉全 sha，不用 HEAD / 分支名）:
  git -C /Users/zuowei/workspace/ai/nanoclaw worktree add --detach <tmpdir> 2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8
  cd <tmpdir>/container/skills/warp-hestia/scripts && python3 -m unittest -v
  → Ran 84 tests / OK / EXIT=0
我的验证 worktree: scratchpad/wt-nc-t7-test-m3-d（判定后由我拆除）
```

---

**验证者**：test-m3-d ｜ **判定**：**VERIFIED** ｜ 8/8 done_criteria PASS ｜ 4 findings 均非缺陷
**⚠️ 变异测试未经独立复核（§0），本次验证严格度低于原定标准。**

---

# 返工验证（review_fix 轮）— F3 封条与 `end` 之间的夹层

> 本节由 **test-m3-d** 于 2026-09-09 追加。**上方原始验证内容（我在前一轮撰写）一字未改**——
> 其 §0「验证严格度下降声明」被 `CONTRACTS.md` §B 引用，覆盖会使该引用失效，故追加而非重写。

- **验证者**：test-m3-d ｜ **assignment_epoch**：1 ｜ **rework_count**：1
- **判定对象**：nanoclaw `feat/warp-hestia` @ `2a6d39388e4648eb3be0f15b42ee28b52428c5d8`
- **atlas 锚**：`dc2fcefd2264d8391de1425e0e8f21ef0f0b30fc` == `verify_baseline.head`，discovery sha256
  `7d5181006f0f75797983c95b115783597e44b398147ccbf3f1bcc0783cd407bb` == baseline ⇒ **无漂移**

## 🔴 结论：**VERIFIED**（F3 缺陷已闭合）

## 1. 缺陷与修法

封条的作用域到 **seal 行之前**为止（`prepare.seal_digest` 的定义），而分段 check 的作用域各自止于
自己那条 check 行 ⇒ **seal 行与 `<!-- machine-generated: end -->` 之间的文本，既不被任何分段 check
覆盖、也不被封条覆盖**。攻击者可在机器区内插入一张伪造数据表而 `verify.py` 放行（修前实测 exit 0）。

> 这比我前一轮报的 frontmatter 盲区（§5-F3）**更危险**：读者看到的是一张**位于机器区内**的表，
> 比 frontmatter 更像「机器产的」。

修法（`verify.py:131`）是一条**形状**断言，**不触碰摘要算法**：

```python
if seal_i + 1 != end_i:
    return fail("seal 行之后仍有内容：seal 在第 %d 行，而 `%s` 在第 %d 行，中间夹着 %d 行 —— 判为被篡改。…")
```

因为不动摘要算法，**两份 golden 不受影响**——这一点我已独立验证（§4）。

## 2. 独立实测

**原样 golden（不误拒）**：seal 在第 76 行、`end` 在第 77 行，**紧邻** ⇒ `exit 0` ✅

**攻击样本**：在 seal 与 `end` 之间插入一张伪造的「本期数据」表（含 `999.99`）：

```
exit=1
seal 行之后仍有内容：seal 在第 76 行，而 `<!-- machine-generated: end -->` 在第 84 行，
中间夹着 7 行 —— 判为被篡改。
封条的作用域到 seal 行**之前**为止，这中间的文本不受任何校验保护，
故要求 seal 行必须紧邻机器区结束标记之上。
```

⇒ **`exit 1` ✓，报出 seal 76 行 / end 84 行 / 中间夹 7 行 ✓**，与派验指示所列 76 / 84 / 7 **逐字吻合**。
我另对自己构造的样本**独立计数**，同样得 seal 76 / end 84 / 夹 7 行 —— 报数自洽。

## 3. 消融实验 —— 证明缺陷确实被闭合

在 **`mktemp` 隔离副本**上把条件改为 `if False:`（语义消融）。有效性闸：`diff` 仅 1 行、
`ast.parse` 语法闸 OK、**主工作区 `verify.py` 改动 0 行**。

- 同一攻击样本 → **`exit 0`** ⇒ 缺陷复现，与「修前实测 verify exit 0」吻合。
- 跑 `test_verify` → **`FAILED (failures=4)`**：

| 变红的测试 | 夹层内容 |
|---|---|
| `test_any_line_between_seal_and_end_rejected` | 空行 |
| `test_any_line_between_seal_and_end_rejected` | 随便一行文字 |
| `test_any_line_between_seal_and_end_rejected` | `<!-- 无害注释 -->` |
| `test_content_between_seal_and_end_rejected` | 伪造数据表 |

🔴 **这组断言钉的是「形状」而不是「内容看起来坏不坏」**：连**空行**和**无害注释**都拒。
这比只测伪造表强得多——因为攻击者不会只用看起来可疑的内容，而**任何**夹层文本都在两不覆盖的盲区里。
断言强度与缺陷的实际形状对齐，是本次三条修复里设计最扎实的一条。

## 4. golden 未受影响（形状断言不触碰摘要算法）—— 三路独立证实

1. **sha256 现读**：`13d6e47bd775bd3d3b4bfcd60ffc40975d648b8e33dcebb5598ef80ac6f6bfc7`（2026-06-h1）、
   `638cb1a52f585d99d101c741af92a0a9324ef5ca04ceba2d9ab687a9811d1586`（2023-08-monthly）
   —— 与 dev 报值**逐字节相同**，亦与本文件上方原始验证轮记录的值相同。
2. **git 层面**：本次返工 golden 变更文件数 **0**。
3. **重新生成**：用当前 `prepare.py` 显式传 `--now 2026-09-12` 重跑 → 两份均 `IDENTICAL`。

## 5. 回归与范围

`python3 -m unittest -v` → **`Ran 95 tests in 2.918s` / `OK` / EXIT=0**（退出码单独取、不跨管道）。
两把独立的尺同值：静态 63 + 32 = 95；动态 `Ran 63` / `Ran 32` = 95。`test_verify` 由 29 → **32**（+3）。

atlas 侧 `124  0  docs/hestia-m3/TASK-007-warp-hestia-verify.md`（唯一文件、与 `writes` 一致、
**零删除行**、含「返工记录」节）。nanoclaw 侧 `verify.py` 为 `13/0`（纯追加）。

## 6. 与我前一轮所报 F3（frontmatter 盲区）的关系

前一轮我报的 frontmatter 盲区**仍然存在**，且**本次未修、也不应在本轮修**——它已被记入
`CONTRACTS.md` §D 第六条作为**规格层取舍交人拍板**（三段式：实测行为 / 为何不判红 / 待决问题）。

本次 F3 修的是**另一个**盲区（seal↔end 夹层）。两者形状相似（都是「不被任何闸覆盖的文本区」）
但性质不同：**seal↔end 夹层是实现层可判定的形状问题，已修**；frontmatter 是规格层作用域起点的
取舍，**须由人决定**。二者不可混为一谈，本轮判定不涉及后者。

---

**验证者**：test-m3-d ｜ **F3 判定：VERIFIED** ｜ 攻击复现、报数核对、消融均为独立实测

### 补充：本轮变异的实际范围（严格度留痕）

（应 Leader 2026-09-09 要求补记。**上方各节均未改动**。）

**我没有逐个复算 dev 转述的「10 个变异全部 KILLED、0 存活」。** 本条任务上我做的是**针对性消融**
——F3（形状断言恒假 → 夹层攻击复活 exit 0） ——它复现了该缺陷修前的观测值，与 `fix_items` 记录的修前实测**逐字吻合**。

- 消融与那 10 个变异回答的**不是同一个问题**：后者是 dev 为其全部改动设计的、选点带随机性；
  消融是把**这条修复的守卫**拆回缺陷态，直接回答「这条修复真的闭合了那个缺陷吗」。
- 消融的证据强度更高之处：它把**修前的观测**与**消融后的观测**对上了，而 KILLED 计数只说明
  「有断言变红」。
- 但**这不等于那 10 个变异被复核过**。若将来有人质疑这批断言的整体强度，
  **此处是本轮未经二次确认的环节。**

（人类在本轮开工时已决定不做独立复算变异这一项；本节记录的是**实际发生了什么**，不是异议。）

---

**补记者**：test-m3-d ｜ **判定仍为 VERIFIED，本节不改变任何判定**
