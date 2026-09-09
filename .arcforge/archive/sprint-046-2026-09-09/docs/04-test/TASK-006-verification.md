# TASK-006 验证报告（`warp-hestia` 的 `prepare.py` + 夹具 + `test_prepare.py`）

- **验证者**：`test-m3-a`　**被验 owner**：`dev-m3-c`　**判定日期**：2026-09-08
- **判定**：✅ **VERIFIED**
- `assignment_epoch = 1`　`rework_count = 0`

> 本报告每条结论都来自我自己的实跑。**变异一律在 `mktemp -d` 隔离副本上做**，主工作区 `prepare.py` 的 sha256 与
> `git status --porcelain` 指纹在每轮前后校验，**全程未变**。

## ⓪ 双仓库基线（`verify_baseline` 够不到 nanoclaw，这一条靠人工闸）

| 锚 | 基线记录 | 判定前现算 | 判定后现算 |
| --- | --- | --- | --- |
| atlas `head` | `b5873ad840019bbccf7c4d5a41f643db8408d4b3` | 相同 ✅ | 相同 ✅ |
| `discovery_sha256` | `af3e9ff40430fceb4a2862b5443e9d9bf421fba177cce2360633151b0dda3023` | 相同 ✅ | 相同 ✅ |
| **nanoclaw worktree HEAD** | 🔴 **机制不覆盖** | `e8d0decaab2a9bcfd29c952ebff11356f63dd3ba` | 相同 ✅ |

nanoclaw worktree `/Users/zuowei/workspace/ai/wt-warp-hestia`，分支 `feat/warp-hestia`，工作区判定前后均 0 行改动
（我跑测试产生的 `__pycache__/` 已自行清理，见 §③F2）。**三个锚判定前后各记一次，零漂移** ⇒ 转 `verified` 无需 ack。

## ① Done Criteria 覆盖矩阵

| # | 判据（摘要） | 证据 | 判定 |
| --- | --- | --- | --- |
| **functional[0]** | 夹具五期 + 修订契约 + `existing-*.md`；构建树核对 | §2.1 | **PASS** |
| **functional[1]** | 只用标准库；模块结构；`--print-name` 无月份 | §2.2 | **PASS** |
| **functional[2]** | frontmatter 14 基础 + 9 派生 + 4 信号，无 `reviewed`/`source`；判读提示非空 | §2.3 | **PASS** |
| **functional[3]** | 两张表的结构义务；**check 恰 2 + seal 恰 1** | §2.4 | **PASS** |
| **functional[4]** | 三期温度 2/0/1；**golden 逐字节**；确定性；派生分支全覆盖；条数 | §2.5 | **PASS** |
| **boundary** | B1/B2/B6/O3 + 批注保留 + **七个标记** | §2.6 | **PASS** |
| **error_handling** | 侧车缺失 exit **2** 且 stderr 含 `history`；契约解析失败非零 | §2.7 | **PASS** |
| **non_functional** | 提交规范 / 不开 PR / `warp-research` 不动 / 编号前缀 / 交付流程 / 锚全 sha | §2.8 | **PASS**（前缀 3/4，见 ③F3） |

**8/8 PASS。** 四条发现见 §③，**其中只有 F1 需要你裁决**，均不构成 rejected。

## ② 逐条证据

### 2.1 夹具与构建树（functional[0]）

单 commit `e8d0decaab2a9bcfd29c952ebff11356f63dd3ba`，**17 个文件全新增、0 删除**：六份契约 + 六份侧车
（含 `2025-12-annual-rev` 修订对）+ `existing-2026-06-h1.md` + `golden/` 两份 + `prepare.py` + `test_prepare.py`。
DoD 五期齐（`2020-06 h1` / `2025-12 annual` / `2026-06 h1` / `2023-08 monthly` / `2022-07 monthly`），
且 DoD 明写「**不写死总数**」——实际 17 个，与 `ls fixtures/` 一致。

