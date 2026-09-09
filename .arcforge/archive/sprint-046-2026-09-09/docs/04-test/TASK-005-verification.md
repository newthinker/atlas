# TASK-005 验证报告（Sprint M3 · `warp-hestia` skill 文档）

- **验证者**：`test-m3-a`　**被验 owner**：`dev-m3-c`　**判定日期**：2026-09-08
- **判定**：✅ **VERIFIED**（带显式漂移确认，见 §4）
- **assignment_epoch**：1
- **verify_baseline**：`head = 1991c49ec5541fd4d4c0d0cd23117b389e7d00d1`，`discovery_sha256 = 10dd7fde82889077dd7f0c882984a6a45218833372262f2ddb58f08ac6083960`

> **本报告的证据全部由验证者自己实跑产出**。跨仓库任务（AD-M3-1）：代码在 nanoclaw，atlas 门禁只验那份 `.md`，nanoclaw 侧无机制兜底。
> 🔴 **我把每一条判据在两个版本上各跑了一遍**——`aff2521…`（discovery / 交付文档 / Leader 派验消息三方锚定的对象）与 `7ca6b57…`（分支现状）。理由见 §4：判定期间发生了漂移。

---

## 一、Done Criteria 覆盖矩阵

DoD 七条：四条 `functional`（`review`）、一条 `boundary`（`review`）、一条 `error_handling`（`manual`）、两条 `non_functional`（`review`/`manual`）。无 `verify_by: test` 条目 ⇒ 不要求测试断言覆盖，判据是可复现的实地证据。

| # | 完成标准（摘要） | 证据 | `aff2521` | `7ca6b57` |
| --- | --- | --- | --- | --- |
| **functional[0]** | `SKILL.md`：frontmatter 三触发语 + 两条边界声明；§1 三条 I/O；§2 六步；§3 **四条**；模型边界明写 | §2.1 | **PASS** | **PASS** |
| **functional[1]** | `note-format.md`：五种命名 + `--print-name`；frontmatter 表；骨架；check 定义；`mode`；修订；文件名冲突裁决；**两级校验条数写死** | §2.2 | **PASS** | **PASS** |
| **functional[2]** | `glossary.md` 五块 + 跨口径禁忌 + 口径按段 + 月均三条 + 四信号表 + 单位 | §2.3 | **PASS** | **PASS** |
| **functional[3]** | `methodology.md` 提炼非复制：①≤源 40% ②`## ` 交集 ≤1（**`grep -Fxf`，禁 `comm -12`**）③13 名称可 grep | §2.4 | **PASS** | **PASS** |
| **boundary[0]** | Step 2 两边界 + Step 3/5/6 三条失败分支，Step 5 **留 `processing/` 不移** | §2.5 | **PASS** | **PASS** |
| **error_handling[0]** | 只用容器内可用的东西；无联网 / 第三方包 / 写 vault | §2.6 | **PASS** | **PASS** |
| **non_functional[0]+[1]** | nanoclaw 规范提交、不开 PR、`warp-research` 不动、编号带 `M3 的 ` 前缀、atlas 交付流程、**锚全 sha** | §2.7 | **PASS** | **PASS** |

**结论：7/7 PASS，两个版本上都 PASS，无未覆盖标准。** 另有 4 条发现见 §3（其中 1 条是我自己的仪器故障）。

---

## 二、逐条证据

### 2.1 functional[0] —— `SKILL.md`

**§3 恰四条，用三把独立的尺各算一遍**（DoD 明写「初稿写五条是错的」，装反会让 dev 硬造第五条）：

```
awk 段内计数        4
sed 切段后 grep     4
行号区间后 grep -E  4
```

且与**需求原文 line 720-724 逐字节比对**：

```
$ diff -u <需求 ## §3 不做 段> <SKILL.md 同段>
@@ -1,4 +1,5 @@
 ## §3 不做
+
 - 不处理第二份（用户再说一句才继续）
 - 不改数据表、不改 frontmatter、不自己算指标
 - 不写 `reviewed` / `source`（Spool 会覆写）
 - 不上网查资料——本 skill 只解读契约与侧车里的数据
```

**四条 bullet 一字未改**，唯一差异是标题后多一个空行（Markdown 惯例，非内容改动）。

**其余结构**：

```
Step 小节 = 6      （35/43/57/70/75/93 行，Step 1…6 齐）
§1 编号条 = 3      （队列可写四子目录+成对移动 / vault 只读绝不直接写 / 唯一写出口 selvage_call 且 DENIED|ERROR 原文回复不重试）
模型边界   = 5 条   （不改数据表 / 不改 frontmatter / 不自己算指标 / 不写 reviewed|source / 不上网）
```

模型边界五条与 DoD 红字要求的清单**逐条对应**，且提成了 §1 下的独立小节（DoD 要求「必须明写」）。

**frontmatter 六个必含字串 6/6**，且我另加一层核实：**三个触发语与两条边界声明确实落在 1–7 行的 YAML frontmatter 内**（不是散在正文里）：

```
name: warp-hestia        1      处理 hestia 队列        1（在 1-7 行内）
生成金融数据解读             1      解读这期央行数据          1
一次只处理一份              1（在 1-7 行内）  不用于回答一般宏观问题   1（在 1-7 行内）
```

### 2.2 functional[1] —— `references/note-format.md`

**骨架计数，两把独立的尺**（awk 围栏状态机 / 直接行号区间），结果一致：