**修订契约走的是 (b)**（文档 2.2 声明），我核了它确实是一份独立夹具而非二次 emit：
`is_revision = True`、`supersedes_published_at = 2026-01-15`。

> 🔴 **构建树核对我按 Leader 的更正用内容判据**，没有用 DoD 里那条 `grep -E 'TASK-001|TASK-002'`
> （任务 ID 跨 sprint 复用 ⇒ 该命令恒真，Leader 已确认是 DoD 写错）。

### 2.2 只用标准库与接口（functional[1]）

```
$ grep -n '^import\|^from ' prepare.py
14:import argparse   15:import hashlib   16:import json   17:import re   18:import sys
```

**五个全是标准库，零第三方。** 模块结构齐（`months_in_period` / `monthly_average` / `same_caliber_pair` /
`evaluate` / `note_name` / `build_frontmatter` / `render_table_current` / `render_table_history` /
`with_check` / `extract_annotations` / `main`，另有 `seal_digest` / `reading_hints` / `_pick` / `_series`）。

```
$ python3 prepare.py fixtures/2026-06-h1.json --print-name
2026 上半年金融数据解读.md          ← 无月份，符合 TASK-005 的 G6b 裁决
```

### 2.3 frontmatter（functional[2]）

我自己解析输出的 YAML 头逐键比对：

```
14 基础: 14/14 ✅      9 派生: 9/9 ✅      4 信号: 4/4 ✅
含 reviewed: False     含 source: False    （都应 False）
generated_by = warp-hestia@v1      contract_generated_by = contract@v1/replay
```

`<!-- narrative -->` 段内有 `reading_hints()` 生成的判读提示（八问顺序 + 四信号 + 温度 + 剪刀差 +
「温度分母是 `temp_known` 不是恒定 4」），**非空段**，满足 reviewer O4。

### 2.4 两张表与两级校验（functional[3]）

**本期表**——三列且含口径标注：

```
| 指标 | 单位 | 本期 2026-06（口径 2025-01） | 上期 2025-06 | 去年同期 2025-06 |
```

口径不同的夹具上标注**确实会打出来**（这正是变异 M14 的实质，见 §2.9）：

```
2023-08: | 指标 | 单位 | 本期 2023-08（口径 2023-01） | 上期 2023-07 | 去年同期 2022-08 ⚠️口径 2015-01 |
```

**前 12 期表**——数据行 **5 行 == 侧车 `same_type` 的 5 条**（`_series` 只读 `same_type`，
不查库、不补齐、不截断），表头六列。

**两级校验（在真实输出上数，不是在文档里数）**：

```
2026-06-h1     : check=2  seal=1  '## '=4
2023-08-monthly: check=2  seal=1  '## '=4
```

**seal 位置**：第 76 行，`<!-- machine-generated: end -->` 在第 77 行 —— **紧邻其上** ✅
（`begin` 33 / `narrative` 68 / `/narrative` 75 / `seal` 76 / `end` 77 / `## 我的批注` 79）。
`## ` 恰 4 个而 check 恰 2 条 —— 与 `note-format.md`「条数写死、不从标题数推」的契约一致。

### 2.5 测试（functional[4]）

```
$ python3 -m unittest -v
Ran 53 tests in 1.369s
OK                                    EXIT=0
```

**条数用四把独立的尺**（我第一把尺数出 32，是**我的**正则错了，见 ③F4）：

```
尺1 静态 grep -c 'def test_'      53
尺2 '... ok' 出现次数              53
尺3 unittest 自报 Ran              53
尺4 python 内省 TestLoader 递归计数  53
```

**三期 golden 温度**（我自己跑 `prepare.py`，不看它的测试）：

```
2020-06-h1   : temp_score: 2  temp_known: 4
2025-12-annual: temp_score: 0  temp_known: 4
2026-06-h1   : temp_score: 1  temp_known: 4
```

**golden 逐字节比对**——我重新生成两份输出与入库期望文件比：

```
2026-06-h1     : ✅ 与 golden 逐字节相同
2023-08-monthly: ✅ 与 golden 逐字节相同
```

> reviewer O2 要防的正是「`test_deterministic` 只做自比 ⇒ 乱序/错表头/少一列也全绿」。
> 入库期望文件 + 逐字节比对确实闭合了它，而且**我用的是自己的重跑**，不是它的断言。

### 2.6 边界（boundary）

**七个标记齐全**（reviewer U2 + B5 复审后的清单）：

```
<!-- machine-generated: begin -->  1     <!-- narrative -->   1     <!-- /narrative -->  1
<!-- machine-generated: end -->    1     ## 我的批注            1
<!-- check:                        2     <!-- seal:            1
```

**O3 修订路径**：`supersedes_published_at: 2026-01-15`（frontmatter 第 14 行）+ 一级标题下
`> 本文取代 2026-01-15 版本。`（第 34 行）✅

**批注区原样保留**（`--existing`）：

```
旧笔记：这是我手写的第一行批注 / 第二行批注
新笔记：这是我手写的第一行批注 / 第二行批注      ← 逐行相同
```

**B1**（`months_in_period` 越界 ⇒ 0 且不做除数）与 **B2**（`monthly_average` 的 `ok=False` ⇒ `unknown`、
`temp_known < 4`）由代码路径 + 变异 M5/M10/M6 双向证实（§2.9）；**B6**（无 `## 我的批注` 标题 ⇒ 视为空）
由 `extract_annotations` 的实现与变异 M9 证实。

### 2.7 错误路径（error_handling）

```
$ python3 prepare.py fixtures/2026-06-h1.json /nonexistent/x.history.json
exit=2
stderr: history 侧车读取或解析失败: [Errno 2] No such file or directory: '/nonexistent/x.history.json'
              ↑ 含 "history" ✅

$ python3 prepare.py <坏 JSON> fixtures/2026-06-h1.history.json
exit=2
stderr: contract 读取或解析失败: Expecting value: line 1 column 1 (char 0)
```

### 2.8 交付流程与锚点（non_functional）

```
atlas commit 331fe42a46d4e3cb188bac771d2efa117f00beac
  subject: docs(TASK-006): …          匹配 ^[a-z]+\(TASK-006\): → 1 ✅
  numstat: 442  0  docs/hestia-m3/TASK-006-warp-hestia-prepare.md   ← 单文件，与 writes 一致，零越界
  合入: 拓扑 IS-ANCESTOR + 内容 sha256 b8c6a6e54b1ba1a3e790e402772ae9530c295287b4162d9ec8b8c119d82da5b7
        （master 与工作树两处相同）—— 拓扑单独不可信，故补内容判据
nanoclaw: warp-research 累计改动 0 行；不开 PR（TASK-007 统一开）；worktree 未新建、未拆
```

**锚点纪律**：文档内 4 处 `HEAD` 全部是表格标签（紧跟全 sha）或写给我的操作说明，**无一处被当作可复现锚**；
6 个不同的 40 位全 sha，本任务 commit `e8d0deca…` 出现 6 次。

**三条「别据此判红」的前提，我独立核实前提本身成立**：
①`scripts/verify.py` **确实不存在**（`ls` 报 No such file）—— 它是 TASK-007 的产物；
②`fork/main..feat/warp-hestia` 只有 3 个 commit、无 PR；
③`seal_digest` / `with_check` 均已实现并被测试覆盖，TASK-007 可 `import prepare` 复用。

### 2.9 🔴 变异复核 —— 我不审它的流程，直接在**交付物**上重跑

Leader 的关切是：code-simplifier 改了代码形状后，原变异锚点会失效，而 harness 对 `count == 0` 是**静默跳过**
（打「跳过」而不是报错）—— 属「变异根本没生效却给出符合预期结果」这一族。