```
骨架内 check 行 = 2   (须 2)
骨架内 seal  行 = 1   (须 1)
骨架内 ## 标题  = 4   （本期数据 / 前 12 期 / 信号 / 我的批注）
机器区内 ## 标题 = 3   （begin→end 之间）
全文件对照尺: check 3 / seal 2  ≥ 骨架内计数，方向自洽
```

🔴 **骨架与 spec §6.2 逐行 diff——只多一行，零越权偏离**：

```
$ diff -u <spec §6.2 骨架> <note-format 骨架>
@@ -15,6 +15,7 @@
 <!-- narrative -->
 （模型叙述，按 methodology 的八问框架）
 <!-- /narrative -->
+<!-- seal: <sha256> -->
 <!-- machine-generated: end -->
```

那一行正是 DoD 的 reviewer B5 重新设计要求新增的封条，位置（`machine-generated: end` 紧邻其上）也与 DoD 逐字一致。**spec §6.2 骨架本身 check=2 / seal=0 / `## `=4**，与本交付的机器区计数完全吻合。

**文件名冲突裁决——五处出处我逐条核实属实**（这是 DoD 点名「否则会在冒烟里再咬一次」的那条）：

```
需求 line  853  self.assertEqual(out, "2026 上半年金融数据解读.md")            ← 无月份
需求 line  967  N="$V/Wiki/Macro/PBOC/2026 上半年金融数据解读.md"              ← 无月份
需求 line 1012  `Wiki/Macro/PBOC/2026 上半年金融数据解读.md`                   ← 无月份
spec line  197  …h1 / annual 用 `2026-06 上半年…` / `2025 全年…`             ← 带月份
spec line  273  `Wiki/Macro/PBOC/2026-06 上半年金融数据解读.md` 出现            ← 带月份
```

裁决（取无月份版）**写进了 `note-format.md` 文内**（第 23–32 行的「🔴 与上游 spec 的冲突及裁决」小节），不是只记在交付文档里——DoD 要求的正是这个载体。
另核：discovery 自陈订正过「spec line 273 的『上半年』引成两次」，实测该行 `上半年` 恰 **1** 次，**订正是对的**。

**其余六项内容全部在位**：五种 `period_type` 命名（`monthly` 唯一带月份）、`prepare.py --print-name`（2 处）、frontmatter 字段表（`reviewed`/`source` 标「Spool 强制写入」）、正文骨架、`mode` 规则（同名 ⇒ `update`，否则 `create`）、修订（`is_revision` ⇒ `本文取代 …`）。两级校验的**作用域定义与条数**都写清了，且明写「不要按 `## ` 标题数去推 check 行数」。

### 2.3 functional[2] —— `references/glossary.md`

`## ` 小节 = **5**，与 DoD 五块一一对应（`caliber_version` / `_ytd` 与 `_mom` / 月均取法 / 四信号判定表 / 单位）。15 个关键内容的 `grep -cF` 计数与交付文档 ②节 2.4 的表**逐个相同**。

🔴 **三条易错点我抽验到了权威实现** atlas `internal/hestia/signals.go`（glossary 自称与它同源）——这不是计数，是语义核对：

| glossary 的断言 | signals.go 实测 | 判定 |
| --- | --- | --- |
| 楼市 / 消费两信号**没有黄灯** | `Housing`/`Consumption` 都走 `evalThreshold`（`:61-70`），只返 `Green` / `Red` / `Unknown`，**无 Yellow 分支** | ✅ |
| 信贷分子分母**必须同口径**，`_mom` 优先；企业贷款合计 0 ⇒ `unknown` | `sameCaliberPair`（`:91-103`）先试 `aMoM/bMoM`、再试 `aYTD/bYTD`，都不成 ⇒ `false` ⇒ unknown | ✅ |
| 温度分母是 `temp_known` **不是 4**，有 `unknown` 时减一 | `Evaluate` 遍历四信号，`if s == SignalUnknown { continue }` 后才 `t.Known++` | ✅ |
| 月数解析为 0 ⇒ **视为缺失不除**（否则 ÷0 得 ±Inf 被当成很大的月均误判绿灯） | `monthlyAverage`（`:112-125`）`if n == 0 { return 0, false }`；源码注释与 glossary 的理由**几乎逐字相同** | ✅ |
| `corp_mlt_short_expand` 在快照里但**不参与**四信号 | `Evaluate` 只用 Activation/Housing/Consumption/Credit 四项 | ✅ |

`unknown` 四态、跨口径禁忌、`2023-08` 口径按段（5 期）、单位三分（万亿/亿元/百分数）均在位。

### 2.4 functional[3] —— `references/methodology.md` 的三条可跑判据

**判据①（≤ 源 40%）**：

```
源文件 /Users/zuowei/Obsidian/ClawdVault/Projects/Hestia/PBOC2026年上半年金融数据解读-完整版.md = 41113 字节
methodology.md = 12331 字节 → 30.0%，上限 16445 ⇒ PASS，余量 4114
```

**判据②（`## ` 交集 ≤ 1，只比 `## ` 不比 `### `）**：源 **12** 个、methodology **5** 个，交集 **0**。

```
grep -Fxf（不排序不去重）      0   ✅
python set 交集                0   ✅
LC_ALL=C 全链 comm -12         0   ✅
```

🔴 **我复现了 DoD 点名的 `comm` 陷阱，并逐行验证它确实是假的**：