**我的做法**：不去审它的 harness，而是**在当前交付的 `prepare.py` 上跑我自己写的变异**，看红数能否复现。
我的 harness 对锚点命中数 **≠ 1 就大声报错并拒绝记任何结果**（实测触发过一次 —— golden 测试真名是
`test_golden_files_byte_exact`，我猜的名字命中 0 次，harness 立刻停下）。

| 我的变异（对应文档编号） | 文档报的红数 | 我实测 |
| --- | --- | --- |
| `_pick` 改 `_ytd` 优先（M17） | 红 1 | **KILLED 红 1** |
| 去掉口径标注（M14） | 红 2 | **KILLED 红 2** |
| 活化 `>=` 改 `>`（M21） | 红 1 | **KILLED 红 1** |
| `bill_ratio` 允许跨口径（M4） | 红 1 | **KILLED 红 1** |
| `temp_known` 恒 4（M6，对照） | 红 1 | **KILLED 红 1** |
| 去掉 seal 行（M15，对照） | 红 6 | **KILLED 红 6** |

**六个变异、六个红数逐个相同**；对照（未变异）绿；主工作区指纹前后未变。

⇒ **文档 2.10 那张表确产于当前这版代码。** 若它来自简化之前那一轮、锚点已失效，红数不可能六个全中 ——
这是**观察**，不是采信它的自述。

### 2.10 🔴 区分性变异 —— 「那 4 个先存活」的判据是否成立

文档点名的 8 条新增断言我逐个确认**真实存在**（各 1 处）。把它们停掉再跑同样的变异：

| 变异 | A 全部断言 | B 停掉新增断言 | C 再停掉 golden |
| --- | --- | --- | --- |
| **M17** `_pick` | KILLED(红1) | 🔴 **SURVIVED** | SURVIVED |
| **M14** 口径标注 | KILLED(红2) | **KILLED(红1)** | 🔴 SURVIVED |
| **M21** 活化边界 | KILLED(红1) | 🔴 **SURVIVED** | SURVIVED |
| **M4** 跨口径 | KILLED(红1) | 🔴 **SURVIVED** | SURVIVED |

- **M17 / M21 / M4**：停掉新增断言即存活 ⇒ 那几条断言是**唯一的闸**，连 golden 都抓不到它们。
- **M14**：B 列**仍红**（golden 杀的），C 列才存活 —— 这与文档自述**逐字吻合**：
  「只被 golden 逐字节比对杀死，专职的 `test_current_table_has_caliber_annotation` **没响**」。
  它的缺口是**专职断言空洞**（断言「出现了口径字样」，而表头恒含「本期 …（口径 X）」），不是零覆盖。

⇒ **dev 的判据「一条语义若在所有夹具上取值都一样，它就没有被测到，无论有多少测试引用了它」经实测成立**，
且它对 M14 那个更微妙的形态（有专职断言但恒真）也判对了。这四条是本任务最有价值的部分。

## ③ 发现（4 条）

### F1 🟡【中｜**需 Leader 裁决**】实产笔记的 `created` / `updated` 会是空的

**跨 TASK-005/006 的产物间不一致，两侧各自都说得通，合起来漏了**：

```
prepare.py  : ap.add_argument("--now", default="")          ← 缺省空串
SKILL.md:62 : python3 …/prepare.py $Q/processing/$F $Q/processing/$H ${EXISTING:+--existing "$EXISTING"} > /tmp/note.md
                                                             ← 🔴 不传 --now
```

照 Step 3 **原样**跑一次（我跑了）：

```
---
type: summary
domain: macro
created:                  ← 空
updated:                  ← 空
tags: [macro/pboc]
```

而 `note-format.md` 的 frontmatter 字段表把 `created`/`updated` 列为「vault `CLAUDE.md` 要求的**六个字段**」。

**为什么没被任何测试抓到**：`test_prepare.py` 里唯一的调用 helper（第 41 行）**总是**传 `--now 2026-09-12`；
两份 golden 因此都是 `created: 2026-09-12`；第 243 行的 `BASE` 列表只断言键**存在**、不断言非空。
⇒ **`--now` 缺省这条路径零覆盖**，而它恰是生产路径。文档与 discovery 均未提及（`--now` 命中 0）。

**⚠️ 这不构成本任务 DoD 违反** —— DoD functional[2] 的原文是「必须逐个断言**存在**」，14 个键确实都在。
`--now` 缺省为空也是**有意的确定性选择**（否则 golden 会随日期变动）。缺的是**两个产物之间的接线**。

**修法二选一**（都很小）：①`prepare.py` 在 `--now` 缺省时取当天（测试仍显式传值，golden 不受影响）；
②`SKILL.md` Step 3 改成 `--now $(date +%F)`。**②要动已 verified 的 TASK-005 产物 ⇒ 归你裁决**，
我倾向 ① —— 它把默认行为修正在唯一的实现点上，不需要每个调用方记得传。

### F2 🟢【低｜已由 dev 自报，仅确认】`__pycache__` 未进 `.gitignore`

我跑测试后工作区多出 `container/skills/warp-hestia/scripts/__pycache__/`（`check-ignore` 退出码 **1** = 未被忽略）。
**dev 已在文档 ④E 主动写明**，并说明不改的理由（nanoclaw 仓库级配置，超出本任务 `writes`），
且最终提交是 **17 个文件、0 个 `.pyc`**（我核过 numstat）。我已清理自己产生的那份，工作区复归 0 行。**非缺陷。**

### F3 🟢【低】任务编号前缀 3/4

```
prepare.py:2        「…M3 的 TASK-005。」          ✅
prepare.py:390      「…（M3 的 TASK-005）」        ✅
test_prepare.py:2   「…M3 的 TASK-005。」          ✅
test_prepare.py:130 「…（TASK-005 的 G6b 裁决）」  ❌ 缺 `M3 的`
```

DoD 把它写成「义务」。但**为 4 个字符走一轮 `review_fix` + 复验不划算**，且该处上下文（G6b 是需求文档的
reviewer 编号）不存在歧义。建议并入 TASK-007（同目录，边际成本零）。**不判红。**

### F4 🟢【我自己的仪器故障，记此供后来者】

我第一把数测试条数的尺是 `grep -cE '^test_.* \.\.\. ok$'`，数出 **32**（真值 53）——
`unittest -v` 在测试带 docstring 时，`... ok` 会跟在 docstring 那一行后面，**不在行首**。
我没有据这个 32 去报「条数对不上」，而是换了三把尺复算（全 53）。

⇒ 与本 sprint 反复出现的是同一族：**仪器给出的数「像样」，而它测的性质不是我要的那个**。
判据仍是那句 —— 两把独立的尺不同值时，**先怀疑尺**。

## ④ 结论

**✅ VERIFIED。** 8 条 `done_criteria` 逐条 PASS；双仓库三个锚**判定前后各记一次、零漂移**。

最有力的两条：

1. **文档 2.10 的变异表确产于当前这版代码** —— 我在交付物上跑自己的 6 个变异，**红数六个全中**，
   对照绿、主工作区指纹未变。这直接回答了「简化后锚点失效、表来自旧轮」的疑虑，而且给的是观察不是采信。
2. **那 4 个「先存活、补断言才杀死」的判据成立** —— 区分性变异显示 M17/M21/M4 的新增断言是**唯一的闸**，
   M14 则精确落在文档自述的那个更微妙形态（专职断言恒真、仅 golden 兜住）。

**唯一需要你裁决的是 F1**：`--now` 缺省 + SKILL.md Step 3 不传 ⇒ 实产笔记 `created`/`updated` 为空，
零测试覆盖，跨 TASK-005/006 两个产物。**不影响本任务判定**，但会在冒烟时被 vault 的必需字段规则挡下。

---

# 返工验证（review_fix 轮）— F2 契约×侧车配对校验