```
$ comm -12 <(sort src-h2.txt) <(sort md-h2.txt)      ← 与交付文档 2.3 的演示逐行相同
## 九、常见误读与陷阱                在 methodology.md 中出现 0 次
## 一、这节课讲什么，目标是什么？          在 methodology.md 中出现 0 次
## 五、用数据回答八个社会现实问题          在 methodology.md 中出现 0 次
## 六、指标之间的关联性：五条传导链         在 methodology.md 中出现 0 次
## 七、怎么用在自己的生活、工作和投资上       在 methodology.md 中出现 0 次
计数 = 5   ← ❌ 假阳（判据上限是 1，用它会 reject 一份合格交付）
```

**这 5 行在 methodology.md 里各出现 0 次**（我逐条 `grep -cF` 过），交付文档「一行都不在里面」的说法属实。
另测：`comm` 在三种 locale 管线下给出**三个不同的值**——全默认 **5** / `LC_ALL=C sort` + 默认 `comm` **2** / 全 `LC_ALL=C` **0**。这比「同一坏仪器给 5 与 3 两个假值」更强地说明它不可用。

**判据③**：八问 8 个 + 五链 5 个 = **13/13 命中，未命中 0**（逐条 `grep -cF`，定长匹配无正则）。

### 2.5 boundary[0] —— 四条失败分支与两个边界

「失败分支一览」表体 **4 行**，四条的**队列处置各不相同**且都与 DoD 逐字一致：

| 步 | 触发 | 队列处置 | DoD 要求 |
| --- | --- | --- | --- |
| Step 2 | history 侧车缺失 | 契约**单独**移 `failed/` + 提示 `contract emit --period` | ✅ |
| Step 3 | `prepare.py` 退出码非零 | 契约与侧车**对移 `failed/`**，回 stderr **原文** | ✅（reviewer O8 补的） |
| Step 5 | `verify.py` 退出码非零 | 🔴 **留在 `processing/`，不移** | ✅（Leader 裁决项） |
| Step 6 | `DENIED` / `ERROR` | 契约与侧车**对移 `failed/`**，回原文**不重试** | ✅ |

Step 2 的第二个边界「同名已在 `processing/` ⇒ 直接用那对，**不重新占位**」在正文第 55 行。
Step 5 的 Leader 裁决在**正文（第 82-84 行）与表里各一处**，双写，且给了理由（契约本身没毛病，移 `failed/` 会让好契约要人工捞回；留 `processing/` 则下次会话按 Step 2 自然重试）。

### 2.6 error_handling[0] —— 禁用依赖

```
$ grep -rn 'pip install\|import requests\|import yaml\|curl \|wget ' container/skills/warp-hestia/ > /dev/null
$ echo $?
1                        ← 退出码不跨管道取，1 = 0 命中 = 通过
```

**我另加严了一轮**（DoD 没要求，防「换个写法就绕过」）：`pip3? install` / `import (requests|yaml|pandas|numpy|bs4)` / `apt-get` / `npm i ` / `brew install` ⇒ 同样 **0 命中，退出码 1**；`https?://` **0 处**。

**正向核实**（比反向禁令更强）——把四份文档里出现的所有命令类 token 数出来：

```
mv 4   python3 2   rg 1   ls 1   grep 1   cat 1
```

**恰好就是 DoD 允许的集合**（`python3` 标准库、`ls`/`grep`/`mv`/`cat`/`rg`、`selvage_call`），无一超出。

### 2.7 non_functional —— 提交、范围、编号、锚点

```
编号引用: 裸 TASK-NNN = 5，带 `M3 的 ` 前缀 = 5   ⇒ 5/5，无裸编号
分布: 4×TASK-004 + 1×TASK-005（用需求编号，与 atlas commit 的 Arcforge 编号不同——预期）
warp-research: 两 commit 累计改动 0 个文件
nanoclaw 提交: feat(warp-hestia): …（nanoclaw 规范，非 docs(TASK-005):）  分支 commit 数 = 2
PR: 无（DoD 明写不开）
atlas: subject `docs(TASK-005):` 匹配 1；numstat 415/0 单文件，与 writes 声明一致 ⇒ 零越界
合入 master: 拓扑 IS-ANCESTOR + 内容 sha256 三处（master / 分支 / 工作树）逐字节相同
worktree: /Users/zuowei/workspace/ai/wt-warp-hestia 干净（status 0 行）、未新建、未拆
主 checkout: feat/vendor-agent-reach-skill @ aefea6ce…，2 改 2 删 1 未跟踪，与 TASK-004 交付时相同
```

**修订后交付文档①节的每个数字我都复算过，无一不符**：

```
commit② numstat        3/1 (SKILL.md) + 4/0 (note-format.md)     ✅
累计 d791101…→7ca6b57…  114/110/161/129 = 514 新增 0 删除            ✅
当前字节                5667/5965/12331/6727 = 30690                ✅
```

**锚点纪律**：文档内 6 处 `HEAD` 无一处被当锚使用（1 处是命令原文、5 处是「…HEAD 全 sha」的表格标签或说明，紧跟全 sha）；40 位全 sha 出现 6 个不同值共 20 次。**③节现在把 commit ① / ② 分列，并注明每个 `file:line` 锚在哪棵树**。

---

## 三、发现（4 条）

### F1【中】discovery 内部两句互相矛盾的「这批数字测自哪棵树」

`verification.sampled_on` 仍写：

> 全部数字采于最后一次改动之后：nanoclaw commit `aff2521…`、atlas commit `c73dbe3b…`

但最后一次改动已是 nanoclaw `7ca6b57…` / atlas `41ae450f…`。同一个 `verification` 对象里的兄弟键 `resampled_after_fix` 又给了第二轮的数字（514 / 30690 …）。⇒ **同一份 discovery 里存在两句互相矛盾的采样声明**，而 `files_modified` 也停在第一轮（508 行、5259 / 6219 字节）。

交付文档在这一点上做得**明显更好**：它不但更新了口径，还专门标出唯一的例外（2.7 的 code-simplifier 表刻意保留第一轮字节，因为那张表要回答的问题只有那个时点的值能回答）。**同一位作者、同一轮修订，两个载体一个做到了一个没做到。**

> 建议（走 `review_fix` 时一并）：`sampled_on` 改成两轮并列或直接指向 `resampled_after_fix`；`files_modified` 补第二轮。

### F2【中】`interfaces_exposed[0]` 给了过期锚，而它正是下游实际会读的那个载体

```
interfaces_exposed[0].symbol:
  「nanoclaw worktree = …/wt-warp-hestia，分支 feat/warp-hestia，本任务 HEAD = aff2521433eef8567addcedc988fecad3ed08c6f」
实际分支 HEAD = 7ca6b57e64a561351d7108a937b8607a5e87f0a6
```

交付文档 ③节写对了（「TASK-006 接着 `7ca6b57e…` 往下走」），但 CLAUDE.md 规定下游经 `context_from` 读的是 **discovery 的 `interfaces_exposed`**。**两个载体不一致，而下游读的是错的那个。**

实际风险有限（TASK-006 被要求「在同一分支上继续」，checkout 分支自然拿到 `7ca6b57`），但这正是「决定的载体决定它会不会被执行」的形状。⇒ 建议在 `review_fix` 里把该条改成 `7ca6b57e64a561351d7108a937b8607a5e87f0a6`。

### F3【机制】跨仓库任务的 `verify_baseline` 有结构性盲区

`verify_baseline` 只快照 **atlas** 的 `HEAD` + `discoveries/<ID>.json` 的 sha256。而本任务（以及 TASK-004、TASK-006）的**交付物主体全在 nanoclaw**。

本次 nanoclaw 在派验后 **66 秒**就多了一个 commit（`7ca6b57`）。它被发现，**是因为 atlas 侧的交付文档也一起改了**才触发了 `--ack-drift`。**若 dev 只改 nanoclaw、不动 atlas 文档，基线会完全一致、零告警，而我会静默地判了另一棵树** —— 而 worktree 的工作树默认就是新版，照交付文档的命令裸跑只会量到新版。

这与 AD-29 立项时那个案例是同一个失效模式，只是**载体从「同仓库的后续 commit」换成了「另一个仓库」**。⇒ 建议：跨仓库任务的 `verify_baseline` 增记外部仓库锚（或至少在 DoD 里要求验证者把外部仓库 HEAD 记进验证报告并与 discovery 比对）。本报告已按后者做（§4）。

### F4【我自己的仪器故障，非交付缺陷】`sort -u` 与 `comm` 同源同坏，而 DoD 只钉死了 `comm`

DoD 明令「实现工具必须用 `grep -Fxf`，不得用 `comm -12`」。但**建 `grep -Fxf` 的模式文件时最自然的写法正是 `grep '^## ' | sort -u`** —— 而在 `LANG=en_US.UTF-8` 下：

```
methodology '## '  原始 5  →  sort -u 后 3   （LC_ALL=C sort -u 与 python set 都给 5）
源文件      '## '  原始 12 →  sort -u 后 11  （LC_ALL=C sort -u 与 python set 都给 12）
被吃掉的行: ## 五条传导链（跨字段关联的提示） / ## 写作纪律 …
```

**我第一轮就是这么建的集合，因此一度数出「methodology 只有 3 个 `## `、源文件只有 11 个」，与交付文档声称的 5 / 12 不符** —— 差一步就把一份正确的自证报成缺陷。是先去看 `grep -n '^#\{1,4\} '` 的原始输出才发现被折叠的是我的仪器，不是它的数字。

⇒ 这条不针对本交付（它的数字是对的），是给后续验证者与 TASK-006 的：**`comm` 的坏是「比较」环节的坏，`sort -u` 的坏是「去重」环节的坏，同一个 collation 根因，而 DoD 只堵了前者**。`grep -Fxf` 不需要排序也不需要去重，**直接喂原始 grep 输出即可**；要去重就用 `LC_ALL=C sort -u` 或 python set。方向是**假阴**（左侧被折叠 ⇒ 交集偏小），比 `comm` 的假阳更隐蔽——它会让一份**真该判红**的交付看起来合格。

---

## 四、验证对象漂移（AD-29）—— 本任务发生了，已核清并显式确认

### 时间线（+0800）

| 时刻 | 事件 | 机制是否告警 |
| --- | --- | --- |
| 12:07:30 | nanoclaw `aff2521433eef8567addcedc988fecad3ed08c6f`（四份文档新建） | — |
| 12:12:42 | atlas `c73dbe3b7aabf85df0f31e503d2ca50aa83a8d36`（交付文档 415 行） | — |
| 12:21:32 | atlas merge `1991c49ec5541fd4d4c0d0cd23117b389e7d00d1` ← **verify_baseline.head** | — |
| **12:26:25** | **Leader 派验（`dev_done → verifying`）** | 基线在此刻拍下 |
| 12:27:31 | nanoclaw `7ca6b57e64a561351d7108a937b8607a5e87f0a6`（修 SKILL.md Step 3 + note-format 补口径） | 🔴 **无**（见 F3） |
| 12:30:26 | atlas `41ae450ff460a54cadd1097871595ad60ce19cbf`（交付文档 +148/−16，**在我的声明范围内**） | ✅ `--ack-drift` |
| ~12:31 | `.arcforge/discoveries/TASK-005.json` 整份重写（`10dd7fde…` → `b59695cf…`） | ✅ `--ack-discovery-drift`；但 `transitions.jsonl` **无审计行** |
| 12:33:34 | atlas merge `7e8458474128e9c27c8e51628f3e3758e81f4888` | — |