> 本节由 **test-m3-d** 于 2026-09-09 追加。**上方原始验证内容（test-m3-a 撰写）一字未改**——
> 该文件的 §2.5（三期温度 `2 / 0 / 1`）是 `CONTRACTS.md` §B 明文引用的**来源载体**
> （见 `internal/hestia/CONTRACTS.md` 对本文件的引用），覆盖会使该引用失效，故追加而非重写。
> 追加在文件末尾 ⇒ 原有行号不变，引用继续成立。

- **验证者**：test-m3-d ｜ **assignment_epoch**：1 ｜ **rework_count**：1
- **判定对象**：nanoclaw `feat/warp-hestia` @ `2a6d39388e4648eb3be0f15b42ee28b52428c5d8`
- **atlas 锚**：`dc2fcefd2264d8391de1425e0e8f21ef0f0b30fc` == `verify_baseline.head`，discovery sha256
  `2defb96d30adfc875768099c5457e0f76fdf28203be5abdf333c6247e9711c7a` == baseline ⇒ **无漂移**

## 🔴 结论：**VERIFIED**（F2 缺陷已闭合）

## 1. 缺陷与修法

`prepare.py` 原先只从侧车读 `same_type`，**侧车顶层的 `for` 字段被读 0 次** ⇒ 错配的契约×侧车喂进去，
`prepare` 与 `verify` **双双 exit 0**，产出 3161 字节的笔记：frontmatter / 标题 / 四信号 / 温度全对
（都来自契约）、**两张表全错**（都来自侧车）——**笔记自洽地看起来完全正常**。

修法：新增 `assert_pair(contract, history)`（`prepare.py:223`，调用点 `:457`），判据

```python
pair_key(contract) == history["for"]        # period + "-" + period_type
```

`for` 是这条链路上**唯一能机器判定「这两个文件是一对」的事实**。

## 2. 🔴 判据是**期次**不是文件名（派验点名第 3 条）

修订夹具文件名叫 `2025-12-annual-rev.*`，而它的 `for` 是 `2025-12-annual`。我实测确认：

```
2025-12-annual-rev.history.json 的 for            = 2025-12-annual
2025-12-annual-rev.json 的 period + period_type   = 2025-12-annual
⇒ 正配 exit 0
```

⇒ **若拿文件名做判据，这一对会被误拒。** 实现用的是期次，正确。

**被三条测试钉住**（`test_prepare.py`）：

| 测试 | 断言 |
|---|---|
| `test_revision_fixture_pairs_by_period_not_filename` | **先断言夹具的 `for == "2025-12-annual"` 钉住前提**，再断言 rc==0 |
| `test_mismatched_pair_exits_2` | rc==2、stderr 含契约侧期望值、stderr 含侧车实际 `for`、**stdout=="" 不得产出半份笔记**（四条断言） |
| `test_sidecar_for_field_is_actually_read` | `hasattr(P, "assert_pair")` —— 见 §5 finding |

## 3. 独立实测

**正配六份（五期 + 修订夹具）全部 exit 0：**

| 夹具 | exit | 字节 |
|---|---|---|
| 2020-06-h1 | 0 | 2899 |
| 2022-07-monthly | 0 | 3458 |
| 2023-08-monthly | 0 | 3505 |
| 2025-12-annual | 0 | 3322 |
| **2025-12-annual-rev** | 0 | 3395 |
| 2026-06-h1 | 0 | 3247 |

⇒ **6/6**，与 dev 报「六份全 0」一致。（`2026-06-h1` 的 3247 与 TASK-005 F1 的 create 场景字节数
自洽——同一夹具同一 `--now`。）

**错配 → 拒绝：**

```
$ prepare.py fixtures/2026-06-h1.json fixtures/2025-12-annual.history.json --now 2026-09-12
exit=2   stdout=0 字节
契约与侧车不是一对：契约是 2026-06-h1，而侧车的 for 是 '2025-12-annual'。
两张表全部取自侧车，错配会产出「自洽但表全错」的笔记（frontmatter 与信号来自契约、两张表来自侧车），故拒绝。
```