### 我怎么处置的

**没有条件反射地 ack。** 先做了三件事：

1. **分离两个版本**：用 `git show <全 sha>:<path>` 把 `aff2521` 与 `7ca6b57` 两版四份文档各取出一份到隔离目录，**不在 worktree 里裸跑**（worktree 工作树已是 `7ca6b57`，裸跑量到的是新版而非基线锚定的版本）。
2. **两版各跑一遍全部判据**，见 §1 矩阵最右两列：**七条在两个版本上都 PASS**。
3. **读修订的完整 diff**：nanoclaw `+7/−1`（Step 3 的 `$N` 改由 `prepare.py --print-name` 打印 + 一段理由；note-format 补 4 行说明 `## ` 计数的两个口径），atlas `+148/−16`（更新采样口径与锚点表、补 ④节 G 记录订正、补 2.3 的 `comm` 假阳可复现证据）。**修订是严格改进：没有删除任何已验证内容，没有放宽任何判据，新增数字我全部复算无误。**

⇒ 判定**不因漂移而失效**，据此以显式 `--ack-drift` / `--ack-discovery-drift` 放行。确认值取**转移前现读的当前值**：

```
atlas HEAD          = 7e8458474128e9c27c8e51628f3e3758e81f4888
discovery sha256    = b59695cfa3b43ce2b9cf24182bda4c9e93f846e165dd60d47f50aab81fa92105
nanoclaw 分支 HEAD  = 7ca6b57e64a561351d7108a937b8607a5e87f0a6   ← 机制不覆盖，由本报告记录（F3）
```

### 两条派验前提在我承接时已经失效

Leader 派验消息里的两条，与我实测不符，**都不影响判定，但影响后续处置**：

1. 「**dev 来不及在派验前改完**」——实际 **12:27:31 已改完**（派验后 66 秒）。`SKILL.md` Step 3 的 `$N` 现在是 `$(python3 /app/skills/warp-hestia/scripts/prepare.py --print-name $Q/processing/$F)`，与 `note-format.md` 的「不要自己拼文件名」以及**需求原文**（「`prepare.py` 的 `--print-name` 打印同一规则，**SKILL 用它取 `$N`**」）三方一致。⇒ **计划中 `review_fix` 的这一项已无需返工。**
2. 「**dev 在 `verifying` 期间也写不了 discovery**」——**不成立**。写通道的 discovery 时机守卫（`arcforge-write.sh:986-991`）只在 `status ∈ {verified, accepted}` 时 DENY；`verifying` 期间覆盖写 **exit 0、无告警、无审计行**。dev 确实在 12:31 写了。⇒ 这条如果被当成「不可能发生」而据以规划，会漏掉一整类漂移。

---

## 五、结论

**✅ VERIFIED**（带显式漂移确认）。

7 条 done_criteria **逐条 PASS，且在漂移前后两个版本上各验一遍都 PASS**，无未覆盖标准、无空洞断言。交付文档 ②节的每一条自证数字我都重新求值过，**无一不符**；`comm` 假阳的可复现证据我逐行验证为真。

**按 Leader 指示单列、不计入 verdict 的已知项**：`SKILL.md` Step 3 与 `note-format.md` 的自相矛盾。
🔴 **状态更新：该项已在 nanoclaw commit `7ca6b57e64a561351d7108a937b8607a5e87f0a6` 修复并经我核实**（Step 3 现调 `--print-name`，且补了「为什么」——五种 `period_type` 只有 `monthly` 带月份，拼错会让 Step 5 的 `mode` 把 `update` 判成 `create`）。**这一项不需要再走 `review_fix`。**

**建议进 `review_fix` 的两项**（均为 discovery 载体问题，不影响四份交付文档本身）：

1. **F2 优先**：`interfaces_exposed[0]` 的 `本任务 HEAD` 改为 `7ca6b57e64a561351d7108a937b8607a5e87f0a6` —— 它是 TASK-006 经 `context_from` 实际会读的锚。
2. **F1**：`verification.sampled_on` 与 `files_modified` 补第二轮口径，消除同一份 discovery 里两句互相矛盾的采样声明。

**给 TASK-006 的硬信息**：worktree `/Users/zuowei/workspace/ai/wt-warp-hestia`，分支 `feat/warp-hestia` @ **`7ca6b57e64a561351d7108a937b8607a5e87f0a6`**（**不是** discovery 里写的 `aff2521…`），工作区干净、**未拆**，在其上加 `scripts/`。`note-format.md` 的两级校验规格是 `verify.py` 的唯一契约：**check 恰 2 条 / seal 恰 1 条，条数写死，不从 `## ` 标题数推**。求集合交集时**既不要用 `comm -12`，也不要用 `sort -u` 预处理**（见 F4）。

---

# 返工验证（review_fix 轮）— F1 `--existing` 守卫

> 本节由 **test-m3-d** 于 2026-09-09 追加。**上方原始验证内容（test-m3-a 撰写）一字未改**——
> 该文件被 `CONTRACTS.md` 等下游引用，覆盖会使既有引用失效，故采取追加而非重写。

- **验证者**：test-m3-d ｜ **assignment_epoch**：1 ｜ **rework_count**：1
- **判定对象**：nanoclaw `feat/warp-hestia` @ `2a6d39388e4648eb3be0f15b42ee28b52428c5d8`
- **atlas 锚**：`dc2fcefd2264d8391de1425e0e8f21ef0f0b30fc` == `verify_baseline.head`，discovery sha256
  `22cc2cdbc372577f6343f779a75bd36c194241b3cd25745da97011cd9355b422` == baseline ⇒ **无漂移**

## 🔴 结论：**VERIFIED**（F1 缺陷已闭合）

## 1. 缺陷与修法

`SKILL.md` Step 3 原用 `${EXISTING:+--existing "$EXISTING"}` 判断，而 `$EXISTING` 是**无条件赋值、恒非空**
⇒ 判的是「变量非空」而非「文件存在」，**create 场景**（每期首次生成、笔记尚不存在）会把不存在的路径
传给 `--existing`，`prepare.py` 返回 2、产出 0 字节，每期首次都被移进 `failed/`。

**实际修法是显式 `if/else`**（`SKILL.md:65`），**不是 `fix_items` 里写的 `ARGS=()` 数组写法**——
后者已由 Leader 判定为错并作废，本报告**未以其为验收标准**：

```bash
if [ -f "$EXISTING" ]; then
  python3 …/prepare.py $Q/processing/$F $Q/processing/$H --existing "$EXISTING" > /tmp/note.md
else
  python3 …/prepare.py $Q/processing/$F $Q/processing/$H > /tmp/note.md
fi
```

源码注释另记明为何不用数组：**bash 3.2（macOS 自带）配 `set -u` 时空数组的 `"${ARR[@]}"` 会报
unbound variable，而 create 场景下它恰恰是空的**。这条理由经我实测成立（见 §3）。

## 2. 六格矩阵 —— 我自己跑出来的，不是采信 dev 的表格

派验指示要求「实际验证两个字节数确实不同，而不是采信它的表格」。我照抄 `SKILL.md` 的 if/else 逻辑
（唯一改动：容器路径→worktree 路径），**在 `set -u` 下**用三个 shell × 两场景各跑一次：

| shell | 场景 | exit | 字节数 |
|---|---|---|---|
| bash | create | 0 | **3247** |
| bash | update | 0 | **3263** |
| sh | create | 0 | **3247** |
| sh | update | 0 | **3263** |
| zsh | create | 0 | **3247** |
| zsh | update | 0 | **3263** |

- **两场景可区分：`3247 ≠ 3263`** ✅ ——这正是 dev 自己写的判据「若两场景输出相同，那个矩阵就只
  证明了『没崩』而不是『守卫在起作用』」。该判据成立，且**结论经我独立复现**。
- 三个 shell 的**同场景**输出彼此**逐字节一致**（`cmp` 验证）⇒ 跨 shell 行为一致。
- 与 dev 报的 3247 / 3263 **逐字节吻合**。

## 3. 消融实验 —— 证明缺陷确实存在，而非「现在能跑」

把守卫换回缺陷写法 `${EXISTING:+--existing "$EXISTING"}`，其余不变：

| shell | 场景 | exit | 字节 | stderr |
|---|---|---|---|---|
| bash | create | **2** | **0** | `--existing 读取失败: No such file or directory` |
| bash | update | 0 | 3263 | — |
| sh | create | **2** | **0** | 同上 |
| sh | update | 0 | 3263 | — |
| zsh | create | **2** | **0** | `usage: prepare.py …` |
| zsh | **update** | **2** | **0** | `usage: prepare.py …` |

⇒ 缺陷**完全复现**（create 场景 exit 2 / 0 字节，与修前实测一致）。

### 🔴 一处新发现：原缺陷比 `fix_items` 描述的更严重

**在 zsh 下，缺陷写法 create 与 update 两个场景都坏。** 成因：zsh 默认不做单词分割，
`${EXISTING:+--existing "$EXISTING"}` 整体展开成**一个** argv 元素，argparse 直接报 usage 错误。
bash / sh 会做单词分割，所以只有 create 坏。

- 原缺陷描述只说「create 场景」坏 ⇒ **修复的闭合范围比 `fix_items` 描述的更大**。
- 这同时**印证了源码注释「if/else 在 sh / bash 3.2 / zsh 都对」有实测支撑**——六格全绿即是证据。

## 4. 回归

`python3 -m unittest -v` → **`Ran 95 tests in 2.918s` / `OK` / EXIT=0**（退出码单独取、不跨管道）。
分层两把独立的尺同值：静态 `grep -c '    def test_'` = 63 + 32 = 95；动态 `Ran 63` / `Ran 32` = 95。
新增 11 = (63−55) + (32−29) = 8 + 3，与 dev 报「原 84 + 新增 11」一致。

新增守卫测试 `ExistingGuard`（`test_prepare.py`）：`test_existing_nonexistent_exits_2`（断言 rc==2、
stderr 含 `existing`、**stdout=="" 不得产出半份笔记**）+ `test_create_scenario_without_existing_works`。
其 docstring 点明了这个洞为何从未被行使：「现有三处 `--existing` 用例**全部指向存在的文件**……
**测试用例的取值分布让某条路径永不发生**」——这个归因是准确的。

## 5. 交付与范围