⇒ exit 2 ✓ / 0 字节 ✓ / **打印了两边的值** ✓ 三项均达标。

## 4. 消融实验 —— 证明缺陷确实被闭合

在 **`mktemp` 隔离副本**上把 `assert_pair` 改为恒返回 `(True, "")`（语义消融，模拟「校验不存在」）。
有效性闸：`diff` 仅 1 行新增、`ast.parse` 语法闸 OK、**主工作区 `prepare.py` 改动 0 行**。

> ⚠️ 首次消融我直接删掉调用整行，导致 `ok` 未定义 —— 那是**崩溃型变异**（41 errors），
> 会被误记成「杀死」。已废弃并改用语义消融。此处记一笔。

结果：

- 错配**复活为 `exit 0` / 3161 字节** ⇒ 与修前实测的 3161 **逐字节吻合**，缺陷确认复现。
- 跑 `test_prepare`：**`FAILED (failures=1)`，只有 `test_mismatched_pair_exits_2` 变红。**

## 5. Finding【低，非缺陷】`test_sidecar_for_field_is_actually_read` 断言强度低于其承诺

其 docstring 称「钉住『`for` 被读到了』这件事本身——缺陷正是它命中 0 次」，但实际断言只有
`hasattr(P, "assert_pair")`。**函数存在 ≠ 它被 `main` 调用 ≠ `for` 被读**：§4 的消融保留了函数、
只让它恒真，该条**仍绿**。

- **缺陷不会溜过去**——`test_mismatched_pair_exits_2` 会红（消融实测已证）。
- ⇒ 「**有效但不是唯一的闸**」，与本 sprint `CONTRACTS.md` §B 记录的 O3 属同一形态，**判非缺陷**。
- 建议：断言改为行为判据（如「错配时 stderr 含侧车 `for` 的实际值」），而非符号存在性。

## 6. 回归与范围

`Ran 95 tests / OK / EXIT=0`；两把独立的尺 63 + 32 = 95。
**两份 golden 三路独立证实未变**：sha256 现读 `13d6e47b…` / `638cb1a5…` == dev 报值；
git 层面 golden 变更文件数 **0**；用当前 `prepare.py` 重新生成 → 两份均 `IDENTICAL`。
atlas 侧 `114  0  docs/hestia-m3/TASK-006-warp-hestia-prepare.md`（唯一文件、零删除、含「返工记录」节）。

---

**验证者**：test-m3-d ｜ **F2 判定：VERIFIED** ｜ 正配/错配/期次判据/消融四项均为独立实测

### 补充：本轮变异的实际范围（严格度留痕）

（应 Leader 2026-09-09 要求补记。**上方各节均未改动**。）

**我没有逐个复算 dev 转述的「10 个变异全部 KILLED、0 存活」。** 本条任务上我做的是**针对性消融**
——F2（`assert_pair` 恒真 → 错配复活 exit 0 / 3161 字节） ——它复现了该缺陷修前的观测值，与 `fix_items` 记录的修前实测**逐字吻合**。

- 消融与那 10 个变异回答的**不是同一个问题**：后者是 dev 为其全部改动设计的、选点带随机性；
  消融是把**这条修复的守卫**拆回缺陷态，直接回答「这条修复真的闭合了那个缺陷吗」。
- 消融的证据强度更高之处：它把**修前的观测**与**消融后的观测**对上了，而 KILLED 计数只说明
  「有断言变红」。
- 但**这不等于那 10 个变异被复核过**。若将来有人质疑这批断言的整体强度，
  **此处是本轮未经二次确认的环节。**

（人类在本轮开工时已决定不做独立复算变异这一项；本节记录的是**实际发生了什么**，不是异议。）

---

**补记者**：test-m3-d ｜ **判定仍为 VERIFIED，本节不改变任何判定**