atlas 侧 `git show --numstat dc2fcefd…` → `119  0  docs/hestia-m3/TASK-005-warp-hestia-docs.md`
（唯一文件、与 `writes` 声明一致、**零删除行**）。文档含「返工记录」节。
nanoclaw 侧本次返工共 6 个文件，其中 `SKILL.md` 为 `16/2`（唯一有删除，即换掉 `${EXISTING:+…}` 那行），
其余均纯追加。**两份 golden 未被碰**（git 变更文件数 0）。

---

**验证者**：test-m3-d ｜ **F1 判定：VERIFIED** ｜ 六格矩阵与消融均为独立实测，非采信交付方表格

---

## 🔴 更正：上节「六格矩阵」的 shell 覆盖面被我高估了

> 由 **test-m3-d** 于 2026-09-09 追加，触发者为 Leader 的更正通报（dev-m3-c 补验后自报）。
> **上节内容保留原样未改** —— 它本身就是被更正的对象，改掉会让这条更正失去指涉。

### 事实

我在上节写的「三个 shell」「跨 shell 行为一致」**只覆盖了两个独立实现**。实测身份：

| 路径 | `$BASH_VERSION` | `$ZSH_VERSION` | 真身 |
|---|---|---|---|
| `/bin/bash` | `3.2.57(1)-release` | 无 | bash 3.2.57 |
| `/bin/sh` | **`3.2.57(1)-release`** | 无 | **就是 bash 3.2.57 的 POSIX 模式，不是 dash** |
| `/bin/dash` | 无 | 无 | 真 dash（Darwin 25 系统自带，无需 `brew install`） |
| `/bin/zsh` | 无 | `5.9` | zsh 5.9 |

⇒ 上节那六格 = **bash × 2（重复计数）+ zsh**，**dash 一次都没跑到**。
⚠️ **而六格全绿，从结果上完全看不出这个缺口** —— 这正是它能活到交付的原因。

### 补跑：三个**真正独立**的实现 × 两场景（`set -u`，worktree @ `2a6d3938…`）

| 实现 | 版本 | create | update |
|---|---|---|---|
| bash | 3.2.57 | EXIT=0 / **3247** | EXIT=0 / **3263** |
| **dash** | 系统自带 | EXIT=0 / **3247** | EXIT=0 / **3263** |
| zsh | 5.9 | EXIT=0 / **3247** | EXIT=0 / **3263** |

⇒ **交付的 `if/else` 实现在含 dash 的三个实现上全绿，两场景仍可区分（3247 ≠ 3263）。**
**上节的判定结论不变，变的是证据的覆盖面。**

### `ARGS=()` 对照：两种**不同**的失效，出现在**不同的行**

| 实现 | exit | 错误 |
|---|---|---|
| bash | 1 | `line 8: ARGS[@]: unbound variable`（空数组展开 + `set -u`） |
| sh | 1 | 同上（因 `sh` == bash） |
| **dash** | 2 | **`6: Syntax error: "(" unexpected`**（`ARGS=()` 赋值语法非 POSIX，**解析阶段、更早的一行**） |
| **zsh** | **0** | **无** —— **唯一两条都躲过的** |

update 场景下 dash 同样在第 6 行语法错（语法错发生在解析阶段 ⇒ 两个场景都跑不起来）。

⇒ **`ARGS=()` 写法坏在两个彼此独立的原因上**，分别只在 dash 和 bash 上显形，而 zsh 两条都躲过。
这解释了该写法为何能被写进 `fix_items`：**在写它的人手上的那个 shell 上，它是绿的。**

> ⚠️ **仪器留痕（位置已精确化，2026-09-09 二次核实）**：补跑分两轮。
> **上面这张 `ARGS=()` 对照表**的第二轮（为取完整错误消息而重跑）**复用了同一个输出文件名**
> （`rf2-o.md` / `rf2-o2.md`），dash 因语法错在解析阶段就退出、未写出文件，于是 `wc -c`
> 读到的是 **bash 上一次写入的残留值**。
>
> **该受污染的值未进入本报告任何表格或结论**——`ARGS=()` 对照表只列 `exit` 与错误消息，
> **没有字节数列**。dash 的 `exit=2` 与 `6: Syntax error: "(" unexpected` 是真实的。
>
> 🔴 **上一节 if/else 六格表不受此影响**：那一轮每格用**独立输出文件名**
> （`rf2-{bash,dash,zsh}-{create,update}.md`，六个文件至今在盘上，
> dash 两格分别为 3247 / 3263 字节）⇒ **表中 dash 的 update 3263 是 dash 自己跑出来的，不是残留。**
> Leader 事后独立重跑亦得 dash update = 3263，与此互证。
>
> ⚠️ 但**「残留恰好等于真值」这件事本身才是这条留痕的价值所在**：`prepare.py` 输出确定性，
> 上一次跑的正是同一个 update 场景，故残留 = 真值。**若上一次跑的是 create，残留会是 3247**——
> 那会让对照表出现「dash create 3247 / dash update 3247」，读者据此得到
> 「dash 上两场景不可区分」这个**假结论**，而它长得和真结论一模一样：没有空值、没有异常、
> 没有报错，两个数都是「合理且在别处出现过」的值。**唯一能识破它的是「这个数是这次跑出来的吗」,
> 而那正是复用文件名抹掉的信息。**

### 关于 `SKILL.md:64` 注释的判定 —— **finding【低】，非缺陷，不返工**

注释现写「if/else 在 **sh / bash 3.2 / zsh** 都对」，两处措辞偏弱：①「sh」与「bash 3.2」**是同一个
实现**，列作两项会让读者以为覆盖了两种；②**未提 dash**。

**判定为非缺陷，依据三条**（与代价无关）：

1. 注释断言的**性质**——「if/else 在这些 shell 上都对」——**经我实测为真，且在未被它列出的 dash 上同样为真**。
   即：它的结论不因覆盖面措辞而变假，**方向是低报自己的适用范围，不是高报**。
2. 缺陷的定义是「实现与 `done_criteria` 不符」。注释的证据面措辞**不构成实现缺陷**，
   `done_criteria` 未对注释的 shell 列举方式提出要求。
3. 本报告已记录真实覆盖面（含 dash 实测），**充当该注释的补充证据载体** —— 缺口已被文档化，
   不会静默留存。

（Leader 已表明其倾向亦为不单开 `review_fix`。**本判定的依据是上述三条，不是返工代价。**）

### 方法论：仪器对两种相反成因给出同一读数

`sh` 这个**名字**对「dash」和「bash 的 POSIX 模式」给出**同一个调用形式**，而两者是不同实现。
这与 `unrecognized arguments` 对「参数被拆开」和「参数被粘连」给出同一条错误消息是同一类问题：
**读数相同 ≠ 成因相同**。

⇒ **处方：别信名字，去问它的版本。** 本报告因此把身份探针
（`"$s" -c 'echo ${BASH_VERSION:-${ZSH_VERSION:-dash}}'`）的输出**作为证据的一部分**列出，
而不只是列出 shell 的名字。「我试过了」这句话必须能答上「**在哪个实现上试的**」。

---

**更正者**：test-m3-d ｜ **F1 判定维持 VERIFIED**（覆盖面已补齐，结论不变）

### 补充：探针原样输出、dash 数据的来源、以及本轮变异的实际范围

（应 Leader 2026-09-09 要求补记。**上方两节均未改动**。）

#### 1. 我跑的 `sh` 是什么 —— 原样命令与输出

```
$ sh   -c 'echo ${BASH_VERSION:-not-bash}'
3.2.57(1)-release
$ bash -c 'echo ${BASH_VERSION:-not-bash}'
3.2.57(1)-release
$ dash -c 'echo ${BASH_VERSION:-not-bash}'
not-bash
$ zsh  -c 'echo ${BASH_VERSION:-not-bash}'
not-bash
```

`sh` 与 `bash` 打印**同一个版本号** ⇒ 二者是同一个实现，`sh` 是 bash 3.2.57 的 POSIX 模式。

#### 2. dash 那格是**我自己跑的**，不是转述 dev-m3-c 的补验

Leader 的通报建议「若装不上 dash 就如实标注来源」。**这台机器上 `/bin/dash` 是系统自带的**
（Darwin 25；`ls -la /bin/dash` 存在，`brew list dash` 为空），所以我**没有**采用转述路径：

- 上一节表格里 dash 的 `create 3247 / update 3263` 与 `ARGS=()` 的 `6: Syntax error: "(" unexpected`
  **都是我在 worktree `@2a6d3938…` 上直接执行得到的**。
- 与 dev-m3-c 补验数据的**唯一差异是 bash 侧的行号**：它报第 7 行、我实测第 8 行——因为两人
  构造的 harness 脚本不同，**报的是各自脚本里的行**。**失效的性质一致**（空数组展开 + `set -u`），
  与 dash 第 6 行的语法错**是两个不同的失效**这一结论不受影响。

#### 3. 🔴 本轮变异的实际范围（严格度留痕）

**我没有逐个复算 dev 转述的「10 个变异全部 KILLED、0 存活」。** 我做的是**三个针对性消融**
（F1 换回 `${EXISTING:+…}` / F2 `assert_pair` 恒真 / F3 形状断言恒假），三个都复现了各自缺陷
修前的观测值（`0 字节` / `3161 字节` / `exit 0`）。

- 二者回答的**不是同一个问题**：那 10 个变异是 dev 为其全部改动设计的、选点带随机性；
  三个消融是把**这三条修复各自的守卫**拆回缺陷态，直接回答「这条修复真的闭合了那个缺陷吗」。
- 消融的证据强度更高之处在于：它把**修前的观测**与**消融后的观测**对上了（三个数字逐字吻合），
  而 KILLED 计数只说明「有断言变红」。
- 但**这不等于那 10 个变异被复核过**。若将来有人要质疑这批断言的整体强度，
  **此处是本轮未经二次确认的环节**。

#### 4. 方法论：仪器对两种不同成因给出同一读数（本报告内已有第三例）

| 仪器 | 成因 A | 成因 B | 读数 |
|---|---|---|---|
| `sh` 这个**名字** | dash | bash 的 POSIX 模式 | 同一个调用形式 |
| `unrecognized arguments` | 参数被拆开 | 参数被粘成一个 | 同一条错误消息 |
| **`sort -u`（`LANG=en_US.UTF-8`）** | 真重复行 | **按 collation 折叠的不同行** | 同样「少了几行」 |

第三行不是新举的例子——**它就在本文件上方 test-m3-a 写的原始验证内容里**（`sort -u` 与 `comm`
同根，而 DoD 当时只堵了 `comm`，方向是**假阴**：交集偏小会让真该判红的看起来合格）。
三者同族：**读数相同 ≠ 成因相同。**

⇒ **处方一句话：「我试过了」要能答得上「在哪个实现上试的」。**
本报告因此把身份探针的原样输出（§1）作为证据的一部分，而不只写 shell 的名字。

---

**补记者**：test-m3-d ｜ **判定仍为 VERIFIED，本节不改变任何判定**
